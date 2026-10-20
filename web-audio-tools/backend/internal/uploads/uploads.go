// Package uploads implements resumable chunked uploads stored on disk.
package uploads

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrNotFound       = errors.New("upload not found")
	ErrOffsetMismatch = errors.New("upload offset mismatch")
	ErrTooLarge       = errors.New("upload exceeds declared size")
	ErrIncomplete     = errors.New("upload is not complete")
)

type Upload struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Offset    int64     `json:"offset"`
	Complete  bool      `json:"complete"`
	SHA256    string    `json:"sha256,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type upload struct {
	Upload
	mu   sync.Mutex
	path string
}

type Store struct {
	dir     string
	maxSize int64

	mu      sync.RWMutex
	uploads map[string]*upload
}

func NewStore(dir string, maxSize int64) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir, maxSize: maxSize, uploads: map[string]*upload{}}, nil
}

func NewID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Store) Create(name string, size int64) (Upload, error) {
	if size <= 0 || size > s.maxSize {
		return Upload{}, fmt.Errorf("size must be between 1 and %d bytes", s.maxSize)
	}
	u := &upload{Upload: Upload{
		ID:        NewID(),
		Name:      filepath.Base(name),
		Size:      size,
		CreatedAt: time.Now(),
	}}
	u.path = filepath.Join(s.dir, u.ID+filepath.Ext(u.Name))
	f, err := os.Create(u.path)
	if err != nil {
		return Upload{}, err
	}
	f.Close()

	s.mu.Lock()
	s.uploads[u.ID] = u
	s.mu.Unlock()
	return u.Upload, nil
}

func (s *Store) get(id string) (*upload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.uploads[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

// Status returns a copy of the upload state.
func (s *Store) Status(id string) (Upload, error) {
	u, err := s.get(id)
	if err != nil {
		return Upload{}, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.Upload, nil
}

// Append writes a chunk at `offset`, which must equal the bytes received so far.
// A partially written chunk still advances the offset so the client can resume.
func (s *Store) Append(id string, offset int64, r io.Reader) (int64, error) {
	u, err := s.get(id)
	if err != nil {
		return 0, err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.Complete || offset != u.Offset {
		return u.Offset, ErrOffsetMismatch
	}
	f, err := os.OpenFile(u.path, os.O_WRONLY, 0)
	if err != nil {
		return u.Offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return u.Offset, err
	}
	remaining := u.Size - u.Offset
	n, err := io.Copy(f, io.LimitReader(r, remaining+1))
	if n > remaining {
		n = remaining
		_ = f.Truncate(u.Size)
		u.Offset += n
		return u.Offset, ErrTooLarge
	}
	u.Offset += n
	return u.Offset, err
}

// Complete verifies the size and hashes the content (used as the job cache key).
func (s *Store) Complete(id string) (Upload, error) {
	u, err := s.get(id)
	if err != nil {
		return Upload{}, err
	}
	u.mu.Lock()
	if u.Offset != u.Size {
		u.mu.Unlock()
		return Upload{}, ErrIncomplete
	}
	if !u.Complete {
		sum, err := hashFile(u.path)
		if err != nil {
			u.mu.Unlock()
			return Upload{}, err
		}
		u.SHA256 = sum
		u.Complete = true
	}
	u.mu.Unlock()
	return s.Status(id)
}

// Path returns the file path of a completed upload.
func (s *Store) Path(id string) (string, Upload, error) {
	st, err := s.Status(id)
	if err != nil {
		return "", st, err
	}
	if !st.Complete {
		return "", st, ErrIncomplete
	}
	u, _ := s.get(id)
	return u.path, st, nil
}

// Cleanup removes uploads older than maxAge.
func (s *Store) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, u := range s.uploads {
		if u.CreatedAt.Before(cutoff) {
			os.Remove(u.path)
			delete(s.uploads, id)
		}
	}
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
