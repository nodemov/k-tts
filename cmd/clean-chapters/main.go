// clean-chapters writes an audiobook-ready copy of every chapter: the exact text
// that k-tts reads, one paragraph per line with blank lines between paragraphs.
// With -parts it also writes the TTS chunks so you can check where each request is cut.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"k-tts/pkg/tts"
)

func main() {
	in := flag.String("in", "chapters", "folder of chapter .txt files")
	out := flag.String("out", "chapters-clean", "folder for clean audiobook text")
	engine := flag.String("engine", "router", "engine whose chunk size is used for -parts: router, edge, translate, cloud")
	parts := flag.Bool("parts", false, "also write NNNN.parts.txt showing each TTS chunk")
	flag.Parse()

	maxLen, err := chunkLen(*engine)
	if err != nil {
		fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(*in, "*.txt"))
	if err != nil {
		fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		fatal(fmt.Errorf("no .txt files in %s", *in))
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}

	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			fatal(err)
		}
		clean := tts.FormatAudiobook(string(raw))
		name := strings.TrimSuffix(filepath.Base(file), ".txt")
		if err := os.WriteFile(filepath.Join(*out, name+".txt"), []byte(clean+"\n"), 0o644); err != nil {
			fatal(err)
		}
		if *parts {
			chunks := tts.SplitText(clean, maxLen)
			var b strings.Builder
			for i, c := range chunks {
				fmt.Fprintf(&b, "----- part %d (%d chars) -----\n%s\n\n", i+1, len([]rune(c)), c)
			}
			if err := os.WriteFile(filepath.Join(*out, name+".parts.txt"), []byte(b.String()), 0o644); err != nil {
				fatal(err)
			}
		}
	}
	fmt.Printf("wrote %d clean chapters to %s\n", len(files), *out)
}

func chunkLen(engine string) (int, error) {
	switch engine {
	case "router":
		return tts.RouterChunkLen, nil
	case "edge":
		return tts.EdgeChunkLen, nil
	case "translate":
		return tts.TranslateChunkLen, nil
	case "cloud":
		return tts.CloudChunkLen, nil
	}
	return 0, fmt.Errorf("unknown engine %q (use router, edge, translate, or cloud)", engine)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
