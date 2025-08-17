package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	"cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
)

// แบ่งข้อความเป็นส่วนย่อยสำหรับ Google Translate TTS
func splitText(text string, maxLen int) []string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return []string{text}
	}

	var parts []string

	// แบ่งตามประโยค (จุด, อัศเจรีย์, คำถาม)
	sentences := []string{}
	current := ""

	for i, r := range runes {
		current += string(r)
		if r == '.' || r == '!' || r == '?' || r == '।' {
			// หาช่องว่างถัดไป
			if i+1 < len(runes) && runes[i+1] == ' ' {
				sentences = append(sentences, strings.TrimSpace(current))
				current = ""
			}
		}
	}

	// เพิ่มส่วนที่เหลือ
	if strings.TrimSpace(current) != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}

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

	// หากยังมีส่วนที่ยาวเกินไป ให้แบ่งต่อ
	var finalParts []string
	for _, part := range parts {
		if len([]rune(part)) <= maxLen {
			finalParts = append(finalParts, part)
		} else {
			// แบ่งโดยอักขระ
			partRunes := []rune(part)
			for i := 0; i < len(partRunes); i += maxLen {
				end := i + maxLen
				if end > len(partRunes) {
					end = len(partRunes)
				}
				finalParts = append(finalParts, string(partRunes[i:end]))
			}
		}
	}

	return finalParts
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
		data, err := ioutil.ReadFile(files[0])
		if err != nil {
			return err
		}
		return ioutil.WriteFile(outputFile, data, 0644)
	}

	// สร้างไฟล์รายการสำหรับ ffmpeg (ใช้ relative path จาก tempDir)
	filelistPath := filepath.Join(tempDir, "filelist.txt")
	var filelistContent strings.Builder
	for _, file := range files {
		// ใช้ชื่อไฟล์เท่านั้น (relative path)
		filename := filepath.Base(file)
		filelistContent.WriteString(fmt.Sprintf("file '%s'\n", filename))
	}

	err = ioutil.WriteFile(filelistPath, []byte(filelistContent.String()), 0644)
	if err != nil {
		return err
	}

	// รันคำสั่ง ffmpeg จาก tempDir โดยใช้ absolute path สำหรับ output
	absOutputFile, err := filepath.Abs(outputFile)
	if err != nil {
		return err
	}

	cmd := exec.Command("ffmpeg", "-f", "concat", "-safe", "0", "-i", "filelist.txt", "-c", "copy", absOutputFile, "-y")
	cmd.Dir = tempDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}

	// ลบไฟล์รายการ
	os.Remove(filelistPath)

	return nil
}

// ประมวลผลไฟล์เดียวด้วย Google Translate TTS
func processFileWithGoogleTTS(filename, text, tempDir string) ([]string, error) {
	fmt.Printf("🔄 ประมวลผล: %s\n", filename)

	// แบ่งข้อความเป็นส่วนย่อย
	parts := splitText(text, 80)
	fmt.Printf("📑 แบ่งข้อความเป็น %d ส่วน\n", len(parts))

	var audioFiles []string

	for i, part := range parts {
		fmt.Printf("🎵 กำลังสร้างเสียงส่วนที่ %d/%d...\n", i+1, len(parts))

		// เข้ารหัส URL
		encodedText := url.QueryEscape(part)
		ttsURL := fmt.Sprintf("https://translate.google.com/translate_tts?ie=UTF-8&tl=th&client=tw-ob&q=%s", encodedText)

		// สร้าง HTTP request พร้อม headers
		req, err := http.NewRequest("GET", ttsURL, nil)
		if err != nil {
			fmt.Printf("⚠️ ไม่สามารถสร้าง request ส่วนที่ %d: %s\n", i+1, err.Error())
			continue
		}

		// เพิ่ม headers ที่จำเป็น
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		req.Header.Set("Referer", "https://translate.google.com/")

		// ส่ง request
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("⚠️ ไม่สามารถดาวน์โหลดเสียงส่วนที่ %d: %s\n", i+1, err.Error())
			continue
		}

		// ตรวจสอบ status code
		if resp.StatusCode != 200 {
			fmt.Printf("⚠️ ได้รับ status code %d สำหรับส่วนที่ %d\n", resp.StatusCode, i+1)
			resp.Body.Close()
			continue
		}

		// อ่านข้อมูลเสียง
		audioData, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("⚠️ ไม่สามารถอ่านข้อมูลเสียงส่วนที่ %d: %s\n", i+1, err.Error())
			continue
		}

		// ตรวจสอบว่าได้ไฟล์เสียงจริงๆ
		if len(audioData) < 1000 || strings.Contains(string(audioData[:100]), "<html") {
			fmt.Printf("⚠️ ได้รับข้อมูลที่ไม่ใช่เสียงสำหรับส่วนที่ %d\n", i+1)
			continue
		}

		// บันทึกไฟล์ส่วนย่อยใน tempDir
		tempFilename := filepath.Join(tempDir, fmt.Sprintf("temp_part_%d.mp3", i+1))
		err = ioutil.WriteFile(tempFilename, audioData, 0644)
		if err != nil {
			fmt.Printf("⚠️ ไม่สามารถบันทึกไฟล์ส่วนที่ %d: %s\n", i+1, err.Error())
			continue
		}

		audioFiles = append(audioFiles, tempFilename)
		fmt.Printf("✅ บันทึกส่วนที่ %d สำเร็จ (%.1f KB)\n", i+1, float64(len(audioData))/1024)

		// รอระหว่างการดาวน์โหลด
		time.Sleep(1 * time.Second)
	}

	return audioFiles, nil
}

