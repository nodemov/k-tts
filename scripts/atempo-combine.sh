#!/usr/bin/env bash
# รวม mp3 ตามความยาวที่กำหนด แล้วเร่งด้วย ffmpeg atempo โดยคงระดับเสียง (pitch) เดิม
#
# ผลลัพธ์ไปที่ output/combined/<แรก>-<สุดท้าย>.<speed>x.mp3
#
# ตัวอย่าง:
#   scripts/atempo-combine.sh --from 0152 --to 0201
#   scripts/atempo-combine.sh --size 50
#   scripts/atempo-combine.sh --from 0152 --to 0201 atempo=1.25
#   scripts/atempo-combine.sh --size 50 --atempo 1.5
#
# --from/--to เลือกเฉพาะไฟล์ในช่วงนั้น (เทียบเลข ไม่สนเลขศูนย์นำหน้า)
# --size คือจำนวนไฟล์ต่อก้อน ถ้าไม่ใส่และมี --from/--to จะรวมเป็นไฟล์เดียว

set -euo pipefail

SIZE=""
SPEED="1.1"
FROM=""
TO=""
INPUT="output"
OUTPUT="output/combined"
FORCE=0
DRY_RUN=0

usage() {
	cat <<'EOF'
usage: scripts/atempo-combine.sh [--from N] [--to N] [--size N] [atempo=1.1]
                                 [--input output] [--output output/combined]
                                 [--force] [--dry-run]

กำหนดความยาวแล้วเร่ง mp3 ด้วย atempo (ค่าเริ่มต้น 1.1, เปลี่ยนได้เช่น atempo=1.25)
ระดับเสียง (pitch) เดิมคงไว้ atempo หนึ่งขั้นรับค่า 0.5 ถึง 2.0 ถ้าสูงกว่านั้นจะแบ่งเป็นหลายขั้น
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--from)
		FROM="${2:?--from needs a value}"
		shift 2
		;;
	--to)
		TO="${2:?--to needs a value}"
		shift 2
		;;
	--size | -n)
		SIZE="${2:?--size needs a value}"
		shift 2
		;;
	--speed | -s | --atempo | -a)
		SPEED="${2:?atempo needs a value}"
		shift 2
		;;
	--speed=* | --atempo=* | atempo=*)
		SPEED="${1#*=}"
		if [[ -z "$SPEED" ]]; then
			echo "atempo needs a value" >&2
			exit 1
		fi
		shift
		;;
	--input | -i)
		INPUT="${2:?--input needs a value}"
		shift 2
		;;
	--output | -o)
		OUTPUT="${2:?--output needs a value}"
		shift 2
		;;
	--force | -f)
		FORCE=1
		shift
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "unknown argument: $1" >&2
		usage >&2
		exit 1
		;;
	esac
done

if [[ -z "$FROM$TO$SIZE" ]]; then
	echo "กำหนดความยาวด้วย --from/--to หรือ --size" >&2
	usage >&2
	exit 1
fi
if [[ -n "$FROM" && -z "$TO" ]] || [[ -z "$FROM" && -n "$TO" ]]; then
	echo "--from และ --to ต้องใช้คู่กัน" >&2
	exit 1
fi
if [[ -n "$FROM" ]] && { ! [[ "$FROM" =~ ^[0-9]+$ ]] || ! [[ "$TO" =~ ^[0-9]+$ ]]; }; then
	echo "--from และ --to ต้องเป็นตัวเลข" >&2
	exit 1
fi
if [[ -n "$SIZE" ]] && ! [[ "$SIZE" =~ ^[1-9][0-9]*$ ]]; then
	echo "--size ต้องเป็นจำนวนเต็ม >= 1" >&2
	exit 1
fi

atempo_filter() {
	awk -v s="$1" 'BEGIN {
		if (s + 0 != s || s < 0.5) exit 2
		chain = ""
		while (s > 2.0) {
			chain = chain "atempo=2.00,"
			s = s / 2.0
		}
		if (s < 0.5) exit 2
		printf "%satempo=%.2f\n", chain, s
	}'
}

if ! FILTER="$(atempo_filter "$SPEED")"; then
	echo "atempo ต้องเป็นตัวเลข >= 0.5" >&2
	exit 1
fi

if ! command -v ffmpeg >/dev/null 2>&1; then
	echo "ffmpeg not found" >&2
	exit 1
fi
if [[ ! -d "$INPUT" ]]; then
	echo "input directory not found: $INPUT" >&2
	exit 1
fi

file_id() {
	local base
	base="$(basename "$1" .mp3)"
	if ! [[ "$base" =~ ^[0-9]+$ ]]; then
		return 1
	fi
	echo "$((10#$base))"
}

files=()
while IFS= read -r path; do
	base="$(basename "$path")"
	case "$base" in
	*.temp.mp3 | *.speed.mp3 | *.speedup.mp3) continue ;;
	esac
	if ! id="$(file_id "$path")"; then
		continue
	fi
	if [[ -n "$FROM" ]]; then
		if ((id < 10#$FROM || id > 10#$TO)); then
			continue
		fi
	fi
	files+=("$path")
done < <(find "$INPUT" -maxdepth 1 -type f -name '*.mp3' | sort)

if [[ ${#files[@]} -eq 0 ]]; then
	echo "no .mp3 files matched in $INPUT" >&2
	exit 1
fi

speed_tag="$(awk -v s="$SPEED" 'BEGIN { printf "%.4g", s + 0 }')"
mkdir -p "$OUTPUT"

concat_and_speed() {
	local dest="$1"
	shift
	local list escaped dir abs
	list="$(mktemp "${TMPDIR:-/tmp}/atempo-combine.XXXXXX")"
	for path in "$@"; do
		dir="$(cd "$(dirname "$path")" && pwd)"
		abs="${dir}/$(basename "$path")"
		escaped="${abs//\'/\'\\\'\'}"
		printf "file '%s'\n" "$escaped" >>"$list"
	done
	if ! ffmpeg -hide_banner -nostdin -y \
		-f concat -safe 0 -i "$list" \
		-filter:a "$FILTER" \
		-c:a libmp3lame -b:a 86k -ar 48000 -ac 2 \
		"$dest"; then
		rm -f "$list"
		return 1
	fi
	rm -f "$list"
}

index=0
count=${#files[@]}
while ((index < count)); do
	if [[ -n "$SIZE" ]]; then
		end=$((index + SIZE))
	else
		end=$count
	fi
	if ((end > count)); then
		end=$count
	fi
	chunk=("${files[@]:index:end-index}")
	first="$(basename "${chunk[0]}" .mp3)"
	last="$(basename "${chunk[${#chunk[@]}-1]}" .mp3)"
	if [[ "$first" == "$last" ]]; then
		name="$first"
	else
		name="${first}-${last}"
	fi
	dest="${OUTPUT}/${name}.${speed_tag}x.mp3"
	echo "$dest  (${#chunk[@]} files, atempo=${SPEED})"
	if [[ "$DRY_RUN" -eq 1 ]]; then
		index=$end
		continue
	fi
	if [[ -f "$dest" && "$FORCE" -ne 1 ]]; then
		echo "  skip (already exists, use --force to redo)"
		index=$end
		continue
	fi
	concat_and_speed "$dest" "${chunk[@]}"
	index=$end
done
