package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	templateKindAudio = "audio"
	templateKindVideo = "video"
)

func templateDir() string {
	return filepath.Join(cfg.WorkDir, "templates")
}

func templateKindDir(kind string) string {
	return filepath.Join(templateDir(), kind)
}

func templateMetadataPath(kind string) string {
	return filepath.Join(templateKindDir(kind), "metadata.json")
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
	return os.MkdirAll(templateKindDir(kind), 0o755)
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
	path := templateMetadataPath(kind)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []TemplateItem{}, nil
		}
		return nil, err
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
	return os.WriteFile(templateMetadataPath(kind), data, 0o644)
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
	return saveTemplateList(kind, items)
}

func findTemplateItem(kind, name string) (TemplateItem, string, error) {
	var empty TemplateItem
	items, err := loadTemplateList(kind)
	if err != nil {
		return empty, "", err
	}
	for _, it := range items {
		if it.Name == name {
			path, err := templateFilePath(kind, it.Name)
			if err != nil {
				return empty, "", err
			}
			if _, err := os.Stat(path); err != nil {
				return empty, "", err
			}
			return it, path, nil
		}
	}
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
