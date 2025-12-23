package utils

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadLocalDotEnvForTest(t *testing.T) {
	t.Helper()

	// 约定本地环境文件放在 backend/.env（不会被提交，已在 .gitignore 忽略）。
	// 当前测试文件位于 backend/internal/utils，因此相对路径是 ../../.env
	candidates := []string{
		filepath.Join("..", "..", ".env"),
	}

	for _, p := range candidates {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		func() {
			defer func() { _ = f.Close() }()

			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				k, v, ok := strings.Cut(line, "=")
				if !ok {
					continue
				}
				key := strings.TrimSpace(k)
				if key == "" {
					continue
				}
				val := strings.TrimSpace(v)
				val = strings.Trim(val, "\"")
				val = strings.Trim(val, "'")
				if strings.TrimSpace(val) == "" {
					continue
				}

				// 只在未设置时才注入，避免覆盖用户已配置的环境变量。
				if _, exists := os.LookupEnv(key); !exists {
					_ = os.Setenv(key, val)
				}
			}
		}()
		return
	}
}

func getDracoTokenFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("DRACO_LOCAL")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("DRACO_TOKEN")); v != "" {
		return v
	}
	return ""
}

func solveOnceWithRecover(ctx context.Context, ts int64, token string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("发生 panic：%v", r)
		}
	}()
	return SolveAliyunCaptchaWithContext(ctx, ts, token)
}

func TestSolveAliyunCaptcha_FromEnv(t *testing.T) {
	loadLocalDotEnvForTest(t)

	// 开启验证码点击方式调试输出：仅在 go test -v 时启用，避免默认跑测试时刷屏。
	if testing.Verbose() {
		if _, ok := os.LookupEnv("SNIPING_ENGINE_CAPTCHA_DEBUG"); !ok {
			_ = os.Setenv("SNIPING_ENGINE_CAPTCHA_DEBUG", "1")
		}
	}

	token := getDracoTokenFromEnv()
	if token == "" {
		t.Skip("未配置 draco_local：请在 backend/.env 写入 DRACO_LOCAL=...（或设置环境变量 DRACO_LOCAL/DRACO_TOKEN）")
	}

	if err := WarmupCaptchaBrowser(); err != nil {
		t.Fatalf("验证码浏览器预热失败：%v", err)
	}
	defer func() { _ = CloseCaptchaBrowser() }()

	// 这个测试属于“本地手动验证”，需要重试：验证失败后继续下一次，直到成功或总超时。
	// Go 自带的 `go test` 默认超时是 10 分钟，这里把整体控制在 9 分钟以内，避免把整套测试卡死。
	overallCtx, overallCancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer overallCancel()

	var lastErr error
	for attempt := 1; ; attempt++ {
		select {
		case <-overallCtx.Done():
			if lastErr != nil {
				t.Fatalf("多次重试仍失败：%v", lastErr)
			}
			t.Fatalf("多次重试仍失败：整体超时")
		default:
		}

		// 每次尝试单独限制耗时，避免某一次卡住拖垮整体。
		attemptCtx, attemptCancel := context.WithTimeout(overallCtx, 150*time.Second)
		ts := time.Now().UnixMilli()
		result, err := solveOnceWithRecover(attemptCtx, ts, token)
		attemptCancel()

		if err == nil && strings.TrimSpace(result) != "" {
			t.Logf("SolveAliyunCaptcha 成功（第 %d 次），结果长度：%d", attempt, len(result))
			return
		}

		lastErr = err
		if err != nil {
			t.Logf("第 %d 次失败：%v", attempt, err)
		} else {
			t.Logf("第 %d 次失败：返回结果为空", attempt)
		}

		// 简单退避，避免过于频繁刷新页面/触发风控。
		time.Sleep(800 * time.Millisecond)
	}
}
