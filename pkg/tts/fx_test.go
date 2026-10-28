package tts

import (
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLookupVoiceFX(t *testing.T) {
	for _, name := range []string{"", "none", " NONE "} {
		fx, err := LookupVoiceFX(name)
		if err != nil || fx.Filter != "" {
			t.Errorf("LookupVoiceFX(%q) = %+v, %v; want empty fx", name, fx, err)
		}
	}
	fx, err := LookupVoiceFX("Clear")
	if err != nil || fx.Name != "clear" {
		t.Errorf("LookupVoiceFX(Clear) = %+v, %v", fx, err)
	}
	if _, err := LookupVoiceFX("nope"); err == nil || !strings.Contains(err.Error(), "bright") {
		t.Errorf("LookupVoiceFX(nope) error = %v, want list of presets", err)
	}
}

func TestSpeedOnlyFilterSkipsDynamics(t *testing.T) {
	got := SpeedOnlyFilter(1.75)
	if got != "atempo=1.75" {
		t.Fatalf("filter = %q", got)
	}
	if strings.Contains(got, "dynaudnorm") || strings.Contains(got, "loudnorm") {
		t.Fatalf("filter still colors the voice: %s", got)
	}
}

func TestSpeedUpKeepsRateAndChannels(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp3")
	out := filepath.Join(dir, "out.mp3")
	cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats",
		"-f", "lavfi", "-i", "sine=f=220:d=2:sample_rate=16000",
		"-ac", "1", src)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("source: %v\n%s", err, b)
	}
	if err := SpeedUp(src, out, 2); err != nil {
		t.Fatal(err)
	}
	b, err := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=sample_rate,channels:format=duration",
		"-of", "default=nw=1", out).Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	if !strings.Contains(text, "sample_rate=16000") || !strings.Contains(text, "channels=1") {
		t.Fatalf("probe = %s", text)
	}
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "duration=") {
			continue
		}
		d, err := strconv.ParseFloat(strings.TrimPrefix(line, "duration="), 64)
		if err != nil || math.Abs(d-1) > 0.15 {
			t.Fatalf("duration %q, want ~1s", line)
		}
	}
}

func TestVoiceFXFilter(t *testing.T) {
	if got, want := VoiceFXFilter(1.5, VoiceFX{}, DefaultLoudness), AtempoFilter(1.5); got != want {
		t.Errorf("no fx: got %q, want %q", got, want)
	}
	fx, _ := LookupVoiceFX("clear")
	if got := VoiceFXFilter(1, fx, -14); strings.Contains(got, "atempo") || !strings.HasSuffix(got, "loudnorm=I=-14:TP=-1:LRA=11") {
		t.Errorf("speed 1: got %q", got)
	}
	if got := VoiceFXFilter(2.4, fx, DefaultLoudness); !strings.HasPrefix(got, "atempo=1.50,atempo=1.60,"+fx.Filter) {
		t.Errorf("speed 2.4: got %q", got)
	}
}

func TestVoiceFXPresetsRunInFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	for _, fx := range VoiceFXPresets {
		cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats",
			"-f", "lavfi", "-i", "sine=f=220:d=1",
			"-af", VoiceFXFilter(1.75, fx, DefaultLoudness),
			"-f", "null", "-")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("preset %s: %v\n%s", fx.Name, err, out)
		}
	}
}

func TestVoiceFXPresetsKeepDuration(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	for _, fx := range VoiceFXPresets {
		out := filepath.Join(t.TempDir(), fx.Name+".wav")
		cmd := exec.Command("ffmpeg", "-hide_banner", "-nostats",
			"-f", "lavfi", "-i", "sine=f=220:d=4:sample_rate=24000",
			"-af", VoiceFXFilter(1, fx, DefaultLoudness), out)
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("preset %s: %v\n%s", fx.Name, err, b)
		}
		b, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", out).Output()
		if err != nil {
			t.Fatalf("ffprobe %s: %v", fx.Name, err)
		}
		d, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
		if err != nil || math.Abs(d-4) > 0.1 {
			t.Errorf("preset %s: duration %q, want ~4s", fx.Name, b)
		}
	}
}
