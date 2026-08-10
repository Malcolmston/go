// Go-side parity runner for github.com/malcolmston/prisma.
//
// Speaks the same JSON-Lines protocol as ../node/run.js: one request object per
// line on stdin, exactly one reply per line on stdout, never exiting on a
// failing case.
//
// # Why the models are declared twice
//
// The Go port has no schema language, no migration engine and no code
// generator; a model is a Go struct with `prisma:"..."` tags, interpreted by
// reflection at run time. There is therefore nothing here that could consume
// ../node/schema.prisma. The two logical models below are the hand-written
// counterpart of that datamodel, and the DDL is not written twice: the harness
// runs `prisma db push` once and this runner copies the resulting SQLite file,
// so both sides execute against byte-identical tables, indexes and foreign
// keys.
//
// # Case shape
//
// Every case is a script of steps against one database. `reset` truncates every
// table and clears sqlite_sequence, `seed` inserts the fixed dataset (duplicated
// from ../node/run.js and itself scored by the seed-dump case). A step that
// fails is recorded as {"ok":false,"code":"P2002"} and the script continues, so
// a must-fail step can be followed by a `dump` proving nothing was written. Only
// the error CODE is emitted, never the message.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	prisma "github.com/malcolmston/prisma"
	_ "modernc.org/sqlite"
)

// ------------------------------------------------------------------- models

// User mirrors `model User` in ../node/schema.prisma. Field names are snake_case
// on both sides so a Prisma field name, a SQL column name and the `col=` tag are
// always the same string.
//
// `Name` and `City` are *string, not string, because SQLite must be able to
// receive a real NULL for the not-null-violation case: the port maps Go zero
// values straight through, so a non-pointer string could only ever write "".
type User struct {
	ID    int64   `prisma:"col=id,pk"`
	Name  *string `prisma:"col=name"`
	Email string  `prisma:"col=email"`
	City  *string `prisma:"col=city"`
	Age   *int64  `prisma:"col=age"`
	Score float64 `prisma:"col=score"`
	Posts []Post  `prisma:"relation,fk=author_id,references=id"`
}

// Post mirrors `model Post` in ../node/schema.prisma.
type Post struct {
	ID        int64  `prisma:"col=id,pk"`
	Title     string `prisma:"col=title"`
	Views     int64  `prisma:"col=views"`
	Published bool   `prisma:"col=published"`
	AuthorID  int64  `prisma:"col=author_id"`
	Author    *User  `prisma:"relation,fk=author_id,references=id"`
}

// relationGoName maps a Prisma relation field name to the Go struct field name
// the port's Include/Some/Every/None/Is take.
var relationGoName = map[string]string{"posts": "Posts", "author": "Author"}

// relationTarget maps a relation field to the model it points at.
var relationTarget = map[string]string{"posts": "post", "author": "user"}

// ------------------------------------------------------------------- the seed
//
// Duplicated from ../node/run.js. ids are consecutive from 1: the port treats
// every integer primary key as database-generated and drops it from the INSERT,
// so inserting in this order is the only way it can land on the same ids the
// oracle writes explicitly.

func sp(s string) *string { return &s }
func ip(n int64) *int64   { return &n }

var seedUsers = []User{
	{ID: 1, Name: sp("Ada"), Email: "ada@x.io", City: sp("Paris"), Age: ip(36), Score: 10},
	{ID: 2, Name: sp("Grace"), Email: "grace@x.io", City: sp("Paris"), Age: ip(44), Score: 20},
	{ID: 3, Name: sp("Alan"), Email: "alan@x.io", City: sp("London"), Age: ip(40), Score: 30},
	{ID: 4, Name: sp("Edsger"), Email: "edsger@x.io", City: nil, Age: ip(30), Score: 40},
	{ID: 5, Name: sp("Barbara"), Email: "barbara@x.io", City: sp("London"), Age: nil, Score: 50},
	{ID: 6, Name: sp("Linus"), Email: "linus@x.io", City: sp("Berlin"), Age: ip(24), Score: 60},
	{ID: 7, Name: sp("Ken"), Email: "ken@x.io", City: nil, Age: nil, Score: 70},
	{ID: 8, Name: sp("Donald"), Email: "donald@x.io", City: sp("Berlin"), Age: ip(60), Score: 80},
}

