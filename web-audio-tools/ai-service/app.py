"""AI audio service for Web Audio Tools.

Called only by the Go backend. Both share DATA_DIR, so requests carry file
paths instead of audio bytes. Heavy models are optional: each endpoint falls
back to ffmpeg where a reasonable fallback exists, and returns 501 otherwise.
"""

from __future__ import annotations

import importlib.util
import os
import shutil
import subprocess
import sys
import tempfile
import threading
from functools import lru_cache
from pathlib import Path
from typing import Literal

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

DATA_DIR = Path(os.environ.get("DATA_DIR", Path(__file__).resolve().parent.parent / "data")).resolve()

app = FastAPI(title="Web Audio Tools AI service")

# One heavy model run at a time keeps memory bounded on small machines.
_heavy = threading.Lock()


def has_module(name: str) -> bool:
    return importlib.util.find_spec(name) is not None


def available_models() -> dict[str, bool]:
    return {
        "deepfilternet": has_module("df"),
        "demucs": has_module("demucs"),
        "whisper": has_module("faster_whisper"),
    }


class JobRequest(BaseModel):
    input: str
    output_dir: str
    name: str | None = Field(None, description="original file stem used for output names")


class DenoiseRequest(JobRequest):
    strength: float = Field(0.8, ge=0, le=1)


class SeparateRequest(JobRequest):
    mode: Literal["vocals", "4stems"] = "vocals"


class TranscribeRequest(JobRequest):
    language: str | None = None
    model: Literal["tiny", "base", "small", "medium", "large-v3"] = "small"


class JobResult(BaseModel):
    files: list[str]
    engine: str
    text: str | None = None
    language: str | None = None


def inside_data_dir(raw: str, *, must_exist: bool) -> Path:
    path = Path(raw).resolve()
    if not path.is_relative_to(DATA_DIR):
        raise HTTPException(400, f"path must be inside DATA_DIR ({DATA_DIR})")
    if must_exist and not path.exists():
        raise HTTPException(404, f"not found: {path.name}")
    return path


def resolve(req: JobRequest) -> tuple[Path, Path]:
    src = inside_data_dir(req.input, must_exist=True)
    out = inside_data_dir(req.output_dir, must_exist=False)
    out.mkdir(parents=True, exist_ok=True)
    return src, out


def stem(req: JobRequest, src: Path) -> str:
    raw = Path(req.name or src.stem).name
    return "".join(c if c.isalnum() or c in "._-" else "-" for c in raw).strip("-.")[:60] or "audio"


def ffmpeg(*args: str) -> None:
    if shutil.which("ffmpeg") is None:
        raise HTTPException(500, "ffmpeg is not installed on the AI service host")
    proc = subprocess.run(["ffmpeg", "-hide_banner", "-nostdin", "-y", *args], capture_output=True, text=True)
    if proc.returncode != 0:
        raise HTTPException(500, f"ffmpeg failed: {proc.stderr[-1200:]}")


def to_wav(src: Path, dst: Path, sample_rate: int, channels: int | None = None) -> None:
    args = ["-i", str(src), "-vn", "-ar", str(sample_rate)]
    if channels:
        args += ["-ac", str(channels)]
    ffmpeg(*args, "-c:a", "pcm_s16le", str(dst))


@app.get("/health")
def health() -> dict:
    return {"ok": True, "ffmpeg": shutil.which("ffmpeg") is not None, "models": available_models()}


@lru_cache(maxsize=1)
def deepfilternet():
    from df.enhance import init_df

    model, state, _ = init_df()
    return model, state


@app.post("/denoise", response_model=JobResult)
def denoise(req: DenoiseRequest) -> JobResult:
    src, out_dir = resolve(req)
    out = out_dir / f"{stem(req, src)}-denoised.flac"

    if available_models()["deepfilternet"]:
        from df.enhance import enhance, load_audio, save_audio

        with _heavy, tempfile.TemporaryDirectory() as tmp:
            model, state = deepfilternet()
            wav = Path(tmp) / "in.wav"
            to_wav(src, wav, state.sr())
            audio, _ = load_audio(str(wav), sr=state.sr())
            # strength 1.0 = unlimited attenuation; lower values keep some room tone.
            limit = None if req.strength >= 0.99 else 6 + req.strength * 44
            enhanced = enhance(model, state, audio, atten_lim_db=limit)
            clean = Path(tmp) / "clean.wav"
            save_audio(str(clean), enhanced, state.sr())
            ffmpeg("-i", str(clean), "-c:a", "flac", str(out))
        return JobResult(files=[out.name], engine="deepfilternet")

    nr = 6 + req.strength * 24
    ffmpeg("-i", str(src), "-vn", "-af", f"highpass=f=60,afftdn=nr={nr:.1f}:nf=-40:tn=1", "-c:a", "flac", str(out))
    return JobResult(files=[out.name], engine="ffmpeg-afftdn")


