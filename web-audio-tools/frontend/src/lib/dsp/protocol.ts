export type DspRequest =
	| { id: number; op: 'denoise'; channels: Float32Array[]; sampleRate: number; strength: number }
	| {
			id: number;
			op: 'stretch';
			channels: Float32Array[];
			sampleRate: number;
			rate: number;
			semitones: number;
	  };

export type DspResponse =
	| { id: number; channels: Float32Array<ArrayBuffer>[]; error?: undefined }
	| { id: number; error: string; channels?: undefined };
