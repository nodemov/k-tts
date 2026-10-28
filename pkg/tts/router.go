package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// RouterWorkerCount is the number of 9router VMs configured in .env.
	RouterWorkerCount = 12
	// RouterChunkLen is the max runes per request. Longer calls hit the public gateway timeout.
	RouterChunkLen = 800

	routerTimeout = 2 * time.Minute
)

// RouterWorker is one 9router VM and the API key created on that VM.
type RouterWorker struct {
	BaseURL string
	APIKey  string
}

// RouterConfig is the set of VMs ready to call, plus indexes still waiting for a real key.
type RouterConfig struct {
	Workers []RouterWorker
	Skipped []int
}

// RouterPool sends each speech request to one VM at a time.
type RouterPool struct {
	Voice string
	HTTP  *http.Client
	slots chan RouterWorker
}

type routerSpeechRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type routerErrorBody struct {
	Error string `json:"error"`
}

// RouterStatusError is a non-2xx response from a worker.
type RouterStatusError struct {
	BaseURL string
	Status  int
	Detail  string
}

func (e *RouterStatusError) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("router tts: %s: %s", e.BaseURL, e.Detail)
	}
	return fmt.Sprintf("router tts: %s: status %d", e.BaseURL, e.Status)
}

// Retryable reports whether another worker might succeed.
func (e *RouterStatusError) Retryable() bool {
	return e.Status == http.StatusTooManyRequests || e.Status >= 500
}

// RouterWorkersFromEnv reads ROUTER_VM_1_URL/API_KEY through ROUTER_VM_12_*.
// Workers whose key is still the placeholder are skipped.
func RouterWorkersFromEnv() (RouterConfig, error) {
	var cfg RouterConfig
	for i := 1; i <= RouterWorkerCount; i++ {
		urlName := fmt.Sprintf("ROUTER_VM_%d_URL", i)
		keyName := fmt.Sprintf("ROUTER_VM_%d_API_KEY", i)
		rawURL := strings.TrimSpace(os.Getenv(urlName))
		apiKey := strings.TrimSpace(os.Getenv(keyName))
		if rawURL == "" && apiKey == "" {
			continue
		}
		if isPlaceholderKey(apiKey) {
			cfg.Skipped = append(cfg.Skipped, i)
			continue
		}
		if rawURL == "" {
			return RouterConfig{}, fmt.Errorf("router tts: %s is set but %s is empty", keyName, urlName)
		}
		baseURL, err := normalizeRouterURL(rawURL)
		if err != nil {
			return RouterConfig{}, fmt.Errorf("router tts: %s: %w", urlName, err)
		}
		cfg.Workers = append(cfg.Workers, RouterWorker{BaseURL: baseURL, APIKey: apiKey})
	}
	if len(cfg.Workers) == 0 {
		return RouterConfig{}, fmt.Errorf("router tts: set ROUTER_VM_1_API_KEY through ROUTER_VM_%d_API_KEY in .env", RouterWorkerCount)
	}
	return cfg, nil
}

// NewRouterPool borrows one VM per in-flight request.
func NewRouterPool(workers []RouterWorker, voice string) (*RouterPool, error) {
	if len(workers) == 0 {
		return nil, fmt.Errorf("router tts: no workers")
	}
	slots := make(chan RouterWorker, len(workers))
	for _, worker := range workers {
		if strings.TrimSpace(worker.BaseURL) == "" || strings.TrimSpace(worker.APIKey) == "" {
			return nil, fmt.Errorf("router tts: worker %q is missing a URL or API key", worker.BaseURL)
		}
		baseURL, err := normalizeRouterURL(worker.BaseURL)
		if err != nil {
			return nil, err
		}
		worker.BaseURL = baseURL
		slots <- worker
	}
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = DefaultEdgeVoice
	}
	return &RouterPool{
		Voice: voice,
		HTTP: &http.Client{
			Timeout:       routerTimeout,
			CheckRedirect: refuseRouterRedirect,
		},
		slots: slots,
	}, nil
}

// Synthesize posts text to the next free VM and returns MP3 bytes.
func (p *RouterPool) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("router tts: text is empty")
	}
	if p == nil || p.slots == nil {
		return nil, fmt.Errorf("router tts: pool is not initialized")
	}

	var worker RouterWorker
	select {
	case worker = <-p.slots:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { p.slots <- worker }()
	return p.post(ctx, worker, text)
}

func (p *RouterPool) post(ctx context.Context, worker RouterWorker, text string) ([]byte, error) {
	payload, err := json.Marshal(routerSpeechRequest{
		Model: routerModel(p.Voice),
		Input: text,
	})
	if err != nil {
		return nil, err
	}

	endpoint := worker.BaseURL + "/v1/audio/speech"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+worker.APIKey)

	resp, err := p.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("router tts: %s: %w", worker.BaseURL, err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("router tts: %s: %w", worker.BaseURL, err)
	}
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(audio))
		var body routerErrorBody
		if json.Unmarshal(audio, &body) == nil && body.Error != "" {
			detail = body.Error
		}
		return nil, &RouterStatusError{BaseURL: worker.BaseURL, Status: resp.StatusCode, Detail: detail}
	}
	contentType := resp.Header.Get("Content-Type")
	if len(audio) == 0 || (!strings.Contains(contentType, "audio") && !looksLikeMP3(audio)) {
		return nil, fmt.Errorf("router tts: %s: response is not audio (%s)", worker.BaseURL, contentType)
	}
	return audio, nil
}

func (p *RouterPool) httpClient() *http.Client {
	if p != nil && p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: routerTimeout, CheckRedirect: refuseRouterRedirect}
}

func routerModel(voice string) string {
	voice = strings.TrimSpace(voice)
	if voice == "" {
		voice = DefaultEdgeVoice
	}
	if strings.Contains(voice, "/") {
		return voice
	}
	return "edge-tts/" + voice
}

func normalizeRouterURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("router tts: empty worker URL")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", fmt.Errorf("router tts: invalid worker URL %q", raw)
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isPlaceholderKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "", "replace-me", "changeme", "your-api-key":
		return true
	default:
		return false
	}
}

func refuseRouterRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

func looksLikeMP3(b []byte) bool {
	return bytes.HasPrefix(b, []byte("ID3")) || (len(b) > 1 && b[0] == 0xFF && b[1]&0xE0 == 0xE0)
}