var seedPosts = []Post{
	{ID: 1, Title: "Alpha", Views: 10, Published: true, AuthorID: 1},
	{ID: 2, Title: "Beta", Views: 20, Published: false, AuthorID: 1},
	{ID: 3, Title: "Gamma", Views: 30, Published: true, AuthorID: 2},
	{ID: 4, Title: "Delta", Views: 40, Published: true, AuthorID: 2},
	{ID: 5, Title: "Epsilon", Views: 50, Published: false, AuthorID: 3},
	{ID: 6, Title: "Zeta", Views: 60, Published: true, AuthorID: 3},
	{ID: 7, Title: "Eta", Views: 70, Published: false, AuthorID: 4},
	{ID: 8, Title: "Theta", Views: 80, Published: true, AuthorID: 6},
	{ID: 9, Title: "Iota", Views: 90, Published: true, AuthorID: 6},
	{ID: 10, Title: "Kappa", Views: 100, Published: false, AuthorID: 6},
	{ID: 11, Title: "50% off", Views: 5, Published: true, AuthorID: 8},
	{ID: 12, Title: "beta_two", Views: 15, Published: false, AuthorID: 8},
}

// ----------------------------------------------------------------- protocol

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

// stepResult is one step's answer. Only the error code travels, never the
// message: the harness compares codes. A failed step always carries `code`
// (possibly "", for an error the port does not classify) and a null `value`, so
// the emitted shape matches ../node/run.js exactly and a deep-equal can never
// trip on a merely-absent key.
type stepResult map[string]any

func okStep(v any) stepResult { return stepResult{"ok": true, "value": v} }

func failStep(code string) stepResult {
	return stepResult{"ok": false, "code": code, "value": nil}
}

// step is one operation in a case script, expressed in Prisma's own vocabulary.
type step struct {
	Op      string `json:"op"`
	Model   string `json:"model"`
	Dataset string `json:"dataset"`

	Data   map[string]any   `json:"data"`
	Rows   []map[string]any `json:"rows"`
	Create map[string]any   `json:"create"`
	Update map[string]any   `json:"update"`

	Where    map[string]any   `json:"where"`
	Select   []string         `json:"select"`
	Include  []string         `json:"include"`
	OrderBy  []map[string]any `json:"orderBy"`
	Distinct []string         `json:"distinct"`
	Take     *int             `json:"take"`
	Skip     *int             `json:"skip"`
	Cursor   map[string]any   `json:"cursor"`

	By     []string       `json:"by"`
	Having map[string]any `json:"having"`
	Count  bool           `json:"_count"`
	Sum    []string       `json:"_sum"`
	Avg    []string       `json:"_avg"`
	Min    []string       `json:"_min"`
	Max    []string       `json:"_max"`

	// NullStyle selects how a `field: null` filter is translated for the port.
	// The default ("operator") uses prisma.IsNull, the port's documented null
	// predicate. "literal" uses prisma.Equals(col, nil) — the mechanical
	// translation of Prisma's shorthand — to show what that actually does.
	// Upstream ignores this field.
	NullStyle string `json:"nullStyle"`
}

// ------------------------------------------------------------------ database

var (
	client *prisma.Client
	db     *sql.DB
)

func openDB() error {
	dir, err := os.MkdirTemp("", "prisma-parity-go-")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, "parity.db")
	if tpl := os.Getenv("PARITY_TEMPLATE_DB"); tpl != "" {
		raw, err := os.ReadFile(tpl)
		if err != nil {
			return fmt.Errorf("read template db: %w", err)
		}
		if err := os.WriteFile(file, raw, 0o644); err != nil {
			return err
		}
	} else {
		return errors.New("PARITY_TEMPLATE_DB is not set: the schema comes from `prisma db push`")
	}
	// foreign_keys must be ON to match Prisma's SQLite connector, which enables
	// it on every connection.
	db, err = sql.Open("sqlite", "file:"+file+"?_pragma=foreign_keys(1)")
	if err != nil {
		return err
	}
	// One connection: PRAGMA and rowid sequences are per-connection concerns and
	// a pool would make id assignment non-deterministic.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return err
	}
	client = prisma.NewClient(db)
	if _, err := client.Register(User{}); err != nil {
		return err
	}
	if _, err := client.Register(Post{}); err != nil {
		return err
	}
	return nil
}

// resetTables truncates every table, children first, and clears the
// AUTOINCREMENT counter so ids restart at 1 for every case.
func resetTables(ctx context.Context) error {
	for _, t := range []string{"posts", "users"} {
		if _, err := client.ExecuteRaw(ctx, `DELETE FROM "`+t+`"`); err != nil {
			return err
		}
	}
	_, err := client.ExecuteRaw(ctx, `DELETE FROM sqlite_sequence`)
	return err
}

