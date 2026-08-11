package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/malcolmston/reactcli/internal/project"
	"github.com/malcolmston/reactcli/internal/ui"
)

func newDevCommand() *cobra.Command {
	var (
		port   int
		host   string
		reload bool
	)

	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Rebuild and serve the project on every save",
		Long: `Rebuild and serve the project on every save.

Watches the project for changes, re-runs the build, and serves the output. Open
pages reload themselves when a build finishes, using a server-sent events stream
injected into each HTML response — nothing is added to your source, and the
build output on disk stays exactly what react build would produce.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := requireProject()
			if err != nil {
				return err
			}
			return runDev(cmd.Context(), m, host, port, reload)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 3000, "port to serve on")
	cmd.Flags().StringVar(&host, "host", "localhost", "interface to bind")
	cmd.Flags().BoolVar(&reload, "reload", true, "reload open pages when a build finishes")
	return cmd
}

func runDev(ctx context.Context, m *project.Manifest, host string, port int, reload bool) error {
	printer.Banner(fmt.Sprintf("developing %s", ui.Accent(m.Name)))

	hub := newReloadHub()

	// Build once before serving so the first request gets a real page rather
	// than a 404 that looks like a broken setup.
	if err := runBuild(ctx, m); err != nil {
		// A failing first build is not fatal in dev: the whole point of the
		// server is to watch until it passes.
		printer.Error("%v", err)
	}

	addr := net.JoinHostPort(host, fmt.Sprint(port))
	server := &http.Server{
		Addr:    addr,
		Handler: devHandler(m, hub, reload),
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", addr, err)
	}

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			printer.Error("server: %v", err)
		}
	}()

	printer.Blank()
	printer.Success("serving %s", ui.Accent(fmt.Sprintf("http://%s/", addr)))
	printer.Note("watching for changes — ctrl-c to stop")
	printer.Blank()

	watchErr := watch(ctx, m, func() {
		started := time.Now()
		if err := runBuild(ctx, m); err != nil {
			printer.Error("%v", err)
			hub.broadcast("error")
			return
		}
		printer.Success("rebuilt in %s", time.Since(started).Round(time.Millisecond))
		hub.broadcast("reload")
	})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	return watchErr
}

// devHandler serves the build output, injecting the live-reload client into
// HTML responses.
func devHandler(m *project.Manifest, hub *reloadHub, reload bool) http.Handler {
	mux := http.NewServeMux()

	if reload {
		mux.HandleFunc("/__react/reload", hub.serve)
	}

	files := http.FileServer(http.Dir(m.OutDir()))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(m.OutDir(), filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, "index.html")
		}

		// Only HTML is rewritten, and only in memory. Everything else — CSS,
		// images, fonts — is served straight off disk by the standard file
		// server, which handles ranges and conditional requests properly.
		if reload && strings.HasSuffix(path, ".html") {
			body, err := os.ReadFile(path)
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				_, _ = w.Write(injectReloadClient(body))
				return
			}
		}

		w.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(w, r)
	})

	return mux
}

// reloadClient is appended to served HTML. It is deliberately tiny and
// dependency-free: an EventSource that reloads on a "reload" message and simply
// keeps waiting on an "error" one, so a failed build leaves the last good page
// on screen instead of replacing it with a blank document.
const reloadClient = `<script>
(function () {
  var es = new EventSource("/__react/reload");
  es.addEventListener("reload", function () { location.reload(); });
  es.addEventListener("error", function () { /* build failed; keep the page */ });
})();
</script>`

// injectReloadClient inserts the client before </body>, falling back to
// appending when there is no body tag — a fragment is still worth serving.
func injectReloadClient(html []byte) []byte {
	s := string(html)
	if i := strings.LastIndex(strings.ToLower(s), "</body>"); i >= 0 {
		return []byte(s[:i] + reloadClient + s[i:])
	}
	return []byte(s + reloadClient)
}

// reloadHub tracks connected browsers.
type reloadHub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newReloadHub() *reloadHub {
	return &reloadHub{clients: map[chan string]struct{}{}}
}

// serve holds a server-sent events connection open for one browser.
func (h *reloadHub) serve(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")

	// Buffered so a broadcast never blocks on a browser that has stopped
	// reading — a tab in the background must not be able to stall a rebuild.
	ch := make(chan string, 4)

	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		close(ch)
		h.mu.Unlock()
	}()

	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: {}\n\n", event)
			flusher.Flush()
		}
	}
}

// broadcast sends an event to every connected browser, skipping any whose
// buffer is full rather than waiting for it.
func (h *reloadHub) broadcast(event string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- event:
		default:
		}
	}
}

// watch calls onChange whenever a source file changes, and returns when ctx is
// cancelled.
//
// Changes are debounced. A single save from an editor commonly produces several
// filesystem events — a write, a chmod, an atomic rename — and formatting on
// save can touch a whole package at once. Rebuilding on each would mean several
// redundant builds per keystroke-to-save cycle.
func watch(ctx context.Context, m *project.Manifest, onChange func()) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	if err := addWatchDirs(w, m); err != nil {
		return err
	}

	const debounce = 120 * time.Millisecond
	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			if !isSourceChange(m, event) {
				continue
			}
			// A newly created directory has to be watched too, or files added
			// to it after startup are invisible.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.Add(event.Name)
				}
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.NewTimer(debounce)
			timerC = timer.C

		case <-timerC:
			timerC = nil
			onChange()

		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			printer.Warn("watch: %v", err)
		}
	}
}

// addWatchDirs registers the project's source directories with the watcher.
func addWatchDirs(w *fsnotify.Watcher, m *project.Manifest) error {
	return filepath.WalkDir(m.Root(), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		switch d.Name() {
		case ".git", ".react", "node_modules", "vendor":
			return filepath.SkipDir
		}
		// Watching the output directory would make every build trigger another
		// build. This is the loop that eats a laptop battery.
		if path == m.OutDir() {
			return filepath.SkipDir
		}
		return w.Add(path)
	})
}

// isSourceChange filters out the events that should not trigger a rebuild.
func isSourceChange(m *project.Manifest, event fsnotify.Event) bool {
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	path := event.Name
	if strings.HasPrefix(path, m.OutDir()) {
		return false
	}

	base := filepath.Base(path)
	// Editor scratch files: vim's 4913, Emacs's #file#, and the ~ and .swp
	// families. Rebuilding on these produces builds nobody asked for.
	if strings.HasSuffix(base, "~") || strings.HasPrefix(base, ".#") ||
		strings.HasPrefix(base, "#") || strings.HasSuffix(base, ".swp") ||
		strings.HasSuffix(base, ".swx") || strings.HasPrefix(base, ".goutputstream") {
		return false
	}

	switch filepath.Ext(base) {
	case ".go", ".css", ".html", ".json", ".js", ".svg", ".png", ".jpg", ".jpeg",
		".webp", ".avif", ".ico", ".woff", ".woff2", ".ttf", ".md", ".txt":
		return true
	}
	return false
}
