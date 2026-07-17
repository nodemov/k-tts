package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGoogleTTSWorkerFailsWhenAnyChunkFails(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requestCount.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte{1}, 1200))
			return
		}

		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	originalURL := googleTranslateTTSURL
	googleTranslateTTSURL = server.URL
	t.Cleanup(func() {
		googleTranslateTTSURL = originalURL
	})

	tempDir := t.TempDir()
	job := TTSJob{
		FilePath: "chapter.txt",
		Text:     strings.Repeat("ข้อความทดสอบที่ยาวพอสำหรับแบ่งเป็นหลายส่วน. ", 20),
	}

	audioFiles, err := processFileWithGoogleTTSWorker(job, tempDir)

	if err == nil {
		t.Fatal("expected a failed chunk to fail the entire job")
	}
	if requestCount.Load() < 2 {
		t.Fatalf("test input produced only %d request(s), want at least 2", requestCount.Load())
	}
	if len(audioFiles) != 1 {
		t.Fatalf("got %d completed chunks, want 1", len(audioFiles))
	}
	if filepath.Base(audioFiles[0]) != "temp_part_1.mp3" {
		t.Fatalf("unexpected completed chunk: %s", audioFiles[0])
	}
}
