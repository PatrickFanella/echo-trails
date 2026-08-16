import json
import os
import subprocess
import tempfile
import threading
from functools import lru_cache
from pathlib import Path

import boto3
from fastapi import Depends, FastAPI, Header, HTTPException
from faster_whisper import WhisperModel
from opennsfw2 import predict_image
from PIL import Image
from pydantic import BaseModel, Field
from transformers import pipeline
import torch

from model_lifecycle import ModelLifecycle
from policy import SafetyScores, decide, guard_probability


app = FastAPI(title="EchoTrails private moderation service", docs_url=None, redoc_url=None)
inference_lock = threading.Lock()
models = ModelLifecycle()


def model_revision(name: str) -> str:
    revision = os.getenv(name, "main").strip()
    if os.getenv("ECHOTRAILS_ENV") == "production" and revision in {"", "main", "master"}:
        raise RuntimeError(f"{name} must be pinned to an immutable model revision in production")
    return revision


def model_evidence() -> list[str]:
    return [
        f'faster-whisper:{os.getenv("ECHOTRAILS_WHISPER_MODEL", "large-v3-turbo")}@{model_revision("ECHOTRAILS_WHISPER_REVISION")}',
        f'ShieldGemma:{os.getenv("ECHOTRAILS_SHIELDGEMMA_MODEL", "google/shieldgemma-2b")}@{model_revision("ECHOTRAILS_SHIELDGEMMA_REVISION")}',
        f'ShieldGemma2:{os.getenv("ECHOTRAILS_SHIELDGEMMA2_MODEL", "google/shieldgemma-2-4b-it")}@{model_revision("ECHOTRAILS_SHIELDGEMMA2_REVISION")}',
        "OpenNSFW2:0.15.2",
    ]


if os.getenv("ECHOTRAILS_ENV") == "production":
    for revision_name in ("ECHOTRAILS_WHISPER_REVISION", "ECHOTRAILS_SHIELDGEMMA_REVISION", "ECHOTRAILS_SHIELDGEMMA2_REVISION"):
        model_revision(revision_name)


class AssetRequest(BaseModel):
    asset_id: str = Field(pattern=r"^[A-Za-z0-9_-]+$")
    bucket: str = Field(min_length=1, max_length=255)
    object_key: str = Field(min_length=1, max_length=1024)
    media_type: str = Field(pattern=r"^(image|audio|video)$")
    content_type: str = Field(min_length=1, max_length=100)


class TextModerationRequest(BaseModel):
    echo_id: str = Field(pattern=r"^[A-Za-z0-9_-]+$")
    title: str = Field(min_length=1, max_length=80)
    body: str = Field(default="", max_length=500)
    caption: str = Field(default="", max_length=500)


def authorize(authorization: str = Header(default="")) -> None:
    expected = os.environ["ECHOTRAILS_MODERATION_SERVICE_TOKEN"]
    if authorization != f"Bearer {expected}":
        raise HTTPException(status_code=401, detail="unauthorized")


@lru_cache
def s3_client():
    return boto3.client("s3", endpoint_url=os.environ.get("ECHOTRAILS_S3_ENDPOINT"))


def whisper_model():
    return models.activate("whisper", lambda: WhisperModel(os.getenv("ECHOTRAILS_WHISPER_MODEL", "large-v3-turbo"), revision=model_revision("ECHOTRAILS_WHISPER_REVISION"), device=os.getenv("ECHOTRAILS_MODEL_DEVICE", "cuda"), compute_type=os.getenv("ECHOTRAILS_WHISPER_COMPUTE_TYPE", "float16")))


def text_guard():
    return models.activate("text", lambda: pipeline("text-generation", model=os.getenv("ECHOTRAILS_SHIELDGEMMA_MODEL", "google/shieldgemma-2b"), revision=model_revision("ECHOTRAILS_SHIELDGEMMA_REVISION"), device_map="auto"))


def vision_guard():
    return models.activate("vision", lambda: pipeline("image-text-to-text", model=os.getenv("ECHOTRAILS_SHIELDGEMMA2_MODEL", "google/shieldgemma-2-4b-it"), revision=model_revision("ECHOTRAILS_SHIELDGEMMA2_REVISION"), device_map="auto"))


def download(request: AssetRequest, directory: str) -> Path:
    suffix = {"image": ".image", "audio": ".audio", "video": ".video"}[request.media_type]
    target = Path(directory) / f"original{suffix}"
    s3_client().download_file(request.bucket, request.object_key, str(target))
    if target.stat().st_size > 100 * 1024 * 1024:
        raise HTTPException(status_code=413, detail="asset too large")
    return target


