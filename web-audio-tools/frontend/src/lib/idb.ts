/** Minimal IndexedDB key/value store for user soundboard pads. */

const DB = 'web-audio-tools';
const STORE = 'pads';

export interface StoredPad {
	id: string;
	name: string;
	color: string;
	data: ArrayBuffer;
	type: string;
}

function open(): Promise<IDBDatabase> {
	return new Promise((resolve, reject) => {
		const req = indexedDB.open(DB, 1);
		req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: 'id' });
		req.onsuccess = () => resolve(req.result);
		req.onerror = () => reject(req.error);
	});
}

async function tx<T>(mode: IDBTransactionMode, fn: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
	const db = await open();
	return new Promise((resolve, reject) => {
		const req = fn(db.transaction(STORE, mode).objectStore(STORE));
		req.onsuccess = () => resolve(req.result);
		req.onerror = () => reject(req.error);
	});
}

export const listPads = () => tx<StoredPad[]>('readonly', (s) => s.getAll() as IDBRequest<StoredPad[]>);
export const putPad = (pad: StoredPad) => tx('readwrite', (s) => s.put(pad));
export const deletePad = (id: string) => tx('readwrite', (s) => s.delete(id));
