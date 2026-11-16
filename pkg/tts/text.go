// Package tts รวม logic ที่ใช้ร่วมกันระหว่าง CLI (main.go) และ web-audio-tools backend:
// การทำความสะอาด/แบ่งข้อความภาษาไทย, การเรียก Google TTS หรือ Edge TTS และ ffmpeg pipeline
package tts

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	// บรรทัดที่มีแต่เลขบท เช่น "12", "บทที่ 3", "ตอนที่ 4", "Chapter 5"
	chapterLineRe = regexp.MustCompile(`(?i)^(\d+|(บทที่|ตอนที่)\s*\d+|chapter\s*\d+)[.:]?$`)
	// บรรทัดหัวบท เช่น "บทที่ 51 เสี่ยวเอ้อร์ในวันวาน"
	headingRe     = regexp.MustCompile(`(?i)^(บทที่|ตอนที่|chapter)\s*\d+`)
	spacesRe      = regexp.MustCompile(`[ \t]+`)
	dotsRe        = regexp.MustCompile(`\.{3,}|…+`)
	repeatPunctRe = regexp.MustCompile(`([!?])[!?]+`)
	spaceBeforeRe = regexp.MustCompile(` +([,!?.:;”’)])`)
	spaceAfterRe  = regexp.MustCompile(`([“‘(]) +`)
)

// อักขระที่ไม่ต้องการให้ TTS อ่าน แทนที่ด้วยช่องว่าง
var stripReplacer = strings.NewReplacer(
	"#", " ", "*", " ", "_", " ", "~", " ", "`", " ", "^", " ", "|", " ", "\\", " ", "/", " ",
	"[", " ", "]", " ", "{", " ", "}", " ", "<", " ", ">", " ", "@", " ", "$", " ",
	"&", " ", "+", " ", "=", " ", "§", " ", "¶", " ", "†", " ", "‡", " ", "•", " ",
	"【", " ", "】", " ", "《", " ", "》", " ", "「", " ", "」", " ", "『", " ", "』", " ",
	"〈", " ", "〉", " ", "—", " ", "–", " ",
	"：", ":", "，", ",", "！", "!", "？", "?", "（", "(", "）", ")",
	"%", " เปอร์เซ็นต์ ",
)

