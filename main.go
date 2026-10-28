package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// translateFallbackSpeed คือความเร็วของเสียงที่ถอยไปใช้ Google Translate TTS
const translateFallbackSpeed = 1.75

// เร่งความเร็วเสียงจาก 9router โดยไม่ใส่ voice FX หรือ loudness
func speedUpRouterAudio(path string, speed float64) error {
	if speed == 1 {
		fmt.Println("⚡ ใช้เสียงจาก 9router ตามเดิม")
		return nil
	}
	fmt.Printf("⚡ กำลังเร่งความเร็วเสียง 9router เป็น %.2fx โดยไม่ปรับโทนเสียง...\n", speed)
	if err := tts.SpeedUp(path, path, speed); err != nil {
		return err
	}
	fmt.Printf("⚡ เร่งความเร็วเสร็จสิ้น (%.2fx)\n", speed)
	return nil
}

// เร่งความเร็วเสียง Google Translate TTS ด้วย atempo อย่างเดียว
func speedUpTranslateAudio(path string, speed float64) error {
	fmt.Printf("⚡ กำลังเร่งความเร็ว Google Translate TTS เป็น %.2fx...\n", speed)
	if err := tts.SpeedUp(path, path, speed); err != nil {
		return err
	}
	fmt.Printf("⚡ เร่งความเร็วเสร็จสิ้น (%.2fx)\n", speed)
	return nil
}

func finishWithTranslateFallback(workerID int, job TTSJob, workerTempDir string, routerErr error) error {
	audioFiles, err := processFileWithGoogleTTSWorker(job, workerTempDir)
	if err != nil || len(audioFiles) == 0 {
		return fmt.Errorf("9router ล้มเหลว: %v; Google Translate TTS ล้มเหลว: %v", routerErr, err)
	}
	fmt.Printf("🔗 Worker %d: กำลังรวมไฟล์เสียง %s...\n", workerID, filepath.Base(job.FilePath))
	if err := combineAudioFiles(workerTempDir, job.OutputPath); err != nil {
		return fmt.Errorf("ไม่สามารถรวมไฟล์เสียงได้: %v", err)
	}
	if err := speedUpTranslateAudio(job.OutputPath, translateFallbackSpeed); err != nil {
		return fmt.Errorf("ไม่สามารถเร่งความเร็วได้: %v", err)
	}
	return nil
}