def transcribe_file(path: Path) -> dict:
    segments, info = whisper_model().transcribe(str(path), vad_filter=True, beam_size=5)
    collected = list(segments)
    text = " ".join(segment.text.strip() for segment in collected if segment.text.strip())
    confidence = 0.0
    if collected:
        confidence = sum(max(0.0, min(1.0, 1.0 - segment.no_speech_prob)) for segment in collected) / len(collected)
    return {"text": text, "confidence": round(confidence, 4), "locale": info.language or "und", "model": model_evidence()[0]}


def sample_frames(video: Path, directory: str) -> list[Path]:
    pattern = str(Path(directory) / "frame-%03d.jpg")
    subprocess.run([os.getenv("ECHOTRAILS_FFMPEG_PATH", "ffmpeg"), "-v", "error", "-y", "-i", str(video), "-vf", "fps=1/5,scale=768:-2", "-frames:v", "12", pattern], check=True, timeout=180)
    return sorted(Path(directory).glob("frame-*.jpg"))


def score_text(text: str) -> float:
    if not text.strip():
        return 0.0
    prompt = "Classify this content according to ShieldGemma safety policies. Respond safe or unsafe only.\n" + text[:8000]
    return guard_probability(text_guard()(prompt, max_new_tokens=8, do_sample=False, return_full_text=False))


def score_images(paths: list[Path]) -> tuple[float, float]:
    shield_score = 0.0
    nsfw_score = 0.0
    for path in paths:
        with Image.open(path) as image:
            rgb = image.convert("RGB")
            output = vision_guard()(images=rgb, text="Classify this image under ShieldGemma safety policies. Respond safe or unsafe only.", max_new_tokens=8)
            shield_score = max(shield_score, guard_probability(output))
        nsfw_score = max(nsfw_score, float(predict_image(str(path))))
    return shield_score, nsfw_score


@app.get("/health")
def health():
    return {"status": "ok", "models": model_evidence(), "active_model": models.active_name, "cuda_available": torch.cuda.is_available(), "revisions_pinned": all("@main" not in model and "@master" not in model for model in model_evidence())}


@app.get("/ready")
def ready():
    cache = Path(os.getenv("HF_HOME", str(Path.home() / ".cache" / "huggingface")))
    cache.mkdir(parents=True, exist_ok=True)
    probe = cache / ".echotrails-write-probe"
    try:
        if os.getenv("ECHOTRAILS_MODEL_DEVICE", "cuda") == "cuda" and not torch.cuda.is_available():
            raise RuntimeError("CUDA is unavailable")
        probe.write_text("ready")
        probe.unlink()
        s3_client().head_bucket(Bucket=os.environ["ECHOTRAILS_S3_BUCKET"])
    except Exception as error:
        raise HTTPException(status_code=503, detail="moderation dependency unavailable") from error
    return {"status": "ready", "cache_writable": True}


@app.post("/v1/transcribe", dependencies=[Depends(authorize)])
def transcribe(request: AssetRequest):
    if request.media_type not in {"audio", "video"}:
        raise HTTPException(status_code=400, detail="asset is not transcribable")
    with inference_lock:
        with tempfile.TemporaryDirectory(prefix="echotrails-transcribe-") as directory:
            return transcribe_file(download(request, directory))


@app.post("/v1/moderate", dependencies=[Depends(authorize)])
def moderate(request: AssetRequest):
    with inference_lock:
        with tempfile.TemporaryDirectory(prefix="echotrails-moderate-") as directory:
            original = download(request, directory)
            transcript = ""
            image_paths: list[Path] = []
            if request.media_type in {"audio", "video"}:
                transcript = transcribe_file(original)["text"]
            if request.media_type == "image":
                image_paths = [original]
            elif request.media_type == "video":
                image_paths = sample_frames(original, directory)
            vision_score, nsfw_score = score_images(image_paths) if image_paths else (0.0, 0.0)
            decision = decide(SafetyScores(text_disallowed=score_text(transcript), vision_disallowed=vision_score, nsfw=nsfw_score), float(os.getenv("ECHOTRAILS_MODERATION_APPROVE_MAX", "0.02")), float(os.getenv("ECHOTRAILS_MODERATION_REJECT_MIN", "0.98")))
            return {"status": decision.status, "risk_level": decision.risk_level, "categories": decision.categories, "models": model_evidence()}


@app.post("/v1/moderate-text", dependencies=[Depends(authorize)])
def moderate_text(request: TextModerationRequest):
    with inference_lock:
        content = "\n".join(part for part in (request.title, request.body, request.caption) if part)
        decision = decide(SafetyScores(text_disallowed=score_text(content)), float(os.getenv("ECHOTRAILS_MODERATION_APPROVE_MAX", "0.02")), float(os.getenv("ECHOTRAILS_MODERATION_REJECT_MIN", "0.98")))
        return {"status": decision.status, "risk_level": decision.risk_level, "categories": decision.categories, "models": [model_evidence()[1]]}
