import shutil
import subprocess
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

import app as service

needs_ffmpeg = pytest.mark.skipif(shutil.which("ffmpeg") is None, reason="ffmpeg not installed")


@pytest.fixture()
def data_dir(tmp_path, monkeypatch):
    monkeypatch.setattr(service, "DATA_DIR", tmp_path.resolve())
    return tmp_path.resolve()


@pytest.fixture()
def client():
    return TestClient(service.app)


@pytest.fixture()
def no_models(monkeypatch):
    monkeypatch.setattr(service, "available_models", lambda: {"deepfilternet": False, "demucs": False, "whisper": False})


def make_tone(path: Path) -> Path:
    subprocess.run(
        ["ffmpeg", "-hide_banner", "-y", "-f", "lavfi", "-i", "sine=frequency=330:duration=1", "-ac", "2", str(path)],
        check=True,
        capture_output=True,
    )
    return path


def test_health(client):
    body = client.get("/health").json()
    assert body["ok"] is True
    assert set(body["models"]) == {"deepfilternet", "demucs", "whisper"}


def test_rejects_paths_outside_data_dir(client, data_dir, tmp_path_factory):
    outside = tmp_path_factory.mktemp("outside") / "x.wav"
    outside.write_bytes(b"x")
    res = client.post("/denoise", json={"input": str(outside), "output_dir": str(data_dir / "jobs" / "a")})
    assert res.status_code == 400

    inside = data_dir / "in.wav"
    inside.write_bytes(b"x")
    res = client.post("/denoise", json={"input": str(inside), "output_dir": str(data_dir / ".." / "escape")})
    assert res.status_code == 400


@needs_ffmpeg
def test_denoise_falls_back_to_ffmpeg(client, data_dir, no_models):
    src = make_tone(data_dir / "abc123.wav")
    out_dir = data_dir / "jobs" / "1"
    res = client.post("/denoise", json={"input": str(src), "output_dir": str(out_dir), "strength": 0.5, "name": "my clip"})
    assert res.status_code == 200, res.text
    body = res.json()
    assert body["engine"] == "ffmpeg-afftdn"
    assert body["files"] == ["my-clip-denoised.flac"]
    assert (out_dir / "my-clip-denoised.flac").stat().st_size > 0


@needs_ffmpeg
def test_separate_vocals_fallback_and_4stems_unavailable(client, data_dir, no_models):
    src = make_tone(data_dir / "song.wav")
    out_dir = data_dir / "jobs" / "2"
    res = client.post("/separate", json={"input": str(src), "output_dir": str(out_dir), "mode": "vocals"})
    assert res.status_code == 200, res.text
    assert sorted(res.json()["files"]) == ["song-no_vocals.mp3", "song-vocals.mp3"]

    res = client.post("/separate", json={"input": str(src), "output_dir": str(out_dir), "mode": "4stems"})
    assert res.status_code == 501


def test_transcribe_requires_whisper(client, data_dir, no_models):
    src = data_dir / "talk.wav"
    src.write_bytes(b"x")
    res = client.post("/transcribe", json={"input": str(src), "output_dir": str(data_dir / "jobs" / "3")})
    assert res.status_code == 501


def test_transcript_writers(tmp_path):
    files, text = service.write_transcript([(0.0, 1.25, "สวัสดี"), (61.5, 3725.042, "ครับ")], tmp_path, "t")
    assert files == ["t.srt", "t.txt"]
    assert text == "สวัสดี\nครับ"
    srt = (tmp_path / "t.srt").read_text(encoding="utf-8")
    assert "00:00:00,000 --> 00:00:01,250" in srt
    assert "00:01:01,500 --> 01:02:05,042" in srt
