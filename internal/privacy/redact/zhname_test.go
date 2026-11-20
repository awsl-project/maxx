package redact

import (
	"strings"
	"testing"
)

// Chinese names are masked with the model on, both those it finds and
// those it misses. modelSaid is what the real model (privacy-filter q8,
// threshold 0.5, on an M4 Pro) found in each sentence, run on as it was.
func TestChineseNames(t *testing.T) {
	modelSaid := map[string]string{
		"王芳，她下周一": "private_person",
		"刘洋":      "private_person",
		"赵丽颖":     "private_person",
		"欧阳娜娜":    "private_person",
		"司马光":     "private_person",
		"张伟先生":    "private_person",
		"周杰伦":     "private_person",
		"田中太郎":    "private_person",
		// as it might: a name spaced out, a surname and a title
		"张 伟": "private_person",
		"李老师": "private_person",
	}
	f := &fakeModel{values: modelSaid}
	o := Options{Personal: true, Model: f.find}
	for _, c := range []struct {
		s     string
		names []string // masked
		keep  []string // left as they are
	}{
		{"帮我写封信给张伟，他住在北京市朝阳区建国路88号", []string{"张伟"}, []string{"，他住在"}},
		{"请把合同发给王芳，她下周一到上海。", []string{"王芳"}, []string{"，她下周一到上海"}},
		{"李娜和刘洋昨天开会讨论了项目进度。", []string{"李娜", "刘洋"}, []string{"和", "昨天开会"}},
		{"客户陈建国反馈导出功能在他电脑上失败了。", []string{"陈建国"}, []string{"反馈导出"}},
		{"客户张三反馈导出失败", []string{"张三"}, []string{"反馈"}},
		{"联系人：赵丽颖，手机", []string{"赵丽颖"}, nil},
		{"提交者是欧阳娜娜，审核人是司马光。", []string{"欧阳娜娜", "司马光"}, []string{"审核人是"}},
		{"张伟先生，您好！感谢您的来信。", []string{"张伟"}, []string{"先生，您好"}},
		{"我叫黄晓明，是这个仓库的维护者。", []string{"黄晓明"}, []string{"我叫"}},
		{"// author: 周杰伦", []string{"周杰伦"}, nil},
		{"给小王发个消息，说老李明天不来了。", []string{"小王", "老李"}, []string{"发个消息", "明天不来了"}},
		{"马云说过，今天很残酷。", []string{"马云"}, []string{"说过"}},
		{"田中太郎さんに連絡してください。", []string{"田中太郎"}, nil},
		{"转交张 伟处理", []string{"张 伟", "伟"}, nil},
		{"感谢李老师的帮助", []string{"李老师", "李"}, nil},
		// none of these is a name
		{"这个函数的作用是把高度转换成像素。", nil, []string{"高度"}},
		{"王国的军队在黄河边扎营。", nil, []string{"王国", "黄河"}},
		{"我们需要一个更高效的排序算法。", nil, []string{"高效"}},
		{"请帮我修复这个 bug，张开的括号没有闭合。", nil, []string{"张开"}},
		{"给张量赋值之前先检查形状。", nil, []string{"张量"}},
		{"在建国路上，路边有一家店。", nil, []string{"建国路", "路边"}},
		{"把方法和文件名都改一下", nil, []string{"方法", "文件"}},
		{"给老师发个消息", nil, []string{"老师"}},
		{"王老板说这个需求下周要上线，邓总明天要来看演示。", []string{"王老板", "邓总"}, []string{"明天要来"}},
		{"请帮我回复一下孙经理的邮件", []string{"孙经理"}, []string{"的邮件"}},
		{"抄送给吴敏和郑浩。", []string{"吴敏", "郑浩"}, nil},
		{"负责人：许文强", []string{"许文强"}, nil},
		{"我是郭靖，想咨询一下退款的问题。", []string{"郭靖"}, []string{"想咨询"}},
		{"和平时一样，周五发版。", nil, []string{"周五", "和平"}},
		{"方总结一下：说明文档里要写清楚安装步骤。", nil, []string{"方总结", "说明"}},
		{"运行测试的时候，内存一直在涨。签名和工具调用都会保留。", nil, []string{"签名和工具"}},
		// xuanwo on X: words of a report, masked as 关键, 易触 and 边界
		{"### 关键发现：5 组 bot/人类重复 PR", nil, []string{"关键发现"}},
		{"`gh pr view` 易触发 GraphQL rate limit", nil, []string{"易触发"}},
		{"\n\n先读判断和边界，仅当来源", nil, []string{"和边界，"}},
		{"打开文件之前，边界检查失败。", nil, []string{"边界检查"}},
		// and names around the same cues still are
		{"### 张伟发了个消息", []string{"张伟"}, []string{"发了个消息"}},
		{"我和张伟，还有他。", []string{"张伟"}, []string{"我和"}},
		{"这件事。 李娜说过。", []string{"李娜"}, []string{"说过"}},
		// zeewell on X: 文案 (文 a surname) masked as a name; prompts it
		// is in, each with a cue that was taken for a person's: 一下
		// before it, 已经, 明天 or 他们 after it at a sentence's start, 说
		// of 说明 after it
		{"帮我改一下这段文案", nil, []string{"文案"}},
		{"润色一下文案。", nil, []string{"文案"}},
		{"检查一下文案，看看有没有错别字", nil, []string{"文案"}},
		{"文案：首页标题换成「更快的网关」", nil, []string{"文案"}},
		{"把首页文案换成英文", nil, []string{"文案"}},
		{"### 文案\n\n- 文案和图片都要改", nil, []string{"文案"}},
		{"文案已经改好了，你再看看。", nil, []string{"文案"}},
		{"文案明天给你，图片今天先发。", nil, []string{"文案"}},
		{"文案说明要简短，不超过两行。", nil, []string{"文案说明"}},
		{"文案他们说下午定稿", nil, []string{"文案他们"}},
		{"这张图发给文案。", nil, []string{"文案"}},
		// and names in the same places still are
		{"问一下张伟，这段谁来写", []string{"张伟"}, []string{"问一下"}},
		{"张伟说明天来改。", []string{"张伟"}, []string{"说明天"}},
		{"客户张伟已经到了。", []string{"张伟"}, []string{"已经到了"}},
		{"这张图发给张伟。", []string{"张伟"}, []string{"发给"}},
	} {
		out, _ := Mask(c.s, o)
		for _, n := range c.names {
			if strings.Contains(out, n) {
				t.Errorf("%q: %q not masked: %q", c.s, n, out)
			}
		}
		for _, k := range c.keep {
			if !strings.Contains(out, k) {
				t.Errorf("%q: %q masked: %q", c.s, k, out)
			}
		}
		if back := Restore(out, false); back != c.s {
			t.Errorf("round trip: %q", back)
		}
	}
}

