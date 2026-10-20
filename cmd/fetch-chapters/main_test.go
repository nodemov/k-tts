package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseArgs(t *testing.T) {
	documentID, outputDir, err := parseArgs([]string{"doc-123", "exports"})
	if err != nil {
		t.Fatal(err)
	}
	if documentID != "doc-123" || outputDir != "exports" {
		t.Fatalf("parseArgs() = (%q, %q), want (%q, %q)", documentID, outputDir, "doc-123", "exports")
	}

	documentID, outputDir, err = parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if documentID != "" || outputDir != "chapters" {
		t.Fatalf("parseArgs() = (%q, %q), want empty document ID and default output directory", documentID, outputDir)
	}
}

func TestWriteChapters(t *testing.T) {
	outputDir := t.TempDir()
	chapters := []chapter{
		{start: 1, end: 200, content: "first chapter"},
		{start: 201, end: 400, content: "second chapter"},
	}

	if err := writeChapters(outputDir, chapters); err != nil {
		t.Fatal(err)
	}

	for _, item := range chapters {
		path := filepath.Join(outputDir, chapterFilename(item.start, item.end))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != item.content {
			t.Fatalf("%s = %q, want %q", path, string(data), item.content)
		}
	}
}