func seed(ctx context.Context, name string) (any, error) {
	if name == "" {
		name = "base"
	}
	if name != "base" {
		return nil, fmt.Errorf("unknown dataset %s", name)
	}
	if _, err := prisma.NewQuery[User](client).CreateMany(ctx, seedUsers); err != nil {
		return nil, err
	}
	if _, err := prisma.NewQuery[Post](client).CreateMany(ctx, seedPosts); err != nil {
		return nil, err
	}
	return map[string]any{"users": len(seedUsers), "posts": len(seedPosts)}, nil
}

// -------------------------------------------------------------- canonical rows

func userMap(u *User, sel, inc []string) map[string]any {
	all := map[string]any{
		"id": u.ID, "name": derefStr(u.Name), "email": u.Email,
		"city": derefStr(u.City), "age": derefInt(u.Age), "score": u.Score,
	}
	out := project(all, sel)
	if contains(inc, "posts") {
		posts := make([]any, 0, len(u.Posts))
		for i := range u.Posts {
			posts = append(posts, postMap(&u.Posts[i], nil, nil))
		}
		out["posts"] = posts
	}
	return out
}

func postMap(p *Post, sel, inc []string) map[string]any {
	all := map[string]any{
		"id": p.ID, "title": p.Title, "views": p.Views,
		"published": p.Published, "author_id": p.AuthorID,
	}
	out := project(all, sel)
	if contains(inc, "author") {
		if p.Author == nil {
			out["author"] = nil
		} else {
			out["author"] = userMap(p.Author, nil, nil)
		}
	}
	return out
}

// project keeps only the selected columns. The port returns a fully-typed struct
// whose unselected fields are zero rather than absent, so the projection has to
// be re-applied here for the emitted row to mean the same thing as Prisma's.
func project(all map[string]any, sel []string) map[string]any {
	if len(sel) == 0 {
		return all
	}
	out := make(map[string]any, len(sel))
	for _, c := range sel {
		v, ok := all[c]
		if !ok {
			v = nil
		}
		out[c] = v
	}
	return out
}

func derefStr(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}

func derefInt(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- value coercion

// jsonValue turns a decoded JSON value into something worth binding. Whole
// float64s become int64 so an id or a views count is bound as an integer and
// SQLite's numeric affinity behaves the same as it does for the oracle.
func jsonValue(v any) any {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return int64(f)
	}
	return f
}

// driverValue normalises a value scanned by the port's raw/group-by paths:
// []byte becomes a string so a TEXT column compares equal to the oracle's.
func driverValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// ------------------------------------------------------------------- filters

// buildConds translates a Prisma `where` object into the port's Condition
// values. Keys are visited in sorted order so the generated SQL is stable.
func buildConds(model string, where map[string]any, nullStyle string) ([]prisma.Condition, error) {
	if len(where) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(where))
	for k := range where {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []prisma.Condition
	for _, k := range keys {
		v := where[k]
		switch k {
		case "AND", "OR", "NOT":
			group, err := buildGroup(model, k, v, nullStyle)
			if err != nil {
				return nil, err
			}
			out = append(out, group)
		default:
			if target, isRel := relationTarget[k]; isRel && isRelationOf(model, k) {
				c, err := buildRelationFilter(k, target, v, nullStyle)
				if err != nil {
					return nil, err
				}
				out = append(out, c)
				continue
			}
			cs, err := buildFieldConds(model, k, v, nullStyle)
			if err != nil {
				return nil, err
			}
			out = append(out, cs...)
		}
	}
	return out, nil
}

func isRelationOf(model, field string) bool {
	return (model == "user" && field == "posts") || (model == "post" && field == "author")
}

func buildGroup(model, op string, v any, nullStyle string) (prisma.Condition, error) {
	var members []map[string]any
	switch t := v.(type) {
	case []any:
		for _, m := range t {
			mm, ok := m.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s member must be an object", op)
			}
			members = append(members, mm)
		}
	case map[string]any:
		members = []map[string]any{t}
	default:
		return nil, fmt.Errorf("%s must be an object or array", op)
	}

	var conds []prisma.Condition
	for _, m := range members {
		cs, err := buildConds(model, m, nullStyle)
		if err != nil {
			return nil, err
		}
		// Each member's own keys are AND-ed together, matching Prisma.
		conds = append(conds, prisma.And(cs...))
	}
	switch op {
	case "AND":
		return prisma.And(conds...), nil
	case "OR":
		return prisma.Or(conds...), nil
	default:
		return prisma.NotGroup(prisma.And(conds...)), nil
	}
}

