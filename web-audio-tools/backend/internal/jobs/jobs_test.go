package jobs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitDone(t *testing.T, m *Manager, id string) Job {
	t.Helper()
	signal, unsubscribe, ok := m.Subscribe(id)
	if !ok {
		t.Fatalf("job %s not found", id)
	}
	defer unsubscribe()
	deadline := time.After(5 * time.Second)
	for {
		job, _ := m.Get(id)
		if job.Status == Done || job.Status == Failed {
			return job
		}
		select {
		case <-signal:
		case <-deadline:
			t.Fatalf("timeout waiting for job, last state %+v", job)
		}
	}
}

func TestJobLifecycleAndCache(t *testing.T) {
	m, err := NewManager(t.TempDir(), 2, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	runs := 0
	task := func(ctx context.Context, r *Reporter) error {
		runs++
		r.Progress(0.5, "half")
		r.SetResult("text", "hi")
		os.WriteFile(filepath.Join(r.Dir(), "out.txt"), []byte("ok"), 0o644)
		os.WriteFile(filepath.Join(r.Dir(), ".hidden"), []byte("x"), 0o644)
		return nil
	}
	job, err := m.Submit("transcribe", "key-1", task)
	if err != nil {
		t.Fatal(err)
	}
	if job.Files == nil {
		t.Fatal("files must serialise as [] while running")
	}
	done := waitDone(t, m, job.ID)
	if done.Status != Done || done.Progress != 1 {
		t.Fatalf("unexpected final state %+v", done)
	}
	if len(done.Files) != 1 || done.Files[0].Name != "out.txt" || done.Files[0].Size != 2 {
		t.Fatalf("files = %+v", done.Files)
	}
	if done.Result["text"] != "hi" {
		t.Fatalf("result = %+v", done.Result)
	}

	again, _ := m.Submit("transcribe", "key-1", task)
	if !again.Cached || again.ID != job.ID || runs != 1 {
		t.Fatalf("expected cache hit, got %+v (runs=%d)", again, runs)
	}
}

func TestFailedJobsAreNotCached(t *testing.T) {
	m, _ := NewManager(t.TempDir(), 1, time.Minute)
	fail := func(ctx context.Context, r *Reporter) error { return errors.New("boom") }
	job, _ := m.Submit("enhance", "k", fail)
	if got := waitDone(t, m, job.ID); got.Status != Failed || got.Error != "boom" {
		t.Fatalf("got %+v", got)
	}
	retry, _ := m.Submit("enhance", "k", fail)
	if retry.Cached || retry.ID == job.ID {
		t.Fatal("failed job must not be served from cache")
	}
}

func TestPanicIsRecovered(t *testing.T) {
	m, _ := NewManager(t.TempDir(), 1, time.Minute)
	job, _ := m.Submit("x", "", func(ctx context.Context, r *Reporter) error { panic("oops") })
	if got := waitDone(t, m, job.ID); got.Status != Failed {
		t.Fatalf("got %+v", got)
	}
}

func TestFilePathRejectsTraversal(t *testing.T) {
	m, _ := NewManager(t.TempDir(), 1, time.Minute)
	job, _ := m.Submit("x", "", func(ctx context.Context, r *Reporter) error { return nil })
	for _, name := range []string{"../secret", "a/b", ".hidden", ""} {
		if _, err := m.FilePath(job.ID, name); err == nil {
			t.Errorf("expected rejection for %q", name)
		}
	}
	if _, err := m.FilePath(job.ID, "ok.mp3"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
