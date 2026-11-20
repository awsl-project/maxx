package redact

import (
	"strings"
	"testing"
)

// A disassembly's symbols read as addresses: a function reached through
// the PLT or the GOT (system@plt, puts@got.plt) and a symbol's version
// (memcpy@GLIBC_2.14, foo@@GLIBC_2.2.5). .got is a top-level domain, and
// the local model took system@got for an email, so a reverse engineer
// had to turn masking off (ch35tnut_ on X, 10-28). These are objdump's,
// readelf's, nm's, gdb's and IDA's lines as they print them.
var symbolLines = []string{
	"  401126:\te8 05 ff ff ff       \tcall   401030 <system@plt>",
	"0000000000401030 <system@plt>:",
	"  401030:\tff 25 e2 2f 00 00    \tjmp    *0x2fe2(%rip)        # 404018 <system@GLIBC_2.2.5>",
	"  4011a2:\tff 15 48 2e 00 00    \tcall   *0x2e48(%rip)        # 403ff0 <puts@got.plt>",
	"  401156:\te8 e5 fe ff ff       \tcall   401040 <_ZNSt8ios_base4InitC1Ev@plt>",
	"   7: 0000000000000000     0 FUNC    GLOBAL DEFAULT  UND printf@GLIBC_2.2.5 (2)",
	"0000000000000000       F *UND*\t0000000000000000              memcpy@GLIBC_2.14",
	"0000000000001139 g     F .text\t000000000000001e              foo@@GLIBC_2.2.5",
	"                 U __libc_start_main@GLIBC_2.34",
	" foo@Base 1.0",
	"   0x0000555555555171 <+8>:\tcall   0x555555555030 <printf@plt>",
	"\tcall\tprintf@PLT",
	"call    _system",
	".plt.sec:0000000000001060 jmp     cs:off_3FD0 ; system@got",
	"extrn system@@GLIBC_2_2_5:near",
	"  401040:\tff 25 d2 2f 00 00    \tjmp    *0x2fd2(%rip)        # 404018 <memcpy@plt.sec>",
	"  401050:\tff 25 ca 2f 00 00    \tjmp    *0x2fca(%rip)        # 403fe0 <free@plt.got>",
}

func TestSymbolIsNoEmail(t *testing.T) {
	o := Options{Secrets: true, Personal: true}
	for _, s := range symbolLines {
		if out, n := Mask(s, o); n != 0 || out != s {
			t.Errorf("rule: %q masked: %q", s, out)
		}
	}

	// nor does the local model's EMAIL take one
	f := &fakeModel{values: map[string]string{}}
	for _, v := range []string{"system@plt", "system@GLIBC_2.2.5", "puts@got.plt", "_ZNSt8ios_base4InitC1Ev@plt",
		"printf@GLIBC_2.2.5", "memcpy@GLIBC_2.14", "foo@@GLIBC_2.2.5", "__libc_start_main@GLIBC_2.34", "foo@Base",
		"printf@plt", "printf@PLT", "system@got", "free@plt.got", "system@@GLIBC_2_2_5", "memcpy@plt.sec"} {
		f.values[v] = "private_email"
	}
	for _, s := range symbolLines {
		if out, n := Mask(s, Options{Personal: true, Model: f.find}); n != 0 {
			t.Errorf("model: %q masked: %q", s, out)
		}
	}

	// an address is still one: in prose, in code, at a host, by the model
	f = &fakeModel{values: map[string]string{"rina.k@fastmail.com": "private_email", "ops@build01.acme.io": "private_email"}}
	for _, s := range []string{
		"call 401030 <system@plt> then mail rina.k@fastmail.com",
		`	const owner = "rina.k@fastmail.com" // see <puts@got.plt>`,
		"objdump -d ./a.out | grep system@plt, and send it to ops@build01.acme.io",
	} {
		for _, m := range []Model{nil, f.find} {
			out, n := Mask(s, Options{Personal: true, Model: m})
			if n != 1 || strings.Contains(out, "rina.k@") || strings.Contains(out, "ops@build01") || !strings.Contains(out, "puts@got.plt") && !strings.Contains(out, "system@plt") {
				t.Errorf("%q: %d masked: %q", s, n, out)
			}
		}
	}
}
