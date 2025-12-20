package main

import (
	"fmt"
	"time"

	"sniping_engine/internal/utils"
)

func main() {
	// 获取当前时间戳
	timestamp := time.Now().UnixMilli()
	fmt.Printf("测试时间戳: %d\n", timestamp)

	// 这里需要替换为实际的draco_local cookie值
	// 你可以从浏览器的开发者工具中获取这个值
	dracoToken := "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJsb2dpbiIsInBhdGgiOiIvIiwidG9rZW5LZXkiOiIzZGU4NTNiM2EwNmNhZWM4MTM3NGJjOTgyMWZmMTdkMTVkZTlkMzhhYjFlMDVkYWYyNWNkZTU2NjYxMWM3YjgzIiwibmJmIjoxNzY2MjE4MTA5LCJkb21haW4iOiI0MDA4MTE3MTE3LmNvbSIsImlzcyI6ImRyYWNvIiwidGVuYW50SWQiOjEsImV4cGlyZV90aW1lIjoyNTkyMDAwLCJleHAiOjE3Njg4MTAxMDksImlhdCI6MTc2NjIxODEwOX0.Fs_ApcRVaiZpAxj5c0aTqr_tFCTDItmGPhjQKsRdX80"

	if dracoToken == "your_actual_draco_local_cookie_value" {
		fmt.Println("错误: 请先替换为实际的draco_local cookie值")
		fmt.Println("你可以从浏览器的开发者工具中获取这个值")
		return
	}

	fmt.Printf("使用dracoToken: %s\n", dracoToken)
	fmt.Println("开始调用SolveAliyunCaptcha方法...")

	// 调用验证码解决方法
	captchaResult, err := utils.SolveAliyunCaptcha(timestamp, dracoToken)
	if err != nil {
		fmt.Printf("调用失败: %v\n", err)
		return
	}

	if captchaResult == "" {
		fmt.Println("调用成功，但返回结果为空")
		return
	}

	fmt.Printf("调用成功，返回结果: %s\n", captchaResult)
	fmt.Printf("结果长度: %d\n", len(captchaResult))
}