// FormatAudiobook แปลงข้อความบทนิยายให้อยู่ในรูปแบบสะอาดสำหรับหนังสือเสียง:
//   - บรรทัดแรกเป็นชื่อบท (ถ้ามี) ตามด้วยบรรทัดว่าง
//   - แต่ละย่อหน้าอยู่บรรทัดเดียว คั่นด้วยบรรทัดว่าง
//   - ลบสัญลักษณ์/อีโมจิ/markup, เลขบทที่อยู่บรรทัดเดี่ยว และตัวคั่นฉาก (*** ... ---)
//   - ยุบช่องว่างและเครื่องหมายวรรคตอนซ้ำ ๆ
func FormatAudiobook(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var b strings.Builder
	for _, r := range text {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff':
			// zero-width ใช้เป็นตัวคั่นคำในภาษาไทย ให้เป็นช่องว่างเพื่อใช้เป็นจุดตัดได้
			b.WriteRune(' ')
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		case unicode.Is(unicode.So, r) || unicode.Is(unicode.Sk, r) || unicode.Is(unicode.Co, r) ||
			(unicode.IsControl(r)) || (r >= 0x2500 && r <= 0x259F):
			// อีโมจิ, box drawing, สัญลักษณ์ตกแต่ง
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	text = stripReplacer.Replace(b.String())

	var paragraphs []string
	for _, line := range strings.Split(text, "\n") {
		line = dotsRe.ReplaceAllString(line, "...")
		line = repeatPunctRe.ReplaceAllString(line, "$1")
		line = spacesRe.ReplaceAllString(line, " ")
		line = spaceBeforeRe.ReplaceAllString(line, "$1")
		line = spaceAfterRe.ReplaceAllString(line, "$1")
		line = strings.TrimSpace(line)
		if line == "" || chapterLineRe.MatchString(line) || !hasSpeakableText(line) {
			continue
		}
		paragraphs = append(paragraphs, line)
	}
	return strings.Join(paragraphs, "\n\n")
}

// CleanText ทำความสะอาดข้อความก่อนส่ง TTS (ใช้ FormatAudiobook) ย่อหน้าคั่นด้วย "\n\n"
func CleanText(text string) string {
	return FormatAudiobook(text)
}

// ระดับความเหมาะสมของจุดตัดหลังแต่ละคำ (ยิ่งสูงยิ่งเป็นจุดหยุดหายใจที่ดี)
const (
	breakSpace     = 1 // ช่องว่างระหว่างคำ/วลี
	breakClause    = 2 // หลัง , ; : หรือปิดเครื่องหมายคำพูด
	breakSentence  = 3 // หลัง . ! ? ...
	breakParagraph = 4 // จบย่อหน้า
	breakForced    = 5 // หลังชื่อบท ต้องตัดเสมอ
)

type token struct {
	text  string
	brk   int // ระดับจุดตัดหลังคำนี้
	runes int
}

// SplitText แบ่งข้อความเป็นส่วนย่อยยาวไม่เกิน maxLen ตัวอักษร โดยตัดที่ช่องว่างเท่านั้น
// (ไม่ตัดกลางคำภาษาไทย) และเลือกจุดตัดที่เป็นจบย่อหน้า/จบประโยคก่อน
// ถ้าคำเดียว (ช่วงที่ไม่มีช่องว่าง) ยาวกว่า maxLen จะยอมให้ส่วนนั้นยาวเกินได้ถึง maxLen*3/2
// เกินกว่านั้นจึงค่อยตัดที่เครื่องหมายวรรคตอนหรือขอบพยางค์
func SplitText(text string, maxLen int) []string {
	if maxLen <= 0 {
		maxLen = 1
	}
	tokens := tokenize(text, maxLen)

	var parts []string
	for len(tokens) > 0 {
		n, size := 0, 0
		for n < len(tokens) {
			add := tokens[n].runes
			if n > 0 {
				add++ // ช่องว่างคั่น
			}
			if n > 0 && size+add > maxLen {
				break
			}
			size += add
			n++
			if tokens[n-1].brk == breakForced {
				break
			}
		}
		if n < len(tokens) && tokens[n-1].brk != breakForced {
			n = bestCut(tokens[:n], maxLen)
		}
		if part := joinTokens(tokens[:n]); hasSpeakableText(part) {
			parts = append(parts, part)
		}
		tokens = tokens[n:]
	}
	return parts
}

// bestCut เลือกจำนวนคำในส่วนนี้ โดยหาจุดตัดระดับสูงสุดที่ไม่ทำให้ส่วนสั้นเกินไป
func bestCut(tokens []token, maxLen int) int {
	minLen := maxLen / 2
	for level := breakParagraph; level > breakSpace; level-- {
		size := 0
		best := 0
		for i, t := range tokens {
			size += t.runes
			if i > 0 {
				size++
			}
			if t.brk >= level && size >= minLen {
				best = i + 1
			}
		}
		if best > 0 {
			return best
		}
	}
	return len(tokens)
}

func joinTokens(tokens []token) string {
	var b strings.Builder
	for i, t := range tokens {
		if i > 0 {
			if tokens[i-1].brk >= breakParagraph {
				b.WriteByte('\n')
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(t.text)
	}
	return b.String()
}

// tokenize แยกข้อความเป็นคำตามช่องว่าง พร้อมระดับจุดตัดหลังแต่ละคำ
func tokenize(text string, maxLen int) []token {
	var tokens []token
	paragraphs := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for pi, para := range paragraphs {
		words := strings.Fields(para)
		if len(words) == 0 {
			continue
		}
		heading := pi == firstNonEmpty(paragraphs) && headingRe.MatchString(strings.TrimSpace(para))
		for wi, w := range words {
			brk := wordBreak(w)
			if wi == len(words)-1 {
				brk = breakParagraph
				if heading {
					brk = breakForced
				}
			}
			for _, piece := range splitLongWord(w, maxLen) {
				tokens = append(tokens, token{text: piece, brk: breakClause, runes: len([]rune(piece))})
			}
			tokens[len(tokens)-1].brk = brk
		}
	}
	return tokens
}

func firstNonEmpty(lines []string) int {
	for i, l := range lines {
		if strings.TrimSpace(l) != "" {
			return i
		}
	}
	return -1
}

// wordBreak ให้ระดับจุดตัดหลังคำ w ตามเครื่องหมายท้ายคำ
func wordBreak(w string) int {
	r := []rune(w)
	last := r[len(r)-1]
	// ข้ามเครื่องหมายคำพูดปิดเพื่อดูเครื่องหมายก่อนหน้า เช่น ...ไหม?”
	end := last
	if (last == '”' || last == '’' || last == '"' || last == ')') && len(r) > 1 {
		end = r[len(r)-2]
	}
	switch end {
	case '.', '!', '?', '…', '।':
		return breakSentence
	case ',', ';', ':':
		return breakClause
	}
	if last == '”' || last == '’' || last == '"' || last == ')' {
		return breakClause
	}
	return breakSpace
}

// splitLongWord ตัดคำที่ไม่มีช่องว่างและยาวเกินขีดจำกัดจริง ๆ (maxLen*3/2)
// โดยพยายามตัดหลังเครื่องหมายวรรคตอนก่อน แล้วจึงใช้ขอบพยางค์ไทยเป็นทางเลือกสุดท้าย
func splitLongWord(w string, maxLen int) []string {
	hardMax := maxLen + maxLen/2
	runes := []rune(w)
	if len(runes) <= hardMax {
		return []string{w}
	}
	var parts []string
	for len(runes) > hardMax {
		cut := 0
		for i := maxLen; i > maxLen/3; i-- {
			if isPunctBreak(runes[i-1]) {
				cut = i
				break
			}
		}
		if cut == 0 {
			cut = findBestBreakPoint(runes, 0, maxLen)
		}
		parts = append(parts, string(runes[:cut]))
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		parts = append(parts, string(runes))
	}
	return parts
}

func isPunctBreak(r rune) bool {
	switch r {
	case ',', ';', ':', '.', '!', '?', '…', '”', '’', '"', ')', 'ๆ', 'ฯ':
		return true
	}
	return false
}

// hasSpeakableText reports whether the chunk contains a letter or number.
// Scene breaks such as "..." are not sent to TTS.
func hasSpeakableText(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

// findBestBreakPoint หาจุดตัดใน runes[start:maxEnd] สำหรับข้อความไทยที่ไม่มีช่องว่างเลย
// ตัดหลังสระ/วรรณยุกต์ โดยไม่ให้สระหน้า (เ แ โ ใ ไ) ถูกแยกจากพยัญชนะที่ตามมา
func findBestBreakPoint(runes []rune, start, maxEnd int) int {
	for i := maxEnd - 1; i > start; i-- {
		r := runes[i]
		if (isThaiFollowVowel(r) || isThaiToneMarker(r)) && i+1 < len(runes) && !isThaiCombining(runes[i+1]) {
			return i + 1
		}
	}
	return maxEnd
}

// สระที่ตามหลังพยัญชนะ (ะ า ำ ิ ี ึ ื ุ ู) และ อ
func isThaiFollowVowel(r rune) bool {
	return (r >= 0x0E30 && r <= 0x0E39) || r == 0x0E2D
}

// อักขระที่ต้องติดกับตัวก่อนหน้า (สระบน/ล่าง, วรรณยุกต์, การันต์, ๆ)
func isThaiCombining(r rune) bool {
	return r == 0x0E31 || (r >= 0x0E33 && r <= 0x0E3A) || (r >= 0x0E47 && r <= 0x0E4E) || r == 'ๆ'
}

// ตรวจสอบวรรณยุกต์ไทย
func isThaiToneMarker(r rune) bool {
	return r >= 0x0E48 && r <= 0x0E4B // ่ ้ ๊ ๋
}
