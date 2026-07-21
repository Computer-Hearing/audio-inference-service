import triton_python_backend_utils as pb_utils # это не внешняя библиотека, а служебный модуль triton. Ставить не надо
import librosa
import numpy as np
import io
import json


class TritonPythonModel:
    def initialize(self, args):
        self.model_config = json.loads(args['model_config'])
        print("Model initialized")

    def execute(self, requests):
        responses = []

        for request in requests:
            # Получаем данные от сервиса
            raw_audio_tensor = pb_utils.get_input_tensor_by_name(request, "RAW_AUDIO")
            raw_audio_bytes = raw_audio_tensor.as_numpy().tobytes()

            y, sr = librosa.load(io.BytesIO(raw_audio_bytes), sr=44100)
            mel_spectrogram = librosa.feature.melspectrogram(y=y, sr=sr, n_mels=256, fmax=16384)
            mel_spectrogram_db = librosa.power_to_db(mel_spectrogram, ref=np.max)
            input_tensor = self.truncate_spectrogram(mel_spectrogram_db, (256, 173))

            # Вызываем ONNX-модель через BLS
            # Создаем запрос к модели
            inference_request = pb_utils.InferenceRequest(
                model_name="torch_audio_cnn",
                requested_output_names=["output"],
                inputs=[pb_utils.Tensor("input", input_tensor)]
            )

            # Выполняем синхронный запрос [citation:6]
            inference_response = inference_request.exec()

            if inference_response.has_error():
                error_msg = inference_response.error().message()
                responses.append(
                    pb_utils.InferenceResponse(
                        error=pb_utils.TritonError(error_msg)
                    )
                )
                continue

            # Получаем результат от модели
            output_tensor = pb_utils.get_output_tensor_by_name(
                inference_response, "output"
            )

            # Формируем финальный ответ для сервиса
            final_response = pb_utils.InferenceResponse(
                output_tensors=[output_tensor]
            )
            responses.append(final_response)

        return responses

    @staticmethod
    def truncate_spectrogram(spectrogram, target_shape) -> np.ndarray:
        """
        Приводит спектрограмму к определенному размеру.
        (Чтобы скармливать нейросети одинаковые по размеру массивы)

        Если спектрограмма длиннее целевого размера - обрезает справа.
        Если короче - дополняет нулями справа.

        Parameters:
        spectrogram : Исходная спектрограмма
        target_shape : Целевой размер для обрезки
        """
        if spectrogram.shape[1] > target_shape[1]:
            return spectrogram[:, :target_shape[1]]
        elif spectrogram.shape[1] < target_shape[1]:
            padding = target_shape[1] - spectrogram.shape[1]
            return np.pad(spectrogram, ((0, 0), (0, padding)), mode='constant')
        return spectrogram

