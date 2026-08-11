// Command run is the Go side of the figlet parity harness: the same JSON Lines
// protocol as node/run.mjs, answered by github.com/malcolmston/chalk/figlet.
//
// It reads one request object per line from stdin and writes exactly one
// response object per line to stdout, flushing each. A failing case is reported
// as {"ok": false, "error": "..."} — including a panic, which is recovered so a
// single bad case cannot take the runner down.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malcolmston/chalk/figlet"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

var fontDir = os.Getenv("FIGLET_FONTS")

// optionSpec is the language-agnostic option object used in the case files. The
// keys are upstream's, so one case drives both runners.
type optionSpec struct {
	HorizontalLayout *string `json:"horizontalLayout"`
	VerticalLayout   *string `json:"verticalLayout"`
	Width            *int    `json:"width"`
	WhitespaceBreak  *bool   `json:"whitespaceBreak"`
	ShowHardBlanks   *bool   `json:"showHardBlanks"`
	PrintDirection   *int    `json:"printDirection"`
}

func layoutFor(name string) (figlet.Layout, error) {
	switch name {
	case "default":
		return figlet.LayoutDefault, nil
	case "full":
		return figlet.LayoutFull, nil
	case "fitted":
		return figlet.LayoutFitted, nil
	case "controlled smushing":
		return figlet.LayoutControlledSmush, nil
	case "universal smushing":
		return figlet.LayoutUniversalSmush, nil
	default:
		// Upstream silently ignores an unrecognised layout name, so this runner
		// must too: getHorizontalFittingRules returns undefined and the font's
		// own rules stay in force.
		return figlet.LayoutDefault, nil
	}
}

func buildOptions(raw json.RawMessage) (figlet.Options, error) {
	var o figlet.Options
	if len(raw) == 0 || string(raw) == "null" {
		return o, nil
	}
	var spec optionSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return o, err
	}
	if spec.HorizontalLayout != nil {
		l, err := layoutFor(*spec.HorizontalLayout)
		if err != nil {
			return o, err
		}
		o.Layout = l
	}
	if spec.VerticalLayout != nil {
		l, err := layoutFor(*spec.VerticalLayout)
		if err != nil {
			return o, err
		}
		o.VerticalLayout = l
	}
	if spec.Width != nil {
		o.Width = *spec.Width
	}
	if spec.WhitespaceBreak != nil {
		o.WhitespaceBreak = *spec.WhitespaceBreak
	}
	if spec.ShowHardBlanks != nil {
		o.ShowHardBlanks = *spec.ShowHardBlanks
	}
	o.PrintDirection = spec.PrintDirection
	return o, nil
}

// fontCache keeps each .flf parsed once, so a hundred cases over twenty fonts
// stay fast and, more importantly, identical.
var fontCache = map[string]*figlet.Font{}

func loadNamed(name string) (*figlet.Font, error) {
	if f, ok := fontCache[name]; ok {
		return f, nil
	}
	f, err := figlet.LoadFontFile(filepath.Join(fontDir, name+".flf"))
	if err != nil {
		return nil, err
	}
	fontCache[name] = f
	return f, nil
}

// rules mirrors the fittingRules object upstream exposes on a font's options.
type rules struct {
	HLayout int  `json:"hLayout"`
	HRule1  bool `json:"hRule1"`
	HRule2  bool `json:"hRule2"`
	HRule3  bool `json:"hRule3"`
	HRule4  bool `json:"hRule4"`
	HRule5  bool `json:"hRule5"`
	HRule6  bool `json:"hRule6"`
	VLayout int  `json:"vLayout"`
	VRule1  bool `json:"vRule1"`
	VRule2  bool `json:"vRule2"`
	VRule3  bool `json:"vRule3"`
	VRule4  bool `json:"vRule4"`
	VRule5  bool `json:"vRule5"`
}

type meta struct {
	HardBlank       string `json:"hardBlank"`
	Height          int    `json:"height"`
	Baseline        int    `json:"baseline"`
	MaxLength       int    `json:"maxLength"`
	OldLayout       int    `json:"oldLayout"`
	NumCommentLines int    `json:"numCommentLines"`
	PrintDirection  int    `json:"printDirection"`
	FullLayout      int    `json:"fullLayout"`
	FittingRules    rules  `json:"fittingRules"`
}

