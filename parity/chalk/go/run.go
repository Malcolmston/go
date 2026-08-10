// Command run is the Go-side parity runner for github.com/malcolmston/chalk.
//
// It speaks JSON Lines on stdio: one request object per line in, exactly one
// response object per line out, in the same order. A panic or error inside a
// case is reported as {"ok": false, "error": "..."} and the runner keeps going,
// so one bad case never costs the rest of the suite.
//
// Colour support is never auto-detected: every case names the level it wants
// and the runner pins it with chalk.SetLevel before rendering, mirroring
// upstream's `new Chalk({level: N})`.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/malcolmston/chalk"
)

type request struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type response struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// node mirrors the case-file rendering tree: either a bare JSON string, or an
// object carrying a style chain plus either nested children or a list of raw
// operands.
type node struct {
	Chain    []json.RawMessage `json:"chain"`
	Children []json.RawMessage `json:"children"`
	Args     []any             `json:"args"`

	hasArgs bool
	text    string
	isText  bool
}

func parseNode(raw json.RawMessage) (*node, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return &node{text: s, isText: true}, nil
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("node is neither string nor object: %w", err)
	}

	n := &node{}
	if err := json.Unmarshal(raw, n); err != nil {
		return nil, err
	}
	_, n.hasArgs = probe["args"]
	return n, nil
}

// step is one chain entry: a bare name ("bold") or [name, params...].
type step struct {
	name   string
	params []any
}

func parseStep(raw json.RawMessage) (step, error) {
	var name string
	if err := json.Unmarshal(raw, &name); err == nil {
		return step{name: name}, nil
	}

	var parts []any
	if err := json.Unmarshal(raw, &parts); err != nil {
		return step{}, fmt.Errorf("bad chain step %s", raw)
	}
	if len(parts) == 0 {
		return step{}, fmt.Errorf("empty chain step")
	}
	s, ok := parts[0].(string)
	if !ok {
		return step{}, fmt.Errorf("chain step name is not a string: %v", parts[0])
	}
	return step{name: s, params: parts[1:]}, nil
}

func (s step) intParam(i int) (int, error) {
	if i >= len(s.params) {
		return 0, fmt.Errorf("%s: missing parameter %d", s.name, i)
	}
	f, ok := s.params[i].(float64)
	if !ok {
		return 0, fmt.Errorf("%s: parameter %d is not a number: %v", s.name, i, s.params[i])
	}
	return int(f), nil
}

func (s step) strParam(i int) (string, error) {
	if i >= len(s.params) {
		return "", fmt.Errorf("%s: missing parameter %d", s.name, i)
	}
	v, ok := s.params[i].(string)
	if !ok {
		return "", fmt.Errorf("%s: parameter %d is not a string: %v", s.name, i, s.params[i])
	}
	return v, nil
}

// simple maps every zero-argument upstream style name to its Go method.
var simple = map[string]func(*chalk.Style) *chalk.Style{
	// modifiers
	"reset":         (*chalk.Style).Reset,
	"bold":          (*chalk.Style).Bold,
	"dim":           (*chalk.Style).Dim,
	"italic":        (*chalk.Style).Italic,
	"underline":     (*chalk.Style).Underline,
	"overline":      (*chalk.Style).Overline,
	"inverse":       (*chalk.Style).Inverse,
	"hidden":        (*chalk.Style).Hidden,
	"strikethrough": (*chalk.Style).Strikethrough,
	"visible":       (*chalk.Style).Visible,

	// foreground, basic
	"black":   (*chalk.Style).Black,
	"red":     (*chalk.Style).Red,
	"green":   (*chalk.Style).Green,
	"yellow":  (*chalk.Style).Yellow,
	"blue":    (*chalk.Style).Blue,
	"magenta": (*chalk.Style).Magenta,
	"cyan":    (*chalk.Style).Cyan,
	"white":   (*chalk.Style).White,

	// foreground, bright
	"blackBright":   (*chalk.Style).BrightBlack,
	"redBright":     (*chalk.Style).BrightRed,
	"greenBright":   (*chalk.Style).BrightGreen,
	"yellowBright":  (*chalk.Style).BrightYellow,
	"blueBright":    (*chalk.Style).BrightBlue,
	"magentaBright": (*chalk.Style).BrightMagenta,
	"cyanBright":    (*chalk.Style).BrightCyan,
	"whiteBright":   (*chalk.Style).BrightWhite,
	"gray":          (*chalk.Style).Gray,
	"grey":          (*chalk.Style).Grey,

	// background, basic
	"bgBlack":   (*chalk.Style).BgBlack,
	"bgRed":     (*chalk.Style).BgRed,
	"bgGreen":   (*chalk.Style).BgGreen,
	"bgYellow":  (*chalk.Style).BgYellow,
	"bgBlue":    (*chalk.Style).BgBlue,
	"bgMagenta": (*chalk.Style).BgMagenta,
	"bgCyan":    (*chalk.Style).BgCyan,
	"bgWhite":   (*chalk.Style).BgWhite,

	// background, bright
	"bgBlackBright":   (*chalk.Style).BgBrightBlack,
	"bgRedBright":     (*chalk.Style).BgBrightRed,
	"bgGreenBright":   (*chalk.Style).BgBrightGreen,
	"bgYellowBright":  (*chalk.Style).BgBrightYellow,
	"bgBlueBright":    (*chalk.Style).BgBrightBlue,
	"bgMagentaBright": (*chalk.Style).BgBrightMagenta,
	"bgCyanBright":    (*chalk.Style).BgBrightCyan,
	"bgWhiteBright":   (*chalk.Style).BgBrightWhite,
	"bgGray":          (*chalk.Style).BgGray,
	// NB: upstream also has bgGrey; the port has no BgGrey alias, so it is
	// deliberately left unmapped and reports "unknown style".
}

