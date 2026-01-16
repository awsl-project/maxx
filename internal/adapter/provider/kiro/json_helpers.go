package kiro

import "encoding/json"

// FastMarshal 高性能 JSON 序列化 (匹配 kiro2api utils/json.go)
// 注：kiro2api 使用 bytedance/sonic，这里使用标准库
func FastMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// FastUnmarshal 高性能 JSON 反序列化
func FastUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// SafeMarshal 安全 JSON 序列化（带验证）
func SafeMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// SafeUnmarshal 安全 JSON 反序列化（带验证）
func SafeUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// MarshalIndent 带缩进的 JSON 序列化
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// jsonUnmarshal 内部使用的反序列化函数
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
