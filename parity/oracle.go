package parity

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// An Oracle is the ORIGINAL, non-Go implementation of the library a port
// mirrors — the real chalk from npm, the real numpy from PyPI, the real
// activerecord gem. Parity cases are answered by actually running it, so the
// expectations a port is measured against are produced by upstream itself
// rather than transcribed by hand.
type Oracle interface {
	// Info describes the upstream implementation for the parity report.
	Info() OracleInfo
	// Open prepares the upstream implementation (locating the interpreter,
	// installing the package) and starts a session that can answer cases. It
	// returns an *Unavailable error when the runtime or package cannot be had
	// on this machine, which the suite treats as "fall back to goldens", not
	// as a failure.
	Open(ctx context.Context) (Session, error)
}

// A Session is a live upstream process that answers cases one at a time. One
// session serves a whole suite, so the interpreter starts (and the library
// loads) once rather than per case.
type Session interface {
	Eval(req Request) (Result, error)
	Close() error
}

// OracleInfo identifies an upstream implementation in the parity report.
type OracleInfo struct {
	Runtime string `json:"runtime"`           // "node", "python", "ruby", …
	Package string `json:"package,omitempty"` // "chalk@5.3.0"
	Version string `json:"version,omitempty"` // interpreter version, filled in on Open
}

// Request is one case handed to the upstream driver. Src is evaluated as the
// body of a function whose only argument is the case's Input, decoded into the
// runtime's native types; whatever it returns is the upstream answer.
type Request struct {
	ID    string          `json:"id"`
	Src   string          `json:"src"`
	Input json.RawMessage `json:"input,omitempty"`
}

// Result is the upstream driver's answer. OK is false when upstream raised —
// which is itself a behavior a port must reproduce, so errors are compared too.
type Result struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Unavailable reports that an upstream implementation cannot run here — no
// interpreter, no package manager, no network. It is never a test failure: the
// suite falls back to goldens recorded on a machine that did have it.
type Unavailable struct {
	Runtime string
	Reason  string
}

func (u *Unavailable) Error() string {
	return fmt.Sprintf("upstream %s unavailable: %s", u.Runtime, u.Reason)
}

func unavailable(runtime, format string, a ...any) error {
	return &Unavailable{Runtime: runtime, Reason: fmt.Sprintf(format, a...)}
}

// Runtime is the generic recipe for running an upstream implementation: an
// interpreter, a driver program that speaks the newline-delimited JSON protocol
// above, and an optional command that installs the upstream package into a
// scratch directory. Node, Python and Ruby below are pre-built Runtimes; any
// other ecosystem (Elixir, Rust, Java, a C binary) is one literal away.
type Runtime struct {
	Name string // "node" — also the parity.json runtime label
	Bin  string // interpreter binary, looked up on PATH ("node")

	// Driver is the source of the program implementing the protocol, and
	// DriverFile the name it is written as inside the work directory.
	Driver     string
	DriverFile string

	// Args builds the interpreter invocation for a prepared work directory.
	Args func(dir string) []string

	// Install prepares dir so the driver can load Package. It runs once per
	// (runtime, package) and its result is cached across test runs.
	Install func(ctx context.Context, dir, pkg string) error

	// Env adds environment variables for both Install and the driver.
	Env func(dir string) []string

	// VersionArgs prints the interpreter version ("--version" for most).
	VersionArgs []string
}

// Use binds a Runtime to a specific upstream package — "chalk@5.3.0",
// "numpy==2.0.1" — producing the Oracle a suite runs its cases against.
func (r Runtime) Use(pkg string) Oracle { return &procOracle{rt: r, pkg: pkg} }

// Node runs the real npm package in Node.js. The case source is an async
// function body with `input`, `require` and `imp` (dynamic import, for
// ESM-only packages) in scope:
//
//	Upstream: `const chalk = (await imp('chalk')).default; return chalk.bold(input)`
func Node(pkg string) Oracle { return NodeRuntime.Use(pkg) }

// Python runs the real PyPI distribution in CPython. The case source is a
// function body with `input` in scope:
//
//	Upstream: `import numpy as np; return np.linalg.det(input).item()`
func Python(pkg string) Oracle { return PythonRuntime.Use(pkg) }