func metaOf(f *figlet.Font) meta {
	m := f.Metadata()
	r := f.FittingRules()
	return meta{
		HardBlank:       string(m.HardBlank),
		Height:          m.Height,
		Baseline:        m.Baseline,
		MaxLength:       m.MaxLength,
		OldLayout:       m.OldLayout,
		NumCommentLines: m.NumCommentLines,
		PrintDirection:  m.PrintDirection,
		FullLayout:      m.FullLayout,
		FittingRules: rules{
			HLayout: r.HorizontalLayout,
			HRule1:  r.HorizontalRules[0],
			HRule2:  r.HorizontalRules[1],
			HRule3:  r.HorizontalRules[2],
			HRule4:  r.HorizontalRules[3],
			HRule5:  r.HorizontalRules[4],
			HRule6:  r.HorizontalRules[5],
			VLayout: r.VerticalLayout,
			VRule1:  r.VerticalRules[0],
			VRule2:  r.VerticalRules[1],
			VRule3:  r.VerticalRules[2],
			VRule4:  r.VerticalRules[3],
			VRule5:  r.VerticalRules[4],
		},
	}
}

func str(raw json.RawMessage) (string, error) {
	var s string
	err := json.Unmarshal(raw, &s)
	return s, err
}

func dispatch(req request) (any, error) {
	switch req.Fn {
	case "textSync":
		if len(req.Args) < 2 {
			return nil, fmt.Errorf("textSync wants (font, text[, options])")
		}
		name, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		text, err := str(req.Args[1])
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if len(req.Args) > 2 {
			raw = req.Args[2]
		}
		opts, err := buildOptions(raw)
		if err != nil {
			return nil, err
		}
		f, err := loadNamed(name)
		if err != nil {
			return nil, err
		}
		return f.Render(text, opts), nil

	case "readAndRender":
		if len(req.Args) < 2 {
			return nil, fmt.Errorf("readAndRender wants (file, text[, options])")
		}
		file, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		text, err := str(req.Args[1])
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if len(req.Args) > 2 {
			raw = req.Args[2]
		}
		opts, err := buildOptions(raw)
		if err != nil {
			return nil, err
		}
		f, err := figlet.LoadFontFile(filepath.Join(fontDir, file))
		if err != nil {
			return nil, err
		}
		return f.Render(text, opts), nil

	case "registryRender":
		if len(req.Args) < 2 {
			return nil, fmt.Errorf("registryRender wants (font, text)")
		}
		name, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		text, err := str(req.Args[1])
		if err != nil {
			return nil, err
		}
		return figlet.RenderFont(name, text)

	case "metadata":
		if len(req.Args) < 1 {
			return nil, fmt.Errorf("metadata wants (font)")
		}
		name, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		f, err := loadNamed(name)
		if err != nil {
			return nil, err
		}
		return metaOf(f), nil

	case "parseFont":
		if len(req.Args) < 1 {
			return nil, fmt.Errorf("parseFont wants (data)")
		}
		data, err := str(req.Args[0])
		if err != nil {
			return nil, err
		}
		f, err := figlet.ParseFont(strings.NewReader(data))
		if err != nil {
			return nil, err
		}
		return metaOf(f), nil

	case "fontsSync":
		entries, err := os.ReadDir(fontDir)
		if err != nil {
			return nil, err
		}
		var names []string
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".flf") {
				names = append(names, strings.TrimSuffix(e.Name(), ".flf"))
			}
		}
		sort.Strings(names)
		return names, nil
	}
	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

// call runs one case, turning a panic into an ordinary failed case.
func call(req request) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return dispatch(req)
}

func main() {
	if fontDir == "" {
		fmt.Fprintln(os.Stderr, "FIGLET_FONTS is not set")
		os.Exit(2)
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		value, err := call(req)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
		} else {
			_ = enc.Encode(response{ID: req.ID, OK: true, Value: value})
		}
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			os.Exit(1)
		}
	}
}
