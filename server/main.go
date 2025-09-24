package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
)

var (
	cfg             Config
	rabbitConn      *amqp.Connection
	rabbitChannel   *amqp.Channel
	rabbitQueueName string
	redisClient     *redis.Client
	autoTaskCounter atomic.Uint64
)

func nextAutoTaskID() string {
	seq := autoTaskCounter.Add(1)
	return fmt.Sprintf("auto-%d-%d", time.Now().UnixNano(), seq)
}

func main() {
	cfg = loadConfig()

	// 启动时打印关键配置信息，便于排查多实例/路径不一致问题
	if hn, _ := os.Hostname(); hn != "" {
		log.Printf("实例主机名: %s", hn)
	}
	log.Printf("配置摘要: Port=%s, WorkDir=%s", cfg.Port, cfg.WorkDir)
	log.Printf("目录映射: HostVoiceDir=%s, HostVideoDir=%s, HostResultDir=%s", cfg.HostVoiceDir, cfg.HostVideoDir, cfg.HostResultDir)
	log.Printf("模板目录: AudioTemplateDir=%s, VideoTemplateDir=%s", cfg.AudioTemplateDir, cfg.VideoTemplateDir)
	log.Printf("外部服务: TTSBaseURL=%s, VideoBaseURL=%s", cfg.TTSBaseURL, cfg.VideoBaseURL)
	log.Printf("队列与缓存: RabbitURL=%s, RedisAddr=%s, QueuePrefix=%s", cfg.RabbitURL, cfg.RedisAddr, cfg.QueuePrefix)
	log.Printf("视频容器: Name=%s, DataRoot=%s, WaitTimeout=%s", cfg.GenVideoContainer, cfg.ContainerDataRoot, cfg.VideoWaitTimeout)
	if err := initRedis(); err != nil {
		log.Fatalf("初始化 Redis 失败: %v", err)
	}
	if err := initRabbitMQ(); err != nil {
		log.Fatalf("初始化 RabbitMQ 失败: %v", err)
	}
	if err := loadUsers(); err != nil {
		log.Printf("加载用户文件失败: %v (将允许空用户列表)", err)
	} else {
		log.Printf("已加载用户列表: 来自 %s", cfg.UsersFile)
	}

	if err := ensureFFmpeg(); err != nil {
		log.Printf("警告: %v (请确保运行环境已安装 ffmpeg)", err)
	}

	r := gin.Default()

	// 直通封装：与 heygem.txt 相同路径，统一从本服务调用
	r.POST("/v1/preprocess_and_tran", handleProxyPreprocess)
	r.POST("/v1/invoke", handleProxyInvoke)
	r.POST("/easy/submit", handleProxySubmit)

	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
		// 认证相关
		api.GET("/auth/me", handleAuthMe)
		api.GET("/auth/users", handleAuthUsers)
		api.POST("/auth/login", handleAuthLogin)
		api.POST("/auth/logout", handleAuthLogout)
		api.GET("/files", handleListFiles)

		api.GET("/templates", handleTemplateList)
		api.POST("/templates/audio", handleUploadAudioTemplate)
		api.POST("/templates/video", handleUploadVideoTemplate)

		// 移除手动 TTS 与视频提交相关接口，统一走自动化流程

		api.POST("/auto/process", handleAutoProcess)
		api.GET("/auto/status/:taskId", handleAutoStatus)
		api.GET("/auto/tasks", handleAutoTasks)
		api.POST("/auto/tasks/:taskId/retry", handleAutoRetry)
		api.GET("/auto/archive", handleAutoArchive)

		api.GET("/download/video/:filename", handleDownloadVideo)
	}

	// 静态资源（如构建后的前端）- 使用更精确的路由避免冲突
	staticCandidates := []string{}
	if cfg.StaticDir != "" {
		staticCandidates = append(staticCandidates, cfg.StaticDir)
	}
	staticCandidates = append(staticCandidates, "./client/dist", "../client/dist")
	for _, p := range staticCandidates {
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			// 资产缓存策略：assets 长缓存、html 不缓存
			r.Use(func(c *gin.Context) {
				path := c.Request.URL.Path
				if strings.HasPrefix(path, "/assets/") {
					c.Writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
			})

			// 提供 /static 访问整包资源，兼容旧路径
			r.Static("/static", p)
			// 显式提供 /assets 资源，匹配 Vite 产物引用路径 /assets/*
			r.Static("/assets", filepath.Join(p, "assets"))
			// 常见静态文件
			r.StaticFile("/favicon.ico", filepath.Join(p, "favicon.ico"))
			r.StaticFile("/robots.txt", filepath.Join(p, "robots.txt"))
			// 根路径返回 index.html
			r.GET("/", func(c *gin.Context) {
				c.Writer.Header().Set("Cache-Control", "no-cache")
				c.File(filepath.Join(p, "index.html"))
			})
			// SPA 路由回退：非 API GET 请求兜底到 index.html
			r.NoRoute(func(c *gin.Context) {
				if c.Request.Method == http.MethodGet &&
					!strings.HasPrefix(c.Request.URL.Path, "/api/") &&
					!strings.HasPrefix(c.Request.URL.Path, "/v1/") {
					c.Writer.Header().Set("Cache-Control", "no-cache")
					c.File(filepath.Join(p, "index.html"))
					return
				}
				c.JSON(404, gin.H{"error": "not found"})
			})
			log.Printf("静态资源目录: %s", p)
			break
		}
	}

	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("服务启动于 %s", addr)
	startQueueWorker()
	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

// 直通封装：/v1/preprocess_and_tran -> TTS_BASE_URL/v1/preprocess_and_tran
func handleProxyPreprocess(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	url := fmt.Sprintf("%s/v1/preprocess_and_tran", cfg.TTSBaseURL)
	resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// 直通封装：/v1/invoke -> TTS_BASE_URL/v1/invoke（上游返回音频流，这里原样转发）
func handleProxyInvoke(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	url := fmt.Sprintf("%s/v1/invoke", cfg.TTSBaseURL)
	resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	// 透传上游 headers（含 Content-Type/Length/Disposition 等）
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// 直通封装：/easy/submit -> VIDEO_BASE_URL/easy/submit
func handleProxySubmit(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	url := fmt.Sprintf("%s/easy/submit", cfg.VideoBaseURL)
	resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		c.JSON(502, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

func handleListFiles(c *gin.Context) {
	dir := c.Query("dir")
	var root string
	switch dir {
	case "voice":
		root = cfg.HostVoiceDir
	case "video":
		root = cfg.HostVideoDir
	case "result":
		root = cfg.HostResultDir
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "dir 仅支持 voice|video|result"})
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	c.JSON(200, gin.H{"dir": root, "files": files})
}

// 已移除：手动上传音频处理（统一走自动化流程）
// func handleUploadAudio(c *gin.Context) { }

// /api/upload/video: multipart form: file, out_name(optional default zhuqi.mp4)
func handleUploadVideo(c *gin.Context) {
	ctx := c.Request.Context()
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少视频文件 file"})
		return
	}
	baseName := c.PostForm("out_name")
	if baseName == "" {
		baseName = "zhuqi.mp4"
	}
	srcPath, err := saveMultipartFile(file, filepath.Join(cfg.WorkDir, "upload"), baseName)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	silentPath := filepath.Join(cfg.WorkDir, "video", "silent.mp4")
	os.MkdirAll(filepath.Dir(silentPath), 0o755)
	_, stderr, err := run(ctx, "ffmpeg", "-y", "-i", srcPath, "-an", "-c:v", "copy", silentPath)
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("ffmpeg 静音失败: %v | %s", err, stderr)})
		return
	}

	// 拷贝到 face2face
	dst := filepath.Join(cfg.HostVideoDir, "silent.mp4")
	if err := copyFile(silentPath, dst); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"src": srcPath, "silent": silentPath, "copied_to": dst})
}