func fetchTranslateWithRetry(text string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		audio, err := tts.FetchTranslateTTS(context.Background(), text, "th")
		if err == nil {
			return audio, nil
		}
		last = err
		fmt.Printf("⚠️ Google Translate TTS ล้มเหลว ลองใหม่ครั้งที่ %d: %s\n", attempt, err.Error())
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, last
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
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s ด้วย Google Translate TTS\n", filepath.Base(job.FilePath))

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

		audioData, err := fetchTranslateWithRetry(part)
		if err != nil {
			return nil, fmt.Errorf("ส่วน %d: %w", i+1, err)
		}

		// บันทึกไฟล์ส่วนย่อยใน workerTempDir
		tempFilename := filepath.Join(workerTempDir, fmt.Sprintf("temp_part_%d.mp3", i+1))
		if err := os.WriteFile(tempFilename, audioData, 0644); err != nil {
			return nil, fmt.Errorf("ไม่สามารถบันทึกไฟล์ส่วน %d: %w", i+1, err)
		}

		audioFiles = append(audioFiles, tempFilename)
		fmt.Printf("✅ Worker: บันทึก %s ส่วน %d สำเร็จ (%.1f KB)\n", filepath.Base(job.FilePath), i+1, float64(len(audioData))/1024)

		// รอระหว่างคำขอของ worker นี้ worker อื่นยังยิงพร้อมกันได้
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

// ประมวลผลไฟล์เดียวด้วย Edge TTS สำหรับ multi-worker
func processFileWithEdgeTTSWorker(ctx context.Context, client *tts.EdgeClient, job TTSJob, workerTempDir string) ([]string, error) {
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s ด้วย Edge TTS\n", filepath.Base(job.FilePath))

	cleanedText := tts.CleanText(job.Text)
	if cleanedText == "" {
		return nil, fmt.Errorf("ไม่มีข้อความที่สามารถอ่านได้หลังจากทำความสะอาด")
	}

	parts := tts.SplitText(cleanedText, tts.EdgeChunkLen)
	fmt.Printf("📑 ไฟล์ %s แบ่งเป็น %d ส่วน\n", filepath.Base(job.FilePath), len(parts))

	var audioFiles []string
	for i, part := range parts {
		fmt.Printf("🎵 Worker กำลังสร้างเสียง %s ส่วน %d/%d...\n", filepath.Base(job.FilePath), i+1, len(parts))
		audioData, err := synthesizeEdgeWithRetry(ctx, client, part)
		if err != nil {
			return nil, fmt.Errorf("ส่วน %d: %w", i+1, err)
		}

		tempFilename := filepath.Join(workerTempDir, fmt.Sprintf("temp_part_%d.mp3", i+1))
		if err := os.WriteFile(tempFilename, audioData, 0644); err != nil {
			return nil, fmt.Errorf("ไม่สามารถบันทึกไฟล์ส่วน %d: %w", i+1, err)
		}
		audioFiles = append(audioFiles, tempFilename)
		fmt.Printf("✅ Worker: บันทึก %s ส่วน %d สำเร็จ (%.1f KB)\n", filepath.Base(job.FilePath), i+1, float64(len(audioData))/1024)
	}
	return audioFiles, nil
}

// ประมวลผลไฟล์เดียวด้วย 9router โดยยิงแต่ละส่วนไปคนละ VM
func processFileWithRouter(ctx context.Context, pool *tts.RouterPool, job TTSJob, workerTempDir string) ([]string, error) {
	fmt.Printf("🔄 Worker กำลังประมวลผล: %s ด้วย 9router\n", filepath.Base(job.FilePath))

	cleanedText := tts.CleanText(job.Text)
	if cleanedText == "" {
		return nil, fmt.Errorf("ไม่มีข้อความที่สามารถอ่านได้หลังจากทำความสะอาด")
	}

	parts := tts.SplitText(cleanedText, tts.RouterChunkLen)
	fmt.Printf("📑 ไฟล์ %s แบ่งเป็น %d ส่วน\n", filepath.Base(job.FilePath), len(parts))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	audioParts := make([][]byte, len(parts))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i, part := range parts {
		wg.Add(1)
		go func(i int, part string) {
			defer wg.Done()
			audio, err := synthesizeRouterWithRetry(ctx, pool, part)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("ส่วน %d: %w", i+1, err)
					cancel()
				}
				return
			}
			audioParts[i] = audio
		}(i, part)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}

	var audioFiles []string
	for i, audio := range audioParts {
		tempFilename := filepath.Join(workerTempDir, fmt.Sprintf("temp_part_%d.mp3", i+1))
		if err := os.WriteFile(tempFilename, audio, 0644); err != nil {
			return nil, fmt.Errorf("ไม่สามารถบันทึกไฟล์ส่วน %d: %w", i+1, err)
		}
		audioFiles = append(audioFiles, tempFilename)
		fmt.Printf("✅ Worker: บันทึก %s ส่วน %d สำเร็จ (%.1f KB)\n", filepath.Base(job.FilePath), i+1, float64(len(audio))/1024)
	}
	return audioFiles, nil
}

