// Go-side parity runner for github.com/malcolmston/migrate.
//
// Speaks the same JSON-Lines protocol as parity/migrate/ruby/run.rb: one JSON
// object per line in, exactly one out, never exiting because a case failed.
//
// Two case groups are understood:
//
//	fn = "ddl"      args = [[op, ...]]
//	    Renders each op with migrate.NewSchema(migrate.SQLite), executes the
//	    emitted statements, and reports a canonical read-back of the schema.
//
//	fn = "migrate"  args = [{"migrations": [...], "script": [...]}]
//	    Registers migrations on a migrate.Migrator and drives it through a
//	    script of up/down/redo/migrate-to steps, reporting the version
//	    bookkeeping plus the schema after every step.
//
// The emitted value has the same four top-level keys as the Ruby runner. Only
// "steps", "schema" and "bookkeeping" are compared; "sql" and "raw" are
// diagnostics.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	migrate "github.com/malcolmston/migrate"
	_ "modernc.org/sqlite"
)

// ------------------------------------------------------------------ protocol

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

// op is one neutral schema operation, shared by both case groups.
type op struct {
	Op string `json:"op"`

	Table  string `json:"table"`
	Table1 string `json:"table1"`
	Table2 string `json:"table2"`
	From   string `json:"from"`
	To     string `json:"to"`
	Name   string `json:"name"`
	Column string `json:"column"`

	// create_table
	ID          *json.RawMessage `json:"id"`
	PrimaryKey  *json.RawMessage `json:"primary_key"`
	IfNotExists *bool            `json:"if_not_exists"`
	// Columns is decoded lazily: create_table uses column specs here while the
	// index ops use the same JSON key for a plain list of column names.
	Columns json.RawMessage `json:"columns"`

	// columns / indexes
	Type    string `json:"type"`
	SQLType string `json:"sql_type"`
	Unique  *bool  `json:"unique"`
	Where   string `json:"where"`
	Using   string `json:"using"`

	Limit      *int             `json:"limit"`
	Null       *bool            `json:"null"`
	Precision  *int             `json:"precision"`
	Scale      *int             `json:"scale"`
	Default    *json.RawMessage `json:"default"`
	DefaultRaw *string          `json:"default_raw"`
	Values     []string         `json:"values"`

	// references / foreign keys
	Index       *bool  `json:"index"`
	ForeignKey  *bool  `json:"foreign_key"`
	Polymorphic *bool  `json:"polymorphic"`
	ToTable     string `json:"to_table"`
	OnDelete    string `json:"on_delete"`
	OnUpdate    string `json:"on_update"`

	Expression string `json:"expression"`
	SQL        string `json:"sql"`

	// change_table
	Changes []changeSpec `json:"changes"`
}

// changeSpec is one entry of a change_table bulk-alteration block.
type changeSpec struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	From    string   `json:"from"`
	To      string   `json:"to"`
	Names   []string `json:"names"`
	Columns []string `json:"columns"`
	Unique  *bool    `json:"unique"`
	SQLType string   `json:"sql_type"`

	Limit      *int             `json:"limit"`
	Null       *bool            `json:"null"`
	Default    *json.RawMessage `json:"default"`
	DefaultRaw *string          `json:"default_raw"`
}

// colSpec is one column inside a create_table block.
type colSpec struct {
	Type string `json:"type"`
	Name string `json:"name"`

	Limit      *int             `json:"limit"`
	Null       *bool            `json:"null"`
	Precision  *int             `json:"precision"`
	Scale      *int             `json:"scale"`
	Default    *json.RawMessage `json:"default"`
	DefaultRaw *string          `json:"default_raw"`
	PrimaryKey *bool            `json:"primary_key"`
	Array      *bool            `json:"array"`
	Values     []string         `json:"values"`

	Index       *bool  `json:"index"`
	ForeignKey  *bool  `json:"foreign_key"`
	Polymorphic *bool  `json:"polymorphic"`
	ToTable     string `json:"to_table"`

	Expression string `json:"expression"`
}

// indexColumns tolerates both "columns" spellings used by the case files: an
// index op names its columns in "columns", a create_table op uses the same key
// for column specs, so the raw JSON is decoded twice.
type opColumns struct {
	Columns []json.RawMessage `json:"columns"`
}

// strings keeps only the elements that are JSON strings, so the same key can
// hold column names (index ops) or column specs (create_table).
func (o opColumns) strings() []string {
	out := []string{}
	for _, raw := range o.Columns {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// ------------------------------------------------------------------ helpers

var schema = migrate.NewSchema(migrate.SQLite)

// splitStmts splits the ";\n"-joined multi-statement output of the schema DSL.
func splitStmts(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ";\n") {
		if p := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part), ";")); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normSpace(s string) string {
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(s, " "))
}

