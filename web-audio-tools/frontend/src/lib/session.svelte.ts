/** The "current clip" shared between tools, so audio can hop editor → enhance → fun. */
export const session = $state<{ buffer: AudioBuffer | null; name: string; version: number }>({
	buffer: null,
	name: '',
	version: 0
});

export function setSessionAudio(buffer: AudioBuffer, name: string) {
	session.buffer = buffer;
	session.name = name;
	session.version++;
}
