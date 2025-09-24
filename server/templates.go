package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	templateKindAudio = "audio"
	templateKindVideo = "video"
)

func templateKindDir(kind string) string {
	switch kind {
	case templateKindAudio:
		return cfg.AudioTemplateDir
	case templateKindVideo:
		return cfg.VideoTemplateDir
	default:
		return filepath.Join(cfg.WorkDir, "templates", kind)
	}
}

func templateRedisKey(kind string) string {
	base := strings.TrimSpace(cfg.QueuePrefix)
	if base == "" {
		base = "digital_people"
	}
	return fmt.Sprintf("%s:templates:%s", base, kind)
}

func templateFileExt(kind string) string {
	switch kind {
	case templateKindAudio:
		return ".wav"
	case templateKindVideo:
		return ".mp4"
	default:
		return ""
	}
}

func ensureTemplateKindDir(kind string) error {
    if ext := templateFileExt(kind); ext == "" {
        return fmt.Errorf("unsupported template kind: %s", kind)
    }
    dir := templateKindDir(kind)
    if err := os.MkdirAll(dir, 0o755); err != nil {
        return err
    }
    return nil
}

func sanitizeTemplateKey(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		trimmed = "template"
	}
	sanitized := sanitizeFilename(trimmed)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	sanitized = strings.Trim(sanitized, "-_")
	sanitized = strings.TrimSpace(sanitized)
	if sanitized == "" {
		sanitized = fmt.Sprintf("template-%d", time.Now().Unix())
	}
	return strings.ToLower(sanitized)
}

func loadTemplateList(kind string) ([]TemplateItem, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("Redis 未初始化")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := redisClient.Get(ctx, templateRedisKey(kind)).Bytes()
	if err != nil {
		if err == redis.Nil {
			return []TemplateItem{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return []TemplateItem{}, nil
	}
	var items []TemplateItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func saveTemplateList(kind string, items []TemplateItem) error {
    if err := ensureTemplateKindDir(kind); err != nil {
        return err
    }
    // 按更新时间倒序，便于前端展示
    sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
    data, err := json.Marshal(items)
    if err != nil {
        return err
    }
    if redisClient == nil {
        return fmt.Errorf("Redis 未初始化")
    }
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    if err := redisClient.Set(ctx, templateRedisKey(kind), data, 0).Err(); err != nil {
        return err
    }
    log.Printf("[模板索引] 已保存: kind=%s, count=%d", kind, len(items))
    return nil
}

func templateFilePath(kind, name string) (string, error) {
	if ext := templateFileExt(kind); ext == "" {
		return "", fmt.Errorf("unsupported template kind: %s", kind)
	}
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return "", fmt.Errorf("template name 不能为空")
	}
	// 防止路径穿越：重新 sanitize 并要求不改变名字
	if sanitized := sanitizeFilename(cleaned); sanitized != cleaned {
		return "", fmt.Errorf("模板标识包含非法字符")
	}
	return filepath.Join(templateKindDir(kind), cleaned+templateFileExt(kind)), nil
}

func upsertTemplateItem(kind string, item TemplateItem) error {
    items, err := loadTemplateList(kind)
    if err != nil {
        return err
    }
    replaced := false
    for idx := range items {
        if items[idx].Name == item.Name {
            items[idx] = item
            replaced = true
            break
        }
    }
    if !replaced {
        items = append(items, item)
    }
    if err := saveTemplateList(kind, items); err != nil {
        return err
    }
    if replaced {
        log.Printf("[模板索引] 更新: kind=%s, name=%s", kind, item.Name)
    } else {
        log.Printf("[模板索引] 新增: kind=%s, name=%s", kind, item.Name)
    }
    return nil
}

func findTemplateItem(kind, name string) (TemplateItem, string, error) {
    var empty TemplateItem
    items, err := loadTemplateList(kind)
    if err != nil {
        return empty, "", err
    }
    log.Printf("[模板查找] 开始: kind=%s, name=%s, items=%d", kind, name, len(items))
    for _, it := range items {
        if it.Name == name {
            path, err := templateFilePath(kind, it.Name)
            if err != nil {
                return empty, "", err
            }
            if _, err := os.Stat(path); err != nil {
                if os.IsNotExist(err) {
                    legacyDir := filepath.Join(cfg.WorkDir, "templates", kind)
                    legacyPath := filepath.Join(legacyDir, it.Name+templateFileExt(kind))
                    if st, legacyErr := os.Stat(legacyPath); legacyErr == nil && !st.IsDir() {
                        log.Printf("[模板查找] 文件缺失，尝试从旧目录迁移: src=%s dst=%s size=%d", legacyPath, path, st.Size())
                        if copyErr := copyFile(legacyPath, path); copyErr != nil {
                            return empty, "", fmt.Errorf("模版迁移失败: %w", copyErr)
                        }
                        if st2, e2 := os.Stat(path); e2 == nil { log.Printf("[模板查找] 迁移完成: dst_size=%d", st2.Size()) }
                    } else {
                        log.Printf("[模板查找] 目标与旧目录均缺失: name=%s dst=%s legacy=%s", name, path, legacyPath)
                        return empty, "", fmt.Errorf("模板 %s 文件缺失", name)
                    }
                } else {
                    return empty, "", err
                }
            }
            log.Printf("[模板查找] 命中: name=%s, path=%s", name, path)
            return it, path, nil
        }
    }
    log.Printf("[模板查找] 未找到: name=%s", name)
    return empty, "", fmt.Errorf("模板 %s 未找到", name)
}

func listTemplates(kind string) ([]TemplateItem, error) {
	items, err := loadTemplateList(kind)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []TemplateItem{}, nil
	}
	return items, nil
}
