package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// --- 并发配置 ---
var (
	captchaSemaphoreMu sync.RWMutex
	captchaSemaphore   = make(chan struct{}, 1)
)

type CaptchaEngineState string

const (
	CaptchaEngineStateStopped  CaptchaEngineState = "stopped"
	CaptchaEngineStateStarting CaptchaEngineState = "starting"
	CaptchaEngineStateReady    CaptchaEngineState = "ready"
	CaptchaEngineStateError    CaptchaEngineState = "error"
)

type CaptchaEngineStatus struct {
	State         CaptchaEngineState `json:"state"`
	StartedAtMs   int64              `json:"startedAtMs"`
	ReadyAtMs     int64              `json:"readyAtMs"`
	LastError     string             `json:"lastError,omitempty"`
	WarmPages     int                `json:"warmPages"`
	PagePoolSize  int                `json:"pagePoolSize"`
	SolveCount    int64              `json:"solveCount"`
	TotalSolveMs  int64              `json:"totalSolveMs"`
	LastSolveAtMs int64              `json:"lastSolveAtMs"`
	LastSolveMs   int64              `json:"lastSolveMs"`
	LastAttempts  int64              `json:"lastAttempts"`
	GoRoutines    int                `json:"goRoutines"`
}

type CaptchaSolveMetrics struct {
	Attempts int           `json:"attempts"`
	Duration time.Duration `json:"duration"`
}

// SetCaptchaMaxConcurrent 设置验证码求解（无头浏览器）的并发数上限。
// n <= 0 时会自动按 1 处理。
func SetCaptchaMaxConcurrent(n int) {
	if n <= 0 {
		n = 1
	}
	captchaSemaphoreMu.Lock()
	captchaSemaphore = make(chan struct{}, n)
	captchaSemaphoreMu.Unlock()
}