// jsonScalar decodes a JSON default into the Go value migrate.Default expects,
// collapsing whole floats to int so 0 renders as "0" and not "0e+00".
func jsonScalar(raw *json.RawMessage) (any, error) {
	if raw == nil {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(*raw, &v); err != nil {
		return nil, err
	}
	if f, ok := v.(float64); ok && f == float64(int64(f)) {
		return int64(f), nil
	}
	return v, nil
}

func colOptions(c colSpec) ([]migrate.ColumnOption, error) {
	var opts []migrate.ColumnOption
	if c.Limit != nil {
		opts = append(opts, migrate.Limit(*c.Limit))
	}
	if c.Null != nil && !*c.Null {
		opts = append(opts, migrate.NotNull())
	}
	if c.Precision != nil {
		opts = append(opts, migrate.Precision(*c.Precision))
	}
	if c.Scale != nil {
		opts = append(opts, migrate.Scale(*c.Scale))
	}
	if c.Default != nil {
		v, err := jsonScalar(c.Default)
		if err != nil {
			return nil, err
		}
		opts = append(opts, migrate.Default(v))
	}
	if c.DefaultRaw != nil {
		opts = append(opts, migrate.DefaultRaw(*c.DefaultRaw))
	}
	if c.PrimaryKey != nil && *c.PrimaryKey {
		opts = append(opts, migrate.PrimaryKey())
	}
	if c.Array != nil && *c.Array {
		opts = append(opts, migrate.Array())
	}
	return opts, nil
}

func refOptions(c colSpec) []migrate.ReferenceOption {
	var opts []migrate.ReferenceOption
	if c.Index != nil && *c.Index {
		opts = append(opts, migrate.ReferenceIndex())
	}
	if c.ForeignKey != nil && *c.ForeignKey {
		opts = append(opts, migrate.WithForeignKey())
	}
	if c.Polymorphic != nil && *c.Polymorphic {
		opts = append(opts, migrate.Polymorphic())
	}
	if c.Null != nil && !*c.Null {
		opts = append(opts, migrate.ReferenceNotNull())
	}
	if c.ToTable != "" {
		opts = append(opts, migrate.ReferenceTable(c.ToTable))
	}
	return opts
}

// addColumnTo places one neutral column spec on the create_table builder using
// the port's typed DSL methods, so the port's own type mapping is what gets
// measured.
func addColumnTo(t *migrate.Table, c colSpec) error {
	opts, err := colOptions(c)
	if err != nil {
		return err
	}
	switch c.Type {
	case "string":
		t.String(c.Name, opts...)
	case "text":
		t.Text(c.Name, opts...)
	case "integer":
		t.Integer(c.Name, opts...)
	case "bigint":
		t.BigInteger(c.Name, opts...)
	case "float":
		t.Float(c.Name, opts...)
	case "decimal":
		p, s := 0, 0
		if c.Precision != nil {
			p = *c.Precision
		}
		if c.Scale != nil {
			s = *c.Scale
		}
		t.Decimal(c.Name, p, s, opts...)
	case "boolean":
		t.Boolean(c.Name, opts...)
	case "date":
		t.Date(c.Name, opts...)
	case "time":
		t.Time(c.Name, opts...)
	case "datetime":
		t.Timestamp(c.Name, opts...)
	case "binary":
		t.Binary(c.Name, opts...)
	case "json":
		t.JSON(c.Name, opts...)
	case "jsonb":
		t.JSONB(c.Name, opts...)
	case "uuid":
		t.UUID(c.Name, opts...)
	case "enum":
		t.Enum(c.Name, c.Values, opts...)
	case "timestamps":
		t.Timestamps()
	case "references":
		t.References(c.Name, refOptions(c)...)
	case "check":
		return errors.New("migrate: Table has no check-constraint builder")
	default:
		return fmt.Errorf("unknown column type %q", c.Type)
	}
	return nil
}

// applyChange dispatches one change_table entry onto the port's AlterTable.
func applyChange(a *migrate.AlterTable, ch changeSpec) error {
	opts, err := colOptions(colSpec{Limit: ch.Limit, Null: ch.Null, Default: ch.Default, DefaultRaw: ch.DefaultRaw})
	if err != nil {
		return err
	}
	switch ch.Kind {
	case "column":
		switch ch.Type {
		case "string":
			a.String(ch.Name, opts...)
		case "text":
			a.Text(ch.Name, opts...)
		case "integer":
			a.Integer(ch.Name, opts...)
		case "bigint":
			a.BigInteger(ch.Name, opts...)
		case "boolean":
			a.Boolean(ch.Name, opts...)
		case "datetime":
			a.Timestamp(ch.Name, opts...)
		default:
			if ch.SQLType == "" {
				return fmt.Errorf("migrate: AlterTable has no %q method", ch.Type)
			}
			a.Column(ch.Name, ch.SQLType, opts...)
		}
	case "change":
		if ch.SQLType == "" {
			return errors.New("migrate: AlterTable.Change takes a raw SQL type")
		}
		a.Change(ch.Name, ch.SQLType, opts...)
	case "remove":
		a.Remove(ch.Names...)
	case "rename":
		a.Rename(ch.From, ch.To)
	case "index":
		var iopts []migrate.IndexOption
		if ch.Unique != nil && *ch.Unique {
			iopts = append(iopts, migrate.UniqueIndex())
		}
		a.Index(ch.Columns, iopts...)
	case "remove_index":
		a.RemoveIndex(ch.Name)
	case "timestamps":
		a.Timestamps()
	case "references":
		a.References(ch.Name)
	default:
		return fmt.Errorf("unknown change_table entry %q", ch.Kind)
	}
	return nil
}

func indexOptions(o op) []migrate.IndexOption {
	var opts []migrate.IndexOption
	if o.Unique != nil && *o.Unique {
		opts = append(opts, migrate.UniqueIndex())
	}
	if o.Name != "" {
		opts = append(opts, migrate.IndexName(o.Name))
	}
	if o.Where != "" {
		opts = append(opts, migrate.Where(o.Where))
	}
	if o.Using != "" {
		opts = append(opts, migrate.Using(o.Using))
	}
	return opts
}

func fkOptions(o op) []migrate.ForeignKeyOption {
	var opts []migrate.ForeignKeyOption
	if o.Column != "" {
		opts = append(opts, migrate.FKColumn(o.Column))
	}
	if o.Name != "" {
		opts = append(opts, migrate.FKName(o.Name))
	}
	if o.OnDelete != "" {
		opts = append(opts, migrate.OnDelete(refAction(o.OnDelete)))
	}
	if o.OnUpdate != "" {
		opts = append(opts, migrate.OnUpdate(refAction(o.OnUpdate)))
	}
	return opts
}

func refAction(s string) migrate.ReferentialAction {
	switch strings.ToLower(s) {
	case "cascade":
		return migrate.Cascade
	case "nullify", "set_null":
		return migrate.SetNull
	case "restrict":
		return migrate.Restrict
	case "set_default":
		return migrate.SetDefault
	default:
		return migrate.NoAction
	}
}

func tableOptions(o op) ([]migrate.TableOption, error) {
	var opts []migrate.TableOption
	if o.ID != nil {
		var v any
		if err := json.Unmarshal(*o.ID, &v); err != nil {
			return nil, err
		}
		if b, ok := v.(bool); ok && !b {
			opts = append(opts, migrate.WithoutID())
		} else {
			return nil, errors.New("migrate: CreateTable has no option to rename or retype the auto primary key")
		}
	}
	if o.PrimaryKey != nil {
		return nil, errors.New("migrate: CreateTable has no primary_key option")
	}
	if o.IfNotExists != nil && *o.IfNotExists {
		opts = append(opts, migrate.IfNotExists())
	}
	return opts, nil
}

// defaultIndexName reproduces the ActiveRecord "index_<table>_on_<cols>"
// convention, which the port's AddIndex also uses. The port's DropIndex takes a
// name only, so a "remove index by columns" op has to derive it here.
func defaultIndexName(table string, cols []string) string {
	return "index_" + table + "_on_" + strings.Join(cols, "_and_")
}

// ------------------------------------------------------------- op rendering

// render turns one neutral op into the SQL statement(s) the port emits for it.
func render(o op, cols []string) ([]string, error) {
	switch o.Op {
	case "create_table":
		opts, err := tableOptions(o)
		if err != nil {
			return nil, err
		}
		var specs []colSpec
		if len(o.Columns) > 0 {
			if err := json.Unmarshal(o.Columns, &specs); err != nil {
				return nil, fmt.Errorf("create_table columns: %w", err)
			}
		}
		var buildErr error
		stmt := schema.CreateTable(o.Table, func(t *migrate.Table) {
			for _, c := range specs {
				if err := addColumnTo(t, c); err != nil && buildErr == nil {
					buildErr = err
				}
			}
		}, opts...)
		if buildErr != nil {
			return nil, buildErr
		}
		return splitStmts(stmt), nil
	case "drop_table":
		return splitStmts(schema.DropTable(o.Table)), nil
	case "rename_table":
		return splitStmts(schema.RenameTable(o.From, o.To)), nil
	case "change_table":
		var buildErr error
		stmt := schema.ChangeTable(o.Table, func(a *migrate.AlterTable) {
			for _, ch := range o.Changes {
				if err := applyChange(a, ch); err != nil && buildErr == nil {
					buildErr = err
				}
			}
		})
		if buildErr != nil {
			return nil, buildErr
		}
		return splitStmts(stmt), nil
	case "add_column":
		c := colSpec{Type: o.Type, Name: o.Name, Limit: o.Limit, Null: o.Null,
			Precision: o.Precision, Scale: o.Scale, Default: o.Default, DefaultRaw: o.DefaultRaw}
		opts, err := colOptions(c)
		if err != nil {
			return nil, err
		}
		// AlterTable carries the abstract type vocabulary for ALTER TABLE; only
		// a subset of the create_table types is available there.
		var stmt string
		var unsupported bool
		stmt = schema.ChangeTable(o.Table, func(a *migrate.AlterTable) {
			switch o.Type {
			case "string":
				a.String(o.Name, opts...)
			case "text":
				a.Text(o.Name, opts...)
			case "integer":
				a.Integer(o.Name, opts...)
			case "bigint":
				a.BigInteger(o.Name, opts...)
			case "boolean":
				a.Boolean(o.Name, opts...)
			case "datetime":
				a.Timestamp(o.Name, opts...)
			default:
				unsupported = true
			}
		})
		if unsupported {
			if o.SQLType == "" {
				return nil, fmt.Errorf("migrate: AlterTable has no %q method; add_column needs an explicit SQL type spelling", o.Type)
			}
			return splitStmts(schema.AddColumn(o.Table, o.Name, o.SQLType, opts...)), nil
		}
		return splitStmts(stmt), nil
	case "remove_column":
		return splitStmts(schema.DropColumn(o.Table, o.Name)), nil
	case "change_column":
		if o.SQLType == "" {
			return nil, errors.New("migrate: ChangeColumn takes a raw SQL type; the port has no abstract change_column")
		}
		c := colSpec{Null: o.Null, Default: o.Default, DefaultRaw: o.DefaultRaw, Limit: o.Limit}
		opts, err := colOptions(c)
		if err != nil {
			return nil, err
		}
		return splitStmts(schema.ChangeColumn(o.Table, o.Name, o.SQLType, opts...)), nil
	case "change_column_null":
		null := true
		if o.Null != nil {
			null = *o.Null
		}
		return splitStmts(schema.ChangeColumnNull(o.Table, o.Column, null)), nil
	case "change_column_default":
		if o.DefaultRaw != nil {
			return splitStmts(schema.ChangeColumnDefaultRaw(o.Table, o.Column, *o.DefaultRaw)), nil
		}
		v, err := jsonScalar(o.Default)
		if err != nil {
			return nil, err
		}
		if v == nil {
			return splitStmts(schema.DropColumnDefault(o.Table, o.Column)), nil
		}
		return splitStmts(schema.ChangeColumnDefault(o.Table, o.Column, v)), nil
	case "rename_column":
		return splitStmts(schema.RenameColumn(o.Table, o.From, o.To)), nil
	case "add_index":
		return splitStmts(schema.AddIndex(o.Table, cols, indexOptions(o)...)), nil
	case "remove_index":
		name := o.Name
		if name == "" {
			name = defaultIndexName(o.Table, cols)
		}
		return splitStmts(schema.DropIndex(name)), nil
	case "rename_index":
		return splitStmts(schema.RenameIndex(o.Table, o.From, o.To)), nil
	case "add_timestamps":
		return splitStmts(schema.AddTimestamps(o.Table)), nil
	case "remove_timestamps":
		return splitStmts(schema.RemoveTimestamps(o.Table)), nil
	case "add_reference":
		c := colSpec{Index: o.Index, ForeignKey: o.ForeignKey, Polymorphic: o.Polymorphic,
			Null: o.Null, ToTable: o.ToTable}
		return splitStmts(schema.AddReference(o.Table, o.Name, refOptions(c)...)), nil
	case "remove_reference":
		c := colSpec{Index: o.Index, ForeignKey: o.ForeignKey, Polymorphic: o.Polymorphic, ToTable: o.ToTable}
		return splitStmts(schema.RemoveReference(o.Table, o.Name, refOptions(c)...)), nil
	case "add_foreign_key":
		return splitStmts(schema.AddForeignKey(o.From, o.To, fkOptions(o)...)), nil
	case "remove_foreign_key":
		return splitStmts(schema.RemoveForeignKey(o.From, o.To, fkOptions(o)...)), nil
	case "add_check_constraint":
		var opts []migrate.ConstraintOption
		if o.Name != "" {
			opts = append(opts, migrate.ConstraintName(o.Name))
		}
		return splitStmts(schema.AddCheckConstraint(o.Table, o.Expression, opts...)), nil
	case "remove_check_constraint":
		var opts []migrate.ConstraintOption
		if o.Name != "" {
			opts = append(opts, migrate.ConstraintName(o.Name))
		}
		return splitStmts(schema.RemoveCheckConstraint(o.Table, o.Expression, opts...)), nil
	case "add_unique_constraint":
		var opts []migrate.ConstraintOption
		if o.Name != "" {
			opts = append(opts, migrate.ConstraintName(o.Name))
		}
		return splitStmts(schema.AddUniqueConstraint(o.Table, cols, opts...)), nil
	case "create_join_table":
		return splitStmts(schema.CreateJoinTable(o.Table1, o.Table2, nil)), nil
	case "drop_join_table":
		return splitStmts(schema.DropJoinTable(o.Table1, o.Table2)), nil
	case "truncate_table":
		return splitStmts(schema.TruncateTable(o.Table)), nil
	case "execute", "insert":
		return []string{o.SQL}, nil
	case "raw_sql":
		// Only meaningful inside a migration, where the port hands the whole
		// script to its own statement splitter.
		return []string{o.SQL}, nil
	default:
		return nil, fmt.Errorf("unknown op %q", o.Op)
	}
}

// ---------------------------------------------------------- schema readback

type column struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	NotNull bool    `json:"notnull"`
	Default *string `json:"default"`
	PK      int     `json:"pk"`
}

