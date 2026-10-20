// Package server exposes the HTTP API and serves the built frontend.
package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"

	"k-tts/web-audio-tools/backend/internal/ai"
	"k-tts/web-audio-tools/backend/internal/jobs"
	"k-tts/web-audio-tools/backend/internal/media"
	"k-tts/web-audio-tools/backend/internal/uploads"
)

const maxChunk = 64 << 20

type Config struct {
	Uploads    *uploads.Store
	Jobs       *jobs.Manager
	AI         *ai.Client
	TTS        *texttospeech.Client
	StaticDir  string
	CORSOrigin string
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) http.Handler {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("POST /api/uploads", s.createUpload)
	s.mux.HandleFunc("HEAD /api/uploads/{id}", s.headUpload)
	s.mux.HandleFunc("PATCH /api/uploads/{id}", s.patchUpload)
	s.mux.HandleFunc("POST /api/uploads/{id}/complete", s.completeUpload)
	s.mux.HandleFunc("POST /api/jobs", s.createJob)
	s.mux.HandleFunc("GET /api/jobs/{id}", s.getJob)
	s.mux.HandleFunc("GET /api/jobs/{id}/events", s.jobEvents)
	s.mux.HandleFunc("GET /api/jobs/{id}/files/{name}", s.jobFile)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	if cfg.StaticDir != "" {
		s.mux.HandleFunc("/", s.static)
	}
	return s.logging(s.cors(s.mux))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"ffmpeg":   media.Available(),
		"cloudTTS": s.cfg.TTS != nil,
		"ai":       s.cfg.AI.Health(r.Context()),
	})
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	u, err := s.cfg.Uploads.Create(body.Name, body.Size)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": u.ID, "offset": 0})
}

func (s *Server) headUpload(w http.ResponseWriter, r *http.Request) {
	st, err := s.cfg.Uploads.Status(r.PathValue("id"))
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(st.Offset, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(st.Size, 10))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) patchUpload(w http.ResponseWriter, r *http.Request) {
	offset, err := strconv.ParseInt(r.Header.Get("Upload-Offset"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing Upload-Offset header")
		return
	}
	next, err := s.cfg.Uploads.Append(r.PathValue("id"), offset, http.MaxBytesReader(w, r.Body, maxChunk))
	switch {
	case errors.Is(err, uploads.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, uploads.ErrOffsetMismatch):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "offset": next})
	case err != nil:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "offset": next})
	default:
		w.Header().Set("Upload-Offset", strconv.FormatInt(next, 10))
		writeJSON(w, http.StatusOK, map[string]any{"offset": next})
	}
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	st, err := s.cfg.Uploads.Complete(r.PathValue("id"))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, uploads.ErrNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req jobRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	task, key, err := s.buildTask(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	job, err := s.cfg.Jobs.Submit(req.Kind, key, task)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.cfg.Jobs.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

// jobEvents streams job snapshots as Server-Sent Events until the job finishes.
func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	signal, unsubscribe, ok := s.cfg.Jobs.Subscribe(id)
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	defer unsubscribe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	send := func() bool {
		job, _ := s.cfg.Jobs.Get(id)
		data, _ := json.Marshal(job)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		_ = rc.Flush()
		return job.Status != jobs.Done && job.Status != jobs.Failed
	}
	if !send() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			_ = rc.Flush()
		case <-signal:
			if !send() {
				return
			}
		}
	}
}

func (s *Server) jobFile(w http.ResponseWriter, r *http.Request) {
	p, err := s.cfg.Jobs.FilePath(r.PathValue("id"), r.PathValue("name"))
	if err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	if _, err := os.Stat(p); err != nil {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, p)
}

// static serves the adapter-static build: exact file, `<path>.html`, then the SPA fallback.
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	for _, candidate := range []string{clean, clean + ".html", path.Join(clean, "index.html")} {
		full := filepath.Join(s.cfg.StaticDir, filepath.FromSlash(candidate))
		if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
			s.serveStatic(w, r, full, candidate)
			return
		}
	}
	s.serveStatic(w, r, filepath.Join(s.cfg.StaticDir, "200.html"), "/200.html")
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request, full, urlPath string) {
	if strings.HasPrefix(urlPath, "/_app/immutable/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("Vary", "Accept-Encoding")
	accept := r.Header.Get("Accept-Encoding")
	for _, enc := range []struct{ token, ext string }{{"br", ".br"}, {"gzip", ".gz"}} {
		if !strings.Contains(accept, enc.token) {
			continue
		}
		f, err := os.Open(full + enc.ext)
		if err != nil {
			continue
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			continue
		}
		w.Header().Set("Content-Encoding", enc.token)
		http.ServeContent(w, r, filepath.Base(full), fi.ModTime(), f)
		return
	}
	http.ServeFile(w, r, full)
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && strings.HasPrefix(r.URL.Path, "/api/") && (s.cfg.CORSOrigin == "*" || s.cfg.CORSOrigin == origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, HEAD, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Upload-Offset")
			w.Header().Set("Access-Control-Expose-Headers", "Upload-Offset, Upload-Length")
			w.Header().Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("api", "method", r.Method, "path", r.URL.Path, "status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}
