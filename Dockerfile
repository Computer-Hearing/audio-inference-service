FROM nvcr.io/nvidia/tritonserver:24.09-py3

RUN apt-get update && apt-get install -y \
    ffmpeg \
    libsndfile1 \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
    librosa \
    numpy \
    scipy \
    soundfile