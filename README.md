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
- **Edge TTS**: เรียกเซิร์ฟเวอร์ edge-tts ในเครื่อง แล้วประมวลผลหลายตอนพร้อมกัน
- **9router**: ยิง `/v1/audio/speech` ไปยัง VM 1–12 พร้อมกัน แต่ละเครื่องใช้ API key ของตัวเองใน `.env`

### 🎵 การปรับแต่งเสียงขั้นสูง
- ปรับความเร็วเสียง (ค่าเริ่มต้น: 1.75x)
- Voice FX สำหรับหนังสือเสียง (clear, bright, warm, deep, soft, broadcast, room, mellow)
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
├── cmd/fetch-chapters/  # ดึงตอนแปลไทยจาก novels DB เป็นไฟล์บท
├── cmd/combine-audio/   # รวมไฟล์เสียงใน output/ ทีละ N ไฟล์
├── go.mod               # Go module dependencies
├── chapters/            # โฟลเดอร์สำหรับไฟล์ข้อความต้นฉบับ (.txt)
├── output/              # โฟลเดอร์สำหรับไฟล์เสียงที่สร้างขึ้น (.mp3)
└── README.md           # คู่มือการใช้งาน
```

## 🚀 การติดตั้งและใช้งาน

### ดึงตอนแปลไทยจาก PostgreSQL (novels)

ระบุ `book_id` แล้วดึงเฉพาะตอนที่แปลไทยแล้ว (`status_translate = 'done'`) จากตาราง `chapters` เรียงตาม `chapter_index` แล้วเขียนเป็นไฟล์เช่น `chapters/0001.txt`, `chapters/0002.txt` (ชื่อไฟล์เป็นเลขตอน 1-based zero-padded):

```bash
คัดลอก `.env.example` เป็น `.env` แล้วใส่ connection string:

```bash
cp .env.example .env
# แก้ DATABASE_URL ใน .env
```

จากนั้นรันได้เลย (โปรแกรมโหลด `.env` อัตโนมัติ; ถ้ายังไม่มี `sslmode` จะใส่ `sslmode=disable` ให้):

```bash
go run ./cmd/fetch-chapters <book_id> [from to] [output_dir]
```

`from` / `to` เป็นเลขตอนแบบ 1-based (ตรงกับชื่อไฟล์ `0001.txt` …) — ถ้าไม่ระบุจะดึงทุกตอนที่แปลไทยแล้ว

ตัวอย่าง:

```bash
go run ./cmd/fetch-chapters 01a154d4-f65b-7fa4-b848-46cb3e9ed226
go run ./cmd/fetch-chapters 01a1489e-e0f5-7f18-8418-f8bbb9c0f9ba 1 100
go run ./cmd/fetch-chapters 01a1489e-e0f5-7f18-8418-f8bbb9c0f9ba 1 100 chapters
```

ถ้าไม่ส่ง `book_id` โปรแกรมจะถามทาง stdin  
ไฟล์ `.env` ถูก gitignore แล้ว — อย่า commit ความลับขึ้น git

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

เลือกเอนจินด้วย `-engine cloud`, `-engine translate`, `-engine edge`, `-engine router` หรือปล่อยว่าง (ใช้ Cloud ถ้ามี credentials ไม่เช่นนั้นใช้ Translate) จำนวนไฟล์ที่ทำพร้อมกันตั้งด้วย `-workers` (ค่าเริ่มต้น 4)

### ใช้ Edge TTS

เปิดเซิร์ฟเวอร์ edge-tts ก่อน แล้วให้ `TTS_CONCURRENCY` ไม่ต่ำกว่า `-workers` (ค่าเริ่มต้นทั้งคู่คือ 4) จากนั้นแปลงทุกไฟล์ใน `chapters/` พร้อมกัน:

```bash
# ในโปรเจกต์ edge-tts
TTS_CONCURRENCY=4 uvicorn server:app --host 127.0.0.1 --port 8000

# ในโปรเจกต์ k-tts
go run . -engine edge -workers 4
```

`-edge-url` ค่าเริ่มต้น `http://127.0.0.1:8000` และ `-voice` ค่าเริ่มต้น `th-TH-PremwadeeNeural` ความเร็ว 1.75x และ voice FX `clear` ยังถูกใส่หลังได้ไฟล์ MP3 แล้ว

### ใช้ 9router 12 VM

สร้าง API key บน dashboard ของแต่ละ VM แล้วใส่ใน `.env` แทน `replace-me` คีย์ของเครื่องหลักใช้กับ VM โดยตรงไม่ได้ แต่ละเครื่องต้องมีคีย์ของตัวเอง VM ที่ยังเป็น `replace-me` จะถูกข้าม

```bash
go run . -engine router -workers 4
```

แต่ละก้อนข้อความไปคนละ VM พร้อมกัน สูงสุด 12 คำขอค้าง โมเดลคือ `edge-tts/<voice>` ไฟล์เสียงใช้ของ 9router ตามเดิม แล้วเร่งความเร็วตาม `AUDIO_SPEED_MULTIPLIER` โดยไม่ใส่ voice FX หรือ loudness ถ้า 9router ล้มเหลว ทั้งตอนนั้นจะถอยไปใช้ Google Translate TTS แล้วเร่งเป็น 1.75x ด้วย `atempo` เอนจิน Google ยังปรับทั้งความเร็วและ voice FX ตามค่าใน `main.go`