// 已移除：手动 TTS 预处理（统一走自动化流程）
// func handleTTSPreprocess(c *gin.Context) { }

// /api/tts/invoke: JSON expects fields used by upstream + output_speaker(optional defaults speaker)
func handleTTSInvoke(c *gin.Context) {
	var req TTSInvokeReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "无效的 JSON"})
		return
	}
	if req.Speaker == "" {
		req.Speaker = "demo001"
	}
	if req.Format == "" {
		req.Format = "wav"
	}
	if req.TopP == 0 {
		req.TopP = 0.7
	}
	if req.MaxNewTokens == 0 {
		req.MaxNewTokens = 1024
	}
	if req.ChunkLength == 0 {
		req.ChunkLength = 100
	}
	if req.RepetitionPenalty == 0 {
		req.RepetitionPenalty = 1.2
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}

	body, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/v1/invoke", cfg.TTSBaseURL)
	resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		c.JSON(resp.StatusCode, gin.H{"error": string(b)})
		return
	}
	// 保存为 voice/data/speaker.wav
	outVoice := filepath.Join(cfg.HostVoiceDir, sanitizeFilename(req.Speaker)+".wav")
	f, err := os.Create(outVoice)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	f.Close()

	// 复制到视频目录，便于后续 /easy/submit 使用
	outInVideo := filepath.Join(cfg.HostVideoDir, filepath.Base(outVoice))
	if err := copyFile(outVoice, outInVideo); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"voice": outVoice, "copied_to_video_dir": outInVideo})
}

// /api/video/submit: JSON {audio_filename, video_filename, code, chaofen, watermark_switch, pn}
func handleVideoSubmit(c *gin.Context) {
	var req SubmitVideoReq
	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "无效的 JSON"})
		return
	}
	if req.AudioFilename == "" {
		req.AudioFilename = "demo001.wav"
	}
	if req.VideoFilename == "" {
		req.VideoFilename = "silent.mp4"
	}
	if req.Code == "" {
		req.Code = fmt.Sprintf("task-%d", time.Now().Unix())
	}

	// 确保文件存在于视频目录（容器应把该目录挂载为 /code/data）
	if _, err := os.Stat(filepath.Join(cfg.HostVideoDir, req.AudioFilename)); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("音频不存在: %v", err)})
		return
	}
	if _, err := os.Stat(filepath.Join(cfg.HostVideoDir, req.VideoFilename)); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("视频不存在: %v", err)})
		return
	}

	payload := map[string]any{
		"audio_url":        filepath.Join(cfg.ContainerDataRoot, req.AudioFilename),
		"video_url":        filepath.Join(cfg.ContainerDataRoot, req.VideoFilename),
		"code":             req.Code,
		"chaofen":          req.Chaofen,
		"watermark_switch": req.WatermarkSwitch,
		"pn":               req.PN,
	}
	if _, ok := payload["chaofen"].(int); !ok || req.Chaofen == 0 {
		payload["chaofen"] = 0
	}
	if _, ok := payload["watermark_switch"].(int); !ok || req.WatermarkSwitch == 0 {
		payload["watermark_switch"] = 0
	}
	if _, ok := payload["pn"].(int); !ok || req.PN == 0 {
		payload["pn"] = 1
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/easy/submit", cfg.VideoBaseURL)
	resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	c.JSON(resp.StatusCode, gin.H{"submit_payload": payload, "upstream_status": resp.StatusCode, "upstream_body": string(b)})
}

// /api/video/result?code=task001&copy_to_company=1
func handleVideoResult(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(400, gin.H{"error": "缺少 code"})
		return
	}
	copyCompany := c.Query("copy_to_company") == "1"

	hostOut := filepath.Join(cfg.HostResultDir, fmt.Sprintf("%s-r.mp4", code))
	if st, err := os.Stat(hostOut); err == nil && !st.IsDir() && st.Size() > 0 {
		companyOut := ""
		if copyCompany {
			companyOut = filepath.Join(cfg.WindowsCompanyDir, fmt.Sprintf("%s-r.mp4", code))
			if err := copyFile(hostOut, companyOut); err != nil {
				c.JSON(500, gin.H{"error": fmt.Sprintf("复制到 Windows 目录失败: %v", err)})
				return
			}
		}

		c.JSON(200, gin.H{"result": hostOut, "copied_to_company": companyOut})
		return
	}

	container := cfg.GenVideoContainer
	inside := filepath.Join(cfg.ContainerDataRoot, "temp", fmt.Sprintf("%s-r.mp4", code))
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, rmErr, rmRunErr := run(cleanupCtx, "docker", "exec", "-i", container, "bash", "-lc", fmt.Sprintf("rm -f '%s'", inside)); rmRunErr != nil {
			log.Printf("清理容器文件失败: %v | %s", rmRunErr, rmErr)
		}
	}()

	// docker exec 检查文件是否存在
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	_, stderr, err := run(ctx, "docker", "exec", "-i", container, "bash", "-lc", fmt.Sprintf("test -f '%s' && echo FOUND || echo MISSING", inside))
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("docker exec 失败: %v | %s", err, stderr)})
		return
	}
	status := strings.TrimSpace(stderr)
	// 注意: 我们将 stdout/stderr 视为总输出，部分 docker 版本输出到 stderr，这里更保险再测一次
	out, outErr, err := run(ctx, "docker", "exec", "-i", container, "bash", "-lc", fmt.Sprintf("if [ -f '%s' ]; then echo FOUND; else echo MISSING; fi", inside))
	if err != nil {
		_ = status
		c.JSON(500, gin.H{"error": fmt.Sprintf("docker exec 检测失败: %v | %s", err, outErr)})
		return
	}
	if !strings.Contains(out, "FOUND") {
		c.JSON(404, gin.H{"error": "生成文件未就绪", "path": inside})
		return
	}

	// docker cp 拷贝到 HostResultDir
	if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_, cpErr, err := run(ctx, "docker", "cp", fmt.Sprintf("%s:%s", container, inside), hostOut)
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("docker cp 失败: %v | %s", err, cpErr)})
		return
	}

	companyOut := ""
	if copyCompany {
		companyOut = filepath.Join(cfg.WindowsCompanyDir, fmt.Sprintf("%s-r.mp4", code))
		if err := copyFile(hostOut, companyOut); err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("复制到 Windows 目录失败: %v", err)})
			return
		}
	}

	c.JSON(200, gin.H{"result": hostOut, "copied_to_company": companyOut})
}

// 全局任务状态存储
var (
	taskStatusMu  sync.RWMutex
	taskStatusMap = make(map[string]*AutoProcessStatus)
)

type queuedTask struct {
	TaskID    string         `json:"task_id"`
	AudioPath string         `json:"audio_path"`
	VideoPath string         `json:"video_path"`
	Req       AutoProcessReq `json:"req"`
}

var queueStarted bool

func initRedis() error {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("连接 Redis 失败: %w", err)
	}
	return nil
}

func initRabbitMQ() error {
	conn, err := amqp.Dial(cfg.RabbitURL)
	if err != nil {
		return fmt.Errorf("连接 RabbitMQ 失败: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("创建 RabbitMQ channel 失败: %w", err)
	}
	queueName := fmt.Sprintf("%s_tasks", cfg.QueuePrefix)
	if _, err := ch.QueueDeclare(queueName, true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("声明 RabbitMQ 队列失败: %w", err)
	}
	if err := ch.Qos(1, 0, false); err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("设置 RabbitMQ Qos 失败: %w", err)
	}
	rabbitConn = conn
	rabbitChannel = ch
	rabbitQueueName = queueName
	return nil
}

func redisTaskStatusKey(taskID string) string {
	return fmt.Sprintf("%s:task:%s", cfg.QueuePrefix, taskID)
}

func redisTaskIndexKey() string {
	return fmt.Sprintf("%s:task_ids", cfg.QueuePrefix)
}

