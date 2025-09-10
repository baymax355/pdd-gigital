package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "time"

    "github.com/gin-gonic/gin"
)

var cfg Config

func main() {
    cfg = loadConfig()

    if err := ensureFFmpeg(); err != nil {
        log.Printf("警告: %v (请确保运行环境已安装 ffmpeg)", err)
    }

    r := gin.Default()

    // 静态资源（如构建后的前端）
    staticCandidates := []string{}
    if cfg.StaticDir != "" { staticCandidates = append(staticCandidates, cfg.StaticDir) }
    staticCandidates = append(staticCandidates, "./client/dist", "../client/dist")
    for _, p := range staticCandidates {
        if st, err := os.Stat(p); err == nil && st.IsDir() {
            r.Static("/", p)
            log.Printf("静态资源目录: %s", p)
            break
        }
    }

    // 直通封装：与 heygem.txt 相同路径，统一从本服务调用
    r.POST("/v1/preprocess_and_tran", handleProxyPreprocess)
    r.POST("/v1/invoke", handleProxyInvoke)
    r.POST("/easy/submit", handleProxySubmit)

    api := r.Group("/api")
    {
        api.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
        api.GET("/files", handleListFiles)

        api.POST("/upload/audio", handleUploadAudio)
        api.POST("/upload/video", handleUploadVideo)

        api.POST("/tts/preprocess", handleTTSPreprocess)
        api.POST("/tts/invoke", handleTTSInvoke)

        api.POST("/video/submit", handleVideoSubmit)
        api.GET("/video/result", handleVideoResult)
    }

    addr := fmt.Sprintf(":%s", cfg.Port)
    log.Printf("服务启动于 %s", addr)
    if err := r.Run(addr); err != nil {
        log.Fatal(err)
    }
}

// 直通封装：/v1/preprocess_and_tran -> TTS_BASE_URL/v1/preprocess_and_tran
func handleProxyPreprocess(c *gin.Context) {
    body, err := io.ReadAll(c.Request.Body)
    if err != nil { c.JSON(400, gin.H{"error": err.Error()}); return }
    url := fmt.Sprintf("%s/v1/preprocess_and_tran", cfg.TTSBaseURL)
    resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
    if err != nil { c.JSON(502, gin.H{"error": err.Error()}); return }
    defer resp.Body.Close()
    for k, vs := range resp.Header { for _, v := range vs { c.Writer.Header().Add(k, v) } }
    c.Status(resp.StatusCode)
    io.Copy(c.Writer, resp.Body)
}

// 直通封装：/v1/invoke -> TTS_BASE_URL/v1/invoke（上游返回音频流，这里原样转发）
func handleProxyInvoke(c *gin.Context) {
    body, err := io.ReadAll(c.Request.Body)
    if err != nil { c.JSON(400, gin.H{"error": err.Error()}); return }
    url := fmt.Sprintf("%s/v1/invoke", cfg.TTSBaseURL)
    resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
    if err != nil { c.JSON(502, gin.H{"error": err.Error()}); return }
    defer resp.Body.Close()
    // 透传上游 headers（含 Content-Type/Length/Disposition 等）
    for k, vs := range resp.Header { for _, v := range vs { c.Writer.Header().Add(k, v) } }
    c.Status(resp.StatusCode)
    io.Copy(c.Writer, resp.Body)
}

// 直通封装：/easy/submit -> VIDEO_BASE_URL/easy/submit
func handleProxySubmit(c *gin.Context) {
    body, err := io.ReadAll(c.Request.Body)
    if err != nil { c.JSON(400, gin.H{"error": err.Error()}); return }
    url := fmt.Sprintf("%s/easy/submit", cfg.VideoBaseURL)
    resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
    if err != nil { c.JSON(502, gin.H{"error": err.Error()}); return }
    defer resp.Body.Close()
    for k, vs := range resp.Header { for _, v := range vs { c.Writer.Header().Add(k, v) } }
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
        if !e.IsDir() { files = append(files, e.Name()) }
    }
    c.JSON(200, gin.H{"dir": root, "files": files})
}

