package main

import (
	"github.com/malcolmston/streamlit/st"
)

// appCtx is the side channel through which a Go app publishes what it observed.
//
// Streamlit's oracle reads the app's own session-state keys straight out of
// st.session_state; the Go port's published API (v0.3.0) has no way to
// enumerate a [st.State] — no Keys, Len or iteration — so each app names the
// keys it publishes explicitly. See COVERAGE.md.
type appCtx struct {
	state map[string]any
	// computed counts real (uncached) computations for the caching apps. It
	// lives on the context rather than in a package variable so each case is
	// independent.
	computed int
}

func newAppCtx() *appCtx { return &appCtx{state: map[string]any{}} }

// put stores a value in the session's state and publishes it to the runner.
func (a *appCtx) put(s *st.Session, key string, v any) {
	s.State.Set(key, v)
	a.state[key] = v
}

// pub publishes a value to the runner without touching session state. It is
// used for values the app merely observed rather than persisted.
func (a *appCtx) pub(key string, v any) { a.state[key] = v }

// widgetSnippets holds one widget call per entry, mirroring
// python/apps.py:WIDGET_SNIPPETS. Composing them keeps the single-widget apps
// and the combined app in step on both sides.
var widgetSnippets = map[string]func(*appCtx, *st.Session){
	"button": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.Button("Go", "w_button"))
	},
	"checkbox": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.Checkbox("Agree", true, "w_checkbox"))
	},
	"radio": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.Radio("Pick", []string{"alpha", "beta", "gamma"}, "w_radio"))
	},
	"selectbox": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.SelectBox("Choose", []string{"alpha", "beta", "gamma"}, "w_selectbox"))
	},
	"multiselect": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", sortedStrings(s.MultiSelect("Many", []string{"alpha", "beta", "gamma"}, "w_multiselect")))
	},
	"slider": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.Slider("Amount", 0, 10, 3, 1, "w_slider"))
	},
	"text_input": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.TextInput("Name", "ada", "w_text_input"))
	},
	"text_area": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.TextArea("Bio", "hello", "w_text_area"))
	},
	"number_input": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.NumberInput("Count", 5, "w_number_input"))
	},
	"date_input": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.DateInput("Day", "2020-01-02", "w_date_input"))
	},
	"time_input": func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.TimeInput("Moment", "09:30", "w_time_input"))
	},
	"file_uploader": func(a *appCtx, s *st.Session) {
		files := s.FileUploader("Upload", "w_file_uploader")
		names := make([]string, 0, len(files))
		for _, f := range files {
			names = append(names, f.Name)
		}
		a.put(s, "ss_v", names)
	},
}

// widgetOrder fixes the order the combined widgets-defaults app draws its
// widgets in, matching the insertion order of python/apps.py:WIDGET_SNIPPETS.
var widgetOrder = []string{
	"button", "checkbox", "radio", "selectbox", "multiselect", "slider",
	"text_input", "text_area", "number_input", "date_input", "time_input",
	"file_uploader",
}