func persistTaskStatus(status *AutoProcessStatus) {
	if status == nil || redisClient == nil {
		return
	}
	data, err := json.Marshal(status)
	if err != nil {
		log.Printf("序列化任务状态失败: %v", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Set(ctx, redisTaskStatusKey(status.TaskID), data, 0).Err(); err != nil {
		log.Printf("写入 Redis 任务状态失败: %v", err)
	}
}

func loadTaskStatus(taskID string) (*AutoProcessStatus, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := redisClient.Get(ctx, redisTaskStatusKey(taskID)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var status AutoProcessStatus
	if err := json.Unmarshal(res, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func addTaskToIndex(taskID string, start int64) {
	if redisClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.ZAdd(ctx, redisTaskIndexKey(), redis.Z{
		Score:  float64(start),
		Member: taskID,
	}).Err(); err != nil {
		log.Printf("写入任务索引失败: %v", err)
	}
}

func listTaskStatuses() ([]*AutoProcessStatus, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ids, err := redisClient.ZRevRange(ctx, redisTaskIndexKey(), 0, -1).Result()
	if err != nil {
		return nil, err
	}
	result := make([]*AutoProcessStatus, 0, len(ids))
	for _, id := range ids {
		status, err := loadTaskStatus(id)
		if err != nil {
			log.Printf("读取任务状态失败(%s): %v", id, err)
			continue
		}
		if status != nil {
			result = append(result, status)
		}
	}
	return result, nil
}

func publishTask(t queuedTask) error {
	if rabbitChannel == nil {
		return fmt.Errorf("RabbitMQ 未初始化")
	}
	body, err := json.Marshal(t)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = rabbitChannel.PublishWithContext(ctx, "", rabbitQueueName, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
	if err != nil {
		log.Printf("[队列] 发布失败: id=%s err=%v", t.TaskID, err)
		return err
	}
	log.Printf("[队列] 发布成功: id=%s, audio=%s, video=%s", t.TaskID, t.AudioPath, t.VideoPath)
	return nil
}

func getOrCreateTaskStatus(taskID string) *AutoProcessStatus {
	taskStatusMu.RLock()
	status, ok := taskStatusMap[taskID]
	taskStatusMu.RUnlock()
	if ok {
		return status
	}
	status, err := loadTaskStatus(taskID)
	if err != nil {
		log.Printf("从 Redis 加载任务状态失败(%s): %v", taskID, err)
	}
	if status == nil {
		status = &AutoProcessStatus{TaskID: taskID}
	}
	taskStatusMu.Lock()
	taskStatusMap[taskID] = status
	taskStatusMu.Unlock()
	return status
}

func startQueueWorker() {
	if queueStarted {
		return
	}
	queueStarted = true
	go func() {
		for {
			if rabbitChannel == nil {
				log.Printf("RabbitMQ 通道未就绪，3 秒后重试...")
				time.Sleep(3 * time.Second)
				continue
			}
			msgs, err := rabbitChannel.Consume(rabbitQueueName, "", false, false, false, false, nil)
			if err != nil {
				log.Printf("RabbitMQ 消费初始化失败: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}
			log.Printf("任务队列工作线程已启动，监听队列=%s", rabbitQueueName)
			for msg := range msgs {
				var t queuedTask
				if err := json.Unmarshal(msg.Body, &t); err != nil {
					log.Printf("解析任务消息失败: %v", err)
					msg.Nack(false, false)
					continue
				}
				status := getOrCreateTaskStatus(t.TaskID)
				if status.StartTime == 0 {
					status.StartTime = time.Now().Unix()
				}
				status.Status = "processing"
				status.CurrentStep = "排队完成，开始处理"
				if status.Progress < 5 {
					status.Progress = 5
				}
				persistTaskStatus(status)
				processAutomatically(context.Background(), t.TaskID, t.AudioPath, t.VideoPath, t.Req)
				if err := msg.Ack(false); err != nil {
					log.Printf("确认 RabbitMQ 消息失败: %v", err)
				}
			}
			log.Printf("RabbitMQ 消费通道已关闭，5 秒后重试...")
			time.Sleep(5 * time.Second)
		}
	}()
}

// /api/auto/process: 全自动化处理接口
func handleAutoProcess(c *gin.Context) {
	// 登录校验
	loginUser := usernameFromContext(c)
	if loginUser == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}
	req := AutoProcessReq{
		Speaker:           c.PostForm("speaker"),
		Text:              c.PostForm("text"),
		CopyToCompany:     parseBool(c.PostForm("copy_to_company")),
		UseTTS:            true,
		AudioTemplateName: strings.TrimSpace(c.PostForm("audio_template_name")),
		VideoTemplateName: strings.TrimSpace(c.PostForm("video_template_name")),
	}
	if v := c.PostForm("use_tts"); v != "" {
		req.UseTTS = parseBool(v)
	}

	// 不再需要任务名称，统一使用任务ID标识本次任务
	req.TaskName = ""

	var (
		audioFile *multipart.FileHeader
		videoFile *multipart.FileHeader
		err       error
	)

	var audioTemplatePath string
	if req.AudioTemplateName != "" {
		_, path, err := findTemplateItem(templateKindAudio, req.AudioTemplateName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("音频模版无效: %v", err)})
			return
		}
		audioTemplatePath = path
	} else {
		audioFile, err = c.FormFile("audio")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少音频文件或模版"})
			return
		}
	}

	var videoTemplatePath string
	if req.VideoTemplateName != "" {
		_, path, err := findTemplateItem(templateKindVideo, req.VideoTemplateName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("视频模版无效: %v", err)})
			return
		}
		videoTemplatePath = path
	} else {
		videoFile, err = c.FormFile("video")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "缺少视频文件或模版"})
			return
		}
	}

	log.Printf("解析的请求参数: TaskName=%s, Speaker=%s, TextLen=%d, CopyToCompany=%v, UseTTS=%v, AudioTemplate=%s, VideoTemplate=%s", req.TaskName, req.Speaker, len([]rune(req.Text)), req.CopyToCompany, req.UseTTS, req.AudioTemplateName, req.VideoTemplateName)

	taskID := nextAutoTaskID()
	status := &AutoProcessStatus{
		TaskID:      taskID,
		TaskName:    "",
		Username:    loginUser,
		Status:      "processing",
		CurrentStep: "上传文件",
		Progress:    0,
		StartTime:   time.Now().Unix(),
	}
	status.Request = &req
	taskStatusMu.Lock()
	taskStatusMap[taskID] = status
	taskStatusMu.Unlock()
	addTaskToIndex(taskID, status.StartTime)
	persistTaskStatus(status)

	var audioPath string
	if audioTemplatePath != "" {
		audioPath = audioTemplatePath
		log.Printf("使用音频模版: %s", audioPath)
	} else {
		log.Printf("开始保存音频文件: %s", audioFile.Filename)
		uniqueAudioName := fmt.Sprintf("ref-%s%s", sanitizeFilename(taskID), strings.ToLower(filepath.Ext(audioFile.Filename)))
		audioPath, err = saveMultipartFile(audioFile, filepath.Join(cfg.WorkDir, "upload"), uniqueAudioName)
		if err != nil {
			log.Printf("音频保存失败: %v", err)
			status.Status = "failed"
			status.Error = fmt.Sprintf("音频上传失败: %v", err)
			persistTaskStatus(status)
			c.JSON(500, gin.H{"error": status.Error})
			return
		}
		log.Printf("音频文件保存成功: %s", audioPath)
	}
	status.AudioPath = audioPath

	var videoPath string
	if videoTemplatePath != "" {
		videoPath = videoTemplatePath
		log.Printf("使用视频模版: %s", videoPath)
	} else {
		log.Printf("开始保存视频文件: %s", videoFile.Filename)
		uniqueVideoName := fmt.Sprintf("video-%s%s", sanitizeFilename(taskID), strings.ToLower(filepath.Ext(videoFile.Filename)))
		videoPath, err = saveMultipartFile(videoFile, filepath.Join(cfg.WorkDir, "upload"), uniqueVideoName)
		if err != nil {
			log.Printf("视频保存失败: %v", err)
			status.Status = "failed"
			status.Error = fmt.Sprintf("视频上传失败: %v", err)
			persistTaskStatus(status)
			c.JSON(500, gin.H{"error": status.Error})
			return
		}
		log.Printf("视频文件保存成功: %s", videoPath)
	}
	status.VideoPath = videoPath
	persistTaskStatus(status)

	status.Status = "queued"
	status.CurrentStep = "等待排队执行"
	persistTaskStatus(status)
	if err := publishTask(queuedTask{TaskID: taskID, AudioPath: audioPath, VideoPath: videoPath, Req: req}); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("任务入队失败: %v", err)
		persistTaskStatus(status)
		c.JSON(503, gin.H{"error": status.Error})
		return
	}
	log.Printf("任务已入队: task_id=%s, audio=%s, video=%s, use_tts=%v", taskID, audioPath, videoPath, req.UseTTS)

	c.JSON(200, gin.H{"task_id": taskID, "status": "started"})
}

