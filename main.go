package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"

	"k-tts/pkg/tts"
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

// รวมไฟล์เสียงด้วย ffmpeg
func combineAudioFiles(tempDir, outputFile string) error {
	files, err := tts.CombineAudioFiles(tempDir, outputFile)
	if len(files) > 0 {
		fmt.Println("📋 ลำดับไฟล์ที่จะรวม:")
		for i, file := range files {
			fmt.Printf("   %d. %s\n", i+1, filepath.Base(file))
		}
	}
	return err
}

// ปรับความเร็วและใส่ voice FX ของไฟล์เสียงด้วย ffmpeg
func adjustAudioSpeed(inputFile, outputFile string, speed float64, fx tts.VoiceFX) error {
	if fx.Name != "" {
		fmt.Printf("⚡ กำลังปรับความเร็วเป็น %.2fx + voice fx %s...\n", speed, fx.Name)
	} else {
		fmt.Printf("⚡ กำลังปรับความเร็วเป็น %.2fx...\n", speed)
	}
	return tts.ApplyVoiceFX(inputFile, outputFile, speed, fx, tts.DefaultLoudness)
}

// ประมวลผลไฟล์เดียวด้วย Google Translate TTS สำหรับ multi-worker
func processFileWithGoogleTTSWorker(job TTSJob, workerTempDir string) ([]string, error) {
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s\n", filepath.Base(job.FilePath))

	// ทำความสะอาดข้อความก่อนประมวลผล
	cleanedText := tts.CleanText(job.Text)
	if cleanedText == "" {
		return nil, fmt.Errorf("ไม่มีข้อความที่สามารถอ่านได้หลังจากทำความสะอาด")
	}

	// แบ่งข้อความเป็นส่วนย่อย (เพิ่มขนาดให้เหมาะสมกับภาษาไทย)
	parts := tts.SplitText(cleanedText, tts.TranslateChunkLen)
	fmt.Printf("📑 ไฟล์ %s แบ่งเป็น %d ส่วน\n", filepath.Base(job.FilePath), len(parts))

	var audioFiles []string

	for i, part := range parts {
		fmt.Printf("🎵 Worker กำลังสร้างเสียง %s ส่วน %d/%d...\n", filepath.Base(job.FilePath), i+1, len(parts))

		audioData, err := tts.FetchTranslateTTS(context.Background(), part, "th")
		if err != nil {
			fmt.Printf("⚠️ Worker: ไม่สามารถดาวน์โหลดเสียง %s ส่วน %d: %s\n", filepath.Base(job.FilePath), i+1, err.Error())
			continue
		}

		// บันทึกไฟล์ส่วนย่อยใน workerTempDir
		tempFilename := filepath.Join(workerTempDir, fmt.Sprintf("temp_part_%d.mp3", i+1))
		if err := os.WriteFile(tempFilename, audioData, 0644); err != nil {
			fmt.Printf("⚠️ Worker: ไม่สามารถบันทึกไฟล์ %s ส่วน %d: %s\n", filepath.Base(job.FilePath), i+1, err.Error())
			continue
		}

		audioFiles = append(audioFiles, tempFilename)
		fmt.Printf("✅ Worker: บันทึก %s ส่วน %d สำเร็จ (%.1f KB)\n", filepath.Base(job.FilePath), i+1, float64(len(audioData))/1024)

		// รอระหว่างการดาวน์โหลดเพื่อไม่ให้ถูก rate limit
		time.Sleep(tts.TranslateDelay)
	}

	return audioFiles, nil
}

// ประมวลผลไฟล์เดียวด้วย Google Cloud TTS สำหรับ multi-worker
func processFileWithCloudTTSWorker(client *texttospeech.Client, ctx context.Context, job TTSJob) error {
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s ด้วย Google Cloud TTS\n", filepath.Base(job.FilePath))

	// ทำความสะอาดข้อความก่อนประมวลผล
	cleanedText := tts.CleanText(job.Text)
	if cleanedText == "" {
		return fmt.Errorf("ไม่มีข้อความที่สามารถอ่านได้หลังจากทำความสะอาด")
	}

	fmt.Printf("🎙️ Worker กำลังสร้างเสียง %s ด้วย Google Cloud TTS...\n", filepath.Base(job.FilePath))
	audio, err := tts.SynthesizeCloud(ctx, client, cleanedText, tts.DefaultVoice)
	if err != nil {
		return err
	}

	// บันทึกไฟล์ชั่วคราว
	tempFile := job.OutputPath + ".temp.mp3"
	if err := os.WriteFile(tempFile, audio, 0644); err != nil {
		return fmt.Errorf("ไม่สามารถบันทึกไฟล์เสียงชั่วคราวได้: %v", err)
	}
	defer os.Remove(tempFile)

	// ปรับปรุงคุณภาพเสียง
	fmt.Println("🎛️ กำลังปรับปรุงคุณภาพเสียง...")
	if err := tts.EnhanceAudioQuality(tempFile, job.OutputPath); err != nil {
		return fmt.Errorf("ไม่สามารถปรับปรุงคุณภาพเสียงได้: %v", err)
	}
	return nil
}

// TTS Worker function
func ttsWorker(workerID int, jobs <-chan TTSJob, results chan<- TTSResult, client *texttospeech.Client, ctx context.Context, useCloudTTS bool, audioSpeed float64, voiceFX tts.VoiceFX) {
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
				err = adjustAudioSpeed(job.OutputPath, tempSpeedFile, audioSpeed, voiceFX)
				if err == nil {
					os.Rename(tempSpeedFile, job.OutputPath)
					fmt.Printf("⚡ Worker %d: ปรับความเร็วเสร็จสิ้น (%.2fx)\n", workerID, audioSpeed)
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
					err = adjustAudioSpeed(job.OutputPath, job.OutputPath, audioSpeed, voiceFX)
					if err != nil {
						fmt.Printf("⚠️ Worker %d: ไม่สามารถปรับความเร็วได้: %s\n", workerID, err.Error())
					} else {
						fmt.Printf("⚡ Worker %d: ปรับความเร็วเสร็จสิ้น (%.2fx)\n", workerID, audioSpeed)
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
	// ตั้งค่าความเร็ว (1.0 = ปกติ, 1.3 = เร็วขึ้น 30%, 1.75 = เร็วขึ้น 75%)
	const AUDIO_SPEED_MULTIPLIER = 1.75
	// voice FX สำหรับหนังสือเสียง ("" = ไม่ใส่, clear, bright, warm, deep, soft, broadcast, room)
	const VOICE_FX = "clear"
	// จำนวน workers (จำนวนไฟล์ที่ประมวลผลพร้อมกัน)
	const NUM_WORKERS = 4

	voiceFX, err := tts.LookupVoiceFX(VOICE_FX)
	if err != nil {
		panic(err)
	}

	fmt.Printf("🚀 เริ่มต้นระบบ Multi-Worker TTS (%d workers)\n", NUM_WORKERS)

	// สร้าง folders ที่จำเป็น
	outputDir := "output"
	err = ensureDir(outputDir)
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
			ttsWorker(id, jobsChan, resultsChan, client, ctx, useCloudTTS, AUDIO_SPEED_MULTIPLIER, voiceFX)
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
