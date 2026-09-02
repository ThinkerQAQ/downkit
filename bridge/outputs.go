package downkit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MediaOutput is the common result produced by every media download path.
// SubtitleArtifacts may already be populated when yt-dlp fetched media and
// source-site subtitles in the same operation.
type MediaOutput struct {
	Path              string
	Title             string
	Index             int
	ItemID            string
	SubtitleArtifacts []SubtitleArtifact
	SubtitleAttempted bool
}

// SubtitleArtifact describes a standalone subtitle file associated with a
// media output. Source is intentionally explicit so future ASR and translation
// stages can coexist with source-site subtitles.
type SubtitleArtifact struct {
	Path       string
	Language   string
	Format     string
	Source     string
	MediaPath  string
	MediaIndex int
	MediaID    string
}

func (a *app) finalizeOutputs(outputs []MediaOutput) error {
	outputs, err := validateMediaOutputs(outputs)
	if err != nil {
		return err
	}

	artifacts := subtitleArtifactsFromMedia(outputs)
	if a.opts.subtitleRequest.enabled() {
		if a.subtitlePipeline == nil {
			return errors.New("字幕流水线尚未配置")
		}
		publishJobPhaseProgress("processing", 70, "正在处理字幕", 0, 0, 0)
		generated, pipelineErr := a.subtitlePipeline.Process(context.Background(), a.opts.subtitleRequest, outputs)
		if pipelineErr != nil {
			return fmt.Errorf("字幕处理失败：%w", pipelineErr)
		}
		artifacts = appendUniqueSubtitleArtifacts(artifacts, generated...)
	}

	for _, output := range outputs {
		publishJobOutputFile(output.Path, output.Index, output.ItemID)
		fmt.Fprintln(consoleOut, "完成：", output.Path)
	}
	for _, artifact := range artifacts {
		if _, statErr := os.Stat(artifact.Path); statErr != nil {
			return fmt.Errorf("字幕产物不存在 %s：%w", artifact.Path, statErr)
		}
		// Subtitle files are separate task outputs. Do not reuse a playlist
		// item's index/ID or the job view would replace its media output.
		publishJobOutput(artifact.Path)
		fmt.Fprintln(consoleOut, "字幕：", artifact.Path)
	}

	if !a.opts.keepWork {
		if err := os.RemoveAll(a.workDir); err != nil {
			fmt.Fprintln(consoleOut, "警告：无法清理工作目录：", err)
		}
	}
	return nil
}

func validateMediaOutputs(outputs []MediaOutput) ([]MediaOutput, error) {
	seen := make(map[string]bool)
	validated := make([]MediaOutput, 0, len(outputs))
	for _, output := range outputs {
		output.Path = filepath.Clean(strings.TrimSpace(output.Path))
		if output.Path == "." || output.Path == "" {
			continue
		}
		info, err := os.Stat(output.Path)
		if err != nil || !info.Mode().IsRegular() {
			if err == nil {
				err = errors.New("不是普通文件")
			}
			return nil, fmt.Errorf("媒体产物无效 %s：%w", output.Path, err)
		}
		key := strings.ToLower(output.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		validated = append(validated, output)
	}
	if len(validated) == 0 {
		return nil, errors.New("下载完成但没有生成媒体文件")
	}
	return validated, nil
}

func subtitleArtifactsFromMedia(outputs []MediaOutput) []SubtitleArtifact {
	var result []SubtitleArtifact
	for _, output := range outputs {
		for _, artifact := range output.SubtitleArtifacts {
			if artifact.MediaPath == "" {
				artifact.MediaPath = output.Path
			}
			if artifact.MediaIndex == 0 {
				artifact.MediaIndex = output.Index
			}
			if artifact.MediaID == "" {
				artifact.MediaID = output.ItemID
			}
			result = appendUniqueSubtitleArtifacts(result, artifact)
		}
	}
	return result
}

func appendUniqueSubtitleArtifacts(existing []SubtitleArtifact, values ...SubtitleArtifact) []SubtitleArtifact {
	seen := make(map[string]bool, len(existing)+len(values))
	for _, artifact := range existing {
		seen[strings.ToLower(filepath.Clean(artifact.Path))] = true
	}
	for _, artifact := range values {
		artifact.Path = filepath.Clean(strings.TrimSpace(artifact.Path))
		if artifact.Path == "." || artifact.Path == "" {
			continue
		}
		key := strings.ToLower(artifact.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		existing = append(existing, artifact)
	}
	return existing
}
