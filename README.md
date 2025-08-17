# K-TTS: Multi-Worker Text-to-Speech System

โปรแกรม Text-to-Speech แบบ Multi-Worker สำหรับการแปลงข้อความเป็นเสียงภาษาไทยคุณภาพสูง รองรับการประมวลผลหลายไฟล์พร้อมกัน

## ✨ คุณสมบัติหลัก

### 🚀 Multi-Worker Processing
- ประมวลผลหลายไฟล์พร้อมกัน (ค่าเริ่มต้น: 4 workers)
- ลดเวลาการประมวลผลอย่างมีนัยสำคัญ
- การจัดการ rate limiting อัตโนมัติ

### 🎤 รองรับ TTS Engine หลายประเภท
- **Google Cloud TTS** (แนะนำ): คุณภาพเสียงสูง, เสียงธรรมชาติ
- **Google Translate TTS** (สำรอง): ใช้งานได้ทันที, ไม่ต้องตั้งค่า

### 🎵 การปรับแต่งเสียงขั้นสูง
- ปรับความเร็วเสียง (ค่าเริ่มต้น: 1.6x)
- Audio enhancement ด้วย ffmpeg
- Dynamic range normalization
- High-quality MP3 encoding (320kbps, 48kHz)

### � การประมวลผลข้อความอัจฉริยะ
- ทำความสะอาดข้อความอัตโนมัติ
- แบ่งข้อความยาวตามจุดแบ่งที่เหมาะสม
- รองรับภาษาไทยเป็นพิเศษ (Thai-specific text segmentation)
- ลบเครื่องหมายพิเศษและหมายเลขบทที่ไม่ต้องการ

## 📁 โครงสร้างโปรเจค

```
k-tts/
├── main.go              # ไฟล์หลักของโปรแกรม
├── go.mod               # Go module dependencies
├── chapters/            # โฟลเดอร์สำหรับไฟล์ข้อความต้นฉบับ (.txt)
├── output/              # โฟลเดอร์สำหรับไฟล์เสียงที่สร้างขึ้น (.mp3)
└── README.md           # คู่มือการใช้งาน
```

## 🚀 การติดตั้งและใช้งาน

### ความต้องการระบบ
- Go 1.19 หรือสูงกว่า
- ffmpeg (สำหรับการรวมและปรับแต่งไฟล์เสียง)
- Google Cloud credentials (ถ้าใช้ Cloud TTS)

### การติดตั้ง ffmpeg
```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt update && sudo apt install ffmpeg

# Windows (chocolatey)
choco install ffmpeg
```

### การใช้งานพื้นฐาน
1. วางไฟล์ข้อความ (.txt) ในโฟลเดอร์ `chapters/`
2. รันโปรแกรม:
   ```bash
   go run main.go
   ```
3. ไฟล์เสียงจะถูกสร้างในโฟลเดอร์ `output/`

### ตัวอย่างการใช้งาน
```bash
# สร้างไฟล์ข้อความตัวอย่าง
mkdir -p chapters
echo "สวัสดีครับ นี่คือระบบ Text-to-Speech ขั้นสูง" > chapters/001.txt
echo "โปรแกรมนี้สามารถประมวลผลหลายไฟล์พร้อมกัน" > chapters/002.txt

# รันโปรแกรม
go run main.go

# ตรวจสอบผลลัพธ์
ls output/
# Output: 001.mp3  002.mp3
```

## ⚙️ การตั้งค่า Google Cloud TTS (แนะนำ)

### 1. ตั้งค่า Google Cloud Project
```bash
# ติดตั้ง Google Cloud CLI
brew install google-cloud-sdk  # macOS
# หรือ: https://cloud.google.com/sdk/docs/install

# เข้าสู่ระบบ
gcloud auth login

# สร้าง project ใหม่
gcloud projects create YOUR_PROJECT_ID

# ตั้งค่า project
gcloud config set project YOUR_PROJECT_ID

# เปิดใช้งาน billing
gcloud billing accounts list
gcloud billing projects link YOUR_PROJECT_ID --billing-account=BILLING_ACCOUNT_ID

# เปิดใช้งาน Text-to-Speech API
gcloud services enable texttospeech.googleapis.com
```

### 2. สร้าง Service Account
```bash
# สร้าง service account
gcloud iam service-accounts create k-tts-service \
    --display-name="K-TTS Service Account"

# ให้สิทธิ์ Cloud TTS
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
    --member="serviceAccount:k-tts-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/cloudtts.user"

# ดาวน์โหลด credentials
gcloud iam service-accounts keys create ~/k-tts-credentials.json \
    --iam-account=k-tts-service@YOUR_PROJECT_ID.iam.gserviceaccount.com

# ตั้งค่า environment variable
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/k-tts-credentials.json"
```

### 3. ทดสอบการเชื่อมต่อ
```bash
# ทดสอบ Google Cloud TTS
gcloud alpha text-to-speech speak --text="สวัสดี" --language-code=th-TH
```