func sortedStrings(xs []string) []string {
	out := make([]string, 0, len(xs))
	out = append(out, xs...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// apps is the Go half of the logical app registry. Names must match
// python/apps.py:APPS exactly.
var apps = map[string]func(*appCtx, *st.Session){}

func init() {
	for name, fn := range widgetSnippets {
		apps["wd-"+name] = fn
	}
	apps["widgets-defaults"] = func(a *appCtx, s *st.Session) {
		for _, name := range widgetOrder {
			widgetSnippets[name](a, s)
		}
	}

	apps["text-elements"] = func(a *appCtx, s *st.Session) {
		s.Title("Parity")
		s.Header("Section")
		s.Subheader("Subsection")
		s.Markdown("**bold** and `code`")
		s.Caption("a caption")
		s.Latex("x^2 + y^2")
		s.Code("print(1)", "python")
		s.Text("plain text")
		s.Divider()
		s.Success("all good")
		s.Info("for your information")
		s.Warning("be careful")
		s.Error("it broke")
	}

	apps["write-dispatch"] = func(a *appCtx, s *st.Session) {
		s.Write("a string")
		s.Write(3)
		s.Write(true)
	}

	// The Go port has no help, disabled, placeholder or format parameter on any
	// widget; TextInputMax is the only one of the five concepts it models.
	apps["widgets-labelled"] = func(a *appCtx, s *st.Session) {
		s.Checkbox("Agree", true, "w_checkbox")
		s.Slider("Amount", 0, 10, 3, 1, "w_slider")
		s.TextInputMax("Name", "ada", 8, "w_text_input")
	}

	apps["slider-bounds"] = func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.Slider("Amount", 0, 10, 3, 1, "w_slider"))
	}
	apps["number-bounds"] = func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.NumberInputRange("Count", 0, 10, 5, "w_number_input"))
	}
	apps["selectbox-unknown"] = func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", s.SelectBox("Choose", []string{"alpha", "beta", "gamma"}, "w_selectbox"))
	}
	apps["multiselect-unknown"] = func(a *appCtx, s *st.Session) {
		a.put(s, "ss_v", sortedStrings(s.MultiSelect("Many", []string{"alpha", "beta", "gamma"}, "w_multiselect")))
	}

	apps["layout"] = func(a *appCtx, s *st.Session) {
		cols := s.Columns(2)
		cols[0].Text("left")
		cols[1].Text("right")

		tabs := s.Tabs([]string{"One", "Two"})
		tabs[0].Text("in one")
		tabs[1].Text("in two")

		s.Expander("More", true).Text("inside expander")
		s.Container().Text("inside container")
		s.Sidebar().Text("in sidebar")
	}

	apps["layout-nested"] = func(a *appCtx, s *st.Session) {
		cols := s.Columns(2)
		cols[0].Expander("Nested", false).Text("deep")
		cols[1].Text("shallow")
	}

	apps["layout-widget-in-column"] = func(a *appCtx, s *st.Session) {
		cols := s.Columns(2)
		a.put(s, "ss_v", cols[0].TextInput("Name", "ada", "w_text_input"))
		cols[1].Text("right")
	}

	// The Go port takes tabular data as [][]string with the header in row 0;
	// Streamlit takes a list of records. Same logical table either way.
	rows := [][]string{{"name", "qty"}, {"ada", "1"}, {"bob", "2"}}
	apps["dataframe"] = func(a *appCtx, s *st.Session) { s.DataFrame(rows) }
	apps["table"] = func(a *appCtx, s *st.Session) { s.Table(rows) }
	apps["json-element"] = func(a *appCtx, s *st.Session) {
		s.JSON(map[string]any{"b": 2, "a": []int{1, 2}})
	}

	apps["metric"] = func(a *appCtx, s *st.Session) {
		s.Metric("Revenue", "1.2M", "+4.1%")
		s.Metric("Latency", "120ms", "-8ms")
		s.Metric("Flat", "0", "0")
	}

	apps["chart-line"] = func(a *appCtx, s *st.Session) { s.LineChart([]float64{1, 3, 2}) }
	apps["chart-area"] = func(a *appCtx, s *st.Session) { s.AreaChart([]float64{1, 3, 2}) }
	apps["chart-bar"] = func(a *appCtx, s *st.Session) { s.BarChart([]float64{1, 3, 2}) }
	apps["chart-scatter"] = func(a *appCtx, s *st.Session) {
		s.ScatterChart([]float64{1, 2, 3}, []float64{4, 5, 6})
	}

	apps["form-atomic"] = func(a *appCtx, s *st.Session) {
		f := s.Form("signup")
		name := f.TextInput("Name", "", "f_name")
		age := f.NumberInput("Age", 18, "f_age")
		submitted := f.FormSubmitButton("Register")
		a.put(s, "ss_name", name)
		a.put(s, "ss_age", age)
		a.put(s, "ss_submitted", submitted)
	}

	apps["session-state"] = func(a *appCtx, s *st.Session) {
		a.put(s, "ss_runs", s.State.GetInt("ss_runs", 0)+1)
		clicks := s.State.GetInt("ss_clicks", 0)
		if s.Button("Bump", "w_button") {
			clicks++
		}
		a.put(s, "ss_clicks", clicks)
	}

	apps["button-one-run"] = func(a *appCtx, s *st.Session) {
		a.put(s, "ss_hit", s.Button("Fire", "w_button"))
		a.put(s, "ss_runs", s.State.GetInt("ss_runs", 0)+1)
	}

	apps["widget-hidden-then-shown"] = func(a *appCtx, s *st.Session) {
		if s.Checkbox("Show", true, "w_flag") {
			a.put(s, "ss_v", s.TextInput("Name", "ada", "w_text_input"))
		} else {
			a.put(s, "ss_v", nil)
		}
	}

	apps["cache-hits"] = func(a *appCtx, s *st.Session) {
		n := s.NumberInput("N", 2, "w_number_input")
		out := s.Cache(cacheKey("double", n), func() any {
			a.computed++
			return n * 2
		})
		a.pub("ss_computed", a.computed)
		a.pub("ss_out", out)
	}

	apps["cache-invalidate"] = func(a *appCtx, s *st.Session) {
		if s.Button("Clear", "w_button") {
			st.CacheClear()
		}
		out := s.Cache("constant", func() any {
			a.computed++
			return a.computed
		})
		a.pub("ss_computed", a.computed)
		a.pub("ss_out", out)
	}

	apps["stop"] = func(a *appCtx, s *st.Session) {
		s.Text("before")
		if s.Checkbox("Halt", false, "w_checkbox") {
			s.Stop()
		}
		s.Text("after")
	}

	// Session.Stop halts by panicking an unexported sentinel, so a recover() in
	// app code changes control flow. This app is the analogue of wrapping
	// st.stop() in `try: ... except BaseException:`.
	apps["stop-swallowed"] = func(a *appCtx, s *st.Session) {
		s.Text("before")
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.Text("swallowed")
				}
			}()
			s.Stop()
		}()
		s.Text("after")
	}

	apps["auto-keys"] = func(a *appCtx, s *st.Session) {
		if s.Checkbox("Show extra", false, "w_flag") {
			s.Slider("Extra", 0, 10, 0, 1)
		}
		a.put(s, "ss_target", s.Slider("Target", 0, 100, 5, 1))
	}

}

// cacheKey builds the explicit cache key the Go port requires. Streamlit's
// @st.cache_data hashes the arguments itself; [st.Session.Cache] takes a
// caller-derived key, so the argument has to be spliced in by hand.
func cacheKey(name string, n float64) string {
	return name + ":" + trimFloat(n)
}
