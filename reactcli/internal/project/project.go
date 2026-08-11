// Package project models a react project on disk: the react.json manifest, the
// conventions around it, and the discovery walk that lets every command be run
// from anywhere inside the tree.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ManifestName is the file that marks the root of a react project. Its presence
// is the only thing that makes a directory a project — there is no global
// registry and no hidden state elsewhere.
const ManifestName = "react.json"

// ErrNotFound is returned by [Find] when no manifest exists in the current
// directory or any parent. Commands match on it to print a "run react new
// first" hint instead of a bare path error.
var ErrNotFound = errors.New("no react.json found in this directory or any parent")

// Manifest is the project configuration. It is deliberately small: everything
// in it is either something the CLI cannot infer (which page renders where) or
// something that must survive across machines (which integrations are wired
// up, and at which versions).
//
// Anything derivable from the Go toolchain — the module path, the dependency
// set, the Go version — is deliberately absent, because duplicating go.mod into
// a second file is how the two drift apart.
type Manifest struct {
	// Name is the human-readable project name. It defaults to the directory
	// name and is used only in output.
	Name string `json:"name"`

	// Entry is the Go package, relative to the project root, whose main
	// function renders the site. The CLI runs it with `go run`, so a project is
	// an ordinary Go program that the CLI orchestrates rather than a plugin
	// loaded into the CLI's own process.
	Entry string `json:"entry"`

	// Out is the directory build output is written to, relative to the root.
	Out string `json:"out"`

	// Static, when set, is a directory whose contents are copied verbatim into
	// Out before the render runs.
	Static string `json:"static,omitempty"`

	// Client is the directory holding JavaScript client components — modules
	// marked "use client". Empty means the project has no interactive half.
	Client string `json:"client,omitempty"`

	// RSCPath is the URL the browser fetches the Flight payload from, and
	// MountID the element it renders into. Both are recorded rather than
	// assumed because the generated Go and the generated browser entry must
	// agree on them, and a project may serve the payload anywhere.
	RSCPath string `json:"rscPath,omitempty"`
	MountID string `json:"mountId,omitempty"`

	// Integrations records the ecosystem tools wired into this project, keyed
	// by integration name. Committing this is what makes a clone reproducible:
	// `react install` reads it and restores the same tool versions.
	Integrations map[string]Integration `json:"integrations,omitempty"`

	// root is the absolute directory containing the manifest. It is not
	// serialized — it is where the file was found, not part of its content.
	root string
}

// Integration is one wired-up ecosystem tool.
type Integration struct {
	// Version is the exact version installed, never a range. A CLI that
	// resolves "latest" on every machine is a CLI that produces different
	// output on every machine.
	Version string `json:"version"`

	// Input and Output are the integration's source and generated artifact,
	// both relative to the project root. For Tailwind these are the CSS entry
	// and the compiled stylesheet.
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}

// Root returns the absolute path of the directory holding the manifest.
func (m *Manifest) Root() string { return m.root }

// Path resolves a manifest-relative path against the project root, so callers
// never have to remember which of the two a given field is expressed in.
func (m *Manifest) Path(rel string) string { return filepath.Join(m.root, filepath.FromSlash(rel)) }

// OutDir returns the absolute build output directory.
func (m *Manifest) OutDir() string { return m.Path(m.Out) }

// ToolsDir returns the directory where downloaded integration binaries live. It
// is inside the project rather than in a shared cache so that two projects can
// pin different versions of the same tool without fighting, and so that
// deleting the project removes everything it installed.
func (m *Manifest) ToolsDir() string { return filepath.Join(m.root, ".react", "bin") }

// Find locates the project containing dir by walking upward until a manifest
// appears, mirroring how git finds .git and npm finds package.json. Running a
// command from a subdirectory is the common case, not the exception.
func Find(dir string) (*Manifest, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	for {
		candidate := filepath.Join(abs, ManifestName)
		if _, err := os.Stat(candidate); err == nil {
			return Load(candidate)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}

		parent := filepath.Dir(abs)
		if parent == abs {
			return nil, ErrNotFound
		}
		abs = parent
	}
}

// Load reads a manifest from an explicit path and applies defaults for the
// fields a hand-edited file may have left out.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(data)))
	// Unknown fields are an error rather than a silent no-op: a typo in a
	// config key that does nothing is far more expensive to find than one that
	// fails immediately.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	m.root = dir
	m.applyDefaults()
	return &m, nil
}

// applyDefaults fills in the fields a minimal manifest may omit.
func (m *Manifest) applyDefaults() {
	if m.Name == "" {
		m.Name = filepath.Base(m.root)
	}
	if m.Entry == "" {
		m.Entry = "."
	}
	if m.Out == "" {
		m.Out = "dist"
	}
	if m.Integrations == nil {
		m.Integrations = map[string]Integration{}
	}
	if m.Client != "" {
		if m.RSCPath == "" {
			m.RSCPath = "/rsc"
		}
		if m.MountID == "" {
			m.MountID = "root"
		}
	}
}

// Save writes the manifest back, formatted the way a human would format it —
// this file is meant to be read, diffed and hand-edited, so it gets indentation
// and a trailing newline.
func (m *Manifest) Save() error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(m.root, ManifestName), data, 0o644)
}

// New builds a manifest for a project being created at root.
func New(root, name string) *Manifest {
	m := &Manifest{
		Name:         name,
		Entry:        ".",
		Out:          "dist",
		Static:       "public",
		Integrations: map[string]Integration{},
		root:         root,
	}
	m.applyDefaults()
	return m
}
