package log

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/mattn/go-isatty"
)

// A interface to package local structured logging.
type StrLogger interface {
	Level() Level
}

// An interface to some methods of the standard library log package.
type StdLogger interface {
	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Fatalln(...interface{})
	Panic(...interface{})
	Panicf(string, ...interface{})
	Panicln(...interface{})
	Print(...interface{})
	Printf(string, ...interface{})
	Println(...interface{})
}

// An interface to package local logging functionality.
type XtrLogger interface {
	At(Level, ...interface{})
	Atf(Level, string, ...interface{})
	AtTo(Level, io.Writer, ...interface{})
	AtTof(Level, io.Writer, string, ...interface{})
	Log(Entry)
}

// Package level mutex to prevent interference with other mutexes.
type Mutex interface {
	Lock()
	Unlock()
}

type Logger interface {
	io.Writer
	StrLogger
	StdLogger
	XtrLogger
	Mutex
	Formatter
	FormatterManager
	Hooks
}

type logger struct {
	io.Writer
	level Level
	Formatter
	formatters formatters
	Hooks
	sync.Mutex
}

func New(w io.Writer, l Level, tag string) *logger {
	ret := &logger{
		Writer:     w,
		level:      l,
		formatters: defaultFormatters(tag),
		Hooks:      newHooks(),
	}
	ret.SwapFormatter("null")
	return ret
}

func (l *logger) Level() Level {
	return l.level
}

func log(e Entry) {
	fire(PRE, e.EntryLevel(), e)
	reader := read(e)
	e.Lock()
	copy(e, reader)
	e.Unlock()
	fire(POST, e.EntryLevel(), e)
}

func read(e Entry) *bytes.Buffer {
	reader, err := e.Read()
	if err != nil {
		fmt.Fprintf(os.Stdout, "log: Failed to obtain reader -- %v\n", err)
	}
	return reader
}

func copy(e Entry, r *bytes.Buffer) {
	_, err := io.Copy(e, r)
	if err != nil {
		fmt.Fprintf(os.Stdout, "log: Failed to write -- %v\n", err)
	}
}

func fire(at Timing, lv Level, e Entry) {
	if err := e.Fire(at, lv, e); err != nil {
		e.Lock()
		fmt.Fprintf(os.Stdout, "log: Failed to fire hook -- %v\n", err)
		e.Unlock()
	}
}

//
func (l *logger) Fatal(v ...interface{}) {
	if l.level >= LFatal {
		log(newEntry(l, LFatal, mkFields(0, v...)...))
	}
}

//
func (l *logger) Fatalf(format string, v ...interface{}) {
	if l.level >= LFatal {
		log(newEntry(l, LFatal, mkFormatFields(format, v...)...))
	}
}

//
func (l *logger) Fatalln(v ...interface{}) {
	if l.level >= LFatal {
		log(newEntry(l, LFatal, mkFields(0, v...)...))
	}
}

//
func (l *logger) Panic(v ...interface{}) {
	if l.level >= LPanic {
		log(newEntry(l, LPanic, mkFields(0, v...)...))
	}
}

//
func (l *logger) Panicf(format string, v ...interface{}) {
	if l.level >= LPanic {
		log(newEntry(l, LPanic, mkFormatFields(format, v...)...))
	}
}

//
func (l *logger) Panicln(v ...interface{}) {
	l.Panic(v...)
}

//
func (l *logger) Print(v ...interface{}) {
	if l.level >= LError {
		log(newEntry(l, LInfo, mkFields(0, v...)...))
	}
}

//
func (l *logger) Printf(format string, v ...interface{}) {
	if l.level >= LError {
		log(newEntry(l, LInfo, mkFormatFields(format, v...)...))
	}
}

//
func (l *logger) Println(v ...interface{}) {
	if l.level >= LError {
		log(newEntry(l, LInfo, mkFields(0, v...)...))
	}
}

//
func (l *logger) At(lv Level, v ...interface{}) {
	log(newEntry(l, lv, mkFields(0, v...)...))
}

//
func (l *logger) Atf(lv Level, m string, v ...interface{}) {
	log(newEntry(l, lv, mkFormatFields(m, v...)...))
}

