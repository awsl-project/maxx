package privacy

import (
	"regexp"
	"strings"
)

// Natural-language credential contexts follow Magpie's CJK rules. These stay
// separate from config assignments so ordinary words and quantities remain safe.
const namedSecretChars = `[A-Za-z0-9_\-./+=~!@#%^&*]`

func namedRule(names string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:` + names + `)[ \t]*(?:[:：=][ \t]*|[为為是][ \t]*[:：]?[ \t]*|は[ \t]*|는[ \t]*)?(?:["'“「])?(` + namedSecretChars + `{3,}[A-Za-z0-9_\-/+=~!@#%^&*])`)
}

var namedRules = []rule{
	{"named_password", namedRule(`密码|密碼|口令|PIN[ ]?码|PIN碼|安全码|安全碼|パスワード|暗証番号|비밀번호|암호|비번`), 1, namedPasswordValue},
	{"named_key", namedRule(`密钥|密鑰|秘钥|秘鑰|私钥|私鑰|金钥|金鑰|令牌|权杖|權杖|授权码|授權碼|秘密鍵|トークン|シークレット|APIキー|アクセスキー|토큰|비밀키|API[ ]?키`), 1, nil},
	{"named_key", regexp.MustCompile(`(?i)(?:` + credentialNames + `|token|secret|pwd|api[ ]key|access[ ]key|secret[ ]key|key|\bpin(?:[ ]?code)?)[ \t]*(?:[为為是][ \t]*[:：]?|：)[ \t]*["'“「]?(` + namedSecretChars + `{3,}[A-Za-z0-9_\-/+=~!@#%^&*])`), 1, nil},
}

func namedKeyValue(text string, start, end int) bool {
	value := text[start:end]
	// Paths name where the key is stored, rather than disclosing its value.
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "~/") {
		return false
	}
	if strings.Trim(value, "0123456789") != "" {
		letters := strings.ContainsFunc(value, func(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' })
		return letters && (len(value) >= 16 || strings.ContainsAny(value, "0123456789"))
	}
	if len(value) < 6 {
		return false
	}
	before := strings.TrimRight(text[:start], " \t\"'“「")
	separated := false
	for _, separator := range []string{"为", "為", "是", "：", ":", "=", "は", "는"} {
		separated = separated || strings.HasSuffix(before, separator)
	}
	after := strings.ToLower(strings.TrimLeft(text[end:], " \t"))
	for _, unit := range []string{"位", "比特", "字节", "字節", "个", "個", "次", "秒", "毫秒", "分钟", "分鐘", "小时", "小時", "天", "年", "bit", "byte", "milliseconds", "ms", "sec", "min", "ビット", "バイト", "비트", "바이트"} {
		if strings.HasPrefix(after, unit) {
			return false
		}
	}
	return separated
}

func namedPasswordValue(value string) bool {
	return !strings.HasPrefix(value, "*") && strings.Trim(value, "xX.") != ""
}
