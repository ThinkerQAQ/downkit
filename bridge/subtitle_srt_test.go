package downkit

import (
	"strings"
	"testing"
	"time"
)

func TestParseAndMarshalSRTPreservesCueData(t *testing.T) {
	input := "\ufeff1\r\n00:00:01,250 --> 00:00:03,500 position:50%\r\n第一行  \r\n第二行\r\n\r\n7\n01:02:03.004 --> 01:02:04,005\n日本語\n"
	document, err := parseSRT([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(document.Cues) != 2 || document.Cues[0].ID != 1 || document.Cues[1].ID != 7 {
		t.Fatalf("unexpected cues: %#v", document.Cues)
	}
	first := document.Cues[0]
	if first.Start != 1250*time.Millisecond || first.End != 3500*time.Millisecond || first.Settings != "position:50%" || first.Text != "第一行\n第二行" {
		t.Fatalf("unexpected first cue: %#v", first)
	}
	encoded, err := document.marshalSRT()
	if err != nil {
		t.Fatal(err)
	}
	wantParts := []string{
		"1\r\n00:00:01,250 --> 00:00:03,500 position:50%\r\n第一行\r\n第二行",
		"7\r\n01:02:03,004 --> 01:02:04,005\r\n日本語",
	}
	for _, part := range wantParts {
		if !strings.Contains(string(encoded), part) {
			t.Fatalf("marshaled SRT does not contain %q:\n%s", part, encoded)
		}
	}
	roundTrip, err := parseSRT(encoded)
	if err != nil || len(roundTrip.Cues) != 2 || roundTrip.Cues[1] != document.Cues[1] {
		t.Fatalf("round trip failed: %#v, %v", roundTrip, err)
	}
}

func TestParseSRTRejectsMalformedInput(t *testing.T) {
	tests := []string{
		"not-an-id\n00:00:01,000 --> 00:00:02,000\ntext\n",
		"1\n00:00:03,000 --> 00:00:02,000\ntext\n",
		"1\n00:61:00,000 --> 01:02:00,000\ntext\n",
		"1\n00:00:01,000 --> 00:00:02,000\none\n\n1\n00:00:03,000 --> 00:00:04,000\ntwo\n",
		"\r\n\r\n",
	}
	for _, input := range tests {
		if _, err := parseSRT([]byte(input)); err == nil {
			t.Fatalf("malformed SRT was accepted: %q", input)
		}
	}
}

func TestSubtitleDocumentWithTranslations(t *testing.T) {
	source := SubtitleDocument{Cues: []SubtitleCue{
		{ID: 1, Start: time.Second, End: 2 * time.Second, Text: "こんにちは"},
		{ID: 2, Start: 3 * time.Second, End: 4 * time.Second, Text: "世界"},
	}}
	translated, err := source.withTranslations(map[int]string{1: "你好", 2: "世界"}, false)
	if err != nil || translated.Cues[0].Text != "你好" || translated.Cues[0].Start != source.Cues[0].Start {
		t.Fatalf("translated document mismatch: %#v, %v", translated, err)
	}
	bilingual, err := source.withTranslations(map[int]string{1: "你好", 2: "世界"}, true)
	if err != nil || bilingual.Cues[0].Text != "こんにちは\n你好" || bilingual.Cues[1].Text != "世界\n世界" {
		t.Fatalf("bilingual document mismatch: %#v, %v", bilingual, err)
	}
	if _, err := source.withTranslations(map[int]string{1: "你好"}, false); err == nil {
		t.Fatal("missing translation was accepted")
	}
	if _, err := source.withTranslations(map[int]string{1: "你好", 2: "世界", 3: "extra"}, false); err == nil {
		t.Fatal("unknown translation was accepted")
	}
	if _, err := source.withTranslations(map[int]string{1: "", 2: "世界"}, false); err == nil {
		t.Fatal("empty translation was accepted")
	}
}
