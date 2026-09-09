import triton_python_backend_utils as pb_utils
import numpy as np
import torch
import torchaudio
import io
import json
torchaudio.set_audio_backend("soundfile")


class TritonPythonModel:
    def initialize(self, args):
        self.model_config = json.loads(args['model_config'])
        # Параметры нормализации
        self.mel_mean = -41.9199
        self.mel_std = 17.9253
        # Параметры Mel-спектрограммы
        self.sample_rate = 44000
        self.n_mels = 256
        self.n_fft = 2048
        self.hop_length = 512
        self.f_max = 16384
        self.target_width = 173

        # Трансформации на GPU если есть
        self.device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        self.mel_transform = torchaudio.transforms.MelSpectrogram(
            sample_rate=self.sample_rate,
            n_fft=self.n_fft,
            hop_length=self.hop_length,
            n_mels=self.n_mels,
            f_max=self.f_max,
        ).to(self.device)
        self.db_transform = torchaudio.transforms.AmplitudeToDB(
            top_db=80.0
        ).to(self.device)

        # Кэш для ресемплеров
        self.resamplers = {}
        # Для моно
        self.downmix = torchaudio.transforms.DownmixMono()

        print(f"Model initialized on {self.device} with torchaudio")
        print(f"Available backends: {torchaudio.list_audio_backends()}")

    def _load_audio(self, audio_bytes):
        """Загружает аудио из байтов и приводит к нужному формату"""
        try:
            waveform, orig_sr = torchaudio.load(io.BytesIO(audio_bytes))
        except Exception as e:
            raise RuntimeError(f"Failed to load audio: {e}")

        # Ресемплинг при необходимости
        if orig_sr != self.sample_rate:
            if orig_sr not in self.resamplers:
                self.resamplers[orig_sr] = torchaudio.transforms.Resample(orig_sr, self.sample_rate)
            waveform = self.resamplers[orig_sr](waveform)

        # Приводим к моно
        if waveform.shape[0] > 1:
            waveform = self.downmix(waveform)

        return waveform.squeeze(0).float()

    def execute(self, requests):
        all_audio_bytes = []
        batch_sizes = []

        # Сбор всех аудио
        for request in requests:
            raw_tensor = pb_utils.get_input_tensor_by_name(request, "RAW_AUDIO")
            audio_batch = raw_tensor.as_numpy()
            batch_sizes.append(audio_batch.shape[0])
            for i in range(audio_batch.shape[0]):
                all_audio_bytes.append(audio_batch[i].tobytes())

        total_batch = len(all_audio_bytes)
        if total_batch == 0:
            return [pb_utils.InferenceResponse() for _ in requests]

        # Загрузка всех аудио
        audio_tensors_cpu = []
        for audio_bytes in all_audio_bytes:
            try:
                audio_tensors_cpu.append(self._load_audio(audio_bytes))
            except Exception as e:
                error_msg = f"Audio loading failed: {e}"
                return [pb_utils.InferenceResponse(error=pb_utils.TritonError(error_msg))
                        for _ in requests]

        # Паддинг до максимальной длины
        max_len = max(t.shape[0] for t in audio_tensors_cpu)
        padded_batch = torch.stack([
            torch.nn.functional.pad(t, (0, max_len - t.shape[0]))
            for t in audio_tensors_cpu
        ], dim=0)

        # Вычисление спектрограмм на GPU
        audio_batch_gpu = padded_batch.to(self.device)
        mel_spec_batch = self.mel_transform(audio_batch_gpu)
        mel_spec_db_batch = self.db_transform(mel_spec_batch)

        # Обработка каждой спектрограммы
        mel_spec_db_np = mel_spec_db_batch.cpu().numpy()
        processed = []
        for i in range(mel_spec_db_np.shape[0]):
            spec = mel_spec_db_np[i]

            # Обрезка/дополнение
            if spec.shape[1] > self.target_width:
                spec = spec[:, :self.target_width]
            elif spec.shape[1] < self.target_width:
                pad = self.target_width - spec.shape[1]
                spec = np.pad(spec, ((0, 0), (0, pad)), mode='constant')

            # Нормализация
            spec = (spec - self.mel_mean) / (self.mel_std + 1e-8)
            spec = np.expand_dims(spec, axis=0).astype(np.float32)
            processed.append(spec)

        batch_input = np.stack(processed, axis=0)

        # Вызов ONNX-модели
        inference_request = pb_utils.InferenceRequest(
            model_name="torch_audio_cnn",
            requested_output_names=["category_output", "target_output"],
            inputs=[pb_utils.Tensor("input", batch_input)]
        )
        inference_response = inference_request.exec()

        if inference_response.has_error():
            error_msg = inference_response.error().message()
            return [pb_utils.InferenceResponse(error=pb_utils.TritonError(error_msg))
                    for _ in requests]

        cat_out = pb_utils.get_output_tensor_by_name(inference_response, "category_output")
        tgt_out = pb_utils.get_output_tensor_by_name(inference_response, "target_output")
        cat_np = cat_out.as_numpy()
        tgt_np = tgt_out.as_numpy()

        # Разбивка по запросам
        responses = []
        offset = 0
        for bsz in batch_sizes:
            cat_slice = cat_np[offset:offset + bsz]
            tgt_slice = tgt_np[offset:offset + bsz]
            offset += bsz
            responses.append(
                pb_utils.InferenceResponse(
                    output_tensors=[
                        pb_utils.Tensor("category_output", cat_slice),
                        pb_utils.Tensor("target_output", tgt_slice)
                    ]
                )
            )
        return responses