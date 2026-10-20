// Package jobs runs long audio tasks on a bounded worker pool, tracks their
// progress for SSE subscribers and de-duplicates identical requests.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"k-tts/web-audio-tools/backend/internal/uploads"
)

type Status string

const (
	Queued  Status = "queued"
	Running Status = "running"
	Done    Status = "done"
	Failed  Status = "error"
)

var ErrQueueFull = errors.New("job queue is full, try again later")

type File struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type Job struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Status    Status         `json:"status"`
	Progress  float64        `json:"progress"`
	Message   string         `json:"message"`
	Error     string         `json:"error,omitempty"`
	Files     []File         `json:"files"`
	Result    map[string]any `json:"result,omitempty"`
	Cached    bool           `json:"cached"`
	CreatedAt time.Time      `json:"createdAt"`
}

// Task does the work. Output files must be written into r.Dir().
type Task func(ctx context.Context, r *Reporter) error

type entry struct {
	job  Job
	dir  string
	task Task
	key  string
	subs map[chan struct{}]struct{}
}

type Manager struct {
	dir     string
	timeout time.Duration
	queue   chan *entry

	mu    sync.Mutex
	jobs  map[string]*entry
	cache map[string]string
}

func NewManager(dir string, workers int, timeout time.Duration) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	m := &Manager{
		dir:     dir,
		timeout: timeout,
		queue:   make(chan *entry, 256),
		jobs:    map[string]*entry{},
		cache:   map[string]string{},
	}
	for i := 0; i < max(1, workers); i++ {
		go m.worker()
	}
	return m, nil
}

// Submit enqueues a task. If `cacheKey` matches a finished (or in-flight) job,
// that job is returned instead of doing the work twice.
func (m *Manager) Submit(kind, cacheKey string, task Task) (Job, error) {
	m.mu.Lock()
	if cacheKey != "" {
		if id, ok := m.cache[cacheKey]; ok {
			if e, ok := m.jobs[id]; ok && e.job.Status != Failed {
				snap := m.snapshot(e)
				snap.Cached = true
				m.mu.Unlock()
				return snap, nil
			}
		}
	}
	id := uploads.NewID()
	e := &entry{
		job:  Job{ID: id, Kind: kind, Status: Queued, Message: "queued", Files: []File{}, CreatedAt: time.Now()},
		dir:  filepath.Join(m.dir, id),
		task: task,
		key:  cacheKey,
		subs: map[chan struct{}]struct{}{},
	}
	select {
	case m.queue <- e:
	default:
		m.mu.Unlock()
		return Job{}, ErrQueueFull
	}
	m.jobs[id] = e
	if cacheKey != "" {
		m.cache[cacheKey] = id
	}
	snap := m.snapshot(e)
	m.mu.Unlock()
	return snap, nil
}

func (m *Manager) Get(id string) (Job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.jobs[id]
	if !ok {
		return Job{}, false
	}
	return m.snapshot(e), true
}

// Subscribe returns a channel that is signalled whenever the job changes.
// Signals coalesce, so readers should call Get for the latest state.
func (m *Manager) Subscribe(id string) (<-chan struct{}, func(), bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.jobs[id]
	if !ok {
		return nil, func() {}, false
	}
	ch := make(chan struct{}, 1)
	e.subs[ch] = struct{}{}
	return ch, func() {
		m.mu.Lock()
		delete(e.subs, ch)
		m.mu.Unlock()
	}, true
}

// FilePath resolves an output file, rejecting anything outside the job folder.
func (m *Manager) FilePath(id, name string) (string, error) {
	m.mu.Lock()
	e, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok || name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", os.ErrNotExist
	}
	return filepath.Join(e.dir, name), nil
}

// Cleanup deletes finished jobs (and their files) older than maxAge.
func (m *Manager) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, e := range m.jobs {
		if (e.job.Status == Done || e.job.Status == Failed) && e.job.CreatedAt.Before(cutoff) {
			os.RemoveAll(e.dir)
			delete(m.jobs, id)
			if e.key != "" && m.cache[e.key] == id {
				delete(m.cache, e.key)
			}
		}
	}
}

func (m *Manager) snapshot(e *entry) Job {
	j := e.job
	j.Files = append([]File{}, e.job.Files...)
	if e.job.Result != nil {
		j.Result = make(map[string]any, len(e.job.Result))
		for k, v := range e.job.Result {
			j.Result[k] = v
		}
	}
	return j
}

func (m *Manager) update(e *entry, fn func(j *Job)) {
	m.mu.Lock()
	fn(&e.job)
	for ch := range e.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	m.mu.Unlock()
}

func (m *Manager) worker() {
	for e := range m.queue {
		m.run(e)
	}
}

func (m *Manager) run(e *entry) {
	m.update(e, func(j *Job) {
		j.Status = Running
		j.Message = "starting"
	})
	err := os.MkdirAll(e.dir, 0o755)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
		err = safeRun(ctx, e.task, &Reporter{m: m, e: e})
		cancel()
	}
	files := listFiles(e.job.ID, e.dir)
	m.update(e, func(j *Job) {
		j.Files = files
		if err != nil {
			j.Status = Failed
			j.Error = err.Error()
			j.Message = "failed"
			return
		}
		j.Status = Done
		j.Progress = 1
		j.Message = "done"
	})
	if err != nil {
		slog.Warn("job failed", "id", e.job.ID, "kind", e.job.Kind, "err", err)
	} else {
		slog.Info("job done", "id", e.job.ID, "kind", e.job.Kind, "files", len(files))
	}
}

func safeRun(ctx context.Context, task Task, r *Reporter) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("internal error: %v", p)
		}
	}()
	return task(ctx, r)
}

func listFiles(id, dir string) []File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []File{}
	}
	files := []File{}
	for _, de := range entries {
		if de.IsDir() || strings.HasPrefix(de.Name(), ".") || strings.HasPrefix(de.Name(), "tmp-") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		files = append(files, File{Name: de.Name(), Size: info.Size(), URL: fmt.Sprintf("/api/jobs/%s/files/%s", id, url.PathEscape(de.Name()))})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
}

// Reporter is handed to tasks to publish progress and results.
type Reporter struct {
	m *Manager
	e *entry
}

func (r *Reporter) Dir() string { return r.e.dir }

func (r *Reporter) Progress(p float64, message string) {
	r.m.update(r.e, func(j *Job) {
		j.Progress = min(1, max(0, p))
		if message != "" {
			j.Message = message
		}
	})
}

func (r *Reporter) SetResult(key string, value any) {
	r.m.update(r.e, func(j *Job) {
		if j.Result == nil {
			j.Result = map[string]any{}
		}
		j.Result[key] = value
	})
}
