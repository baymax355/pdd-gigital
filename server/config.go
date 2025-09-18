package main

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	Port              string
	WorkDir           string
	StaticDir         string
	HostVoiceDir      string
	HostVideoDir      string
	HostResultDir     string
	WindowsCompanyDir string
	TTSBaseURL        string
	VideoBaseURL      string
	GenVideoContainer string
	ContainerDataRoot string
	RabbitURL         string
	QueuePrefix       string
	RedisAddr         string
	RedisPassword     string
	VideoWaitTimeout  time.Duration
	AudioTemplateDir  string
    VideoTemplateDir  string
    UsersFile         string
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
		RabbitURL:         getenv("RABBITMQ_URL", "amqp://root:pddrabitmq1041@192.168.7.240:5672"),
		QueuePrefix:       getenv("QUEUE_PREFIX", "digital_people"),
		RedisAddr:         getenv("REDIS_ADDR", "192.168.7.29:6379"),
        RedisPassword:     getenv("REDIS_PASSWORD", ""),
    }

	cfg.AudioTemplateDir = getenv("AUDIO_TEMPLATE_DIR", "")
	if cfg.AudioTemplateDir == "" {
		cfg.AudioTemplateDir = filepath.Join(cfg.HostVoiceDir, "_templates")
	}
    cfg.VideoTemplateDir = getenv("VIDEO_TEMPLATE_DIR", "")
    if cfg.VideoTemplateDir == "" {
        cfg.VideoTemplateDir = filepath.Join(cfg.HostVideoDir, "_templates")
    }

    // 用户配置文件（宿主机JSON）
    cfg.UsersFile = getenv("USERS_FILE", "")
    if cfg.UsersFile == "" {
        cfg.UsersFile = filepath.Join(cfg.WorkDir, "users.json")
    }

	timeoutMinutes := 10
	if v := os.Getenv("AUTO_VIDEO_TIMEOUT_MINUTES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeoutMinutes = parsed
		}
	}
	cfg.VideoWaitTimeout = time.Duration(timeoutMinutes) * time.Minute

	mustMkdirAll(cfg.WorkDir)
	mustMkdirAll(cfg.HostVoiceDir)
	mustMkdirAll(cfg.HostVideoDir)
	mustMkdirAll(cfg.HostResultDir)
    mustMkdirAll(cfg.AudioTemplateDir)
    mustMkdirAll(cfg.VideoTemplateDir)

    return cfg
}

func mustMkdirAll(path string) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		log.Fatalf("mkdir %s failed: %v", path, err)
	}
}
