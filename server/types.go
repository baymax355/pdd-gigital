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

// 自动化处理请求
type AutoProcessReq struct {
    Speaker         string `json:"speaker"`
    Text            string `json:"text"`
    CopyToCompany   bool   `json:"copy_to_company"`
}

// 自动化处理状态
type AutoProcessStatus struct {
    TaskID          string `json:"task_id"`
    Status          string `json:"status"` // "processing", "completed", "failed"
    CurrentStep     string `json:"current_step"`
    Progress        int    `json:"progress"` // 0-100
    Error           string `json:"error,omitempty"`
    ResultVideo     string `json:"result_video,omitempty"`
    ResultPath      string `json:"result_path,omitempty"`
    StartTime       int64  `json:"start_time"`       // 开始时间戳
    EndTime         int64  `json:"end_time,omitempty"` // 结束时间戳
    TotalDuration   int64  `json:"total_duration,omitempty"` // 总耗时(秒)
}

