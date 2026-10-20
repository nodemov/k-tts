package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"k-tts/web-audio-tools/backend/internal/ai"
	"k-tts/web-audio-tools/backend/internal/jobs"
	"k-tts/web-audio-tools/backend/internal/uploads"
)

type fixture struct {
	srv    *httptest.Server
	static string
}

func newFixture(t *testing.T, aiURL string) fixture {
	t.Helper()
	dir := t.TempDir()
	store, _ := uploads.NewStore(filepath.Join(dir, "uploads"), 1<<20)
	manager, _ := jobs.NewManager(filepath.Join(dir, "jobs"), 2, time.Minute)
	static := filepath.Join(dir, "build")
	os.MkdirAll(filepath.Join(static, "_app", "immutable"), 0o755)
	os.WriteFile(filepath.Join(static, "200.html"), []byte("fallback"), 0o644)
	os.WriteFile(filepath.Join(static, "editor.html"), []byte("editor page"), 0o644)
	os.WriteFile(filepath.Join(static, "_app", "immutable", "app.js"), []byte("console.log(1)"), 0o644)
	os.WriteFile(filepath.Join(static, "_app", "immutable", "app.js.br"), []byte("BROTLI"), 0o644)

	h := New(Config{Uploads: store, Jobs: manager, AI: ai.NewClient(aiURL), StaticDir: static, CORSOrigin: "*"})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return fixture{srv: srv, static: static}
}

func do(t *testing.T, method, url, body string, headers map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultTransport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decode[T any](t *testing.T, res *http.Response) T {
	t.Helper()
	defer res.Body.Close()
	var v T
	if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

func upload(t *testing.T, base, content string) string {
	t.Helper()
	created := decode[map[string]any](t, do(t, "POST", base+"/api/uploads", `{"name":"clip.wav","size":`+itoa(len(content))+`}`, nil))
	id := created["id"].(string)

	half := len(content) / 2
	res := do(t, "PATCH", base+"/api/uploads/"+id, content[:half], map[string]string{"Upload-Offset": "0"})
	if res.StatusCode != 200 {
		t.Fatalf("patch status %d", res.StatusCode)
	}
	res.Body.Close()

	res = do(t, "PATCH", base+"/api/uploads/"+id, "zz", map[string]string{"Upload-Offset": "0"})
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.StatusCode)
	}
	res.Body.Close()

	head := do(t, "HEAD", base+"/api/uploads/"+id, "", nil)
	if head.Header.Get("Upload-Offset") != itoa(half) {
		t.Fatalf("HEAD offset = %q", head.Header.Get("Upload-Offset"))
	}
	head.Body.Close()

	do(t, "PATCH", base+"/api/uploads/"+id, content[half:], map[string]string{"Upload-Offset": itoa(half)}).Body.Close()
	done := decode[map[string]any](t, do(t, "POST", base+"/api/uploads/"+id+"/complete", "", nil))
	if done["complete"] != true {
		t.Fatalf("complete = %+v", done)
	}
	return id
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestUploadAndAIJobOverSSE(t *testing.T) {
	var gotInput, gotDir string
	aiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "models": map[string]bool{"whisper": true}})
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		gotInput, _ = body["input"].(string)
		gotDir, _ = body["output_dir"].(string)
		os.WriteFile(filepath.Join(gotDir, "clip.srt"), []byte("1\n00:00:00,000 --> 00:00:01,000\nสวัสดี\n"), 0o644)
		json.NewEncoder(w).Encode(map[string]any{"files": []string{"clip.srt"}, "text": "สวัสดี", "engine": "faster-whisper"})
	}))
	defer aiSrv.Close()
	f := newFixture(t, aiSrv.URL)
	base := f.srv.URL

	health := decode[map[string]any](t, do(t, "GET", base+"/api/health", "", nil))
	if health["ai"].(map[string]any)["reachable"] != true {
		t.Fatalf("health = %+v", health)
	}

	id := upload(t, base, "RIFF-not-really-audio")
	job := decode[jobs.Job](t, do(t, "POST", base+"/api/jobs", `{"kind":"transcribe","uploadId":"`+id+`","params":{"language":"th"}}`, nil))
	if job.ID == "" {
		t.Fatal("no job id")
	}

	res := do(t, "GET", base+"/api/jobs/"+job.ID+"/events", "", nil)
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	var last jobs.Job
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			json.Unmarshal([]byte(data), &last)
		}
	}
	res.Body.Close()
	if last.Status != jobs.Done || last.Result["text"] != "สวัสดี" || len(last.Files) != 1 {
		t.Fatalf("final job = %+v", last)
	}
	if !filepath.IsAbs(gotInput) || !filepath.IsAbs(gotDir) {
		t.Fatalf("AI service must receive absolute paths: %q %q", gotInput, gotDir)
	}

	file := do(t, "GET", base+last.Files[0].URL, "", nil)
	if file.StatusCode != 200 {
		t.Fatalf("file status %d", file.StatusCode)
	}
	file.Body.Close()

	cached := decode[jobs.Job](t, do(t, "POST", base+"/api/jobs", `{"kind":"transcribe","uploadId":"`+id+`","params":{"language":"th"}}`, nil))
	if !cached.Cached || cached.ID != job.ID {
		t.Fatalf("expected cached job, got %+v", cached)
	}

	if res := do(t, "GET", base+"/api/jobs/"+job.ID+"/files/..%2F..%2Fuploads", "", nil); res.StatusCode != 404 {
		t.Fatalf("traversal status %d", res.StatusCode)
	}
}

