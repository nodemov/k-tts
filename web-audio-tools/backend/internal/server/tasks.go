package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k-tts/pkg/tts"
	"k-tts/web-audio-tools/backend/internal/jobs"
	"k-tts/web-audio-tools/backend/internal/media"
)

type jobRequest struct {
	Kind     string          `json:"kind"`
	UploadID string          `json:"uploadId"`
	Params   json.RawMessage `json:"params"`
}

var unsafeName = regexp.MustCompile(`[^\p{L}\p{N}._-]+`)

func safeName(name string) string {
	name = strings.Trim(unsafeName.ReplaceAllString(name, "-"), "-.")
	if r := []rune(name); len(r) > 60 {
		name = string(r[:60])
	}
	if name == "" {
		return "audio"
	}
	return name
}

func cacheKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func decodeParams[T any](raw json.RawMessage, into *T) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}
	return nil
}

// canonical re-encodes params so equivalent requests share a cache entry.
func canonical(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (s *Server) buildTask(req jobRequest) (jobs.Task, string, error) {
	if req.Kind == "tts" {
		return s.ttsTask(req.Params)
	}

	if req.UploadID == "" {
		return nil, "", errors.New("uploadId is required")
	}
	input, upload, err := s.cfg.Uploads.Path(req.UploadID)
	if err != nil {
		return nil, "", err
	}
	input, err = filepath.Abs(input)
	if err != nil {
		return nil, "", err
	}
	base := safeName(strings.TrimSuffix(upload.Name, filepath.Ext(upload.Name)))

	switch req.Kind {
	case "enhance":
		p := struct {
			Loudness float64 `json:"loudness"`
			Denoise  bool    `json:"denoise"`
			Format   string  `json:"format"`
			Bitrate  int     `json:"bitrate"`
		}{Loudness: -16, Denoise: true, Format: "mp3", Bitrate: 192}
		if err := decodeParams(req.Params, &p); err != nil {
			return nil, "", err
		}
		if p.Loudness < -36 || p.Loudness > -5 {
			return nil, "", errors.New("loudness must be between -36 and -5 LUFS")
		}
		if _, err := media.CodecArgs(p.Format, p.Bitrate); err != nil {
			return nil, "", err
		}
		task := func(ctx context.Context, r *jobs.Reporter) error {
			out := filepath.Join(r.Dir(), fmt.Sprintf("%s-enhanced.%s", base, p.Format))
			return media.Enhance(ctx, input, out, media.EnhanceOptions{Loudness: p.Loudness, Denoise: p.Denoise, Format: p.Format, Bitrate: p.Bitrate}, r.Progress)
		}
		return task, cacheKey("enhance", upload.SHA256, canonical(p)), nil

	case "convert":
		p := struct {
			Format  string `json:"format"`
			Bitrate int    `json:"bitrate"`
		}{Format: "mp3", Bitrate: 192}
		if err := decodeParams(req.Params, &p); err != nil {
			return nil, "", err
		}
		if _, err := media.CodecArgs(p.Format, p.Bitrate); err != nil {
			return nil, "", err
		}
		task := func(ctx context.Context, r *jobs.Reporter) error {
			out := filepath.Join(r.Dir(), fmt.Sprintf("%s.%s", base, p.Format))
			return media.Convert(ctx, input, out, p.Format, p.Bitrate, func(f float64) { r.Progress(f, "converting") })
		}
		return task, cacheKey("convert", upload.SHA256, canonical(p)), nil

	case "denoise", "separate", "transcribe":
		params := map[string]any{}
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, "", err
		}
		key := cacheKey(req.Kind, upload.SHA256, canonical(params))
		params["name"] = base
		op := req.Kind
		task := func(ctx context.Context, r *jobs.Reporter) error {
			r.Progress(0.05, "processing on AI service")
			dir, err := filepath.Abs(r.Dir())
			if err != nil {
				return err
			}
			res, err := s.cfg.AI.Call(ctx, op, input, dir, params)
			if err != nil {
				return err
			}
			if res.Text != "" {
				r.SetResult("text", res.Text)
			}
			if res.Language != "" {
				r.SetResult("language", res.Language)
			}
			if res.Engine != "" {
				r.SetResult("engine", res.Engine)
			}
			return nil
		}
		return task, key, nil
	}
	return nil, "", fmt.Errorf("unknown job kind %q", req.Kind)
}

type chapter struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

type ttsParams struct {
	Chapters []chapter `json:"chapters"`
	Speed    float64   `json:"speed"`
	Engine   string    `json:"engine"`
	Voice    string    `json:"voice"`
}

