package tts

import (
	"fmt"
	"strings"
)

// VoiceFX คือ preset ปรับโทนเสียงสำหรับหนังสือเสียง (ffmpeg audio filter chain)
type VoiceFX struct {
	Name        string
	Description string
	Filter      string
}

// ทุก preset จบที่ loudness เดียวกัน (peak -1 dBTP) เพื่อให้สลับ preset แล้วความดังไม่กระโดด
// สูงกว่า MaxLoudness แล้ว loudnorm ดันต่อไม่ขึ้น (limiter ตัน)
const (
	DefaultLoudness = MaxLoudness
	MinLoudness     = -24.0
	MaxLoudness     = -9.0
)

func loudnessStage(lufs float64) string {
	return fmt.Sprintf("loudnorm=I=%g:TP=-1:LRA=11", lufs)
}

// VoiceFXPresets รายการ voice FX ที่รองรับ เรียงจากใช้บ่อยไปน้อย
var VoiceFXPresets = []VoiceFX{
	{
		Name:        "clear",
		Description: "ชัดเจน ตัดเสียงอู้ เน้นย่านพูด ฟังรู้เรื่องแม้เปิดเร็ว",
		Filter: "highpass=f=80," +
			"deesser=i=0.3," +
			"equalizer=f=300:t=q:w=1:g=-3," +
			"equalizer=f=3000:t=q:w=1.2:g=3," +
			"acompressor=threshold=-20dB:ratio=3:attack=10:release=200",
	},
	{
		Name:        "bright",
		Description: "สดใส มีประกาย เสียงเปิดโล่ง เหมาะกับนิยายแนวสดใส/วัยรุ่น",
		Filter: "highpass=f=90," +
			"deesser=i=0.3," +
			"equalizer=f=250:t=q:w=1:g=-2," +
			"equalizer=f=4000:t=q:w=1:g=2," +
			"treble=g=5:f=6000," +
			"acompressor=threshold=-20dB:ratio=2.5:attack=10:release=200",
	},
	{
		Name:        "warm",
		Description: "อบอุ่น นุ่มหู เพิ่มเนื้อเสียงย่านต่ำ ลดความแหลม",
		Filter: "highpass=f=60," +
			"deesser=i=0.4," +
			"bass=g=4:f=220," +
			"equalizer=f=3500:t=q:w=1:g=-1.5," +
			"treble=g=-3:f=7000," +
			"acompressor=threshold=-22dB:ratio=2.5:attack=15:release=250",
	},
	{
		Name:        "deep",
		Description: "ทุ้มลึก หนักแน่น แบบผู้บรรยายสารคดี/นิยายดาร์ก",
		Filter: "highpass=f=50," +
			"bass=g=6:f=220," +
			"equalizer=f=500:t=q:w=1:g=-2," +
			"equalizer=f=2500:t=q:w=1:g=1," +
			"treble=g=-4:f=7000," +
			"acompressor=threshold=-22dB:ratio=4:attack=5:release=250",
	},
	{
		Name:        "soft",
		Description: "นุ่มนวล ไม่บาดหู เหมาะฟังก่อนนอน/ฟังยาวๆ",
		Filter: "highpass=f=70," +
			"lowpass=f=9000," +
			"treble=g=-4:f=5000," +
			"deesser=i=0.6," +
			"acompressor=threshold=-24dB:ratio=2:attack=20:release=300",
	},
	{
		Name:        "broadcast",
		Description: "แบบดีเจ/ผู้ประกาศข่าว บีบเสียงแน่น ดังสม่ำเสมอ เด่นชัด",
		Filter: "highpass=f=80," +
			"deesser=i=0.3," +
			"bass=g=2:f=180," +
			"equalizer=f=400:t=q:w=1:g=-3," +
			"equalizer=f=3000:t=q:w=1:g=3," +
			"acompressor=threshold=-24dB:ratio=5:attack=5:release=150",
	},
	{
		Name:        "room",
		Description: "มีมิติแบบห้องเล็ก (reverb บางๆ) ลดความแห้งของเสียง TTS",
		Filter: "highpass=f=70," +
			"deesser=i=0.3," +
			"equalizer=f=3000:t=q:w=1:g=1.5," +
			"acompressor=threshold=-22dB:ratio=2.5:attack=10:release=200," +
			"aecho=0.9:0.8:35|60:0.18|0.1",
	},
	{
		Name:        "mellow",
		Description: "ทุ้มนุ่ม ลดคีย์เสียงลง ~3.4 semitone (ความเร็วเท่าเดิม) เน้นเนื้อเสียงต่ำ ตัดแหลมเกิน 8kHz",
		// asetrate ลด pitch พร้อมยืดเวลา แล้ว atempo (1/0.82) ดึงความเร็วกลับ ต้อง aresample ก่อนเพราะ asetrate อิง sample rate ของ input
		Filter: "aresample=48000,asetrate=39360,aresample=48000,atempo=1.2195," +
			"firequalizer=gain_entry='" +
			"entry(100,7);entry(126,4);entry(159,2.5);entry(200,2.5);entry(252,2.3);entry(317,2);" +
			"entry(400,1.5);entry(504,0.7);entry(635,-0.4);entry(800,-1);entry(1008,-1);entry(1270,-0.1);" +
			"entry(1600,2);entry(2016,2.5);entry(2540,2.3);entry(3200,4.5);entry(4032,4.8);entry(5080,1.6);" +
			"entry(6400,-1.7);entry(8063,-12);entry(9000,-24);entry(10159,-40);entry(12800,-40)'",
	},
}

// LookupVoiceFX หา preset ตามชื่อ ("" หรือ "none" = ไม่ใส่ FX และคืน VoiceFX ว่าง)
func LookupVoiceFX(name string) (VoiceFX, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "none" {
		return VoiceFX{}, nil
	}
	for _, fx := range VoiceFXPresets {
		if fx.Name == name {
			return fx, nil
		}
	}
	return VoiceFX{}, fmt.Errorf("unknown voice fx %q (available: %s)", name, strings.Join(VoiceFXNames(), ", "))
}

// VoiceFXNames คืนชื่อ preset ทั้งหมดรวม "none"
func VoiceFXNames() []string {
	names := []string{"none"}
	for _, fx := range VoiceFXPresets {
		names = append(names, fx.Name)
	}
	return names
}

// VoiceFXFilter รวม atempo, FX และ loudness (LUFS) เป็น filter chain เดียว (fx ว่าง = ปรับความเร็วอย่างเดียวแบบเดิม)
func VoiceFXFilter(speed float64, fx VoiceFX, lufs float64) string {
	if fx.Filter == "" {
		return AtempoFilter(speed)
	}
	var stages []string
	if speed != 1 {
		stages = append(stages, atempoChain(speed))
	}
	return strings.Join(append(stages, fx.Filter, loudnessStage(lufs)), ",")
}
