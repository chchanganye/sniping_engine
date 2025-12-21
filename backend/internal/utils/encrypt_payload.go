package utils

import (
	"encoding/base64"
	"strings"
)

// EncryptPayload 按目标站点的规则加密密码等字段。
// 算法：
// 1) 逐字符取 key（循环）
// 2) (char ^ keyChar) + index
// 3) 再整体偏移 +17
// 4) Base64 编码后，前后各拼接 "=="
func EncryptPayload(input string) string {
	keyStr := "sxdSybCzy20251119ModifyVeryGood"
	key := []rune(keyStr)
	inputRunes := []rune(input)

	var sb strings.Builder
	sb.Grow(len(inputRunes))

	for n, char := range inputRunes {
		keyChar := key[n%len(key)]
		step1 := (int(char) ^ int(keyChar)) + n
		finalVal := step1 + 17
		sb.WriteRune(rune(finalVal))
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(sb.String()))
	return "==" + encoded + "=="
}