//
func (l *logger) AtTo(lv Level, to io.Writer, v ...interface{}) {
	e := newEntry(l, lv, mkFields(0, v...)...)
	fire(PRE, lv, e)
	reader, _ := e.Read()
	io.Copy(to, reader)
	fire(POST, lv, e)
}

//
func (l *logger) AtTof(lv Level, to io.Writer, m string, v ...interface{}) {
	e := newEntry(l, lv, mkFormatFields(m, v...)...)
	fire(PRE, lv, e)
	reader, _ := e.Read()
	io.Copy(to, reader)
	fire(POST, lv, e)
}

//
func (l *logger) Log(e Entry) {
	log(e)
}

//
func (l *logger) SetFormatter(k string, f Formatter) {
	l.formatters[k] = f
}

//
func (l *logger) GetFormatter(k string) Formatter {
	if f, ok := l.formatters[k]; ok {
		return f
	}
	return &NullFormatter{}
}

//
func (l *logger) SwapFormatter(f string) {
	nf := l.GetFormatter(f)
	l.Lock()
	l.Formatter = nf
	l.Unlock()
}

//
type Hook interface {
	Fire(Entry) error
}

//
type HookFunc func(Entry) error

type hook struct {
	fn HookFunc
}

func hookFor(fn HookFunc) *hook {
	return &hook{fn}
}

func (h *hook) Fire(e Entry) error {
	return h.fn(e)
}

//
type Hooks interface {
	AddHook(Timing, Level, ...Hook)
	Fire(Timing, Level, Entry) error
}

type Timing int

const (
	PRE Timing = iota
	POST
)

type hooks struct {
	has map[Timing]map[Level][]Hook
}

func newHooks() *hooks {
	has := make(map[Timing]map[Level][]Hook)
	has[PRE] = make(map[Level][]Hook)
	has[POST] = make(map[Level][]Hook)
	h := &hooks{has}
	h.AddHook(POST, LFatal, hookFor(func(Entry) error { os.Exit(1); return nil }))
	h.AddHook(POST, LPanic, hookFor(func(Entry) error { panic("panic hook"); return nil }))
	return h
}

//
func (h *hooks) AddHook(t Timing, l Level, hk ...Hook) {
	m := h.has[t]
	m[l] = append(m[l], hk...)
	h.has[t] = m
}

//
func (h *hooks) Fire(at Timing, lv Level, e Entry) error {
	m := h.has[at]
	if l, ok := m[lv]; ok {
		for _, hook := range l {
			if err := hook.Fire(e); err != nil {
				return err
			}
		}
	}
	return nil
}

//
type Entry interface {
	Logger
	Fielder
	Reader
	Created() time.Time
	SetEntryLevel(l Level)
	EntryLevel() Level
}

//
type Fielder interface {
	Fields() []Field
}

//
type Field struct {
	Order int
	Key   string
	Value interface{}
}

func mkFields(index int, v ...interface{}) []Field {
	var ret []Field
	for i, vv := range v {
		idx := i + index
		ret = append(ret, Field{idx, fmt.Sprintf("Field%d", idx), vv})
	}
	return ret
}

func mkFormatFields(format string, v ...interface{}) []Field {
	var ret []Field
	ret = append(ret, Field{1, "Format", format})
	ret = append(ret, mkFields(1, v...)...)
	return ret
}

//
type FieldsSort []Field

//
func (f FieldsSort) Len() int {
	return len(f)
}

//
func (f FieldsSort) Less(i, j int) bool {
	return f[i].Order < f[j].Order
}

//
func (f FieldsSort) Swap(i, j int) {
	f[i], f[j] = f[j], f[i]
}

//
type Reader interface {
	Read() (*bytes.Buffer, error)
}

type entry struct {
	created time.Time
	level   Level
	Reader
	Logger
	fields []Field
}

func newEntry(l Logger, lv Level, f ...Field) *entry {
	return &entry{
		created: time.Now(),
		level:   lv,
		Logger:  l,
		fields:  f,
	}
}

//
func (e *entry) Read() (*bytes.Buffer, error) {
	s, err := e.Format(e)
	return bytes.NewBuffer(s), err
}

//
func (e *entry) Fields() []Field {
	return e.fields
}

//
func (e *entry) SetEntryLevel(l Level) {
	e.level = l
}

//
func (e *entry) EntryLevel() Level {
	if e.level != LUnrecognized {
		return e.level
	}
	return e.Level()
}

