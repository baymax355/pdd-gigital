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
	DigitalPeopleDir  string
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
	// 默认仍指向共享盘，允许通过环境变量覆盖，方便本地开发调试
	sharedRoot := getenv("DIGITAL_PEOPLE_DIR", "/mnt/windows-digitalpeople")

	cfg := Config{
		Port:              getenv("APP_PORT", "8090"),
		DigitalPeopleDir:  sharedRoot,
		WorkDir:           getenv("APP_WORKDIR", filepath.Join(sharedRoot, "workdir")),
		StaticDir:         getenv("STATIC_DIR", ""),
		HostVoiceDir:      getenv("HOST_VOICE_DIR", filepath.Join(sharedRoot, "voice", "data")),
		HostVideoDir:      getenv("HOST_VIDEO_DIR", filepath.Join(sharedRoot, "face2face")),
		HostResultDir:     getenv("HOST_RESULT_DIR", filepath.Join(sharedRoot, "face2face", "result")),
		WindowsCompanyDir: getenv("WIN_COMPANY_DIR", sharedRoot),
		// 默认仍指向 compose 服务名，可通过环境变量改为具体 IP/域名
		TTSBaseURL:        getenv("TTS_BASE_URL", "http://heygem-tts:8080"),
		VideoBaseURL:      getenv("VIDEO_BASE_URL", "http://heygem-gen-video:8383"),
		GenVideoContainer: getenv("GEN_VIDEO_CONTAINER", "heygem-gen-video"),
		ContainerDataRoot: getenv("GEN_VIDEO_CONTAINER_DATA_ROOT", "/code/data"),
		// 队列/缓存保持原有默认，必要时自行修改环境变量
		RabbitURL:     getenv("RABBITMQ_URL", "amqp://root:pddrabitmq1041@192.168.7.240:5672"),
		QueuePrefix:   getenv("QUEUE_PREFIX", "digital_people"),
		RedisAddr:     getenv("REDIS_ADDR", "192.168.7.29:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
	}

	// 模板目录固定
	cfg.AudioTemplateDir = filepath.Join(cfg.HostVoiceDir, "_templates")
	cfg.VideoTemplateDir = filepath.Join(cfg.HostVideoDir, "_templates")

	// 用户配置文件（宿主机JSON）
	cfg.UsersFile = getenv("USERS_FILE", "")
	if cfg.UsersFile == "" {
		cfg.UsersFile = filepath.Join(cfg.WorkDir, "users.json")
	}

	timeoutMinutes := 15
	if v := os.Getenv("AUTO_VIDEO_TIMEOUT_MINUTES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeoutMinutes = parsed
		}
	}
	cfg.VideoWaitTimeout = time.Duration(timeoutMinutes) * time.Minute

	maybeMkdirAll(cfg.WorkDir)
	maybeMkdirAll(cfg.HostVoiceDir)
	maybeMkdirAll(cfg.HostVideoDir)
	maybeMkdirAll(cfg.HostResultDir)
	maybeMkdirAll(cfg.AudioTemplateDir)
	maybeMkdirAll(cfg.VideoTemplateDir)

	return cfg
}

func maybeMkdirAll(path string) {
	if path == "" {
		return
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		log.Fatalf("mkdir %s failed: %v", path, err)
	}
}
