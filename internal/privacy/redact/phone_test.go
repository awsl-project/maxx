package redact

import (
	"strings"
	"testing"
)

// Eleven digits that start 13 to 19 are a phone only where something says
// so: a phone's marks (+86), a field named for one, or prose with no id's
// word before them. Ids of the same form go as written (_kaichen on X,
// 10-28: "长int，假如以1为开头的，刚好匹配到长度，会被当作电话被mask").
func TestPhoneNeedsAPhonesContext(t *testing.T) {
	Forget()
	defer Forget()
	o := Options{Secrets: true, Personal: true}
	masked := []struct{ s, v string }{
		{"我的手机号是 13800138000", "13800138000"},
		{"电话：13800138001", "13800138001"},
		{"联系电话：13800138012，找张经理", "13800138012"},
		{"联系 13800138008 张经理", "13800138008"},
		{"请打 13800138009", "13800138009"},
		{"+86 13800138002", "13800138002"},
		{"+8613800138003", "13800138003"},
		{"8613800138014", "13800138014"},
		{`{"phone": "13800138004"}`, "13800138004"},
		{`{"mobile":"13800138005"}`, "13800138005"},
		{`{"phoneNumber": 13800138015}`, ""}, // a bare JSON number is no phone (noNumbers), as before
		{`{"contact_phone": "13800138016"}`, "13800138016"},
		{"phone_number=13800138006", "13800138006"},
		{"tel: 13800138007", "13800138007"},
		{"<tel>13800138010</tel>", "13800138010"},
		{"<mobile type=\"cn\">13800138017</mobile>", "13800138017"},
		{"phone no. 13800138011", "13800138011"},
		{"Phone No: 13800138018", "13800138018"},
		{"mobile number: 13800138019", "13800138019"},
		{"call 13800138013 now", "13800138013"},
		{"mail me: jane.doe@acme.io or 13812345678", "13812345678"},
		{"13800138020", "13800138020"},
		{"张三 13800138021 北京", "13800138021"},
	}
	for _, c := range masked {
		out := Preview(c.s, o)
		if c.v == "" {
			continue
		}
		if strings.Contains(out, c.v) || !strings.Contains(out, "{{PHONE_") {
			t.Errorf("%q: phone not masked: %q", c.s, out)
		}
	}
	kept := []string{
		// _kaichen's: an id of a phone's form
		`{"order_id": "13800138100"}`,
		`{"id": "13912345678"}`,
		`{"userId":"15012345679"}`,
		"ts=2026-10-28T10:00:00Z level=info id=13912345679 msg=done",
		"订单号 13912345670 已发货",
		"订单号：13912345671",
		"订单号是 13912345673",
		"编号 13912345674",
		"user_id=15012345678&page=2",
		"SELECT * FROM orders WHERE id = 18612345678;",
		"id: 17712345678",
		"<id>17712345670</id>",
		"order 13800138101 failed",
		"Order #13800138103 shipped",
		"ticket 13800138104",
		"#13912345672",
		// a piece of a hash or of a longer word
		`{"hash": "9f13800138102abc"}`,
		"build-13800138105x",
		"v13800138106",
	}
	for _, s := range kept {
		if out := Preview(s, o); out != s {
			t.Errorf("%q masked: %q", s, out)
		}
	}
}