func buildRelationFilter(field, target string, v any, nullStyle string) (prisma.Condition, error) {
	spec, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("relation filter %s must be an object", field)
	}
	if len(spec) != 1 {
		return nil, fmt.Errorf("relation filter %s takes exactly one quantifier", field)
	}
	goRel := relationGoName[field]
	for kind, inner := range spec {
		im, ok := inner.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("relation filter %s.%s must be an object", field, kind)
		}
		conds, err := buildConds(target, im, nullStyle)
		if err != nil {
			return nil, err
		}
		switch kind {
		case "some":
			return prisma.Some(goRel, conds...), nil
		case "every":
			return prisma.Every(goRel, conds...), nil
		case "none":
			return prisma.None(goRel, conds...), nil
		case "is":
			return prisma.Is(goRel, conds...), nil
		default:
			return nil, fmt.Errorf("unsupported relation quantifier %q", kind)
		}
	}
	return nil, fmt.Errorf("empty relation filter")
}

// buildFieldConds translates one scalar field's filter. Several operators on the
// same field are AND-ed, as Prisma does.
func buildFieldConds(model, col string, v any, nullStyle string) ([]prisma.Condition, error) {
	if v == nil {
		return []prisma.Condition{nullCond(col, false, nullStyle)}, nil
	}
	ops, ok := v.(map[string]any)
	if !ok {
		return []prisma.Condition{prisma.Equals(col, jsonValue(v))}, nil
	}

	keys := make([]string, 0, len(ops))
	for k := range ops {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []prisma.Condition
	for _, k := range keys {
		ov := ops[k]
		switch k {
		case "equals":
			if ov == nil {
				out = append(out, nullCond(col, false, nullStyle))
			} else {
				out = append(out, prisma.Equals(col, jsonValue(ov)))
			}
		case "not":
			switch t := ov.(type) {
			case nil:
				out = append(out, nullCond(col, true, nullStyle))
			case map[string]any:
				inner, err := buildFieldConds(model, col, t, nullStyle)
				if err != nil {
					return nil, err
				}
				out = append(out, prisma.NotGroup(prisma.And(inner...)))
			default:
				out = append(out, prisma.Not(col, jsonValue(ov)))
			}
		case "in", "notIn":
			list, ok := ov.([]any)
			if !ok {
				return nil, fmt.Errorf("%s.%s must be an array", col, k)
			}
			vals := make([]any, len(list))
			for i, x := range list {
				vals[i] = jsonValue(x)
			}
			if k == "in" {
				out = append(out, prisma.In(col, vals...))
			} else {
				out = append(out, prisma.NotIn(col, vals...))
			}
		case "lt":
			out = append(out, prisma.Lt(col, jsonValue(ov)))
		case "lte":
			out = append(out, prisma.Lte(col, jsonValue(ov)))
		case "gt":
			out = append(out, prisma.Gt(col, jsonValue(ov)))
		case "gte":
			out = append(out, prisma.Gte(col, jsonValue(ov)))
		case "contains", "startsWith", "endsWith":
			s, ok := ov.(string)
			if !ok {
				return nil, fmt.Errorf("%s.%s must be a string", col, k)
			}
			switch k {
			case "contains":
				out = append(out, prisma.Contains(col, s))
			case "startsWith":
				out = append(out, prisma.StartsWith(col, s))
			default:
				out = append(out, prisma.EndsWith(col, s))
			}
		default:
			return nil, fmt.Errorf("unsupported filter operator %q on %s", k, col)
		}
	}
	return out, nil
}

// nullCond renders Prisma's `field: null` / `field: {not: null}`. The port has
// no shorthand for either; IsNull/IsNotNull are its documented predicates, and
// the "literal" style shows what the mechanical translation to Equals/Not does
// instead.
func nullCond(col string, negate bool, style string) prisma.Condition {
	if style == "literal" {
		if negate {
			return prisma.Not(col, nil)
		}
		return prisma.Equals(col, nil)
	}
	if negate {
		return prisma.IsNotNull(col)
	}
	return prisma.IsNull(col)
}

// ------------------------------------------------------------- query assembly

// applyOpts pours a step's Prisma-shaped arguments into the port's builder.
func applyOpts[T any](q *prisma.Query[T], model string, s step) (*prisma.Query[T], error) {
	conds, err := buildConds(model, s.Where, s.NullStyle)
	if err != nil {
		return nil, err
	}
	if len(conds) > 0 {
		q = q.Where(conds...)
	}
	for _, ob := range s.OrderBy {
		cols := make([]string, 0, len(ob))
		for c := range ob {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		for _, col := range cols {
			switch spec := ob[col].(type) {
			case string:
				q = q.OrderBy(col, direction(spec))
			case map[string]any:
				dir := direction(str(spec["sort"]))
				switch str(spec["nulls"]) {
				case "first":
					q = q.OrderByNulls(col, dir, prisma.NullsFirst)
				case "last":
					q = q.OrderByNulls(col, dir, prisma.NullsLast)
				default:
					q = q.OrderBy(col, dir)
				}
			default:
				return nil, fmt.Errorf("bad orderBy for %s", col)
			}
		}
	}
	if len(s.Select) > 0 {
		q = q.Select(s.Select...)
	}
	for _, r := range s.Include {
		goRel, ok := relationGoName[r]
		if !ok {
			return nil, fmt.Errorf("unknown relation %q", r)
		}
		q = q.Include(goRel)
	}
	if len(s.Distinct) > 0 {
		q = q.Distinct(s.Distinct...)
	}
	if s.Cursor != nil {
		cols := make([]string, 0, len(s.Cursor))
		for c := range s.Cursor {
			cols = append(cols, c)
		}
		sort.Strings(cols)
		for _, c := range cols {
			q = q.Cursor(c, jsonValue(s.Cursor[c]))
		}
	}
	if s.Take != nil {
		q = q.Take(*s.Take)
	}
	if s.Skip != nil {
		q = q.Skip(*s.Skip)
	}
	return q, nil
}

func direction(s string) prisma.Direction {
	if strings.EqualFold(s, "desc") {
		return prisma.Desc
	}
	return prisma.Asc
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// ------------------------------------------------------------------- decoding

// decodeUser builds a User from a Prisma `data` object. A missing key stays at
// the Go zero value, which is exactly what the port would write.
func decodeUser(data map[string]any) (User, error) {
	var u User
	for k, v := range data {
		switch k {
		case "id":
			u.ID = toInt(v)
		case "name":
			u.Name = toStrPtr(v)
		case "email":
			u.Email = str(v)
		case "city":
			u.City = toStrPtr(v)
		case "age":
			if v != nil {
				u.Age = ip(toInt(v))
			}
		case "score":
			u.Score = toFloat(v)
		case "posts":
			// handled by the nested-write path
		default:
			return u, fmt.Errorf("unknown User field %q", k)
		}
	}
	return u, nil
}

func decodePost(data map[string]any) (Post, error) {
	var p Post
	for k, v := range data {
		switch k {
		case "id":
			p.ID = toInt(v)
		case "title":
			p.Title = str(v)
		case "views":
			p.Views = toInt(v)
		case "published":
			b, _ := v.(bool)
			p.Published = b
		case "author_id":
			p.AuthorID = toInt(v)
		default:
			return p, fmt.Errorf("unknown Post field %q", k)
		}
	}
	return p, nil
}

func toInt(v any) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	}
	return 0
}

func toFloat(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func toStrPtr(v any) *string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

// ------------------------------------------------------------- update payload

// updateData translates Prisma's update `data` into the port's assignment map,
// mapping the atomic operators onto prisma.Increment / Decrement / Multiply /
// Divide / SetTo.
func updateData(data map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(data))
	for col, v := range data {
		ops, ok := v.(map[string]any)
		if !ok {
			out[col] = jsonValue(v)
			continue
		}
		if len(ops) != 1 {
			return nil, fmt.Errorf("update %s takes exactly one operator", col)
		}
		for op, ov := range ops {
			switch op {
			case "increment":
				out[col] = prisma.Increment(jsonValue(ov))
			case "decrement":
				out[col] = prisma.Decrement(jsonValue(ov))
			case "multiply":
				out[col] = prisma.Multiply(jsonValue(ov))
			case "divide":
				out[col] = prisma.Divide(jsonValue(ov))
			case "set":
				out[col] = prisma.SetTo(jsonValue(ov))
			default:
				return nil, fmt.Errorf("unsupported update operator %q", op)
			}
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- aggregates

func runAggregate[T any](ctx context.Context, s step, model string) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	a := q.Aggregate()
	if s.Count {
		a = a.Count()
	}
	if len(s.Sum) > 0 {
		a = a.Sum(s.Sum...)
	}
	if len(s.Avg) > 0 {
		a = a.Avg(s.Avg...)
	}
	if len(s.Min) > 0 {
		a = a.Min(s.Min...)
	}
	if len(s.Max) > 0 {
		a = a.Max(s.Max...)
	}
	res, err := a.Exec(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if s.Count {
		out["_count"] = res.Count
	}
	// The port returns float64 maps and omits a key whose aggregate came back
	// NULL; Prisma always returns the key, with a null value. The emitted shape
	// is the port's own, so that difference is scored rather than hidden.
	if len(s.Sum) > 0 {
		out["_sum"] = floatMap(res.Sum)
	}
	if len(s.Avg) > 0 {
		out["_avg"] = floatMap(res.Avg)
	}
	if len(s.Min) > 0 {
		out["_min"] = floatMap(res.Min)
	}
	if len(s.Max) > 0 {
		out["_max"] = floatMap(res.Max)
	}
	return out, nil
}

func floatMap(m map[string]float64) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

// aggAlias is the column alias the port gives an aggregate term; the group-by
// rows and HAVING conditions are keyed by it.
func aggAlias(fn, col string) string {
	if fn == "_count" {
		return "_count"
	}
	return fn + "_" + col
}

func runGroupBy[T any](ctx context.Context, s step, model string) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	g := q.GroupBy(s.By...)
	if s.Count {
		g = g.Count()
	}
	if len(s.Sum) > 0 {
		g = g.Sum(s.Sum...)
	}
	if len(s.Avg) > 0 {
		g = g.Avg(s.Avg...)
	}
	if len(s.Min) > 0 {
		g = g.Min(s.Min...)
	}
	if len(s.Max) > 0 {
		g = g.Max(s.Max...)
	}
	if len(s.Having) > 0 {
		hs, err := havingConds(s.Having)
		if err != nil {
			return nil, err
		}
		g = g.Having(hs...)
	}
	rows, err := g.Exec(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, reshapeGroupRow(r))
	}
	return out, nil
}

// boolColumns lists the columns whose SQL type is BOOLEAN. GroupQuery.Exec
// scans into `any`, so it hands back whatever the driver produced — for SQLite
// that is the integer 0 or 1, because the port carries no type information into
// a grouped result the way a typed FindMany does. Restoring the bool here keeps
// the group-by cases about grouping semantics; the representation difference is
// recorded against GroupBy in COVERAGE.md instead of being smeared across every
// case that groups on a flag.
var boolColumns = map[string]bool{"published": true}

// reshapeGroupRow turns the port's flat {city, _count, _sum_views} row into
// Prisma's nested {city, _count, _sum:{views}} shape. Only key naming and the
// boolean representation above are touched; no numeric value is.
func reshapeGroupRow(r map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range r {
		v = driverValue(v)
		if boolColumns[k] {
			if n, ok := v.(int64); ok {
				v = n != 0
			}
		}
		switch {
		case k == "_count":
			out["_count"] = v
		case strings.HasPrefix(k, "_sum_"), strings.HasPrefix(k, "_avg_"),
			strings.HasPrefix(k, "_min_"), strings.HasPrefix(k, "_max_"):
			fn, col := k[:4], k[5:]
			m, _ := out[fn].(map[string]any)
			if m == nil {
				m = map[string]any{}
				out[fn] = m
			}
			m[col] = v
		default:
			out[k] = v
		}
	}
	return out
}

// havingConds translates Prisma's `having: {views: {_sum: {gt: 100}}}` into
// conditions over the port's aggregate aliases.
func havingConds(having map[string]any) ([]prisma.Condition, error) {
	cols := make([]string, 0, len(having))
	for c := range having {
		cols = append(cols, c)
	}
	sort.Strings(cols)

	var out []prisma.Condition
	for _, col := range cols {
		fns, ok := having[col].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("having %s must be an object", col)
		}
		for fn, spec := range fns {
			ops, ok := spec.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("having %s.%s must be an object", col, fn)
			}
			alias := aggAlias(fn, col)
			for op, v := range ops {
				switch op {
				case "gt":
					out = append(out, prisma.Gt(alias, jsonValue(v)))
				case "gte":
					out = append(out, prisma.Gte(alias, jsonValue(v)))
				case "lt":
					out = append(out, prisma.Lt(alias, jsonValue(v)))
				case "lte":
					out = append(out, prisma.Lte(alias, jsonValue(v)))
				case "equals":
					out = append(out, prisma.Equals(alias, jsonValue(v)))
				default:
					return nil, fmt.Errorf("unsupported having operator %q", op)
				}
			}
		}
	}
	return out, nil
}

