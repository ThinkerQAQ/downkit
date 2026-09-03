package downkit

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var srtTimingRE = regexp.MustCompile(`^\s*(\d{1,6}:\d{2}:\d{2}[,.]\d{3})\s*-->\s*(\d{1,6}:\d{2}:\d{2}[,.]\d{3})(?:\s+(.*?))?\s*$`)

// SubtitleDocument is DownKit's model-independent subtitle representation.
// Timing is parsed and written by DownKit; translation engines only receive a
// cue ID and its text, so a model cannot alter synchronization metadata.
type SubtitleDocument struct {
	Cues []SubtitleCue
}

type SubtitleCue struct {
	ID       int
	Start    time.Duration
	End      time.Duration
	Settings string
	Text     string
}

func parseSRT(data []byte) (SubtitleDocument, error) {
	content := strings.TrimPrefix(string(data), "\ufeff")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	document := SubtitleDocument{}
	seen := make(map[int]bool)

	for position := 0; position < len(lines); {
		for position < len(lines) && strings.TrimSpace(lines[position]) == "" {
			position++
		}
		if position >= len(lines) {
			break
		}

		idLine := position + 1
		id, err := strconv.Atoi(strings.TrimSpace(lines[position]))
		if err != nil || id <= 0 {
			return SubtitleDocument{}, fmt.Errorf("SRT 第 %d 行的字幕序号无效", idLine)
		}
		if seen[id] {
			return SubtitleDocument{}, fmt.Errorf("SRT 字幕序号 %d 重复", id)
		}
		seen[id] = true
		position++
		if position >= len(lines) {
			return SubtitleDocument{}, fmt.Errorf("SRT 字幕 %d 缺少时间轴", id)
		}

		matches := srtTimingRE.FindStringSubmatch(lines[position])
		if matches == nil {
			return SubtitleDocument{}, fmt.Errorf("SRT 字幕 %d 的时间轴无效", id)
		}
		start, err := parseSRTTimestamp(matches[1])
		if err != nil {
			return SubtitleDocument{}, fmt.Errorf("SRT 字幕 %d 的开始时间无效：%w", id, err)
		}
		end, err := parseSRTTimestamp(matches[2])
		if err != nil {
			return SubtitleDocument{}, fmt.Errorf("SRT 字幕 %d 的结束时间无效：%w", id, err)
		}
		if end < start {
			return SubtitleDocument{}, fmt.Errorf("SRT 字幕 %d 的结束时间早于开始时间", id)
		}
		position++

		textLines := make([]string, 0, 2)
		for position < len(lines) && strings.TrimSpace(lines[position]) != "" {
			textLines = append(textLines, strings.TrimRight(lines[position], " \t"))
			position++
		}
		document.Cues = append(document.Cues, SubtitleCue{
			ID: id, Start: start, End: end, Settings: strings.TrimSpace(matches[3]), Text: strings.Join(textLines, "\n"),
		})
	}

	if len(document.Cues) == 0 {
		return SubtitleDocument{}, errors.New("SRT 中没有字幕条目")
	}
	return document, nil
}

func parseSRTTimestamp(value string) (time.Duration, error) {
	value = strings.Replace(value, ".", ",", 1)
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return 0, errors.New("时间格式必须是 时:分:秒,毫秒")
	}
	secondsParts := strings.Split(parts[2], ",")
	if len(secondsParts) != 2 {
		return 0, errors.New("时间格式必须包含毫秒")
	}
	hours, hoursErr := strconv.Atoi(parts[0])
	minutes, minutesErr := strconv.Atoi(parts[1])
	seconds, secondsErr := strconv.Atoi(secondsParts[0])
	milliseconds, millisecondsErr := strconv.Atoi(secondsParts[1])
	if hoursErr != nil || minutesErr != nil || secondsErr != nil || millisecondsErr != nil ||
		hours < 0 || minutes < 0 || minutes > 59 || seconds < 0 || seconds > 59 || milliseconds < 0 || milliseconds > 999 {
		return 0, errors.New("时间数值超出范围")
	}
	return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second + time.Duration(milliseconds)*time.Millisecond, nil
}

func (d SubtitleDocument) marshalSRT() ([]byte, error) {
	if len(d.Cues) == 0 {
		return nil, errors.New("不能生成空 SRT")
	}
	var builder strings.Builder
	seen := make(map[int]bool)
	for cueIndex, cue := range d.Cues {
		if cue.ID <= 0 || seen[cue.ID] {
			return nil, fmt.Errorf("字幕序号 %d 无效或重复", cue.ID)
		}
		if cue.Start < 0 || cue.End < cue.Start {
			return nil, fmt.Errorf("字幕 %d 的时间轴无效", cue.ID)
		}
		seen[cue.ID] = true
		if cueIndex > 0 {
			builder.WriteString("\r\n")
		}
		fmt.Fprintf(&builder, "%d\r\n%s --> %s", cue.ID, formatSRTTimestamp(cue.Start), formatSRTTimestamp(cue.End))
		if settings := strings.TrimSpace(cue.Settings); settings != "" {
			builder.WriteByte(' ')
			builder.WriteString(settings)
		}
		builder.WriteString("\r\n")
		builder.WriteString(strings.ReplaceAll(cue.Text, "\n", "\r\n"))
		builder.WriteString("\r\n")
	}
	return []byte(builder.String()), nil
}

func formatSRTTimestamp(value time.Duration) string {
	totalMilliseconds := value.Milliseconds()
	hours := totalMilliseconds / int64(time.Hour/time.Millisecond)
	totalMilliseconds %= int64(time.Hour / time.Millisecond)
	minutes := totalMilliseconds / int64(time.Minute/time.Millisecond)
	totalMilliseconds %= int64(time.Minute / time.Millisecond)
	seconds := totalMilliseconds / int64(time.Second/time.Millisecond)
	milliseconds := totalMilliseconds % int64(time.Second/time.Millisecond)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, milliseconds)
}

// withTranslations validates the model response by cue ID before producing a
// translated or bilingual document. Unknown, missing, or empty translations
// are rejected instead of silently shifting text between timestamps.
func (d SubtitleDocument) withTranslations(translations map[int]string, bilingual bool) (SubtitleDocument, error) {
	known := make(map[int]bool, len(d.Cues))
	for _, cue := range d.Cues {
		known[cue.ID] = true
		translated, ok := translations[cue.ID]
		if !ok {
			return SubtitleDocument{}, fmt.Errorf("翻译结果缺少字幕 %d", cue.ID)
		}
		if strings.TrimSpace(translated) == "" {
			return SubtitleDocument{}, fmt.Errorf("字幕 %d 的翻译结果为空", cue.ID)
		}
	}
	unknown := make([]int, 0)
	for id := range translations {
		if !known[id] {
			unknown = append(unknown, id)
		}
	}
	if len(unknown) > 0 {
		sort.Ints(unknown)
		return SubtitleDocument{}, fmt.Errorf("翻译结果包含未知字幕 %v", unknown)
	}

	result := SubtitleDocument{Cues: make([]SubtitleCue, len(d.Cues))}
	copy(result.Cues, d.Cues)
	for index := range result.Cues {
		translated := strings.TrimSpace(translations[result.Cues[index].ID])
		if bilingual && strings.TrimSpace(result.Cues[index].Text) != "" {
			result.Cues[index].Text = strings.TrimSpace(result.Cues[index].Text) + "\n" + translated
		} else {
			result.Cues[index].Text = translated
		}
	}
	return result, nil
}