func handleUploadAudioTemplate(c *gin.Context) {
	uploadTemplateWithName(c, templateKindAudio)
}

func handleUploadVideoTemplate(c *gin.Context) {
	uploadTemplateWithName(c, templateKindVideo)
}

func uploadTemplateWithName(c *gin.Context, kind string) {
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = strings.TrimSpace(c.PostForm("template_name"))
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少模版文件 file"})
		return
	}

	log.Printf("[模板上传] 开始: kind=%s, name(raw)=%s, file=%s(size=%d)", kind, name, file.Filename, file.Size)

	originalBase := strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))
	if name == "" {
		name = originalBase
	}

	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = originalBase
	}

	sanitized := sanitizeTemplateKey(name)
	log.Printf("[模板上传] 规范化名称: sanitized=%s (display=%s)", sanitized, displayName)
	if err := ensureTemplateKindDir(kind); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tmpDir := filepath.Join(cfg.WorkDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	tmpName := sanitized + filepath.Ext(file.Filename)
	tmpPath, err := saveMultipartFile(file, tmpDir, tmpName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("模版暂存失败: %v", err)})
		return
	}
	defer os.Remove(tmpPath)
	log.Printf("[模板上传] 暂存完成: tmp=%s", tmpPath)

	finalPath, err := templateFilePath(kind, sanitized)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("[模板上传] 目标路径: final=%s", finalPath)

	ctx := c.Request.Context()
	switch kind {
	case templateKindAudio:
		log.Printf("[模板上传] 音频转换命令: ffmpeg -y -i %s -ar 16000 -ac 1 -c:a pcm_s16le %s", tmpPath, finalPath)
		_, stderr, err := run(ctx, "ffmpeg", "-y", "-i", tmpPath,
			"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", finalPath)
		if err != nil {
			log.Printf("[模板上传] 音频转换失败: %v | %s", err, stderr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("音频模版转换失败: %v | %s", err, stderr)})
			return
		}
		if st, e := os.Stat(finalPath); e == nil {
			log.Printf("[模板上传] 音频转换成功: size=%d bytes", st.Size())
		}
	case templateKindVideo:
		log.Printf("[模板上传] 视频快速转封装: ffmpeg -y -i %s -c:v copy -c:a copy %s", tmpPath, finalPath)
		_, stderr, err := run(ctx, "ffmpeg", "-y", "-i", tmpPath, "-c:v", "copy", "-c:a", "copy", finalPath)
		if err != nil {
			log.Printf("[模板上传] 快速转封装失败，尝试重新编码: %v | %s", err, stderr)
			_, stderr, err = run(ctx, "ffmpeg", "-y", "-i", tmpPath,
				"-c:v", "libx264", "-preset", "veryfast", "-crf", "20",
				"-c:a", "aac", "-b:a", "192k", finalPath)
			if err != nil {
				log.Printf("[模板上传] 视频重编码失败: %v | %s", err, stderr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("视频模版转换失败: %v | %s", err, stderr)})
				return
			}
		}
		if st, e := os.Stat(finalPath); e == nil {
			log.Printf("[模板上传] 视频转换成功: size=%d bytes", st.Size())
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的模版类型"})
		return
	}

	item := TemplateItem{
		Name:         sanitized,
		DisplayName:  displayName,
		OriginalName: file.Filename,
		Kind:         kind,
		UpdatedAt:    time.Now().Unix(),
	}
	if err := upsertTemplateItem(kind, item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("模版信息保存失败: %v", err)})
		return
	}
	log.Printf("[模板上传] 索引已更新: kind=%s, name=%s -> %s", kind, item.Name, finalPath)

	c.JSON(200, gin.H{"message": "模版已更新", "template": item})
}

