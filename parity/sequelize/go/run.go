// Go parity runner for github.com/malcolmston/sequelize.
//
// Speaks JSON Lines on stdio, per parity/HARNESS.md:
//
//	in : {"id":..,"fn":..,"args":[..]}
//	out: {"id":..,"ok":true,"value":..} | {"id":..,"ok":false,"error":".."}
//
// Each case runs against a freshly built, identically seeded in-memory SQLite
// database, so cases are order-independent and mutations do not leak. The process
// is started once and streams every case. Never exits on a failing case; logs go
// to stderr only.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/malcolmston/sequelize"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
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

var dbCounter int

// schema builds and seeds a fresh in-memory database, returning the User and Post
// models. Every case gets its own so state never leaks between cases.
func schema(ctx context.Context) (*sequelize.Sequelize, *sequelize.Model, *sequelize.Model, error) {
	dbCounter++
	// A distinct shared-cache in-memory DB per case keeps them isolated.
	dsn := fmt.Sprintf("file:seqparity%d?mode=memory&cache=shared", dbCounter)
	db, err := sequelize.New("sqlite", dsn)
	if err != nil {
		return nil, nil, nil, err
	}
	User := db.Define("User", sequelize.Attributes{
		"id":     {Type: sequelize.INTEGER(), PrimaryKey: true, AutoIncrement: true},
		"name":   {Type: sequelize.STRING(64), AllowNull: sequelize.NotNull()},
		"age":    {Type: sequelize.INTEGER()},
		"email":  {Type: sequelize.STRING(128), Unique: true},
		"active": {Type: sequelize.BOOLEAN()},
		"score":  {Type: sequelize.FLOAT()},
	}, sequelize.Timestamps(false))
	Post := db.Define("Post", sequelize.Attributes{
		"id":     {Type: sequelize.INTEGER(), PrimaryKey: true, AutoIncrement: true},
		"title":  {Type: sequelize.STRING(128), AllowNull: sequelize.NotNull()},
		"userId": {Type: sequelize.INTEGER()},
	}, sequelize.Timestamps(false))

	User.HasMany(Post, sequelize.ForeignKey("userId"), sequelize.As("posts"))
	Post.BelongsTo(User, sequelize.ForeignKey("userId"), sequelize.As("user"))

	if err := db.Sync(ctx); err != nil {
		return nil, nil, nil, err
	}
	if err := User.Err(); err != nil {
		return nil, nil, nil, err
	}
	if err := Post.Err(); err != nil {
		return nil, nil, nil, err
	}

	if _, err := User.BulkCreate(ctx, []sequelize.Values{
		{"id": 1, "name": "Alice", "age": 30, "email": "alice@example.com", "active": true, "score": 4.5},
		{"id": 2, "name": "Bob", "age": 25, "email": "bob@example.com", "active": false, "score": 3.0},
		{"id": 3, "name": "Carol", "age": 35, "email": "carol@example.com", "active": true, "score": 5.0},
		{"id": 4, "name": "Dave", "age": 25, "email": nil, "active": false, "score": 2.5},
	}); err != nil {
		return nil, nil, nil, err
	}
	if _, err := Post.BulkCreate(ctx, []sequelize.Values{
		{"id": 1, "title": "Hello", "userId": 1},
		{"id": 2, "title": "World", "userId": 1},
		{"id": 3, "title": "Foo", "userId": 2},
		{"id": 4, "title": "Bar", "userId": 3},
	}); err != nil {
		return nil, nil, nil, err
	}
	return db, User, Post, nil
}

// ------------------------------------------------------------- where DSL

// buildWhere translates the language-agnostic where DSL into a *Clause.
func buildWhere(raw json.RawMessage) (*sequelize.Clause, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return buildWhereMap(m)
}

