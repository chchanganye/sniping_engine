package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"sniping_engine/internal/utils"
)

func main() {
	var (
		dracoToken = flag.String("draco", "", "draco_local 的值（也可用环境变量 DRACO_LOCAL / DRACO_TOKEN）")
		timestamp  = flag.Int64("timestamp", 0, "时间戳（毫秒），默认使用当前时间")
	)
	flag.Parse()

	ts := *timestamp
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}

	token := strings.TrimSpace(*dracoToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("DRACO_LOCAL"))
	}
	if token == "" {
		token = strings.TrimSpace(os.Getenv("DRACO_TOKEN"))
	}
	if token == "" {
		fmt.Println("错误：缺少 draco_local，请通过参数 -draco 传入，或设置环境变量 DRACO_LOCAL/DRACO_TOKEN")
		return
	}

	fmt.Printf("测试时间戳：%d\n", ts)
	fmt.Println("开始调用 SolveAliyunCaptcha...")

	captchaResult, err := utils.SolveAliyunCaptcha(ts, token)
	if err != nil {
		fmt.Printf("调用失败：%v\n", err)
		return
	}
	if strings.TrimSpace(captchaResult) == "" {
		fmt.Println("调用成功，但返回结果为空")
		return
	}

	fmt.Printf("调用成功，结果长度：%d\n", len(captchaResult))
}
