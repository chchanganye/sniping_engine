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
	"net/url"
	"os"
	"os/exec"
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

const aliyunCaptchaTargetURL = "https://m.4008117117.com/aliyun-captcha&cookie=true"

type CaptchaEngineStatus struct {
	State         CaptchaEngineState `json:"state"`
	StartedAtMs   int64              `json:"startedAtMs"`
	ReadyAtMs     int64              `json:"readyAtMs"`
	LastError     string             `json:"lastError,omitempty"`
	WarmPages     int                `json:"warmPages"`
	PagePoolSize  int                `json:"pagePoolSize"`
	TotalPages    int                `json:"totalPages"`
	IdlePages     int                `json:"idlePages"`
	BusyPages     int                `json:"busyPages"`
	Refreshing    int                `json:"refreshingPages"`
	SolveCount    int64              `json:"solveCount"`
	TotalSolveMs  int64              `json:"totalSolveMs"`
	LastSolveAtMs int64              `json:"lastSolveAtMs"`
	LastSolveMs   int64              `json:"lastSolveMs"`
	LastAttempts  int64              `json:"lastAttempts"`
	GoRoutines    int                `json:"goRoutines"`
}

type CaptchaPageInfo struct {
	ID             string `json:"id"`
	State          string `json:"state"`
	CreatedAtMs    int64  `json:"createdAtMs"`
	LastUsedAtMs   int64  `json:"lastUsedAtMs"`
	LastOpenedAtMs int64  `json:"lastOpenedAtMs"`
	LastError      string `json:"lastError,omitempty"`
}

type CaptchaPagesStatus struct {
	NowMs       int64             `json:"nowMs"`
	Total       int               `json:"total"`
	Idle        int               `json:"idle"`
	Busy        int               `json:"busy"`
	Refreshing  int               `json:"refreshing"`
	PagePool    int               `json:"pagePool"`
	Pages       []CaptchaPageInfo `json:"pages"`
}

type CaptchaPagesRefreshOptions struct {
	ForceRecreate bool
	EnsurePages   int
}

type CaptchaPagesRefreshResult struct {
	Refreshed int `json:"refreshed"`
	Recreated int `json:"recreated"`
	Failed    int `json:"failed"`
}

