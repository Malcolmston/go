// Command prisma-example exercises github.com/malcolmston/prisma end to end
// against a real, throwaway SQLite database created in a temp directory. No
// external database server and no network access are required.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malcolmston/prisma"
	_ "modernc.org/sqlite" // pure-Go, cgo-free SQLite driver
)

// ---------------------------------------------------------------- models

// User is a model: plain struct + `prisma` struct tags. Table name defaults to
// the pluralized snake_case of the type name ("users").
type User struct {
	ID      int64          `prisma:"col=id,pk,auto"`
	Name    string         `prisma:"col=name"`
	Email   string         `prisma:"col=email"`
	Age     int64          `prisma:"col=age"`
	City    sql.NullString `prisma:"col=city"`
	Profile string         `prisma:"col=profile"` // JSON document
	Secret  string         `prisma:"-"`           // never mapped to a column
	Posts   []Post         `prisma:"relation,fk=author_id,references=id"`
}

// Post has both a to-one relation (Author, eager-loadable with Include) and a
// foreign-key column.
type Post struct {
	ID       int64  `prisma:"col=id,pk,auto"`
	Title    string `prisma:"col=title"`
	Status   string `prisma:"col=status"`
	Views    int64  `prisma:"col=views"`
	AuthorID int64  `prisma:"col=author_id"`
	Author   *User  `prisma:"relation,fk=author_id,references=id"`
}

// AuditLog overrides its table name through the TableNamer interface.
type AuditLog struct {
	ID     int64  `prisma:"col=id,pk,auto"`
	Action string `prisma:"col=action"`
}

func (AuditLog) TableName() string { return "audit_trail" }

// ---------------------------------------------------------------- main

