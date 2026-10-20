// Package ai talks to the Python AI service (Demucs, DeepFilterNet, Whisper).
// Both services share DATA_DIR, so only file paths cross the wire.
package ai

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

type Client struct {
	base string
	http *http.Client
}

type Health struct {
	Reachable bool            `json:"reachable"`
	Models    map[string]bool `json:"models"`
}

type Result struct {
	Files    []string `json:"files"`
	Text     string   `json:"text,omitempty"`
	Language string   `json:"language,omitempty"`
	Engine   string   `json:"engine,omitempty"`
}

func NewClient(base string) *Client {
	return &Client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 3 * time.Hour}}
}

func (c *Client) Health(ctx context.Context) Health {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	h := Health{Models: map[string]bool{}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/health", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return h
	}
	defer resp.Body.Close()
	var body struct {
		Models map[string]bool `json:"models"`
	}
	if resp.StatusCode == http.StatusOK && json.NewDecoder(resp.Body).Decode(&body) == nil {
		h.Reachable = true
		if body.Models != nil {
			h.Models = body.Models
		}
	}
	return h
}

// Call posts `{input, output_dir, ...params}` to `/{op}`.
func (c *Client) Call(ctx context.Context, op, input, outputDir string, params map[string]any) (Result, error) {
	payload := map[string]any{"input": input, "output_dir": outputDir}
	for k, v := range params {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/"+op, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("AI service unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Detail any `json:"detail"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Detail != nil {
			return Result{}, fmt.Errorf("AI service: %v", e.Detail)
		}
		return Result{}, fmt.Errorf("AI service: %s", resp.Status)
	}
	var res Result
	return res, json.Unmarshal(raw, &res)
}
