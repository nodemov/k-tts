package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
)

// โครงสร้างข้อมูลสำหรับงานแต่ละไฟล์
type TTSJob struct {
	ID         int
	FilePath   string
	OutputPath string
	Text       string
}

// โครงสร้างข้อมูลสำหรับผลลัพธ์
type TTSResult struct {
	Job     TTSJob
	Success bool
	Error   error
	Size    int64
}

// แบ่งข้อความเป็นส่วนย่อยสำหรับ Google Translate TTS
func splitText(text string, maxLen int) []string {
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
			subParts := splitLongText(part, maxLen)
			finalParts = append(finalParts, subParts...)
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

// ทำความสะอาดข้อความโดยลบอักขระพิเศษที่ไม่ต้องการให้อ่าน
func cleanTextForTTS(text string) string {
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
		if trimmed == "" ||
			regexp.MustCompile(`^(\d+|บทที่\s*\d+|Chapter\s*\d+)$`).MatchString(trimmed) {
			continue
		}
		filteredLines = append(filteredLines, line)
	}

	cleaned = strings.Join(filteredLines, "\n")

	// ลบช่องว่างที่ซ้ำซ้อน
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")

	return strings.TrimSpace(cleaned)
}

// ตรวจสอบและสร้าง folder
func ensureDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

// ลบไฟล์ใน folder temp
func cleanTempFolder(tempDir string) {
	files, err := filepath.Glob(filepath.Join(tempDir, "*"))
	if err != nil {
		return
	}
	for _, file := range files {
		os.Remove(file)
	}
}

// ฟังก์ชันสำหรับเรียงลำดับไฟล์ตามหมายเลขที่ฝังในชื่อไฟล์แบบธรรมชาติ (Natural Sorting)
func sortFilesNaturally(files []string) {
	// ใช้ regex เพื่อดึงหมายเลขจากชื่อไฟล์ temp_part_*.mp3
	re := regexp.MustCompile(`temp_part_(\d+)\.mp3`)

	sort.Slice(files, func(i, j int) bool {
		// ดึงหมายเลขจากไฟล์ i
		matchI := re.FindStringSubmatch(filepath.Base(files[i]))
		numI := 0
		if len(matchI) > 1 {
			numI, _ = strconv.Atoi(matchI[1])
		}

		// ดึงหมายเลขจากไฟล์ j
		matchJ := re.FindStringSubmatch(filepath.Base(files[j]))
		numJ := 0
		if len(matchJ) > 1 {
			numJ, _ = strconv.Atoi(matchJ[1])
		}

		return numI < numJ
	})
}

// รวมไฟล์เสียงด้วย ffmpeg
func combineAudioFiles(tempDir, outputFile string) error {
	// หาไฟล์ temp_part_*.mp3 ใน tempDir
	pattern := filepath.Join(tempDir, "temp_part_*.mp3")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return fmt.Errorf("ไม่พบไฟล์เสียงใน %s", tempDir)
	}

	// เรียงลำดับไฟล์แบบ natural sorting (1, 2, 3, ..., 10, 11, 12 แทนที่จะเป็น 1, 10, 11, 12, 2, 3)
	sortFilesNaturally(files)

	// แสดงลำดับไฟล์ที่จะรวม
	fmt.Println("📋 ลำดับไฟล์ที่จะรวม:")
	for i, file := range files {
		fmt.Printf("   %d. %s\n", i+1, filepath.Base(file))
	}

	if len(files) == 1 {
		// หากมีไฟล์เดียว ให้คัดลอกไปยัง output
		data, err := os.ReadFile(files[0])
		if err != nil {
			return err
		}
		return os.WriteFile(outputFile, data, 0644)
	}

	// สร้างไฟล์รายการสำหรับ ffmpeg (ใช้ relative path จาก tempDir)
	filelistPath := filepath.Join(tempDir, "filelist.txt")
	var filelistContent strings.Builder
	for _, file := range files {
		// ใช้ชื่อไฟล์เท่านั้น (relative path)
		filename := filepath.Base(file)
		filelistContent.WriteString(fmt.Sprintf("file '%s'\n", filename))
	}

	err = os.WriteFile(filelistPath, []byte(filelistContent.String()), 0644)
	if err != nil {
		return err
	}

	// รันคำสั่ง ffmpeg จาก tempDir โดยใช้ absolute path สำหรับ output
	absOutputFile, err := filepath.Abs(outputFile)
	if err != nil {
		return err
	}

	// ใช้ high-quality encoding parameters
	cmd := exec.Command("ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", "filelist.txt",
		"-c:a", "libmp3lame", // ใช้ LAME MP3 encoder คุณภาพสูง
		"-b:a", "320k", // Bitrate 320kbps (คุณภาพสูงสุด)
		"-ar", "48000", // Sample rate 48kHz
		"-ac", "2", // Stereo
		"-af", "volume=1.2,dynaudnorm=p=0.9:s=5", // ปรับ volume และ normalize
		absOutputFile,
		"-y")

	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}

	// ลบไฟล์รายการ
	os.Remove(filelistPath)

	return nil
}