// ------------------------------------------------------------------ find ops

type emitter[T any] func(*T, []string, []string) map[string]any

func findMany[T any](ctx context.Context, s step, model string, em emitter[T]) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	rows, err := q.FindMany(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rows))
	for i := range rows {
		out = append(out, em(&rows[i], s.Select, s.Include))
	}
	return out, nil
}

func findOne[T any](ctx context.Context, s step, model string, em emitter[T]) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	var rec T
	switch s.Op {
	case "findFirst":
		rec, err = q.FindFirst(ctx)
	case "findUnique":
		rec, err = q.FindUnique(ctx)
	case "findFirstOrThrow":
		rec, err = q.FindFirstOrThrow(ctx)
	default:
		rec, err = q.FindUniqueOrThrow(ctx)
	}
	if err != nil {
		return nil, err
	}
	return em(&rec, s.Select, s.Include), nil
}

// ---------------------------------------------------------------- write ops

func createOne[T any](ctx context.Context, s step, dec func(map[string]any) (T, error), em emitter[T]) (any, error) {
	val, err := dec(s.Data)
	if err != nil {
		return nil, err
	}
	out, err := prisma.NewQuery[T](client).Create(ctx, val)
	if err != nil {
		return nil, err
	}
	return em(&out, nil, nil), nil
}