type CaptchaStopAllResult struct {
	AtMs int64 `json:"atMs"`
	Busy int   `json:"busy"`
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

// captchaHeadlessMode 无头模式开关：默认 true（生产环境）。
// 如需本地调试打开浏览器窗口，可设置环境变量：SNIPING_ENGINE_CAPTCHA_HEADLESS=0
//
// 注意：这里必须“动态读取环境变量”，不能在包初始化时只读一次。
// 因为本项目的本地验证码测试会从 backend/.env 注入环境变量，而注入发生在测试用例开始时。
func captchaHeadlessMode() bool {
	v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_HEADLESS"))
	if v == "" {
		return true
	}
	v = strings.ToLower(v)
	return !(v == "0" || v == "false" || v == "no" || v == "off")
}

type solveRequest struct {
	SlideImage      string `json:"slide_image"`
	BackgroundImage string `json:"background_image"`
	Token           string `json:"token"`
	Type            string `json:"type"`
}

type solveResponse struct {
	Code json.RawMessage `json:"code"`
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

	captchaEngineStateChMu sync.Mutex
	captchaEngineStateCh   = make(chan struct{})

	captchaWarmupMu      sync.Mutex
	captchaWarmupRunning bool

	captchaStopMu     sync.Mutex
	captchaStopOnce   sync.Once
	captchaStopCtx    context.Context
	captchaStopCancel context.CancelFunc

	captchaSolveCount    atomic.Int64
	captchaSolveTotalMs  atomic.Int64
	captchaLastSolveAtMs atomic.Int64
	captchaLastSolveMs   atomic.Int64
	captchaLastAttempts  atomic.Int64
)

type captchaPage struct {
	id          string
	createdAtMs int64

	incognito *rod.Browser
	page      *rod.Page

	state          atomic.Int32 // 0=idle 1=busy 2=refreshing
	lastUsedAtMs   atomic.Int64
	lastOpenedAtMs atomic.Int64
	lastError      atomic.Value // string
}

const (
	captchaPageStateIdle int32 = iota
	captchaPageStateBusy
	captchaPageStateRefreshing
)

func (cp *captchaPage) stateString() string {
	if cp == nil {
		return "unknown"
	}
	switch cp.state.Load() {
	case captchaPageStateIdle:
		return "idle"
	case captchaPageStateBusy:
		return "busy"
	case captchaPageStateRefreshing:
		return "refreshing"
	default:
		return "unknown"
	}
}

func (cp *captchaPage) lastErrorString() string {
	if cp == nil {
		return ""
	}
	v := cp.lastError.Load()
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

var (
	captchaPagesMu  sync.Mutex
	captchaPagesAll []*captchaPage
	captchaPageSeq  atomic.Uint64
)

func closeChanSafe(ch chan struct{}) {
	defer func() { _ = recover() }()
	close(ch)
}

func broadcastCaptchaEngineStateChanged() {
	captchaEngineStateChMu.Lock()
	closeChanSafe(captchaEngineStateCh)
	captchaEngineStateCh = make(chan struct{})
	captchaEngineStateChMu.Unlock()
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

	broadcastCaptchaEngineStateChanged()
}

func GetCaptchaEngineStatus() CaptchaEngineStatus {
	captchaPagePoolMu.Lock()
	poolSize := len(captchaPagePool)
	captchaPagePoolMu.Unlock()

	captchaPagesMu.Lock()
	all := make([]*captchaPage, len(captchaPagesAll))
	copy(all, captchaPagesAll)
	captchaPagesMu.Unlock()

	idle := 0
	busy := 0
	refreshing := 0
	for _, p := range all {
		switch p.state.Load() {
		case captchaPageStateIdle:
			idle++
		case captchaPageStateBusy:
			busy++
		case captchaPageStateRefreshing:
			refreshing++
		}
	}

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
		TotalPages:    len(all),
		IdlePages:     idle,
		BusyPages:     busy,
		Refreshing:    refreshing,
		SolveCount:    captchaSolveCount.Load(),
		TotalSolveMs:  captchaSolveTotalMs.Load(),
		LastSolveAtMs: captchaLastSolveAtMs.Load(),
		LastSolveMs:   captchaLastSolveMs.Load(),
		LastAttempts:  captchaLastAttempts.Load(),
		GoRoutines:    runtime.NumGoroutine(),
	}
}

// WaitCaptchaEngineReady 等待验证码引擎进入“已就绪”状态；若进入“异常”状态则返回错误。
func WaitCaptchaEngineReady(ctx context.Context) (CaptchaEngineStatus, error) {
	for {
		st := GetCaptchaEngineStatus()
		if st.State == CaptchaEngineStateReady {
			return st, nil
		}
		if st.State == CaptchaEngineStateError {
			if strings.TrimSpace(st.LastError) != "" {
				return st, errors.New(strings.TrimSpace(st.LastError))
			}
			return st, errors.New("验证码引擎启动失败")
		}

		captchaEngineStateChMu.Lock()
		ch := captchaEngineStateCh
		captchaEngineStateChMu.Unlock()

		select {
		case <-ch:
			continue
		case <-ctx.Done():
			return GetCaptchaEngineStatus(), ctx.Err()
		}
	}
}

func getCaptchaMaxConcurrent() int {
	captchaSemaphoreMu.RLock()
	sem := captchaSemaphore
	captchaSemaphoreMu.RUnlock()
	if sem == nil || cap(sem) <= 0 {
		return 1
	}
	return cap(sem)
}

// EnsureCaptchaEngineReady 确保验证码引擎已启动并就绪：
// - 若未启动/启动失败，会触发一次预热（可能需要下载浏览器）
// - 若正在启动，会等待就绪
func EnsureCaptchaEngineReady(ctx context.Context, warmPages int) (CaptchaEngineStatus, error) {
	st := GetCaptchaEngineStatus()
	if st.State == CaptchaEngineStateReady {
		return st, nil
	}
	if warmPages <= 0 {
		warmPages = st.WarmPages
	}
	if warmPages <= 0 {
		warmPages = getCaptchaMaxConcurrent()
	}

	needStart := st.State == CaptchaEngineStateStopped || st.State == CaptchaEngineStateError
	if needStart {
		captchaWarmupMu.Lock()
		st2 := GetCaptchaEngineStatus()
		if (st2.State == CaptchaEngineStateStopped || st2.State == CaptchaEngineStateError) && !captchaWarmupRunning {
			captchaWarmupRunning = true
			go func(pages int) {
				defer func() {
					captchaWarmupMu.Lock()
					captchaWarmupRunning = false
					captchaWarmupMu.Unlock()
				}()
				_ = WarmupCaptchaEngine(pages)
			}(warmPages)
		}
		captchaWarmupMu.Unlock()
	}

	return WaitCaptchaEngineReady(ctx)
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
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	for i := 0; i < warmPages; i++ {
		pageCtx, pageCancel := context.WithTimeout(ctx, 20*time.Second)
		cp, page, err := acquireCaptchaPage(pageCtx)
		if err != nil {
			pageCancel()
			SetCaptchaEngineState(CaptchaEngineStateError, err.Error(), warmPages)
			return err
		}
		if err := navigateCaptchaPage(page, aliyunCaptchaTargetURL); err != nil {
			pageCancel()
			discardCaptchaPage(cp)
			SetCaptchaEngineState(CaptchaEngineStateError, err.Error(), warmPages)
			return err
		}
		nowMs := time.Now().UnixMilli()
		cp.lastOpenedAtMs.Store(nowMs)
		cp.lastUsedAtMs.Store(nowMs)
		cp.lastError.Store("")
		pageCancel()
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
	captchaPagesMu.Lock()
	all := make([]*captchaPage, len(captchaPagesAll))
	copy(all, captchaPagesAll)
	captchaPagesAll = nil
	captchaPagesMu.Unlock()

	captchaPagePoolMu.Lock()
	captchaPagePool = nil
	captchaPagePoolMu.Unlock()

	for _, p := range all {
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

	b, l, err := launchCaptchaBrowser(captchaHeadlessMode())
	if err != nil {
		return nil, err
	}
	captchaBrowser = b
	captchaBrowserLauncher = l
	return captchaBrowser, nil
}

func detectSystemChromeBin() string {
	if v := strings.TrimSpace(os.Getenv("ROD_BROWSER_BIN")); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	if v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CHROME_BIN")); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}

	candidates := []string{
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/google-chrome-stable",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("chromium"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	if p, err := exec.LookPath("chromium-browser"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	if p, err := exec.LookPath("google-chrome"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	if p, err := exec.LookPath("google-chrome-stable"); err == nil && strings.TrimSpace(p) != "" {
		return p
	}
	return ""
}

func launchCaptchaBrowser(headless bool) (*rod.Browser, *launcher.Launcher, error) {
	l := launcher.New().Headless(headless)
	if runtime.GOOS == "linux" {
		l = l.NoSandbox(true).Set("disable-dev-shm-usage")
	}
	if bin := detectSystemChromeBin(); bin != "" {
		l = l.Bin(bin)
	}
	u, err := l.Launch()
	if err != nil {
		l.Kill()
		return nil, nil, err
	}

	b := rod.New().ControlURL(u)
	if err := b.Connect(); err != nil {
		l.Kill()
		return nil, nil, err
	}
	return b, l, nil
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

func captchaMaxSolveAttempts() int {
	v := strings.TrimSpace(os.Getenv("SNIPING_ENGINE_CAPTCHA_MAX_TRIES"))
	if v == "" {
		return 3
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 3
	}
	if n <= 0 {
		return 3
	}
	if n > 10 {
		return 10
	}
	return n
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

func GetCaptchaMaxConcurrent() int {
	return getCaptchaMaxConcurrent()
}

func getCaptchaStopContext() context.Context {
	captchaStopOnce.Do(func() {
		captchaStopMu.Lock()
		captchaStopCtx, captchaStopCancel = context.WithCancel(context.Background())
		captchaStopMu.Unlock()
	})

	captchaStopMu.Lock()
	ctx := captchaStopCtx
	captchaStopMu.Unlock()
	return ctx
}

func StopAllCaptchaFetching() CaptchaStopAllResult {
	nowMs := time.Now().UnixMilli()
	st := GetCaptchaPagesStatus()

	_ = getCaptchaStopContext()
	captchaStopMu.Lock()
	if captchaStopCancel != nil {
		captchaStopCancel()
	}
	captchaStopCtx, captchaStopCancel = context.WithCancel(context.Background())
	captchaStopMu.Unlock()

	return CaptchaStopAllResult{AtMs: nowMs, Busy: st.Busy}
}

func GetCaptchaPagesStatus() CaptchaPagesStatus {
	nowMs := time.Now().UnixMilli()

	captchaPagesMu.Lock()
	all := make([]*captchaPage, len(captchaPagesAll))
	copy(all, captchaPagesAll)
	captchaPagesMu.Unlock()

	captchaPagePoolMu.Lock()
	poolSize := len(captchaPagePool)
	captchaPagePoolMu.Unlock()

	out := CaptchaPagesStatus{
		NowMs:    nowMs,
		PagePool: poolSize,
		Pages:    make([]CaptchaPageInfo, 0, len(all)),
	}

	for _, cp := range all {
		if cp == nil {
			continue
		}
		switch cp.state.Load() {
		case captchaPageStateIdle:
			out.Idle++
		case captchaPageStateBusy:
			out.Busy++
		case captchaPageStateRefreshing:
			out.Refreshing++
		}
		out.Pages = append(out.Pages, CaptchaPageInfo{
			ID:             cp.id,
			State:          cp.stateString(),
			CreatedAtMs:    cp.createdAtMs,
			LastUsedAtMs:   cp.lastUsedAtMs.Load(),
			LastOpenedAtMs: cp.lastOpenedAtMs.Load(),
			LastError:      cp.lastErrorString(),
		})
	}
	out.Total = len(out.Pages)
	return out
}

func EnsureCaptchaPagePool(ctx context.Context, ensureTotalPages int) error {
	if ensureTotalPages <= 0 {
		return nil
	}
	if ensureTotalPages > 20 {
		ensureTotalPages = 20
	}

	captchaPagesMu.Lock()
	currentTotal := len(captchaPagesAll)
	captchaPagesMu.Unlock()

	missing := ensureTotalPages - currentTotal
	if missing <= 0 {
		return nil
	}

	for i := 0; i < missing; i++ {
		pageCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		cp, page, err := newCaptchaPage(pageCtx)
		if err != nil {
			cancel()
			return err
		}
		if err := navigateCaptchaPage(page, aliyunCaptchaTargetURL); err != nil {
			cancel()
			discardCaptchaPage(cp)
			return err
		}
		nowMs := time.Now().UnixMilli()
		cp.lastOpenedAtMs.Store(nowMs)
		cp.lastUsedAtMs.Store(nowMs)
		cp.lastError.Store("")
		releaseCaptchaPage(cp)
		cancel()
	}
	return nil
}

func RefreshCaptchaPages(ctx context.Context, opts CaptchaPagesRefreshOptions) (CaptchaPagesRefreshResult, error) {
	var res CaptchaPagesRefreshResult
	if opts.EnsurePages > 0 {
		if err := EnsureCaptchaPagePool(ctx, opts.EnsurePages); err != nil {
			return res, err
		}
	}

	captchaPagePoolMu.Lock()
	toRefresh := make([]*captchaPage, len(captchaPagePool))
	copy(toRefresh, captchaPagePool)
	captchaPagePool = nil
	captchaPagePoolMu.Unlock()

	if len(toRefresh) == 0 {
		return res, nil
	}

	for _, cp := range toRefresh {
		if cp == nil || cp.page == nil {
			continue
		}
		cp.state.Store(captchaPageStateRefreshing)
	}

	for _, cp := range toRefresh {
		if cp == nil || cp.page == nil {
			continue
		}

		if opts.ForceRecreate {
			discardCaptchaPage(cp)
			pageCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			ncp, page, err := newCaptchaPage(pageCtx)
			if err != nil {
				cancel()
				res.Failed++
				continue
			}
			if err := navigateCaptchaPage(page, aliyunCaptchaTargetURL); err != nil {
				cancel()
				discardCaptchaPage(ncp)
				res.Failed++
				continue
			}
			nowMs := time.Now().UnixMilli()
			ncp.lastOpenedAtMs.Store(nowMs)
			ncp.lastUsedAtMs.Store(nowMs)
			ncp.lastError.Store("")
			releaseCaptchaPage(ncp)
			cancel()
			res.Recreated++
			continue
		}

		pageCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		p := cp.page.Context(pageCtx)
		err := navigateCaptchaPage(p, aliyunCaptchaTargetURL)
		cancel()
		if err != nil {
			cp.lastError.Store(err.Error())
			discardCaptchaPage(cp)
			pageCtx2, cancel2 := context.WithTimeout(ctx, 25*time.Second)
			ncp, page, err2 := newCaptchaPage(pageCtx2)
			if err2 != nil {
				cancel2()
				res.Failed++
				continue
			}
			if err2 := navigateCaptchaPage(page, aliyunCaptchaTargetURL); err2 != nil {
				cancel2()
				discardCaptchaPage(ncp)
				res.Failed++
				continue
			}
			nowMs := time.Now().UnixMilli()
			ncp.lastOpenedAtMs.Store(nowMs)
			ncp.lastUsedAtMs.Store(nowMs)
			ncp.lastError.Store("")
			releaseCaptchaPage(ncp)
			cancel2()
			res.Recreated++
			continue
		}

		nowMs := time.Now().UnixMilli()
		cp.lastOpenedAtMs.Store(nowMs)
		cp.lastUsedAtMs.Store(nowMs)
		cp.lastError.Store("")
		releaseCaptchaPage(cp)
		res.Refreshed++
	}

	return res, nil
}

func registerCaptchaPage(cp *captchaPage) {
	if cp == nil {
		return
	}
	captchaPagesMu.Lock()
	captchaPagesAll = append(captchaPagesAll, cp)
	captchaPagesMu.Unlock()
}

func removeCaptchaPage(cp *captchaPage) {
	if cp == nil {
		return
	}
	captchaPagesMu.Lock()
	defer captchaPagesMu.Unlock()
	for i, p := range captchaPagesAll {
		if p == cp {
			copy(captchaPagesAll[i:], captchaPagesAll[i+1:])
			captchaPagesAll = captchaPagesAll[:len(captchaPagesAll)-1]
			return
		}
	}
}

func discardCaptchaPage(cp *captchaPage) {
	if cp == nil {
		return
	}
	removeCaptchaPage(cp)
	if cp.page != nil {
		_ = cp.page.Close()
	}
	if cp.incognito != nil {
		_ = cp.incognito.Close()
	}
	cp.page = nil
	cp.incognito = nil
	cp.state.Store(captchaPageStateIdle)
}

func newCaptchaPage(ctx context.Context) (*captchaPage, *rod.Page, error) {
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

	nowMs := time.Now().UnixMilli()
	cp := &captchaPage{
		id:          fmt.Sprintf("p-%d", captchaPageSeq.Add(1)),
		createdAtMs: nowMs,
		incognito:   incognito,
		page:        page,
	}
	cp.state.Store(captchaPageStateBusy)
	cp.lastUsedAtMs.Store(nowMs)
	cp.lastOpenedAtMs.Store(0)
	cp.lastError.Store("")
	registerCaptchaPage(cp)

	p := page.Context(ctx)
	_ = proto.NetworkEnable{}.Call(p)
	_ = proto.NetworkSetCacheDisabled{CacheDisabled: true}.Call(p)
	return cp, p, nil
}

func acquireCaptchaPage(ctx context.Context) (*captchaPage, *rod.Page, error) {
	captchaPagePoolMu.Lock()
	n := len(captchaPagePool)
	if n > 0 {
		cp := captchaPagePool[n-1]
		captchaPagePool = captchaPagePool[:n-1]
		captchaPagePoolMu.Unlock()
		if cp != nil && cp.page != nil {
			nowMs := time.Now().UnixMilli()
			cp.state.Store(captchaPageStateBusy)
			cp.lastUsedAtMs.Store(nowMs)
			p := cp.page.Context(ctx)
			_ = proto.NetworkEnable{}.Call(p)
			_ = proto.NetworkSetCacheDisabled{CacheDisabled: true}.Call(p)
			return cp, p, nil
		}
	} else {
		captchaPagePoolMu.Unlock()
	}

	return newCaptchaPage(ctx)
}

func releaseCaptchaPage(cp *captchaPage) {
	if cp == nil || cp.page == nil {
		return
	}

	// 不再归还到 about:blank：保持页面“预打开”状态，降低抢购时的首次加载延迟。
	cp.state.Store(captchaPageStateIdle)

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
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	started := time.Now()
	metrics := CaptchaSolveMetrics{Attempts: 0, Duration: 0}

	ctx, cancel := context.WithTimeout(parent, 360*time.Second)
	defer cancel()

	stopCtx := getCaptchaStopContext()
	if stopCtx != nil {
		go func() {
			select {
			case <-stopCtx.Done():
				cancel()
			case <-ctx.Done():
				return
			}
		}()
	}

	// 如果验证码引擎还没就绪（首次启动可能在下载浏览器），这里先等待，避免抢购阶段因为超时而直接失败。
	if _, err := EnsureCaptchaEngineReady(ctx, 0); err != nil {
		metrics.Duration = time.Since(started)
		return "", metrics, err
	}

	release, err := acquireCaptchaSlot(ctx)
	if err != nil {
		return "", metrics, err
	}
	defer release()

	makeTargetURL := func(_ int) string {
		return aliyunCaptchaTargetURL
	}

	cp, page, err := acquireCaptchaPage(ctx)
	if err != nil {
		return "", metrics, err
	}

	var (
		verifySuccess bool
		lastErr       error
		discardAfter  bool
	)
	defer func() {
		if cp == nil || cp.page == nil {
			return
		}
		if discardAfter {
			discardCaptchaPage(cp)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
				defer cancel()
				ncp, p, err := newCaptchaPage(ctx)
				if err != nil {
					return
				}
				if err := navigateCaptchaPage(p, aliyunCaptchaTargetURL); err != nil {
					discardCaptchaPage(ncp)
					return
				}
				nowMs := time.Now().UnixMilli()
				ncp.lastOpenedAtMs.Store(nowMs)
				ncp.lastUsedAtMs.Store(nowMs)
				ncp.lastError.Store("")
				releaseCaptchaPage(ncp)
			}()
			return
		}
		// 释放前尽量把页面“重置到可复用状态”，避免页面打开太久导致卡死/白屏。
		// 注意：HijackRequests 会在本函数返回前 Stop（defer），这里再执行 Navigate 不会残留拦截器。
		resetCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		p := cp.page.Context(resetCtx)
		err := navigateCaptchaPage(p, aliyunCaptchaTargetURL)
		cancel()
		if err != nil {
			cp.lastError.Store(err.Error())
			discardCaptchaPage(cp)
			return
		}
		nowMs := time.Now().UnixMilli()
		cp.lastOpenedAtMs.Store(nowMs)
		cp.lastUsedAtMs.Store(nowMs)
		if verifySuccess {
			cp.lastError.Store("")
		} else if lastErr != nil {
			errText := strings.TrimSpace(lastErr.Error())
			if errText == "" {
				errText = "验证码失败"
			}
			if len(errText) > 240 {
				errText = errText[:240]
			}
			cp.lastError.Store(errText)
		} else {
			cp.lastError.Store("验证码失败")
		}
		releaseCaptchaPage(cp)
	}()

	// --- 状态 ---
	var (
		mu           sync.Mutex
		backB64      string
		shadowB64    string
		hasTriggered bool

		pageSceneID   string
		finalResult   string
	)

	type apiSolveResult struct {
		X   float64
		Err error
	}

	apiSolveCh := make(chan apiSolveResult, 10)
	verifyResultCh := make(chan string, 10)

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

	drainApiSolveChan := func(ch chan apiSolveResult) {
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}

	resetState := func() {
		mu.Lock()
		backB64 = ""
		shadowB64 = ""
		hasTriggered = false
		mu.Unlock()

		drainApiSolveChan(apiSolveCh)
		drainStringChan(verifyResultCh)
	}

	parseSolveResponseCode := func(raw json.RawMessage) (int, error) {
		raw = bytes.TrimSpace(raw)
		if len(raw) == 0 {
			return 0, errors.New("missing code")
		}
		if len(raw) > 0 && raw[0] == '"' {
			var s string
			if err := json.Unmarshal(raw, &s); err != nil {
				return 0, err
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return 0, errors.New("empty code")
			}
			n, err := strconv.Atoi(s)
			if err != nil {
				return 0, err
			}
			return n, nil
		}
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			return 0, err
		}
		return n, nil
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
				Token:           strings.TrimSpace(JfbymToken),
				Type:            strings.TrimSpace(JfbymType),
			}
			if reqBody.Token == "" {
				select {
				case apiSolveCh <- apiSolveResult{Err: errors.New("打码服务 token 为空")}:
				default:
				}
				return
			}

			form := url.Values{}
			form.Set("slide_image", reqBody.SlideImage)
			form.Set("background_image", reqBody.BackgroundImage)
			form.Set("token", reqBody.Token)
			form.Set("type", reqBody.Type)

			req, err := http.NewRequestWithContext(ctx, http.MethodPost, JfbymApiUrl, strings.NewReader(form.Encode()))
			if err != nil {
				select {
				case apiSolveCh <- apiSolveResult{Err: err}:
				default:
				}
				return
			}
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			resp, err := captchaHTTPClient.Do(req)
			if err != nil {
				select {
				case apiSolveCh <- apiSolveResult{Err: err}:
				default:
				}
				return
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			var sr solveResponse
			if err := json.Unmarshal(respBody, &sr); err != nil {
				debugf("打码接口返回非 JSON（len=%d）", len(respBody))
				select {
				case apiSolveCh <- apiSolveResult{Err: fmt.Errorf("打码接口返回非 JSON: %w", err)}:
				default:
				}
				return
			}

			code, err := parseSolveResponseCode(sr.Code)
			if err != nil {
				select {
				case apiSolveCh <- apiSolveResult{Err: fmt.Errorf("解析打码接口 code 失败: %w", err)}:
				default:
				}
				return
			}
			// JFBYM 的成功 code 常见为 10000（也可能是 0），这里兼容两种。
			if code != 0 && code != 10000 {
				msg := strings.TrimSpace(sr.Msg)
				if msg == "" {
					msg = "打码接口返回失败"
				}
				debugf("打码失败 code=%d msg=%s", code, msg)
				select {
				case apiSolveCh <- apiSolveResult{Err: fmt.Errorf("%s (code=%d)", msg, code)}:
				default:
				}
				return
			}
			debugf("打码返回 success code=%d msg=%s", code, strings.TrimSpace(sr.Msg))

			var items []solveItem
			_ = json.Unmarshal(sr.Data, &items)
			if len(items) == 0 {
				var single solveItem
				if json.Unmarshal(sr.Data, &single) == nil {
					items = append(items, single)
				}
			}

			for _, d := range items {
				val, err := strconv.ParseFloat(d.Data, 64)
				if err != nil {
					continue
				}
				if val <= 0 {
					continue
				}
				select {
				case apiSolveCh <- apiSolveResult{X: val}:
				default:
				}
				return
			}

			// 有些返回 data 可能就是纯数字/字符串
			var rawStr string
			if json.Unmarshal(sr.Data, &rawStr) == nil {
				if v, err := strconv.ParseFloat(strings.TrimSpace(rawStr), 64); err == nil {
					select {
					case apiSolveCh <- apiSolveResult{X: v}:
					default:
					}
					return
				}
			}
			var rawNum float64
			if json.Unmarshal(sr.Data, &rawNum) == nil && rawNum > 0 {
				select {
				case apiSolveCh <- apiSolveResult{X: rawNum}:
				default:
				}
				return
			}

			debugf("打码接口返回无可用结果 code=%d dataLen=%d", code, len(sr.Data))
			select {
			case apiSolveCh <- apiSolveResult{Err: errors.New("打码接口返回无可用结果")}:
			default:
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
		debugf("捕获 back.png bytes=%d", len(body))
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
		debugf("捕获 shadow.png bytes=%d", len(body))
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
			debugf("捕获 verifyResult success securityTokenLen=%d", len(res.Result.SecurityToken))
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
			debugf("捕获 verifyResult failed")
			select {
			case verifyResultCh <- "":
			default:
			}
		}
	})

	go router.Run()

	// --- 打开页面：优先复用“已预打开”的页面，必要时再导航 ---
	ensureCaptchaPageOpened := func() error {
		lastOpenedAt := cp.lastOpenedAtMs.Load()
		if lastOpenedAt > 0 && time.Since(time.UnixMilli(lastOpenedAt)) > 2*time.Minute {
			// 页面打开太久：强制刷新唤醒，避免卡死/白屏。
		} else {
			if _, err := page.Timeout(500 * time.Millisecond).Element("#button"); err == nil {
				return nil
			}
		}
		if err := navigateCaptchaPage(page, makeTargetURL(1)); err != nil {
			cp.lastError.Store(err.Error())
			return err
		}
		nowMs := time.Now().UnixMilli()
		cp.lastOpenedAtMs.Store(nowMs)
		cp.lastUsedAtMs.Store(nowMs)
		cp.lastError.Store("")
		return nil
	}

	if err := ensureCaptchaPageOpened(); err != nil {
		lastErr = fmt.Errorf("打开页面失败: %v", err)
		metrics.Duration = time.Since(started)
		return "", metrics, lastErr
	}
	pageSceneID = extractSceneID(page)

	maxTries := captchaMaxSolveAttempts()

	// --- 验证循环 ---
	for tryCount := 1; !verifySuccess; tryCount++ {
		if maxTries > 0 && tryCount > maxTries {
			discardAfter = true
			break
		}
		metrics.Attempts = tryCount
		select {
		case <-ctx.Done():
			if lastErr != nil {
				metrics.Duration = time.Since(started)
				return "", metrics, lastErr
			}
			lastErr = errors.New("验证码流程超时")
			metrics.Duration = time.Since(started)
			return "", metrics, lastErr
		default:
		}

		// 验证失败后需要“换一张新图”再滑动：每次重试都重新加载页面，确保 back/shadow 会重新请求。
		resetState()
		if tryCount > 1 {
			if err := navigateCaptchaPage(page, makeTargetURL(tryCount)); err != nil {
				lastErr = err
				cp.lastError.Store(err.Error())
				continue
			}
			nowMs := time.Now().UnixMilli()
			cp.lastOpenedAtMs.Store(nowMs)
			cp.lastUsedAtMs.Store(nowMs)
			cp.lastError.Store("")
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
		case sr := <-apiSolveCh:
			if sr.Err != nil {
				lastErr = sr.Err
				continue
			}
			apiX = sr.X
		case <-time.After(25 * time.Second):
			lastErr = errors.New("等待打码结果超时")
			continue
		case <-ctx.Done():
			metrics.Duration = time.Since(started)
			return "", metrics, errors.New("等待打码结果超时")
		}

		offset := (rng.Float64() * 0.2) - 0.1
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

		// 先用“人类轨迹”拖到接近目标位置（避免每次轨迹完全一致被风控），再做自适应微调。
		targetX := startX + finalDistance

		// 轻微过冲：更像人类拖动。
		overshoot := 0.0
		if rng.Intn(10) < 4 {
			overshoot = 1 + rng.Float64()*3 // 1~4px
		}
		endX := targetX + overshoot
		endY := startY + (rng.Float64()*6 - 3)

		midRatio := 0.55 + rng.Float64()*0.25 // 0.55~0.80
		midX := startX + (endX-startX)*midRatio
		midY := startY + (rng.Float64()*10 - 5)

		steps1 := 8 + rng.Intn(10)
		steps2 := 12 + rng.Intn(16)
		executeTrack(rng, page, generateBezierTrack(rng, startX, startY, midX, midY, steps1))
		captchaSleep(20*time.Millisecond, 40*time.Millisecond)
		executeTrack(rng, page, generateBezierTrack(rng, midX, midY, endX, endY, steps2))

		currentMouseX := endX
		captchaSleep(60*time.Millisecond, 40*time.Millisecond)

		targetPuzzlePos := finalDistance
		tolerance := 0.8 + rng.Float64()*0.6
		maxAttempts := 24 + rng.Intn(18)
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

			randomY := startY + (rng.Float64()*6 - 3)
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

	if discardAfter && !verifySuccess {
		if maxTries <= 0 {
			maxTries = 3
		}
		if lastErr != nil {
			lastErr = fmt.Errorf("%w（连续失败%d次，已自动重建页面）", lastErr, maxTries)
		} else {
			lastErr = fmt.Errorf("验证码失败（连续失败%d次，已自动重建页面）", maxTries)
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
func generateBezierTrack(rng *rand.Rand, startX, startY, endX, endY float64, steps int) []Point {
	if steps < 2 {
		steps = 2
	}

	rr := rng
	if rr == nil {
		rr = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	var track []Point

	dx := endX - startX
	dy := endY - startY

	// 控制点随机化：同样的起终点，每次生成的轨迹都不同。
	cx1 := startX + dx*(0.15+rr.Float64()*0.25)
	cx2 := startX + dx*(0.55+rr.Float64()*0.35)
	jitterY := 2.0 + rr.Float64()*6.0
	cy1 := startY + dy*(0.10+rr.Float64()*0.40) + (rr.Float64()*2-1)*jitterY
	cy2 := startY + dy*(0.60+rr.Float64()*0.30) + (rr.Float64()*2-1)*jitterY

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

		// 轻微抖动：避免轨迹过于光滑/完全一致。
		if i > 0 && i < steps {
			x += (rr.Float64()*2 - 1) * 0.35
			y += (rr.Float64()*2 - 1) * 0.90
			// 保持 x 单调递增（拖动时更自然，也避免出现突然回拉）。
			if len(track) > 0 && x < track[len(track)-1].X {
				x = track[len(track)-1].X + rr.Float64()*0.25
			}
		}

		track = append(track, Point{x, y})
	}
	return track
}

// 执行轨迹移动。
func executeTrack(rng *rand.Rand, page *rod.Page, track []Point) {
	rr := rng
	if rr == nil {
		rr = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if page == nil || len(track) == 0 {
		return
	}
	lastIdx := len(track) - 1
	for i, p := range track {
		page.Mouse.MustMoveTo(p.X, p.Y)
		if lastIdx <= 0 {
			continue
		}

		progress := float64(i) / float64(lastIdx)
		base := 3 * time.Millisecond
		jitter := 4 * time.Millisecond
		switch {
		case progress < 0.25:
			base = 5 * time.Millisecond
			jitter = 5 * time.Millisecond
		case progress < 0.85:
			base = 2 * time.Millisecond
			jitter = 4 * time.Millisecond
		default:
			base = 6 * time.Millisecond
			jitter = 6 * time.Millisecond
		}

		// 偶尔短暂停顿一下，更像人类操作。
		if rr.Intn(100) < 3 {
			captchaSleep(time.Duration(25+rr.Intn(40))*time.Millisecond, 0)
			continue
		}
		captchaSleep(base, jitter)
	}
}
