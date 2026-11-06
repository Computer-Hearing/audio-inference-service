import triton_python_backend_utils as pb_utils
import numpy as np
import librosa
import io
import json


class TritonPythonModel:
    def initialize(self, args):
        self.model_config = json.loads(args['model_config'])
        self.mel_mean = -35.6561
        self.mel_std = 23.0875
        # Параметры Mel-спектрограммы
        self.sample_rate = 44100
        self.n_mels = 256
        self.n_fft = 2048
        self.hop_length = 512
        self.f_max = 16384
        self.target_width = 173

        print(f"Model initialized with librosa")

    def finalize(self):
        """Обязательный метод для Triton"""
        print("Model finalized")

    def _load_audio(self, audio_bytes):
        try:
            audio_data, orig_sr = librosa.load(io.BytesIO(audio_bytes), sr=None, mono=True)
        except Exception as e:
            raise RuntimeError(f"Failed to load audio: {e}")

        if orig_sr != self.sample_rate:
            audio_data = librosa.resample(audio_data, orig_sr=orig_sr, target_sr=self.sample_rate)

        return audio_data.astype(np.float32)

    def _audio_to_mel(self, audio):
        mel_spec = librosa.feature.melspectrogram(
            y=audio,
            sr=self.sample_rate,
            n_mels=self.n_mels,
            n_fft=self.n_fft,
            hop_length=self.hop_length,
            fmax=self.f_max,
            power=2.0
        )
        mel_spec_db = librosa.power_to_db(mel_spec, ref=np.max)
        return mel_spec_db

    def execute(self, requests):
        all_audio_bytes = []
        batch_sizes = []

        for request in requests:
            raw_tensor = pb_utils.get_input_tensor_by_name(request, "RAW_AUDIO")
            audio_batch = raw_tensor.as_numpy()
            batch_sizes.append(audio_batch.shape[0])
            for i in range(audio_batch.shape[0]):
                if isinstance(audio_batch[i], bytes):
                    all_audio_bytes.append(audio_batch[i])
                else:
                    all_audio_bytes.append(audio_batch[i].tobytes())

        total_batch = len(all_audio_bytes)
        if total_batch == 0:
            return [pb_utils.InferenceResponse() for _ in requests]

        processed = []
        for audio_bytes in all_audio_bytes:
            try:
                audio = self._load_audio(audio_bytes)
                mel_spec = self._audio_to_mel(audio)

                # Truncate/Pad
                if mel_spec.shape[1] > self.target_width:
                    mel_spec = mel_spec[:, :self.target_width]
                elif mel_spec.shape[1] < self.target_width:
                    pad_width = self.target_width - mel_spec.shape[1]
                    mel_spec = np.pad(mel_spec, ((0, 0), (0, pad_width)), mode='constant')

                # Нормализация
                mel_spec = (mel_spec - self.mel_mean) / (self.mel_std + 1e-8)

                # Транспонируем для RNN (173, 256) и делаем contiguous
                mel_spec = np.ascontiguousarray(mel_spec.T).astype(np.float32)  # (173, 256)

                processed.append(mel_spec)

            except Exception as e:
                error_msg = f"Audio processing failed: {e}"
                return [pb_utils.InferenceResponse(error=pb_utils.TritonError(error_msg))
                        for _ in requests]

        batch_input = np.stack(processed, axis=0)

        inference_request = pb_utils.InferenceRequest(
            model_name="road_rnn_99k_09915acc_1_infer",
            requested_output_names=["target_output"],
            inputs=[pb_utils.Tensor("input", batch_input)]
        )
        inference_response = inference_request.exec()

        if inference_response.has_error():
            error_msg = inference_response.error().message()
            return [pb_utils.InferenceResponse(error=pb_utils.TritonError(error_msg))
                    for _ in requests]

        tgt_out = pb_utils.get_output_tensor_by_name(inference_response, "target_output")
        tgt_np = tgt_out.as_numpy()

        responses = []
        offset = 0
        for bsz in batch_sizes:
            tgt_slice = tgt_np[offset:offset + bsz]
            offset += bsz
            responses.append(
                pb_utils.InferenceResponse(
                    output_tensors=[
                        pb_utils.Tensor("target_output", tgt_slice)
                    ]
                )
            )
        return responses