// Ruby runs the real gem in CRuby. The case source is a block body with
// `input` in scope; its last expression is the answer.
func Ruby(pkg string) Oracle { return RubyRuntime.Use(pkg) }

// NodeRuntime, PythonRuntime and RubyRuntime are the built-in ecosystems.
// Each installs into a per-package cache directory and then loads the driver.
var (
	NodeRuntime = Runtime{
		Name: "node", Bin: "node",
		Driver: driverSource("node.mjs"), DriverFile: "parity_driver.mjs",
		Args:        func(dir string) []string { return []string{filepath.Join(dir, "parity_driver.mjs")} },
		VersionArgs: []string{"--version"},
		Install: func(ctx context.Context, dir, pkg string) error {
			manifest := `{"name":"parity-oracle","private":true,"version":"0.0.0","type":"module"}`
			if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifest), 0o644); err != nil {
				return err
			}
			if pkg == "" {
				return nil
			}
			return install(ctx, dir, "npm", "install", "--no-audit", "--no-fund", "--loglevel", "error", pkg)
		},
	}

	PythonRuntime = Runtime{
		Name: "python", Bin: pythonBin(),
		Driver: driverSource("python.py"), DriverFile: "parity_driver.py",
		Args:        func(dir string) []string { return []string{filepath.Join(dir, "parity_driver.py")} },
		VersionArgs: []string{"--version"},
		Env:         func(dir string) []string { return []string{"PYTHONPATH=" + filepath.Join(dir, "site")} },
		Install: func(ctx context.Context, dir, pkg string) error {
			if pkg == "" {
				return nil
			}
			return install(ctx, dir, pythonBin(), "-m", "pip", "install", "--disable-pip-version-check",
				"--quiet", "--target", filepath.Join(dir, "site"), pkg)
		},
	}

	RubyRuntime = Runtime{
		Name: "ruby", Bin: "ruby",
		Driver: driverSource("ruby.rb"), DriverFile: "parity_driver.rb",
		Args:        func(dir string) []string { return []string{filepath.Join(dir, "parity_driver.rb")} },
		VersionArgs: []string{"--version"},
		Env:         func(dir string) []string { return []string{"GEM_HOME=" + filepath.Join(dir, "gems")} },
		Install: func(ctx context.Context, dir, pkg string) error {
			if pkg == "" {
				return nil
			}
			name, version, _ := strings.Cut(pkg, "@")
			args := []string{"install", "--no-document", "--install-dir", filepath.Join(dir, "gems"), name}
			if version != "" {
				args = append(args, "-v", version)
			}
			return install(ctx, dir, "gem", args...)
		},
	}
)

// procOracle is an Oracle backed by a Runtime and one upstream package.
type procOracle struct {
	rt      Runtime
	pkg     string
	version string
}

func (o *procOracle) Info() OracleInfo {
	return OracleInfo{Runtime: o.rt.Name, Package: o.pkg, Version: o.version}
}