func acquireCaptchaSlot(ctx context.Context) (func(), error) {
	captchaSemaphoreMu.RLock()
	sem := captchaSemaphore
	captchaSemaphoreMu.RUnlock()

	select {
	case sem <- struct{}{}:
		return func() {
			select {
			case <-sem:
			default:
			}
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

const (
	JfbymToken  = "DAxk0GILbeSmlvuC_bf-ak99PB7rMPEflWi6JKJvwmE"
	JfbymApiUrl = "http://api.jfbym.com/api/YmServer/customApi"
	JfbymType   = "20111"

	// 滑动偏移量（如需要可调）。
	SlideOffset = 0.0
)

// 无头模式开关：默认 true（生产环境）。
// 如需本地调试打开浏览器窗口，可设置环境变量：SNIPING_ENGINE_CAPTCHA_HEADLESS=0
var HeadlessMode = func() bool {
	v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_HEADLESS"))
	if v == "" {
		return true
	}
	v = strings.ToLower(v)
	return !(v == "0" || v == "false" || v == "no" || v == "off")
}()

type solveRequest struct {
	SlideImage      string `json:"slide_image"`
	BackgroundImage string `json:"background_image"`
	Token           string `json:"token"`
	Type            string `json:"type"`
}

type solveResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type solveItem struct {
	Code int    `json:"code"`
	Data string `json:"data"`
}

type AliResult struct {
	Result struct {
		CertifyId     string `json:"certifyId"`
		SceneId       string `json:"sceneId"`
		IsSign        bool   `json:"isSign"`
		SecurityToken string `json:"securityToken"`
		VerifyResult  *bool  `json:"VerifyResult"`
	} `json:"Result"`
}

type OutputResult struct {
	CertifyId     string `json:"certifyId"`
	SceneId       string `json:"sceneId"`
	IsSign        bool   `json:"isSign"`
	SecurityToken string `json:"securityToken"`
}

// Point 坐标点。
type Point struct {
	X, Y float64
}

// --- 浏览器与 HTTP Client 复用 ---
var (
	captchaBrowserMu       sync.Mutex
	captchaBrowser         *rod.Browser
	captchaBrowserLauncher *launcher.Launcher

	// 复用 HTTP Client，利用 Keep-Alive 连接池，减少 TCP/TLS 握手开销。
	captchaHTTPClient = newCaptchaHTTPClient()

	captchaSleepScaleOnce sync.Once
	captchaSleepScaleVal  float64

	captchaPagePoolMu sync.Mutex
	captchaPagePool   []*captchaPage

	captchaEngineMu      sync.RWMutex
	captchaEngineState   CaptchaEngineState = CaptchaEngineStateStopped
	captchaEngineStarted int64
	captchaEngineReadyAt int64
	captchaEngineErr     string
	captchaEngineWarm    int

	captchaSolveCount    atomic.Int64
	captchaSolveTotalMs  atomic.Int64
	captchaLastSolveAtMs atomic.Int64
	captchaLastSolveMs   atomic.Int64
	captchaLastAttempts  atomic.Int64
)

type captchaPage struct {
	incognito *rod.Browser
	page      *rod.Page
}

func SetCaptchaEngineState(state CaptchaEngineState, errText string, warmPages int) {
	now := time.Now().UnixMilli()

	captchaEngineMu.Lock()
	defer captchaEngineMu.Unlock()

	if state == CaptchaEngineStateStarting {
		captchaEngineStarted = now
		captchaEngineReadyAt = 0
	}
	if state == CaptchaEngineStateReady {
		if captchaEngineStarted == 0 {
			captchaEngineStarted = now
		}
		captchaEngineReadyAt = now
	}
	if state == CaptchaEngineStateError {
		if captchaEngineStarted == 0 {
			captchaEngineStarted = now
		}
	}
	captchaEngineState = state
	captchaEngineErr = strings.TrimSpace(errText)
	if warmPages > 0 {
		captchaEngineWarm = warmPages
	}
}

func GetCaptchaEngineStatus() CaptchaEngineStatus {
	captchaPagePoolMu.Lock()
	poolSize := len(captchaPagePool)
	captchaPagePoolMu.Unlock()

	captchaEngineMu.RLock()
	state := captchaEngineState
	startedAt := captchaEngineStarted
	readyAt := captchaEngineReadyAt
	lastErr := captchaEngineErr
	warm := captchaEngineWarm
	captchaEngineMu.RUnlock()

	return CaptchaEngineStatus{
		State:         state,
		StartedAtMs:   startedAt,
		ReadyAtMs:     readyAt,
		LastError:     lastErr,
		WarmPages:     warm,
		PagePoolSize:  poolSize,
		SolveCount:    captchaSolveCount.Load(),
		TotalSolveMs:  captchaSolveTotalMs.Load(),
		LastSolveAtMs: captchaLastSolveAtMs.Load(),
		LastSolveMs:   captchaLastSolveMs.Load(),
		LastAttempts:  captchaLastAttempts.Load(),
		GoRoutines:    runtime.NumGoroutine(),
	}
}

// WarmupCaptchaBrowser 预热验证码浏览器（可选）。
// 不调用也没关系，首次 SolveAliyunCaptcha 时会自动初始化。
func WarmupCaptchaBrowser() error {
	_, err := getCaptchaBrowser()
	return err
}

// WarmupCaptchaEngine 启动并预热验证码引擎：
// - 启动全局浏览器
// - 预创建一定数量的页面放入池中（减少首次使用延迟）
func WarmupCaptchaEngine(maxWarmPages int) error {
	// warmPages 默认跟随配置的并发上限，但要限制一个合理的上限，避免占用太多资源。
	warmPages := maxWarmPages
	if warmPages <= 0 {
		warmPages = 1
	}
	if warmPages > 6 {
		warmPages = 6
	}
	if v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_WARM_PAGES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 8 {
			warmPages = n
		}
	}

	SetCaptchaEngineState(CaptchaEngineStateStarting, "", warmPages)

	if err := WarmupCaptchaBrowser(); err != nil {
		SetCaptchaEngineState(CaptchaEngineStateError, err.Error(), warmPages)
		return err
	}

	// 预热页面池：创建 warmPages 个页面并归还到池里。
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for i := 0; i < warmPages; i++ {
		cp, _, err := acquireCaptchaPage(ctx)
		if err != nil {
			SetCaptchaEngineState(CaptchaEngineStateError, err.Error(), warmPages)
			return err
		}
		releaseCaptchaPage(cp)
	}

	SetCaptchaEngineState(CaptchaEngineStateReady, "", warmPages)
	return nil
}

// CloseCaptchaBrowser 关闭全局验证码浏览器（通常在进程退出时调用）。
func CloseCaptchaBrowser() error {
	captchaBrowserMu.Lock()
	defer captchaBrowserMu.Unlock()

	var firstErr error
	captchaPagePoolMu.Lock()
	for _, p := range captchaPagePool {
		if p == nil {
			continue
		}
		if p.page != nil {
			_ = p.page.Close()
		}
		if p.incognito != nil {
			_ = p.incognito.Close()
		}
	}
	captchaPagePool = nil
	captchaPagePoolMu.Unlock()

	if captchaBrowser != nil {
		if err := captchaBrowser.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		captchaBrowser = nil
	}
	if captchaBrowserLauncher != nil {
		captchaBrowserLauncher.Kill()
		captchaBrowserLauncher = nil
	}
	return firstErr
}

func getCaptchaBrowser() (*rod.Browser, error) {
	captchaBrowserMu.Lock()
	defer captchaBrowserMu.Unlock()

	if captchaBrowser != nil {
		return captchaBrowser, nil
	}

	l := launcher.New().Headless(HeadlessMode)
	u, err := l.Launch()
	if err != nil {
		l.Kill()
		return nil, err
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		l.Kill()
		return nil, err
	}

	captchaBrowser = b
	captchaBrowserLauncher = l
	return captchaBrowser, nil
}

func newCaptchaHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}

func captchaSleepScale() float64 {
	captchaSleepScaleOnce.Do(func() {
		// 默认 1.0；设置 SNIPING_ENGINE_CAPTCHA_FAST=1 则会更快（更短等待）。
		captchaSleepScaleVal = 1.0

		fast := strings.EqualFold(strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_FAST")), "1") ||
			strings.EqualFold(strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_FAST")), "true")
		if fast {
			captchaSleepScaleVal = 0.35
		}

		if v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_SLEEP_SCALE")); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1.0 {
				captchaSleepScaleVal = f
			}
		}
	})
	return captchaSleepScaleVal
}

func captchaSleep(base time.Duration, jitter time.Duration) {
	d := base
	if jitter > 0 {
		d += time.Duration(rand.Int63n(int64(jitter) + 1))
	}
	scale := captchaSleepScale()
	if scale > 0 && scale < 1 {
		d = time.Duration(float64(d) * scale)
	}
	if d > 0 {
		time.Sleep(d)
	}
}

func drainFloat64Chan(ch chan float64) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func drainStringChan(ch chan string) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func acquireCaptchaPage(ctx context.Context) (*captchaPage, *rod.Page, error) {
	captchaPagePoolMu.Lock()
	n := len(captchaPagePool)
	if n > 0 {
		cp := captchaPagePool[n-1]
		captchaPagePool = captchaPagePool[:n-1]
		captchaPagePoolMu.Unlock()
		if cp != nil && cp.page != nil {
			return cp, cp.page.Context(ctx), nil
		}
	} else {
		captchaPagePoolMu.Unlock()
	}

	mainBrowser, err := getCaptchaBrowser()
	if err != nil {
		return nil, nil, err
	}

	incognito, err := mainBrowser.Incognito()
	if err != nil {
		return nil, nil, err
	}

	var page *rod.Page
	if err := rod.Try(func() {
		page = stealth.MustPage(incognito)
		page.MustEmulate(devices.IPhoneX)
	}); err != nil {
		_ = incognito.Close()
		return nil, nil, err
	}

	cp := &captchaPage{incognito: incognito, page: page}
	return cp, page.Context(ctx), nil
}

func releaseCaptchaPage(cp *captchaPage) {
	if cp == nil || cp.page == nil {
		return
	}

	// 复用页面时做最小清理，避免下次复用落在上一次的页面状态里。
	_ = rod.Try(func() {
		p := cp.page.Context(context.Background()).Timeout(2 * time.Second)
		_ = p.Navigate("about:blank")
	})

	captchaPagePoolMu.Lock()
	captchaPagePool = append(captchaPagePool, cp)
	captchaPagePoolMu.Unlock()
}

func clickCaptchaButton(page *rod.Page) error {
	debugEnabled := func() bool {
		v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_DEBUG"))
		return strings.EqualFold(v, "1") || strings.EqualFold(v, "true")
	}
	debugf := func(format string, args ...any) {
		if !debugEnabled() {
			return
		}
		fmt.Printf("[验证码调试] "+format+"\n", args...)
	}

	isSliderReady := func(p *rod.Page) bool {
		el, err := p.Timeout(300 * time.Millisecond).Element("#aliyunCaptcha-sliding-slider")
		if err != nil {
			return false
		}
		v, _ := el.Visible()
		return v
	}

	waitSliderReady := func(p *rod.Page, d time.Duration) bool {
		deadline := time.Now().Add(d)
		for time.Now().Before(deadline) {
			if isSliderReady(p) {
				return true
			}
			captchaSleep(80*time.Millisecond, 0)
		}
		return false
	}

	clickByID := func(p *rod.Page) bool {
		res, err := p.Timeout(300 * time.Millisecond).Eval(`() => {
			const btn = document.getElementById('button');
			if (!btn) return false;
			try { btn.scrollIntoView({block: 'center', inline: 'center'}); } catch (e) {}
			try { btn.click(); } catch (e) { return false; }
			return true;
		}`)
		return err == nil && res != nil && res.Value.Bool()
	}

	if isSliderReady(page) {
		return nil
	}

	// “按钮出现就立刻点”：不要长时间等待 MustWaitVisible，否则会感觉有延迟。
	// 这里采用短间隔循环：能点就点，点完快速检查滑块是否出现；没出现就继续点直到超时。
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if isSliderReady(page) {
			return nil
		}

		clicked := false

		// 先用 JS 点击（最快，不依赖鼠标坐标/可见性等待）。
		if clickByID(page) {
			clicked = true
			debugf("已尝试点击：JS 点击（#button）")
			if waitSliderReady(page, 900*time.Millisecond) {
				debugf("已进入滑块阶段：JS 点击（#button）")
				return nil
			}
		}

		// 再用 Rod 点击（有些页面会过滤纯 JS click，这里兜底一下）。
		if el, err := page.Timeout(200 * time.Millisecond).Element("#button"); err == nil {
			_ = rod.Try(func() {
				_ = el.ScrollIntoView()
				el.MustClick()
			})
			clicked = true
			debugf("已尝试点击：Rod 点击（#button）")
			if waitSliderReady(page, 900*time.Millisecond) {
				debugf("已进入滑块阶段：Rod 点击（#button）")
				return nil
			}
		}

		if !clicked {
			// 按钮还没出现在 DOM 里，短暂等待即可。
			captchaSleep(60*time.Millisecond, 20*time.Millisecond)
			continue
		}

		// 已点过但还没进入滑块阶段：稍微等待一下再继续下一轮点击。
		captchaSleep(120*time.Millisecond, 40*time.Millisecond)
	}

	return errors.New("未能自动点击“安全验证”按钮（或点击后未进入滑块阶段）")
}