func createMany[T any](ctx context.Context, s step, dec func(map[string]any) (T, error)) (any, error) {
	vals := make([]T, 0, len(s.Rows))
	for _, r := range s.Rows {
		v, err := dec(r)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	n, err := prisma.NewQuery[T](client).CreateMany(ctx, vals)
	if err != nil {
		return nil, err
	}
	return map[string]any{"count": n}, nil
}

func updateRows[T any](ctx context.Context, s step, model string) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	data, err := updateData(s.Data)
	if err != nil {
		return nil, err
	}
	var n int64
	if s.Op == "update" {
		n, err = q.Update(ctx, data)
	} else {
		n, err = q.UpdateMany(ctx, data)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"count": n}, nil
}

func deleteRows[T any](ctx context.Context, s step, model string) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	var n int64
	if s.Op == "delete" {
		n, err = q.Delete(ctx)
	} else {
		n, err = q.DeleteMany(ctx)
	}
	if err != nil {
		return nil, err
	}
	return map[string]any{"count": n}, nil
}

// upsertRow maps Prisma's upsert onto the port's ON CONFLICT form: the keys of
// Prisma's unique `where` become the conflict columns. The port has no place for
// the where VALUES — it only ever conflicts on what the `create` payload
// actually collides with — which is the behaviour the upsert cases probe.
func upsertRow[T any](ctx context.Context, s step, dec func(map[string]any) (T, error), model string) (any, error) {
	create, err := dec(s.Create)
	if err != nil {
		return nil, err
	}
	data, err := updateData(s.Update)
	if err != nil {
		return nil, err
	}
	conflict := make([]string, 0, len(s.Where))
	for c := range s.Where {
		conflict = append(conflict, c)
	}
	sort.Strings(conflict)

	q := prisma.NewQuery[T](client)
	q, err = applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	n, err := q.Upsert(ctx, conflict, create, data)
	if err != nil {
		return nil, err
	}
	return map[string]any{"count": n}, nil
}