//
func (e *entry) Created() time.Time {
	return e.created
}

//
type Level int

const (
	LUnrecognized Level = iota //
	LPanic                     //
	LFatal                     //
	LError                     //
	LWarn                      //
	LInfo                      //
	LDebug                     //
)

var Levels []Level = []Level{LPanic, LFatal, LError, LWarn, LInfo, LDebug}

var stringToLevel = map[string]Level{
	"panic": LPanic,
	"fatal": LFatal,
	"error": LError,
	"warn":  LWarn,
	"info":  LInfo,
	"debug": LDebug,
}

//
func StringToLevel(lv string) Level {
	if level, ok := stringToLevel[strings.ToLower(lv)]; ok {
		return level
	}
	return LUnrecognized
}

//
func (l Level) String() string {
	switch l {
	case LPanic:
		return "panic"
	case LFatal:
		return "fatal"
	case LError:
		return "error"
	case LWarn:
		return "warn"
	case LInfo:
		return "info"
	case LDebug:
		return "debug"
	}
	return "unrecognized"
}

//
func (lv Level) Color() func(io.Writer, ...interface{}) {
	switch lv {
	case LPanic:
		return red
	case LFatal:
		return magenta
	case LError:
		return cyan
	case LWarn:
		return yellow
	case LInfo:
		return green
	case LDebug:
		return blue
	}
	return white
}

//
type Formatter interface {
	Format(Entry) ([]byte, error)
}

//
type FormatterManager interface {
	SetFormatter(string, Formatter)
	GetFormatter(string) Formatter
	SwapFormatter(string)
}

type formatters map[string]Formatter

func defaultFormatters(tag string) formatters {
	return formatters{
		"null": DefaultNullFormatter(),
		"raw":  DefaultRawFormatter(),
		"text": MakeTextFormatter(tag),
	}
}

type NullFormatter struct{}

func DefaultNullFormatter() Formatter {
	return &NullFormatter{}
}

func (n *NullFormatter) Format(e Entry) ([]byte, error) {
	return nil, nil
}

type RawFormatter struct{}

func DefaultRawFormatter() Formatter {
	return &RawFormatter{}
}

func (r *RawFormatter) Format(e Entry) ([]byte, error) {
	b := &bytes.Buffer{}
	fds := FieldsSort(e.Fields())
	format(b, fds)
	b.Write([]byte("\n"))
	return b.Bytes(), nil
}

// A text formatter
//
// Uses the package color functions (linux only) and involves a bit of
// overengineering with text.template where string+" " or fmt.Sprintf would
// work(this provides multiple templating options to work with in formatting as
// opposed to appending or formatting by package fmt).
//
// If minimalism is your thing, rewrite this.
type TextFormatter struct {
	Name            string
	TimestampFormat string
	Sort            bool
}

func MakeTextFormatter(name string) Formatter {
	return &TextFormatter{
		name,
		time.StampNano,
		false,
	}
}

func (t *TextFormatter) Format(e Entry) ([]byte, error) {
	fs := e.Fields()
	var keys []string = make([]string, 0, len(fs))
	for _, k := range fs {
		keys = append(keys, k.Key)
	}

	if t.Sort {
		sort.Strings(keys)
	}

	timestampFormat := t.TimestampFormat
	if timestampFormat == "" {
		timestampFormat = time.StampNano
	}

	b := &bytes.Buffer{}

	t.formatFields(b, e, keys, timestampFormat)

	b.WriteByte('\n')

	return b.Bytes(), nil
}

type TmplBase struct {
	t    *template.Template
	k, v string
}

var baseTmpls map[string]*TmplBase = map[string]*TmplBase{
	"LVL":  &TmplBase{nil, "Lvl", `{{.Lvl}} `},
	"NAME": &TmplBase{nil, "Name", `{{.Name}} `},
	"TIME": &TmplBase{nil, "Time", `{{.Time}} `},
}

func initializeTemplate(t *TmplBase) {
	var err error
	tx := template.New("")
	t.t, err = tx.Parse(t.v)
	if err != nil {
		panic(err)
	}
}

func initializeTemplates() {
	for _, v := range baseTmpls {
		initializeTemplate(v)
	}
}

func tmplTo(v string, b *bytes.Buffer, t *TmplBase) string {
	d := map[string]interface{}{t.k: v}
	t.t.Execute(b, d)
	return b.String()
}