func extractSceneID(page *rod.Page) string {
	result, err := page.Eval(`() => {
		let scripts = document.getElementsByTagName('script');
		for (let s of scripts) {
			let match = s.textContent.match(/SceneId:\s*["']([^"']+)["']/);
			if (match) return match[1];
		}
		return '';
	}`)
	if err != nil {
		return ""
	}
	return result.Value.Str()
}

func navigateCaptchaPage(page *rod.Page, targetURL string) error {
	waitDom := page.WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)
	if err := page.Navigate(targetURL); err != nil {
		return err
	}
	waitDom()
	return nil
}

// SolveAliyunCaptcha 执行验证码验证并返回 Base64 编码的结果。
func SolveAliyunCaptcha(timestamp int64, dracoToken string) (string, error) {
	return SolveAliyunCaptchaWithContext(context.Background(), timestamp, dracoToken)
}

func SolveAliyunCaptchaWithMetrics(parent context.Context, timestamp int64, dracoToken string) (string, CaptchaSolveMetrics, error) {
	return solveAliyunCaptchaWithMetrics(parent, timestamp, dracoToken)
}

// SolveAliyunCaptchaWithContext 执行验证码验证并返回 Base64 编码的结果（支持 ctx 取消）。
func SolveAliyunCaptchaWithContext(parent context.Context, timestamp int64, dracoToken string) (string, error) {
	result, _, err := solveAliyunCaptchaWithMetrics(parent, timestamp, dracoToken)
	return result, err
}