### รวมไฟล์เสียงหลายตอนเป็นไฟล์เดียว

เรียง `output/*.mp3` ตามชื่อไฟล์ แล้วรวมทีละ `size` ไฟล์ด้วย ffmpeg (stream copy ไม่ re-encode) ชื่อไฟล์ผลลัพธ์เป็น `<ไฟล์แรก>-<ไฟล์สุดท้าย>.mp3`:

```bash
go run ./cmd/combine-audio [-speed N] [-fx name] [-loudness LUFS] [size] [input_dir] [output_dir]
```

ค่าเริ่มต้น: `size=10`, `input_dir=output`, `output_dir=<input_dir>/combined`

```bash
go run ./cmd/combine-audio 10
# output/0081.mp3 ... output/0100.mp3 ->
#   output/combined/0081-0090.mp3
#   output/combined/0091-0100.mp3

go run ./cmd/combine-audio 5 output albums
```

ถ้าจำนวนไฟล์หารไม่ลงตัว กลุ่มสุดท้ายจะมีไฟล์น้อยกว่า `size` (เช่น `0101-0103.mp3`)

### 🎙️ Voice FX สำหรับหนังสือเสียง

ใส่ `-fx <ชื่อ>` (ใช้คู่กับ `-speed` ได้ — ปรับความเร็วและ FX ใน encode รอบเดียว) ทุก preset ปรับความดังจบที่ระดับเดียวกัน (ค่าเริ่มต้น −9 LUFS = ดังสุด) สลับ preset แล้วเสียงไม่ดัง/เบากระโดด

ปรับความดังด้วย `-loudness` (ใช้คู่กับ `-fx` เท่านั้น): ค่ายิ่งใกล้ 0 ยิ่งดัง ใส่ได้ตั้งแต่ −24 ถึง −9 (ค่าเริ่มต้น −9 คือดังสุดแล้ว ถ้าตั้งเกินกว่านี้ limiter จะดันต่อไม่ขึ้น ใช้ flag นี้เพื่อลดความดังลง) peak ถูกจำกัดที่ −1 dB เสียงจึงไม่แตก

| ชื่อ | โทนเสียง |
| --- | --- |
| `clear` | ชัด ตัดเสียงอู้ เน้นย่านพูด ฟังรู้เรื่องแม้เปิดเร็ว |
| `bright` | สดใส มีประกาย แหลมที่สุดในชุด |
| `warm` | อบอุ่น นุ่ม มีเนื้อเสียงมากขึ้น |
| `deep` | ทุ้มลึก หนักแน่น เบสมากที่สุด |
| `soft` | นุ่มนวล ไม่บาดหู เหมาะฟังก่อนนอน |
| `broadcast` | แบบดีเจ/ผู้ประกาศข่าว บีบเสียงแน่น |
| `room` | มี reverb บางๆ ให้เสียง TTS ไม่แห้ง |
| `mellow` | ทุ้มนุ่ม ลดคีย์เสียงลง ~3.4 semitone (ความเร็วเท่าเดิม) ตัดแหลมเกิน 8kHz |

```bash
# รวมทีละ 10 ตอน + เสียงชัด (ดังสุด)
go run ./cmd/combine-audio -fx clear 10

# เร่ง 1.2x + เสียงอบอุ่น
go run ./cmd/combine-audio -speed 1.2 -fx warm 10

# เสียงนุ่ม + เบาลง
go run ./cmd/combine-audio -fx soft -loudness -14 10

# ใส่ FX ทีละไฟล์โดยไม่รวม (size=1) ไปไว้อีกโฟลเดอร์
go run ./cmd/combine-audio -fx bright 1 output output-bright
```

`main.go` ตั้ง `VOICE_FX = "clear"` ไว้เป็นค่าเริ่มต้น ไฟล์ที่สร้างใหม่จึงได้เสียงชัด + ดังสุดตั้งแต่ตอนสร้าง ตอนรวมไฟล์เหล่านี้ไม่ต้องใส่ `-fx` ซ้ำ (EQ จะทับกันสองรอบ) ใช้ `-fx` เฉพาะกับไฟล์เก่าที่สร้างก่อนมี FX

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
const AUDIO_SPEED_MULTIPLIER = 1.75 // ความเร็วเสียง (1.0 = ปกติ)
const VOICE_FX = "clear"            // voice FX: "", clear, bright, warm, deep, soft, broadcast, room, mellow
```

จำนวน workers ตั้งตอนรันด้วย `-workers` (ค่าเริ่มต้น 4)

### พารามิเตอร์ที่สามารถปรับได้
- **ความเร็วเสียง**: 0.25x - 4.0x
- **จำนวน Workers**: 1 ขึ้นไป (แนะนำ 2-6) ต้องไม่เกิน `TTS_CONCURRENCY` ของเซิร์ฟเวอร์ Edge
- **คุณภาพเสียง**: 320kbps MP3, 48kHz sampling rate
- **ขนาดการแบ่งข้อความ**: 150 ตัวอักษรต่อส่วนสำหรับ Google Translate, 120 สำหรับ Edge TTS

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