type index struct {
	Name    string   `json:"name"`
	Raw     string   `json:"raw"`
	Unique  bool     `json:"unique"`
	Partial bool     `json:"partial"`
	Origin  string   `json:"origin"`
	Columns []string `json:"columns"`
}

type foreignKey struct {
	Table    string  `json:"table"`
	From     string  `json:"from"`
	To       *string `json:"to"`
	OnDelete string  `json:"on_delete"`
	OnUpdate string  `json:"on_update"`
}

type check struct {
	Name string  `json:"name"`
	Raw  *string `json:"raw"`
	Expr string  `json:"expr"`
}

type table struct {
	Name        string       `json:"name"`
	Columns     []column     `json:"columns"`
	Indexes     []index      `json:"indexes"`
	ForeignKeys []foreignKey `json:"foreign_keys"`
	Checks      []check      `json:"checks"`
}

type briefTable struct {
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

var bookkeepingTables = map[string]bool{"schema_migrations": true, "ar_internal_metadata": true}

var wsRe = regexp.MustCompile(`\s+`)

func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func tableNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		if !bookkeepingTables[n] {
			out = append(out, n)
		}
	}
	return out, rows.Err()
}

func tableSQL(db *sql.DB, name string) string {
	var s sql.NullString
	_ = db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&s)
	return s.String
}

