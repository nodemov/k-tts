package tts

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
)

const (
	// TranslateChunkLen ความยาวสูงสุดต่อ request ของ Google Translate TTS
	TranslateChunkLen = 150
	// CloudChunkLen Cloud TTS จำกัด 5000 bytes ต่อ request (ภาษาไทย ~3 bytes/ตัวอักษร)
	CloudChunkLen = 1400
	// TranslateDelay รอระหว่าง request เพื่อไม่ให้ถูก rate limit
	TranslateDelay = 800 * time.Millisecond

	DefaultVoice = "th-TH-Neural2-C"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// FetchTranslateTTS ดาวน์โหลดเสียงหนึ่งส่วนจาก Google Translate TTS
func FetchTranslateTTS(ctx context.Context, text, lang string) ([]byte, error) {
	ttsURL := fmt.Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&tl=%s&client=tw-ob&q=%s",
		url.QueryEscape(lang), url.QueryEscape(text))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ttsURL, nil)
	if err != nil {
		return nil, err
	}

	// เพิ่ม headers ที่จำเป็น
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Referer", "https://translate.google.com/")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// ตรวจสอบว่าได้ไฟล์เสียงจริงๆ
	head := audio
	if len(head) > 100 {
		head = head[:100]
	}
	if len(audio) < 1000 || strings.Contains(string(head), "<html") {
		return nil, fmt.Errorf("ได้รับข้อมูลที่ไม่ใช่เสียง")
	}
	return audio, nil
}

// SynthesizeCloud สร้างเสียง MP3 48kHz ด้วย Google Cloud TTS
func SynthesizeCloud(ctx context.Context, client *texttospeech.Client, text, voice string) ([]byte, error) {
	if voice == "" {
		voice = DefaultVoice
	}
	req := &texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "th-TH",
			Name:         voice,
			SsmlGender:   texttospeechpb.SsmlVoiceGender_FEMALE,
		},
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding:   texttospeechpb.AudioEncoding_MP3,
			SampleRateHertz: 48000,
			SpeakingRate:    1.0,
			Pitch:           0.0,
			VolumeGainDb:    2.0,
		},
	}

	resp, err := client.SynthesizeSpeech(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("ไม่สามารถสร้างเสียงได้: %v", err)
	}
	return resp.AudioContent, nil
}
