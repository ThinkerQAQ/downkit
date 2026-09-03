package downkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTranslationModelChecksGGUFMagic(t *testing.T) {
	directory := t.TempDir()
	valid := filepath.Join(directory, "model.gguf")
	invalid := filepath.Join(directory, "model.bin")
	if err := os.WriteFile(valid, []byte("GGUFmodel"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(invalid, []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateTranslationModel(valid); err != nil {
		t.Fatalf("valid GGUF rejected: %v", err)
	}
	if err := validateTranslationModel(invalid); err == nil {
		t.Fatal("non-GGUF model was accepted")
	}
}

func TestLlamaTranslateCueUsesTranslateGemmaMessageShape(t *testing.T) {
	var received llamaChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" || request.Method != http.MethodPost {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"你好，世界"}}]}`))
	}))
	defer server.Close()

	translator := &llamaTranslator{baseURL: server.URL, modelPath: "translategemma.gguf", client: server.Client()}
	translated, err := translator.translateCue(context.Background(), "ja", "zh-Hans", "こんにちは、世界")
	if err != nil || translated != "你好，世界" {
		t.Fatalf("unexpected translation: %q, %v", translated, err)
	}
	if received.Temperature != 0 || len(received.Messages) != 1 || len(received.Messages[0].Content) != 1 {
		t.Fatalf("unexpected request: %#v", received)
	}
	content := received.Messages[0].Content[0]
	if content.Type != "text" || content.SourceLanguage != "ja" || content.TargetLanguage != "zh-Hans" || content.Text != "こんにちは、世界" {
		t.Fatalf("TranslateGemma content shape mismatch: %#v", content)
	}
}

func TestLlamaTranslatorWritesTranslatedAndBilingualSRT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var input llamaChatRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Error(err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		translated := map[string]string{"こんにちは": "你好", "世界": "世界"}
		text := input.Messages[0].Content[0].Text
		_ = json.NewEncoder(response).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": translated[text]}}},
		})
	}))
	defer server.Close()

	directory := t.TempDir()
	mediaPath := filepath.Join(directory, "video.mp4")
	sourcePath := filepath.Join(directory, "video.ja.srt")
	input := "1\n00:00:01,000 --> 00:00:02,000\nこんにちは\n\n2\n00:00:03,000 --> 00:00:04,000\n世界\n"
	if err := os.WriteFile(sourcePath, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	translator := &llamaTranslator{baseURL: server.URL, modelPath: "translategemma.gguf", workDir: directory, client: server.Client()}
	artifact, err := translator.Translate(context.Background(), SubtitleArtifact{
		Path: sourcePath, Format: "srt", Language: "ja", MediaPath: mediaPath,
	}, "zh-Hans", true)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(artifact.Path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parseSRT(data)
	if err != nil || len(result.Cues) != 2 || result.Cues[0].Text != "こんにちは\n你好" || result.Cues[0].Start.Seconds() != 1 {
		t.Fatalf("unexpected bilingual output: %#v, %v", result, err)
	}
}

func TestTranslationCheckpointDiscardsUnknownAndEmptyEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "translated.json")
	if err := os.WriteFile(path, []byte(`{"1":"你好","2":"","99":"extra"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	document := SubtitleDocument{Cues: []SubtitleCue{{ID: 1}, {ID: 2}}}
	checkpoint := loadTranslationCheckpoint(path, document)
	if len(checkpoint) != 1 || checkpoint[1] != "你好" {
		t.Fatalf("unexpected checkpoint: %#v", checkpoint)
	}
}

func TestCompletedTranslationCountUsesKnownNonEmptyCues(t *testing.T) {
	document := SubtitleDocument{Cues: []SubtitleCue{{ID: 1}, {ID: 2}, {ID: 3}}}
	translations := map[int]string{1: "done", 2: " ", 99: "unknown"}
	if got := completedTranslationCount(translations, document); got != 1 {
		t.Fatalf("completed translations = %d, want 1", got)
	}
}

func TestTranslationSourceLanguage(t *testing.T) {
	if got, err := translationSourceLanguage("JA-orig"); err != nil || got != "ja" {
		t.Fatalf("source language = %q, %v", got, err)
	}
	if _, err := translationSourceLanguage("auto"); err == nil {
		t.Fatal("automatic source language was accepted")
	}
}