## 🔧 การปรับแต่งโปรแกรม

### การแก้ไขค่าคงที่สำคัญ
```go
// ในไฟล์ main.go
const AUDIO_SPEED_MULTIPLIER = 1.6  // ความเร็วเสียง (1.0 = ปกติ)
const NUM_WORKERS = 4               // จำนวน workers
```

### พารามิเตอร์ที่สามารถปรับได้
- **ความเร็วเสียง**: 0.25x - 4.0x
- **จำนวน Workers**: 1-10 (แนะนำ 2-6)
- **คุณภาพเสียง**: 320kbps MP3, 48kHz sampling rate
- **ขนาดการแบ่งข้อความ**: 150 ตัวอักษรต่อส่วน

## 📊 คุณสมบัติเทคนิค

### Audio Processing Pipeline
1. **Text Cleaning**: ลบอักขระพิเศษและหมายเลขบท
2. **Smart Text Splitting**: แบ่งข้อความตามจุดแบ่งที่เหมาะสม
3. **TTS Generation**: สร้างเสียงด้วย Google TTS
4. **Audio Combination**: รวมไฟล์เสียงด้วย ffmpeg
5. **Speed Adjustment**: ปรับความเร็วด้วย atempo filter
6. **Audio Enhancement**: ปรับปรุงคุณภาพเสียง

### ข้อกำหนดไฟล์เสียง
- **Format**: MP3
- **Quality**: 320kbps
- **Sample Rate**: 48kHz
- **Channels**: Stereo
- **Enhancement**: Dynamic normalization, volume boost

### Thai Language Optimization
- Thai vowel และ tone marker detection
- Smart sentence breaking สำหรับภาษาไทย
- Thai-specific text cleaning rules

## 🎛️ Audio Enhancement Features

### ffmpeg Filters ที่ใช้
```bash
# สำหรับการปรับความเร็ว
-af "atempo=1.6,dynaudnorm=p=0.9:s=5"

# สำหรับการปรับปรุงคุณภาพ
-af "highpass=f=80,lowpass=f=15000,dynaudnorm=p=0.9:s=5:m=15,volume=1.1"

# สำหรับการรวมไฟล์
-c:a libmp3lame -b:a 320k -ar 48000 -ac 2
```

### การจัดการไฟล์ขนาดใหญ่
- แบ่งข้อความอัตโนมัติ
- ประมวลผลแบบ chunk
- การรวมไฟล์แบบ lossless
- Natural sorting สำหรับลำดับไฟล์

## 🚨 การแก้ไขปัญหา

### ปัญหาทั่วไป

**1. ไม่สามารถเชื่อมต่อ Google Cloud TTS**
```
⚠️ ไม่สามารถเชื่อมต่อ Google Cloud TTS, ใช้ Google Translate TTS แทน
```
- ตรวจสอบ `GOOGLE_APPLICATION_CREDENTIALS`
- ตรวจสอบสิทธิ์ service account
- ตรวจสอบการเปิดใช้งาน API

**2. ffmpeg ไม่พบ**
```
ffmpeg error: executable file not found
```
- ติดตั้ง ffmpeg ตามขั้นตอนด้านบน
- ตรวจสอบ PATH environment

**3. Rate Limiting**
```
⚠️ Worker: ได้รับ status code 429
```
- ลดจำนวน workers
- เพิ่มเวลารอระหว่างการดาวน์โหลด

**4. ไฟล์ข้อความว่างเปล่า**
```
⚠️ ไฟล์ xxx.txt ว่างเปล่า
```
- ตรวจสอบการ encoding ไฟล์ (ควรเป็น UTF-8)
- ตรวจสอบเนื้อหาไฟล์

## 📈 Performance Tips

### การปรับแต่งประสิทธิภาพ
1. **เพิ่มจำนวน Workers**: เหมาะสำหรับไฟล์จำนวนมาก
2. **ลดขนาดการแบ่งข้อความ**: สำหรับข้อความซับซ้อน
3. **ใช้ SSD**: เพื่อการเขียนไฟล์ที่เร็วขึ้น
4. **RAM เพียงพอ**: อย่างน้อย 4GB สำหรับการประมวลผลหลายไฟล์

### การเลือกใช้ TTS Engine
- **Google Cloud TTS**: สำหรับงานที่ต้องการคุณภาพสูง
- **Google Translate TTS**: สำหรับการทดสอบหรืองานไม่สำคัญ

## 📋 ข้อมูลเพิ่มเติม

### Dependencies
```go
// ใน go.mod
cloud.google.com/go/texttospeech v1.7.4
```

### License
ใช้งานได้อย่างอิสระ กรุณาปฏิบัติตาม Google Cloud TTS และ Google Translate TOS

### การพัฒนาต่อ
- เพิ่มรองรับภาษาอื่นๆ
- GUI สำหรับการตั้งค่า
- Batch processing แบบ advanced
- Real-time TTS streaming