// ประมวลผลไฟล์เดียวด้วย Google Cloud TTS
func processFileWithCloudTTS(client *texttospeech.Client, ctx context.Context, filename, text, outputPath string) error {
	fmt.Printf("🔄 ประมวลผล: %s ด้วย Google Cloud TTS\n", filename)

	// สร้าง request
	req := &texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: text},
		},
		Voice: &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "th-TH",
			Name:         "th-TH-Neural2-C",
			SsmlGender:   texttospeechpb.SsmlVoiceGender_FEMALE,
		},
		AudioConfig: &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
			SpeakingRate:  1.5,
			Pitch:         0.0,
			VolumeGainDb:  0.0,
		},
	}

	// เรียก API
	fmt.Println("🎙️ กำลังสร้างเสียงด้วย Google Cloud TTS...")
	resp, err := client.SynthesizeSpeech(ctx, req)
	if err != nil {
		return fmt.Errorf("ไม่สามารถสร้างเสียงได้: %v", err)
	}

	// บันทึกไฟล์เสียง
	err = ioutil.WriteFile(outputPath, resp.AudioContent, 0644)
	if err != nil {
		return fmt.Errorf("ไม่สามารถบันทึกไฟล์เสียงได้: %v", err)
	}

	return nil
}

func main() {
	// สร้าง folders ที่จำเป็น
	outputDir := "output"
	tempDir := filepath.Join(outputDir, "temp")

	err := ensureDir(outputDir)
	if err != nil {
		panic("ไม่สามารถสร้าง output folder: " + err.Error())
	}

	err = ensureDir(tempDir)
	if err != nil {
		panic("ไม่สามารถสร้าง temp folder: " + err.Error())
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

	// ประมวลผลแต่ละไฟล์
	for i, file := range files {
		fmt.Printf("\n🎯 กำลังประมวลผลไฟล์ %d/%d: %s\n", i+1, len(files), filepath.Base(file))

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

		fmt.Printf("📊 จำนวนอักษร: %d\n", len([]rune(text)))

		// สร้างชื่อไฟล์ output
		baseName := strings.TrimSuffix(filepath.Base(file), ".txt")
		outputFile := filepath.Join(outputDir, baseName+".mp3")

		if useCloudTTS {
			// ใช้ Google Cloud TTS
			err = processFileWithCloudTTS(client, ctx, baseName, text, outputFile)
			if err != nil {
				fmt.Printf("❌ Google Cloud TTS ล้มเหลว: %s\n", err.Error())
				fmt.Println("🔄 เปลี่ยนไปใช้ Google Translate TTS...")
				useCloudTTS = false
			} else {
				if info, err := os.Stat(outputFile); err == nil {
					fmt.Printf("✅ เสร็จสิ้น → %s (%.1f KB)\n", outputFile, float64(info.Size())/1024)
				}
				continue
			}
		}

		if !useCloudTTS {
			// ใช้ Google Translate TTS
			audioFiles, err := processFileWithGoogleTTS(baseName, text, tempDir)
			if err != nil || len(audioFiles) == 0 {
				fmt.Printf("❌ ไม่สามารถประมวลผล %s ได้\n", file)
				continue
			}

			// รวมไฟล์เสียง
			fmt.Println("🔗 กำลังรวมไฟล์เสียง...")
			err = combineAudioFiles(tempDir, outputFile)
			if err != nil {
				fmt.Printf("❌ ไม่สามารถรวมไฟล์เสียงได้: %s\n", err.Error())
				continue
			}

			// แสดงผลลัพธ์
			if info, err := os.Stat(outputFile); err == nil {
				fmt.Printf("✅ เสร็จสิ้น → %s (%.1f KB)\n", outputFile, float64(info.Size())/1024)
			}

			// ลบไฟล์ temp
			fmt.Println("🗑️  กำลังลบไฟล์ชั่วคราว...")
			cleanTempFolder(tempDir)
		}
	}

	fmt.Printf("\n🎉 ประมวลผลเสร็จสิ้นทั้งหมด!\n")
	fmt.Printf("📁 ไฟล์เสียงทั้งหมดอยู่ใน folder: %s\n", outputDir)

	// แสดงรายการไฟล์ที่สร้างขึ้น
	outputPattern := filepath.Join(outputDir, "*.mp3")
	outputFiles, err := filepath.Glob(outputPattern)
	if err == nil && len(outputFiles) > 0 {
		fmt.Println("📄 ไฟล์ที่สร้างขึ้น:")
		for i, file := range outputFiles {
			if info, err := os.Stat(file); err == nil {
				fmt.Printf("   %d. %s (%.1f KB)\n", i+1, filepath.Base(file), float64(info.Size())/1024)
			}
		}
	}
}