// ปรับความเร็วของไฟล์เสียงด้วย ffmpeg
func adjustAudioSpeed(inputFile, outputFile string, speed float64) error {
	// สร้างไฟล์ temp สำหรับการปรับความเร็ว
	tempFile := inputFile + ".temp.mp3"

	fmt.Printf("⚡ กำลังปรับความเร็วเป็น %.1fx...\n", speed)

	// สร้าง atempo filter สำหรับความเร็วสูง (แบ่งเป็นขั้นๆ หากเกิน 2.0)
	var audioFilter string
	if speed <= 2.0 {
		audioFilter = fmt.Sprintf("atempo=%.2f,dynaudnorm=p=0.9:s=5", speed)
	} else {
		// สำหรับความเร็วสูงกว่า 2.0 ต้องใช้ atempo หลายครั้ง
		// เช่น 2.4x = 1.5 * 1.6
		firstStep := 1.5
		secondStep := speed / firstStep
		if secondStep > 2.0 {
			// หากยังเกิน 2.0 ให้แบ่งเป็น 3 ขั้น
			firstStep = 1.4
			secondStep = 1.5
			thirdStep := speed / (firstStep * secondStep)
			audioFilter = fmt.Sprintf("atempo=%.2f,atempo=%.2f,atempo=%.2f,dynaudnorm=p=0.9:s=5", firstStep, secondStep, thirdStep)
		} else {
			audioFilter = fmt.Sprintf("atempo=%.2f,atempo=%.2f,dynaudnorm=p=0.9:s=5", firstStep, secondStep)
		}
	}

	// ใช้ high-quality speed adjustment
	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-af", audioFilter, // รวม atempo และ dynaudnorm ใน filter เดียว
		"-c:a", "libmp3lame", // High-quality MP3 encoder
		"-b:a", "320k", // Maximum bitrate
		"-ar", "48000", // High sample rate
		"-ac", "2", // Stereo
		tempFile,
		"-y")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// ลบไฟล์ temp หากมีข้อผิดพลาด
		os.Remove(tempFile)
		return fmt.Errorf("ffmpeg speed adjustment error: %v\nOutput: %s", err, string(output))
	}

	// แทนที่ไฟล์เดิมด้วยไฟล์ที่ปรับความเร็วแล้ว
	err = os.Rename(tempFile, outputFile)
	if err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("ไม่สามารถแทนที่ไฟล์ได้: %v", err)
	}

	return nil
}

// เพิ่มฟังก์ชันสำหรับ post-processing เสียงคุณภาพสูง
func enhanceAudioQuality(inputFile, outputFile string) error {
	fmt.Println("🎛️ กำลังปรับปรุงคุณภาพเสียง...")

	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-c:a", "libmp3lame",
		"-b:a", "320k",
		"-ar", "48000",
		"-ac", "2",
		"-af", "highpass=f=80,lowpass=f=15000,dynaudnorm=p=0.9:s=5:m=15,volume=1.1", // Filter chain สำหรับปรับปรุงเสียง
		outputFile,
		"-y")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg enhancement error: %v\nOutput: %s", err, string(output))
	}

	return nil
}

