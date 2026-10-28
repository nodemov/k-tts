package tts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var partFileRe = regexp.MustCompile(`temp_part_(\d+)\.mp3`)

// SortFilesNaturally เรียงลำดับไฟล์ temp_part_N.mp3 ตามหมายเลข (1, 2, ..., 10 แทนที่จะเป็น 1, 10, 2)
func SortFilesNaturally(files []string) {
	num := func(path string) int {
		m := partFileRe.FindStringSubmatch(filepath.Base(path))
		if len(m) < 2 {
			return 0
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}
	sort.Slice(files, func(i, j int) bool { return num(files[i]) < num(files[j]) })
}

// CombineAudioFiles รวมไฟล์ temp_part_*.mp3 ใน tempDir เป็น outputFile และคืนลำดับไฟล์ที่ใช้รวม
func CombineAudioFiles(tempDir, outputFile string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(tempDir, "temp_part_*.mp3"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("ไม่พบไฟล์เสียงใน %s", tempDir)
	}
	SortFilesNaturally(files)

	if len(files) == 1 {
		// หากมีไฟล์เดียว ให้คัดลอกไปยัง output
		data, err := os.ReadFile(files[0])
		if err != nil {
			return files, err
		}
		return files, os.WriteFile(outputFile, data, 0644)
	}

	// สร้างไฟล์รายการสำหรับ ffmpeg (ใช้ relative path จาก tempDir)
	filelistPath := filepath.Join(tempDir, "filelist.txt")
	var filelist strings.Builder
	for _, file := range files {
		fmt.Fprintf(&filelist, "file '%s'\n", filepath.Base(file))
	}
	if err := os.WriteFile(filelistPath, []byte(filelist.String()), 0644); err != nil {
		return files, err
	}
	defer os.Remove(filelistPath)

	absOutputFile, err := filepath.Abs(outputFile)
	if err != nil {
		return files, err
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
	if output, err := cmd.CombinedOutput(); err != nil {
		return files, fmt.Errorf("ffmpeg error: %v\nOutput: %s", err, string(output))
	}
	return files, nil
}

// ConcatFiles รวมไฟล์ mp3 ตามลำดับใน files เป็น outputFile แบบ stream copy (ไม่ re-encode)
func ConcatFiles(files []string, outputFile string) error {
	if len(files) == 0 {
		return fmt.Errorf("ไม่มีไฟล์ให้รวม")
	}

	var filelist strings.Builder
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			return err
		}
		fmt.Fprintf(&filelist, "file '%s'\n", strings.ReplaceAll(abs, "'", `'\''`))
	}

	listFile, err := os.CreateTemp("", "k-tts-concat-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(listFile.Name())
	if _, err := listFile.WriteString(filelist.String()); err != nil {
		listFile.Close()
		return err
	}
	if err := listFile.Close(); err != nil {
		return err
	}

	cmd := exec.Command("ffmpeg",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile.Name(),
		"-c", "copy",
		outputFile,
		"-y")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg concat error: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// AtempoFilter สร้าง filter chain ของ atempo (ffmpeg รองรับครั้งละไม่เกิน 2.0x จึงต้องแบ่งเป็นขั้น)
func AtempoFilter(speed float64) string {
	return atempoChain(speed) + ",dynaudnorm=p=0.9:s=5"
}

func atempoChain(speed float64) string {
	if speed <= 2.0 {
		return fmt.Sprintf("atempo=%.2f", speed)
	}
	// เช่น 2.4x = 1.5 * 1.6
	firstStep := 1.5
	secondStep := speed / firstStep
	if secondStep > 2.0 {
		// หากยังเกิน 2.0 ให้แบ่งเป็น 3 ขั้น
		firstStep = 1.4
		secondStep = 1.5
		thirdStep := speed / (firstStep * secondStep)
		return fmt.Sprintf("atempo=%.2f,atempo=%.2f,atempo=%.2f", firstStep, secondStep, thirdStep)
	}
	return fmt.Sprintf("atempo=%.2f,atempo=%.2f", firstStep, secondStep)
}

// AdjustSpeed ปรับความเร็วของไฟล์เสียงด้วย ffmpeg (inputFile และ outputFile เป็นไฟล์เดียวกันได้)
func AdjustSpeed(inputFile, outputFile string, speed float64) error {
	if err := reencode(inputFile, outputFile, AtempoFilter(speed)); err != nil {
		return fmt.Errorf("ffmpeg speed adjustment error: %w", err)
	}
	return nil
}

// SpeedOnlyFilter เป็น atempo อย่างเดียว ไม่มี dynaudnorm, voice FX หรือ loudness
func SpeedOnlyFilter(speed float64) string {
	return atempoChain(speed)
}

// SpeedUp เร่งความเร็วอย่างเดียวและคง sample rate กับจำนวนช่องเสียงเดิม
// speed 1 คืนไฟล์เดิมโดยไม่ encode ใหม่
func SpeedUp(inputFile, outputFile string, speed float64) error {
	if speed == 1 {
		if inputFile == outputFile {
			return nil
		}
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return err
		}
		return os.WriteFile(outputFile, data, 0644)
	}

	tempFile := inputFile + ".speedup.mp3"
	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-af", SpeedOnlyFilter(speed),
		"-c:a", "libmp3lame",
		"-q:a", "2",
		tempFile,
		"-y")
	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("ffmpeg speed up error: %v\nOutput: %s", err, string(output))
	}
	if err := os.Rename(tempFile, outputFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("ไม่สามารถแทนที่ไฟล์ได้: %v", err)
	}
	return nil
}

// ApplyVoiceFX ปรับความเร็ว ใส่ voice FX และปรับความดังเป็น lufs ใน encode รอบเดียว (inputFile และ outputFile เป็นไฟล์เดียวกันได้)
func ApplyVoiceFX(inputFile, outputFile string, speed float64, fx VoiceFX, lufs float64) error {
	if err := reencode(inputFile, outputFile, VoiceFXFilter(speed, fx, lufs)); err != nil {
		return fmt.Errorf("ffmpeg voice fx (%s) error: %w", fx.Name, err)
	}
	return nil
}

// reencode encode inputFile ผ่าน filter แล้วเขียนทับ outputFile ผ่านไฟล์ชั่วคราว
func reencode(inputFile, outputFile, filter string) error {
	tempFile := inputFile + ".temp.mp3"

	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-af", filter,
		"-c:a", "libmp3lame", // High-quality MP3 encoder
		"-b:a", "320k", // Maximum bitrate
		"-ar", "48000", // High sample rate
		"-ac", "2", // Stereo
		tempFile,
		"-y")

	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("%v\nOutput: %s", err, string(output))
	}

	// แทนที่ไฟล์เดิมด้วยไฟล์ที่ปรับความเร็วแล้ว
	if err := os.Rename(tempFile, outputFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("ไม่สามารถแทนที่ไฟล์ได้: %v", err)
	}
	return nil
}

// EnhanceAudioQuality post-processing เสียง TTS ให้คุณภาพสูงขึ้น
func EnhanceAudioQuality(inputFile, outputFile string) error {
	cmd := exec.Command("ffmpeg",
		"-i", inputFile,
		"-c:a", "libmp3lame",
		"-b:a", "320k",
		"-ar", "48000",
		"-ac", "2",
		"-af", "highpass=f=80,lowpass=f=15000,dynaudnorm=p=0.9:s=5:m=15,volume=1.1", // Filter chain สำหรับปรับปรุงเสียง
		outputFile,
		"-y")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg enhancement error: %v\nOutput: %s", err, string(output))
	}
	return nil
}
