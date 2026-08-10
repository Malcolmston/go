// Command fastmcp-example exercises the github.com/malcolmston/fastmcp MCP
// framework end to end, in-process: it builds a server with tools (plain,
// dynamic, structured-output, progress-reporting, sampling and roots), static /
// templated / binary resources, a prompt with completion, then drives it with
// the real MCP client over the in-memory transport and over Streamable HTTP.
//
// The program always terminates on its own; nothing blocks on stdin.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/malcolmston/fastmcp"
	"github.com/malcolmston/fastmcp/client"
	"github.com/malcolmston/fastmcp/contrib"
	"github.com/malcolmston/fastmcp/transport"
)

// ---------------------------------------------------------------------------
// Tool argument / result types. Exported fields drive the reflected JSON schema.
// ---------------------------------------------------------------------------

// AddArgs are the arguments of the "add" tool.
type AddArgs struct {
	A float64 `json:"a" jsonschema:"description=the first addend"`
	B float64 `json:"b" jsonschema:"description=the second addend"`
}

// StatsArgs are the arguments of the "stats" structured-output tool.
type StatsArgs struct {
	Values []float64 `json:"values" jsonschema:"description=samples to summarize"`
	Label  *string    `json:"label,omitempty" jsonschema:"description=optional label"`
}