// nestedCreateUser runs Prisma's `create` with a nested `posts: { create: [...] }`
// through the port's NestedCreate builder.
func nestedCreateUser(ctx context.Context, s step) (any, error) {
	parent, err := decodeUser(s.Data)
	if err != nil {
		return nil, err
	}
	n := prisma.NewQuery[User](client).NestedCreate(parent)
	if raw, ok := s.Data["posts"].(map[string]any); ok {
		list, ok := raw["create"].([]any)
		if !ok {
			return nil, fmt.Errorf("nested posts.create must be an array")
		}
		children := make([]any, 0, len(list))
		for _, c := range list {
			cm, ok := c.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("nested child must be an object")
			}
			p, err := decodePost(cm)
			if err != nil {
				return nil, err
			}
			children = append(children, p)
		}
		n = n.Create("Posts", children...)
	}
	out, err := n.Exec(ctx)
	if err != nil {
		return nil, err
	}
	return userMap(&out, nil, nil), nil
}

func countRows[T any](ctx context.Context, s step, model string) (any, error) {
	q := prisma.NewQuery[T](client)
	q, err := applyOpts(q, model, s)
	if err != nil {
		return nil, err
	}
	return q.Count(ctx)
}

// --------------------------------------------------------------- step driver

func runStep(ctx context.Context, s step) (any, error) {
	isPost := s.Model == "post"
	switch s.Op {
	case "reset":
		return nil, resetTables(ctx)
	case "seed":
		return seed(ctx, s.Dataset)

	case "dump":
		d := step{Op: "findMany", Model: s.Model, OrderBy: []map[string]any{{"id": "asc"}}}
		if isPost {
			return findMany(ctx, d, "post", postMap)
		}
		return findMany(ctx, d, "user", userMap)

	case "findMany":
		if isPost {
			return findMany(ctx, s, "post", postMap)
		}
		return findMany(ctx, s, "user", userMap)

	case "findFirst", "findUnique", "findFirstOrThrow", "findUniqueOrThrow":
		if isPost {
			return findOne(ctx, s, "post", postMap)
		}
		return findOne(ctx, s, "user", userMap)

	case "create":
		if isPost {
			return createOne(ctx, s, decodePost, postMap)
		}
		return createOne(ctx, s, decodeUser, userMap)

	case "createMany":
		if isPost {
			return createMany(ctx, s, decodePost)
		}
		return createMany(ctx, s, decodeUser)

	case "nestedCreate":
		if isPost {
			return nil, fmt.Errorf("nestedCreate is only defined for user")
		}
		return nestedCreateUser(ctx, s)

	case "update", "updateMany":
		if isPost {
			return updateRows[Post](ctx, s, "post")
		}
		return updateRows[User](ctx, s, "user")

	case "delete", "deleteMany":
		if isPost {
			return deleteRows[Post](ctx, s, "post")
		}
		return deleteRows[User](ctx, s, "user")

	case "upsert":
		if isPost {
			return upsertRow(ctx, s, decodePost, "post")
		}
		return upsertRow(ctx, s, decodeUser, "user")

	case "count":
		if isPost {
			return countRows[Post](ctx, s, "post")
		}
		return countRows[User](ctx, s, "user")

	case "aggregate":
		if isPost {
			return runAggregate[Post](ctx, s, "post")
		}
		return runAggregate[User](ctx, s, "user")

	case "groupBy":
		if isPost {
			return runGroupBy[Post](ctx, s, "post")
		}
		return runGroupBy[User](ctx, s, "user")

	default:
		return nil, fmt.Errorf("unknown op %q", s.Op)
	}
}

