package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k-tts/pkg/tts"
)

const usage = "usage: go run ./cmd/combine-audio [-speed N] [-fx name] [-loudness LUFS] [size] [input_dir] [output_dir]"

type combineOptions struct {
	size      int
	inputDir  string
	outputDir string
	speed     float64
	fx        tts.VoiceFX
	loudness  float64
}

type group struct {
	name  string
	files []string
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}

	files, err := listAudioFiles(opts.inputDir)
	if err != nil {
		fatal(err)
	}
	if len(files) == 0 {
		fatal(fmt.Errorf("no .mp3 files found in %s", opts.inputDir))
	}

	if err := os.MkdirAll(opts.outputDir, 0755); err != nil {
		fatal(fmt.Errorf("create output directory: %w", err))
	}

	groups := groupFiles(files, opts.size)
	fmt.Printf("found %d files, combining into %d files (%d per file)\n", len(files), len(groups), opts.size)
	if opts.speed != 1 {
		fmt.Printf("speeding up combined files by %.2fx\n", opts.speed)
	}
	if opts.fx.Name != "" {
		fmt.Printf("applying voice fx %q at %g LUFS: %s\n", opts.fx.Name, opts.loudness, opts.fx.Description)
	}
	for _, g := range groups {
		outputFile := filepath.Join(opts.outputDir, g.name+".mp3")
		if err := tts.ConcatFiles(g.files, outputFile); err != nil {
			fatal(fmt.Errorf("combine %s: %w", g.name, err))
		}
		if opts.speed != 1 || opts.fx.Name != "" {
			if err := tts.ApplyVoiceFX(outputFile, outputFile, opts.speed, opts.fx, opts.loudness); err != nil {
				fatal(fmt.Errorf("process %s: %w", g.name, err))
			}
		}
		fmt.Printf("  %s (%d files)\n", outputFile, len(g.files))
	}
}

func parseArgs(args []string) (combineOptions, error) {
	opts := combineOptions{size: 10, inputDir: "output"}

	fs := flag.NewFlagSet("combine-audio", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Float64Var(&opts.speed, "speed", 1, "speed multiplier applied after combining (atempo minimum is 0.5)")
	fxName := fs.String("fx", "", "voice fx preset applied after combining")
	fs.Float64Var(&opts.loudness, "loudness", tts.DefaultLoudness, "target loudness in LUFS for -fx (higher = louder)")
	if err := fs.Parse(args); err != nil {
		return combineOptions{}, fmt.Errorf("%s\n%v", usage, err)
	}
	if opts.speed < 0.5 {
		return combineOptions{}, fmt.Errorf("%s\nspeed must be >= 0.5", usage)
	}
	if opts.loudness < tts.MinLoudness || opts.loudness > tts.MaxLoudness {
		return combineOptions{}, fmt.Errorf("%s\nloudness must be between %g and %g LUFS", usage, tts.MinLoudness, tts.MaxLoudness)
	}
	fx, err := tts.LookupVoiceFX(*fxName)
	if err != nil {
		return combineOptions{}, fmt.Errorf("%s\n%v", usage, err)
	}
	opts.fx = fx
	loudnessSet := false
	fs.Visit(func(f *flag.Flag) { loudnessSet = loudnessSet || f.Name == "loudness" })
	if loudnessSet && fx.Name == "" {
		return combineOptions{}, fmt.Errorf("%s\n-loudness requires -fx", usage)
	}
	args = fs.Args()

	if len(args) > 3 {
		return combineOptions{}, errors.New(usage)
	}
	if len(args) >= 1 {
		size, err := strconv.Atoi(strings.TrimSpace(args[0]))
		if err != nil || size < 1 {
			return combineOptions{}, fmt.Errorf("%s\nsize must be an integer >= 1", usage)
		}
		opts.size = size
	}
	if len(args) >= 2 {
		opts.inputDir = strings.TrimSpace(args[1])
		if opts.inputDir == "" {
			return combineOptions{}, errors.New("input_dir cannot be empty")
		}
	}
	opts.outputDir = filepath.Join(opts.inputDir, "combined")
	if len(args) == 3 {
		opts.outputDir = strings.TrimSpace(args[2])
		if opts.outputDir == "" {
			return combineOptions{}, errors.New("output_dir cannot be empty")
		}
	}
	return opts, nil
}

// listAudioFiles returns *.mp3 in dir sorted by name, skipping leftover temp files from the TTS pipeline.
func listAudioFiles(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.mp3"))
	if err != nil {
		return nil, err
	}
	var result []string
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasSuffix(base, ".temp.mp3") || strings.HasSuffix(base, ".speed.mp3") {
			continue
		}
		result = append(result, file)
	}
	sort.Strings(result)
	return result, nil
}

func groupFiles(files []string, size int) []group {
	var groups []group
	for start := 0; start < len(files); start += size {
		end := min(start+size, len(files))
		chunk := files[start:end]
		first := strings.TrimSuffix(filepath.Base(chunk[0]), ".mp3")
		last := strings.TrimSuffix(filepath.Base(chunk[len(chunk)-1]), ".mp3")
		name := first
		if first != last {
			name = first + "-" + last
		}
		groups = append(groups, group{name: name, files: chunk})
	}
	return groups
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
