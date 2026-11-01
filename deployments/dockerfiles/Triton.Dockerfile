FROM w0dr/tritonserver:24.09-py3

COPY ../triton/models/* /models

RUN apt-get update && apt-get install -y \
    ffmpeg \
    libsndfile1 \
    && rm -rf /var/lib/apt/lists/*

RUN pip install --no-cache-dir \
    librosa \
    numpy \
    scipy \
    soundfile
