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
    "unicode"
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
		case unicode.Is(unicode.Han, r):
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
	if err != nil {
		return err
	}
	defer in.Close()
	srcClean := filepath.Clean(src)
	dstClean := filepath.Clean(dst)
	if srcClean == dstClean {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyFileWithRetries(src, dst string, retries int, delay time.Duration) error {
    var lastErr error
    for i := 1; i <= retries; i++ {
        if err := copyFile(src, dst); err != nil {
            lastErr = err
            time.Sleep(delay)
            continue
        }
        // 校验是否可读
        if err := waitFileReadable(dst, 3, 100*time.Millisecond); err != nil {
            lastErr = err
            time.Sleep(delay)
            continue
        }
        return nil
    }
    if lastErr == nil {
        lastErr = fmt.Errorf("copy failed after %d retries", retries)
    }
    return lastErr
}

func removeFileIfExists(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

// 等待文件可读（处理网络共享延迟或并发可见性问题）
func waitFileReadable(path string, retries int, delay time.Duration) error {
    if path == "" {
        return fmt.Errorf("empty path")
    }
    var lastErr error
    for i := 0; i < retries; i++ {
        f, err := os.Open(path)
        if err == nil {
            fi, _ := f.Stat()
            _ = f.Close()
            if fi != nil && fi.Size() >= 0 {
                return nil
            }
        } else {
            lastErr = err
        }
        time.Sleep(delay)
    }
    if lastErr == nil {
        lastErr = fmt.Errorf("file not readable after %d retries", retries)
    }
    return lastErr
}

func httpJSON(ctx context.Context, method, url string, body []byte, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
    client := &http.Client{Timeout: 10 * time.Minute}
    return client.Do(req)
}

// 带备用地址的 HTTP JSON 调用：primary 失败或 5xx 时尝试 fallback
// 注：容器内运行时不再需要备用地址逻辑，保持简单直连

func sanitizeTaskName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}

	mapped := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsSpace(r):
			return '_'
		case r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r == '-' || r == '_':
			return r
		case unicode.Is(unicode.Han, r):
			return r
		default:
			return '_'
		}
	}, trimmed)

	var b strings.Builder
	var lastUnderscore bool
	for _, r := range mapped {
		if r == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
			b.WriteRune(r)
			continue
		}
		lastUnderscore = false
		b.WriteRune(r)
	}

	cleaned := strings.Trim(b.String(), "_-")
	return cleaned
}
