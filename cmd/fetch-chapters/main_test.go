package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "DATABASE_URL=postgresql://u:p@localhost:5432/db\n# comment\nOTHER=keep\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DATABASE_URL", "")
	_ = os.Unsetenv("DATABASE_URL")
	_ = os.Unsetenv("OTHER")

	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DATABASE_URL"); got != "postgresql://u:p@localhost:5432/db" {
		t.Fatalf("DATABASE_URL = %q", got)
	}

	t.Setenv("DATABASE_URL", "already-set")
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("DATABASE_URL"); got != "already-set" {
		t.Fatalf("existing env should win, got %q", got)
	}
}

func TestEnsureSSLMode(t *testing.T) {
	got, err := ensureSSLMode("postgresql://u:p@localhost:5432/db")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("ensureSSLMode() = %q, want sslmode=disable", got)
	}

	got, err = ensureSSLMode("postgresql://u:p@localhost:5432/db?sslmode=require")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "sslmode=require") || strings.Contains(got, "sslmode=disable") {
		t.Fatalf("ensureSSLMode() = %q, want existing sslmode=require", got)
	}
}

func TestParseArgs(t *testing.T) {
	opts, err := parseArgs([]string{"book-123", "exports"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.bookID != "book-123" || opts.outputDir != "exports" || opts.from != 0 || opts.to != 0 {
		t.Fatalf("parseArgs() = %+v, want book+output_dir", opts)
	}

	opts, err = parseArgs([]string{"book-123", "1", "100"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.bookID != "book-123" || opts.from != 1 || opts.to != 100 || opts.outputDir != "chapters" {
		t.Fatalf("parseArgs() = %+v, want range 1-100", opts)
	}

	opts, err = parseArgs([]string{"book-123", "1", "100", "exports"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.from != 1 || opts.to != 100 || opts.outputDir != "exports" {
		t.Fatalf("parseArgs() = %+v, want range + output_dir", opts)
	}

	if _, err := parseArgs([]string{"book-123", "1"}); err == nil {
		t.Fatal("expected error when only from is provided")
	}
	if _, err := parseArgs([]string{"book-123", "100", "1"}); err == nil {
		t.Fatal("expected error when from > to")
	}

	opts, err = parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.bookID != "" || opts.outputDir != "chapters" {
		t.Fatalf("parseArgs() = %+v, want empty book id and default output directory", opts)
	}
}

func TestChapterFilename(t *testing.T) {
	if got := chapterFilename(0); got != "0001.txt" {
		t.Fatalf("chapterFilename(0) = %q, want %q", got, "0001.txt")
	}
	if got := chapterFilename(74); got != "0075.txt" {
		t.Fatalf("chapterFilename(74) = %q, want %q", got, "0075.txt")
	}
}

func TestFormatChapterContent(t *testing.T) {
	got := formatChapterContent(chapter{
		title: "บทที่ 1",
		text:  "เนื้อเรื่อง",
	})
	want := "บทที่ 1\n\nเนื้อเรื่อง\n"
	if got != want {
		t.Fatalf("formatChapterContent() = %q, want %q", got, want)
	}
}

func TestWriteChapters(t *testing.T) {
	outputDir := t.TempDir()
	chapters := []chapter{
		{index: 0, title: "บทที่ 1", text: "first chapter"},
		{index: 1, title: "บทที่ 2", text: "second chapter"},
	}

	if err := writeChapters(outputDir, chapters); err != nil {
		t.Fatal(err)
	}

	for _, item := range chapters {
		path := filepath.Join(outputDir, chapterFilename(item.index))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want := formatChapterContent(item)
		if string(data) != want {
			t.Fatalf("%s = %q, want %q", path, string(data), want)
		}
	}
}