func main() {
	ctx := context.Background()

	dir, err := os.MkdirTemp("", "prisma-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	db := open(filepath.Join(dir, "app.db"))
	defer func() { _ = db.Close() }()

	client := prisma.NewClient(db)

	section("1. Model definition & schema introspection")
	demoModels(client)

	section("2. Query building / SQL generation (no database touched)")
	demoSQLGeneration(client)

	section("3. Create: Create, CreateMany, nested writes")
	createSchema(ctx, client)
	demoCreate(ctx, client)

	section("4. Read: FindMany, FindFirst, FindUnique, Count, Exists, Select")
	demoRead(ctx, client)

	section("5. Filtering: the full operator vocabulary")
	demoFilters(ctx, client)

	section("6. Relations: Include (LEFT JOIN) and relation filters")
	demoRelations(ctx, client)

	section("7. Update & Delete, including arithmetic update operators")
	demoUpdateDelete(ctx, client)

	section("8. Aggregations and GroupBy/Having")
	demoAggregates(ctx, client)

	section("9. Distinct, cursor pagination, Paginate, NULLS ordering, Clone")
	demoPagination(ctx, client)

	section("10. Upsert, transactions, batches and raw SQL")
	demoTxAndRaw(ctx, client)

	section("11. Typed Prisma-style errors")
	demoErrors(ctx, client)

	fmt.Println("\nAll sections completed.")
}

// ---------------------------------------------------------------- section 1

func demoModels(client *prisma.Client) {
	for _, v := range []any{User{}, Post{}, AuditLog{}} {
		m, err := client.Register(v)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("model %-9s table=%-12s columns=%v\n", m.Name, m.Table, m.Columns())
		if m.PK != nil {
			fmt.Printf("  pk: %s (column %q, auto=%t)\n", m.PK.GoName, m.PK.Column, m.PK.IsAuto)
		}
		for _, rel := range m.Relations {
			r := rel.Relation
			fmt.Printf("  relation %-6s -> %-8s fk=%s references=%s many=%t\n",
				rel.GoName, r.Target.Name(), r.ForeignKey, r.References, r.Many)
		}
		if f, ok := m.FieldByColumn("id"); ok {
			fmt.Printf("  FieldByColumn(\"id\") -> %s\n", f.GoName)
		}
	}
	fmt.Println("note: User.Secret is tagged `prisma:\"-\"` and does not appear as a column")
	fmt.Println("note: AuditLog.TableName() overrode the default \"audit_logs\" -> \"audit_trail\"")

	// Registry can also be used standalone / shared between clients.
	reg := prisma.NewRegistry()
	if _, err := reg.Register(&Post{}); err != nil {
		log.Fatal(err)
	}
	shared := prisma.NewClient(client.DB(), prisma.WithRegistry(reg), prisma.WithDialect(prisma.Dollar))
	fmt.Printf("\nWithRegistry + WithDialect(Dollar): %s\n", firstOf(prisma.NewQuery[Post](shared).
		Where(prisma.Gt("views", 10)).Take(5).FindManySQL()))

	// Model compilation errors.
	if _, err := prisma.NewRegistry().Register(42); err != nil {
		fmt.Println("Register(42) ->", err)
	}
	type Broken struct {
		ID    int64
		Other []Post `prisma:"relation"` // missing fk=
	}
	if _, err := prisma.NewRegistry().Register(Broken{}); err != nil {
		fmt.Println("relation without fk ->", err)
	}
}

// ---------------------------------------------------------------- section 2

func demoSQLGeneration(client *prisma.Client) {
	show := func(label string, r sqlResult) {
		if r.err != nil {
			fmt.Printf("%-22s ERROR %v\n", label, r.err)
			return
		}
		fmt.Printf("%-22s %s\n%-22s args=%v\n", label, r.sql, "", r.args)
	}

	show("FindManySQL", res(prisma.NewQuery[User](client).
		Where(prisma.StartsWith("email", "ada"), prisma.Gt("age", 20)).
		OrderBy("name", prisma.Asc).Take(10).Skip(5).FindManySQL()))

	show("CountSQL", res(prisma.NewQuery[User](client).
		Where(prisma.In("age", 30, 40)).CountSQL()))

	show("ExistsSQL", res(prisma.NewQuery[User](client).
		Where(prisma.Equals("email", "x@y.z")).ExistsSQL()))

	show("CountDistinctSQL", res(prisma.NewQuery[User](client).CountDistinctSQL("city")))

	show("CreateSQL", res(prisma.NewQuery[User](client).
		CreateSQL(User{Name: "Ada", Email: "ada@example.com", Age: 36})))

	show("CreateManySQL", res(prisma.NewQuery[User](client).SkipDuplicates().
		CreateManySQL([]User{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}})))

	show("UpdateSQL", res(prisma.NewQuery[User](client).
		Where(prisma.Equals("id", 1)).
		UpdateSQL(map[string]any{"name": "Ada L.", "age": prisma.Increment(1)})))

	show("DeleteSQL", res(prisma.NewQuery[User](client).
		Where(prisma.Lt("age", 18)).DeleteSQL()))

	show("UpsertSQL", res(prisma.NewQuery[User](client).
		UpsertSQL([]string{"email"},
			User{Name: "Ada", Email: "ada@example.com"},
			map[string]any{"name": "Ada Lovelace"})))

	show("Aggregate().SQL", res(prisma.NewQuery[Post](client).
		Where(prisma.Equals("status", "published")).
		Aggregate().Count().Sum("views").Avg("views").Min("views").Max("views").SQL()))

	show("GroupBy().SQL", res(prisma.NewQuery[Post](client).
		GroupBy("status").Count().Sum("views").
		Having(prisma.Gt("_count", 1)).SQL()))

	show("relation filter", res(prisma.NewQuery[User](client).
		Where(prisma.Some("Posts", prisma.Equals("status", "published"))).FindManySQL()))

	show("nested rel filters", res(prisma.NewQuery[User](client).
		Where(prisma.Or(
			prisma.Every("Posts", prisma.Gt("views", 0)),
			prisma.None("Posts", prisma.Equals("status", "draft")),
		)).FindManySQL()))

	show("Is (to-one)", res(prisma.NewQuery[Post](client).
		Where(prisma.Is("Author", prisma.Equals("name", "Ada"))).FindManySQL()))

	show("JSON path filter", res(prisma.NewQuery[User](client).
		Where(prisma.JSONEquals("profile", []string{"address", "city"}, "Paris")).FindManySQL()))

	show("WhereRaw + OrWhere", res(prisma.NewQuery[User](client).
		WhereRaw("length(name) > ?", 3).
		OrWhere(prisma.Equals("age", 1), prisma.Equals("age", 2)).FindManySQL()))

	show("Include join", res(prisma.NewQuery[Post](client).
		Include("Author").Where(prisma.Gt("views", 1)).
		OrderBy("views", prisma.Desc).FindManySQL()))

	// Dialect differences on the identical query.
	fmt.Println("\nsame query rendered per dialect:")
	for _, d := range []struct {
		name string
		d    prisma.Dialect
	}{{"Question", prisma.Question}, {"Dollar", prisma.Dollar}, {"MySQLDialect", prisma.MySQLDialect}} {
		c := prisma.NewClient(client.DB(), prisma.WithDialect(d.d), prisma.WithRegistry(client.Registry()))
		s, _, err := prisma.NewQuery[User](c).Where(prisma.Equals("email", "a@b"), prisma.Gt("age", 5)).
			Distinct("city").OrderByNulls("city", prisma.Asc, prisma.NullsLast).Take(3).FindManySQL()
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %-12s %s\n", d.name, s)
		u, _, err := prisma.NewQuery[User](c).UpsertSQL([]string{"email"},
			User{Name: "n", Email: "e"}, map[string]any{"name": "n2"})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %-12s %s\n", "", u)
	}

	// Error paths surface at SQL-build time.
	if _, _, err := prisma.NewQuery[User](client).Where(prisma.Equals("nope", 1)).FindManySQL(); err != nil {
		fmt.Println("\nunknown column in Where ->", err)
	} else {
		fmt.Println("\nHOLE: an unknown column in Where is NOT validated; it is emitted verbatim.")
		s, _, _ := prisma.NewQuery[User](client).Where(prisma.Equals("nope", 1)).FindManySQL()
		fmt.Println("      ", s)
	}
	if _, err := prisma.NewQuery[User](client).Include("Nope").FindMany(context.Background()); err != nil {
		fmt.Println("unknown relation in Include ->", err)
	}
	if _, _, err := prisma.NewQuery[User](client).Select("nope").FindManySQL(); err != nil {
		fmt.Println("unknown column in Select ->", err)
	}
}