func handleTemplateList(c *gin.Context) {
	kind := strings.TrimSpace(c.Query("kind"))
	switch kind {
	case "", templateKindAudio, templateKindVideo:
		// ok
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "kind 仅支持 audio|video"})
		return
	}

	if kind == "" {
		audio, err := listTemplates(templateKindAudio)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		video, err := listTemplates(templateKindVideo)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"audio": audio, "video": video})
		return
	}

	items, err := listTemplates(kind)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{kind: items})
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// 异步自动化处理函数
func processAutomatically(ctx context.Context, taskID string, audioPath, videoPath string, req AutoProcessReq) {
	status := getOrCreateTaskStatus(taskID)
	// 任务名称不再使用，沿用空字符串；对外统一使用 taskID
	var body []byte
	var url string
	var resp *http.Response
	cleanupPaths := make(map[string]struct{})
	var cleanupFuncs []func()
	addCleanupPath := func(path string) {
		if path == "" {
			return
		}
		cleanupPaths[path] = struct{}{}
	}
	addCleanupFunc := func(fn func()) {
		if fn == nil {
			return
		}
		cleanupFuncs = append(cleanupFuncs, fn)
	}
	defer func() {
		for _, fn := range cleanupFuncs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("执行清理回调时发生异常: %v", r)
					}
				}()
				fn()
			}()
		}
		for path := range cleanupPaths {
			if err := removeFileIfExists(path); err != nil {
				log.Printf("清理临时文件失败: %s (%v)", path, err)
			}
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("处理异常: %v", r)
			status.Progress = 0
		}
		// 确保失败场景记录结束时间与耗时
		if status.Status == "failed" && status.EndTime == 0 {
			status.EndTime = time.Now().Unix()
			status.TotalDuration = status.EndTime - status.StartTime
		}
		persistTaskStatus(status)
	}()

	// 创建新的上下文，避免HTTP请求上下文被取消
	processCtx := context.Background()

	// 步骤1: 处理音频 (10%)
	status.CurrentStep = "处理音频文件"
	status.Progress = 10
	persistTaskStatus(status)

	// 直接转换音频格式: MP3/其他格式 -> WAV (16kHz单声道)
	work := filepath.Join(cfg.WorkDir, "audio")
	os.MkdirAll(work, 0o755)
	tempSlug := fmt.Sprintf("%s_temp", sanitizeFilename(taskID))
	srcLocal := filepath.Join(work, fmt.Sprintf("%s_src.wav", tempSlug))
	norm := filepath.Join(work, fmt.Sprintf("%s_norm.wav", tempSlug))

	log.Printf("自动任务[%s] 音频准备: initialAudioPath=%s templateName=%s", taskID, audioPath, req.AudioTemplateName)
	currentAudioPath := audioPath
	if _, statErr := os.Stat(currentAudioPath); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) && req.AudioTemplateName != "" {
			log.Printf("自动任务[%s] 检测到音频缺失，尝试按模版恢复: missingPath=%s", taskID, currentAudioPath)
			log.Printf("自动任务[%s] 重新定位音频模版: name=%s", taskID, req.AudioTemplateName)
			if _, retryPath, retryErr := findTemplateItem(templateKindAudio, req.AudioTemplateName); retryErr == nil {
				log.Printf("自动任务[%s] 模版重载成功: newAudioPath=%s", taskID, retryPath)
				currentAudioPath = retryPath
				status.AudioPath = currentAudioPath
				persistTaskStatus(status)
			} else {
				log.Printf("自动任务[%s] 模版重载失败: name=%s err=%v", taskID, req.AudioTemplateName, retryErr)
				status.Status = "failed"
				status.Error = fmt.Sprintf("音频模板缺失且恢复失败: %v", retryErr)
				return
			}
		} else {
			log.Printf("自动任务[%s] 音频文件不可用: path=%s err=%v", taskID, currentAudioPath, statErr)
			status.Status = "failed"
			status.Error = fmt.Sprintf("音频文件不可用: %v", statErr)
			return
		}
	}
	// 共享盘可能存在可见性延迟，这里做短暂重试确保可读
	if err := waitFileReadable(currentAudioPath, 5, 200*time.Millisecond); err != nil {
		log.Printf("自动任务[%s] 音频可读性检查失败，尝试再次按模版名定位: path=%s err=%v", taskID, currentAudioPath, err)
		if req.AudioTemplateName != "" {
			if _, retryPath, retryErr := findTemplateItem(templateKindAudio, req.AudioTemplateName); retryErr == nil {
				currentAudioPath = retryPath
				status.AudioPath = currentAudioPath
				persistTaskStatus(status)
				// 再次等待
				if e2 := waitFileReadable(currentAudioPath, 5, 200*time.Millisecond); e2 != nil {
					status.Status = "failed"
					status.Error = fmt.Sprintf("音频文件不可读: %v", e2)
					return
				}
			} else {
				status.Status = "failed"
				status.Error = fmt.Sprintf("音频文件不可读: %v", retryErr)
				return
			}
		} else {
			status.Status = "failed"
			status.Error = fmt.Sprintf("音频文件不可读: %v", err)
			return
		}
	}

	// 为避免直接从共享盘读取不稳定，先复制一份到本地工作目录（任务专属文件）
	log.Printf("自动任务[%s] 复制模板音频到本地: src=%s dst=%s", taskID, currentAudioPath, srcLocal)
	if err := copyFileWithRetries(currentAudioPath, srcLocal, 5, 200*time.Millisecond); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("音频拷贝到工作目录失败: %v", err)
		return
	}

	audioPath = srcLocal
	log.Printf("自动任务[%s] 音频转换开始: src=%s dst=%s", taskID, audioPath, norm)
	// 简单的格式转换，不做任何音频处理
	_, stderr, err := run(processCtx, "ffmpeg", "-y", "-i", audioPath,
		"-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", norm,
	)
	if err != nil {
		log.Printf("自动任务[%s] 音频转换失败: src=%s err=%v stderr=%s", taskID, audioPath, err, stderr)
		status.Status = "failed"
		status.Error = fmt.Sprintf("音频格式转换失败: %v | %s", err, stderr)
		return
	}
	log.Printf("自动任务[%s] 音频转换完成: output=%s", taskID, norm)

	// 拷贝到 voice/data 目录（任务专属名）
	dst := filepath.Join(cfg.HostVoiceDir, fmt.Sprintf("%s_norm.wav", tempSlug))
	log.Printf("自动任务[%s] 拷贝音频到语音目录: src=%s dst=%s", taskID, norm, dst)
	if err := copyFile(norm, dst); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("音频拷贝失败: %v", err)
		return
	}
	// 同步拷贝到视频目录（任务专属名），便于“自带音频”链路直接使用
	audioInVideo := fmt.Sprintf("%s_norm.wav", tempSlug)
	dstInVideo := filepath.Join(cfg.HostVideoDir, audioInVideo)
	log.Printf("自动任务[%s] 拷贝音频到视频目录: src=%s dst=%s", taskID, norm, dstInVideo)
	if err := copyFile(norm, dstInVideo); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("音频拷贝到视频目录失败: %v", err)
		return
	}
	// 任务结束时清理临时音频
	addCleanupPath(dst)
	addCleanupPath(dstInVideo)
	log.Printf("自动任务[%s] 音频拷贝完成", taskID)

	// 步骤2: 处理视频 (20%)
	status.CurrentStep = "处理视频文件"
	status.Progress = 20
	persistTaskStatus(status)

	log.Printf("自动任务[%s] 视频处理开始: src=%s", taskID, videoPath)
	// 若视频路径不存在，尝试按模板名重新定位
	if _, statErr := os.Stat(videoPath); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) && req.VideoTemplateName != "" {
			log.Printf("自动任务[%s] 检测到视频缺失，尝试按模版恢复: missingPath=%s, name=%s", taskID, videoPath, req.VideoTemplateName)
			if _, retryPath, retryErr := findTemplateItem(templateKindVideo, req.VideoTemplateName); retryErr == nil {
				log.Printf("自动任务[%s] 视频模版重载成功: newVideoPath=%s", taskID, retryPath)
				videoPath = retryPath
				status.VideoPath = videoPath
				persistTaskStatus(status)
			} else {
				log.Printf("自动任务[%s] 视频模版重载失败: name=%s err=%v", taskID, req.VideoTemplateName, retryErr)
				status.Status = "failed"
				status.Error = fmt.Sprintf("视频模板缺失且恢复失败: %v", retryErr)
				return
			}
		} else {
			log.Printf("自动任务[%s] 视频文件不可用: path=%s err=%v", taskID, videoPath, statErr)
			status.Status = "failed"
			status.Error = fmt.Sprintf("视频文件不可用: %v", statErr)
			return
		}
	}
	// 视频静音处理
	silentPath := filepath.Join(cfg.WorkDir, "video", fmt.Sprintf("%s_silent.mp4", tempSlug))
	os.MkdirAll(filepath.Dir(silentPath), 0o755)
	log.Printf("自动任务[%s] 视频静音命令: src=%s dst=%s", taskID, videoPath, silentPath)
	_, stderr, err = run(processCtx, "ffmpeg", "-y", "-i", videoPath, "-an", "-c:v", "copy", silentPath)
	if err != nil {
		log.Printf("自动任务[%s] 视频静音失败: src=%s err=%v stderr=%s", taskID, videoPath, err, stderr)
		status.Status = "failed"
		status.Error = fmt.Sprintf("视频静音失败: %v | %s", err, stderr)
		return
	}
	log.Printf("自动任务[%s] 视频静音完成: output=%s", taskID, silentPath)

	// 拷贝到 face2face 目录（任务专属名）
	videoSilentName := fmt.Sprintf("%s_silent.mp4", tempSlug)
	dstVideo := filepath.Join(cfg.HostVideoDir, videoSilentName)
	log.Printf("自动任务[%s] 拷贝视频到目标目录: src=%s dst=%s", taskID, silentPath, dstVideo)
	if err := copyFile(silentPath, dstVideo); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("视频拷贝失败: %v", err)
		return
	}
	addCleanupPath(dstVideo)
	log.Printf("自动任务[%s] 视频拷贝完成", taskID)

	// 如果使用自带音频，跳过 TTS 流程（稍后直接用任务专属 ref_norm 作为合成音频）
	// 否则执行 TTS 预处理 + 合成
	audioForVideo := audioInVideo // 默认自带音频文件名（已带任务ID）
	if req.UseTTS {
		// 步骤3: TTS预处理 (30%)
		status.CurrentStep = "TTS预处理"
		status.Progress = 30
		persistTaskStatus(status)

		preprocessReq := PreprocessReq{Format: "wav", ReferenceAudio: fmt.Sprintf("%s_norm.wav", tempSlug), Lang: "zh"}

		body, _ := json.Marshal(preprocessReq)
		url := fmt.Sprintf("%s/v1/preprocess_and_tran", cfg.TTSBaseURL)
		resp, err := httpJSON(processCtx, http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
		if err != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS预处理失败: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS预处理失败: %s", string(b))
			return
		}

		var preResp PreprocessResp
		if err := json.NewDecoder(resp.Body).Decode(&preResp); err != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS预处理解析失败: %v", err)
			return
		}
		// 预处理可能以 HTTP 200 + code != 0 的方式返回失败，需要显式拦截
		if preResp.Code != 0 || preResp.ASRFormatAudioURL == "" || preResp.ReferenceAudioText == "" {
			status.Status = "failed"
			// 将上游 msg 透出，便于定位（典型：asr failed）
			status.Error = fmt.Sprintf("TTS预处理失败: code=%d, msg=%s", preResp.Code, preResp.Msg)
			return
		}

		log.Printf("TTS预处理响应: ReferenceAudio=%s, ReferenceText=%s", preResp.ASRFormatAudioURL, preResp.ReferenceAudioText)

		// 步骤4: TTS合成 (50%)
		status.CurrentStep = "TTS语音合成"
		status.Progress = 50
		persistTaskStatus(status)

		if req.Speaker == "" {
			req.Speaker = "demo001"
		}

		// 使用map构建请求，避免结构体问题
		ttsReq := map[string]interface{}{
			"speaker":            req.Speaker,
			"text":               req.Text,
			"format":             "wav",
			"topP":               0.7,
			"max_new_tokens":     1024,
			"chunk_length":       100,
			"repetition_penalty": 1.2,
			"temperature":        0.7,
			"need_asr":           false,
			"streaming":          false,
			"is_fixed_seed":      0,
			"is_norm":            0,
			"reference_audio":    preResp.ASRFormatAudioURL,
			"reference_text":     preResp.ReferenceAudioText,
		}

		body, _ = json.Marshal(ttsReq)
		log.Printf("TTS请求内容: %s", string(body))
		url = fmt.Sprintf("%s/v1/invoke", cfg.TTSBaseURL)
		resp, err = httpJSON(processCtx, http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
		if err != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS合成失败: %v", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS合成失败: %s", string(b))
			return
		}

		// 保存TTS生成的音频（任务专属名，避免并发覆盖）
		outVoiceBase := fmt.Sprintf("%s-%s.wav", sanitizeFilename(req.Speaker), sanitizeFilename(taskID))
		outVoice := filepath.Join(cfg.HostVoiceDir, outVoiceBase)
		f, err := os.Create(outVoice)
		if err != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS音频保存失败: %v", err)
			return
		}
		if _, err := io.Copy(f, resp.Body); err != nil {
			f.Close()
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS音频写入失败: %v", err)
			return
		}
		f.Close()

		// 复制到视频目录
		outInVideo := filepath.Join(cfg.HostVideoDir, filepath.Base(outVoice))
		if err := copyFile(outVoice, outInVideo); err != nil {
			status.Status = "failed"
			status.Error = fmt.Sprintf("TTS音频拷贝失败: %v", err)
			return
		}
		// 设置将要用于视频合成的音频文件名（容器内路径拼接时只需要文件名）
		audioForVideo = filepath.Base(outVoice)
		// 清理任务结束后不再需要的临时音频
		addCleanupPath(outVoice)
		addCleanupPath(outInVideo)
	}

	// 步骤5: 视频合成提交 (70%)
	status.CurrentStep = "提交视频合成任务"
	status.Progress = 70
	persistTaskStatus(status)

	// 统一使用任务ID作为视频生成 code 与最终结果文件名
	taskCode := taskID
	containerResultName := fmt.Sprintf("%s-r.mp4", taskCode)
	resultFilename := fmt.Sprintf("%s.mp4", taskID)
	containerResultPath := filepath.Join(cfg.ContainerDataRoot, "temp", containerResultName)
	hostTempVideoPath := filepath.Join(cfg.HostVideoDir, "temp", containerResultName)
	addCleanupPath(hostTempVideoPath)
	addCleanupFunc(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, rmErr, rmRunErr := run(cleanupCtx, "docker", "exec", "-i", cfg.GenVideoContainer, "bash", "-lc", fmt.Sprintf("rm -f '%s'", containerResultPath)); rmRunErr != nil {
			log.Printf("清理容器内临时文件失败: %v | %s", rmRunErr, rmErr)
		}
	})
	payload := map[string]any{
		"audio_url":        filepath.Join(cfg.ContainerDataRoot, audioForVideo),
		"video_url":        filepath.Join(cfg.ContainerDataRoot, videoSilentName),
		"code":             taskCode,
		"chaofen":          0,
		"watermark_switch": 0,
		"pn":               1,
	}

	body, _ = json.Marshal(payload)
	url = fmt.Sprintf("%s/easy/submit", cfg.VideoBaseURL)
	resp, err = httpJSON(processCtx, http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
	if err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("视频合成提交失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		status.Status = "failed"
		status.Error = fmt.Sprintf("视频合成提交失败: %s", string(b))
		return
	}

	// 步骤6: 轮询视频合成结果 (70-100%)
	status.CurrentStep = "等待视频合成完成"
	status.Progress = 80
	persistTaskStatus(status)

	// 开始轮询，间隔30秒
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	maxWait := cfg.VideoWaitTimeout
	if maxWait <= 0 {
		maxWait = 15 * time.Minute
	}
	timeout := time.After(maxWait)
	checkCount := 0

	for {
		select {
		case <-ticker.C:
			checkCount++
			// 检查视频是否生成完成
			inside := containerResultPath

			// 更新状态信息
			status.CurrentStep = fmt.Sprintf("等待视频合成完成 (已检查 %d 次，约 %d 分钟，最多等待约 %.1f 分钟)", checkCount, checkCount/2, maxWait.Minutes())
			persistTaskStatus(status)

			// 优先通过宿主机挂载目录检测结果文件，避免依赖 docker 命令
			hostOut := filepath.Join(cfg.HostResultDir, resultFilename)
			if st, err := os.Stat(hostTempVideoPath); err == nil && !st.IsDir() {
				log.Printf("检测到结果文件: %s (size=%d)", hostTempVideoPath, st.Size())
				// 等待大小稳定（连续3次相同，每次间隔3s）
				last := st.Size()
				stable := 1
				for i := 2; i <= 3; i++ {
					time.Sleep(3 * time.Second)
					st2, e2 := os.Stat(hostTempVideoPath)
					if e2 != nil {
						log.Printf("稳定性检查失败: %v", e2)
						break
					}
					cur := st2.Size()
					log.Printf("稳定性检查 #%d: %d bytes", i, cur)
					if cur == last {
						stable++
					} else {
						last = cur
						stable = 1
					}
				}
				if stable >= 3 {
					// 复制并校验
					copyOK := false
					for attempt := 1; attempt <= 3; attempt++ {
						log.Printf("=== 复制尝试(本地) #%d (src=%s -> dst=%s) ===", attempt, hostTempVideoPath, hostOut)
						if err := copyFile(hostTempVideoPath, hostOut); err != nil {
							log.Printf("复制失败: %v", err)
							if attempt < 3 {
								time.Sleep(5 * time.Second)
								continue
							}
						} else {
							if dstStat, e3 := os.Stat(hostOut); e3 == nil && dstStat.Size() == last {
								log.Printf("✅ 文件复制成功，大小匹配: %d bytes", last)
								copyOK = true
								break
							}
							log.Printf("❌ 复制后大小不匹配或读取失败")
							if attempt < 3 {
								time.Sleep(5 * time.Second)
								continue
							}
						}
					}
					if !copyOK {
						status.Status = "failed"
						status.Error = "视频拷贝到结果目录失败，已重试3次"
						return
					}
					if err := removeFileIfExists(hostTempVideoPath); err != nil {
						log.Printf("清理本地临时文件失败: %v", err)
					}

					// 可选拷贝到公司目录
					if req.CopyToCompany {
						companyOut := filepath.Join(cfg.WindowsCompanyDir, resultFilename)
						log.Printf("复制到公司目录: %s", companyOut)
						if err := copyFile(hostOut, companyOut); err != nil {
							log.Printf("拷贝到公司目录失败: %v", err)
						}
					}
					// 完成
					status.Status = "completed"
					status.CurrentStep = "处理完成"
					status.Progress = 100
					status.ResultVideo = resultFilename
					status.ResultPath = hostOut
					status.EndTime = time.Now().Unix()
					status.TotalDuration = status.EndTime - status.StartTime
					log.Printf("任务 %s 完成，总耗时: %d 秒 (%.1f 分钟)", taskID, status.TotalDuration, float64(status.TotalDuration)/60)
					return
				} else {
					log.Printf("文件大小未稳定，继续等待...")
				}
			}

			// 检查文件是否存在且写入完成
			checkCmd := fmt.Sprintf("docker exec -i %s bash -lc 'if [ -f \"%s\" ]; then echo FOUND; else echo MISSING; fi'", cfg.GenVideoContainer, inside)
			log.Printf("执行文件检查命令: %s", checkCmd)

			checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			stdout, stderr, err := run(checkCtx, "docker", "exec", "-i", cfg.GenVideoContainer, "bash", "-lc", fmt.Sprintf("if [ -f '%s' ]; then echo FOUND; else echo MISSING; fi", inside))
			cancel()

			// 静默检查，只在出错时记录日志
			if err != nil {
				log.Printf("视频合成检查 #%d 出错: 文件路径=%s, 错误=%v, stdout=%s, stderr=%s", checkCount, inside, err, stdout, stderr)
			}

			// 检查stdout和stderr中是否包含FOUND
			if err == nil && (strings.Contains(stdout, "FOUND") || strings.Contains(stderr, "FOUND")) {
				// 文件存在，但需要检查是否还在写入
				log.Printf("文件已存在，检查是否还在写入...")

				// 等待文件稳定 - 检查文件大小是否还在变化
				var lastSize string
				var stableCount int

				for stabilityCheck := 1; stabilityCheck <= 5; stabilityCheck++ {
					sizeCmd := fmt.Sprintf("docker exec -i %s stat -c %%s %s", cfg.GenVideoContainer, inside)
					log.Printf("稳定性检查 #%d: %s", stabilityCheck, sizeCmd)

					sizeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					sizeOut, _, sizeErr := run(sizeCtx, "docker", "exec", "-i", cfg.GenVideoContainer, "stat", "-c", "%s", inside)
					cancel()

					if sizeErr != nil {
						log.Printf("稳定性检查失败: %v", sizeErr)
						break
					}

					currentSize := strings.TrimSpace(sizeOut)
					log.Printf("当前文件大小: %s bytes", currentSize)

					if stabilityCheck == 1 {
						lastSize = currentSize
						stableCount = 1
					} else if currentSize == lastSize {
						stableCount++
						log.Printf("文件大小稳定 (连续%d次相同)", stableCount)
						if stableCount >= 3 {
							log.Printf("✅ 文件写入完成，大小稳定: %s bytes", currentSize)
							break
						}
					} else {
						log.Printf("文件大小仍在变化: %s -> %s", lastSize, currentSize)
						lastSize = currentSize
						stableCount = 1
					}

					if stabilityCheck < 5 {
						time.Sleep(3 * time.Second)
					}
				}

				// 如果文件稳定了，继续处理
				if stableCount >= 3 {
					// 视频生成完成
					status.CurrentStep = "下载最终视频"
					status.Progress = 95
					persistTaskStatus(status)

					// 拷贝到结果目录 - 带重试和完整性检查
					hostOut := filepath.Join(cfg.HostResultDir, resultFilename)
					if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil {
						status.Status = "failed"
						status.Error = fmt.Sprintf("创建结果目录失败: %v", err)
						return
					}

					// 获取容器中文件的大小 - 等待文件稳定
					log.Printf("开始检查容器文件: %s", inside)

					// 等待文件大小稳定（最多等待30秒）
					var expectedSize string
					for waitAttempt := 1; waitAttempt <= 6; waitAttempt++ {
						sizeCmd := fmt.Sprintf("docker exec -i %s stat -c %%s %s", cfg.GenVideoContainer, inside)
						log.Printf("执行命令: %s", sizeCmd)

						sizeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						sizeOut, _, err := run(sizeCtx, "docker", "exec", "-i", cfg.GenVideoContainer, "stat", "-c", "%s", inside)
						cancel()
						if err != nil {
							log.Printf("无法获取容器文件大小 (第%d次): %v", waitAttempt, err)
							if waitAttempt < 6 {
								time.Sleep(5 * time.Second)
								continue
							}
						} else {
							currentSize := strings.TrimSpace(sizeOut)
							log.Printf("容器文件大小检查 #%d: %s bytes", waitAttempt, currentSize)

							if waitAttempt == 1 {
								expectedSize = currentSize
							} else if currentSize == expectedSize {
								log.Printf("文件大小已稳定: %s bytes", expectedSize)
								break
							} else {
								log.Printf("文件大小仍在变化: %s -> %s", expectedSize, currentSize)
								expectedSize = currentSize
							}

							if waitAttempt < 6 {
								time.Sleep(5 * time.Second)
							}
						}
					}

					// 重试复制，最多3次
					var copySuccess bool
					for attempt := 1; attempt <= 3; attempt++ {
						log.Printf("=== 复制尝试 #%d ===", attempt)
						log.Printf("源文件: %s:%s", cfg.GenVideoContainer, inside)
						log.Printf("目标文件: %s", hostOut)
						log.Printf("期望大小: %s bytes", expectedSize)

						copyCmd := fmt.Sprintf("docker cp %s:%s %s", cfg.GenVideoContainer, inside, hostOut)
						log.Printf("执行命令: %s", copyCmd)

						copyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
						startTime := time.Now()
						_, cpErr, err := run(copyCtx, "docker", "cp", fmt.Sprintf("%s:%s", cfg.GenVideoContainer, inside), hostOut)
						copyDuration := time.Since(startTime)
						cancel()

						log.Printf("复制耗时: %v", copyDuration)

						if err != nil {
							log.Printf("第%d次复制失败: %v | %s", attempt, err, cpErr)
							if attempt < 3 {
								log.Printf("等待5秒后重试...")
								time.Sleep(5 * time.Second)
								continue
							}
						} else {
							log.Printf("复制命令执行成功，开始验证文件...")

							// 验证文件大小 - 添加更详细的检查
							if expectedSize != "" {
								// 先检查容器中的文件大小是否还在变化
								checkCmd := fmt.Sprintf("docker exec -i %s stat -c %%s %s", cfg.GenVideoContainer, inside)
								log.Printf("复制后检查容器文件大小: %s", checkCmd)

								checkCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
								checkOut, _, checkErr := run(checkCtx, "docker", "exec", "-i", cfg.GenVideoContainer, "stat", "-c", "%s", inside)
								cancel()

								if checkErr == nil {
									currentContainerSize := strings.TrimSpace(checkOut)
									log.Printf("复制后容器文件大小: %s bytes (复制前: %s bytes)", currentContainerSize, expectedSize)

									if currentContainerSize != expectedSize {
										log.Printf("⚠️ 容器文件大小在复制过程中发生了变化!")
										log.Printf("这可能是视频合成还在进行中，或者有并发写入")
									}
								}

								// 检查目标文件
								if stat, err := os.Stat(hostOut); err == nil {
									actualSize := fmt.Sprintf("%d", stat.Size())
									log.Printf("文件大小验证: 期望 %s bytes, 实际 %s bytes", expectedSize, actualSize)

									// 计算差异
									expectedInt, _ := strconv.ParseInt(expectedSize, 10, 64)
									actualInt, _ := strconv.ParseInt(actualSize, 10, 64)
									diff := actualInt - expectedInt
									log.Printf("大小差异: %d bytes (%.2f MB)", diff, float64(diff)/1024/1024)

									if actualSize == expectedSize {
										log.Printf("✅ 文件复制成功，大小完全匹配!")
										copySuccess = true
										break
									} else {
										log.Printf("❌ 文件大小不匹配，需要重试")
										if attempt < 3 {
											log.Printf("等待5秒后重试...")
											time.Sleep(5 * time.Second)
											continue
										}
									}
								} else {
									log.Printf("❌ 无法读取目标文件: %v", err)
									if attempt < 3 {
										time.Sleep(5 * time.Second)
										continue
									}
								}
							} else {
								// 无法验证大小，假设成功
								log.Printf("⚠️ 无法验证文件大小，假设复制成功")
								copySuccess = true
								break
							}
						}
					}

					if !copySuccess {
						status.Status = "failed"
						status.Error = "视频拷贝到结果目录失败，已重试3次"
						return
					}

					if err := removeFileIfExists(hostTempVideoPath); err != nil {
						log.Printf("清理本地临时文件失败: %v", err)
					}

					// 可选拷贝到Windows目录 - 直接从容器复制，避免使用被截断的文件
					if req.CopyToCompany {
						companyOut := filepath.Join(cfg.WindowsCompanyDir, resultFilename)
						companyCmd := fmt.Sprintf("docker cp %s:%s %s", cfg.GenVideoContainer, inside, companyOut)
						log.Printf("执行Windows拷贝命令: %s", companyCmd)

						companyCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
						startTime := time.Now()
						_, cpErr, err := run(companyCtx, "docker", "cp", fmt.Sprintf("%s:%s", cfg.GenVideoContainer, inside), companyOut)
						copyDuration := time.Since(startTime)
						cancel()

						log.Printf("Windows拷贝耗时: %v", copyDuration)
						if err != nil {
							log.Printf("拷贝到Windows目录失败: %v | %s", err, cpErr)
						} else {
							log.Printf("成功拷贝到Windows目录: %s", companyOut)
						}
					}

					// 完成
					status.Status = "completed"
					status.CurrentStep = "处理完成"
					status.Progress = 100
					status.ResultVideo = resultFilename
					status.ResultPath = hostOut
					status.EndTime = time.Now().Unix()
					status.TotalDuration = status.EndTime - status.StartTime
					log.Printf("任务 %s 完成，总耗时: %d 秒 (%.1f 分钟)", taskID, status.TotalDuration, float64(status.TotalDuration)/60)
					return
				} else {
					log.Printf("文件大小未稳定，继续等待...")
				}
			}

			// 更新进度
			if status.Progress < 90 {
				status.Progress += 2
				persistTaskStatus(status)
			}

		case <-timeout:
			status.Status = "failed"
			status.Progress = 100
			status.CurrentStep = "视频合成超时"
			status.Error = "视频合成超时"
			status.EndTime = time.Now().Unix()
			status.TotalDuration = status.EndTime - status.StartTime
			log.Printf("任务 %s 超时失败，总耗时: %d 秒 (%.1f 分钟)，超时时长: %.1f 分钟", taskID, status.TotalDuration, float64(status.TotalDuration)/60, maxWait.Minutes())
			return
		}
	}
}