// ประมวลผลไฟล์เดียวด้วย Google Translate TTS สำหรับ multi-worker
func processFileWithGoogleTTSWorker(job TTSJob, workerTempDir string) ([]string, error) {
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s\n", filepath.Base(job.FilePath))

	// ทำความสะอาดข้อความก่อนประมวลผล
	cleanedText := cleanTextForTTS(job.Text)
	if cleanedText == "" {
		return nil, fmt.Errorf("ไม่มีข้อความที่สามารถอ่านได้หลังจากทำความสะอาด")
	}

	// แบ่งข้อความเป็นส่วนย่อย (เพิ่มขนาดให้เหมาะสมกับภาษาไทย)
	parts := splitText(cleanedText, 150)
	fmt.Printf("📑 ไฟล์ %s แบ่งเป็น %d ส่วน\n", filepath.Base(job.FilePath), len(parts))

	var audioFiles []string

	for i, part := range parts {
		fmt.Printf("🎵 Worker กำลังสร้างเสียง %s ส่วน %d/%d...\n", filepath.Base(job.FilePath), i+1, len(parts))

		// เข้ารหัส URL
		encodedText := url.QueryEscape(part)
		ttsURL := fmt.Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&tl=th&client=tw-ob&q=%s", encodedText)

		// สร้าง HTTP request พร้อม headers
		req, err := http.NewRequest("GET", ttsURL, nil)
		if err != nil {
			fmt.Printf("⚠️ Worker: ไม่สามารถสร้าง request %s ส่วน %d: %s\n", filepath.Base(job.FilePath), i+1, err.Error())
			continue
		}

		// เพิ่ม headers ที่จำเป็น
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Referer", "https://translate.google.com/")

		// ส่ง request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("⚠️ Worker: ไม่สามารถดาวน์โหลดเสียง %s ส่วน %d: %s\n", filepath.Base(job.FilePath), i+1, err.Error())
			continue
		}

		// ตรวจสอบ status code
		if resp.StatusCode != 200 {
			fmt.Printf("⚠️ Worker: ได้รับ status code %d สำหรับ %s ส่วน %d\n", resp.StatusCode, filepath.Base(job.FilePath), i+1)
			resp.Body.Close()
			continue
		}

		// อ่านข้อมูลเสียง
		audioData, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("⚠️ Worker: ไม่สามารถอ่านข้อมูลเสียง %s ส่วน %d: %s\n", filepath.Base(job.FilePath), i+1, err.Error())
			continue
		}

		// ตรวจสอบว่าได้ไฟล์เสียงจริงๆ
		if len(audioData) < 1000 || strings.Contains(string(audioData[:100]), "<html") {
			fmt.Printf("⚠️ Worker: ได้รับข้อมูลที่ไม่ใช่เสียงสำหรับ %s ส่วน %d\n", filepath.Base(job.FilePath), i+1)
			continue
		}

		// บันทึกไฟล์ส่วนย่อยใน workerTempDir
		tempFilename := filepath.Join(workerTempDir, fmt.Sprintf("temp_part_%d.mp3", i+1))
		err = os.WriteFile(tempFilename, audioData, 0644)
		if err != nil {
			fmt.Printf("⚠️ Worker: ไม่สามารถบันทึกไฟล์ %s ส่วน %d: %s\n", filepath.Base(job.FilePath), i+1, err.Error())
			continue
		}

		audioFiles = append(audioFiles, tempFilename)
		fmt.Printf("✅ Worker: บันทึก %s ส่วน %d สำเร็จ (%.1f KB)\n", filepath.Base(job.FilePath), i+1, float64(len(audioData))/1024)

		// รอระหว่างการดาวน์โหลดเพื่อไม่ให้ถูก rate limit
		time.Sleep(800 * time.Millisecond)
	}

	return audioFiles, nil
}

