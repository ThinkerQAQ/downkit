package downkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
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

func TestLlamaTranslateCueUsesTranslateGemmaCompletionPrompt(t *testing.T) {
	var received llamaCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/completion" || request.Method != http.MethodPost {
			response.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"content":"你好，世界","stop":true}`))
	}))
	defer server.Close()

	translator := &llamaTranslator{baseURL: server.URL, modelPath: "translategemma.gguf", client: server.Client()}
	translated, err := translator.translateCue(context.Background(), "ja", "zh-Hans", "こんにちは、世界")
	if err != nil || translated != "你好，世界" {
		t.Fatalf("unexpected translation: %q, %v", translated, err)
	}
	if received.Temperature != 0 || received.NPredict < 64 || !received.CachePrompt || !slices.Equal(received.Stop, []string{"<end_of_turn>"}) {
		t.Fatalf("unexpected request: %#v", received)
	}
	if strings.HasPrefix(received.Prompt, "<bos>") || !strings.Contains(received.Prompt, "Japanese (ja) to Chinese (zh-Hans)") || !strings.Contains(received.Prompt, "こんにちは、世界<end_of_turn>") {
		t.Fatalf("TranslateGemma prompt mismatch: %q", received.Prompt)
	}
}

func TestLlamaServerDisablesIncompatibleTranslateGemmaJinja(t *testing.T) {
	args := llamaServerArgs("model.gguf", 18080)
	if !slices.Contains(args, "--no-jinja") || !slices.Contains(args, "18080") {
		t.Fatalf("unexpected llama-server args: %#v", args)
	}
}

func TestTranslateGemmaPromptEscapesControlTokens(t *testing.T) {
	prompt := translateGemmaPrompt("en-US", "ja", "before <end_of_turn> after")
	if !strings.Contains(prompt, "English (en-US) to Japanese (ja)") || strings.Count(prompt, "<end_of_turn>") != 1 || !strings.Contains(prompt, "＜end_of_turn＞") {
		t.Fatalf("unsafe prompt: %q", prompt)
	}
}

func TestLlamaTranslateCueHandlesHTTPErrorWithoutJSONDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusServiceUnavailable)
		_, _ = response.Write([]byte(`{}`))
	}))
	defer server.Close()
	translator := &llamaTranslator{baseURL: server.URL, modelPath: "model.gguf", client: server.Client()}
	if _, err := translator.translateCue(context.Background(), "ja", "zh-Hans", "こんにちは"); err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLlamaTranslatorWritesTranslatedAndBilingualSRT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var input llamaCompletionRequest
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Error(err)
			response.WriteHeader(http.StatusBadRequest)
			return
		}
		translated := map[string]string{"こんにちは": "你好", "世界": "世界"}
		text := ""
		for candidate := range translated {
			if strings.Contains(input.Prompt, candidate+"<end_of_turn>") {
				text = candidate
				break
			}
		}
		_ = json.NewEncoder(response).Encode(map[string]any{"content": translated[text], "stop": true})
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