func (s *Server) ttsTask(raw json.RawMessage) (jobs.Task, string, error) {
	p := ttsParams{Speed: 1, Engine: "auto"}
	if err := decodeParams(raw, &p); err != nil {
		return nil, "", err
	}
	total := 0
	for _, ch := range p.Chapters {
		total += len([]rune(ch.Text))
	}
	switch {
	case len(p.Chapters) == 0 || total == 0:
		return nil, "", errors.New("no text to synthesise")
	case len(p.Chapters) > 1000 || total > 10_000_000:
		return nil, "", errors.New("too much text for one job (max 1000 chapters / 10M characters)")
	case p.Speed < 0.5 || p.Speed > 3:
		return nil, "", errors.New("speed must be between 0.5 and 3")
	case p.Engine != "auto" && p.Engine != "cloud" && p.Engine != "translate":
		return nil, "", errors.New("engine must be auto, cloud or translate")
	case p.Engine == "cloud" && s.cfg.TTS == nil:
		return nil, "", errors.New("Google Cloud TTS is not configured on the server (set GOOGLE_APPLICATION_CREDENTIALS)")
	}

	task := func(ctx context.Context, r *jobs.Reporter) error {
		n := float64(len(p.Chapters))
		for i, ch := range p.Chapters {
			report := func(frac float64, msg string) {
				r.Progress((float64(i)+frac)/n, fmt.Sprintf("บท %d/%d · %s", i+1, len(p.Chapters), msg))
			}
			out := filepath.Join(r.Dir(), fmt.Sprintf("%03d-%s.mp3", i+1, safeName(ch.Title)))
			tmp := filepath.Join(r.Dir(), fmt.Sprintf("tmp-%03d", i+1))
			err := s.synthesizeChapter(ctx, ch.Text, p, tmp, out, report)
			os.RemoveAll(tmp)
			if err != nil {
				return fmt.Errorf("บท %d (%s): %w", i+1, ch.Title, err)
			}
		}
		return nil
	}
	return task, cacheKey("tts", canonical(p)), nil
}

func (s *Server) synthesizeChapter(ctx context.Context, text string, p ttsParams, tmp, out string, report func(float64, string)) error {
	clean := tts.CleanText(text)
	if clean == "" {
		return errors.New("ไม่มีข้อความที่อ่านได้หลังทำความสะอาด")
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}

	useCloud := p.Engine == "cloud" || (p.Engine == "auto" && s.cfg.TTS != nil)
	if useCloud {
		err := s.cloudParts(ctx, clean, p.Voice, tmp, report)
		if err != nil && p.Engine == "cloud" {
			return err
		}
		if err != nil {
			slog.Warn("cloud TTS failed, falling back to Translate TTS", "err", err)
			os.RemoveAll(tmp)
			if err := os.MkdirAll(tmp, 0o755); err != nil {
				return err
			}
			useCloud = false
		}
	}
	if !useCloud {
		if err := translateParts(ctx, clean, tmp, report); err != nil {
			return err
		}
	}

	report(0.85, "รวมไฟล์เสียง")
	if _, err := tts.CombineAudioFiles(tmp, out); err != nil {
		return err
	}
	if useCloud {
		report(0.9, "ปรับปรุงคุณภาพเสียง")
		enhanced := filepath.Join(tmp, "enhanced.mp3")
		if err := tts.EnhanceAudioQuality(out, enhanced); err != nil {
			return err
		}
		if err := os.Rename(enhanced, out); err != nil {
			return err
		}
	}
	if p.Speed != 1 {
		report(0.95, fmt.Sprintf("ปรับความเร็ว %.2fx", p.Speed))
		if err := tts.AdjustSpeed(out, out, p.Speed); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) cloudParts(ctx context.Context, text, voice, dir string, report func(float64, string)) error {
	parts := tts.SplitText(text, tts.CloudChunkLen)
	for i, part := range parts {
		report(0.8*float64(i)/float64(len(parts)), fmt.Sprintf("Cloud TTS ส่วน %d/%d", i+1, len(parts)))
		audio, err := tts.SynthesizeCloud(ctx, s.cfg.TTS, part, voice)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("temp_part_%d.mp3", i+1)), audio, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func translateParts(ctx context.Context, text, dir string, report func(float64, string)) error {
	parts := tts.SplitText(text, tts.TranslateChunkLen)
	written := 0
	var lastErr error
	for i, part := range parts {
		report(0.8*float64(i)/float64(len(parts)), fmt.Sprintf("Translate TTS ส่วน %d/%d", i+1, len(parts)))
		audio, err := tts.FetchTranslateTTS(ctx, part, "th")
		if err == nil {
			err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("temp_part_%d.mp3", i+1)), audio, 0o644)
		}
		if err != nil {
			lastErr = err
			slog.Warn("translate TTS part failed", "part", i+1, "err", err)
		} else {
			written++
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(tts.TranslateDelay):
		}
	}
	if written == 0 {
		return fmt.Errorf("Google Translate TTS ล้มเหลวทุกส่วน: %v", lastErr)
	}
	return nil
}