func normType(t string) string {
	return strings.ToUpper(normSpace(t))
}

func readColumns(db *sql.DB, t string) ([]column, error) {
	rows, err := db.Query(`SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, t)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []column{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var def sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); err != nil {
			return nil, err
		}
		c := column{Name: name, Type: normType(typ), NotNull: notnull != 0, PK: pk}
		// SQLite forbids a null rowid, so an INTEGER PRIMARY KEY is NOT NULL
		// whether or not the DDL said so. Normalised on both sides.
		if pk > 0 && c.Type == "INTEGER" {
			c.NotNull = true
		}
		if def.Valid {
			v := def.String
			c.Default = &v
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func indexShape(name, t string, cols []string) string {
	if name == defaultIndexName(t, cols) {
		return "<default>"
	}
	if strings.HasPrefix(name, "sqlite_autoindex_") {
		return "<autoindex>"
	}
	return name
}

func readIndexes(db *sql.DB, t string) ([]index, error) {
	rows, err := db.Query(`SELECT seq, name, "unique", origin, partial FROM pragma_index_list(?)`, t)
	if err != nil {
		return nil, err
	}
	type raw struct {
		name          string
		uniq, partial int
		origin        string
	}
	var raws []raw
	for rows.Next() {
		var r raw
		var seq int
		if err := rows.Scan(&seq, &r.name, &r.uniq, &r.origin, &r.partial); err != nil {
			_ = rows.Close()
			return nil, err
		}
		raws = append(raws, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := []index{}
	for _, r := range raws {
		irows, err := db.Query(`SELECT seqno, cid, name FROM pragma_index_info(?) ORDER BY seqno`, r.name)
		if err != nil {
			return nil, err
		}
		cols := []string{}
		for irows.Next() {
			var seqno, cid int
			var cn sql.NullString
			if err := irows.Scan(&seqno, &cid, &cn); err != nil {
				_ = irows.Close()
				return nil, err
			}
			if cn.Valid {
				cols = append(cols, cn.String)
			} else {
				cols = append(cols, "<expr>")
			}
		}
		_ = irows.Close()
		out = append(out, index{
			Name: indexShape(r.name, t, cols), Raw: r.name,
			Unique: r.uniq != 0, Partial: r.partial != 0, Origin: r.origin, Columns: cols,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		ac, bc := strings.Join(a.Columns, ","), strings.Join(b.Columns, ",")
		if ac != bc {
			return ac < bc
		}
		return a.Raw < b.Raw
	})
	return out, nil
}

func readForeignKeys(db *sql.DB, t string) ([]foreignKey, error) {
	rows, err := db.Query(`SELECT id, seq, "table", "from", "to", on_update, on_delete FROM pragma_foreign_key_list(?)`, t)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []foreignKey{}
	for rows.Next() {
		var id, seq int
		var tbl, from, onUpd, onDel string
		var to sql.NullString
		if err := rows.Scan(&id, &seq, &tbl, &from, &to, &onUpd, &onDel); err != nil {
			return nil, err
		}
		fk := foreignKey{Table: tbl, From: from, OnDelete: strings.ToUpper(onDel), OnUpdate: strings.ToUpper(onUpd)}
		if to.Valid {
			v := to.String
			fk.To = &v
		}
		out = append(out, fk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Table != out[j].Table {
			return out[i].Table < out[j].Table
		}
		return out[i].From < out[j].From
	})
	return out, nil
}

var checkRe = regexp.MustCompile(`(?i)CHECK\s*\(`)
var constraintTailRe = regexp.MustCompile(`(?i)CONSTRAINT\s+("([^"]+)"|[A-Za-z_][A-Za-z0-9_]*)\s*$`)

// readChecks extracts CHECK(...) bodies from a CREATE TABLE statement by paren
// matching; SQLite exposes no pragma for check constraints.
func readChecks(createSQL string) []check {
	out := []check{}
	i := 0
	for i < len(createSQL) {
		loc := checkRe.FindStringIndex(createSQL[i:])
		if loc == nil {
			break
		}
		start := i + loc[0]
		open := i + loc[1] - 1
		depth, j := 0, open
		for ; j < len(createSQL); j++ {
			switch createSQL[j] {
			case '(':
				depth++
			case ')':
				depth--
			}
			if depth == 0 {
				break
			}
		}
		if j >= len(createSQL) {
			break
		}
		body := createSQL[open+1 : j]
		c := check{Name: "<auto>", Expr: normSpace(strings.ReplaceAll(body, `"`, ""))}
		if m := constraintTailRe.FindStringSubmatch(createSQL[:start]); m != nil {
			n := m[2]
			if n == "" {
				n = m[1]
			}
			c.Raw = &n
		}
		out = append(out, c)
		i = j + 1
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Expr < out[b].Expr })
	return out
}

func snapshot(db *sql.DB) (map[string]any, error) {
	names, err := tableNames(db)
	if err != nil {
		return nil, err
	}
	tables := []table{}
	for _, n := range names {
		cols, err := readColumns(db, n)
		if err != nil {
			return nil, err
		}
		idx, err := readIndexes(db, n)
		if err != nil {
			return nil, err
		}
		fks, err := readForeignKeys(db, n)
		if err != nil {
			return nil, err
		}
		tables = append(tables, table{
			Name: n, Columns: cols, Indexes: idx, ForeignKeys: fks,
			Checks: readChecks(tableSQL(db, n)),
		})
	}
	return map[string]any{"tables": tables}, nil
}

func brief(db *sql.DB) []briefTable {
	names, err := tableNames(db)
	if err != nil {
		return []briefTable{}
	}
	out := []briefTable{}
	for _, n := range names {
		cols, err := readColumns(db, n)
		if err != nil {
			continue
		}
		cn := []string{}
		for _, c := range cols {
			cn = append(cn, c.Name)
		}
		out = append(out, briefTable{Name: n, Columns: cn})
	}
	return out
}

func rawDiagnostics(db *sql.DB) map[string]any {
	names, _ := tableNames(db)
	sqls := map[string]string{}
	nm := map[string]any{}
	for _, n := range names {
		sqls[n] = normSpace(tableSQL(db, n))
		idx, _ := readIndexes(db, n)
		ir := []string{}
		for _, i := range idx {
			ir = append(ir, i.Raw)
		}
		cr := []any{}
		for _, c := range readChecks(tableSQL(db, n)) {
			if c.Raw == nil {
				cr = append(cr, nil)
			} else {
				cr = append(cr, *c.Raw)
			}
		}
		nm[n] = map[string]any{"indexes": ir, "checks": cr}
	}
	return map[string]any{"names": nm, "tables_sql": sqls}
}

func bookkeepingShape(db *sql.DB) map[string]any {
	cols, err := readColumns(db, "schema_migrations")
	shape := []map[string]any{}
	if err == nil {
		for _, c := range cols {
			shape = append(shape, map[string]any{
				"name": c.Name, "type": c.Type, "notnull": c.NotNull, "pk": c.PK,
			})
		}
	}
	return map[string]any{"table": "schema_migrations", "columns": shape}
}

// ------------------------------------------------------------------ database

// withDB opens a fresh SQLite database in its own temp directory, runs fn, and
// removes the directory again so no case can observe another's state.
func withDB(fn func(db *sql.DB) (any, error)) (any, error) {
	dir, err := os.MkdirTemp("", "migrate-parity-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	db, err := sql.Open("sqlite", filepath.Join(dir, "parity.sqlite3")+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	return fn(db)
}

// ------------------------------------------------------------- group 1: DDL

func decodeOps(raw json.RawMessage) ([]op, [][]string, error) {
	var ops []op
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, nil, err
	}
	var raws []opColumns
	if err := json.Unmarshal(raw, &raws); err != nil {
		return nil, nil, err
	}
	cols := make([][]string, len(ops))
	for i := range raws {
		cols[i] = raws[i].strings()
	}
	return ops, cols, nil
}

func runDDL(raw json.RawMessage) (any, error) {
	ops, colLists, err := decodeOps(raw)
	if err != nil {
		return nil, err
	}
	return withDB(func(db *sql.DB) (any, error) {
		steps := []map[string]any{}
		errs := []map[string]any{}
		emitted := []string{}
		ctx := context.Background()
		for i, o := range ops {
			ok, errText := true, ""
			stmts, rerr := render(o, colLists[i])
			if rerr != nil {
				ok, errText = false, rerr.Error()
			} else {
				for _, s := range stmts {
					emitted = append(emitted, normSpace(s))
					if _, eerr := db.ExecContext(ctx, s); eerr != nil {
						ok, errText = false, eerr.Error()
						break
					}
				}
			}
			steps = append(steps, map[string]any{"i": i, "op": o.Op, "ok": ok})
			if !ok {
				errs = append(errs, map[string]any{"i": i, "error": errText})
			}
		}
		snap, err := snapshot(db)
		if err != nil {
			return nil, err
		}
		diag := rawDiagnostics(db)
		diag["step_errors"] = errs
		return map[string]any{"steps": steps, "schema": snap, "sql": emitted, "raw": diag}, nil
	})
}

// --------------------------------------------------- group 2: migration state

type migSpec struct {
	Version uint64 `json:"version"`
	Name    string `json:"name"`
	Up      []op   `json:"up"`
	Down    []op   `json:"down"`

	upCols   [][]string
	downCols [][]string
	hasDown  bool
}

type migrateSpec struct {
	Migrations        []json.RawMessage `json:"migrations"`
	ReportBookkeeping bool              `json:"report_bookkeeping"`
	Script            []struct {
		Op      string `json:"op"`
		N       int    `json:"n"`
		Version uint64 `json:"version"`
	} `json:"script"`
}

// buildMigration turns a neutral migration spec into a migrate.Migration. A
// direction that is a single "raw_sql" op is handed over as UpSQL/DownSQL so the
// port's own statement splitter is exercised; anything else becomes a Go
// direction that renders and executes each op in turn.
func buildMigration(m migSpec, sink *[]string) migrate.Migration {
	out := migrate.Migration{Version: m.Version, Name: m.Name}

	dir := func(ops []op, cols [][]string, present bool) (migrate.MigrateFunc, string) {
		if len(ops) == 1 && ops[0].Op == "raw_sql" {
			return nil, ops[0].SQL
		}
		if len(ops) == 0 && !present {
			return nil, ""
		}
		return func(ctx context.Context, tx *sql.Tx) error {
			for i, o := range ops {
				stmts, err := render(o, cols[i])
				if err != nil {
					return err
				}
				for _, s := range stmts {
					*sink = append(*sink, normSpace(s))
					if _, err := tx.ExecContext(ctx, s); err != nil {
						return fmt.Errorf("exec %q: %w", normSpace(s), err)
					}
				}
			}
			return nil
		}, ""
	}

	out.Up, out.UpSQL = dir(m.Up, m.upCols, true)
	out.Down, out.DownSQL = dir(m.Down, m.downCols, m.hasDown)
	if out.UpSQL != "" {
		*sink = append(*sink, normSpace(out.UpSQL))
	}
	return out
}

// migrationStatus projects Migrator.Status into the same neutral shape as
// ActiveRecord's migrations_status: {version, applied}. Note that the port's
// MigrationStatus carries no failed/dirty flag for this projection to lose.
func migrationStatus(ctx context.Context, mg *migrate.Migrator) []map[string]any {
	out := []map[string]any{}
	sts, err := mg.Status(ctx)
	if err != nil {
		return out
	}
	for _, st := range sts {
		out = append(out, map[string]any{"version": st.Version, "applied": st.Applied})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i]["version"].(uint64) < out[j]["version"].(uint64)
	})
	return out
}

