FROM nvidia/cuda:12.8.1-cudnn-runtime-ubuntu24.04

ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PIP_NO_CACHE_DIR=1
RUN apt-get update && apt-get install -y --no-install-recommends python3 python3-pip ffmpeg libgl1 libglib2.0-0 && rm -rf /var/lib/apt/lists/*
WORKDIR /service
COPY requirements.txt .
RUN python3 -m pip install --break-system-packages -r requirements.txt
COPY app.py model_lifecycle.py policy.py ./
RUN useradd --create-home --uid 10001 worker && chown -R worker:worker /service
USER worker
EXPOSE 8081
CMD ["python3", "-m", "uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8081", "--no-access-log"]
