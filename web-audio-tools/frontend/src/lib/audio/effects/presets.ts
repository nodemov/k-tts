import { withSettings, type EffectSettings, type SettingsPatch } from './settings';

export interface Preset {
	id: string;
	name: string;
	emoji?: string;
	description: string;
	patch: SettingsPatch;
}

export const ENHANCE_PRESETS: Preset[] = [
	{
		id: 'voice',
		name: 'One-click Voice',
		description: 'ลดเสียงรบกวน ตัดเสียงก้อง ปรับความชัด และความดังมาตรฐาน podcast',
		patch: {
			denoise: 0.55,
			highpass: 80,
			gate: -55,
			deesser: 0.45,
			eq: [0, 0, -1, -2.5, -1, 0, 1.5, 3, 2, 2.5],
			compressor: { enabled: true, threshold: -22, ratio: 3, attack: 0.008, release: 0.18, makeup: 3 },
			limiter: -1,
			loudness: -16,
			trimSilence: true
		}
	},
	{
		id: 'podcast',
		name: 'Podcast Studio',
		description: 'เสียงอุ่น หนักแน่นแบบไมค์ broadcast และตัดช่วงเงียบยาว',
		patch: {
			denoise: 0.4,
			highpass: 70,
			gate: -52,
			deesser: 0.5,
			eq: [0, 1.5, 2, -2, -1.5, 0, 1, 2.5, 1.5, 1],
			compressor: { enabled: true, threshold: -24, ratio: 4, attack: 0.005, release: 0.25, makeup: 5 },
			limiter: -1,
			loudness: -16,
			trimSilence: true
		}
	},
	{
		id: 'music',
		name: 'Music Master',
		description: 'mastering เบา ๆ เพิ่มความอิ่มและดังระดับ streaming −14 LUFS',
		patch: {
			eq: [1.5, 1, 0, -0.5, -0.5, 0, 0.5, 1, 1.5, 2],
			compressor: { enabled: true, threshold: -16, ratio: 2, attack: 0.02, release: 0.3, makeup: 2 },
			limiter: -1,
			loudness: -14
		}
	},
	{
		id: 'phone-rescue',
		name: 'Phone Recording Rescue',
		description: 'กู้เสียงอัดจากมือถือ: ลดเสียงลม/ซ่า เพิ่มย่านกลาง',
		patch: {
			denoise: 0.75,
			highpass: 120,
			lowpass: 12000,
			gate: -50,
			deesser: 0.3,
			eq: [0, 0, 1, 1, 0.5, 1, 2, 2.5, 0, -2],
			compressor: { enabled: true, threshold: -20, ratio: 3.5, attack: 0.01, release: 0.2, makeup: 4 },
			limiter: -1,
			loudness: -16
		}
	},
	{
		id: 'audiobook',
		name: 'Audiobook (TTS)',
		description: 'เหมาะกับเสียง TTS / หนังสือเสียง: นุ่ม ไม่บาดหู ดังสม่ำเสมอ',
		patch: {
			highpass: 60,
			deesser: 0.6,
			eq: [0, 1, 1.5, 0, -1, -0.5, 0, 0.5, -1, -2],
			compressor: { enabled: true, threshold: -20, ratio: 2.5, attack: 0.01, release: 0.25, makeup: 2 },
			limiter: -1.5,
			loudness: -18,
			trimSilence: true
		}
	}
];