func applyStep(s *chalk.Style, st step) (*chalk.Style, error) {
	if fn, ok := simple[st.name]; ok {
		return fn(s), nil
	}

	switch st.name {
	case "level":
		n, err := st.intParam(0)
		if err != nil {
			return nil, err
		}
		return s.Level(chalk.Level(n)), nil

	case "hex", "bgHex":
		h, err := st.strParam(0)
		if err != nil {
			return nil, err
		}
		if st.name == "hex" {
			return s.Hex(h), nil
		}
		return s.BgHex(h), nil

	case "ansi256", "bgAnsi256":
		n, err := st.intParam(0)
		if err != nil {
			return nil, err
		}
		if st.name == "ansi256" {
			return s.Ansi256(n), nil
		}
		return s.BgAnsi256(n), nil

	case "rgb", "bgRgb":
		var v [3]int
		for i := range v {
			n, err := st.intParam(i)
			if err != nil {
				return nil, err
			}
			v[i] = n
		}
		if st.name == "rgb" {
			return s.RGB(v[0], v[1], v[2]), nil
		}
		return s.BgRGB(v[0], v[1], v[2]), nil
	}

	return nil, fmt.Errorf("unknown style: %s", st.name)
}

func buildStyle(chain []json.RawMessage) (*chalk.Style, error) {
	s := chalk.New()
	for _, raw := range chain {
		st, err := parseStep(raw)
		if err != nil {
			return nil, err
		}
		if s, err = applyStep(s, st); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func renderNode(raw json.RawMessage) (string, error) {
	n, err := parseNode(raw)
	if err != nil {
		return "", err
	}
	if n.isText {
		return n.text, nil
	}

	s, err := buildStyle(n.Chain)
	if err != nil {
		return "", err
	}

	if n.hasArgs {
		return s.Sprint(n.Args...), nil
	}

	var sb strings.Builder
	for _, child := range n.Children {
		out, err := renderNode(child)
		if err != nil {
			return "", err
		}
		sb.WriteString(out)
	}
	return s.Sprint(sb.String()), nil
}

func intArg(args []json.RawMessage, i int) (int, error) {
	if i >= len(args) {
		return 0, fmt.Errorf("missing argument %d", i)
	}
	var n int
	if err := json.Unmarshal(args[i], &n); err != nil {
		return 0, fmt.Errorf("argument %d is not an integer: %s", i, args[i])
	}
	return n, nil
}

func strArg(args []json.RawMessage, i int) (string, error) {
	if i >= len(args) {
		return "", fmt.Errorf("missing argument %d", i)
	}
	var s string
	if err := json.Unmarshal(args[i], &s); err != nil {
		return "", fmt.Errorf("argument %d is not a string: %s", i, args[i])
	}
	return s, nil
}

// setLevel pins the global level, the Go equivalent of `new Chalk({level})`.
func setLevel(n int) {
	chalk.SetLevel(chalk.Level(n))
}

func handle(req request) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	switch req.Fn {
	case "render":
		level, err := intArg(req.Args, 0)
		if err != nil {
			return nil, err
		}
		setLevel(level)
		if len(req.Args) < 2 {
			return nil, fmt.Errorf("render: missing node")
		}
		return renderNode(req.Args[1])

	case "level":
		level, err := intArg(req.Args, 0)
		if err != nil {
			return nil, err
		}
		setLevel(level)
		return int(chalk.GetLevel()), nil

	case "enabled":
		level, err := intArg(req.Args, 0)
		if err != nil {
			return nil, err
		}
		setLevel(level)
		return chalk.Enabled(), nil

	case "names":
		which, err := strArg(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("the Go port exports no %s list", which)

	case "template":
		return nil, fmt.Errorf("the port provides no tagged-template equivalent (see API-DEVIATIONS.md)")

	case "strip":
		s, err := strArg(req.Args, 0)
		if err != nil {
			return nil, err
		}
		return chalk.Strip(s), nil
	}

	return nil, fmt.Errorf("unknown fn: %s", req.Fn)
}

func main() {
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	out := bufio.NewWriter(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}

		var req request
		res := response{}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			res.OK = false
			res.Error = "bad request: " + err.Error()
		} else {
			res.ID = req.ID
			value, err := handle(req)
			if err != nil {
				res.OK = false
				res.Error = err.Error()
			} else {
				res.OK = true
				res.Value = value
			}
		}

		enc, err := json.Marshal(res)
		if err != nil {
			enc, _ = json.Marshal(response{ID: res.ID, OK: false, Error: "unencodable value: " + err.Error()})
		}
		out.Write(enc)
		out.WriteByte('\n')
		if err := out.Flush(); err != nil {
			fmt.Fprintln(os.Stderr, "flush:", err)
			return
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
	}
}
