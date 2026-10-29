package redact

import (
	"strings"
	"testing"
)

// Addresses of articles about SK hynix were masked as sk- keys
// (innerpeacebuild on X, the reporter's search results). A key beside
// them still is, and so are keys in lower case: DeepSeek's and
// OpenRouter's.
func TestSlugIsNoKey(t *testing.T) {
	o := Options{Secrets: true}
	for _, s := range []string{
		"https://www.cnbc.com/2026/10/28/sk-hynix-third-quarter-profit.html",
		"https://finance.yahoo.com/markets/stocks/articles/sk-hynix-announces-3q26-financial-234500011.html",
		"https://www.techpowerup.com/354203/sk-hynix-announces-3q26-financial-results",
		"https://au.investing.com/news/stock-market-news/sk-hynix-q3-2026-slides-revenue-tops-100t-80-margin-on-ai-boom",
	} {
		if out, n := Mask(s, o); n != 0 || out != s {
			t.Errorf("%q masked: %q", s, out)
		}
	}
	for _, key := range []string{
		"sk-proj-abcdEFGH1234ijklMNOP5678qrst",
		"sk-" + strings.Repeat("0123456789abcdef", 2),
		"sk-or-v1-" + strings.Repeat("0123456789abcdef", 4), // built, as a literal one stops GitHub's push
		"sk-ant-api03-Ab3dE-fGh1_jKl2mN-oPq3rSt4",
		"sk-" + "1f0e3dad-9990-4b5c-8a1b-2c3d4e5f6a7b", // a UUID in lower case: no capital, no run of 16
	} {
		in := "url sk-hynix-third-quarter-profit key " + key
		out, n := Mask(in, o)
		if n != 1 || strings.Contains(unmarked(out), key) || !strings.Contains(out, "sk-hynix-third-quarter-profit") {
			t.Errorf("%q: %d masked: %q", key, n, out)
		}
	}
}