// /api/auto/status/:taskId: 查询自动化处理状态
func handleAutoStatus(c *gin.Context) {
	taskID := c.Param("taskId")
	status, err := loadTaskStatus(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取任务状态失败: %v", err)})
		return
	}
	if status == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}

	c.JSON(http.StatusOK, status)
}

func handleAutoRetry(c *gin.Context) {
	loginUser := usernameFromContext(c)
	if loginUser == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "请先登录"})
		return
	}

	taskID := strings.TrimSpace(c.Param("taskId"))
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少任务ID"})
		return
	}

	status, err := loadTaskStatus(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取任务状态失败: %v", err)})
		return
	}
	if status == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		return
	}
	if status.Status != "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "仅支持重试失败的任务"})
		return
	}
	if status.AudioPath == "" || status.VideoPath == "" || status.Request == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务缺少重试所需的资源信息"})
		return
	}
	if _, err := os.Stat(status.AudioPath); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("音频文件不可用: %v", err)})
		return
	}
	if _, err := os.Stat(status.VideoPath); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("视频文件不可用: %v", err)})
		return
	}

	payload := queuedTask{
		TaskID:    taskID,
		AudioPath: status.AudioPath,
		VideoPath: status.VideoPath,
		Req:       *status.Request,
	}

	status.RetryCount++
	status.Status = "queued"
	status.CurrentStep = "等待排队执行"
	status.Progress = 0
	status.StartTime = time.Now().Unix()
	status.EndTime = 0
	status.TotalDuration = 0
	status.Error = ""
	status.ResultVideo = ""
	status.ResultPath = ""
	status.Username = loginUser

	taskStatusMu.Lock()
	taskStatusMap[taskID] = status
	taskStatusMu.Unlock()

	addTaskToIndex(taskID, status.StartTime)
	persistTaskStatus(status)

	if err := publishTask(payload); err != nil {
		status.Status = "failed"
		status.Error = fmt.Sprintf("任务重新入队失败: %v", err)
		status.EndTime = time.Now().Unix()
		status.TotalDuration = status.EndTime - status.StartTime
		persistTaskStatus(status)
		c.JSON(http.StatusInternalServerError, gin.H{"error": status.Error})
		return
	}

	persistTaskStatus(status)
	c.JSON(http.StatusOK, gin.H{"task_id": taskID, "status": status.Status, "retry_count": status.RetryCount})
}

