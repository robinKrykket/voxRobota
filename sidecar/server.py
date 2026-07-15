#!/usr/bin/env python3
"""voxclaude STT/TTS sidecar.

A tiny stdlib HTTP server exposing:
  GET  /health          -> "ok"
  POST /stt  (WAV body) -> {"text": "..."}   via faster-whisper
  POST /tts  (JSON)     -> WAV bytes          via kokoro-onnx

Everything runs locally on CPU by default. Models load once at startup.
"""
import io
import json
import os
import sys
import wave
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import numpy as np
import soundfile as sf
from faster_whisper import WhisperModel
from kokoro_onnx import Kokoro

HOST = os.environ.get("VOX_SIDECAR_HOST", "127.0.0.1")
PORT = int(os.environ.get("VOX_SIDECAR_PORT", "8123"))

WHISPER_MODEL = os.environ.get("VOX_WHISPER_MODEL", "base.en")
WHISPER_DEVICE = os.environ.get("VOX_WHISPER_DEVICE", "cpu")       # or "cuda"
WHISPER_COMPUTE = os.environ.get("VOX_WHISPER_COMPUTE", "int8")    # int8 | float16

KOKORO_MODEL = os.environ.get("VOX_KOKORO_MODEL", "kokoro-v1.0.onnx")
KOKORO_VOICES = os.environ.get("VOX_KOKORO_VOICES", "voices-v1.0.bin")
DEFAULT_VOICE = os.environ.get("VOX_VOICE", "af_heart")

print(f"[sidecar] loading whisper '{WHISPER_MODEL}' ({WHISPER_DEVICE}/{WHISPER_COMPUTE})…", flush=True)
whisper = WhisperModel(WHISPER_MODEL, device=WHISPER_DEVICE, compute_type=WHISPER_COMPUTE)

print(f"[sidecar] loading kokoro '{KOKORO_MODEL}'…", flush=True)
kokoro = Kokoro(KOKORO_MODEL, KOKORO_VOICES)

print(f"[sidecar] ready on http://{HOST}:{PORT}", flush=True)


def transcribe(wav_bytes: bytes) -> str:
    audio, sr = sf.read(io.BytesIO(wav_bytes), dtype="float32")
    if audio.ndim > 1:               # downmix to mono
        audio = audio.mean(axis=1)
    if sr != 16000:                  # faster-whisper wants 16 kHz
        # Cheap linear resample; capture is already 16 kHz so this rarely runs.
        n = int(len(audio) * 16000 / sr)
        audio = np.interp(np.linspace(0, len(audio), n, endpoint=False),
                          np.arange(len(audio)), audio).astype("float32")
    segments, _ = whisper.transcribe(audio, language="en", vad_filter=True)
    return "".join(seg.text for seg in segments).strip()


def synthesize(text: str, voice: str) -> bytes:
    samples, sr = kokoro.create(text, voice=voice or DEFAULT_VOICE, speed=1.0, lang="en-us")
    pcm16 = np.clip(samples, -1.0, 1.0)
    pcm16 = (pcm16 * 32767).astype("<i2")
    buf = io.BytesIO()
    with wave.open(buf, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(sr)
        w.writeframes(pcm16.tobytes())
    return buf.getvalue()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):        # quiet by default
        pass

    def _send(self, code, body, ctype="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/health":
            self._send(200, b"ok", "text/plain")
        else:
            self._send(404, b"not found", "text/plain")

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        try:
            if self.path == "/stt":
                text = transcribe(body)
                self._send(200, json.dumps({"text": text}).encode())
            elif self.path == "/tts":
                req = json.loads(body or b"{}")
                wav = synthesize(req.get("text", ""), req.get("voice", ""))
                self._send(200, wav, "audio/wav")
            else:
                self._send(404, b"not found", "text/plain")
        except Exception as e:  # surface errors to the Go client
            self._send(500, json.dumps({"error": str(e)}).encode())


if __name__ == "__main__":
    try:
        ThreadingHTTPServer((HOST, PORT), Handler).serve_forever()
    except KeyboardInterrupt:
        print("\n[sidecar] bye", flush=True)
        sys.exit(0)
