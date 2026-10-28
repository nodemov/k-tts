package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestRouterWorkersFromEnvSkipsPlaceholders(t *testing.T) {
	t.Setenv("ROUTER_VM_1_URL", "https://9router-vm-1.devbucket.space")
	t.Setenv("ROUTER_VM_1_API_KEY", "vm-1-key")
	t.Setenv("ROUTER_VM_2_URL", "https://9router-vm-2.devbucket.space/")
	t.Setenv("ROUTER_VM_2_API_KEY", "replace-me")
	for i := 3; i <= RouterWorkerCount; i++ {
		t.Setenv("ROUTER_VM_"+itoa(i)+"_URL", "")
		t.Setenv("ROUTER_VM_"+itoa(i)+"_API_KEY", "")
	}

	cfg, err := RouterWorkersFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workers) != 1 || cfg.Workers[0].APIKey != "vm-1-key" {
		t.Fatalf("workers = %+v", cfg.Workers)
	}
	if cfg.Workers[0].BaseURL != "https://9router-vm-1.devbucket.space" {
		t.Fatalf("url = %s", cfg.Workers[0].BaseURL)
	}
	if len(cfg.Skipped) != 1 || cfg.Skipped[0] != 2 {
		t.Fatalf("skipped = %v", cfg.Skipped)
	}
}

func TestRouterWorkersFromEnvRequiresAKey(t *testing.T) {
	for i := 1; i <= RouterWorkerCount; i++ {
		t.Setenv("ROUTER_VM_"+itoa(i)+"_URL", "https://9router-vm-"+itoa(i)+".devbucket.space")
		t.Setenv("ROUTER_VM_"+itoa(i)+"_API_KEY", "replace-me")
	}
	_, err := RouterWorkersFromEnv()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRouterPoolUsesWorkerKey(t *testing.T) {
	var gotAuth string
	var got routerSpeechRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Errorf("path %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3-bytes"))
	}))
	defer server.Close()

	pool, err := NewRouterPool([]RouterWorker{{BaseURL: server.URL, APIKey: "vm-key"}}, "th-TH-PremwadeeNeural")
	if err != nil {
		t.Fatal(err)
	}
	audio, err := pool.Synthesize(context.Background(), "สวัสดี")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "mp3-bytes" {
		t.Fatalf("audio = %q", audio)
	}
	if gotAuth != "Bearer vm-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if got.Model != "edge-tts/th-TH-PremwadeeNeural" || got.Input != "สวัสดี" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestRouterPoolAssignsDifferentWorkers(t *testing.T) {
	entered := make(chan string, 2)
	release := make(chan struct{})
	var mu sync.Mutex
	keys := map[string]string{}

	newServer := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			keys[name] = r.Header.Get("Authorization")
			mu.Unlock()
			entered <- name
			<-release
			w.Header().Set("Content-Type", "audio/mpeg")
			_, _ = w.Write([]byte("ok"))
		}))
	}
	first := newServer("a")
	second := newServer("b")
	defer first.Close()
	defer second.Close()

	pool, err := NewRouterPool([]RouterWorker{
		{BaseURL: first.URL, APIKey: "key-a"},
		{BaseURL: second.URL, APIKey: "key-b"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			_, err := pool.Synthesize(context.Background(), "สวัสดี")
			errCh <- err
		}()
	}

	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		seen[<-entered] = true
	}
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if !seen["a"] || !seen["b"] {
		t.Fatalf("workers used = %v", seen)
	}
	if keys["a"] != "Bearer key-a" || keys["b"] != "Bearer key-b" {
		t.Fatalf("keys = %v", keys)
	}
}

func TestRouterPoolReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(routerErrorBody{Error: "API key required for remote API access"})
	}))
	defer server.Close()

	pool, err := NewRouterPool([]RouterWorker{{BaseURL: server.URL, APIKey: "bad"}}, DefaultEdgeVoice)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Synthesize(context.Background(), "สวัสดี")
	status, ok := err.(*RouterStatusError)
	if !ok {
		t.Fatalf("error = %v", err)
	}
	if status.Retryable() {
		t.Fatal("401 should not be retried")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Fatalf("error = %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
