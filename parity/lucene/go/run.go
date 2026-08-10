// Command run is the Go-side parity runner for github.com/malcolmston/lucene.
//
// It speaks the harness JSON-Lines protocol on stdio: one case object per line
// in, exactly one answer object per line out, in order. A panic or error is
// reported as {"ok":false} and the loop continues, so a single bad case can
// never take the run down.
//
// Usage: run <path to fixtures/corpus.json>
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/malcolmston/lucene"
)

type caseIn struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type caseOut struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

type fixture struct {
	Fields    []string            `json:"fields"`
	Docs      []map[string]string `json:"docs"`
	StopWords []string            `json:"stopWords"`
}

var (
	indexes   = map[string]*lucene.Index{}
	stopWords []string
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: run <corpus.json>")
		os.Exit(2)
	}
	if err := buildIndexes(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "fixture:", err)
		os.Exit(2)
	}

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<22)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var c caseIn
		if err := json.Unmarshal(line, &c); err != nil {
			fmt.Fprintln(os.Stderr, "bad case line:", err)
			continue
		}
		emit(out, run(c))
	}
	out.Flush()
}

func emit(w *bufio.Writer, o caseOut) {
	b, err := json.Marshal(o)
	if err != nil {
		b, _ = json.Marshal(caseOut{ID: o.ID, OK: false, Error: "marshal: " + err.Error()})
	}
	w.Write(b)
	w.WriteByte('\n')
	w.Flush()
}

// run dispatches one case, converting both returned errors and panics into
// ok:false so that the runner survives every input.
func run(c caseIn) (res caseOut) {
	res = caseOut{ID: c.ID}
	defer func() {
		if r := recover(); r != nil {
			res = caseOut{ID: c.ID, OK: false, Error: fmt.Sprintf("panic: %v", r)}
		}
	}()
	v, err := dispatch(c)
	if err != nil {
		return caseOut{ID: c.ID, OK: false, Error: err.Error()}
	}
	return caseOut{ID: c.ID, OK: true, Value: v}
}

func buildIndexes(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f fixture
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	stopWords = f.StopWords

	for name, a := range map[string]*lucene.Analyzer{
		"stop":     analyzerOf("stop"),
		"standard": analyzerOf("standard"),
	} {
		idx := lucene.NewIndex(a)
		for _, d := range f.Docs {
			fields := map[string]string{}
			for _, fn := range f.Fields {
				fields[fn] = d[fn]
			}
			if err := idx.Add(lucene.Document{ID: d["id"], Fields: fields}); err != nil {
				return err
			}
		}
		indexes[name] = idx
	}
	return nil
}

// analyzerOf maps the case files' analyzer names onto the port's configuration
// options. The names describe the upstream pipeline; this is the port's nearest
// equivalent of each.
func analyzerOf(name string) *lucene.Analyzer {
	switch name {
	case "lower":
		return lucene.NewAnalyzer(lucene.WithStemming(false))
	case "stop":
		return lucene.NewAnalyzer(lucene.WithStemming(false), lucene.WithStopWords(stopWords))
	case "porter":
		return lucene.NewAnalyzer(lucene.WithStemming(true))
	case "standard":
		return lucene.NewAnalyzer(lucene.WithStemming(true), lucene.WithStopWords(stopWords))
	case "english":
		return lucene.NewStandardAnalyzer()
	default:
		panic("unknown analyzer: " + name)
	}
}

func indexOf(name string) *lucene.Index {
	idx, ok := indexes[name]
	if !ok {
		panic("unknown index: " + name)
	}
	return idx
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	if out == nil {
		out = []string{}
	}
	return out
}
