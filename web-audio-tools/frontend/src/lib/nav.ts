export interface NavItem {
	href: string;
	code: string;
	label: string;
	th: string;
	blurb: string;
	accent: string;
}

export const NAV: NavItem[] = [
	{ href: '/editor', code: '01', label: 'Editor', th: 'ตัดต่อเสียง', blurb: 'waveform + spectrogram, ตัด/คัดลอก/วาง, fade, undo', accent: 'var(--color-signal)' },
	{ href: '/enhance', code: '02', label: 'Enhance', th: 'ปรับเสียงให้ดีขึ้น', blurb: 'One-click: ลดนอยส์ EQ คอมเพรสเซอร์ LUFS', accent: 'var(--color-phosphor)' },
	{ href: '/fun', code: '03', label: 'Voice FX', th: 'เปลี่ยนเสียงสนุก ๆ', blurb: 'ชิพมังก์ หุ่นยนต์ ปีศาจ 8D อัดไมค์สด', accent: '#ff5cc8' },
	{ href: '/mixer', code: '04', label: 'Mixer', th: 'มิกซ์หลายแทร็ก', blurb: 'วางหลายไฟล์บน timeline แล้ว mixdown', accent: 'var(--color-cobalt)' },
	{ href: '/soundboard', code: '05', label: 'Soundboard', th: 'ปุ่มเสียงเอฟเฟกต์', blurb: 'pad เสียงมีม + เพิ่มเสียงของคุณเอง', accent: '#ffd23f' },
	{ href: '/visualizer', code: '06', label: 'Visualizer', th: 'ภาพจากเสียง', blurb: 'visual แบบ realtime แล้วอัดเป็นวิดีโอ', accent: '#9d7bff' },
	{ href: '/ai', code: '07', label: 'AI Lab', th: 'AI แยกเสียง ถอดความ TTS', blurb: 'แยกเสียงร้อง, ถอดเสียงเป็น SRT, หนังสือเสียงไทย', accent: 'var(--color-cobalt)' },
	{ href: '/batch', code: '08', label: 'Batch', th: 'ประมวลผลหลายไฟล์', blurb: 'ใช้ preset เดียวกับไฟล์ทั้งโฟลเดอร์ ได้ ZIP', accent: 'var(--color-cream)' }
];
