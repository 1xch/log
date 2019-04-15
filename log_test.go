package log

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func errOut(t *testing.T, w, m string) {
	t.Errorf("LOG TEST ERROR\n%s: %s", w, m)
}

func TestLevel(t *testing.T) {
	for _, v := range Levels {
		tl := v
		tls := tl.String()
		if StringToLevel(tls) != tl && tl.String() != tls {
			errOut(t, "Level", fmt.Sprintf("variable not equal to level and viceversa for %v", v))
		}
	}
	//Color()
}

func TestEntry(t *testing.T) {
	b := new(bytes.Buffer)
	l := New(b, LDebug, "TEST")
	l.SwapFormatter("raw")
	f := []Field{
		{0, "one", "first field-"},
		{1, "two", "second field-"},
		{2, "three", "third field"},
	}
	e := newEntry(l, LDebug, f...)
	r, err := e.Read()
	if err != nil {
		errOut(t, "Entry", err.Error())
	}
	v := r.String()
	vexp := "first field-second field-third field\n"
	if v != vexp {
		errOut(t, "Entry", fmt.Sprintf("%s instead of expected %s", v, vexp))
	}
	if len(e.Fields()) != len(f) {
		errOut(t, "Entry", "Fields length error")
	}
	e.SetEntryLevel(LDebug)
	if e.EntryLevel() != LDebug {
		errOut(t, "Entry", "EntryLevel error")
	}
	if e.created != e.Created() {
		errOut(t, "Entry", "Created function not equal to struct data")
	}
}

func TestColor(t *testing.T) {
	// spew.Dump(NoColor)
}

type fmtr struct {
	kind, message   string
	expect, xexpect []string
}

var fmtrs []fmtr = []fmtr{
	{"null", "", []string{""}, []string{"MESSAGE", "DEBUG"}},
	{"raw", "MESSAGE", []string{"MESSAGE"}, []string{"DEBUG"}},
	{"text", "MESSAGE", []string{"INFO", "TEST"}, []string{"PANIC"}},
}

func contains(s string, exp []string) bool {
	for _, v := range exp {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}

func TestFormatter(t *testing.T) {
	b := new(bytes.Buffer)
	l := New(b, LDebug, "TEST")
	for _, f := range fmtrs {
		l.SwapFormatter(f.kind)
		l.Print(f.message)
		res := b.String()
		if !contains(res, f.expect) {
			errOut(t, "Formatter", fmt.Sprintf("output is %s, expected %s", res, f.expect))
		}
		if contains(res, f.xexpect) {
			errOut(t, "Formatter", fmt.Sprintf("%s was found in %s, but should not", res, f.expect))
		}
		b.Reset()
	}
}

type testHook struct {
	ran bool
}

func (h *testHook) Fire(e Entry) error {
	h.ran = true
	return nil
}

func TestHook(t *testing.T) {
	b := new(bytes.Buffer)
	l1 := New(b, LDebug, "no")
	h1 := &testHook{}
	l1.AddHook(POST, LError, h1)
	l1.Print("RUN1")
	if h1.ran {
		errOut(t, "Hook", "hook run, but not expected to")
	}
	l2 := New(b, LDebug, "yes")
	h2 := &testHook{}
	l2.AddHook(POST, LDebug, h2)
	l2.Log(newEntry(l2, LDebug, mkFields(0, "RUN2")...))
	if !h2.ran {
		errOut(t, "HOOK", "hook not run, but should have")
	}
}

func probe(t *testing.T, tag, exp string, fn func(), b *bytes.Buffer) {
	fn()
	res := b.String()
	if res != exp {
		errOut(t, tag, fmt.Sprintf("got %s, but expected %s", res, exp))
	}
	b.Reset()
}

func TestPrint(t *testing.T) {
	b := new(bytes.Buffer)
	l := New(b, LDebug, "TEST")
	l.SwapFormatter("raw")

	message := "MESSAGE"
	exp := "MESSAGE\n"

	probe(t, "Print", exp, func() { l.Print(message) }, b)
	probe(t, "Printf", exp, func() { l.Printf("%s", message) }, b)
	probe(t, "Println", exp, func() { l.Println(message) }, b)
}

func testFatal(t *testing.T, name string, fn func(Logger)) {
	if os.Getenv("FATAL") == "1" {
		b := new(bytes.Buffer)
		l := New(b, LFatal, "TEST")
		fn(l)
		return
	}
	cmd := exec.Command(os.Args[0], fmt.Sprintf("-test.run=%s", name))
	cmd.Env = append(os.Environ(), "FATAL=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		return
	}
	errOut(t, name, fmt.Sprintf("process ran with err %v, want exit status 1", err))
}

func TestFatal(t *testing.T) {
	testFatal(t, "TestFatal", func(l Logger) { l.Fatal("FATAL!") })
}

func TestFatalf(t *testing.T) {
	testFatal(t, "TestFatalf", func(l Logger) { l.Fatalf("%s", "FATAL!") })
}

func TestFatalln(t *testing.T) {
	testFatal(t, "TestFatalln", func(l Logger) { l.Fatalln("FATAL!") })
}

func testPanic(t *testing.T, fn func(Logger)) {
	b := new(bytes.Buffer)
	l := New(b, LDebug, "TEST")
	l.SwapFormatter("raw")
	defer func() {
		if r := recover(); r == nil {
			errOut(t, "Panic", "expected panic")
		}
	}()
	fn(l)
	errOut(t, "Panic", "expected panic")
}

func TestPanic(t *testing.T) {
	testPanic(t, func(l Logger) { l.Panic("PANIC!") })
}

func TestPanicf(t *testing.T) {
	testPanic(t, func(l Logger) { l.Panicf("%s", "PANIC!") })
}

func TestPanicln(t *testing.T) {
	testPanic(t, func(l Logger) { l.Panicln("PANIC!") })
}

func TestXtr(t *testing.T) {
	b := new(bytes.Buffer)
	l := New(b, LDebug, "TESTXTR")
	l.SwapFormatter("raw")

	message := "MESSAGE"
	exp := "MESSAGE\n"

	probe(t, "At", exp, func() { l.At(LInfo, message) }, b)
	probe(t, "Atf", exp, func() { l.Atf(LInfo, "%s", message) }, b)
	c := new(bytes.Buffer)
	probe(t, "AtTo", exp, func() { l.AtTo(LInfo, c, message) }, c)
	probe(t, "AtTof", exp, func() { l.AtTof(LInfo, c, "%s", message) }, c)
	e := newEntry(l, LInfo, mkFields(0, message)...)
	probe(t, "Log", exp, func() { l.Log(e) }, b)
}
