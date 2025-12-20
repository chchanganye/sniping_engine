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

	// ⬇️ 无头模式开关：false 表示显示浏览器窗口（方便调试），true 表示无头模式（生产环境使用）
	HeadlessMode = false
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
func SolveAliyunCaptcha(timestamp int64, dracoToken string) (string, error) {
	rand.Seed(time.Now().UnixNano())

	// 构造目标 URL
	targetUrl := fmt.Sprintf(
		"https://m.4008117117.com/aliyun-captcha?t=%d&cookie=true&draco_local=%s",
		timestamp, dracoToken,
	)

	// 1. 启动浏览器
	u := launcher.New().Headless(HeadlessMode).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()

	defer func() {
		_ = browser.Close()
		launcher.New().Kill()
	}()

	page := stealth.MustPage(browser)
	page.MustEmulate(devices.IPhoneX)

	// 设置总超时时间 (60秒)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

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
		finalResult   string
		errResult     error
	)

	sliderElCh := make(chan *rod.Element, 100)
	apiXCh := make(chan float64, 10)
	verifyResultCh := make(chan string, 10)

	// 【新增】控制点击停止的信号
	stopClicking := make(chan struct{})
	var stopClickingOnce sync.Once

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

		fmt.Printf("⚡️ [第%d次] 图片集齐，请求打码...\n", retryCount+1)
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
				fmt.Println("❌ 打码请求失败:", err)
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
					fmt.Printf("✅ 打码成功，坐标: %.2f\n", val)
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
			fmt.Println("🖼️ 拦截到背景图")
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
			fmt.Println("🖼️ 拦截到滑块图")
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
					verifyResultCh <- jsonBase64
				} else if !*res.Result.VerifyResult {
					verifyResultCh <- ""
				}
			}
		}
	})
	go router.Run()

	// --- 3. 页面交互 ---
	fmt.Println("🚀 打开页面...")
	if err := page.Navigate(targetUrl); err != nil {
		return "", fmt.Errorf("打开页面失败: %v", err)
	}

	// 提取 SceneId
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

	// ------------------------------------------------------------------
	// 【关键修正】点击按钮协程
	// 逻辑：一直尝试点击，直到收到 stopClicking 信号（即滑块可见）才停止
	// ------------------------------------------------------------------
	go func() {
		selectors := []string{"#button", "#aliyunCaptcha-btn", "button[type='button']", ".btn"}
		ticker := time.NewTicker(300 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-stopClicking: // 收到停止信号，退出
				fmt.Println("🛑 滑块已可见，停止点击按钮")
				return
			case <-ticker.C:
				clicked := false
				for _, sel := range selectors {
					if el, err := page.Element(sel); err == nil {
						if v, _ := el.Visible(); v {
							_ = el.ScrollIntoView()
							_ = el.Click(proto.InputMouseButtonLeft, 1)
							fmt.Printf("👉 点击验证按钮: %s\n", sel)
							clicked = true
							break
						}
					}
				}
				// 兜底 JS 点击
				if !clicked {
					_, _ = page.Eval(`() => {
						let btn = document.getElementById('button');
						if(btn) btn.click();
					}`)
				}
			}
		}
	}()

	// ------------------------------------------------------------------
	// 【关键修正】找滑块协程
	// 逻辑：一旦滑块可见，立即发送 stopClicking 信号，并将滑块对象发给主流程
	// ------------------------------------------------------------------
	go func() {
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if el, err := page.Element("#aliyunCaptcha-sliding-slider"); err == nil {
					// 必须确保 Visible 为 true
					if v, _ := el.Visible(); v {
						// 1. 发出停止点击信号 (只发一次，避免 panic)
						stopClickingOnce.Do(func() {
							close(stopClicking)
						})

						// 2. 发送滑块对象
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
		select {
		case <-ctx.Done():
			return "", errors.New("验证流程超时")
		default:
		}

		retryCount++
		fmt.Printf("⏳ [第%d次] 等待...\n", retryCount)

		var sliderEl *rod.Element
		var apiX float64

		gotSlider := false
		gotApiX := false

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
		fmt.Printf("🧮 目标距离: %.2f\n", finalDistance)

		// 获取起点
		box := sliderEl.MustShape().Box()
		startX := box.X + box.Width/2
		startY := box.Y + box.Height/2

		// 1. 按下滑块
		page.Mouse.MustMoveTo(startX, startY)
		time.Sleep(time.Duration(100+rand.Intn(50)) * time.Millisecond)
		page.Mouse.MustDown(proto.InputMouseButtonLeft)
		time.Sleep(time.Duration(50+rand.Intn(50)) * time.Millisecond)

		// -----------------------------------------------------------
		// 2. 自适应滑动策略 (高精度版 - 容差 0.8)
		// -----------------------------------------------------------
		fmt.Println("🔄 开始自适应滑动策略...")

		// 定义获取滑块当前位置的函数
		getPuzzlePos := func() float64 {
			res, _ := page.Eval(`() => {
				let el = document.querySelector('#aliyunCaptcha-puzzle');
				if (!el) return -1;
				// 兼容 left 和 transform 两种位移方式
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

		// 初始滑动 (先滑到理论位置)
		currentMouseX := startX + finalDistance

		// 简单移动到初步位置
		page.Mouse.MustMoveTo(currentMouseX, startY)
		time.Sleep(time.Duration(200) * time.Millisecond)

		// 定义目标位置和参数
		targetPuzzlePos := finalDistance

		// 【修改点1】将容差收紧到 0.8，确保误差在 1px 以内
		tolerance := 0.8
		maxAttempts := 30 // 增加尝试次数，因为高精度需要更多微调
		success := false

		attempt := 0
		for ; attempt < maxAttempts; attempt++ {
			currentPos := getPuzzlePos()
			diff := targetPuzzlePos - currentPos

			fmt.Printf("🔍 第%d次调整: 滑块位置=%.2f, 目标=%.2f, 差异=%.2f\n", attempt+1, currentPos, targetPuzzlePos, diff)

			// 检查是否在容差范围内
			if math.Abs(diff) <= tolerance {
				fmt.Println("✅ 已达到目标位置，停止调整")
				success = true
				break
			}

			// --- 核心修正逻辑 ---

			dampingFactor := 0.5

			// 【修改点2】动态阻尼策略优化
			// 距离大时保守(0.5)，距离近时稍微激进一点(0.9)，确保能推进最后 1px
			absDiff := math.Abs(diff)
			if absDiff < 3 {
				// 距离非常近，几乎按 1:1 移动，否则容易因为移动太小被忽略
				dampingFactor = 0.9
			} else if absDiff < 10 {
				dampingFactor = 0.7
			} else {
				dampingFactor = 0.5
			}

			moveStep := diff * dampingFactor

			// 限制单次最大修正幅度
			if moveStep > 30 {
				moveStep = 30
			} else if moveStep < -30 {
				moveStep = -30
			}

			currentMouseX += moveStep

			// 添加微小的随机 Y 轴抖动
			randomY := startY + (rand.Float64()*2 - 1)

			fmt.Printf("🎯 修正鼠标: 步长=%.2f, 新鼠标X=%.2f\n", moveStep, currentMouseX)

			// 执行移动
			page.Mouse.MustMoveTo(currentMouseX, randomY)

			// 必须有足够的停顿让页面 JS 响应动画
			time.Sleep(time.Duration(150+rand.Intn(100)) * time.Millisecond)
		}

		// 最终位置检查
		finalPos := getPuzzlePos()
		fmt.Printf("🏁 最终滑块位置: %.2f, 目标: %.2f, 最终差异: %.2f\n", finalPos, targetPuzzlePos, finalPos-targetPuzzlePos)

		if success {
			fmt.Println("🎉 调整成功！")
		} else {
			fmt.Printf("⚠️ 调整超时，已尝试%d次\n", attempt)
		}
		// 4. 停顿后松开滑块
		time.Sleep(time.Duration(300+rand.Intn(200)) * time.Millisecond)
		page.Mouse.MustUp(proto.InputMouseButtonLeft)

		// 等待结果
		select {
		case resStr := <-verifyResultCh:
			if resStr != "" {
				verifySuccess = true
				finalResult = resStr
				// 打印最终的JSON结构，方便调试
				fmt.Println("📋 最终验证码结果JSON:")
				fmt.Println(finalResult)
			} else {
				fmt.Println("❌ 验证失败，重置状态...")
				resetState()
				time.Sleep(1 * time.Second)
			}
		case <-time.After(5 * time.Second):
			fmt.Println("⚠️ 结果等待超时，重置...")
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
