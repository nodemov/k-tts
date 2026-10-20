// Package media wraps the ffmpeg CLI with progress reporting.
package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// Available reports whether ffmpeg and ffprobe are on PATH.
func Available() bool {
	_, e1 := exec.LookPath("ffmpeg")
	_, e2 := exec.LookPath("ffprobe")
	return e1 == nil && e2 == nil
}

// Probe returns the duration in seconds.
func Probe(ctx context.Context, path string) (float64, error) {
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	return strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
}

// Run executes ffmpeg and maps `-progress` output to 0..1 via onProgress.
// It returns ffmpeg's stderr so callers can parse filter reports.
func Run(ctx context.Context, duration float64, args []string, onProgress func(float64)) (string, error) {
	full := append([]string{"-hide_banner", "-nostdin", "-y", "-progress", "pipe:1", "-nostats"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	scanProgress(stdout, duration, onProgress)
	if err := cmd.Wait(); err != nil {
		tail := stderr.String()
		if len(tail) > 1500 {
			tail = tail[len(tail)-1500:]
		}
		return stderr.String(), fmt.Errorf("ffmpeg failed: %w\n%s", err, tail)
	}
	return stderr.String(), nil
}

func scanProgress(r io.Reader, duration float64, onProgress func(float64)) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok || onProgress == nil || duration <= 0 {
			continue
		}
		if key == "out_time_us" || key == "out_time_ms" {
			if us, err := strconv.ParseFloat(value, 64); err == nil {
				onProgress(min(1, us/1e6/duration))
			}
		}
	}
}

// CodecArgs returns encoder arguments for an output format.
func CodecArgs(format string, bitrate int) ([]string, error) {
	if bitrate <= 0 {
		bitrate = 192
	}
	b := fmt.Sprintf("%dk", bitrate)
	switch format {
	case "mp3":
		return []string{"-c:a", "libmp3lame", "-b:a", b}, nil
	case "wav":
		return []string{"-c:a", "pcm_s16le"}, nil
	case "flac":
		return []string{"-c:a", "flac"}, nil
	case "m4a":
		return []string{"-c:a", "aac", "-b:a", b}, nil
	case "ogg":
		return []string{"-c:a", "libvorbis", "-b:a", b}, nil
	case "opus":
		return []string{"-c:a", "libopus", "-b:a", b}, nil
	}
	return nil, fmt.Errorf("unsupported format %q", format)
}

type EnhanceOptions struct {
	Loudness float64
	Denoise  bool
	Format   string
	Bitrate  int
}

type loudnormStats struct {
	InputI       string `json:"input_i"`
	InputTP      string `json:"input_tp"`
	InputLRA     string `json:"input_lra"`
	InputThresh  string `json:"input_thresh"`
	TargetOffset string `json:"target_offset"`
}

func parseLoudnorm(stderr string) (loudnormStats, error) {
	var stats loudnormStats
	start := strings.LastIndex(stderr, "{")
	end := strings.LastIndex(stderr, "}")
	if start < 0 || end < start {
		return stats, errors.New("loudnorm report not found")
	}
	err := json.Unmarshal([]byte(stderr[start:end+1]), &stats)
	return stats, err
}

// Enhance runs a two-pass EBU R128 loudness normalisation after clean-up filters.
func Enhance(ctx context.Context, in, out string, opts EnhanceOptions, onProgress func(float64, string)) error {
	duration, err := Probe(ctx, in)
	if err != nil {
		return err
	}
	pre := []string{"highpass=f=70"}
	if opts.Denoise {
		pre = append(pre, "afftdn=nr=12:nf=-35:tn=1")
	}
	pre = append(pre, "acompressor=threshold=-20dB:ratio=3:attack=10:release=200:makeup=2")
	chain := strings.Join(pre, ",")
	target := fmt.Sprintf("I=%.1f:TP=-1.5:LRA=11", opts.Loudness)

	stderr, err := Run(ctx, duration, []string{"-i", in, "-af", chain + ",loudnorm=" + target + ":print_format=json", "-f", "null", "-"},
		func(p float64) { onProgress(p*0.5, "analysing loudness") })
	if err != nil {
		return err
	}
	stats, err := parseLoudnorm(stderr)
	if err != nil {
		return err
	}
	codec, err := CodecArgs(opts.Format, opts.Bitrate)
	if err != nil {
		return err
	}
	second := fmt.Sprintf("%s,loudnorm=%s:measured_I=%s:measured_TP=%s:measured_LRA=%s:measured_thresh=%s:offset=%s:linear=true",
		chain, target, stats.InputI, stats.InputTP, stats.InputLRA, stats.InputThresh, stats.TargetOffset)
	args := append([]string{"-i", in, "-vn", "-af", second, "-ar", "48000"}, codec...)
	_, err = Run(ctx, duration, append(args, out), func(p float64) { onProgress(0.5+p*0.5, "rendering") })
	return err
}

// Convert transcodes to another format.
func Convert(ctx context.Context, in, out, format string, bitrate int, onProgress func(float64)) error {
	duration, err := Probe(ctx, in)
	if err != nil {
		return err
	}
	codec, err := CodecArgs(format, bitrate)
	if err != nil {
		return err
	}
	args := append([]string{"-i", in, "-vn"}, codec...)
	_, err = Run(ctx, duration, append(args, out), onProgress)
	return err
}
