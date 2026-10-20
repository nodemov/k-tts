// Package tts รวม logic ที่ใช้ร่วมกันระหว่าง CLI (main.go) และ web-audio-tools backend:
// การทำความสะอาด/แบ่งข้อความภาษาไทย, การเรียก Google TTS และ ffmpeg pipeline
package tts

import (
	"regexp"
	"strings"
)

var (
	chapterLineRe = regexp.MustCompile(`^(\d+|บทที่\s*\d+|Chapter\s*\d+)$`)
	spacesRe      = regexp.MustCompile(`\s+`)
)

// SplitText แบ่งข้อความเป็นส่วนย่อยที่ยาวไม่เกิน maxLen ตัวอักษร โดยแบ่งตามประโยคก่อน
func SplitText(text string, maxLen int) []string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}

	var parts []string

	// แบ่งตามประโยค (จุด, อัศเจรีย์, คำถาม)
	sentences := splitIntoSentences(text)

	// รวมประโยคจนกว่าจะถึงขีดจำกัด
	currentPart := ""
	for _, sentence := range sentences {
		testPart := currentPart
		if testPart != "" {
			testPart += " "
		}
		testPart += sentence

		if len([]rune(testPart)) > maxLen && currentPart != "" {
			parts = append(parts, strings.TrimSpace(currentPart))
			currentPart = sentence
		} else {
			currentPart = testPart
		}
	}

	// เพิ่มส่วนสุดท้าย
	if strings.TrimSpace(currentPart) != "" {
		parts = append(parts, strings.TrimSpace(currentPart))
	}

	// หากยังมีส่วนที่ยาวเกินไป ให้แบ่งด้วยวิธีอัจฉริยะกว่า
	var finalParts []string
	for _, part := range parts {
		if len([]rune(part)) <= maxLen {
			finalParts = append(finalParts, part)
		} else {
			// แบ่งด้วยการหาจุดแบ่งที่เหมาะสม
			finalParts = append(finalParts, splitLongText(part, maxLen)...)
		}
	}

	return finalParts
}

// แบ่งข้อความเป็นประโยค
func splitIntoSentences(text string) []string {
	var sentences []string
	runes := []rune(text)
	current := ""

	for i, r := range runes {
		current += string(r)

		// จุดจบประโยคภาษาไทยและอังกฤษ
		if r == '.' || r == '!' || r == '?' || r == '।' || r == '|' {
			// ตรวจสอบว่าไม่ใช่ทศนิยม (เช่น 3.14)
			isDecimal := false
			if r == '.' && i > 0 && i < len(runes)-1 {
				if isDigit(runes[i-1]) && isDigit(runes[i+1]) {
					isDecimal = true
				}
			}

			if !isDecimal {
				// หาช่องว่างถัดไป หรือจบข้อความ
				if i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n' {
					sentences = append(sentences, strings.TrimSpace(current))
					current = ""
				}
			}
		}
	}

	// เพิ่มส่วนที่เหลือ
	if strings.TrimSpace(current) != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}

	return sentences
}

// ตรวจสอบว่าเป็นตัวเลขหรือไม่
func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// แบ่งข้อความยาวด้วยการหาจุดแบ่งที่เหมาะสม
func splitLongText(text string, maxLen int) []string {
	runes := []rune(text)
	var parts []string

	start := 0
	for start < len(runes) {
		end := start + maxLen
		if end > len(runes) {
			end = len(runes)
		}

		// หาจุดแบ่งที่เหมาะสม
		if end < len(runes) {
			// หาช่องว่างย้อนกลับ
			bestBreak := findBestBreakPoint(runes, start, end)
			if bestBreak > start {
				end = bestBreak
			}
		}

		part := string(runes[start:end])
		parts = append(parts, strings.TrimSpace(part))
		start = end

		// ข้ามช่องว่างที่อาจเหลือ
		for start < len(runes) && runes[start] == ' ' {
			start++
		}
	}

	return parts
}

// หาจุดแบ่งที่ดีที่สุด
func findBestBreakPoint(runes []rune, start, maxEnd int) int {
	// หาช่องว่างย้อนกลับจากจุดสิ้นสุด
	for i := maxEnd - 1; i > start; i-- {
		if runes[i] == ' ' {
			return i
		}
	}

	// หาเครื่องหมายวรรคตอนย้อนกลับ
	for i := maxEnd - 1; i > start; i-- {
		r := runes[i]
		if r == ',' || r == ';' || r == ':' || r == '(' || r == ')' ||
			r == '[' || r == ']' || r == '{' || r == '}' || r == '"' || r == '\'' {
			return i + 1 // แบ่งหลังเครื่องหมาย
		}
	}

	// หาสระหรือพยัญชนะไทยที่เหมาะสม
	for i := maxEnd - 1; i > start; i-- {
		r := runes[i]
		// ตัวอักษรไทยที่เป็นจุดแบ่งที่ดี
		if isThaiVowel(r) || isThaiToneMarker(r) {
			// แบ่งหลังสระหรือวรรณยุกต์
			if i+1 < maxEnd {
				return i + 1
			}
		}
	}

	return maxEnd
}

// ตรวจสอบสระไทย
func isThaiVowel(r rune) bool {
	return (r >= 0x0E30 && r <= 0x0E39) || // สระ
		(r >= 0x0E40 && r <= 0x0E44) || // เ แ โ ใ ไ
		r == 0x0E2D || r == 0x0E2E // อ ฮ
}

// ตรวจสอบวรรณยุกต์ไทย
func isThaiToneMarker(r rune) bool {
	return r >= 0x0E48 && r <= 0x0E4B // ่ ้ ๊ ๋
}

// CleanText ลบอักขระพิเศษ เลขบท และช่องว่างซ้ำซ้อนที่ไม่ต้องการให้อ่าน
func CleanText(text string) string {
	// ลบอักขระพิเศษที่ไม่ต้องการ
	specialChars := []string{
		"#", "*", "_", "~", "`", "^", "|", "\\", "/",
		"[", "]", "{", "}", "<", ">", "@", "$", "%",
		"&", "+", "=", "§", "¶", "†", "‡", "•", "…",
	}

	cleaned := text
	for _, char := range specialChars {
		cleaned = strings.ReplaceAll(cleaned, char, " ")
	}

	// ลบเลขบท/หมายเลขที่อยู่ในบรรทัดเดี่ยว (เช่น "1" "2" "บทที่ 1")
	lines := strings.Split(cleaned, "\n")
	var filteredLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// ข้ามบรรทัดที่มีแต่ตัวเลข หรือ "บทที่ X" หรือ "Chapter X"
		if trimmed == "" || chapterLineRe.MatchString(trimmed) {
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	cleaned = strings.Join(filteredLines, "\n")

	// ลบช่องว่างที่ซ้ำซ้อน
	cleaned = spacesRe.ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}
