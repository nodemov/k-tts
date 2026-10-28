package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSynthesizePostsJSONAndReturnsAudio(t *testing.T) {
	var got edgeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/tts" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("content type %q", ct)
		}
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

	client := &EdgeClient{BaseURL: server.URL + "/", Voice: "th-TH-PremwadeeNeural", Rate: "+10%", Volume: "-5%"}
	audio, err := client.Synthesize(context.Background(), "สวัสดี")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "mp3-bytes" {
		t.Fatalf("audio = %q", audio)
	}
	if got.Text != "สวัสดี" || got.Voice != "th-TH-PremwadeeNeural" || got.Rate != "+10%" || got.Volume != "-5%" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestSynthesizeDefaultsVoiceRateVolume(t *testing.T) {
	var got edgeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	audio, err := (&EdgeClient{BaseURL: server.URL}).Synthesize(context.Background(), "ทดสอบ")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "ok" {
		t.Fatalf("audio = %q", audio)
	}
	if got.Voice != DefaultEdgeVoice || got.Rate != "+0%" || got.Volume != "+0%" {
		t.Fatalf("payload = %+v", got)
	}
}

func TestSynthesizeReturnsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(edgeError{Error: "บริการสร้างเสียงไม่ตอบกลับ ลองใหม่อีกครั้ง"})
	}))
	defer server.Close()

	_, err := (&EdgeClient{BaseURL: server.URL}).Synthesize(context.Background(), "สวัสดี")
	if err == nil || err.Error() != "edge tts: บริการสร้างเสียงไม่ตอบกลับ ลองใหม่อีกครั้ง" {
		t.Fatalf("error = %v", err)
	}
}

func TestSynthesizeRejectsEmptyText(t *testing.T) {
	_, err := (&EdgeClient{}).Synthesize(context.Background(), "  ")
	if err == nil {
		t.Fatal("expected error")
	}
}