func buildWhereMap(m map[string]json.RawMessage) (*sequelize.Clause, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var clauses []*sequelize.Clause
	for _, k := range keys {
		switch k {
		case "and":
			sub, err := buildLogicalList(m[k])
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, sequelize.And(sub...))
		case "or":
			sub, err := buildLogicalList(m[k])
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, sequelize.Or(sub...))
		case "not":
			var inner map[string]json.RawMessage
			if err := json.Unmarshal(m[k], &inner); err != nil {
				return nil, err
			}
			c, err := buildWhereMap(inner)
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, sequelize.Not(c))
		default:
			c, err := buildField(k, m[k])
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, c)
		}
	}
	return sequelize.And(clauses...), nil
}

func buildLogicalList(raw json.RawMessage) ([]*sequelize.Clause, error) {
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, err
	}
	out := make([]*sequelize.Clause, 0, len(list))
	for _, m := range list {
		c, err := buildWhereMap(m)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// buildField handles one attribute condition: a scalar (eq / in / is null) or an
// object of operator keys.
func buildField(field string, raw json.RawMessage) (*sequelize.Clause, error) {
	var probe any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, err
	}
	switch v := probe.(type) {
	case nil:
		return sequelize.Op.IsNull(field), nil
	case []any:
		return sequelize.Op.In(field, v), nil
	case map[string]any:
		return buildOperators(field, v)
	default:
		return sequelize.Op.Eq(field, v), nil
	}
}

