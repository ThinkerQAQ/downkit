package downkit

import "testing"

func TestPrepareYTDLPAccessFallsBackFromProxyToDirect(t *testing.T) {
	a := app{opts: options{proxy: "http://127.0.0.1:11111"}, workDir: t.TempDir()}
	access, err := a.prepareYTDLPAccess("https://www.bilibili.com/video/test")
	if err != nil {
		t.Fatal(err)
	}
	defer access.cleanup()
	if len(access.attempts) != 2 || access.attempts[0].proxy == "" || access.attempts[1].proxy != "" {
		t.Fatalf("attempt order = %#v", access.attempts)
	}
}

func TestPrepareYTDLPAccessDoesNotDuplicateDirectAttempt(t *testing.T) {
	a := app{workDir: t.TempDir()}
	access, err := a.prepareYTDLPAccess("https://www.bilibili.com/video/test")
	if err != nil {
		t.Fatal(err)
	}
	defer access.cleanup()
	if len(access.attempts) != 1 || access.attempts[0].proxy != "" {
		t.Fatalf("attempts = %#v", access.attempts)
	}
}