func synthesizeRouterWithRetry(ctx context.Context, pool *tts.RouterPool, text string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		audio, err := pool.Synthesize(ctx, text)
		if err == nil {
			return audio, nil
		}
		last = err
		if ctx.Err() != nil {
			return nil, err
		}
		var status *tts.RouterStatusError
		if errors.As(err, &status) && !status.Retryable() {
			return nil, err
		}
		if attempt < 3 {
			fmt.Printf("⚠️ 9router ล้มเหลว ลองใหม่ครั้งที่ %d: %s\n", attempt, err.Error())
			select {
			case <-time.After(time.Duration(attempt) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, last
}

func synthesizeEdgeWithRetry(ctx context.Context, client *tts.EdgeClient, text string) ([]byte, error) {
	var last error
	for attempt := 1; attempt <= 3; attempt++ {
		audio, err := client.Synthesize(ctx, text)
		if err == nil {
			return audio, nil
		}
		last = err
		if attempt < 3 {
			fmt.Printf("⚠️ Edge TTS ล้มเหลว ลองใหม่ครั้งที่ %d: %s\n", attempt, err.Error())
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return nil, last
}

// TTS Worker function
func ttsWorker(workerID int, jobs <-chan TTSJob, results chan<- TTSResult, client *texttospeech.Client, ctx context.Context, useCloudTTS bool, edgeClient *tts.EdgeClient, routerPool *tts.RouterPool, audioSpeed float64, voiceFX tts.VoiceFX) {
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
		cleanTempFolder(workerTempDir)

		var processingError error

		if routerPool != nil {
			audioFiles, err := processFileWithRouter(ctx, routerPool, job, workerTempDir)
			if err != nil || len(audioFiles) == 0 {
				fmt.Printf("⚠️ Worker %d: 9router ล้มเหลว ใช้ Google Translate TTS แทน: %v\n", workerID, err)
				cleanTempFolder(workerTempDir)
				if err := finishWithTranslateFallback(workerID, job, workerTempDir, err); err != nil {
					processingError = err
				}
			} else {
				fmt.Printf("🔗 Worker %d: กำลังรวมไฟล์เสียง %s...\n", workerID, filepath.Base(job.FilePath))
				err = combineAudioFiles(workerTempDir, job.OutputPath)
				if err != nil {
					processingError = fmt.Errorf("ไม่สามารถรวมไฟล์เสียงได้: %v", err)
				} else if err = speedUpRouterAudio(job.OutputPath, audioSpeed); err != nil {
					fmt.Printf("⚠️ Worker %d: ไม่สามารถเร่งความเร็วได้: %s\n", workerID, err.Error())
				}
			}
		} else if edgeClient != nil {
			audioFiles, err := processFileWithEdgeTTSWorker(ctx, edgeClient, job, workerTempDir)
			if err != nil || len(audioFiles) == 0 {
				processingError = fmt.Errorf("edge TTS ล้มเหลว: %v", err)
			} else {
				fmt.Printf("🔗 Worker %d: กำลังรวมไฟล์เสียง %s...\n", workerID, filepath.Base(job.FilePath))
				err = combineAudioFiles(workerTempDir, job.OutputPath)
				if err != nil {
					processingError = fmt.Errorf("ไม่สามารถรวมไฟล์เสียงได้: %v", err)
				} else {
					err = adjustAudioSpeed(job.OutputPath, job.OutputPath, audioSpeed, voiceFX)
					if err != nil {
						fmt.Printf("⚠️ Worker %d: ไม่สามารถปรับความเร็วได้: %s\n", workerID, err.Error())
					} else {
						fmt.Printf("⚡ Worker %d: ปรับความเร็วเสร็จสิ้น (%.2fx)\n", workerID, audioSpeed)
					}
				}
			}
		} else if useCloudTTS && client != nil {
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
				} else if err = speedUpTranslateAudio(job.OutputPath, translateFallbackSpeed); err != nil {
					processingError = fmt.Errorf("ไม่สามารถเร่งความเร็วได้: %v", err)
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

// parseChapterSet แปลง "102,218,51-60" เป็นชุดเลขตอน ค่าว่างหมายถึงทุกตอน
func parseChapterSet(spec string) (map[int]struct{}, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	set := make(map[int]struct{})
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			bounds := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(strings.TrimSpace(bounds[0]))
			if err != nil || start < 1 {
				return nil, fmt.Errorf("invalid chapter range %q", part)
			}
			end, err := strconv.Atoi(strings.TrimSpace(bounds[1]))
			if err != nil || end < start {
				return nil, fmt.Errorf("invalid chapter range %q", part)
			}
			for n := start; n <= end; n++ {
				set[n] = struct{}{}
			}
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid chapter number %q", part)
		}
		set[n] = struct{}{}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("-only is empty")
	}
	return set, nil
}

func chapterNumber(path string) (int, bool) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	n, err := strconv.Atoi(base)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func filterChapters(files []string, selected map[int]struct{}) ([]string, []int) {
	if selected == nil {
		return files, nil
	}
	found := make(map[int]bool, len(selected))
	filtered := make([]string, 0, len(selected))
	for _, file := range files {
		n, ok := chapterNumber(file)
		if !ok {
			continue
		}
		if _, want := selected[n]; !want {
			continue
		}
		filtered = append(filtered, file)
		found[n] = true
	}
	var missing []int
	for n := range selected {
		if !found[n] {
			missing = append(missing, n)
		}
	}
	sort.Ints(missing)
	return filtered, missing
}

func parseEngine(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "auto":
		return "auto", nil
	case "cloud":
		return "cloud", nil
	case "translate":
		return "translate", nil
	case "edge":
		return "edge", nil
	case "router":
		return "router", nil
	default:
		return "", fmt.Errorf("unknown engine %q (use cloud, translate, edge, or router)", name)
	}
}

func main() {
	engineName := flag.String("engine", "", "tts engine: cloud, translate, edge, or router (default: cloud if credentials exist, otherwise translate)")
	workers := flag.Int("workers", 4, "number of chapter files processed at once")
	only := flag.String("only", "", "chapter numbers to process, comma-separated or ranges (e.g. 102,218,241,294 or 51-60); empty processes every chapter")
	edgeURL := flag.String("edge-url", tts.DefaultEdgeURL, "edge-tts server base URL")
	voice := flag.String("voice", tts.DefaultEdgeVoice, "edge-tts voice name")
	flag.Parse()
	if *workers < 1 {
		fmt.Fprintln(os.Stderr, "workers must be at least 1")
		os.Exit(1)
	}
	engine, err := parseEngine(*engineName)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	selected, err := parseChapterSet(*only)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := loadDotEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// ตั้งค่าความเร็ว (1.0 = ปกติ, 1.3 = เร็วขึ้น 30%, 1.75 = เร็วขึ้น 75%)
	const AUDIO_SPEED_MULTIPLIER = 1.25
	// voice FX สำหรับหนังสือเสียง ("" = ไม่ใส่, clear, bright, warm, deep, soft, broadcast, room, mellow)
	const VOICE_FX = ""

	voiceFX, err := tts.LookupVoiceFX(VOICE_FX)
	if err != nil {
		panic(err)
	}

	fmt.Printf("🚀 เริ่มต้นระบบ Multi-Worker TTS (%d workers)\n", *workers)

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
	files, missingChapters := filterChapters(files, selected)
	if len(missingChapters) > 0 {
		fmt.Printf("⚠️ ไม่พบไฟล์บท: %v\n", missingChapters)
	}
	if len(files) == 0 {
		panic("ไม่พบไฟล์บทตามที่ระบุใน -only")
	}

	fmt.Printf("📚 พบไฟล์ที่จะประมวลผล %d ไฟล์:\n", len(files))
	for i, file := range files {
		fmt.Printf("   %d. %s\n", i+1, filepath.Base(file))
	}

	ctx := context.Background()
	var client *texttospeech.Client
	var edgeClient *tts.EdgeClient
	var routerPool *tts.RouterPool
	useCloudTTS := false
	switch engine {
	case "router":
		routerCfg, err := tts.RouterWorkersFromEnv()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		routerPool, err = tts.NewRouterPool(routerCfg.Workers, *voice)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("🎙️ ใช้ 9router (%d VMs, voice %s)\n", len(routerCfg.Workers), routerPool.Voice)
		for _, worker := range routerCfg.Workers {
			fmt.Printf("   - %s\n", worker.BaseURL)
		}
		if len(routerCfg.Skipped) > 0 {
			fmt.Printf("⚠️ ข้าม VM ที่คีย์ยังเป็น replace-me: %v\n", routerCfg.Skipped)
		}
	case "edge":
		edgeClient = &tts.EdgeClient{BaseURL: *edgeURL, Voice: *voice}
		fmt.Printf("🎙️ ใช้ Edge TTS (%s, voice %s)\n", edgeClient.BaseURL, edgeClient.Voice)
	case "translate":
		fmt.Println("🎙️ ใช้ Google Translate TTS")
	default:
		client, err = texttospeech.NewClient(ctx)
		if err != nil {
			if engine == "cloud" {
				fmt.Fprintf(os.Stderr, "ไม่สามารถเชื่อมต่อ Google Cloud TTS: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("⚠️ ไม่สามารถเชื่อมต่อ Google Cloud TTS, ใช้ Google Translate TTS แทน")
		} else {
			defer client.Close()
			useCloudTTS = true
			fmt.Println("✅ ใช้ Google Cloud TTS")
		}
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

	fmt.Printf("🎯 เตรียมประมวลผล %d งาน ด้วย %d workers\n", len(jobs), *workers)

	// สร้าง channels สำหรับการประสานงาน
	jobsChan := make(chan TTSJob, len(jobs))
	resultsChan := make(chan TTSResult, len(jobs))

	// เริ่มต้น workers
	var wg sync.WaitGroup
	for workerID := 1; workerID <= *workers; workerID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ttsWorker(id, jobsChan, resultsChan, client, ctx, useCloudTTS, edgeClient, routerPool, AUDIO_SPEED_MULTIPLIER, voiceFX)
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

// loadDotEnv loads KEY=VALUE pairs without overriding variables already set.
func loadDotEnv(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	for lineNum, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("%s:%d: invalid line", path, lineNum+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return fmt.Errorf("%s:%d: empty key", path, lineNum+1)
		}
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return fmt.Errorf("set %s: %w", key, err)
			}
		}
	}
	return nil
}
