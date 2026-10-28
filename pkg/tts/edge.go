package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultEdgeURL is the local edge-tts server started with uvicorn.
	DefaultEdgeURL = "http://127.0.0.1:8000"
	// DefaultEdgeVoice is the current female Thai voice on the edge-tts server.
	DefaultEdgeVoice = "th-TH-PremwadeeNeural"
	// EdgeChunkLen is the max runes sent in one POST. Longer Thai requests lose audio.
	EdgeChunkLen = 120

	edgeTimeout = 3 * time.Minute
)

// EdgeClient calls a self-hosted edge-tts server.
type EdgeClient struct {
	BaseURL string
	Voice   string
	Rate    string
	Volume  string
	HTTP    *http.Client
}

type edgeRequest struct {
	Text   string `json:"text"`
	Voice  string `json:"voice"`
	Rate   string `json:"rate"`
	Volume string `json:"volume"`
}

type edgeError struct {
	Error string `json:"error"`
}

// Synthesize posts text to the edge-tts server and returns MP3 bytes.
func (c *EdgeClient) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("edge tts: text is empty")
	}

	payload, err := json.Marshal(edgeRequest{
		Text:   text,
		Voice:  c.voice(),
		Rate:   c.rate(),
		Volume: c.volume(),
	})
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimRight(c.baseURL(), "/") + "/tts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("edge tts: %w", err)
	}
	defer resp.Body.Close()

	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edge tts: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var payload edgeError
		if json.Unmarshal(audio, &payload) == nil && payload.Error != "" {
			return nil, fmt.Errorf("edge tts: %s", payload.Error)
		}
		return nil, fmt.Errorf("edge tts: status %d", resp.StatusCode)
	}
	if len(audio) == 0 {
		return nil, fmt.Errorf("edge tts: empty audio")
	}
	return audio, nil
}

func (c *EdgeClient) baseURL() string {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return DefaultEdgeURL
	}
	return c.BaseURL
}

func (c *EdgeClient) voice() string {
	if c == nil || strings.TrimSpace(c.Voice) == "" {
		return DefaultEdgeVoice
	}
	return c.Voice
}

func (c *EdgeClient) rate() string {
	if c == nil || strings.TrimSpace(c.Rate) == "" {
		return "+0%"
	}
	return c.Rate
}

func (c *EdgeClient) volume() string {
	if c == nil || strings.TrimSpace(c.Volume) == "" {
		return "+0%"
	}
	return c.Volume
}

func (c *EdgeClient) httpClient() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: edgeTimeout}
}