func TestJobValidation(t *testing.T) {
	f := newFixture(t, "http://127.0.0.1:1")
	cases := map[string]string{
		"unknown kind":    `{"kind":"nope","uploadId":"x"}`,
		"missing upload":  `{"kind":"enhance"}`,
		"bad upload":      `{"kind":"enhance","uploadId":"missing"}`,
		"empty tts":       `{"kind":"tts","params":{"chapters":[]}}`,
		"bad speed":       `{"kind":"tts","params":{"chapters":[{"title":"a","text":"ข้อความ"}],"speed":9}}`,
		"cloud not ready": `{"kind":"tts","params":{"chapters":[{"title":"a","text":"ข้อความ"}],"engine":"cloud"}}`,
	}
	for name, body := range cases {
		res := do(t, "POST", f.srv.URL+"/api/jobs", body, nil)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %d", name, res.StatusCode)
		}
		res.Body.Close()
	}
}

func TestStaticServing(t *testing.T) {
	f := newFixture(t, "http://127.0.0.1:1")
	read := func(path string, headers map[string]string) (*http.Response, string) {
		res := do(t, "GET", f.srv.URL+path, "", headers)
		defer res.Body.Close()
		var b strings.Builder
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			b.WriteString(sc.Text())
		}
		return res, b.String()
	}

	if _, body := read("/editor", nil); body != "editor page" {
		t.Errorf("/editor served %q", body)
	}
	if _, body := read("/some/deep/link", nil); body != "fallback" {
		t.Errorf("SPA fallback served %q", body)
	}
	if _, body := read("/../../etc/passwd", nil); strings.Contains(body, "root:") {
		t.Errorf("traversal served %q", body)
	}
	res, body := read("/_app/immutable/app.js", map[string]string{"Accept-Encoding": "gzip, br"})
	if body != "BROTLI" || res.Header.Get("Content-Encoding") != "br" {
		t.Errorf("precompressed: body=%q enc=%q", body, res.Header.Get("Content-Encoding"))
	}
	if !strings.Contains(res.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("cache-control %q", res.Header.Get("Cache-Control"))
	}
	if !strings.Contains(res.Header.Get("Content-Type"), "javascript") {
		t.Errorf("content-type %q", res.Header.Get("Content-Type"))
	}
	if res, _ := read("/api/unknown", nil); res.StatusCode != 404 {
		t.Errorf("unknown api status %d", res.StatusCode)
	}
}