func solveAliyunCaptchaWithMetrics(parent context.Context, timestamp int64, dracoToken string) (string, CaptchaSolveMetrics, error) {
	rand.Seed(time.Now().UnixNano())
	started := time.Now()
	metrics := CaptchaSolveMetrics{Attempts: 0, Duration: 0}

	ctx, cancel := context.WithTimeout(parent, 360*time.Second)
	defer cancel()

	release, err := acquireCaptchaSlot(ctx)
	if err != nil {
		return "", metrics, err
	}
	defer release()

	makeTargetURL := func(attempt int) string {
		t := timestamp
		if attempt > 1 {
			t = time.Now().UnixMilli()
		}
		// 额外带一个随机参数，避免中间层缓存导致不刷新验证码资源。
		return fmt.Sprintf("https://m.4008117117.com/aliyun-captcha?t=%d&cookie=true&draco_local=%s&r=%d", t, dracoToken, rand.Int63())
	}

	cp, page, err := acquireCaptchaPage(ctx)
	if err != nil {
		return "", metrics, err
	}
	defer releaseCaptchaPage(cp)

	// --- 状态 ---
	var (
		mu           sync.Mutex
		backB64      string
		shadowB64    string
		hasTriggered bool

		pageSceneID   string
		verifySuccess bool
		finalResult   string
		lastErr       error
	)

	apiXCh := make(chan float64, 10)
	verifyResultCh := make(chan string, 10)

	resetState := func() {
		mu.Lock()
		backB64 = ""
		shadowB64 = ""
		hasTriggered = false
		mu.Unlock()

		drainFloat64Chan(apiXCh)
		drainStringChan(verifyResultCh)
	}

	checkAndSolve := func() {
		mu.Lock()
		if hasTriggered || backB64 == "" || shadowB64 == "" {
			mu.Unlock()
			return
		}
		hasTriggered = true
		slide := shadowB64
		bg := backB64
		mu.Unlock()

		go func() {
			reqBody := solveRequest{
				SlideImage:      slide,
				BackgroundImage: bg,
				Token:           JfbymToken,
				Type:            JfbymType,
			}
			bs, _ := json.Marshal(reqBody)

			resp, err := captchaHTTPClient.Post(JfbymApiUrl, "application/json", bytes.NewReader(bs))
			if err != nil {
				return
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			var sr solveResponse
			if err := json.Unmarshal(respBody, &sr); err != nil {
				return
			}

			var items []solveItem
			_ = json.Unmarshal(sr.Data, &items)
			if len(items) == 0 {
				var single solveItem
				if json.Unmarshal(sr.Data, &single) == nil {
					items = append(items, single)
				}
			}

			for _, d := range items {
				if d.Code != 0 {
					continue
				}
				val, err := strconv.ParseFloat(d.Data, 64)
				if err != nil {
					continue
				}
				select {
				case apiXCh <- val:
				default:
				}
				return
			}
		}()
	}

	// --- 请求拦截：丢弃非必须资源（加速 Load 与并发能力）---
	router := page.HijackRequests()
	defer func() { _ = router.Stop() }()

	// 注意：拦截过多资源可能导致验证码页面“白屏/不渲染”。
	// 默认不做额外拦截；如你确认页面能正常显示，再通过环境变量开启：
	// SNIPING_ENGINE_CAPTCHA_BLOCK_RESOURCES=1
	blockResources := strings.EqualFold(strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_BLOCK_RESOURCES")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_BLOCK_RESOURCES")), "true")
	if blockResources {
		router.MustAdd("*", func(ctx *rod.Hijack) {
			u := ctx.Request.URL().String()
			if strings.Contains(u, "back.png") ||
				strings.Contains(u, "shadow.png") ||
				strings.Contains(u, "captcha-open.aliyuncs.com") {
				ctx.Skip = true
				return
			}

			switch ctx.Request.Type() {
			// 不要拦截得太激进，否则验证码页面可能渲染不出来（尤其是某些脚本依赖样式/图片）。
			// 目前只丢弃“字体/媒体”两类资源，兼顾稳定性与速度。
			case proto.NetworkResourceTypeFont,
				proto.NetworkResourceTypeMedia:
				ctx.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
				return
			default:
				ctx.ContinueRequest(&proto.FetchContinueRequest{})
				return
			}
		})
	}

	router.MustAdd("*back.png*", func(ctx *rod.Hijack) {
		_ = ctx.LoadResponse(captchaHTTPClient, true)
		body := ctx.Response.Payload().Body
		if len(body) == 0 {
			return
		}
		b64 := base64.StdEncoding.EncodeToString(body)
		mu.Lock()
		backB64 = b64
		mu.Unlock()
		checkAndSolve()
	})

	router.MustAdd("*shadow.png*", func(ctx *rod.Hijack) {
		_ = ctx.LoadResponse(captchaHTTPClient, true)
		body := ctx.Response.Payload().Body
		if len(body) == 0 {
			return
		}
		b64 := base64.StdEncoding.EncodeToString(body)
		mu.Lock()
		shadowB64 = b64
		mu.Unlock()
		checkAndSolve()
	})

	router.MustAdd("*captcha-open.aliyuncs.com*", func(ctx *rod.Hijack) {
		_ = ctx.LoadResponse(captchaHTTPClient, true)
		body := ctx.Response.Payload().Body

		var res AliResult
		if json.Unmarshal(body, &res) != nil {
			return
		}

		if res.Result.VerifyResult == nil {
			return
		}

		if *res.Result.VerifyResult && res.Result.SecurityToken != "" {
			sceneID := pageSceneID
			if sceneID == "" {
				sceneID = res.Result.SceneId
			}
			output := OutputResult{
				CertifyId:     res.Result.CertifyId,
				SceneId:       sceneID,
				IsSign:        true,
				SecurityToken: res.Result.SecurityToken,
			}
			orderedJSON, _ := json.Marshal(output)
			jsonBase64 := base64.StdEncoding.EncodeToString(orderedJSON)
			select {
			case verifyResultCh <- jsonBase64:
			default:
			}
			return
		}

		if !*res.Result.VerifyResult {
			select {
			case verifyResultCh <- "":
			default:
			}
		}
	})

	go router.Run()

	// --- 打开页面：只等 DOMContentLoaded 即可（不等图片等资源）---
	if err := navigateCaptchaPage(page, makeTargetURL(1)); err != nil {
		metrics.Duration = time.Since(started)
		return "", metrics, fmt.Errorf("打开页面失败: %v", err)
	}
	pageSceneID = extractSceneID(page)

	// --- 验证循环 ---
	for tryCount := 1; !verifySuccess; tryCount++ {
		metrics.Attempts = tryCount
		select {
		case <-ctx.Done():
			if lastErr != nil {
				metrics.Duration = time.Since(started)
				return "", metrics, lastErr
			}
			metrics.Duration = time.Since(started)
			return "", metrics, errors.New("验证码流程超时")
		default:
		}

		// 验证失败后需要“换一张新图”再滑动：每次重试都重新加载页面，确保 back/shadow 会重新请求。
		resetState()
		if tryCount > 1 {
			if err := navigateCaptchaPage(page, makeTargetURL(tryCount)); err != nil {
				lastErr = err
				continue
			}
			pageSceneID = extractSceneID(page)
		}

		// 1) 点击按钮打开验证码（Rod 内置等待机制）。
		if err := clickCaptchaButton(page); err != nil {
			lastErr = err
			continue
		}

		// 2) 等待滑块出现。
		sliderEl, err := page.Timeout(15 * time.Second).Element("#aliyunCaptcha-sliding-slider")
		if err != nil {
			lastErr = err
			continue
		}
		if err := rod.Try(func() { sliderEl.Timeout(15 * time.Second).MustWaitVisible() }); err != nil {
			lastErr = err
			continue
		}

		// 3) 等待打码结果。
		var apiX float64
		select {
		case apiX = <-apiXCh:
		case <-time.After(25 * time.Second):
			lastErr = errors.New("等待打码结果超时")
			continue
		case <-ctx.Done():
			metrics.Duration = time.Since(started)
			return "", metrics, errors.New("等待打码结果超时")
		}

		offset := (rand.Float64() * 0.2) - 0.1
		finalDistance := apiX + SlideOffset + offset

		// 获取起点（滑块中心点）。
		shape, err := sliderEl.Shape()
		if err != nil || shape == nil || shape.Box() == nil {
			lastErr = errors.New("获取滑块坐标失败")
			continue
		}
		box := shape.Box()
		startX := box.X + box.Width/2
		startY := box.Y + box.Height/2

		// 按下滑块。
		page.Mouse.MustMoveTo(startX, startY)
		captchaSleep(60*time.Millisecond, 20*time.Millisecond)
		page.Mouse.MustDown(proto.InputMouseButtonLeft)
		captchaSleep(30*time.Millisecond, 20*time.Millisecond)

		getPuzzlePos := func() float64 {
			res, _ := page.Eval(`() => {
				let el = document.querySelector('#aliyunCaptcha-puzzle');
				if (!el) return -1;
				let left = parseFloat(el.style.left) || 0;
				if (left === 0) {
					let transform = el.style.transform;
					let match = transform.match(/translate\(([-\d.]+)px/);
					if (match) return parseFloat(match[1]);
				}
				return left;
			}`)
			return res.Value.Num()
		}

		// 先移动到理论位置，再做自适应微调。
		currentMouseX := startX + finalDistance
		page.Mouse.MustMoveTo(currentMouseX, startY)
		captchaSleep(120*time.Millisecond, 40*time.Millisecond)

		targetPuzzlePos := finalDistance
		tolerance := 1.0
		maxAttempts := 30
		success := false

		for attempt := 0; attempt < maxAttempts; attempt++ {
			currentPos := getPuzzlePos()
			diff := targetPuzzlePos - currentPos
			if math.Abs(diff) <= tolerance {
				success = true
				break
			}

			dampingFactor := 0.5
			absDiff := math.Abs(diff)
			if absDiff < 3 {
				dampingFactor = 0.9
			} else if absDiff < 10 {
				dampingFactor = 0.7
			}

			moveStep := diff * dampingFactor
			if moveStep > 30 {
				moveStep = 30
			} else if moveStep < -30 {
				moveStep = -30
			}
			currentMouseX += moveStep

			randomY := startY + (rand.Float64()*2 - 1)
			page.Mouse.MustMoveTo(currentMouseX, randomY)
			captchaSleep(80*time.Millisecond, 40*time.Millisecond)
		}

		// 松开滑块。
		_ = success
		captchaSleep(160*time.Millisecond, 80*time.Millisecond)
		page.Mouse.MustUp(proto.InputMouseButtonLeft)

		// 等待验证结果（由接口回包触发）。
		select {
		case resStr := <-verifyResultCh:
			if resStr != "" {
				verifySuccess = true
				finalResult = resStr
				break
			}
			lastErr = errors.New("验证失败")
			captchaSleep(350*time.Millisecond, 150*time.Millisecond)
		case <-time.After(6 * time.Second):
			lastErr = errors.New("等待验证结果超时")
			captchaSleep(350*time.Millisecond, 150*time.Millisecond)
		case <-ctx.Done():
			metrics.Duration = time.Since(started)
			return "", metrics, errors.New("等待验证结果超时")
		}
	}

	if verifySuccess {
		metrics.Duration = time.Since(started)
		captchaSolveCount.Add(1)
		captchaSolveTotalMs.Add(metrics.Duration.Milliseconds())
		captchaLastSolveAtMs.Store(time.Now().UnixMilli())
		captchaLastSolveMs.Store(metrics.Duration.Milliseconds())
		captchaLastAttempts.Store(int64(metrics.Attempts))
		return finalResult, metrics, nil
	}
	if lastErr != nil {
		metrics.Duration = time.Since(started)
		return "", metrics, lastErr
	}
	metrics.Duration = time.Since(started)
	return "", metrics, errors.New("验证码验证失败")
}

// 生成贝塞尔曲线轨迹。
func generateBezierTrack(startX, startY, endX, endY float64, steps int) []Point {
	var track []Point

	cx1 := startX + (endX-startX)/4
	cy1 := startY + (rand.Float64()-0.5)*2

	cx2 := startX + (endX-startX)*3/4
	cy2 := startY + (rand.Float64()-0.5)*2

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := math.Pow(1-t, 3)*startX +
			3*math.Pow(1-t, 2)*t*cx1 +
			3*(1-t)*math.Pow(t, 2)*cx2 +
			math.Pow(t, 3)*endX

		y := math.Pow(1-t, 3)*startY +
			3*math.Pow(1-t, 2)*t*cy1 +
			3*(1-t)*math.Pow(t, 2)*cy2 +
			math.Pow(t, 3)*endY

		track = append(track, Point{x, y})
	}
	return track
}

// 执行轨迹移动。
func executeTrack(page *rod.Page, track []Point) {
	for _, p := range track {
		page.Mouse.MustMoveTo(p.X, p.Y)
		if rand.Intn(10) > 7 {
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Millisecond)
		}
	}
}
