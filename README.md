# การใช้งาน Google TTS แบบ Offline

โปรแกรมนี้รองรับ 2 วิธีในการใช้ Google TTS:

## ✨ คุณสมบัติ
- รองรับข้อความภาษาไทยได้ดี
- แบ่งข้อความยาวอัตโนมัติสำหรับ Google Translate TTS
- ใช้งานได้ทันทีโดยไม่ต้องตั้งค่า credentials (fallback mode)
- สามารถอัพเกรดเป็น Google Cloud TTS ได้ในภายหลัง

## 🚀 วิธีใช้งาน
1. ใส่ข้อความที่ต้องการในไฟล์ `1.txt`
2. รันคำสั่ง: `go run main.go`
3. ไฟล์เสียงจะถูกสร้างที่ `output_gtranslate.mp3` หรือ `output_google.mp3`

## วิธีที่ 1: Google Cloud Text-to-Speech API (แนะนำ)
**ข้อดี:** คุณภาพเสียงสูง, เสียงธรรมชาติ, รองรับหลายภาษา, ไม่จำกัดความยาว
**ข้อเสีย:** ต้องตั้งค่า Google Cloud credentials

### การตั้งค่า:
1. ไปที่ [Google Cloud Console](https://console.cloud.google.com/)
2. สร้าง project ใหม่หรือเลือก project ที่มีอยู่
3. เปิดใช้งาน Text-to-Speech API
4. สร้าง Service Account และดาวน์โหลด credentials JSON
5. ตั้งค่า environment variable:
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS="path/to/your/credentials.json"
   ```

## วิธีที่ 2: Google Translate TTS (วิธีสำรอง - ใช้อยู่ในปัจจุบัน)
**ข้อดี:** ไม่ต้องตั้งค่าอะไร, ใช้งานได้ทันที, แบ่งข้อความยาวอัตโนมัติ
**ข้อเสีย:** คุณภาพเสียงต่ำกว่า, อาจไม่เสถียร, จำกัดความยาวต่อครั้ง

## 📁 ไฟล์เอาต์พุต:
- `output_google.mp3` - จาก Google Cloud TTS (คุณภาพสูง)
- `output_gtranslate.mp3` - จาก Google Translate TTS (คุณภาพปกติ)

## 📝 ตัวอย่างการใช้งาน:
```bash
# แก้ไขไฟล์ข้อความ
echo "สวัสดีครับ ยินดีต้อนรับสู่ระบบ TTS" > 1.txt

# รันโปรแกรม
go run main.go

# เล่นไฟล์เสียง (macOS)
afplay output_gtranslate.mp3
```

## 🛠️ ตัวอย่างการตั้งค่า Google Cloud (macOS):
```bash
# ติดตั้ง Google Cloud CLI
brew install google-cloud-sdk

# เข้าสู่ระบบ
gcloud auth login

# ตั้งค่า project
gcloud config set project YOUR_PROJECT_ID

# เปิดใช้งาน API
gcloud services enable texttospeech.googleapis.com

# สร้าง service account
gcloud iam service-accounts create tts-service

# ให้สิทธิ์
gcloud projects add-iam-policy-binding YOUR_PROJECT_ID \
  --member="serviceAccount:tts-service@YOUR_PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/cloudtts.user"

# ดาวน์โหลด key
gcloud iam service-accounts keys create ~/tts-credentials.json \
  --iam-account=tts-service@YOUR_PROJECT_ID.iam.gserviceaccount.com

# ตั้งค่า environment
export GOOGLE_APPLICATION_CREDENTIALS="$HOME/tts-credentials.json"
```

## 📊 ข้อมูลเทคนิค:
- รองรับเสียงไทยคุณภาพสูง (th-TH-Neural2-C)
- ปรับความเร็วได้ (0.25-4.0x)
- ปรับ pitch ได้ (-20 ถึง +20)
- ปรับระดับเสียงได้ (-96 ถึง +16 dB)
- แบ่งข้อความยาวอัตโนมัติ (200 ตัวอักษรต่อส่วน สำหรับ Google Translate)
