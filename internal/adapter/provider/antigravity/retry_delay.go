package antigravity

import (
	"encoding/json"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RateLimitReason represents the type of rate limiting
type RateLimitReason int

const (
	RateLimitReasonUnknown                RateLimitReason = iota
	RateLimitReasonQuotaExhausted                         // QUOTA_EXHAUSTED - 配额耗尽
	RateLimitReasonRateLimitExceeded                      // RATE_LIMIT_EXCEEDED - 速率限制
	RateLimitReasonModelCapacityExhausted                 // MODEL_CAPACITY_EXHAUSTED - 模型容量耗尽
	RateLimitReasonServerError                            // 5xx errors
)

// Default retry delays by reason (like Antigravity-Manager)
const (
	DefaultQuotaExhaustedDelay = 60 * time.Second // 60 seconds (first failure, like Antigravity-Manager)
	DefaultRateLimitDelay      = 30 * time.Second // 30 seconds
	DefaultModelCapacityDelay  = 15 * time.Second // 15 seconds (temporary GPU unavailability)
	DefaultServerErrorDelay    = 20 * time.Second // 20 seconds
	DefaultUnknownDelay        = 60 * time.Second // 60 seconds
	MinRetryDelay              = 2 * time.Second  // Minimum 2 seconds (safety buffer)
	JitterFactor               = 0.0              // Jitter disabled for stability (aligned with Antigravity-Manager)
)

// RetryInfo contains parsed retry information from a 429 response
type RetryInfo struct {
	Delay  time.Duration
	Reason RateLimitReason
}

// ParseRetryInfo parses retry delay and reason from error response body
// Supports Google API error format with retryDelay and quotaResetDelay fields
func ParseRetryInfo(statusCode int, body []byte) *RetryInfo {
	if statusCode != 429 && statusCode != 500 && statusCode != 503 && statusCode != 529 {
		return nil
	}

	bodyStr := string(body)

	// Parse reason
	var reason RateLimitReason
	if statusCode == 429 {
		reason = parseRateLimitReason(bodyStr)
	} else {
		reason = RateLimitReasonServerError
	}

	// Try to parse delay from response body
	delay := parseRetryDelay(body)
	if delay == 0 {
		return nil
	}

	return &RetryInfo{
		Delay:  delay,
		Reason: reason,
	}
}

// ApplyJitter adds random jitter to delay to prevent thundering herd
// Returns delay ± JitterFactor (e.g., 1000ms ± 20% = 800-1200ms)
func ApplyJitter(delay time.Duration) time.Duration {
	if delay <= 0 || JitterFactor == 0 {
		return delay
	}

	jitterRange := float64(delay) * JitterFactor
	jitter := (rand.Float64()*2 - 1) * jitterRange // -jitterRange to +jitterRange
	result := time.Duration(float64(delay) + jitter)

	if result < time.Millisecond {
		result = time.Millisecond
	}

	return result
}

// parseRetryDelay extracts retry delay from error response body
// Supports formats:
// - error.details[].retryDelay (type == RetryInfo)
// - error.details[].metadata.quotaResetDelay (type == ErrorInfo)
func parseRetryDelay(body []byte) time.Duration {
	var errorResp struct {
		Error struct {
			Details []struct {
				Type       string `json:"@type"`
				RetryDelay string `json:"retryDelay"`
				Metadata   struct {
					QuotaResetDelay string `json:"quotaResetDelay"`
				} `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errorResp); err != nil {
		return 0
	}

	// Method 1: RetryInfo.retryDelay (like CLIProxyAPI)
	for _, detail := range errorResp.Error.Details {
		if strings.Contains(detail.Type, "RetryInfo") && detail.RetryDelay != "" {
			if d := parseDurationString(detail.RetryDelay); d > 0 {
				return d
			}
		}
	}

	// Method 2: ErrorInfo.metadata.quotaResetDelay
	for _, detail := range errorResp.Error.Details {
		if detail.Metadata.QuotaResetDelay != "" {
			if d := parseDurationString(detail.Metadata.QuotaResetDelay); d > 0 {
				return d
			}
		}
	}

	return 0
}

// parseDurationString parses duration strings in various formats
// Supports: "1.203608125s", "500ms", "2m30s", "1h30m", "42s"
func parseDurationString(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// Try standard Go duration parsing first
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	// Try parsing as plain number (seconds)
	if seconds, err := strconv.ParseFloat(s, 64); err == nil {
		return time.Duration(seconds * float64(time.Second))
	}

	// Try regex for complex formats like "2h21m25s"
	return parseDurationRegex(s)
}

// parseDurationRegex parses duration using regex for complex formats
func parseDurationRegex(s string) time.Duration {
	var total time.Duration

	// Hours
	if re := regexp.MustCompile(`(\d+)h`); re.MatchString(s) {
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			if hours, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
				total += time.Duration(hours) * time.Hour
			}
		}
	}

	// Minutes
	if re := regexp.MustCompile(`(\d+)m(?:[^s]|$)`); re.MatchString(s) {
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			if mins, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
				total += time.Duration(mins) * time.Minute
			}
		}
	}

	// Seconds (including decimal)
	if re := regexp.MustCompile(`(\d+(?:\.\d+)?)s`); re.MatchString(s) {
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			if secs, err := strconv.ParseFloat(matches[1], 64); err == nil {
				total += time.Duration(secs * float64(time.Second))
			}
		}
	}

	// Milliseconds
	if re := regexp.MustCompile(`(\d+)ms`); re.MatchString(s) {
		if matches := re.FindStringSubmatch(s); len(matches) > 1 {
			if ms, err := strconv.ParseInt(matches[1], 10, 64); err == nil {
				total += time.Duration(ms) * time.Millisecond
			}
		}
	}

	return total
}

// parseRateLimitReason determines the rate limit reason from response body
// (like Antigravity-Manager's parse_rate_limit_reason)
func parseRateLimitReason(body string) RateLimitReason {
	// Try to parse from JSON
	var errorResp struct {
		Error struct {
			Message string `json:"message"`
			Details []struct {
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}

	if err := json.Unmarshal([]byte(body), &errorResp); err == nil {
		// Check reason field first
		if len(errorResp.Error.Details) > 0 {
			switch errorResp.Error.Details[0].Reason {
			case "QUOTA_EXHAUSTED":
				return RateLimitReasonQuotaExhausted
			case "RATE_LIMIT_EXCEEDED":
				return RateLimitReasonRateLimitExceeded
			case "MODEL_CAPACITY_EXHAUSTED":
				return RateLimitReasonModelCapacityExhausted
			}
		}

		// [Antigravity-Manager] Try to detect TPM from message field
		if errorResp.Error.Message != "" {
			msgLower := strings.ToLower(errorResp.Error.Message)
			if strings.Contains(msgLower, "per minute") || strings.Contains(msgLower, "rate limit") {
				return RateLimitReasonRateLimitExceeded
			}
		}
	}

	// Fallback to text matching (like Antigravity-Manager)
	// [FIX] Prioritize "per minute" detection to avoid misclassifying TPM as quota exhaustion
	bodyLower := strings.ToLower(body)
	if strings.Contains(bodyLower, "per minute") || strings.Contains(bodyLower, "rate limit") || strings.Contains(bodyLower, "too many requests") {
		return RateLimitReasonRateLimitExceeded
	}
	if strings.Contains(bodyLower, "exhausted") || strings.Contains(bodyLower, "quota") {
		return RateLimitReasonQuotaExhausted
	}

	return RateLimitReasonUnknown
}

// getDefaultDelay returns default delay based on reason
func getDefaultDelay(reason RateLimitReason) time.Duration {
	switch reason {
	case RateLimitReasonQuotaExhausted:
		return DefaultQuotaExhaustedDelay
	case RateLimitReasonRateLimitExceeded:
		return DefaultRateLimitDelay
	case RateLimitReasonModelCapacityExhausted:
		return DefaultModelCapacityDelay
	case RateLimitReasonServerError:
		return DefaultServerErrorDelay
	default:
		return DefaultUnknownDelay
	}
}

// IsQuotaExhausted checks if the error indicates quota exhaustion
// Only returns true for explicit QUOTA_EXHAUSTED (like Antigravity-Manager)
func IsQuotaExhausted(body []byte) bool {
	bodyStr := string(body)
	return strings.Contains(bodyStr, "QUOTA_EXHAUSTED")
}

// FormatRetryAfterHeader formats the delay for Retry-After header
func FormatRetryAfterHeader(delay time.Duration) string {
	return strconv.FormatInt(int64(delay.Seconds()), 10)
}

// ReasonString returns a human-readable string for the reason
func (r RateLimitReason) String() string {
	switch r {
	case RateLimitReasonQuotaExhausted:
		return "QUOTA_EXHAUSTED"
	case RateLimitReasonRateLimitExceeded:
		return "RATE_LIMIT_EXCEEDED"
	case RateLimitReasonModelCapacityExhausted:
		return "MODEL_CAPACITY_EXHAUSTED"
	case RateLimitReasonServerError:
		return "SERVER_ERROR"
	default:
		return "UNKNOWN"
	}
}