// /api/upload/audio: multipart form: file, trim_silence ("true"/"false"), out_name (optional)
func handleUploadAudio(c *gin.Context) {
    ctx := c.Request.Context()
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "缺少音频文件 file"})
        return
    }
    trim := strings.ToLower(c.PostForm("trim_silence")) == "true"
    outName := c.PostForm("out_name")
    if outName == "" { outName = "ref.wav" }
    // 保存原始音频
    srcPath, err := saveMultipartFile(file, filepath.Join(cfg.WorkDir, "upload"), outName)
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }

    // 处理: 可选去静音 -> 响度归一 + 16k 单声道 PCM
    work := filepath.Join(cfg.WorkDir, "audio")
    os.MkdirAll(work, 0o755)
    trimmed := filepath.Join(work, "ref_trim.wav")
    norm := filepath.Join(work, "ref_norm.wav")

    if trim {
        // ffmpeg silenceremove
        _, stderr, err := run(ctx, "ffmpeg", "-y", "-i", srcPath,
            "-af", "silenceremove=start_periods=1:start_duration=0:start_threshold=-50dB:stop_periods=1:stop_duration=0:stop_threshold=-50dB",
            "-ac", "1", "-ar", "16000", "-acodec", "pcm_s16le", trimmed,
        )
        if err != nil { c.JSON(500, gin.H{"error": fmt.Sprintf("ffmpeg trim 失败: %v | %s", err, stderr)}); return }
    } else {
        trimmed = srcPath
    }

    // loudnorm
    _, stderr, err := run(ctx, "ffmpeg", "-y", "-i", trimmed,
        "-af", "loudnorm=I=-16:TP=-1.5:LRA=11",
        "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", norm,
    )
    if err != nil { c.JSON(500, gin.H{"error": fmt.Sprintf("ffmpeg loudnorm 失败: %v | %s", err, stderr)}); return }

    // 拷贝到 voice/data 目录
    dst := filepath.Join(cfg.HostVoiceDir, "ref_norm.wav")
    if err := copyFile(norm, dst); err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }

    c.JSON(200, gin.H{
        "src": srcPath,
        "trimmed": trimmed,
        "normalized": norm,
        "copied_to": dst,
        "reference_audio": "ref_norm.wav",
    })
}

// /api/upload/video: multipart form: file, out_name(optional default zhuqi.mp4)
func handleUploadVideo(c *gin.Context) {
    ctx := c.Request.Context()
    file, err := c.FormFile("file")
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "缺少视频文件 file"})
        return
    }
    baseName := c.PostForm("out_name")
    if baseName == "" { baseName = "zhuqi.mp4" }
    srcPath, err := saveMultipartFile(file, filepath.Join(cfg.WorkDir, "upload"), baseName)
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }

    silentPath := filepath.Join(cfg.WorkDir, "video", "silent.mp4")
    os.MkdirAll(filepath.Dir(silentPath), 0o755)
    _, stderr, err := run(ctx, "ffmpeg", "-y", "-i", srcPath, "-an", "-c:v", "copy", silentPath)
    if err != nil { c.JSON(500, gin.H{"error": fmt.Sprintf("ffmpeg 静音失败: %v | %s", err, stderr)}); return }

    // 拷贝到 face2face
    dst := filepath.Join(cfg.HostVideoDir, "silent.mp4")
    if err := copyFile(silentPath, dst); err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }

    c.JSON(200, gin.H{"src": srcPath, "silent": silentPath, "copied_to": dst})
}

// /api/tts/preprocess: JSON {format, reference_audio, lang}
func handleTTSPreprocess(c *gin.Context) {
    var req PreprocessReq
    if err := c.BindJSON(&req); err != nil {
        c.JSON(400, gin.H{"error": "无效的 JSON"}); return
    }
    if req.Format == "" { req.Format = "wav" }
    if req.Lang == "" { req.Lang = "zh" }
    if req.ReferenceAudio == "" { req.ReferenceAudio = "ref_norm.wav" }

    // 确保文件存在于 voice/data 目录（TTS 容器应挂载该目录到 /code/data）
    if _, err := os.Stat(filepath.Join(cfg.HostVoiceDir, req.ReferenceAudio)); err != nil {
        c.JSON(400, gin.H{"error": fmt.Sprintf("reference_audio 不存在: %v", err)}); return
    }

    body, _ := json.Marshal(req)
    url := fmt.Sprintf("%s/v1/preprocess_and_tran", cfg.TTSBaseURL)
    resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        b, _ := io.ReadAll(resp.Body)
        c.JSON(resp.StatusCode, gin.H{"error": string(b)}); return
    }
    var pp PreprocessResp
    if err := json.NewDecoder(resp.Body).Decode(&pp); err != nil {
        c.JSON(500, gin.H{"error": fmt.Sprintf("解析响应失败: %v", err)}); return
    }
    c.JSON(200, pp)
}

// /api/tts/invoke: JSON expects fields used by upstream + output_speaker(optional defaults speaker)
func handleTTSInvoke(c *gin.Context) {
    var req TTSInvokeReq
    if err := c.BindJSON(&req); err != nil { c.JSON(400, gin.H{"error": "无效的 JSON"}); return }
    if req.Speaker == "" { req.Speaker = "demo001" }
    if req.Format == "" { req.Format = "wav" }
    if req.TopP == 0 { req.TopP = 0.7 }
    if req.MaxNewTokens == 0 { req.MaxNewTokens = 1024 }
    if req.ChunkLength == 0 { req.ChunkLength = 100 }
    if req.RepetitionPenalty == 0 { req.RepetitionPenalty = 1.2 }
    if req.Temperature == 0 { req.Temperature = 0.7 }

    body, _ := json.Marshal(req)
    url := fmt.Sprintf("%s/v1/invoke", cfg.TTSBaseURL)
    resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        b, _ := io.ReadAll(resp.Body); c.JSON(resp.StatusCode, gin.H{"error": string(b)}); return
    }
    // 保存为 voice/data/speaker.wav
    outVoice := filepath.Join(cfg.HostVoiceDir, sanitizeFilename(req.Speaker)+".wav")
    f, err := os.Create(outVoice)
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
    if _, err := io.Copy(f, resp.Body); err != nil { f.Close(); c.JSON(500, gin.H{"error": err.Error()}); return }
    f.Close()

    // 复制到视频目录，便于后续 /easy/submit 使用
    outInVideo := filepath.Join(cfg.HostVideoDir, filepath.Base(outVoice))
    if err := copyFile(outVoice, outInVideo); err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }

    c.JSON(200, gin.H{"voice": outVoice, "copied_to_video_dir": outInVideo})
}

