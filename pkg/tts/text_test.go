package tts

import (
	"strings"
	"testing"
)

func TestCleanTextRemovesMarkupAndChapterLines(t *testing.T) {
	in := "บทที่ 3\n\n# หัวข้อ *สำคัญ*\n12\nเขาเดินไป...   ช้าๆ"
	got := CleanText(in)
	want := "หัวข้อ สำคัญ เขาเดินไป... ช้าๆ"
	if got != want {
		t.Fatalf("CleanText() = %q, want %q", got, want)
	}
}

func TestSplitTextRespectsMaxLen(t *testing.T) {
	text := strings.Repeat("ประโยคทดสอบภาษาไทยที่ยาวพอสมควร. ", 30)
	parts := SplitText(text, 150)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	for i, p := range parts {
		if n := len([]rune(p)); n > 150 {
			t.Errorf("part %d has %d runes", i, n)
		}
	}
	if joined := strings.Join(parts, " "); strings.Count(joined, "ประโยคทดสอบ") != 30 {
		t.Errorf("text lost while splitting")
	}
}

func TestSplitTextKeepsDecimals(t *testing.T) {
	parts := splitIntoSentences("ราคา 3.14 บาท. จบ")
	if len(parts) != 2 || parts[0] != "ราคา 3.14 บาท." {
		t.Fatalf("unexpected sentences: %q", parts)
	}
}

func TestSplitLongThaiWithoutSpaces(t *testing.T) {
	text := strings.Repeat("กินข้าว", 60)
	for _, p := range SplitText(text, 50) {
		if n := len([]rune(p)); n > 50 || n == 0 {
			t.Fatalf("bad part length %d", n)
		}
	}
}

func TestAtempoFilterChainsAbove2x(t *testing.T) {
	cases := map[float64]int{1.6: 1, 2.4: 2, 3.5: 3}
	for speed, want := range cases {
		if got := strings.Count(AtempoFilter(speed), "atempo="); got != want {
			t.Errorf("speed %.1f: %d atempo stages, want %d", speed, got, want)
		}
	}
}

func TestSortFilesNaturally(t *testing.T) {
	files := []string{"a/temp_part_10.mp3", "a/temp_part_2.mp3", "a/temp_part_1.mp3"}
	SortFilesNaturally(files)
	if files[0] != "a/temp_part_1.mp3" || files[2] != "a/temp_part_10.mp3" {
		t.Fatalf("unexpected order %v", files)
	}
}
