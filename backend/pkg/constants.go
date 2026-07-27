package pkg

const (
	// User
	UsernameCookieKey = "username"
	FormDataAudioKey  = "audio"

	// Validate Chunks
	SecondsPerAudioChunk = 2
	AudioWaveBucketsLen  = 40

	// Triton
	ModelName            = "torch_audio_cnn"
	InputName            = "input"
	MelBins              = 256
	TimeFrames           = 173
	Channels             = 1
	CategoryOutputName   = "category_output"
	TargetOutputName     = "target_output"
	PipelineModelName    = "cnn_predict_pipline"
	RawAudioInputName    = "RAW_AUDIO"
	MaxTritonConcurrency = 4
)
