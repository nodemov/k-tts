package uploads

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestResumableUpload(t *testing.T) {
	s, err := NewStore(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	content := "hello resumable world"
	u, err := s.Create("../../evil/clip.wav", int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "clip.wav" {
		t.Fatalf("name not sanitised: %q", u.Name)
	}

	off, err := s.Append(u.ID, 0, strings.NewReader(content[:5]))
	if err != nil || off != 5 {
		t.Fatalf("first chunk: off=%d err=%v", off, err)
	}
	if _, err := s.Append(u.ID, 0, strings.NewReader("xx")); !errors.Is(err, ErrOffsetMismatch) {
		t.Fatalf("expected offset mismatch, got %v", err)
	}
	if _, err := s.Complete(u.ID); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("expected incomplete, got %v", err)
	}
	if off, err = s.Append(u.ID, 5, strings.NewReader(content[5:])); err != nil || off != int64(len(content)) {
		t.Fatalf("second chunk: off=%d err=%v", off, err)
	}

	st, err := s.Complete(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(content))
	if st.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha mismatch")
	}
	path, _, err := s.Path(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != content {
		t.Fatalf("content = %q", data)
	}
}

func TestRejectsOversize(t *testing.T) {
	s, _ := NewStore(t.TempDir(), 10)
	if _, err := s.Create("a.wav", 11); err == nil {
		t.Fatal("expected size error")
	}
	u, _ := s.Create("a.wav", 4)
	off, err := s.Append(u.ID, 0, strings.NewReader("123456"))
	if !errors.Is(err, ErrTooLarge) || off != 4 {
		t.Fatalf("off=%d err=%v", off, err)
	}
}