// A word that starts with a surname is no name in the places 文案 was
// taken for one (zeewell on X), whichever word it is: zhName's cues, and
// the model's own PERSON label on the word as a whole.
func TestChineseWordsAreNoNames(t *testing.T) {
	words := []string{"文案", "文档", "文风", "方案", "高度", "安全", "马上", "程序", "林间", "关键", "项目", "成本", "段落", "颜色", "查询", "时区", "季度", "边框", "简历", "游戏", "严格", "曾经", "谢谢", "解析", "付费"}
	shapes := []string{"改一下%s", "检查一下%s。", "%s已经好了", "%s明天给你", "%s说明", "%s他们定", "请%s", "发给%s", "%s：", "把%s换成英文"}
	f := &fakeModel{values: map[string]string{}}
	o := Options{Personal: true, Model: f.find}
	for _, w := range words {
		for _, sh := range shapes {
			s := strings.ReplaceAll(sh, "%s", w)
			if out, _ := Mask(s, o); out != s {
				t.Errorf("%q masked: %q", s, out)
			}
		}
		f.values = map[string]string{w: "private_person"}
		s := "帮我改一下这段" + w + "，谢谢"
		if out, _ := Mask(s, o); out != s {
			t.Errorf("%q, the model's: masked: %q", s, out)
		}
		f.values = map[string]string{}
	}
	// words not in zhWords are left by the cues alone, but for one
	// a person's verb is before (请文创, 发给文创)
	for _, w := range []string{"文创", "方位", "高铁", "安防", "林荫"} {
		for _, sh := range shapes[:6] {
			s := strings.ReplaceAll(sh, "%s", w)
			if out, _ := Mask(s, o); out != s {
				t.Errorf("%q masked: %q", s, out)
			}
		}
	}
}

// Without the model, or with personal data off, no Chinese name is
// looked for.
func TestChineseNamesNeedModel(t *testing.T) {
	const s = "客户钟离秋反馈导出失败" // a name no other test masks: one masked once stays masked (known.go)
	if out, _ := Mask(s, Options{Personal: true}); out != s {
		t.Fatalf("no model: %q", out)
	}
	f := &fakeModel{}
	if out, _ := Mask(s, Options{Secrets: true, Model: f.find}); out != s {
		t.Fatalf("personal data off: %q", out)
	}
}

// 毛利 is a gross margin: 毛利低 is no one (_kaichen on X, 10-28), in the
// sentences that made it a name. The rules leave 毛利兰 and 毛利小五郎 to
// the model too, which reads the sentence; what it finds is masked.
func TestMaoliIsAMargin(t *testing.T) {
	f := &fakeModel{values: map[string]string{}}
	o := Options{Personal: true, Model: f.find}
	for _, s := range []string{"毛利低", "毛利低说", "我是毛利低", "这个产品毛利低说明定价有问题", "我是毛利低的那个品类负责人", "毛利低说明成本高", "告诉毛利低的门店先别补货", "毛利率下降了", "毛利额是多少", "看一下毛利润"} {
		if out, _ := Mask(s, o); out != s {
			t.Errorf("%q masked: %q", s, out)
		}
	}
	f.values = map[string]string{"毛利小五郎": "private_person"}
	if out, _ := Mask("毛利小五郎姓什么", o); strings.Contains(out, "毛利小五郎") {
		t.Errorf("the model's name not masked: %q", out)
	}
}

