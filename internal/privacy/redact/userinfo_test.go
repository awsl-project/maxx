package redact

import (
	"regexp"
	"strings"
	"testing"
)

// A URL's user:password@host is no address. With Mask personal data on,
// postgres://app:hunter2pass@db.acme-corp.cn:5432/x went as
// app:{{EMAIL_…}}:5432, the password and the host one EMAIL (#3538). The
// password is a PASSWORD, and the host goes as written.
func TestURLUserinfoIsNoEmail(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	ph := func(kind string) string { return `\{\{` + kind + `_[a-z2-7]{8}\}\}` }
	for _, c := range []struct{ in, want string }{
		// the report's literal string
		{"postgres://app:hunter2pass@db.acme-corp.cn:5432/x", `^postgres://app:` + ph("PASSWORD") + `@db\.acme-corp\.cn:5432/x$`},
		{"mysql://root:Tr0ub4dor3@mysql.acme-corp.cn/shop", `^mysql://root:` + ph("PASSWORD") + `@mysql\.acme-corp\.cn/shop$`},
		{"amqp://guest:rabbitPw9@mq.acme-corp.cn:5672/", `^amqp://guest:` + ph("PASSWORD") + `@mq\.acme-corp\.cn:5672/$`},
		{"mongodb+srv://svc:m0ng0Secret@cluster0.ab1cd.mongodb.net/app?retryWrites=true", `^mongodb\+srv://svc:` + ph("PASSWORD") + `@cluster0\.ab1cd\.mongodb\.net/app\?retryWrites=true$`},
		{"clone https://deploy:gitPa55word@git.acme-corp.cn/team/repo.git now", `^clone https://deploy:` + ph("PASSWORD") + `@git\.acme-corp\.cn/team/repo\.git now$`},
		// Redis has a password and no user
		{"REDIS_URL=redis://:r3disPass@cache.acme-corp.cn:6379/0", `^REDIS_URL=redis://:` + ph("PASSWORD") + `@cache\.acme-corp\.cn:6379/0$`},
		// a user and no password: neither part is an address
		{"git clone https://yetone@github.com/yetone/magpie.git", `^git clone https://yetone@github\.com/yetone/magpie\.git$`},
		{"ssh://git@gitlab.acme-corp.cn:2222/team/repo.git", `^ssh://git@gitlab\.acme-corp\.cn:2222/team/repo\.git$`},
		// an address is still one: in prose, in mailto:, in a URL's path or query
		{"mail rina.k@fastmail.com today", `^mail ` + ph("EMAIL") + ` today$`},
		{"<a href=\"mailto:rina.k@fastmail.com\">", `^<a href="mailto:` + ph("EMAIL") + `">$`},
		{"https://lists.acme-corp.cn/archive/rina.k@fastmail.com", `^https://lists\.acme-corp\.cn/archive/` + ph("EMAIL") + `$`},
		{"https://acme-corp.cn/invite?to=rina.k@fastmail.com", `^https://acme-corp\.cn/invite\?to=` + ph("EMAIL") + `$`},
		{"https://deploy:gitPa55word@git.acme-corp.cn/?cc=rina.k@fastmail.com", `^https://deploy:` + ph("PASSWORD") + `@git\.acme-corp\.cn/\?cc=` + ph("EMAIL") + `$`},
	} {
		if out, _ := Mask(c.in, o); !regexp.MustCompile(c.want).MatchString(out) {
			t.Errorf("%q went as %q", c.in, out)
		}
	}

	// with only Mask secrets on, Redis's password is masked too: the rule
	// asked for a user before it (a value of its own, not one known from above)
	if out, _ := Mask("redis://:r3disOnly7@cache.acme-corp.cn:6379/0", Options{Secrets: true}); !regexp.MustCompile(`^redis://:` + ph("PASSWORD") + `@cache\.acme-corp\.cn:6379/0$`).MatchString(out) {
		t.Errorf("secrets only: %q", out)
	}

	// as a request carries it
	body := `{"messages":[{"role":"user","content":"connect to postgres://app:hunter2pass@db.acme-corp.cn:5432/x"}]}`
	out, n := MaskJSON([]byte(body), o)
	if n != 1 || strings.Contains(string(out), "hunter2pass") || strings.Contains(string(out), "EMAIL_") || !strings.Contains(string(out), "@db.acme-corp.cn:5432/x") {
		t.Errorf("request: %d masked: %s", n, out)
	}

	// nor does the local model's EMAIL take the password and the host
	f := &fakeModel{values: map[string]string{"hunter2pass@db.acme-corp.cn": "private_email"}}
	in := "postgres://app:hunter2pass@db.acme-corp.cn:5432/x"
	if out, _ := Mask(in, Options{Personal: true, Model: f.find}); out != in {
		t.Errorf("model's: %q", out)
	}
}
