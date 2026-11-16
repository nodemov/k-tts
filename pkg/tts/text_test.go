package tts

import (
	"strings"
	"testing"
)

func TestCleanTextRemovesMarkupAndChapterLines(t *testing.T) {
	in := "บทที่ 3\n\n# หัวข้อ *สำคัญ*\n12\nเขาเดินไป……   ช้าๆ!!!"
	got := CleanText(in)
	want := "หัวข้อ สำคัญ\n\nเขาเดินไป... ช้าๆ!"
	if got != want {
		t.Fatalf("CleanText() = %q, want %q", got, want)
	}
}

func TestFormatAudiobookLayout(t *testing.T) {
	in := "บทที่ 51 เสี่ยวเอ้อร์ในวันวาน\r\n\r\n【ระบบ】 แจ้งเตือน 🅣🅦\r\n\r\n* * *\r\n\r\n“ น้องหลิน ไหวหรือไม่ ?”\r\n"
	got := FormatAudiobook(in)
	want := "บทที่ 51 เสี่ยวเอ้อร์ในวันวาน\n\nระบบ แจ้งเตือน\n\n“น้องหลิน ไหวหรือไม่?”"
	if got != want {
		t.Fatalf("FormatAudiobook() = %q, want %q", got, want)
	}
}

func TestSplitTextOnlyCutsAtSpaces(t *testing.T) {
	// คำยาวที่ไม่มีช่องว่าง 30-60 ตัวอักษรคั่นด้วยช่องว่าง
	words := []string{
		"ศิษย์พี่เจียงแล้วกระมัง?",
		"สำนักยุทธ์และโรงเรียนยุทธ์ขนาดใหญ่ทั่วแผ่นดิน",
		"ขอบเขตเดียวกันอย่างแน่นอน",
		"ระลองฝีมือกันอย่างสะดวกเท่านั้นแต่ตอนนี้กลับไม่ใช่แล้ว",
	}
	var all []string
	for i := 0; i < 20; i++ {
		all = append(all, words...)
	}
	text := strings.Join(all, " ")
	for _, maxLen := range []int{50, 120, 800} {
		parts := SplitText(text, maxLen)
		for _, p := range parts {
			for _, w := range strings.Fields(p) {
				found := false
				for _, want := range words {
					if w == want {
						found = true
					}
				}
				if !found {
					t.Fatalf("maxLen %d: word cut in half: %q", maxLen, w)
				}
			}
		}
		if strings.Join(parts, " ") != text {
			t.Fatalf("maxLen %d: text changed after split", maxLen)
		}
	}
}

func TestSplitTextPrefersParagraphAndTitle(t *testing.T) {
	text := "บทที่ 1 เริ่มต้น\n\nเขาเดินไปตลาด แล้วซื้อปลา.\n\nนางยิ้มให้เขา แล้วเดินจากไป."
	parts := SplitText(text, 40)
	want := []string{"บทที่ 1 เริ่มต้น", "เขาเดินไปตลาด แล้วซื้อปลา.", "นางยิ้มให้เขา แล้วเดินจากไป."}
	if strings.Join(parts, "|") != strings.Join(want, "|") {
		t.Fatalf("parts = %q, want %q", parts, want)
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
	parts := SplitText("ราคา 3.14 บาท. จบ", 16)
	if len(parts) != 2 || parts[0] != "ราคา 3.14 บาท." {
		t.Fatalf("unexpected parts: %q", parts)
	}
}

func TestSplitTextDropsSceneBreakEllipsis(t *testing.T) {
	sentence := "เขาเดินไปตลาดแล้วซื้อปลาสดจากเรือ. "
	text := strings.Repeat(sentence, 40) + "... " + strings.Repeat(sentence, 40) + "..."
	parts := SplitText(text, 800)
	if len(parts) < 2 {
		t.Fatalf("expected multiple parts, got %d", len(parts))
	}
	for i, p := range parts {
		if !hasSpeakableText(p) {
			t.Fatalf("part %d is only punctuation: %q", i, p)
		}
	}
	if strings.Count(strings.Join(parts, " "), "เขาเดินไปตลาด") != 80 {
		t.Fatal("spoken text was dropped with the scene break")
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
