package main

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "log"
    "mime/multipart"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "time"
)

func run(ctx context.Context, name string, args ...string) (string, string, error) {
    cmd := exec.CommandContext(ctx, name, args...)
    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    err := cmd.Run()
    return stdout.String(), stderr.String(), err
}

func checkBinary(bin string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    _, _, err := run(ctx, bin, "-version")
    return err
}

func ensureFFmpeg() error {
    if err := checkBinary("ffmpeg"); err != nil {
        return fmt.Errorf("未检测到 ffmpeg，可用性检查失败: %w", err)
    }
    return nil
}

func saveMultipartFile(file *multipart.FileHeader, dstDir, wantName string) (string, error) {
    if file == nil {
        return "", errors.New("file 不能为空")
    }
    
    // 添加调试信息
    log.Printf("保存文件: %s, 大小: %d, 目标目录: %s", file.Filename, file.Size, dstDir)
    
    src, err := file.Open()
    if err != nil {
        log.Printf("打开文件失败: %v", err)
        return "", err
    }
    defer src.Close()

    if err := os.MkdirAll(dstDir, 0o755); err != nil {
        return "", err
    }

    base := wantName
    if base == "" {
        base = filepath.Base(file.Filename)
    }
    base = sanitizeFilename(base)
    dst := filepath.Join(dstDir, base)
    out, err := os.Create(dst)
    if err != nil {
        return "", err
    }
    defer out.Close()
    if _, err := io.Copy(out, src); err != nil {
        return "", err
    }
    return dst, nil
}

func sanitizeFilename(name string) string {
    name = filepath.Base(name)
    name = strings.ReplaceAll(name, "..", "_")
    name = strings.Map(func(r rune) rune {
        switch {
        case r >= 'a' && r <= 'z':
            return r
        case r >= 'A' && r <= 'Z':
            return r
        case r >= '0' && r <= '9':
            return r
        case r == '.' || r == '_' || r == '-' || r == ' ':
            return r
        default:
            return '-'
        }
    }, name)
    if name == "" {
        return "file"
    }
    return name
}

func copyFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil { return err }
    defer in.Close()
    if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil { return err }
    out, err := os.Create(dst)
    if err != nil { return err }
    defer out.Close()
    if _, err := io.Copy(out, in); err != nil { return err }
    return out.Close()
}

func httpJSON(ctx context.Context, method, url string, body []byte, headers map[string]string) (*http.Response, error) {
    req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
    if err != nil { return nil, err }
    for k, v := range headers {
        req.Header.Set(k, v)
    }
    if req.Header.Get("Content-Type") == "" {
        req.Header.Set("Content-Type", "application/json")
    }
    client := &http.Client{ Timeout: 10 * time.Minute }
    return client.Do(req)
}

