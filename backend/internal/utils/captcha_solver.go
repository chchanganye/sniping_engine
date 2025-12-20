package main

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
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

// --- 配置区域 ---
const (
	JfbymToken  = "DAxk0GILbeSmlvuC_bf-ak99PB7rMPEflWi6JKJvwmE"
	JfbymApiUrl = "http://api.jfbym.com/api/YmServer/customApi"
	JfbymType   = "20111"

	// ⬇️ 滑动偏移量
	SlideOffset = 0.0
)

// API 结构体
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

// Point 坐标点
type Point struct {
	X, Y float64
}

// SolveAliyunCaptcha 执行验证码验证并返回 Base64 编码的结果
// timestamp: 请求时间戳 (例如 1766113292639)
// dracoToken: 用户凭证 (draco_local)
func SolveAliyunCaptcha(timestamp int64, dracoToken string) (string, error) {
	rand.Seed(time.Now().UnixNano())

	// 构造目标 URL
	targetUrl := fmt.Sprintf(
		"https://m.4008117117.com/aliyun-captcha?t=%d&cookie=true&draco_local=%s",
		timestamp, dracoToken,
	)

	// 1. 启动浏览器 (默认开启 Headless 模式以供后台调用)
	// 如果需要调试，可以将 Headless(true) 改为 Headless(false)
	u := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()

	// 确保函数结束时关闭浏览器资源
	defer func() {
		_ = browser.Close()
		_ = launcher.New().Kill() // 确保清理僵尸进程
	}()

	page := stealth.MustPage(browser)
	page.MustEmulate(devices.IPhoneX)

	// 设置总超时时间 (例如 60秒)，防止无限挂起
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	page = page.Context(ctx)

	router := page.HijackRequests()
	defer router.MustStop()

	// 2. 状态管理
	var (
		mu            sync.Mutex
		backB64       string
		shadowB64     string
		hasTriggered  bool
		retryCount    int
		verifySuccess bool
		pageSceneId   string
		finalResult   string // 最终返回的 Base64 字符串
		errResult     error  // 错误信息
	)

	sliderElCh := make(chan *rod.Element, 10)
	apiXCh := make(chan float64, 10)
	// 通道传递结果字符串，空字符串表示失败
	verifyResultCh := make(chan string, 10)

	resetState := func() {
		mu.Lock()
		backB64 = ""
		shadowB64 = ""
		hasTriggered = false
		mu.Unlock()
	}

	// --- 异步打码 ---
	checkAndSolve := func() {
		mu.Lock()
		defer mu.Unlock()
		if hasTriggered || backB64 == "" || shadowB64 == "" {
			return
		}
		hasTriggered = true

		// fmt.Printf("⚡️ [第%d次] 请求打码...\n", retryCount+1)
		go func() {
			reqBody := solveRequest{
				SlideImage:      shadowB64,
				BackgroundImage: backB64,
				Token:           JfbymToken,
				Type:            JfbymType,
			}
			bs, _ := json.Marshal(reqBody)

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Post(JfbymApiUrl, "application/json", bytes.NewReader(bs))
			if err != nil {
				// fmt.Println("❌ 打码请求失败:", err)
				return
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			var sr solveResponse
			if err := json.Unmarshal(respBody, &sr); err != nil {
				return
			}

			var items []solveItem
			json.Unmarshal(sr.Data, &items)
			if len(items) == 0 {
				var single solveItem
				json.Unmarshal(sr.Data, &single)
				items = append(items, single)
			}

			for _, d := range items {
				if d.Code == 0 {
					val, _ := strconv.ParseFloat(d.Data, 64)
					apiXCh <- val
					return
				}
			}
		}()
	}

	// --- 拦截器 ---
	router.MustAdd("*back.png*", func(ctx *rod.Hijack) {
		ctx.LoadResponse(http.DefaultClient, true)
		body := ctx.Response.Payload().Body
		if len(body) > 0 {
			b64 := base64.StdEncoding.EncodeToString(body)
			mu.Lock()
			backB64 = b64
			mu.Unlock()
			checkAndSolve()
		}
	})
	router.MustAdd("*shadow.png*", func(ctx *rod.Hijack) {
		ctx.LoadResponse(http.DefaultClient, true)
		body := ctx.Response.Payload().Body
		if len(body) > 0 {
			b64 := base64.StdEncoding.EncodeToString(body)
			mu.Lock()
			shadowB64 = b64
			mu.Unlock()
			checkAndSolve()
		}
	})
	router.MustAdd("*7atwlq.captcha-open.aliyuncs.com*", func(ctx *rod.Hijack) {
		ctx.LoadResponse(http.DefaultClient, true)
		body := ctx.Response.Payload().Body
		var res AliResult
		if json.Unmarshal(body, &res) == nil {
			if res.Result.VerifyResult != nil {
				if *res.Result.VerifyResult && res.Result.SecurityToken != "" {
					sceneId := pageSceneId
					if sceneId == "" {
						sceneId = res.Result.SceneId
					}
					output := OutputResult{
						CertifyId:     res.Result.CertifyId,
						SceneId:       sceneId,
						IsSign:        res.Result.IsSign,
						SecurityToken: res.Result.SecurityToken,
					}
					orderedJson, _ := json.Marshal(output)
					jsonBase64 := base64.StdEncoding.EncodeToString(orderedJson)
					// 发送成功结果
					verifyResultCh <- jsonBase64
				} else if !*res.Result.VerifyResult {
					// 发送失败信号
					verifyResultCh <- ""
				}
			}
		}
	})
	go router.Run()

	// --- 3. 页面交互 ---
	if err := page.Navigate(targetUrl); err != nil {
		return "", fmt.Errorf("打开页面失败: %v", err)
	}

	// 提取 SceneId (带超时保护)
	go func() {
		_ = page.WaitLoad()
		if result, err := page.Eval(`() => {
			let scripts = document.getElementsByTagName('script');
			for (let s of scripts) {
				let match = s.textContent.match(/SceneId:\s*["']([^"']+)["']/);
				if (match) return match[1];
			}
			return '';
		}`); err == nil {
			pageSceneId = result.Value.Str()
		}
	}()

	// 循环点击按钮
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if el, err := page.Element("#button"); err == nil {
					if v, _ := el.Visible(); v {
						_ = el.Click(proto.InputMouseButtonLeft, 1)
						return
					}
				}
			}
		}
	}()

	// 循环找滑块
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if el, err := page.Element("#aliyunCaptcha-sliding-slider"); err == nil {
					if v, _ := el.Visible(); v {
						select {
						case sliderElCh <- el:
						default:
						}
					}
				}
			}
		}
	}()

	// --- 4. 验证循环 ---
	for !verifySuccess {
		// 检查总超时
		select {
		case <-ctx.Done():
			return "", errors.New("验证流程超时")
		default:
		}

		retryCount++
		// fmt.Printf("⏳ [第%d次] 等待...\n", retryCount)

		var sliderEl *rod.Element
		var apiX float64

		gotSlider := false
		gotApiX := false

		// 等待资源
	loopWait:
		for !gotSlider || !gotApiX {
			select {
			case sliderEl = <-sliderElCh:
				if !gotSlider {
					gotSlider = true
				}
			case apiX = <-apiXCh:
				gotApiX = true
			case <-ctx.Done():
				return "", errors.New("等待资源超时")
			}

			if gotSlider && gotApiX {
				break loopWait
			}
		}

		// 计算目标距离
		offset := (rand.Float64()*0.2 - 0.1)
		finalDistance := apiX + SlideOffset + offset

		// 获取起点
		box := sliderEl.MustShape().Box()
		startX := box.X + box.Width/2
		startY := box.Y + box.Height/2

		// 1. 按下
		page.Mouse.MustMoveTo(startX, startY)
		time.Sleep(time.Duration(100+rand.Intn(50)) * time.Millisecond)
		page.Mouse.MustDown(proto.InputMouseButtonLeft)
		time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)

		// 2. 生成贝塞尔轨迹 (带过冲效果)
		overshoot := finalDistance + 3.0 + rand.Float64()*2.0

		track1 := generateBezierTrack(startX, startY, startX+overshoot, startY, 20)
		executeTrack(page, track1)

		time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)

		track2 := generateBezierTrack(startX+overshoot, startY, startX+finalDistance, startY, 10)
		executeTrack(page, track2)

		// 3. 闭环修正
		correctionTimeout := time.After(5 * time.Second)
	correctionLoop:
		for {
			select {
			case <-correctionTimeout:
				break correctionLoop
			default:
				// 使用 Eval 避免元素过期
				res, err := page.Eval(`() => {
					let el = document.querySelector('#aliyunCaptcha-puzzle');
					if (!el) return -1;
					return parseFloat(el.style.left) || 0;
				}`)

				if err != nil {
					time.Sleep(50 * time.Millisecond)
					continue
				}

				currentPuzzleLeft := res.Value.Num()
				if currentPuzzleLeft <= 0 && finalDistance > 10 {
					time.Sleep(20 * time.Millisecond)
					continue
				}

				diff := finalDistance - currentPuzzleLeft
				if math.Abs(diff) < 2.0 {
					break correctionLoop
				}

				currentMouseX := startX + currentPuzzleLeft + diff // 修正鼠标位置
				page.Mouse.MustMoveTo(currentMouseX, startY+(rand.Float64()-0.5))
				time.Sleep(50 * time.Millisecond)
			}
		}

		// 4. 强制锚定 + 停顿
		page.Mouse.MustMoveTo(startX+finalDistance, startY)
		time.Sleep(time.Duration(300+rand.Intn(200)) * time.Millisecond)

		// 5. 松开
		page.Mouse.MustUp(proto.InputMouseButtonLeft)

		// 等待结果
		select {
		case resStr := <-verifyResultCh:
			if resStr != "" {
				verifySuccess = true
				finalResult = resStr
			} else {
				resetState()
				time.Sleep(1 * time.Second)
			}
		case <-time.After(5 * time.Second):
			resetState()
			time.Sleep(1 * time.Second)
		case <-ctx.Done():
			return "", errors.New("验证等待超时")
		}
	}

	if verifySuccess {
		return finalResult, nil
	}

	return "", errResult
}

// 生成贝塞尔曲线轨迹
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

// 执行轨迹移动
func executeTrack(page *rod.Page, track []Point) {
	for _, p := range track {
		page.Mouse.MustMoveTo(p.X, p.Y)
		if rand.Intn(10) > 7 {
			time.Sleep(time.Duration(1+rand.Intn(2)) * time.Millisecond)
		}
	}
}
