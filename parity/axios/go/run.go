// Command run is the Go-side parity runner for github.com/malcolmston/axios.
//
// It reads one JSON object per line on stdin and writes exactly one JSON object
// per line to stdout, mirroring parity/node/run.js. It never exits on a failing
// case: every panic/error is reported as {"ok":false}.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	ax "github.com/malcolmston/axios"
)

var baseURL = ""

type inMsg struct {
	ID   string            `json:"id"`
	Fn   string            `json:"fn"`
	Args []json.RawMessage `json:"args"`
}

type outMsg struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Value any    `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

func sub(s string) string { return strings.ReplaceAll(s, "$BASE", baseURL) }

// ------------------------------------------------------------------- spec ---

type dataSpec struct {
	JSON       *json.RawMessage  `json:"json"`
	Text       *string           `json:"text"`
	URLEncoded map[string]any    `json:"urlencoded"`
	BytesHex   *string           `json:"bytesHex"`
	Form       map[string]string `json:"form"`
}

type serSpec struct {
	Custom  string `json:"custom"`
	Indexes *bool  `json:"indexes"`
	hasIdx  bool
}

func (s *serSpec) UnmarshalJSON(b []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if v, ok := raw["custom"]; ok {
		_ = json.Unmarshal(v, &s.Custom)
	}
	if v, ok := raw["indexes"]; ok {
		s.hasIdx = true
		var p *bool
		_ = json.Unmarshal(v, &p)
		s.Indexes = p
	}
	return nil
}

type spec struct {
	BaseURL           *string        `json:"baseURL"`
	URL               *string        `json:"url"`
	Method            *string        `json:"method"`
	Headers           map[string]any `json:"headers"`
	Params            map[string]any `json:"params"`
	ParamsSerializer  *serSpec       `json:"paramsSerializer"`
	TimeoutMs         *int           `json:"timeoutMs"`
	ValidateStatus    *string        `json:"validateStatus"`
	MaxRedirects      *int           `json:"maxRedirects"`
	Auth              *authSpec      `json:"auth"`
	ResponseType      *string        `json:"responseType"`
	MaxContentLength  *int64         `json:"maxContentLength"`
	MaxBodyLength     *int64         `json:"maxBodyLength"`
	Decompress        *bool          `json:"decompress"`
	TransformRequest  *string        `json:"transformRequest"`
	TransformResponse *string        `json:"transformResponse"`
	Data              *dataSpec      `json:"data"`
	InterceptRequest  []string       `json:"interceptRequest"`
	InterceptResponse []string       `json:"interceptResponse"`
	Requests          []spec         `json:"requests"`
	Pairs             [][2]any       `json:"pairs"`
	Client            *spec          `json:"client"`
	Request           *spec          `json:"request"`
	Expose            string         `json:"expose"`
	ObserveHeaders    []string       `json:"observeHeaders"`
	AssertStatusText  bool           `json:"assertStatusText"`
}

type authSpec struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// ---------------------------------------------------------------- presets ---

var validators = map[string]func(int) bool{
	"all":     func(int) bool { return true },
	"lt400":   func(s int) bool { return s < 400 },
	"lt500":   func(s int) bool { return s < 500 },
	"only418": func(s int) bool { return s == 418 },
	"none":    func(int) bool { return true }, // axios validateStatus:null == accept all
}

var reqTransforms = map[string]ax.RequestTransform{
	"prefix": func(b []byte, _ http.Header) ([]byte, error) {
		return append([]byte("PRE|"), b...), nil
	},
	"setHeader": func(b []byte, h http.Header) ([]byte, error) {
		h.Set("X-Tr", "1")
		return b, nil
	},
	"boom": func([]byte, http.Header) ([]byte, error) {
		return nil, errors.New("transformRequest boom")
	},
}

var resTransforms = map[string]ax.ResponseTransform{
	"identity": func(b []byte, _ http.Header) ([]byte, error) { return b, nil },
	"upper":    func(b []byte, _ http.Header) ([]byte, error) { return []byte(strings.ToUpper(string(b))), nil },
	"wrap":     func(b []byte, _ http.Header) ([]byte, error) { return append([]byte("WRAP|"), b...), nil },
}

var reqInterceptors = map[string]ax.RequestInterceptor{
	"addHeader":  func(r *http.Request) error { r.Header.Set("X-Req-Ic", "1"); return nil },
	"addHeader2": func(r *http.Request) error { r.Header.Set("X-Req-Ic2", "2"); return nil },
	"reject":     func(*http.Request) error { return errors.New("request interceptor rejected") },
}

var resInterceptors = map[string]ax.ResponseInterceptor{
	"tagHeader": func(r *ax.Response) error { r.Headers.Set("X-Res-Ic", "1"); return nil },
	"reject":    func(*ax.Response) error { return errors.New("response interceptor rejected") },
}

func serializerFrom(s *serSpec) (ax.ParamsSerializer, *ax.ArrayFormat) {
	if s == nil {
		return nil, nil
	}
	if s.Custom == "fixed" {
		return func(url.Values) string { return "custom=1&x=Y" }, nil
	}
	if s.hasIdx {
		f := ax.ArrayFormatRepeat // indexes: null -> repeat key
		if s.Indexes != nil {
			if *s.Indexes {
				f = ax.ArrayFormatIndices
			} else {
				f = ax.ArrayFormatBrackets
			}
		}
		return nil, &f
	}
	return nil, nil
}

var responseTypes = map[string]ax.ResponseType{
	"json":   ax.ResponseJSON,
	"text":   ax.ResponseText,
	"stream": ax.ResponseStream,
}

// ------------------------------------------------------------ spec -> cfg ---

func headerGroups(h map[string]any) (http.Header, ax.HeaderDefaults) {
	flat := http.Header{}
	var g ax.HeaderDefaults
	target := map[string]*http.Header{
		"common": &g.Common, "get": &g.Get, "post": &g.Post, "put": &g.Put,
		"patch": &g.Patch, "delete": &g.Delete, "head": &g.Head, "options": &g.Options,
	}
	for k, v := range h {
		if m, ok := v.(map[string]any); ok {
			if p, isGroup := target[strings.ToLower(k)]; isGroup {
				if *p == nil {
					*p = http.Header{}
				}
				for k2, v2 := range m {
					(*p).Set(k2, fmt.Sprint(v2))
				}
				continue
			}
		}
		if v == nil {
			// A null header value means "remove this header" in axios. The port
			// expresses removal as a request-level header key with no values,
			// which overrides (and thus drops) any inherited default.
			flat[http.CanonicalHeaderKey(k)] = []string{}
			continue
		}
		flat.Set(k, fmt.Sprint(v))
	}
	if len(flat) == 0 {
		flat = nil
	}
	return flat, g
}

func clientConfig(s *spec) ax.Config {
	cfg := ax.Config{}
	if s == nil {
		return cfg
	}
	if s.BaseURL != nil {
		cfg.BaseURL = sub(*s.BaseURL)
	}
	if s.Headers != nil {
		cfg.Headers, cfg.HeaderGroups = headerGroups(s.Headers)
	}
	if s.Params != nil {
		cfg.ParamsMap = s.Params
	}
	if ps, af := serializerFrom(s.ParamsSerializer); ps != nil || af != nil {
		cfg.ParamsSerializer = ps
		if af != nil {
			cfg.ArrayFormat = *af
		}
	}
	if s.TimeoutMs != nil {
		cfg.Timeout = time.Duration(*s.TimeoutMs) * time.Millisecond
	}
	if s.ValidateStatus != nil {
		cfg.ValidateStatus = validators[*s.ValidateStatus]
	}
	if s.MaxRedirects != nil {
		cfg.MaxRedirects = s.MaxRedirects
	}
	if s.Auth != nil {
		cfg.BasicAuth = &ax.BasicAuth{Username: s.Auth.Username, Password: s.Auth.Password}
	}
	if s.ResponseType != nil {
		cfg.ResponseType = responseTypes[*s.ResponseType]
	}
	if s.MaxContentLength != nil {
		cfg.MaxContentLength = *s.MaxContentLength
	}
	if s.MaxBodyLength != nil {
		cfg.MaxBodyLength = *s.MaxBodyLength
	}
	if s.Decompress != nil {
		cfg.Decompress = s.Decompress
	}
	if s.TransformRequest != nil {
		cfg.TransformRequest = []ax.RequestTransform{reqTransforms[*s.TransformRequest]}
	}
	if s.TransformResponse != nil {
		cfg.TransformResponse = []ax.ResponseTransform{resTransforms[*s.TransformResponse]}
	}
	for _, n := range s.InterceptRequest {
		cfg.RequestInterceptors = append(cfg.RequestInterceptors, reqInterceptors[n])
	}
	for _, n := range s.InterceptResponse {
		cfg.ResponseInterceptors = append(cfg.ResponseInterceptors, resInterceptors[n])
	}
	return cfg
}

func requestConfig(s *spec) *ax.RequestConfig {
	if s == nil {
		return nil
	}
	rc := &ax.RequestConfig{}
	if s.Headers != nil {
		h, _ := headerGroups(s.Headers)
		rc.Headers = h
	}
	if s.Params != nil {
		rc.ParamsMap = s.Params
	}
	if ps, af := serializerFrom(s.ParamsSerializer); ps != nil || af != nil {
		rc.ParamsSerializer = ps
		rc.ArrayFormat = af
	}
	if s.TimeoutMs != nil {
		rc.Timeout = time.Duration(*s.TimeoutMs) * time.Millisecond
	}
	if s.ValidateStatus != nil {
		rc.ValidateStatus = validators[*s.ValidateStatus]
	}
	if s.Auth != nil {
		rc.BasicAuth = &ax.BasicAuth{Username: s.Auth.Username, Password: s.Auth.Password}
	}
	if s.ResponseType != nil {
		rt := responseTypes[*s.ResponseType]
		rc.ResponseType = &rt
	}
	if s.MaxContentLength != nil {
		rc.MaxContentLength = *s.MaxContentLength
	}
	if s.MaxBodyLength != nil {
		rc.MaxBodyLength = *s.MaxBodyLength
	}
	if s.TransformRequest != nil {
		rc.TransformRequest = []ax.RequestTransform{reqTransforms[*s.TransformRequest]}
	}
	if s.TransformResponse != nil {
		rc.TransformResponse = []ax.ResponseTransform{resTransforms[*s.TransformResponse]}
	}
	return rc
}

func bodyFrom(d *dataSpec) (any, string, error) {
	if d == nil {
		return nil, "", nil
	}
	switch {
	case d.JSON != nil:
		var v any
		if err := json.Unmarshal(*d.JSON, &v); err != nil {
			return nil, "", err
		}
		return v, "", nil
	case d.Text != nil:
		return *d.Text, "", nil
	case d.URLEncoded != nil:
		vals := url.Values{}
		for k, v := range d.URLEncoded {
			vals.Set(k, fmt.Sprint(v))
		}
		return vals, "", nil
	case d.BytesHex != nil:
		b, err := hex.DecodeString(*d.BytesHex)
		return b, "", err
	case d.Form != nil:
		fd := ax.NewFormData()
		keys := make([]string, 0, len(d.Form))
		for k := range d.Form {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fd.AddField(k, d.Form[k])
		}
		body, err := fd.Bytes()
		if err != nil {
			return nil, "", err
		}
		return body, fd.ContentType(), nil
	}
	return nil, "", nil
}

// ------------------------------------------------------------- rendering ---

var dropResHeaders = map[string]bool{
	"date": true, "connection": true, "keep-alive": true,
	"transfer-encoding": true, "content-length": true,
}

func renderResHeaders(h http.Header) map[string]any {
	o := map[string]any{}
	for k, v := range h {
		lk := strings.ToLower(k)
		if dropResHeaders[lk] {
			continue
		}
		o[lk] = strings.Join(v, ", ")
	}
	return o
}

func renderOK(r *ax.Response, sp *spec) map[string]any {
	v := map[string]any{
		"status":     r.Status,
		"resHeaders": renderResHeaders(r.Headers),
	}
	expose := sp.Expose
	if expose == "" {
		expose = "json"
	}
	switch expose {
	case "none":
		v["data"] = nil
	case "text":
		v["data"] = r.Text()
	case "auto":
		v["data"] = autoData(r.Data())
	default:
		var parsed any
		if err := r.JSON(&parsed); err != nil {
			v["data"] = r.Text()
		} else {
			v["data"] = parsed
		}
	}
	if sp.AssertStatusText {
		v["statusText"] = r.StatusText
	}
	if len(sp.ObserveHeaders) > 0 {
		pick := map[string]any{}
		var obs struct {
			Headers map[string]string `json:"headers"`
		}
		_ = json.Unmarshal(r.Bytes(), &obs)
		for _, n := range sp.ObserveHeaders {
			if val, ok := obs.Headers[n]; ok {
				pick[n] = val
			} else {
				pick[n] = nil
			}
		}
		v["pickHeaders"] = pick
	}
	return v
}

func renderErr(err error) map[string]any {
	v := map[string]any{
		"isAxiosError": ax.IsAxiosError(err),
		"isCancel":     ax.IsCancel(err),
		"code":         nil,
		"hasResponse":  false,
		"status":       nil,
	}
	if code := ax.ErrorCode(err); code != "" {
		v["code"] = code
	}
	if ae := ax.AsError(err); ae != nil {
		if ae.Response != nil {
			v["hasResponse"] = true
			v["status"] = ae.Response.Status
		}
	}
	return v
}

// autoData mirrors the node runner's renderData("auto"): it reports the JSON
// kind of the parsed response body and its stringified form, so the port's
// Response.Data (which auto-parses like axios) can be compared to res.data.
func autoData(d any) map[string]any {
	dt := "object"
	switch d.(type) {
	case nil:
		return map[string]any{"dataType": "null", "text": "null"}
	case string:
		return map[string]any{"dataType": "string", "text": d.(string)}
	case bool:
		dt = "boolean"
	case float64:
		dt = "number"
	case []any:
		dt = "array"
	case map[string]any:
		dt = "object"
	}
	b, _ := json.Marshal(d)
	return map[string]any{"dataType": dt, "text": string(b)}
}

// ------------------------------------------------------------------- fns ---

func doRequest(sp *spec) (any, error) {
	c := ax.New(clientConfig(sp.Client))
	rs := sp.Request
	if rs == nil {
		rs = &spec{}
	}
	rc := requestConfig(rs)
	method := "GET"
	if rs.Method != nil {
		method = strings.ToUpper(*rs.Method)
	}
	rawURL := ""
	if rs.URL != nil {
		rawURL = sub(*rs.URL)
	}
	body, ct, err := bodyFrom(rs.Data)
	if err != nil {
		return nil, err
	}
	if ct != "" && rc != nil {
		rc.ContentType = ct
	}
	resp, err := c.Request(method, rawURL, body, rc)
	if err != nil {
		return nil, err
	}
	return renderOK(resp, sp), nil
}

func dispatch(m inMsg) (any, error) {
	arg := func(i int) (*spec, error) {
		var s spec
		if i >= len(m.Args) {
			return &s, nil
		}
		if err := json.Unmarshal(m.Args[i], &s); err != nil {
			return nil, err
		}
		return &s, nil
	}
	switch m.Fn {
	case "request":
		sp, err := arg(0)
		if err != nil {
			return nil, err
		}
		return doRequest(sp)

	case "getUri":
		sp, err := arg(0)
		if err != nil {
			return nil, err
		}
		c := ax.New(clientConfig(sp.Client))
		rs := sp.Request
		if rs == nil {
			rs = &spec{}
		}
		u := ""
		if rs.URL != nil {
			u = sub(*rs.URL)
		}
		return c.GetUri(u, requestConfig(rs))

	case "mergeConfig":
		a, err := arg(0)
		if err != nil {
			return nil, err
		}
		b, err := arg(1)
		if err != nil {
			return nil, err
		}
		m := ax.MergeConfig(clientConfig(a), clientConfig(b))
		hs := map[string]any{}
		for k, v := range m.Headers {
			hs[strings.ToLower(k)] = strings.Join(v, ", ")
		}
		var base any
		if m.BaseURL != "" {
			base = m.BaseURL
		}
		var to any
		if m.Timeout != 0 {
			to = float64(m.Timeout / time.Millisecond)
		}
		var params any
		if m.ParamsMap != nil {
			params = m.ParamsMap
		}
		return map[string]any{"baseURL": base, "timeoutMs": to, "headers": hs, "params": params}, nil

	case "formToJSON":
		sp, err := arg(0)
		if err != nil {
			return nil, err
		}
		vals := url.Values{}
		for _, p := range sp.Pairs {
			vals.Add(fmt.Sprint(p[0]), fmt.Sprint(p[1]))
		}
		return ax.FormToJSON(vals), nil

	case "all":
		sp, err := arg(0)
		if err != nil {
			return nil, err
		}
		c := ax.New(clientConfig(sp.Client))
		calls := make([]func() (*ax.Response, error), 0, len(sp.Requests))
		for i := range sp.Requests {
			rs := sp.Requests[i]
			calls = append(calls, func() (*ax.Response, error) {
				u := ""
				if rs.URL != nil {
					u = sub(*rs.URL)
				}
				method := "GET"
				if rs.Method != nil {
					method = strings.ToUpper(*rs.Method)
				}
				return c.Request(method, u, nil, requestConfig(&rs))
			})
		}
		resps, err := ax.All(calls...)
		if err != nil {
			return nil, err
		}
		out := make([]any, 0, len(resps))
		for _, r := range resps {
			out = append(out, r.Status)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unknown fn: %s", m.Fn)
}

func safeDispatch(m inMsg) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return dispatch(m)
}

func main() {
	if len(os.Args) > 1 {
		baseURL = os.Args[1]
	}
	if b := os.Getenv("PARITY_BASE_URL"); b != "" {
		baseURL = b
	}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<22)
	enc := json.NewEncoder(os.Stdout)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var m inMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			_ = enc.Encode(outMsg{ID: "?", OK: false, Error: "bad input line: " + err.Error()})
			os.Stdout.Sync()
			continue
		}
		v, err := safeDispatch(m)
		if err != nil {
			_ = enc.Encode(outMsg{ID: m.ID, OK: false, Error: err.Error(), Value: renderErr(err)})
		} else {
			_ = enc.Encode(outMsg{ID: m.ID, OK: true, Value: v})
		}
		os.Stdout.Sync()
	}
}