// Stats is the structured result of the "stats" tool; its shape becomes the
// tool's advertised outputSchema.
type Stats struct {
	Label string  `json:"label"`
	Count int     `json:"count"`
	Sum   float64 `json:"sum"`
	Mean  float64 `json:"mean"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// CountdownArgs are the arguments of the progress-reporting "countdown" tool.
type CountdownArgs struct {
	Steps int `json:"steps" jsonschema:"description=number of progress steps to emit"`
}

// notes is the in-memory data behind the templated resource.
var notes = map[string]string{
	"alpha": "the first note",
	"beta":  "the second note",
	"gamma": "the third note",
}

// header prints a labelled section divider.
func header(s string) {
	fmt.Printf("\n=== %s ===\n", s)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "example failed:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srv := buildServer()

	if err := driveInMemory(ctx, srv); err != nil {
		return err
	}
	if err := driveHTTP(ctx, srv); err != nil {
		return err
	}
	header("done")
	fmt.Println("all sections completed")
	return nil
}

// buildServer registers every capability kind the framework offers.
func buildServer() *fastmcp.Server {
	s := fastmcp.New("demo-lab",
		fastmcp.WithVersion("1.2.3"),
		fastmcp.WithInstructions("A demo server exercising tools, resources and prompts."),
	)

	// 1. Struct-argument tool: schema reflected from AddArgs.
	s.Tool("add", "Add two numbers", func(ctx context.Context, a AddArgs) (any, error) {
		return a.A + a.B, nil
	})

	// 2. Dynamic tool: raw map[string]any arguments, no reflected schema.
	s.Tool("echo", "Echo the argument map back as sorted key=value pairs",
		func(ctx context.Context, args map[string]any) (any, error) {
			keys := make([]string, 0, len(args))
			for k := range args {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := make([]string, 0, len(keys))
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s=%v", k, args[k]))
			}
			return strings.Join(parts, " "), nil
		})

	// 3. Structured output: outputSchema reflected from Stats.
	s.ToolWithOutput("stats", "Summarize a list of numbers",
		func(ctx context.Context, a StatsArgs) (Stats, error) {
			if len(a.Values) == 0 {
				return Stats{}, errors.New("values must not be empty")
			}
			out := Stats{
				Label: "unlabelled",
				Count: len(a.Values),
				Min:   a.Values[0],
				Max:   a.Values[0],
			}
			if a.Label != nil {
				out.Label = *a.Label
			}
			for _, v := range a.Values {
				out.Sum += v
				if v < out.Min {
					out.Min = v
				}
				if v > out.Max {
					out.Max = v
				}
			}
			out.Mean = out.Sum / float64(out.Count)
			return out, nil
		})

	// 4. Progress + logging through the per-request Context.
	s.Tool("countdown", "Emit progress notifications while counting down",
		func(ctx context.Context, a CountdownArgs) (any, error) {
			c := fastmcp.FromContext(ctx)
			if c == nil {
				return nil, errors.New("no fastmcp context")
			}
			if a.Steps <= 0 {
				a.Steps = 3
			}
			_ = c.Info(fmt.Sprintf("countdown starting with %d steps", a.Steps))
			for i := 1; i <= a.Steps; i++ {
				if err := c.Progress(float64(i), float64(a.Steps), fmt.Sprintf("step %d", i)); err != nil {
					return nil, err
				}
			}
			_ = c.Debug("countdown finished")
			return fmt.Sprintf("emitted %d progress notifications", a.Steps), nil
		})

	// 5. Server-to-client request: sampling.
	s.Tool("summarize", "Ask the connected client's model to summarize text",
		func(ctx context.Context, args map[string]any) (any, error) {
			c := fastmcp.FromContext(ctx)
			text, _ := args["text"].(string)
			res, err := c.CreateMessage(fastmcp.CreateMessageParams{
				Messages: []fastmcp.SamplingMessage{{
					Role:    "user",
					Content: fastmcp.NewTextContent("Summarize: " + text),
				}},
				SystemPrompt: "You are terse.",
				MaxTokens:    64,
			})
			if err != nil {
				return nil, err
			}
			return fmt.Sprintf("model=%s stop=%s text=%q", res.Model, res.StopReason, res.Content.Text), nil
		})

	// 6. Server-to-client request: roots.
	s.Tool("roots", "List the roots exposed by the connected client",
		func(ctx context.Context, _ map[string]any) (any, error) {
			roots, err := fastmcp.FromContext(ctx).ListRoots()
			if err != nil {
				return nil, err
			}
			names := make([]string, 0, len(roots))
			for _, r := range roots {
				names = append(names, fmt.Sprintf("%s(%s)", r.Name, r.URI))
			}
			return strings.Join(names, ", "), nil
		})

	// 7. A deliberately failing tool: handler errors surface as isError results.
	s.Tool("boom", "Always fails", func(ctx context.Context, _ map[string]any) (any, error) {
		return nil, errors.New("intentional failure")
	})

	// Resources: static text, templated text, static binary.
	s.Resource("config://app", "app-config", "The application configuration", "application/json",
		func(ctx context.Context) (string, error) {
			return `{"theme":"dark","retries":3}`, nil
		})

	s.ResourceTemplate("notes://{id}", "note", "A note by id", "text/plain",
		func(ctx context.Context, params map[string]string) (string, error) {
			body, ok := notes[params["id"]]
			if !ok {
				return "", fmt.Errorf("no note %q", params["id"])
			}
			return body, nil
		})

	s.BinaryResource("blob://logo", "logo", "A tiny binary blob", "application/octet-stream",
		func(ctx context.Context) ([]byte, error) {
			return []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a}, nil
		})

	// Prompt with declared arguments.
	s.Prompt("greet", "Greet somebody in a given tone",
		func(ctx context.Context, args map[string]string) ([]fastmcp.PromptMessage, error) {
			name := args["name"]
			if name == "" {
				name = "world"
			}
			tone := args["tone"]
			if tone == "" {
				tone = "friendly"
			}
			return []fastmcp.PromptMessage{
				fastmcp.NewAssistantMessage("I will be " + tone + "."),
				fastmcp.NewUserMessage("Say hello to " + name + "."),
			}, nil
		},
		fastmcp.PromptArgument{Name: "name", Description: "who to greet", Required: true},
		fastmcp.PromptArgument{Name: "tone", Description: "tone of voice"},
	)

	// Completion for a prompt argument and for a resource-template variable.
	s.CompletePrompt("greet", func(ctx context.Context, argument, value string) []string {
		if argument != "tone" {
			return nil
		}
		var out []string
		for _, t := range []string{"friendly", "formal", "funny"} {
			if strings.HasPrefix(t, value) {
				out = append(out, t)
			}
		}
		return out
	})

	s.CompleteResourceTemplate("notes://{id}", func(ctx context.Context, argument, value string) []string {
		var out []string
		for id := range notes {
			if strings.HasPrefix(id, value) {
				out = append(out, id)
			}
		}
		sort.Strings(out)
		return out
	})

	return s
}

// notifyRecorder collects notifications delivered to the client.
type notifyRecorder struct {
	mu   sync.Mutex
	seen []string
	ping chan struct{}
}

func newRecorder() *notifyRecorder {
	return &notifyRecorder{ping: make(chan struct{}, 64)}
}

func (n *notifyRecorder) handle(method string, params json.RawMessage) {
	n.mu.Lock()
	n.seen = append(n.seen, fmt.Sprintf("%s %s", method, compact(params)))
	n.mu.Unlock()
	select {
	case n.ping <- struct{}{}:
	default:
	}
}

func (n *notifyRecorder) list() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]string, len(n.seen))
	copy(out, n.seen)
	return out
}

// waitFor blocks until a notification whose text contains substr has been seen,
// or the deadline elapses.
func (n *notifyRecorder) waitFor(substr string, d time.Duration) bool {
	deadline := time.After(d)
	for {
		for _, s := range n.list() {
			if strings.Contains(s, substr) {
				return true
			}
		}
		select {
		case <-n.ping:
		case <-deadline:
			return false
		}
	}
}

func compact(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// driveInMemory runs the full request suite over the in-process transport, which
// is bidirectional and therefore supports sampling, roots and notifications.
func driveInMemory(ctx context.Context, srv *fastmcp.Server) error {
	rec := newRecorder()

	tr := transport.InMemory(srv,
		transport.WithContext(ctx),
		transport.WithClientOptions(
			client.WithClientInfo("demo-client", "0.1.0"),
			client.WithNotificationHandler(rec.handle),
			client.WithRoots([]fastmcp.Root{
				{URI: "file:///workspace", Name: "workspace"},
				{URI: "file:///tmp", Name: "tmp"},
			}),
			client.WithSamplingHandler(func(ctx context.Context, params json.RawMessage) (any, error) {
				var p fastmcp.CreateMessageParams
				if err := json.Unmarshal(params, &p); err != nil {
					return nil, err
				}
				prompt := ""
				if len(p.Messages) > 0 {
					prompt = p.Messages[0].Content.Text
				}
				return fastmcp.CreateMessageResult{
					Role:       "assistant",
					Content:    fastmcp.NewTextContent("[fake-llm] " + prompt),
					Model:      "stub-model-1",
					StopReason: "endTurn",
				}, nil
			}),
		),
	)

	c := tr.Client()
	defer func() { _ = c.Close() }()

	header("initialize (in-memory transport)")
	init, err := c.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	fmt.Printf("server:       %s v%s\n", init.ServerInfo.Name, init.ServerInfo.Version)
	fmt.Printf("protocol:     %s\n", init.ProtocolVersion)
	fmt.Printf("instructions: %s\n", init.Instructions)
	fmt.Printf("capabilities: %s\n", compact(init.Capabilities))

	if err := c.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	fmt.Println("ping:         ok")

	header("tools/list")
	tools, err := c.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("tools/list: %w", err)
	}
	for _, t := range tools {
		fmt.Printf("- %-10s %s\n            input=%s\n", t.Name, t.Description, compact(t.InputSchema))
		if len(t.OutputSchema) > 0 {
			fmt.Printf("            output=%s\n", compact(t.OutputSchema))
		}
	}

	header("tools/call")
	if err := showCall(ctx, c, "add", map[string]any{"a": 2.5, "b": 4}); err != nil {
		return err
	}
	if err := showCall(ctx, c, "echo", map[string]any{"z": 1, "a": "hi", "m": true}); err != nil {
		return err
	}
	if err := showCall(ctx, c, "stats", map[string]any{"values": []float64{3, 1, 4, 1, 5}, "label": "digits"}); err != nil {
		return err
	}
	if err := showCall(ctx, c, "summarize", map[string]any{"text": "a long document"}); err != nil {
		return err
	}
	if err := showCall(ctx, c, "roots", map[string]any{}); err != nil {
		return err
	}
	if err := showCall(ctx, c, "boom", map[string]any{}); err != nil {
		return err
	}

	header("log + progress notifications")
	// HOLE: v0.4.0 advertises the "logging" capability in its initialize result
	// but implements no logging/setLevel dispatch case, and client.Client has no
	// SetLoggingLevel method, so a client cannot narrow the severity it receives.
	// The intended call would have been:
	//   if err := c.SetLoggingLevel(ctx, "info"); err != nil { ... }
	// Consequently Context.Log/Debug/Info always deliver every record.
	if err := showCall(ctx, c, "countdown", map[string]any{"steps": 3}); err != nil {
		return err
	}
	if !rec.waitFor("notifications/progress", 3*time.Second) {
		return errors.New("no progress notification arrived")
	}

	header("resources")
	resources, err := c.ListResources(ctx)
	if err != nil {
		return fmt.Errorf("resources/list: %w", err)
	}
	for _, r := range resources {
		fmt.Printf("- %-13s %-11s %s\n", r.URI, r.MIMEType, r.Description)
	}
	fmt.Println("(templated resources are absent from resources/list, per spec)")
	// HOLE: the server implements resources/templates/list, but client.Client in
	// v0.4.0 exposes no ListResourceTemplates method (and no generic Call escape
	// hatch, since Client.call is unexported), so a client written against this
	// package cannot discover resource templates at all. Intended call:
	//   templates, err := c.ListResourceTemplates(ctx)

	for _, uri := range []string{"config://app", "notes://beta", "blob://logo"} {
		res, err := c.ReadResource(ctx, uri)
		if err != nil {
			return fmt.Errorf("read %s: %w", uri, err)
		}
		for _, cc := range res.Contents {
			switch {
			case cc.Blob != "":
				raw, err := base64.StdEncoding.DecodeString(cc.Blob)
				if err != nil {
					return fmt.Errorf("blob decode: %w", err)
				}
				fmt.Printf("read %-13s blob %d bytes % x\n", cc.URI, len(raw), raw)
			default:
				fmt.Printf("read %-13s text %q\n", cc.URI, cc.Text)
			}
		}
	}
	if _, err := c.ReadResource(ctx, "notes://missing"); err != nil {
		fmt.Printf("read notes://missing -> expected error: %v\n", err)
	} else {
		return errors.New("expected an error reading an unknown note")
	}

	header("subscriptions + list-changed broadcasts")
	if err := c.Subscribe(ctx, "config://app"); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	srv.NotifyResourceUpdated("config://app")
	srv.NotifyToolsChanged()
	srv.NotifyPromptsChanged()
	srv.NotifyResourcesChanged()
	if !rec.waitFor("notifications/resources/updated", 3*time.Second) {
		return errors.New("no resources/updated notification arrived")
	}
	if !rec.waitFor("notifications/tools/list_changed", 3*time.Second) {
		return errors.New("no tools/list_changed notification arrived")
	}
	if err := c.Unsubscribe(ctx, "config://app"); err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	fmt.Println("subscribed, received updates, unsubscribed")

	header("prompts")
	prompts, err := c.ListPrompts(ctx)
	if err != nil {
		return fmt.Errorf("prompts/list: %w", err)
	}
	for _, p := range prompts {
		fmt.Printf("- %s: %s args=%v\n", p.Name, p.Description, p.Arguments)
	}
	got, err := c.GetPrompt(ctx, "greet", map[string]string{"name": "Ada", "tone": "formal"})
	if err != nil {
		return fmt.Errorf("prompts/get: %w", err)
	}
	for _, m := range got.Messages {
		fmt.Printf("  %-9s %s\n", m.Role+":", m.Content.Text)
	}

	header("completion/complete")
	pc, err := c.CompletePrompt(ctx, "greet", "tone", "f")
	if err != nil {
		return fmt.Errorf("complete prompt: %w", err)
	}
	fmt.Printf("prompt tone   'f' -> %v (total=%d hasMore=%v)\n", pc.Values, pc.Total, pc.HasMore)
	rc, err := c.CompleteResource(ctx, "notes://{id}", "id", "b")
	if err != nil {
		return fmt.Errorf("complete resource: %w", err)
	}
	fmt.Printf("resource id   'b' -> %v\n", rc.Values)

	header("contrib bulk tool calls")
	results, err := contrib.CallToolsBulk(ctx, c, []contrib.ToolCall{
		{Tool: "add", Arguments: map[string]any{"a": 1, "b": 2}},
		{Tool: "add", Arguments: map[string]any{"a": 10, "b": 20}},
		{Tool: "boom"},
	}, contrib.BulkOptions{Parallel: true, MaxConcurrency: 2, ContinueOnError: true})
	if err == nil {
		return errors.New("expected the bulk batch to report the failing call")
	}
	for _, r := range results {
		fmt.Printf("- %-6s failed=%-5v text=%q\n", r.Tool, r.Failed(), r.Text())
	}
	fmt.Printf("aggregate error: %v\n", err)

	timed, dur, err := contrib.Timed(ctx, c, "add", map[string]any{"a": 7, "b": 8})
	if err != nil {
		return fmt.Errorf("timed call: %w", err)
	}
	fmt.Printf("contrib.Timed add(7,8) = %s in %v\n", timed.Content[0].Text, dur > 0)

	header("notifications received")
	for _, n := range rec.list() {
		fmt.Println("-", n)
	}

	return nil
}

// driveHTTP exercises the Streamable HTTP transport, including a JSON-RPC batch.
// Sampling and roots are unavailable there (no server-to-client channel).
func driveHTTP(ctx context.Context, srv *fastmcp.Server) error {
	header("Streamable HTTP transport")
	ts := httptest.NewServer(srv.HTTPHandler())
	defer ts.Close()

	c := client.NewHTTP(ts.URL, client.WithClientInfo("http-client", "0.1.0"))
	defer func() { _ = c.Close() }()

	init, err := c.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("http initialize: %w", err)
	}
	fmt.Printf("connected to %s v%s over HTTP\n", init.ServerInfo.Name, init.ServerInfo.Version)

	if err := showCall(ctx, c, "add", map[string]any{"a": 40, "b": 2}); err != nil {
		return err
	}

	// Sampling needs a bidirectional transport; over HTTP it must fail.
	res, err := c.CallTool(ctx, "summarize", map[string]any{"text": "x"})
	switch {
	case err != nil:
		fmt.Printf("summarize over HTTP -> protocol error (expected): %v\n", err)
	case res.IsError:
		fmt.Printf("summarize over HTTP -> isError (expected): %s\n", res.Content[0].Text)
	default:
		return errors.New("expected sampling to fail over HTTP")
	}
	return nil
}

// showCall invokes a tool and prints its content, structured payload and error
// flag.
func showCall(ctx context.Context, c *client.Client, name string, args map[string]any) error {
	res, err := c.CallTool(ctx, name, args)
	if err != nil {
		return fmt.Errorf("call %s: %w", name, err)
	}
	texts := make([]string, 0, len(res.Content))
	for _, cc := range res.Content {
		texts = append(texts, cc.Text)
	}
	fmt.Printf("%-10s isError=%-5v content=%q", name, res.IsError, strings.Join(texts, "|"))
	if len(res.StructuredContent) > 0 {
		fmt.Printf(" structured=%s", compact(res.StructuredContent))
	}
	fmt.Println()
	return nil
}