// ประมวลผลไฟล์เดียวด้วย Google Cloud TTS สำหรับ multi-worker
func processFileWithCloudTTSWorker(client *texttospeech.Client, ctx context.Context, job TTSJob) error {
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s ด้วย Google Cloud TTS\n", filepath.Base(job.FilePath))

	// ทำความสะอาดข้อความก่อนประมวลผล
	cleanedText := cleanTextForTTS(job.Text)
	if cleanedText == "" {
		return fmt.Errorf("ไม่มีข้อความที่สามารถอ่านได้หลังจากทำความสะอาด")
	}

	// สร้าง request
	req := &texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: cleanedText},
		},
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "th-TH",
			Name:         "th-TH-Neural2-C",
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

	// เรียก API
	fmt.Printf("🎙️ Worker กำลังสร้างเสียง %s ด้วย Google Cloud TTS...\n", filepath.Base(job.FilePath))
	resp, err := client.SynthesizeSpeech(ctx, req)
	if err != nil {
		return fmt.Errorf("ไม่สามารถสร้างเสียงได้: %v", err)
	}

	// บันทึกไฟล์ชั่วคราว
	tempFile := job.OutputPath + ".temp.mp3"
	err = os.WriteFile(tempFile, resp.AudioContent, 0644)
	if err != nil {
		return fmt.Errorf("ไม่สามารถบันทึกไฟล์เสียงชั่วคราวได้: %v", err)
	}

	// ปรับปรุงคุณภาพเสียง
	err = enhanceAudioQuality(tempFile, job.OutputPath)
	if err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("ไม่สามารถปรับปรุงคุณภาพเสียงได้: %v", err)
	}

	// ลบไฟล์ชั่วคราว
	os.Remove(tempFile)
	return nil
}

// TTS Worker function
func ttsWorker(workerID int, jobs <-chan TTSJob, results chan<- TTSResult, client *texttospeech.Client, ctx context.Context, useCloudTTS bool, audioSpeed float64) {
	fmt.Printf("🚀 Worker %d เริ่มทำงาน\n", workerID)

	for job := range jobs {
		fmt.Printf("👷 Worker %d รับงาน: %s\n", workerID, filepath.Base(job.FilePath))

		// สร้าง temp directory สำหรับ worker นี้
		workerTempDir := filepath.Join("output", fmt.Sprintf("temp_worker_%d", workerID))
		err := ensureDir(workerTempDir)
		if err != nil {
			results <- TTSResult{Job: job, Success: false, Error: fmt.Errorf("ไม่สามารถสร้าง temp directory: %v", err)}
			continue
		}

		var processingError error

		if useCloudTTS && client != nil {
			// ใช้ Google Cloud TTS
			err = processFileWithCloudTTSWorker(client, ctx, job)
			if err != nil {
				fmt.Printf("❌ Worker %d: Google Cloud TTS ล้มเหลว: %s\n", workerID, err.Error())
				// fallback ไป Google Translate TTS
				audioFiles, err2 := processFileWithGoogleTTSWorker(job, workerTempDir)
				if err2 != nil || len(audioFiles) == 0 {
					processingError = fmt.Errorf("cloud TTS และ Translate TTS ล้มเหลวทั้งคู่: %v, %v", err, err2)
				} else {
					// รวมไฟล์เสียง
					err = combineAudioFiles(workerTempDir, job.OutputPath)
					if err != nil {
						processingError = fmt.Errorf("ไม่สามารถรวมไฟล์เสียงได้: %v", err)
					}
				}
			}

			// ปรับความเร็วสำหรับ Cloud TTS
			if processingError == nil {
				tempSpeedFile := job.OutputPath + ".speed.mp3"
				err = adjustAudioSpeed(job.OutputPath, tempSpeedFile, audioSpeed)
				if err == nil {
					os.Rename(tempSpeedFile, job.OutputPath)
					fmt.Printf("⚡ Worker %d: ปรับความเร็วเสร็จสิ้น (%.1fx)\n", workerID, audioSpeed)
				} else {
					fmt.Printf("⚠️ Worker %d: ไม่สามารถปรับความเร็วได้: %s\n", workerID, err.Error())
				}
			}
		} else {
			// ใช้ Google Translate TTS
			audioFiles, err := processFileWithGoogleTTSWorker(job, workerTempDir)
			if err != nil || len(audioFiles) == 0 {
				processingError = fmt.Errorf("google Translate TTS ล้มเหลว: %v", err)
			} else {
				// รวมไฟล์เสียง
				fmt.Printf("🔗 Worker %d: กำลังรวมไฟล์เสียง %s...\n", workerID, filepath.Base(job.FilePath))
				err = combineAudioFiles(workerTempDir, job.OutputPath)
				if err != nil {
					processingError = fmt.Errorf("ไม่สามารถรวมไฟล์เสียงได้: %v", err)
				} else {
					// ปรับความเร็วไฟล์เสียง
					err = adjustAudioSpeed(job.OutputPath, job.OutputPath, audioSpeed)
					if err != nil {
						fmt.Printf("⚠️ Worker %d: ไม่สามารถปรับความเร็วได้: %s\n", workerID, err.Error())
					} else {
						fmt.Printf("⚡ Worker %d: ปรับความเร็วเสร็จสิ้น (%.1fx)\n", workerID, audioSpeed)
					}
				}
			}
		}

		// ลบไฟล์ temp ของ worker นี้
		cleanTempFolder(workerTempDir)
		os.Remove(workerTempDir)

		// ส่งผลลัพธ์
		var fileSize int64 = 0
		if processingError == nil {
			if info, err := os.Stat(job.OutputPath); err == nil {
				fileSize = info.Size()
				fmt.Printf("✅ Worker %d: เสร็จสิ้น %s (%.1f KB)\n", workerID, filepath.Base(job.FilePath), float64(fileSize)/1024)
			}
		} else {
			fmt.Printf("❌ Worker %d: ล้มเหลว %s: %s\n", workerID, filepath.Base(job.FilePath), processingError.Error())
		}

		results <- TTSResult{
			Job:     job,
			Success: processingError == nil,
			Error:   processingError,
			Size:    fileSize,
		}
	}

	fmt.Printf("🏁 Worker %d เสร็จสิ้นงาน\n", workerID)
}