// Where the model finds Chinese names itself (AstrLink Guard,
// NamesByModel), only a nickname is found beside it: a surname with a
// cue around it is the model's to find, and the cues took words for
// names (关键发现, 易触发; xuanwo on X).
func TestChineseNamesByModelOnlyNicks(t *testing.T) {
	f := &fakeModel{values: map[string]string{"龚秋实": "private_person"}}
	o := Options{Personal: true, Model: f.find, NamesByModel: true}
	for _, c := range []struct{ s, masked, kept string }{
		{"小邵说他明天到", "小邵", "说他明天到"},
		{"客户龚秋实和小邵反馈导出失败", "龚秋实和小邵", "反馈导出失败"},
		{"客户钟离夏反馈导出失败", "", "钟离夏"},
		{"## 关键发现", "", "关键发现"},
	} {
		out, _ := Mask(c.s, o)
		if strings.Contains(out, c.masked) && c.masked != "" || !strings.Contains(out, c.kept) {
			t.Errorf("%q: %q", c.s, out)
		}
	}
}

// A word the model takes for a name is left as written (v0.1.1850 on X):
// colours (黄绿色, hh05928822), 关键词 (wangjunxiao128) and the words
// that start with a surname, as Guard took 边…, 应…, 关… and 尚… in
// hh05928822's screenshot. Each sentence is asked of the model's span and
// of the cues alone.
func TestModelWordsAreNoNames(t *testing.T) {
	words := []string{"黄绿色", "黄绿", "红色", "金黄色", "浅蓝色", "蓝绿色", "关键词", "关键字", "关键点", "边界", "边界线", "边距", "应用层", "应该", "尚可", "尚无", "毛利率", "温度计", "程序员", "文件名", "时间戳", "关心", "包括", "管理", "连接"}
	shapes := []string{"%s", "把按钮改成%s", "提取这段文本的%s", "帮我改一下这段%s，谢谢", "%s已经好了", "%s说明", "请%s", "发给%s。", "检查一下%s。", "张伟和%s", "%s：首页"}
	f := &fakeModel{values: map[string]string{}}
	o := Options{Personal: true, Model: f.find}
	for _, w := range words {
		for _, sh := range shapes {
			s := strings.ReplaceAll(sh, "%s", w)
			for _, said := range []map[string]string{{}, {w: "private_person"}} {
				f.values = said
				out, _ := Mask(s, o)
				if strings.Contains(sh, "张伟") {
					out = strings.Replace(out, out[:strings.Index(out, "和")], "张伟", 1)
				}
				if out != s {
					t.Errorf("%q (model said %v) masked: %q", s, said, out)
				}
			}
		}
	}
}

// Names stay masked, the model's and the cues': 黄 starts 黄绿色 and
// 黄晓明, 关 starts 关键词 and 关羽, 毛利 starts 毛利率 and 毛利兰. A
// name that is also a word (方圆, 高歌, 江山) is masked where a role
// before it or a title after it says a person is meant.
func TestModelNamesBesideWords(t *testing.T) {
	f := &fakeModel{values: map[string]string{"黄晓明": "private_person", "关羽": "private_person", "毛利兰": "private_person", "方圆": "private_person", "高歌": "private_person", "江山": "private_person", "黄蓉": "private_person"}}
	o := Options{Personal: true, Model: f.find}
	for _, c := range []struct{ s, name string }{
		{"我叫黄晓明，衣服是黄绿色的", "黄晓明"},
		{"关羽的关键词是忠义", "关羽"},
		{"毛利兰的毛利率", "毛利兰"},
		{"黄蓉穿着红色的衣服", "黄蓉"},
		{"客户方圆反馈导出失败", "方圆"},
		{"联系人：高歌", "高歌"},
		{"感谢江山老师的帮助", "江山"},
	} {
		out, _ := Mask(c.s, o)
		if strings.Contains(out, c.name) {
			t.Errorf("%q: %q not masked: %q", c.s, c.name, out)
		}
		for _, w := range []string{"黄绿色", "关键词", "毛利率", "红色"} {
			if strings.Contains(c.s, w) && !strings.Contains(out, w) {
				t.Errorf("%q: %q masked: %q", c.s, w, out)
			}
		}
	}
	// and with no one meant, the word is a word
	for _, s := range []string{"天下方圆百里", "江山如画"} {
		if out, _ := Mask(s, o); out != s {
			t.Errorf("%q masked: %q", s, out)
		}
	}
}
