# วิธีใช้ `scripts/atempo-combine.sh`

รวมไฟล์ `output/NNNN.mp3` ตามความยาวที่กำหนด แล้วเร่งด้วย ffmpeg `atempo` โดยคงระดับเสียง (pitch) เดิม ไฟล์ต้นฉบับไม่ถูกเขียนทับ

ผลลัพธ์อยู่ที่ `output/combined/<ไฟล์แรก>-<ไฟล์สุดท้าย>.<atempo>x.mp3`

ต้องมี `ffmpeg` อยู่ใน `PATH` ไฟล์ต้นทางต้องชื่อเป็นตัวเลข เช่น `output/0152.mp3` เลขศูนย์นำหน้าไม่ถูกนับ (`152` กับ `0152` คือตอนเดียวกัน)

## รวมหนึ่งช่วง

```bash
scripts/atempo-combine.sh --from 0152 --to 0201
```

ได้ `output/combined/0152-0201.1.1x.mp3` ความเร็วเริ่มต้นคือ `atempo=1.1`

## กำหนดความเร็ว

```bash
scripts/atempo-combine.sh --from 0152 --to 0201 atempo=1.25
scripts/atempo-combine.sh --size 50 --atempo 1.5
scripts/atempo-combine.sh --from 0152 --to 0201 --speed 0.9
```

ใส่ค่าได้สามแบบ: `atempo=1.25`, `--atempo 1.25`, `--speed 1.25`

ค่าต่ำสุดคือ `0.5` ffmpeg รับ `atempo` ได้ครั้งละไม่เกิน `2.0` ถ้าใส่สูงกว่านั้นสคริปต์แบ่งเป็นหลายขั้นเอง เช่น `2.4` เป็น `atempo=2.00,atempo=1.20`

## กำหนดความยาว

`--from` กับ `--to` เลือกช่วงตอน แล้วรวมเป็นไฟล์เดียวเมื่อไม่ได้ใส่ `--size`

`--size` คือจำนวนไฟล์ต่อก้อน เรียงตามชื่อไฟล์

```bash
scripts/atempo-combine.sh --size 50
```

จากไฟล์ `output/0051.mp3` ถึง `output/0201.mp3` ได้สามก้อน

- `output/combined/0051-0100.1.1x.mp3`
- `output/combined/0101-0151.1.1x.mp3`
- `output/combined/0152-0201.1.1x.mp3`

เลือกช่วงแล้วค่อยแบ่งได้

```bash
scripts/atempo-combine.sh --from 0101 --to 0201 --size 50 atempo=1.1
```

ต้องใส่ `--from`/`--to` หรือ `--size` อย่างน้อยหนึ่งอย่าง `--from` ใช้คู่กับ `--to` เท่านั้น

## ดูก่อน encode

```bash
scripts/atempo-combine.sh --size 50 atempo=1.25 --dry-run
```

พิมพ์ชื่อไฟล์ที่จะได้และจำนวนไฟล์ในแต่ละก้อน โดยยังไม่เรียก ffmpeg

## ทำไฟล์ที่มีอยู่แล้วใหม่

ถ้าไฟล์ผลลัพธ์มีอยู่แล้ว สคริปต์จะข้ามก้อนนั้น ใส่ `--force` เพื่อ encode ทับ

```bash
scripts/atempo-combine.sh --from 0152 --to 0201 atempo=1.1 --force
```

## ตัวเลือก

| ตัวเลือก | ความหมาย | ค่าเริ่มต้น |
| --- | --- | --- |
| `--from N` | ตอนเริ่ม (ใช้คู่กับ `--to`) | ไม่กรอง |
| `--to N` | ตอนจบ | ไม่กรอง |
| `--size N`, `-n` | จำนวนไฟล์ต่อก้อน | ทั้งช่วงที่เลือก |
| `atempo=X`, `--atempo X`, `--speed X`, `-s`, `-a` | ความเร็ว | `1.1` |
| `--input DIR`, `-i` | โฟลเดอร์ไฟล์ต้นทาง | `output` |
| `--output DIR`, `-o` | โฟลเดอร์ผลลัพธ์ | `output/combined` |
| `--force`, `-f` | encode ทับไฟล์ที่มีอยู่ | ข้าม |
| `--dry-run` | แสดงแผนอย่างเดียว | encode จริง |
| `-h`, `--help` | แสดงวิธีใช้สั้นๆ | |

## สิ่งที่สคริปต์ทำ

อ่านเฉพาะ `*.mp3` ที่อยู่ชั้นบนสุดของโฟลเดอร์ต้นทาง ชื่อไฟล์ต้องเป็นตัวเลข ไฟล์ `*.temp.mp3`, `*.speed.mp3`, `*.speedup.mp3` ถูกข้าม

แต่ละก้อนถูกรวมแล้วเร่งใน ffmpeg รอบเดียว ด้วย `libmp3lame`, `86 kbps`, `48000 Hz`, สเตอริโอ `atempo` เร่งเวลาโดยไม่เปลี่ยนระดับเสียง