// safeStep runs one step, converting a panic into an ordinary error so a bug in
// the port can never take the runner down and forfeit the remaining cases.
func safeStep(ctx context.Context, s step) (v any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return runStep(ctx, s)
}

// errCode reports the port's Prisma-style code for err, or "" when it has none.
// ErrNotFound is deliberately mapped to "": it is a bare sentinel that carries
// no code, unlike the ErrRecordNotFound the *OrThrow variants return.
func errCode(err error) string {
	var pe *prisma.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func runScript(ctx context.Context, steps []step) map[string]any {
	out := make([]stepResult, 0, len(steps))
	for _, s := range steps {
		v, err := safeStep(ctx, s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "step %s: %v\n", s.Op, err)
			out = append(out, failStep(errCode(err)))
			continue
		}
		out = append(out, okStep(v))
	}
	return map[string]any{"steps": out}
}

// ------------------------------------------------------------------- driver

func main() {
	if err := openDB(); err != nil {
		fmt.Fprintf(os.Stderr, "runner failed to start: %v\n", err)
		os.Exit(1)
	}
	ctx := context.Background()

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "bad request: " + err.Error()})
			continue
		}
		resp := response{ID: req.ID}
		if req.Fn != "script" || len(req.Args) == 0 {
			resp.OK = false
			resp.Error = "unknown fn " + req.Fn
		} else {
			var steps []step
			if err := json.Unmarshal(req.Args[0], &steps); err != nil {
				resp.OK = false
				resp.Error = "bad script: " + err.Error()
			} else {
				resp.OK = true
				resp.Value = runScript(ctx, steps)
			}
		}
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "encode: %v\n", err)
			return
		}
	}
}
