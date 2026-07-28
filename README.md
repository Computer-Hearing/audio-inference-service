# Audio Traffic Classification
Система для multi-task классификации городских акустических событий. Модель одновременно решает две задачи.


---

## Задача

Multi-task классификация звуков дорожного трафика:

1. **Тип транспорта** (5 классов):
   - car
   - emv (спецтранспорт)
   - motorcycle
   - tram
   - truck
2. **Характер движения** (7 классов):
   - acceleration
   - bell
   - braking
   - horn
   - idling
   - passing
   - siren

Обе задачи решаются одновременно на основе одного входного аудиосигнала.

---

## Данные

- **Формат:** WAV, моно
- **Количество файлов:** 6 281

---

## Эксперименты

Все эксперименты проводились в Google Colab с графическим ускорителем NVIDIA T4. Замеры времени инференса производились с усреднением по 100 запускам после прогрева модели. Для воспроизводимости использовался фиксированный seed.

### Сравнительная таблица результатов

| Модель | Бэкенд | Эмбеддер | Вход | Параметры | Время (мс) | Vehicle Acc | Action Acc | Combined Acc |
|--------|--------|----------|------|-----------|------------|-------------|------------|--------------|
| CNN | PyTorch | None | мел-спектрограмма (2D) | — | — | 96.0% | 88.0% | — |
| Transformer (PyTorch) | PyTorch | PANNs | эмбеддинги 2048 | — | — | 97.0% | 89.0% | — |
| LSTM v1 | TensorFlow | YAMNet | 4 × 1024 | 1.58M | 8.30 | 97.8% | 84.8% | 84.0% |
| LSTM v2 | TensorFlow | YAMNet | 4 × 1024 | 645K | 9.44 | 97.8% | 87.0% | 86.0% |
| Transformer v1 | TensorFlow | YAMNet | 4 × 1024 | 17.10M | 8.68 | 95.3% | 79.6% | 77.0% |
| Transformer v2 | TensorFlow | YAMNet | 4 × 1024 | 544K | 12.61 | 97.8% | 89.8% | 89.0% |
| RNN | TensorFlow | YAMNet | 4 × 1024 | — | — | — | — | — |
| MLP | TensorFlow | None | 4 × 1024 | — | — | — | — | — |

---

## Архитектуры

### CNN (PyTorch)
- **Вход:** мел-спектрограмма (128 mel-bands × временные фреймы)
- **Структура:** 4 сверточных блока (Conv2d + BatchNorm + ReLU + MaxPool) → Flatten → Dropout → Dense → 2 головы (softmax)
- **Loss:** взвешенная кросс-энтропия

### Transformer (PyTorch) 
- **Вход:** последовательность эмбеддингов PANNs (размерность 2048)
- **Структура:** Positional Encoding → Multi-head Self-Attention → Feed Forward → LayerNorm → 2 головы (softmax)

### LSTM v1 (TensorFlow)
- **Эмбеддер:** YAMNet (4 кадра × 1024 признака)
- **Архитектура:** LSTM(256, return_sequences=True) → Dropout(0.3) → LSTM(128) → Dropout(0.3) → Dense(256) → Dense(128) → Dense(64) → 2 головы
- **Параметры:** 1.58M
- **Время:** 8.30 мс

### LSTM v2 (TensorFlow) — улучшенная версия
- **Эмбеддер:** YAMNet (4 кадра × 1024 признака)
- **Архитектура:** LSTM(128, dropout=0.2, recurrent_dropout=0.2) → BatchNorm → LSTM(64, dropout=0.2, recurrent_dropout=0.2) → BatchNorm → Dropout(0.3) → Dense(64) → Dropout(0.3) → 2 головы
- **Ключевые улучшения:** уменьшение параметров в 2.4 раза, BatchNormalization, dropout внутри LSTM
- **Параметры:** 645K
- **Время:** 9.44 мс

### Transformer v1 (TensorFlow)
- **Эмбеддер:** YAMNet (4 кадра × 1024 признака)
- **Архитектура:** Positional Encoding → 2× (MultiHeadAttention(4 heads) → FFN(Dense 2048 → 1024)) → GlobalAvgPooling → Dense(256) → Dense(128) → 2 головы
- **Параметры:** 17.10M
- **Время:** 8.68 мс
- **Недостатки:** переобучение на малом датасете, избыточная сложность

### Transformer v2 (TensorFlow) — улучшенная версия
- **Эмбеддер:** YAMNet (4 кадра × 1024 признака)
- **Архитектура:** Dense(512) → Positional Encoding → 1× (MultiHeadAttention(2 heads) → FFN(Dense 512 → 512)) → GlobalAvgPooling → Dense(128) → Dense(64) → 2 головы
- **Ключевые улучшения:** уменьшение параметров в 31 раз, проекция входа, увеличение dropout
- **Параметры:** 544K
- **Время:** 12.61 мс

### RNN / MLP (TensorFlow) — экспериментальные
- **Эмбеддер:** YAMNet (4 кадра × 1024 признака)

---

## Инференс

Инференс построен на **NVIDIA Triton Inference Server**:

1. **Вход:** WAV-файл → клиент отправляет запрос на Triton
2. **Препроцессинг:** внутри Triton через Python backend:
   - Загрузка аудио через Librosa
   - Построение мел-спектрограммы (для CNN)
   - (для Transformer) извлечение эмбеддингов через PANNs
3. **Модель:** Inference через оптимизированную модель (TensorRT)
4. **Выход:** предсказания по двум задачам

```bash
# Запуск Triton
docker-compose up -d

# Отправка запроса (пример)
curl -X POST http://localhost:8000/v2/models/audio_classifier/infer
```

## Запуск проекта
```bash
git clone ...
docker-compose up -d
```
