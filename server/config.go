package main

import (
    "log"
    "os"
)

type Config struct {
    Port                 string
    WorkDir              string
    StaticDir            string
    HostVoiceDir         string
    HostVideoDir         string
    HostResultDir        string
    WindowsCompanyDir    string
    TTSBaseURL           string
    VideoBaseURL         string
    GenVideoContainer    string
    ContainerDataRoot    string
}

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func loadConfig() Config {
    cfg := Config{
        Port:              getenv("APP_PORT", "8090"),
        WorkDir:           getenv("APP_WORKDIR", "./data"),
        StaticDir:         getenv("STATIC_DIR", ""),
        HostVoiceDir:      getenv("HOST_VOICE_DIR", "/root/heygem_data/voice/data"),
        HostVideoDir:      getenv("HOST_VIDEO_DIR", "/root/heygem_data/face2face"),
        HostResultDir:     getenv("HOST_RESULT_DIR", "/root/heygem_data/face2face/result"),
        WindowsCompanyDir: getenv("WIN_COMPANY_DIR", "/mnt/c/company"),
        TTSBaseURL:        getenv("TTS_BASE_URL", "http://127.0.0.1:18180"),
        VideoBaseURL:      getenv("VIDEO_BASE_URL", "http://127.0.0.1:8383"),
        GenVideoContainer: getenv("GEN_VIDEO_CONTAINER", "heygem-gen-video"),
        ContainerDataRoot: getenv("GEN_VIDEO_CONTAINER_DATA_ROOT", "/code/data"),
    }

    mustMkdirAll(cfg.WorkDir)
    mustMkdirAll(cfg.HostVoiceDir)
    mustMkdirAll(cfg.HostVideoDir)
    mustMkdirAll(cfg.HostResultDir)

    return cfg
}

func mustMkdirAll(path string) {
    if err := os.MkdirAll(path, 0o755); err != nil {
        log.Fatalf("mkdir %s failed: %v", path, err)
    }
}
