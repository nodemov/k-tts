package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"k-tts/pkg/tts"
)

const usage = "usage: go run ./cmd/combine-audio [size] [input_dir] [output_dir]"

type combineOptions struct {
	size      int
	inputDir  string
	outputDir string
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
	for _, g := range groups {
		outputFile := filepath.Join(opts.outputDir, g.name+".mp3")
		if err := tts.ConcatFiles(g.files, outputFile); err != nil {
			fatal(fmt.Errorf("combine %s: %w", g.name, err))
		}
		fmt.Printf("  %s (%d files)\n", outputFile, len(g.files))
	}
}

func parseArgs(args []string) (combineOptions, error) {
	opts := combineOptions{size: 10, inputDir: "output"}
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
