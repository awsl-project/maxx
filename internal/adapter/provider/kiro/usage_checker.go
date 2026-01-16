package kiro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	// GetUsageLimitsURL 获取使用限制的 API URL
	GetUsageLimitsURL = "https://codewhisperer.us-east-1.amazonaws.com/getUsageLimits"
)

// CheckUsageLimits 检查 token 的使用限制
// 匹配 kiro2api/auth/usage_checker.go:27-86
func (a *KiroAdapter) CheckUsageLimits(ctx context.Context) (*UsageLimits, error) {
	// 获取 access token
	accessToken, err := a.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// 构建请求 URL
	params := url.Values{}
	params.Add("isEmailRequired", "true")
	params.Add("origin", "AI_EDITOR")
	params.Add("resourceType", "AGENTIC_REQUEST")

	requestURL := fmt.Sprintf("%s?%s", GetUsageLimitsURL, params.Encode())

	// 创建 HTTP 请求
	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create usage limits request: %w", err)
	}

	// 设置请求头 (匹配 kiro2api/auth/usage_checker.go:44-50)
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.0 KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")
	req.Header.Set("user-agent", "aws-sdk-js/1.0.0 ua/2.1 os/darwin#24.6.0 lang/js md/nodejs#20.16.0 api/codewhispererruntime#1.0.0 m/E KiroIDE-0.2.13-66c23a8c5d15afabec89ef9954ef52a119f10d369df04d548fc6c1eac694b0d1")
	req.Header.Set("host", "codewhisperer.us-east-1.amazonaws.com")
	req.Header.Set("amz-sdk-invocation-id", generateUsageInvocationID())
	req.Header.Set("amz-sdk-request", "attempt=1; max=1")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Connection", "close")

	// 发送请求
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usage limits request failed: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read usage limits response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage limits check failed: status %d, response: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var usageLimits UsageLimits
	if err := json.Unmarshal(body, &usageLimits); err != nil {
		return nil, fmt.Errorf("failed to parse usage limits response: %w", err)
	}

	return &usageLimits, nil
}

// GetUsageInfo 获取简化的额度信息
func (a *KiroAdapter) GetUsageInfo(ctx context.Context) (*UsageInfo, error) {
	limits, err := a.CheckUsageLimits(ctx)
	if err != nil {
		return nil, err
	}

	return CalculateUsageInfo(limits), nil
}

// generateUsageInvocationID 生成请求 ID
func generateUsageInvocationID() string {
	return fmt.Sprintf("%d-maxx", time.Now().UnixNano())
}