func buildOperators(field string, ops map[string]any) (*sequelize.Clause, error) {
	keys := make([]string, 0, len(ops))
	for k := range ops {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var clauses []*sequelize.Clause
	for _, op := range keys {
		val := ops[op]
		var c *sequelize.Clause
		switch op {
		case "eq":
			c = sequelize.Op.Eq(field, val)
		case "ne":
			c = sequelize.Op.Ne(field, val)
		case "gt":
			c = sequelize.Op.Gt(field, val)
		case "gte":
			c = sequelize.Op.Gte(field, val)
		case "lt":
			c = sequelize.Op.Lt(field, val)
		case "lte":
			c = sequelize.Op.Lte(field, val)
		case "like":
			c = sequelize.Op.Like(field, val.(string))
		case "notLike":
			c = sequelize.Op.NotLike(field, val.(string))
		case "in":
			c = sequelize.Op.In(field, val)
		case "notIn":
			c = sequelize.Op.NotIn(field, val)
		case "between":
			pair := val.([]any)
			c = sequelize.Op.Between(field, pair[0], pair[1])
		case "notBetween":
			pair := val.([]any)
			c = sequelize.Op.NotBetween(field, pair[0], pair[1])
		case "is":
			c = sequelize.Op.Is(field, val)
		case "not":
			if val == nil {
				c = sequelize.Op.NotNull(field)
			} else {
				c = sequelize.Op.Ne(field, val)
			}
		default:
			return nil, fmt.Errorf("unknown operator %q", op)
		}
		clauses = append(clauses, c)
	}
	return sequelize.And(clauses...), nil
}

// ------------------------------------------------------------- query DSL

type queryDSL struct {
	Where      json.RawMessage `json:"where"`
	Order      [][]string      `json:"order"`
	Limit      *int            `json:"limit"`
	Offset     *int            `json:"offset"`
	Attributes []string        `json:"attributes"`
	Include    []string        `json:"include"`
	Distinct   bool            `json:"distinct"`
}

func buildQuery(raw json.RawMessage) (sequelize.Query, error) {
	var q sequelize.Query
	if len(raw) == 0 {
		return q, nil
	}
	var dsl queryDSL
	if err := json.Unmarshal(raw, &dsl); err != nil {
		return q, err
	}
	where, err := buildWhere(dsl.Where)
	if err != nil {
		return q, err
	}
	q.Where = where
	for _, o := range dsl.Order {
		term := sequelize.OrderTerm{Attribute: o[0]}
		if len(o) > 1 && (o[1] == "DESC" || o[1] == "desc") {
			term.Descending = true
		}
		q.Order = append(q.Order, term)
	}
	q.Limit = dsl.Limit
	q.Offset = dsl.Offset
	q.Attrs = dsl.Attributes
	q.Distinct = dsl.Distinct
	for _, inc := range dsl.Include {
		q.Include = append(q.Include, sequelize.Include{Association: inc})
	}
	return q, nil
}

// ------------------------------------------------------------- normalisation

var tsKeys = map[string]bool{"createdAt": true, "updatedAt": true, "deletedAt": true}

// normalize strips timestamp keys and sorts nested association slices by id, so
// both runners emit the same JSON-comparable shape.
func normalize(v any) any {
	switch x := v.(type) {
	case sequelize.Values:
		out := map[string]any{}
		for k, val := range x {
			if tsKeys[k] {
				continue
			}
			out[k] = normalize(val)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			if tsKeys[k] {
				continue
			}
			out[k] = normalize(val)
		}
		return out
	case []sequelize.Values:
		rows := make([]sequelize.Values, len(x))
		copy(rows, x)
		sort.SliceStable(rows, func(i, j int) bool { return idOf(rows[i]) < idOf(rows[j]) })
		out := make([]any, len(rows))
		for i, r := range rows {
			out[i] = normalize(r)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	default:
		return v
	}
}

func idOf(v sequelize.Values) int64 {
	switch id := v["id"].(type) {
	case int64:
		return id
	case int:
		return int64(id)
	case float64:
		return int64(id)
	}
	return 0
}

// normalizeRows sorts by id (for a stable order) unless the caller supplied an
// explicit order.
func normalizeRows(rows []sequelize.Values, hasOrder bool) any {
	cp := make([]sequelize.Values, len(rows))
	copy(cp, rows)
	if !hasOrder {
		sort.SliceStable(cp, func(i, j int) bool { return idOf(cp[i]) < idOf(cp[j]) })
	}
	out := make([]any, len(cp))
	for i, r := range cp {
		out[i] = normalize(r)
	}
	return out
}

// ------------------------------------------------------------- dispatch

func handle(ctx context.Context, req request) (any, error) {
	db, User, Post, err := schema(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	modelFor := func(name string) *sequelize.Model {
		if name == "Post" {
			return Post
		}
		return User
	}

	switch req.Fn {
	case "createThing":
		Thing := db.Define("Thing", sequelize.Attributes{
			"id":     {Type: sequelize.INTEGER(), PrimaryKey: true, AutoIncrement: true},
			"amount": {Type: sequelize.DECIMAL(10, 2)},
		}, sequelize.Timestamps(false))
		if err := Thing.Sync(ctx); err != nil {
			return nil, err
		}
		vals, err := valuesArg(arg(req, 0))
		if err != nil {
			return nil, err
		}
		row, err := Thing.Create(ctx, vals)
		if err != nil {
			return nil, err
		}
		return normalize(row), nil

	case "findAll":
		q, err := buildQuery(arg(req, 0))
		if err != nil {
			return nil, err
		}
		rows, err := User.FindAll(ctx, q)
		if err != nil {
			return nil, err
		}
		return normalizeRows(rows, len(q.Order) > 0), nil

	case "findAllPost":
		q, err := buildQuery(arg(req, 0))
		if err != nil {
			return nil, err
		}
		rows, err := Post.FindAll(ctx, q)
		if err != nil {
			return nil, err
		}
		return normalizeRows(rows, len(q.Order) > 0), nil

	case "findOne":
		q, err := buildQuery(arg(req, 0))
		if err != nil {
			return nil, err
		}
		row, err := User.FindOne(ctx, q)
		if err != nil {
			return nil, err
		}
		return normalize(row), nil

	case "findByPk":
		var pk any
		if err := json.Unmarshal(arg(req, 0), &pk); err != nil {
			return nil, err
		}
		row, err := User.FindByPk(ctx, pk)
		if err != nil {
			return nil, err
		}
		return normalize(row), nil

	case "count":
		q, err := buildQuery(arg(req, 0))
		if err != nil {
			return nil, err
		}
		n, err := User.Count(ctx, q)
		if err != nil {
			return nil, err
		}
		return n, nil

	case "sum":
		var attr string
		if err := json.Unmarshal(arg(req, 0), &attr); err != nil {
			return nil, err
		}
		q, err := buildQuery(arg(req, 1))
		if err != nil {
			return nil, err
		}
		return User.Sum(ctx, attr, q)

	case "max":
		var attr string
		if err := json.Unmarshal(arg(req, 0), &attr); err != nil {
			return nil, err
		}
		q, err := buildQuery(arg(req, 1))
		if err != nil {
			return nil, err
		}
		return User.Max(ctx, attr, q)

	case "min":
		var attr string
		if err := json.Unmarshal(arg(req, 0), &attr); err != nil {
			return nil, err
		}
		q, err := buildQuery(arg(req, 1))
		if err != nil {
			return nil, err
		}
		return User.Min(ctx, attr, q)

	case "create":
		vals, err := valuesArg(arg(req, 0))
		if err != nil {
			return nil, err
		}
		row, err := User.Create(ctx, vals)
		if err != nil {
			return nil, err
		}
		return normalize(row), nil

	case "bulkCreate":
		var rawRows []json.RawMessage
		if err := json.Unmarshal(arg(req, 0), &rawRows); err != nil {
			return nil, err
		}
		vals := make([]sequelize.Values, 0, len(rawRows))
		for _, r := range rawRows {
			v, err := valuesArg(r)
			if err != nil {
				return nil, err
			}
			vals = append(vals, v)
		}
		rows, err := User.BulkCreate(ctx, vals)
		if err != nil {
			return nil, err
		}
		return normalizeRows(rows, false), nil

	case "update":
		vals, err := valuesArg(arg(req, 0))
		if err != nil {
			return nil, err
		}
		where, err := buildWhere(arg(req, 1))
		if err != nil {
			return nil, err
		}
		return User.Update(ctx, vals, sequelize.Query{Where: where})

	case "destroy":
		where, err := buildWhere(arg(req, 0))
		if err != nil {
			return nil, err
		}
		return User.Destroy(ctx, sequelize.Query{Where: where})

	case "increment":
		vals, err := valuesArg(arg(req, 0))
		if err != nil {
			return nil, err
		}
		where, err := buildWhere(arg(req, 1))
		if err != nil {
			return nil, err
		}
		return User.Increment(ctx, vals, sequelize.Query{Where: where})

	default:
		_ = modelFor
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
}

func arg(req request, i int) json.RawMessage {
	if i < len(req.Args) {
		return req.Args[i]
	}
	return nil
}

// valuesArg decodes a JSON object into Values, turning JSON numbers into int64
// when integral so that keys destined for integer columns round-trip cleanly.
func valuesArg(raw json.RawMessage) (sequelize.Values, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("missing values argument")
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	out := make(sequelize.Values, len(m))
	for k, v := range m {
		if f, ok := v.(float64); ok && f == float64(int64(f)) {
			out[k] = int64(f)
		} else {
			out[k] = v
		}
	}
	return out, nil
}

func main() {
	ctx := context.Background()
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<24)
	out := bufio.NewWriter(os.Stdout)
	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			fmt.Fprintln(os.Stderr, "bad request:", err)
			continue
		}
		resp := response{ID: req.ID}
		func() {
			defer func() {
				if r := recover(); r != nil {
					resp.OK = false
					resp.Error = fmt.Sprintf("panic: %v", r)
				}
			}()
			value, err := handle(ctx, req)
			if err != nil {
				resp.OK = false
				resp.Error = err.Error()
			} else {
				resp.OK = true
				resp.Value = value
			}
		}()
		b, err := json.Marshal(resp)
		if err != nil {
			b, _ = json.Marshal(response{ID: req.ID, OK: false, Error: "marshal: " + err.Error()})
		}
		out.Write(b)
		out.WriteByte('\n')
		out.Flush()
	}
}
