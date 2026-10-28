package privacy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLatestMagpieCredentialContexts(t *testing.T) {
	for _, text := range []string{
		"redis://:RedisExample123@localhost/0",
		"oc_sk_" + strings.Repeat("Test1234", 4),
		"or_sk_" + strings.Repeat("Test1234", 4),
		"密码：786kih", "密碼是 hunter2", "パスワード: example123", "비밀번호는 example123",
		"密钥为12231241", "token 是 20261025", "金鑰：Example123456", "API Key：Example123456",
		"密码是 " + strings.Repeat("Example1234", 12),
	} {
		t.Run(text[:min(len(text), 30)], func(t *testing.T) {
			if Detect(text) == "" {
				t.Fatal("credential context missed")
			}
			body, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{"role": "user", "content": text}}})
			if kind, err := InspectRequest(body, "application/json"); err != nil || kind == "" {
				t.Fatalf("request not protected: %s %v", kind, err)
			}
			if cleaned := ScrubBody(string(body)); strings.Contains(cleaned, text) || !strings.Contains(cleaned, Redacted) {
				t.Fatalf("archive not scrubbed: %s", cleaned)
			}
		})
	}
	for _, text := range []string{
		"https://news.example/sk-hynix-third-quarter-profit.html",
		"密钥为 2048 位", "密钥是 300000 秒", "令牌 300000 毫秒", "令牌是 300000 milliseconds",
		"token 是 expired", "key 是 value", "私钥 ~/.ssh/id_ed25519", "密码：****",
	} {
		if Detect(text) != "" || ScrubText(text) != text {
			t.Errorf("ordinary text treated as a credential: %s", text)
		}
	}
	for _, key := range []string{
		"sk-0123456789abcdef0123456789abcdef",
		"sk-1234abcd-abcd-abcd-abcd-123456abcdef",
		testKey,
	} {
		if Detect(key) == "" {
			t.Errorf("realistic key format mistaken for an article slug: %s", key)
		}
	}
}

func FuzzScrubTextIdempotent(f *testing.F) {
	for _, text := range []string{testKey, "密码：786kih", "密钥是 300000 秒", "redis://:Example123@localhost", "password=\"abc;Example456\"", "token 是 20261025", "plain text"} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if len(text) > 32*1024 {
			t.Skip()
		}
		cleaned := ScrubText(text)
		if again := ScrubText(cleaned); again != cleaned {
			t.Fatal("scrubbing an archived copy again changed its content")
		}
	})
}
