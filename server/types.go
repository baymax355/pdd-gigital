package main

type PreprocessReq struct {
    Format          string `json:"format"`
    ReferenceAudio  string `json:"reference_audio"`
    Lang            string `json:"lang"`
}

type PreprocessResp struct {
    Code int    `json:"code"`
    Msg  string `json:"msg"`
    ReferenceAudioText string `json:"reference_audio_text"`
    ASRFormatAudioURL  string `json:"asr_format_audio_url"`
}

type TTSInvokeReq struct {
    Speaker            string  `json:"speaker"`
    Text               string  `json:"text"`
    Format             string  `json:"format"`
    TopP               float64 `json:"topP"`
    MaxNewTokens       int     `json:"max_new_tokens"`
    ChunkLength        int     `json:"chunk_length"`
    RepetitionPenalty  float64 `json:"repetition_penalty"`
    Temperature        float64 `json:"temperature"`
    NeedASR            bool    `json:"need_asr"`
    Streaming          bool    `json:"streaming"`
    IsFixedSeed        int     `json:"is_fixed_seed"`
    IsNorm             int     `json:"is_norm"`
    ReferenceAudio     string  `json:"reference_audio"`
    ReferenceText      string  `json:"reference_text"`
}

type SubmitVideoReq struct {
    AudioFilename   string `json:"audio_filename"`
    VideoFilename   string `json:"video_filename"`
    Code            string `json:"code"`
    Chaofen         int    `json:"chaofen"`
    WatermarkSwitch int    `json:"watermark_switch"`
    PN              int    `json:"pn"`
}