@app.post("/separate", response_model=JobResult)
def separate(req: SeparateRequest) -> JobResult:
    src, out_dir = resolve(req)
    base = stem(req, src)

    if available_models()["demucs"]:
        with _heavy, tempfile.TemporaryDirectory() as tmp:
            cmd = [sys.executable, "-m", "demucs", "-n", "htdemucs", "--mp3", "--mp3-bitrate", "256", "-o", tmp]
            if req.mode == "vocals":
                cmd.append("--two-stems=vocals")
            proc = subprocess.run([*cmd, str(src)], capture_output=True, text=True)
            if proc.returncode != 0:
                raise HTTPException(500, f"demucs failed: {proc.stderr[-1200:]}")
            files = []
            for f in sorted(Path(tmp).glob("htdemucs/*/*.mp3")):
                dst = out_dir / f"{base}-{f.stem}.mp3"
                shutil.move(f, dst)
                files.append(dst.name)
        if not files:
            raise HTTPException(500, "demucs produced no stems")
        return JobResult(files=files, engine="demucs-htdemucs")

    if req.mode == "4stems":
        raise HTTPException(501, "4-stem separation needs Demucs (pip install -r requirements-ml.txt)")

    # Phase cancellation: centre-panned vocals cancel out of the side signal.
    karaoke = out_dir / f"{base}-no_vocals.mp3"
    vocals = out_dir / f"{base}-vocals.mp3"
    ffmpeg("-i", str(src), "-vn", "-af", "pan=stereo|c0=c0-c1|c1=c1-c0,volume=1.4", "-c:a", "libmp3lame", "-b:a", "192k", str(karaoke))
    ffmpeg("-i", str(src), "-vn", "-af", "pan=mono|c0=0.5*c0+0.5*c1,highpass=f=150,lowpass=f=7000", "-c:a", "libmp3lame", "-b:a", "192k", str(vocals))
    return JobResult(files=[karaoke.name, vocals.name], engine="ffmpeg-phase-cancel")


@lru_cache(maxsize=2)
def whisper(name: str):
    from faster_whisper import WhisperModel

    return WhisperModel(name, device="auto", compute_type="int8")


def srt_time(seconds: float) -> str:
    ms = round(seconds * 1000)
    h, ms = divmod(ms, 3_600_000)
    m, ms = divmod(ms, 60_000)
    s, ms = divmod(ms, 1000)
    return f"{h:02}:{m:02}:{s:02},{ms:03}"


def write_transcript(segments: list[tuple[float, float, str]], out_dir: Path, base: str) -> tuple[list[str], str]:
    text = "\n".join(t for _, _, t in segments)
    srt = "\n".join(f"{i}\n{srt_time(a)} --> {srt_time(b)}\n{t}\n" for i, (a, b, t) in enumerate(segments, 1))
    (out_dir / f"{base}.txt").write_text(text + "\n", encoding="utf-8")
    (out_dir / f"{base}.srt").write_text(srt, encoding="utf-8")
    return [f"{base}.srt", f"{base}.txt"], text


@app.post("/transcribe", response_model=JobResult)
def transcribe(req: TranscribeRequest) -> JobResult:
    src, out_dir = resolve(req)
    if not available_models()["whisper"]:
        raise HTTPException(501, "transcription needs faster-whisper (pip install -r requirements-ml.txt)")

    with _heavy, tempfile.TemporaryDirectory() as tmp:
        wav = Path(tmp) / "in.wav"
        to_wav(src, wav, 16000, channels=1)
        segments, info = whisper(req.model).transcribe(str(wav), language=req.language or None, vad_filter=True)
        parts = [(s.start, s.end, s.text.strip()) for s in segments if s.text.strip()]

    files, text = write_transcript(parts, out_dir, stem(req, src))
    return JobResult(files=files, text=text, language=info.language, engine=f"faster-whisper-{req.model}")


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host=os.environ.get("HOST", "127.0.0.1"), port=int(os.environ.get("PORT", "8001")))
