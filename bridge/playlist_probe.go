package downkit

import (
	"fmt"
	"os"
)

type ytDLPAccess struct {
	attempts       []ytDLPAttempt
	taskCookiePath string
	cleanup        func()
}

func (a *app) prepareYTDLPAccess(pageURL string) (ytDLPAccess, error) {
	access := ytDLPAccess{cleanup: func() {}}
	if len(a.opts.pageCookies) > 0 {
		generated, err := writeTaskCookieFile(a.workDir, "yt-dlp-task-cookies.txt", a.opts.pageCookies)
		if err != nil {
			return access, fmt.Errorf("无法准备扩展传入的结构化 Cookie：%w", err)
		}
		access.taskCookiePath = generated
		access.cleanup = func() { _ = os.Remove(generated) }
	}
	access.attempts = []ytDLPAttempt{{proxy: a.opts.proxy, taskCookiePath: access.taskCookiePath}}
	if a.opts.proxy != "" {
		access.attempts = append(access.attempts, ytDLPAttempt{taskCookiePath: access.taskCookiePath})
	}
	return access, nil
}