// /api/video/submit: JSON {audio_filename, video_filename, code, chaofen, watermark_switch, pn}
func handleVideoSubmit(c *gin.Context) {
    var req SubmitVideoReq
    if err := c.BindJSON(&req); err != nil { c.JSON(400, gin.H{"error": "无效的 JSON"}); return }
    if req.AudioFilename == "" { req.AudioFilename = "demo001.wav" }
    if req.VideoFilename == "" { req.VideoFilename = "silent.mp4" }
    if req.Code == "" { req.Code = fmt.Sprintf("task-%d", time.Now().Unix()) }

    // 确保文件存在于视频目录（容器应把该目录挂载为 /code/data）
    if _, err := os.Stat(filepath.Join(cfg.HostVideoDir, req.AudioFilename)); err != nil { c.JSON(400, gin.H{"error": fmt.Sprintf("音频不存在: %v", err)}); return }
    if _, err := os.Stat(filepath.Join(cfg.HostVideoDir, req.VideoFilename)); err != nil { c.JSON(400, gin.H{"error": fmt.Sprintf("视频不存在: %v", err)}); return }

    payload := map[string]any{
        "audio_url": filepath.Join(cfg.ContainerDataRoot, req.AudioFilename),
        "video_url": filepath.Join(cfg.ContainerDataRoot, req.VideoFilename),
        "code": req.Code,
        "chaofen": req.Chaofen,
        "watermark_switch": req.WatermarkSwitch,
        "pn": req.PN,
    }
    if _, ok := payload["chaofen"].(int); !ok || req.Chaofen == 0 { payload["chaofen"] = 0 }
    if _, ok := payload["watermark_switch"].(int); !ok || req.WatermarkSwitch == 0 { payload["watermark_switch"] = 0 }
    if _, ok := payload["pn"].(int); !ok || req.PN == 0 { payload["pn"] = 1 }

    body, _ := json.Marshal(payload)
    url := fmt.Sprintf("%s/easy/submit", cfg.VideoBaseURL)
    resp, err := httpJSON(c.Request.Context(), http.MethodPost, url, body, map[string]string{"Content-Type": "application/json"})
    if err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
    defer resp.Body.Close()
    b, _ := io.ReadAll(resp.Body)
    c.JSON(resp.StatusCode, gin.H{"submit_payload": payload, "upstream_status": resp.StatusCode, "upstream_body": string(b)})
}

// /api/video/result?code=task001&copy_to_company=1
func handleVideoResult(c *gin.Context) {
    code := c.Query("code")
    if code == "" { c.JSON(400, gin.H{"error": "缺少 code"}); return }
    copyCompany := c.Query("copy_to_company") == "1"

    container := cfg.GenVideoContainer
    inside := filepath.Join(cfg.ContainerDataRoot, "temp", fmt.Sprintf("%s-r.mp4", code))

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
    if err != nil { _ = status; c.JSON(500, gin.H{"error": fmt.Sprintf("docker exec 检测失败: %v | %s", err, outErr)}); return }
    if !strings.Contains(out, "FOUND") {
        c.JSON(404, gin.H{"error": "生成文件未就绪", "path": inside})
        return
    }

    // docker cp 拷贝到 HostResultDir
    hostOut := filepath.Join(cfg.HostResultDir, fmt.Sprintf("%s-r.mp4", code))
    if err := os.MkdirAll(filepath.Dir(hostOut), 0o755); err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
    _, cpErr, err := run(ctx, "docker", "cp", fmt.Sprintf("%s:%s", container, inside), hostOut)
    if err != nil { c.JSON(500, gin.H{"error": fmt.Sprintf("docker cp 失败: %v | %s", err, cpErr)}); return }

    companyOut := ""
    if copyCompany {
        companyOut = filepath.Join(cfg.WindowsCompanyDir, fmt.Sprintf("%s-r.mp4", code))
        if err := copyFile(hostOut, companyOut); err != nil { c.JSON(500, gin.H{"error": fmt.Sprintf("复制到 Windows 目录失败: %v", err)}); return }
    }

    c.JSON(200, gin.H{"result": hostOut, "copied_to_company": companyOut})
}