// 列出所有任务状态（按开始时间倒序）
func handleAutoTasks(c *gin.Context) {
	statuses, err := listTaskStatuses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取任务列表失败: %v", err)})
		return
	}
	// Redis 已按 StartTime 排序（倒序）
	c.JSON(http.StatusOK, gin.H{"tasks": statuses})
}

// 打包下载：GET /api/auto/archive?task_ids=id1,id2 或 /api/auto/archive?all=1
func handleAutoArchive(c *gin.Context) {
	// 收集要打包的文件
	var files []string
	var statuses []*AutoProcessStatus
	if c.Query("all") == "1" || strings.ToLower(c.Query("all")) == "true" {
		var err error
		statuses, err = listTaskStatuses()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取任务列表失败: %v", err)})
			return
		}
	} else {
		ids := strings.Split(c.Query("task_ids"), ",")
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			st, err := loadTaskStatus(id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("读取任务状态失败: %v", err)})
				return
			}
			if st != nil {
				statuses = append(statuses, st)
			}
		}
	}
	for _, st := range statuses {
		if st.Status == "completed" && st.ResultPath != "" {
			if _, err := os.Stat(st.ResultPath); err == nil {
				files = append(files, st.ResultPath)
			}
		}
	}
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有可打包的完成视频"})
		return
	}

	// 设置响应头并流式写 Zip
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=videos.zip")
	zw := zip.NewWriter(c.Writer)
	defer zw.Close()
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		w, err := zw.Create(filepath.Base(path))
		if err != nil {
			f.Close()
			continue
		}
		if _, err := io.Copy(w, f); err != nil {
			f.Close()
			continue
		}
		f.Close()
	}
}

// /api/download/video/:filename: 下载视频文件
func handleDownloadVideo(c *gin.Context) {
	filename := c.Param("filename")
	if filename == "" {
		c.JSON(400, gin.H{"error": "缺少文件名"})
		return
	}

	// 安全检查：确保文件名不包含路径遍历
	filename = sanitizeFilename(filename)

	// 构建文件路径
	filePath := filepath.Join(cfg.HostResultDir, filename)

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(404, gin.H{"error": "文件不存在"})
		return
	}

	// 设置下载头
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Header("Content-Type", "video/mp4")

	// 发送文件
	c.File(filePath)
}