func appliedVersions(db *sql.DB) []uint64 {
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version ASC`)
	if err != nil {
		return []uint64{}
	}
	defer func() { _ = rows.Close() }()
	out := []uint64{}
	for rows.Next() {
		var v uint64
		if err := rows.Scan(&v); err != nil {
			return out
		}
		out = append(out, v)
	}
	return out
}

func runMigrate(raw json.RawMessage) (any, error) {
	var spec migrateSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return nil, err
	}
	return withDB(func(db *sql.DB) (any, error) {
		ctx := context.Background()
		emitted := []string{}
		mg := migrate.New(db)
		for _, rawMig := range spec.Migrations {
			var m migSpec
			if err := json.Unmarshal(rawMig, &m); err != nil {
				return nil, err
			}
			var holder struct {
				Up   json.RawMessage `json:"up"`
				Down json.RawMessage `json:"down"`
			}
			if err := json.Unmarshal(rawMig, &holder); err != nil {
				return nil, err
			}
			if len(holder.Up) > 0 {
				_, c, err := decodeOps(holder.Up)
				if err != nil {
					return nil, err
				}
				m.upCols = c
			}
			m.hasDown = len(holder.Down) > 0
			if len(holder.Down) > 0 {
				_, c, err := decodeOps(holder.Down)
				if err != nil {
					return nil, err
				}
				m.downCols = c
			}
			if err := mg.Register(buildMigration(m, &emitted)); err != nil {
				return nil, err
			}
		}

		steps := []map[string]any{}
		errs := []map[string]any{}
		for i, st := range spec.Script {
			var err error
			switch st.Op {
			case "up":
				err = mg.Migrate(ctx)
			case "up_n":
				err = mg.Up(ctx, st.N)
			case "rollback":
				n := st.N
				if n == 0 {
					n = 1
				}
				err = mg.Rollback(ctx, n)
			case "down_all":
				err = mg.MigrateTo(ctx, 0)
			case "migrate_to":
				err = mg.MigrateTo(ctx, st.Version)
			case "redo":
				err = mg.Redo(ctx)
			case "version", "status":
				_, err = mg.Version(ctx)
			default:
				err = fmt.Errorf("unknown migrate step %q", st.Op)
			}
			applied := appliedVersions(db)
			var max uint64
			for _, v := range applied {
				if v > max {
					max = v
				}
			}
			steps = append(steps, map[string]any{
				"i": i, "op": st.Op, "ok": err == nil,
				"version": max, "applied": applied,
				"status": migrationStatus(ctx, mg), "tables": brief(db),
			})
			if err != nil {
				errs = append(errs, map[string]any{"i": i, "error": err.Error()})
			}
		}

		snap, err := snapshot(db)
		if err != nil {
			return nil, err
		}
		diag := rawDiagnostics(db)
		diag["step_errors"] = errs
		diag["bookkeeping"] = bookkeepingShape(db)
		value := map[string]any{"steps": steps, "schema": snap, "sql": emitted, "raw": diag}
		if spec.ReportBookkeeping {
			value["bookkeeping"] = bookkeepingShape(db)
		}
		return value, nil
	})
}

// ------------------------------------------------------------------- main

func handle(req request) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	if len(req.Args) == 0 {
		return nil, errors.New("missing args")
	}
	switch req.Fn {
	case "ddl":
		return runDDL(req.Args[0])
	case "migrate":
		return runMigrate(req.Args[0])
	default:
		return nil, fmt.Errorf("unknown fn %q", req.Fn)
	}
}

func main() {
	// Deterministic clocks are irrelevant here: applied_at values are never
	// compared, only their presence.
	_ = time.Now
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 1<<20), 1<<26)
	out := bufio.NewWriter(os.Stdout)
	enc := json.NewEncoder(out)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{ID: "?", OK: false, Error: "bad request: " + err.Error()})
			_ = out.Flush()
			continue
		}
		value, err := handle(req)
		if err != nil {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: err.Error()})
		} else {
			_ = enc.Encode(response{ID: req.ID, OK: true, Value: value})
		}
		_ = out.Flush()
	}
}