func (t *TextFormatter) formatFields(b *bytes.Buffer, e Entry, keys []string, timestampFormat string) {
	tb := new(bytes.Buffer)

	lvl := e.EntryLevel()
	lvlColor := lvl.Color()
	lvlText := strings.ToUpper(lvl.String())
	lvlColor(b, tmplTo(lvlText, tb, baseTmpls["LVL"]))
	tb.Reset()

	black(b, tmplTo(t.Name, tb, baseTmpls["NAME"]))
	tb.Reset()

	timestamp := time.Now().Format(timestampFormat)
	blue(b, tmplTo(timestamp, tb, baseTmpls["TIME"]))
	tb.Reset()

	fds := FieldsSort(e.Fields())
	format(b, fds)
}

func formatTo(fds []Field) (bool, string, []interface{}) {
	var formattable bool
	var f string
	var ff []interface{}
	for _, fd := range fds {
		if fd.Key == "Format" {
			formattable = true
			f = fd.Value.(string)
		} else {
			ff = append(ff, fd.Value)
		}
	}
	return formattable, f, ff
}

func format(b *bytes.Buffer, fds FieldsSort) {
	sort.Sort(fds)
	formattable, f, ff := formatTo(fds)
	if formattable {
		fmt.Fprintf(b, f, ff...)
	} else {
		for _, v := range ff {
			fmt.Fprintf(b, "%s", v)
		}
	}
}

//
func Color(value ...Attribute) func(io.Writer, ...interface{}) {
	c := &color{params: make([]Attribute, 0)}
	c.Add(value...)
	return c.Fprint
}

type color struct {
	params []Attribute
}

//
func (c *color) Add(value ...Attribute) *color {
	c.params = append(c.params, value...)
	return c
}

//
func (c *color) Fprint(w io.Writer, a ...interface{}) {
	c.wrap(w, a...)
}

//
func (c *color) Fprintf(w io.Writer, f string, a ...interface{}) {
	c.wrap(w, fmt.Sprintf(f, a...))
}

func (c *color) sequence() string {
	format := make([]string, len(c.params))
	for i, v := range c.params {
		format[i] = strconv.Itoa(int(v))
	}

	return strings.Join(format, ";")
}

func (c *color) wrap(w io.Writer, a ...interface{}) {
	if c.noColor() {
		fmt.Fprint(w, a...)
		return
	}

	c.format(w)
	fmt.Fprint(w, a...)
	c.unformat(w)
}

func (c *color) format(w io.Writer) {
	fmt.Fprintf(w, "%s[%sm", escape, c.sequence())
}

func (c *color) unformat(w io.Writer) {
	fmt.Fprintf(w, "%s[%dm", escape, Reset)
}

var NoColor = !isatty.IsTerminal(os.Stdout.Fd())

func (c *color) noColor() bool {
	return NoColor
}

const escape = "\x1b"

//
type Attribute int

const (
	Reset Attribute = iota
	Bold
	Faint
	Italic
	Underline
	BlinkSlow
	BlinkRapid
	ReverseVideo
	Concealed
	CrossedOut
)

const (
	FgBlack Attribute = iota + 30
	FgRed
	FgGreen
	FgYellow
	FgBlue
	FgMagenta
	FgCyan
	FgWhite
)

const (
	FgHiBlack Attribute = iota + 90
	FgHiRed
	FgHiGreen
	FgHiYellow
	FgHiBlue
	FgHiMagenta
	FgHiCyan
	FgHiWhite
)

const (
	BgBlack Attribute = iota + 40
	BgRed
	BgGreen
	BgYellow
	BgBlue
	BgMagenta
	BgCyan
	BgWhite
)

const (
	BgHiBlack Attribute = iota + 100
	BgHiRed
	BgHiGreen
	BgHiYellow
	BgHiBlue
	BgHiMagenta
	BgHiCyan
	BgHiWhite
)

var (
	black   = Color(FgHiBlack)
	red     = Color(FgHiRed)
	green   = Color(FgHiGreen)
	yellow  = Color(FgHiYellow)
	blue    = Color(FgHiBlue)
	magenta = Color(FgHiMagenta)
	cyan    = Color(FgHiCyan)
	white   = Color(FgHiWhite)
)

func init() {
	initializeTemplates()
}
