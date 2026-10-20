package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseLoudnorm(t *testing.T) {
	stderr := `[Parsed_loudnorm_0 @ 0x1] 
{
	"input_i" : "-27.61",
	"input_tp" : "-4.47",
	"input_lra" : "18.06",
	"input_thresh" : "-39.20",
	"output_i" : "-16.58",
	"target_offset" : "0.58"
}`
	stats, err := parseLoudnorm(stderr)
	if err != nil {
		t.Fatal(err)
	}
	if stats.InputI != "-27.61" || stats.TargetOffset != "0.58" {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestCodecArgs(t *testing.T) {
	if _, err := CodecArgs("exe", 0); err == nil {
		t.Fatal("expected error for unknown format")
	}
	args, _ := CodecArgs("mp3", 0)
	if args[len(args)-1] != "192k" {
		t.Fatalf("default bitrate: %v", args)
	}
}

func TestEnhanceWithFFmpeg(t *testing.T) {
	if !Available() {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "tone.wav")
	if out, err := exec.Command("ffmpeg", "-hide_banner", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=3", "-af", "volume=0.05", in).CombinedOutput(); err != nil {
		t.Fatalf("generate tone: %v\n%s", err, out)
	}
	out := filepath.Join(dir, "tone.mp3")
	var last float64
	err := Enhance(context.Background(), in, out, EnhanceOptions{Loudness: -16, Denoise: true, Format: "mp3", Bitrate: 128}, func(p float64, _ string) {
		if p < last {
			t.Errorf("progress went backwards: %v -> %v", last, p)
		}
		last = p
	})
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() == 0 {
		t.Fatalf("no output: %v", err)
	}
	if d, err := Probe(context.Background(), out); err != nil || d < 2.5 {
		t.Fatalf("duration %v err %v", d, err)
	}
}
