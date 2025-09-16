package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func templateDir() string {
	return filepath.Join(cfg.WorkDir, "templates")
}

func audioTemplatePath() string {
	return filepath.Join(templateDir(), "audio_template.wav")
}

func videoTemplatePath() string {
	return filepath.Join(templateDir(), "video_template.mp4")
}

func templateMetaPath(kind string) string {
	return filepath.Join(templateDir(), kind+"_meta.json")
}

func saveTemplateMeta(kind string, meta TemplateMeta) error {
	if err := os.MkdirAll(templateDir(), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(templateMetaPath(kind), data, 0o644)
}

func loadTemplateMeta(kind string) (TemplateMeta, error) {
	var meta TemplateMeta
	data, err := os.ReadFile(templateMetaPath(kind))
	if err != nil {
		return meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, err
	}
	return meta, nil
}
