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
    // 全部写死到共享盘 /mnt/windows-digitalpeople，避免环境变量造成路径不一致
    sharedRoot := "/mnt/windows-digitalpeople"

    cfg := Config{
        Port:              "8090",
        DigitalPeopleDir:  sharedRoot,
        WorkDir:           filepath.Join(sharedRoot, "workdir"),
        StaticDir:         getenv("STATIC_DIR", ""),
        HostVoiceDir:      filepath.Join(sharedRoot, "voice", "data"),
        HostVideoDir:      filepath.Join(sharedRoot, "face2face"),
        HostResultDir:     filepath.Join(sharedRoot, "face2face", "result"),
        WindowsCompanyDir: sharedRoot,
        // 上游服务地址写死为 compose 内服务名
        TTSBaseURL:        "http://heygem-tts:8080",
        VideoBaseURL:      "http://heygem-gen-video:8383",
        GenVideoContainer: "heygem-gen-video",
        ContainerDataRoot: "/code/data",
        // 队列/缓存保持原有默认，必要时自行修改源码
        RabbitURL:         getenv("RABBITMQ_URL", "amqp://root:pddrabitmq1041@192.168.7.240:5672"),
        QueuePrefix:       getenv("QUEUE_PREFIX", "digital_people"),
        RedisAddr:         getenv("REDIS_ADDR", "192.168.7.29:6379"),
        RedisPassword:     getenv("REDIS_PASSWORD", ""),
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