// ---------------------------------------------------------------- schema

// createSchema hand-writes the DDL: the library has no schema/migration layer.
func createSchema(ctx context.Context, client *prisma.Client) {
	stmts := []string{
		`CREATE TABLE users (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			name    TEXT    NOT NULL,
			email   TEXT    NOT NULL UNIQUE,
			age     INTEGER NOT NULL,
			city    TEXT,
			profile TEXT    NOT NULL DEFAULT '{}'
		)`,
		`CREATE TABLE posts (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			title     TEXT    NOT NULL,
			status    TEXT    NOT NULL,
			views     INTEGER NOT NULL DEFAULT 0,
			author_id INTEGER NOT NULL REFERENCES users(id)
		)`,
		`CREATE TABLE audit_trail (
			id     INTEGER PRIMARY KEY AUTOINCREMENT,
			action TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := client.ExecuteRaw(ctx, s); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Println("schema created with client.ExecuteRaw (see HOLE about missing DDL support)")
}

// ---------------------------------------------------------------- section 3

func demoCreate(ctx context.Context, client *prisma.Client) {
	ada, err := prisma.NewQuery[User](client).Create(ctx, User{
		Name: "Ada", Email: "ada@example.com", Age: 36,
		City:    sql.NullString{String: "London", Valid: true},
		Profile: `{"address":{"city":"London"},"tier":"gold"}`,
		Secret:  "not persisted",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Create -> generated id written back: %d (%s)\n", ada.ID, ada.Name)

	n, err := prisma.NewQuery[User](client).CreateMany(ctx, []User{
		{Name: "Grace", Email: "grace@example.com", Age: 45,
			City: sql.NullString{String: "Paris", Valid: true}, Profile: `{"address":{"city":"Paris"},"tier":"silver"}`},
		{Name: "Alan", Email: "alan@example.com", Age: 41, Profile: `{"tier":"bronze"}`},
		{Name: "Edsger", Email: "edsger@example.com", Age: 52,
			City: sql.NullString{String: "Paris", Valid: true}, Profile: `{"address":{"city":"Paris"},"tier":"gold"}`},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("CreateMany -> rows affected:", n)

	grace, err := prisma.NewQuery[User](client).
		Where(prisma.Equals("email", "grace@example.com")).FindUnique(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// Nested write: parent + children in one transaction, children get the
	// parent's generated id.
	linus, err := prisma.NewQuery[User](client).
		NestedCreate(User{Name: "Linus", Email: "linus@example.com", Age: 54, Profile: `{"tier":"gold"}`}).
		Create("Posts",
			Post{Title: "Kernel notes", Status: "published", Views: 120},
			Post{Title: "Git rant", Status: "draft", Views: 3}).
		Exec(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("NestedCreate -> parent id %d with 2 child posts\n", linus.ID)

	// Statements() lets you inspect a nested write without running it.
	stmts, err := prisma.NewQuery[User](client).
		NestedCreate(User{Name: "Probe", Email: "probe@example.com"}).
		Create("Posts", Post{Title: "child", Status: "draft"}).
		Connect("Posts", prisma.Equals("id", 1)).
		ConnectOrCreate("Posts", []prisma.Condition{prisma.Equals("id", 9999)}, Post{Title: "made", Status: "draft"}).
		Update("Posts", []prisma.Condition{prisma.Equals("status", "draft")}, map[string]any{"views": 0}).
		Statements(42)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("NestedCreate.Statements(parentID=42) ->")
	for _, st := range stmts {
		fmt.Printf("  %s  args=%v\n", st.SQL, st.Args)
	}

	// Ordinary posts for the rest of the demo.
	if _, err := prisma.NewQuery[Post](client).CreateMany(ctx, []Post{
		{Title: "Analytical Engine", Status: "published", Views: 300, AuthorID: ada.ID},
		{Title: "Notes on notes", Status: "draft", Views: 12, AuthorID: ada.ID},
		{Title: "COBOL", Status: "published", Views: 250, AuthorID: grace.ID},
		{Title: "Compilers", Status: "published", Views: 80, AuthorID: grace.ID},
	}); err != nil {
		log.Fatal(err)
	}
	total, err := prisma.NewQuery[Post](client).Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("total posts:", total)
}

// ---------------------------------------------------------------- section 4

func demoRead(ctx context.Context, client *prisma.Client) {
	users, err := prisma.NewQuery[User](client).OrderBy("age", prisma.Asc).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("FindMany ordered by age:")
	for _, u := range users {
		fmt.Printf("  %-3d %-7s %-22s age=%-3d city=%-8s\n", u.ID, u.Name, u.Email, u.Age, nullStr(u.City))
	}

	first, err := prisma.NewQuery[User](client).
		Where(prisma.Gte("age", 45)).OrderBy("age", prisma.Desc).FindFirst(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("FindFirst (oldest >= 45):", first.Name)

	uniq, err := prisma.NewQuery[User](client).
		Where(prisma.Equals("email", "alan@example.com")).FindUnique(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("FindUnique by email:", uniq.Name)

	n, err := prisma.NewQuery[User](client).Where(prisma.Gt("age", 40)).Count(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Count(age > 40):", n)

	exists, err := prisma.NewQuery[User](client).Where(prisma.Equals("name", "Ada")).Exists(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Exists(name = Ada):", exists)

	cd, err := prisma.NewQuery[User](client).CountDistinct(ctx, "city")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("CountDistinct(city):", cd)

	// Select narrows the projection; unselected fields stay zero.
	partial, err := prisma.NewQuery[User](client).
		Select("id", "name").OrderBy("id", prisma.Asc).Take(2).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Select(id, name) -> %+v\n", partial)
}

// ---------------------------------------------------------------- section 5

func demoFilters(ctx context.Context, client *prisma.Client) {
	type probe struct {
		label string
		cond  prisma.Condition
	}
	probes := []probe{
		{"Equals(age, 36)", prisma.Equals("age", 36)},
		{"Not(age, 36)", prisma.Not("age", 36)},
		{"In(age, 36, 45)", prisma.In("age", 36, 45)},
		{"NotIn(age, 36, 45)", prisma.NotIn("age", 36, 45)},
		{"Lt/Lte/Gt/Gte", prisma.And(prisma.Gt("age", 36), prisma.Lte("age", 52))},
		{"Contains(name, a)", prisma.Contains("name", "a")},
		{"StartsWith(email, a)", prisma.StartsWith("email", "a")},
		{"EndsWith(email, .com)", prisma.EndsWith("email", ".com")},
		{"NotContains(name, a)", prisma.NotContains("name", "a")},
		{"NotStartsWith(name, A)", prisma.NotStartsWith("name", "A")},
		{"NotEndsWith(name, s)", prisma.NotEndsWith("name", "s")},
		{"IContains(name, ADA)", prisma.IContains("name", "ADA")},
		{"IStartsWith(name, ad)", prisma.IStartsWith("name", "ad")},
		{"IEndsWith(name, DA)", prisma.IEndsWith("name", "DA")},
		{"IEquals(name, aDa)", prisma.IEquals("name", "aDa")},
		{"IsNull(city)", prisma.IsNull("city")},
		{"IsNotNull(city)", prisma.IsNotNull("city")},
		{"Between(age, 40, 50)", prisma.Between("age", 40, 50)},
		{"NotBetween(age, 40, 50)", prisma.NotBetween("age", 40, 50)},
		{"Or(...)", prisma.Or(prisma.Equals("name", "Ada"), prisma.Equals("name", "Alan"))},
		{"NotGroup(Or(...))", prisma.NotGroup(prisma.Or(prisma.Equals("name", "Ada"), prisma.Equals("name", "Alan")))},
		{"RawCondition", prisma.RawCondition("length(name) = ?", 4)},
		{"JSONEquals(profile...)", prisma.JSONEquals("profile", []string{"address", "city"}, "Paris")},
		{"JSONNotEquals", prisma.JSONNotEquals("profile", []string{"tier"}, "gold")},
		{"JSONContains(tier, ol)", prisma.JSONContains("profile", []string{"tier"}, "ol")},
		{"JSONStartsWith(tier, g)", prisma.JSONStartsWith("profile", []string{"tier"}, "g")},
		{"JSONIsNull(address)", prisma.JSONIsNull("profile", []string{"address"})},
		{"JSONIsNotNull(address)", prisma.JSONIsNotNull("profile", []string{"address"})},
		{"JSONGt via JSONPath", prisma.JSONPath("profile", []string{"tier"}, ">", "a")},
	}
	for _, p := range probes {
		names, err := matchedNames(ctx, client, p.cond)
		if err != nil {
			fmt.Printf("  %-24s ERROR %v\n", p.label, err)
			continue
		}
		fmt.Printf("  %-24s -> %v\n", p.label, names)
	}
}

func matchedNames(ctx context.Context, client *prisma.Client, cond prisma.Condition) ([]string, error) {
	us, err := prisma.NewQuery[User](client).Where(cond).OrderBy("id", prisma.Asc).FindMany(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(us))
	for _, u := range us {
		out = append(out, u.Name)
	}
	return out, nil
}

// ---------------------------------------------------------------- section 6

func demoRelations(ctx context.Context, client *prisma.Client) {
	posts, err := prisma.NewQuery[Post](client).
		Include("Author").
		Where(prisma.Gt("views", 50)).
		OrderBy("views", prisma.Desc).
		FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Include(\"Author\") eager-loads the to-one relation via LEFT JOIN:")
	for _, p := range posts {
		author := "<nil>"
		if p.Author != nil {
			author = fmt.Sprintf("%s <%s>", p.Author.Name, p.Author.Email)
		}
		fmt.Printf("  %-20s views=%-4d author=%s\n", p.Title, p.Views, author)
	}

	cases := []struct {
		label string
		cond  prisma.Condition
	}{
		{"Some(Posts, published)", prisma.Some("Posts", prisma.Equals("status", "published"))},
		{"Every(Posts, views>0)", prisma.Every("Posts", prisma.Gt("views", 0))},
		{"None(Posts, draft)", prisma.None("Posts", prisma.Equals("status", "draft"))},
		{"Some(Posts) any", prisma.Some("Posts")},
		{"None(Posts) any", prisma.None("Posts")},
		{"And(Some, Gt(age))", prisma.And(
			prisma.Some("Posts", prisma.Gt("views", 200)),
			prisma.Gt("age", 30))},
	}
	fmt.Println("relation filters compile to correlated EXISTS subqueries:")
	for _, c := range cases {
		names, err := matchedNames(ctx, client, c.cond)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %-24s -> %v\n", c.label, names)
	}

	// Is() on a to-one relation, filtering posts by their author.
	byAda, err := prisma.NewQuery[Post](client).
		Where(prisma.Is("Author", prisma.Equals("name", "Ada"))).
		OrderBy("id", prisma.Asc).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	titles := make([]string, 0, len(byAda))
	for _, p := range byAda {
		titles = append(titles, p.Title)
	}
	fmt.Println("  Is(Author, name=Ada)     ->", titles)

	fmt.Println("HOLE: Include only supports to-one relations; there is no way to")
	fmt.Println("      eager-load the to-many User.Posts slice (no second query / no")
	fmt.Println("      grouping), so User.Posts is always nil after a read.")
	ada, _ := prisma.NewQuery[User](client).Where(prisma.Equals("name", "Ada")).FindFirst(ctx)
	fmt.Printf("      len(Ada.Posts) after FindFirst = %d\n", len(ada.Posts))
	if _, err := prisma.NewQuery[User](client).Include("Posts").FindMany(ctx); err != nil {
		fmt.Println("      Include(\"Posts\") ->", err)
	} else {
		fmt.Println("      Include(\"Posts\") silently produced a broken/duplicating join")
	}
}

// ---------------------------------------------------------------- section 7

func demoUpdateDelete(ctx context.Context, client *prisma.Client) {
	n, err := prisma.NewQuery[User](client).
		Where(prisma.Equals("name", "Ada")).
		Update(ctx, map[string]any{"name": "Ada Lovelace"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Update name -> rows:", n)

	n, err = prisma.NewQuery[Post](client).
		Where(prisma.Equals("status", "published")).
		UpdateMany(ctx, map[string]any{
			"views": prisma.Increment(10),
		})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("UpdateMany views +10 -> rows:", n)

	if _, err := prisma.NewQuery[Post](client).
		Where(prisma.Equals("title", "Compilers")).
		Update(ctx, map[string]any{"views": prisma.Multiply(2)}); err != nil {
		log.Fatal(err)
	}
	if _, err := prisma.NewQuery[Post](client).
		Where(prisma.Equals("title", "COBOL")).
		Update(ctx, map[string]any{"views": prisma.Decrement(50)}); err != nil {
		log.Fatal(err)
	}
	if _, err := prisma.NewQuery[Post](client).
		Where(prisma.Equals("title", "Notes on notes")).
		Update(ctx, map[string]any{"views": prisma.Divide(3), "status": prisma.SetTo("archived")}); err != nil {
		log.Fatal(err)
	}
	printPosts(ctx, client, "after arithmetic update operators")

	n, err = prisma.NewQuery[Post](client).Where(prisma.Equals("status", "archived")).Delete(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Delete archived -> rows:", n)

	n, err = prisma.NewQuery[Post](client).Where(prisma.Lt("views", 10)).DeleteMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("DeleteMany views < 10 -> rows:", n)
	printPosts(ctx, client, "remaining posts")
}

func printPosts(ctx context.Context, client *prisma.Client, label string) {
	posts, err := prisma.NewQuery[Post](client).OrderBy("id", prisma.Asc).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(label + ":")
	for _, p := range posts {
		fmt.Printf("  %-3d %-20s %-10s views=%d author=%d\n", p.ID, p.Title, p.Status, p.Views, p.AuthorID)
	}
}

// ---------------------------------------------------------------- section 8

func demoAggregates(ctx context.Context, client *prisma.Client) {
	agg, err := prisma.NewQuery[Post](client).
		Where(prisma.Equals("status", "published")).
		Aggregate().Count().Sum("views").Avg("views").Min("views").Max("views").
		Exec(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Aggregate(published): count=%d sum=%.0f avg=%.1f min=%.0f max=%.0f\n",
		agg.Count, agg.Sum["views"], agg.Avg["views"], agg.Min["views"], agg.Max["views"])

	// NOTE: GroupQuery has no OrderBy of its own; the ORDER BY has to be set on
	// the base Query *before* calling GroupBy.
	rows, err := prisma.NewQuery[Post](client).
		OrderBy("status", prisma.Asc).
		GroupBy("status").Count().Sum("views").Max("views").
		Exec(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("GroupBy(status):")
	for _, r := range rows {
		fmt.Printf("  %v\n", sortedMap(r))
	}

	rows, err = prisma.NewQuery[Post](client).
		GroupBy("author_id").Count().Sum("views").
		Having(prisma.Gt("_count", 1)).
		Exec(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("GroupBy(author_id) HAVING _count > 1:")
	for _, r := range rows {
		fmt.Printf("  %v\n", sortedMap(r))
	}
}

// ---------------------------------------------------------------- section 9

func demoPagination(ctx context.Context, client *prisma.Client) {
	cities, err := prisma.NewQuery[User](client).Distinct("city").
		OrderBy("city", prisma.Asc).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print("Distinct(city) -> ")
	for _, u := range cities {
		fmt.Printf("%q ", nullStr(u.City))
	}
	fmt.Println()

	// Distinct on a subset of columns is emulated client-side on dialects
	// without DISTINCT ON, which interacts badly with Take.
	limited, err := prisma.NewQuery[User](client).Distinct("city").
		OrderBy("city", prisma.Asc).Take(3).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("HOLE: Distinct(\"city\").Take(3) returned %d row(s), not 3: on the\n"+
		"      Question/MySQL dialects the LIMIT is applied in SQL and the\n"+
		"      per-column dedupe happens in Go afterwards, so Take and Distinct\n"+
		"      cannot be combined correctly.\n", len(limited))

	nulls, err := prisma.NewQuery[User](client).
		OrderByNulls("city", prisma.Asc, prisma.NullsLast).
		OrderBy("id", prisma.Asc).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print("OrderByNulls(city, Asc, NullsLast) -> ")
	for _, u := range nulls {
		fmt.Printf("%s(%s) ", u.Name, nullStr(u.City))
	}
	fmt.Println()

	// Keyset pagination.
	fmt.Println("Cursor pagination over users (page size 2):")
	var last int64
	for page := 1; ; page++ {
		q := prisma.NewQuery[User](client).OrderBy("id", prisma.Asc).Take(2)
		if last > 0 {
			q = q.Cursor("id", last)
		}
		batch, err := q.FindMany(ctx)
		if err != nil {
			log.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		names := make([]string, 0, len(batch))
		for _, u := range batch {
			names = append(names, fmt.Sprintf("%d:%s", u.ID, u.Name))
		}
		fmt.Printf("  page %d -> %v\n", page, names)
		last = batch[len(batch)-1].ID
	}

	// Offset pagination via Paginate(page, size).
	p2, err := prisma.NewQuery[User](client).OrderBy("id", prisma.Asc).Paginate(2, 2).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print("Paginate(2, 2) -> ")
	for _, u := range p2 {
		fmt.Printf("%d:%s ", u.ID, u.Name)
	}
	fmt.Println()

	// Clone reuses a base query without mutating it.
	base := prisma.NewQuery[User](client).Where(prisma.Gt("age", 30))
	old := base.Clone().Where(prisma.Gt("age", 50))
	baseN, _ := base.Count(ctx)
	oldN, _ := old.Count(ctx)
	fmt.Printf("Clone: base(age>30) count=%d, clone(+age>50) count=%d\n", baseN, oldN)
}

// ---------------------------------------------------------------- section 10

func demoTxAndRaw(ctx context.Context, client *prisma.Client) {
	// Upsert: first call inserts, second updates.
	n, err := prisma.NewQuery[User](client).Upsert(ctx, []string{"email"},
		User{Name: "Barbara", Email: "barbara@example.com", Age: 60, Profile: `{"tier":"gold"}`},
		map[string]any{"name": "Barbara McClintock"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Upsert (insert) -> rows:", n)
	n, err = prisma.NewQuery[User](client).Upsert(ctx, []string{"email"},
		User{Name: "Barbara", Email: "barbara@example.com", Age: 60, Profile: `{"tier":"gold"}`},
		map[string]any{"name": "Barbara McClintock"})
	if err != nil {
		log.Fatal(err)
	}
	got, _ := prisma.NewQuery[User](client).
		Where(prisma.Equals("email", "barbara@example.com")).FindUnique(ctx)
	fmt.Printf("Upsert (conflict) -> rows: %d, name is now %q\n", n, got.Name)

	// Interactive transaction that commits.
	if err := client.Transaction(ctx, func(tx *prisma.Tx) error {
		if _, err := prisma.NewTxQuery[AuditLog](tx).Create(ctx, AuditLog{Action: "committed"}); err != nil {
			return err
		}
		cnt, err := prisma.NewTxQuery[AuditLog](tx).Count(ctx)
		if err != nil {
			return err
		}
		fmt.Println("inside transaction, audit rows visible:", cnt)
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	// Interactive transaction that rolls back.
	errBoom := errors.New("boom")
	err = client.Transaction(ctx, func(tx *prisma.Tx) error {
		if _, err := prisma.NewTxQuery[AuditLog](tx).Create(ctx, AuditLog{Action: "rolled back"}); err != nil {
			return err
		}
		return errBoom
	})
	fmt.Printf("Transaction returning an error -> errors.Is(err, boom) = %t\n", errors.Is(err, errBoom))

	// Batch: all-or-nothing list of writes.
	counts, err := client.Batch(ctx,
		func(ctx context.Context, tx *prisma.Tx) (int64, error) {
			return prisma.NewTxQuery[AuditLog](tx).CreateMany(ctx,
				[]AuditLog{{Action: "batch-1"}, {Action: "batch-2"}})
		},
		func(ctx context.Context, tx *prisma.Tx) (int64, error) {
			return prisma.NewTxQuery[AuditLog](tx).
				Where(prisma.Equals("action", "batch-1")).
				Update(ctx, map[string]any{"action": "batch-1-updated"})
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Batch -> per-op rows affected:", counts)

	logs, err := prisma.NewQuery[AuditLog](client).OrderBy("id", prisma.Asc).FindMany(ctx)
	if err != nil {
		log.Fatal(err)
	}
	actions := make([]string, 0, len(logs))
	for _, l := range logs {
		actions = append(actions, l.Action)
	}
	fmt.Println("audit_trail contents (rollback left no trace):", actions)

	// Raw SQL.
	raw, err := client.QueryRaw(ctx,
		"SELECT u.name AS name, COUNT(p.id) AS posts FROM users u LEFT JOIN posts p ON p.author_id = u.id GROUP BY u.id ORDER BY u.id")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("QueryRaw ->")
	for _, r := range raw {
		fmt.Printf("  %v\n", sortedMap(r))
	}

	into, err := prisma.QueryRawInto[User](ctx, client,
		"SELECT id, name, email, age, city, profile FROM users WHERE age > ? ORDER BY age DESC", 45)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print("QueryRawInto[User] (age > 45) -> ")
	for _, u := range into {
		fmt.Printf("%s(%d) ", u.Name, u.Age)
	}
	fmt.Println()

	affected, err := client.ExecuteRaw(ctx, "UPDATE audit_trail SET action = ? WHERE action = ?",
		"batch-2-renamed", "batch-2")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("ExecuteRaw -> rows affected:", affected)
}

// ---------------------------------------------------------------- section 11

func demoErrors(ctx context.Context, client *prisma.Client) {
	// P2002 unique violation.
	_, err := prisma.NewQuery[User](client).
		Create(ctx, User{Name: "Dup", Email: "ada@example.com", Age: 1})
	fmt.Printf("duplicate email -> errors.Is(err, ErrUniqueViolation) = %t\n", errors.Is(err, prisma.ErrUniqueViolation))
	var pe *prisma.Error
	if errors.As(err, &pe) {
		fmt.Printf("  code=%s message=%q\n  cause=%v\n", pe.Code, pe.Message, pe.Cause)
	}

	// P2003 foreign-key violation.
	_, err = prisma.NewQuery[Post](client).
		Create(ctx, Post{Title: "orphan", Status: "draft", AuthorID: 999999})
	fmt.Printf("bad author_id -> errors.Is(err, ErrForeignKeyViolation) = %t (%v)\n",
		errors.Is(err, prisma.ErrForeignKeyViolation), err)

	// P2011 NOT NULL violation, triggered through raw SQL.
	_, err = client.ExecuteRaw(ctx, "INSERT INTO users (name, email, age) VALUES (NULL, 'x@y.z', 1)")
	fmt.Printf("NULL name via ExecuteRaw -> errors.Is(err, ErrNotNullViolation) = %t\n",
		errors.Is(err, prisma.ErrNotNullViolation))
	if !errors.Is(err, prisma.ErrNotNullViolation) {
		fmt.Printf("  HOLE: ExecuteRaw does not run errors through mapError, so the raw\n" +
			"        driver error leaks out untyped:\n")
		fmt.Printf("        %v\n", err)
	}

	// ErrNotFound vs P2025.
	_, err = prisma.NewQuery[User](client).Where(prisma.Equals("email", "nobody@example.com")).FindFirst(ctx)
	fmt.Printf("FindFirst miss -> errors.Is(err, ErrNotFound) = %t, errors.Is(err, ErrRecordNotFound) = %t\n",
		errors.Is(err, prisma.ErrNotFound), errors.Is(err, prisma.ErrRecordNotFound))

	_, err = prisma.NewQuery[User](client).Where(prisma.Equals("email", "nobody@example.com")).FindFirstOrThrow(ctx)
	if errors.As(err, &pe) {
		fmt.Printf("FindFirstOrThrow miss -> code=%s (%v)\n", pe.Code, err)
	} else {
		fmt.Printf("FindFirstOrThrow miss -> %v\n", err)
	}
	_, err = prisma.NewQuery[User](client).Where(prisma.Equals("id", -1)).FindUniqueOrThrow(ctx)
	fmt.Printf("FindUniqueOrThrow miss -> errors.Is(err, ErrRecordNotFound) = %t\n",
		errors.Is(err, prisma.ErrRecordNotFound))

	// Builder-level validation errors.
	if _, err := prisma.NewQuery[User](client).Update(ctx, map[string]any{}); err != nil {
		fmt.Println("Update with empty data ->", err)
	}
	if _, err := prisma.NewQuery[User](client).CreateMany(ctx, nil); err != nil {
		fmt.Println("CreateMany with no rows ->", err)
	}
	if _, err := prisma.NewQuery[User](client).Aggregate().Exec(ctx); err != nil {
		fmt.Println("Aggregate with no functions ->", err)
	}
	if _, err := prisma.NewQuery[User](client).GroupBy().Exec(ctx); err != nil {
		fmt.Println("GroupBy with no columns ->", err)
	}

	// An unregistered model type.
	type Unregistered struct {
		ID   int64  `prisma:"col=id,pk,auto"`
		Name string `prisma:"col=name"`
	}
	if _, _, err := prisma.NewQuery[Unregistered](client).FindManySQL(); err != nil {
		fmt.Println("query on unregistered type ->", err)
	} else {
		fmt.Println("NOTE: NewQuery auto-registers unknown types on the fly (no error).")
	}
}

// ---------------------------------------------------------------- helpers

func open(path string) *sql.DB {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		log.Fatal(err)
	}
	db.SetMaxOpenConns(1) // SQLite allows a single writer
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatal(err)
	}
	return db
}

func section(title string) {
	fmt.Printf("\n%s\n== %s\n%s\n", strings.Repeat("=", 74), title, strings.Repeat("=", 74))
}

// sqlResult bundles the (sql, args, err) triple that every *SQL method returns
// so it can be passed as one argument.
type sqlResult struct {
	sql  string
	args []any
	err  error
}

func res(s string, args []any, err error) sqlResult { return sqlResult{s, args, err} }

func firstOf(s string, _ []any, err error) string {
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return s
}

func nullStr(n sql.NullString) string {
	if !n.Valid {
		return "NULL"
	}
	return n.String
}

func sortedMap(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := m[k]
		if b, ok := v.([]byte); ok {
			v = string(b)
		}
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	return strings.Join(parts, " ")
}