func main() {
	// ตั้งค่าความเร็ว (1.0 = ปกติ, 1.3 = เร็วขึ้น 30%, 1.4 = เร็วขึ้น 40%)
	const AUDIO_SPEED_MULTIPLIER = 1.6
	// จำนวน workers (จำนวนไฟล์ที่ประมวลผลพร้อมกัน)
	const NUM_WORKERS = 4

	fmt.Printf("🚀 เริ่มต้นระบบ Multi-Worker TTS (%d workers)\n", NUM_WORKERS)

	// สร้าง folders ที่จำเป็น
	outputDir := "output"
	err := ensureDir(outputDir)
	if err != nil {
		panic("ไม่สามารถสร้าง output folder: " + err.Error())
	}

	// หาไฟล์ข้อความทั้งหมดใน chapters
	chaptersDir := "chapters"
	pattern := filepath.Join(chaptersDir, "*.txt")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		panic("ไม่พบไฟล์ .txt ใน folder chapters")
	}

	// เรียงลำดับไฟล์
	sort.Strings(files)

	fmt.Printf("📚 พบไฟล์ที่จะประมวลผล %d ไฟล์:\n", len(files))
	for i, file := range files {
		fmt.Printf("   %d. %s\n", i+1, filepath.Base(file))
	}

	// สร้าง context สำหรับ Google Cloud TTS
	ctx := context.Background()
	client, err := texttospeech.NewClient(ctx)
	useCloudTTS := (err == nil)
	if useCloudTTS {
		defer client.Close()
		fmt.Println("✅ ใช้ Google Cloud TTS")
	} else {
		fmt.Println("⚠️ ไม่สามารถเชื่อมต่อ Google Cloud TTS, ใช้ Google Translate TTS แทน")
	}

	// อ่านไฟล์ทั้งหมดและสร้าง jobs
	var jobs []TTSJob
	for i, file := range files {
		// อ่านเนื้อหาไฟล์
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("❌ ไม่สามารถอ่านไฟล์ %s: %s\n", file, err.Error())
			continue
		}

		text := strings.TrimSpace(string(data))
		if text == "" {
			fmt.Printf("⚠️ ไฟล์ %s ว่างเปล่า\n", file)
			continue
		}

		// สร้างชื่อไฟล์ output
		baseName := strings.TrimSuffix(filepath.Base(file), ".txt")
		outputFile := filepath.Join(outputDir, baseName+".mp3")

		job := TTSJob{
			ID:         i + 1,
			FilePath:   file,
			OutputPath: outputFile,
			Text:       text,
		}
		jobs = append(jobs, job)
	}

	if len(jobs) == 0 {
		fmt.Println("❌ ไม่มีไฟล์ที่สามารถประมวลผลได้")
		return
	}

	fmt.Printf("🎯 เตรียมประมวลผล %d งาน ด้วย %d workers\n", len(jobs), NUM_WORKERS)

	// สร้าง channels สำหรับการประสานงาน
	jobsChan := make(chan TTSJob, len(jobs))
	resultsChan := make(chan TTSResult, len(jobs))

	// เริ่มต้น workers
	var wg sync.WaitGroup
	for workerID := 1; workerID <= NUM_WORKERS; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ttsWorker(id, jobsChan, resultsChan, client, ctx, useCloudTTS, AUDIO_SPEED_MULTIPLIER)
		}(workerID)
	}

	// ส่งงานทั้งหมดลง channel
	startTime := time.Now()
	for _, job := range jobs {
		jobsChan <- job
	}
	close(jobsChan)

	// รอให้ workers เสร็จสิ้น
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// รับผลลัพธ์
	var results []TTSResult
	var successCount, failCount int
	var totalSize int64

	fmt.Println("\n📊 กำลังรับผลลัพธ์...")
	for result := range resultsChan {
		results = append(results, result)
		if result.Success {
			successCount++
			totalSize += result.Size
			fmt.Printf("✅ เสร็จสิ้น: %s (%.1f KB)\n",
				filepath.Base(result.Job.FilePath),
				float64(result.Size)/1024)
		} else {
			failCount++
			fmt.Printf("❌ ล้มเหลว: %s - %s\n",
				filepath.Base(result.Job.FilePath),
				result.Error.Error())
		}
	}

	duration := time.Since(startTime)

	// แสดงสรุปผลลัพธ์
	fmt.Printf("\n🎉 ประมวลผลเสร็จสิ้นทั้งหมด!\n")
	fmt.Printf("⏱️  เวลาที่ใช้: %.1f วินาที\n", duration.Seconds())
	fmt.Printf("✅ สำเร็จ: %d ไฟล์\n", successCount)
	fmt.Printf("❌ ล้มเหลว: %d ไฟล์\n", failCount)
	fmt.Printf("📁 ไฟล์เสียงทั้งหมดอยู่ใน folder: %s\n", outputDir)
	fmt.Printf("💾 ขนาดไฟล์รวม: %.1f MB\n", float64(totalSize)/(1024*1024))

	// แสดงรายการไฟล์ที่สร้างขึ้น
	if successCount > 0 {
		fmt.Println("\n📄 ไฟล์ที่สร้างขึ้น:")
		outputPattern := filepath.Join(outputDir, "*.mp3")
		outputFiles, err := filepath.Glob(outputPattern)
		if err == nil && len(outputFiles) > 0 {
			sort.Strings(outputFiles)
			for i, file := range outputFiles {
				if info, err := os.Stat(file); err == nil {
					fmt.Printf("   %d. %s (%.1f KB)\n", i+1, filepath.Base(file), float64(info.Size())/1024)
				}
			}
		}
	}

	if failCount > 0 {
		fmt.Println("\n⚠️  ไฟล์ที่ล้มเหลว:")
		for _, result := range results {
			if !result.Success {
				fmt.Printf("   - %s: %s\n", filepath.Base(result.Job.FilePath), result.Error.Error())
			}
		}
	}

	fmt.Printf("\n🏁 Multi-Worker TTS เสร็จสิ้น!\n")
}
