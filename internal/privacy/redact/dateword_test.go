package redact

import (
	"strings"
	"testing"
	"time"
)

// JackyCcao on X turned birthdays off and put their own birthday in
// Masked words, and asked that a date word catch the same date written
// with another separator or as 年月日. A date word matches that calendar
// date in every common form, and never another date or a longer number.
func TestDateWordEveryForm(t *testing.T) {
	forms := []string{
		"1990-05-12", "1990-5-12", "1990/05/12", "1990/5/12", "1990.05.12", "1990.5.12",
		"1990、5、12", "1990 5 12", "1990 / 05 / 12", "1990. 5. 12",
		"1990年5月12日", "1990年05月12日", "1990 年 5 月 12 日", "1990年5月12号", "1990年5月12",
		"1990년 5월 12일", "19900512",
		"May 12, 1990", "May 12th 1990", "may 12 1990", "12 May 1990", "12-May-1990",
	}
	for _, word := range []string{"1990-05-12", "1990/5/12", "1990年5月12日", "May 12, 1990", "12 May 1990"} {
		o := Options{Words: []string{word}}
		for _, f := range forms {
			for _, in := range []string{f, "生日是" + f + "，记得", "born " + f + ".", `{"dob":"` + f + `"}`, "IMG_" + f + "_1.jpg", f + "T08:00:00Z"} {
				out, n := Mask(in, o)
				if n != 1 || strings.Contains(out, f) {
					t.Errorf("word %q: %q masked %d as %q", word, in, n, out)
					continue
				}
				if back := Restore(out, false); back != in {
					t.Errorf("word %q: %q came back as %q", word, in, back)
				}
			}
		}
	}
}

// Day-first and month-first forms only where one reading is a date: for
// May 13th both 13/05/1990 and 05/13/1990 are; for May 12th neither
// 05/12/1990 nor 12/05/1990 (December 5th) is, and a word written so is
// matched only as written.
func TestDateWordDayMonthOrder(t *testing.T) {
	for _, c := range []struct {
		word, in string
		n        int
	}{
		{"1990-05-13", "13/05/1990", 1},
		{"1990-05-13", "05/13/1990", 1},
		{"1990-05-13", "13.5.1990", 1},
		{"13.05.1990", "1990年5月13日", 1},
		{"05/13/1990", "1990-05-13", 1},
		{"1990-05-05", "05/05/1990", 1},
		{"1990-05-12", "05/12/1990", 0},
		{"1990-05-12", "12/05/1990", 0},
		{"1990-05-12", "12.05.1990", 0},
		{"05/12/1990", "1990-05-12", 0}, // May 12th or December 5th: no date
		{"05/12/1990", "1990-12-05", 0},
		{"05/12/1990", "on 05/12/1990.", 1}, // still the word as written
	} {
		if _, n := Mask(c.in, Options{Words: []string{c.word}}); n != c.n {
			t.Errorf("word %q: %q masked %d, want %d", c.word, c.in, n, c.n)
		}
	}
}

// A date word never matches another date, a longer number, or a version
// that starts as the date does.
func TestDateWordNotAnotherDate(t *testing.T) {
	for _, word := range []string{"1990-05-12", "1990.5.12", "1990年5月12日"} {
		for _, in := range []string{
			"1990-05-13", "1990-12-05", "1991-05-12", "1990年5月1日", "1990年5月120日", "1990/05/1",
			"1990.5.12.1", "v1990.5.12.1", "3.1990.5.12", "11990-05-12", "1990-05-123", "199005123", "2199005120",
			"19900513", "1990512", "May 13, 1990", "Mayday 12 1990", "12/05/1990", "05/12/1990",
		} {
			if out, n := Mask(in, Options{Words: []string{word}}); n != 0 || out != in {
				t.Errorf("word %q: %q masked as %q", word, in, out)
			}
		}
	}
	// a full stop after the date ends a sentence; it is no version
	if _, n := Mask("生日 1990.5.12.", Options{Words: []string{"1990-05-12"}}); n != 1 {
		t.Errorf("date before a full stop not masked")
	}
}

// What is no date stays a word as before: eight digits (as likely an id),
// a date that isn't one, a word with a date in it.
func TestDateWordOnlyADate(t *testing.T) {
	for _, c := range []struct{ word, in string }{
		{"19900512", "1990-05-12"},
		{"1990-02-30", "1990/2/30"},
		{"1990-13-01", "1990/13/1"},
		{"release-1990-05-12", "1990-05-12"},
	} {
		if out, n := Mask(c.in, Options{Words: []string{c.word}}); n != 0 {
			t.Errorf("word %q: %q masked as %q", c.word, c.in, out)
		}
	}
	// and is found inside a longer one as before
	if _, n := Mask("x19900512y", Options{Words: []string{"19900512"}}); n != 1 {
		t.Errorf("eight-digit word no longer found inside a longer one")
	}
}

// The birthday rule needs a birthday's word right before the date, so a
// log's or a task's own date is not one, however old. A date of birth
// stored as a time, as a database or JavaScript writes it, is still a
// birthday: the time beside a date doesn't say it is a log's (#3878).
func TestBirthdayLogDates(t *testing.T) {
	defer func(c func() time.Time) { dateClock = c }(dateClock)
	dateClock = func() time.Time { return time.Date(2026, 10, 27, 9, 0, 0, 0, time.Local) }
	for _, s := range []string{
		"2019-03-02 10:00:01 INFO birthday reminder sent to user 42",
		"[2019-03-02T10:00:01Z] 生日提醒已发送",
		"logs/2019-03-02.log: born-again job finished",
		"任务创建于 2019-03-02，生日活动筹备",
		"2019-03-02 生日",
	} {
		if out, n := Mask(s, allKinds()); n != 0 || out != s {
			t.Errorf("%q masked as %q", s, out)
		}
	}
	for _, s := range []string{
		`{"birthday":"1990-05-11T16:00:00.000Z"}`,
		"出生日期：1990-05-12 00:00:00",
		"生日：2019-03-02",
	} {
		if out, _ := Mask(s, allKinds()); !strings.Contains(out, "BIRTHDAY") {
			t.Errorf("%q not masked: %q", s, out)
		}
	}
}