func (o *procOracle) Open(ctx context.Context) (Session, error) {
	bin, err := exec.LookPath(o.rt.Bin)
	if err != nil {
		return nil, unavailable(o.rt.Name, "%s is not on PATH", o.rt.Bin)
	}
	o.version = probeVersion(ctx, bin, o.rt.VersionArgs)

	dir, err := o.workDir(ctx)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, o.rt.DriverFile), []byte(o.rt.Driver), 0o644); err != nil {
		return nil, unavailable(o.rt.Name, "writing driver: %v", err)
	}

	cmd := exec.CommandContext(ctx, bin, o.rt.Args(dir)...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if o.rt.Env != nil {
		cmd.Env = append(cmd.Env, o.rt.Env(dir)...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, unavailable(o.rt.Name, "starting driver: %v", err)
	}

	s := &procSession{
		runtime: o.rt.Name,
		cmd:     cmd,
		in:      stdin,
		out:     bufio.NewReaderSize(stdout, 1<<20),
		stderr:  &stderr,
	}
	// Handshake: a driver that cannot even echo a constant is not usable, and
	// saying so here (rather than failing every case) keeps the fallback clean.
	if res, err := s.Eval(Request{ID: handshakeID, Src: driverHandshake(o.rt.Name)}); err != nil || !res.OK {
		s.Close()
		reason := stderr.String()
		if reason == "" && err != nil {
			reason = err.Error()
		}
		return nil, unavailable(o.rt.Name, "driver handshake failed: %s", firstLine(reason))
	}
	return s, nil
}

// workDir returns the prepared, package-installed directory for this oracle,
// installing on first use and reusing the install on every run afterwards.
func (o *procOracle) workDir(ctx context.Context) (string, error) {
	if dir := os.Getenv("PARITY_" + strings.ToUpper(o.rt.Name) + "_DIR"); dir != "" {
		return dir, nil // a preinstalled tree, e.g. one baked into a CI image
	}
	root, err := os.UserCacheDir()
	if err != nil {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "malcolmston-parity", o.rt.Name, slug(o.pkg))
	stamp := filepath.Join(dir, ".installed")
	if _, err := os.Stat(stamp); err == nil {
		return dir, nil
	}
	if o.rt.Install == nil {
		return dir, os.MkdirAll(dir, 0o755)
	}
	if offline() {
		return "", unavailable(o.rt.Name, "%s is not installed and installs are disabled (PARITY_OFFLINE)", o.pkg)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ictx, cancel := context.WithTimeout(ctx, installTimeout())
	defer cancel()
	if err := o.rt.Install(ictx, dir, o.pkg); err != nil {
		return "", unavailable(o.rt.Name, "installing %s: %v", o.pkg, err)
	}
	if err := os.WriteFile(stamp, []byte(o.pkg), 0o644); err != nil {
		return "", err
	}
	return dir, nil
}

// procSession talks the newline-delimited JSON protocol to a driver process.
type procSession struct {
	mu      sync.Mutex
	runtime string
	cmd     *exec.Cmd
	in      io.WriteCloser
	out     *bufio.Reader
	stderr  *strings.Builder
	dead    error
}

func (s *procSession) Eval(req Request) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead != nil {
		return Result{}, s.dead
	}
	line, err := json.Marshal(req)
	if err != nil {
		return Result{}, err
	}
	if _, err := s.in.Write(append(line, '\n')); err != nil {
		return Result{}, s.die(err)
	}
	raw, err := s.out.ReadBytes('\n')
	if err != nil {
		return Result{}, s.die(fmt.Errorf("%w (stderr: %s)", err, firstLine(s.stderr.String())))
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{}, s.die(fmt.Errorf("undecodable driver reply %q: %w", firstLine(string(raw)), err))
	}
	return res, nil
}

// die marks the session unusable so one crashed driver does not turn into one
// confusing failure per remaining case.
func (s *procSession) die(err error) error {
	s.dead = fmt.Errorf("%s driver died: %w", s.runtime, err)
	return s.dead
}

func (s *procSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.in != nil {
		s.in.Close()
	}
	if s.cmd.Process == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() { done <- s.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.cmd.Process.Kill()
		<-done
	}
	return nil
}

const handshakeID = "__parity_handshake__"

// driverHandshake is the trivial "return a constant" program in each runtime.
func driverHandshake(runtime string) string {
	switch runtime {
	case "ruby":
		return `"ok"`
	default:
		return `return "ok"`
	}
}

func install(ctx context.Context, dir, bin string, args ...string) error {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s is not on PATH", bin)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var out strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w: %s", bin, strings.Join(args, " "), err, firstLine(out.String()))
	}
	return nil
}

func probeVersion(ctx context.Context, bin string, args []string) string {
	if len(args) == 0 {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, args...).Output()
	if err != nil {
		return ""
	}
	return firstLine(string(out))
}

// pythonBin prefers python3 but accepts a bare python (Windows, conda).
func pythonBin() string {
	if _, err := exec.LookPath("python3"); err == nil {
		return "python3"
	}
	return "python"
}

func offline() bool {
	v := os.Getenv("PARITY_OFFLINE")
	return v != "" && v != "0" && v != "false"
}

func installTimeout() time.Duration {
	if v := os.Getenv("PARITY_INSTALL_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 10 * time.Minute
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// slug makes a package spec safe as a directory name.
func slug(s string) string {
	if s == "" {
		return "_none"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

var errNoOracle = errors.New("no upstream oracle configured")