export const FUN_PRESETS: Preset[] = [
	{ id: 'chipmunk', name: 'ชิพมังก์', emoji: '🐿️', description: 'เสียงแหลมเล็ก', patch: { pitch: 9, rate: 1.1 } },
	{
		id: 'robot',
		name: 'หุ่นยนต์',
		emoji: '🤖',
		description: 'ring modulation + โลหะ',
		patch: { ringmod: { freq: 55, mix: 0.85 }, echo: { time: 0.012, feedback: 0.55, mix: 0.4 }, highpass: 150 }
	},
	{
		id: 'demon',
		name: 'ปีศาจ',
		emoji: '😈',
		description: 'ต่ำ หนัก ก้องนรก',
		patch: { pitch: -8, drive: 0.35, reverb: { decay: 3.5, mix: 0.35 }, eq: [4, 4, 3, 0, 0, 0, 0, 0, -3, -6] }
	},
	{
		id: 'radio',
		name: 'วิทยุเก่า',
		emoji: '📻',
		description: 'AM radio ปี 1940',
		patch: { highpass: 500, lowpass: 3200, drive: 0.45, compressor: { enabled: true, threshold: -25, ratio: 6, attack: 0.003, release: 0.1, makeup: 6 } }
	},
	{
		id: 'telephone',
		name: 'โทรศัพท์',
		emoji: '📞',
		description: 'สายโทรศัพท์บ้าน',
		patch: { highpass: 300, lowpass: 3400, drive: 0.2, eq: [0, 0, 0, 0, 0, 4, 3, 0, 0, 0] }
	},
	{
		id: 'cave',
		name: 'ในถ้ำ',
		emoji: '🕳️',
		description: 'เสียงสะท้อนยาว',
		patch: { reverb: { decay: 5, mix: 0.55 }, echo: { time: 0.32, feedback: 0.4, mix: 0.3 }, lowpass: 7000 }
	},
	{
		id: 'alien',
		name: 'เอเลี่ยน',
		emoji: '👽',
		description: 'เสียงต่างดาวสั่นไหว',
		patch: { pitch: 4, ringmod: { freq: 22, mix: 0.6 }, echo: { time: 0.08, feedback: 0.5, mix: 0.35 } }
	},
	{
		id: 'stadium',
		name: 'สนามกีฬา',
		emoji: '🏟️',
		description: 'ประกาศในสนาม',
		patch: { highpass: 200, reverb: { decay: 3, mix: 0.4 }, echo: { time: 0.45, feedback: 0.3, mix: 0.25 }, drive: 0.15 }
	},
	{
		id: 'underwater',
		name: 'ใต้น้ำ',
		emoji: '🫧',
		description: 'อู้อี้เหมือนดำน้ำ',
		patch: { lowpass: 500, pitch: -2, ringmod: { freq: 6, mix: 0.25 }, reverb: { decay: 1.5, mix: 0.3 } }
	},
	{
		id: 'megaphone',
		name: 'โทรโข่ง',
		emoji: '📢',
		description: 'ประท้วง/ขายของ',
		patch: { highpass: 700, lowpass: 4000, drive: 0.7, eq: [0, 0, 0, 0, 0, 3, 6, 2, 0, 0] }
	},
	{ id: '8d', name: '8D Audio', emoji: '🎧', description: 'เสียงหมุนรอบหัว (ใส่หูฟัง)', patch: { eightD: 0.12, reverb: { decay: 2.2, mix: 0.25 } } },
	{ id: 'nightcore', name: 'Nightcore', emoji: '⚡', description: 'เร็วขึ้น เสียงสูงขึ้น', patch: { rate: 1.25, pitch: 3.9 } },
	{
		id: 'vaporwave',
		name: 'Vaporwave',
		emoji: '🌴',
		description: 'ช้า ฝัน ย้อนยุค',
		patch: { rate: 0.8, pitch: -3.9, reverb: { decay: 3, mix: 0.3 }, lowpass: 9000 }
	},
	{ id: 'slowmo', name: 'Slow-mo', emoji: '🐢', description: 'ช้าลงครึ่งหนึ่ง เสียงเดิม', patch: { rate: 0.5 } },
	{ id: 'reverse', name: 'ย้อนกลับ', emoji: '🔁', description: 'เล่นถอยหลัง', patch: { reverse: true, reverb: { decay: 1.5, mix: 0.2 } } }
];

export function presetSettings(preset: Preset): EffectSettings {
	return withSettings(preset.patch);
}
