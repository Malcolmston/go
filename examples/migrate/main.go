// Command migrate-example exercises github.com/malcolmston/migrate end to end
// against a real, throwaway SQLite database created in a temp directory. No
// external database server and no network access are required.
package main

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/malcolmston/migrate"
	_ "modernc.org/sqlite" // pure-Go, cgo-free SQLite driver
)

//go:embed migrations/*.sql
var embedded embed.FS

func main() {
	ctx := context.Background()

	dir, err := os.MkdirTemp("", "migrate-example-")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	section("1. Schema DSL / SQL generation (no database involved)")
	demoSchemaDSL()

	section("2. Reversible Change migrations and their derived inverses")
	changeMigrations := demoChangeMigrations()

	section("3. Loading migrations from files (LoadFS + LoadDir)")
	fileMigrations := demoLoaders(dir)

	db := open(filepath.Join(dir, "app.db"))
	defer func() { _ = db.Close() }()

	mg := migrate.New(db, migrate.WithTable("schema_migrations"))
	all := append([]migrate.Migration{}, changeMigrations...)
	all = append(all, fileMigrations...)
	all = append(all, goFuncMigration())
	if err := mg.Register(all...); err != nil {
		log.Fatal(err)
	}

	section("4. Migrate up, Status and Version")
	demoUp(ctx, mg)

	section("5. Real DML against the migrated schema + idempotent seeds")
	demoSeeds(ctx, db)

	section("6. Rollback / Down / MigrateTo / Redo")
	demoRollbacks(ctx, mg)

	section("7. Error handling: duplicate versions, failing migrations, dirty state")
	demoErrors(ctx, dir)

	section("8. Irreversible Change migrations")
	demoIrreversible(ctx, dir)

	section("9. Schema dump")
	demoDump()

	fmt.Println("\nAll sections completed.")
}

// ---------------------------------------------------------------- section 1

func demoSchemaDSL() {
	build := func(t *migrate.Table) {
		t.String("email", migrate.NotNull(), migrate.Unique())
		t.Text("bio")
		t.Integer("login_count", migrate.NotNull(), migrate.Default(0))
		t.Decimal("balance", 12, 2, migrate.Default(0))
		t.Boolean("active", migrate.NotNull(), migrate.Default(true))
		t.Timestamp("last_seen_at", migrate.Precision(3), migrate.WithTimezone())
		t.UUID("public_id")
		t.JSONB("prefs")
		t.Binary("avatar")
		t.Enum("role", []string{"admin", "member"}, migrate.Default("member"))
		t.String("tags", migrate.Array())
		t.References("account", migrate.WithForeignKey(), migrate.ReferenceNotNull())
		t.Timestamps()
	}

	for _, d := range []migrate.Dialect{migrate.ANSI, migrate.Postgres, migrate.MySQL, migrate.SQLite} {
		s := migrate.NewSchema(d)
		fmt.Printf("-- dialect %q (placeholder for arg 1: %s)\n", d.Name(), d.Placeholder(1))
		fmt.Println(s.CreateTable("users", build))
		fmt.Println()
	}

	// DialectByName does dialect lookup from a config string.
	if d, err := migrate.DialectByName("postgres"); err == nil {
		fmt.Println("DialectByName(\"postgres\") ->", d.Name())
	}
	if _, err := migrate.DialectByName("oracle"); errors.Is(err, migrate.ErrUnknownDialect) {
		fmt.Println("DialectByName(\"oracle\") -> ErrUnknownDialect (as expected)")
	}

	pg := migrate.NewSchema(migrate.Postgres)
	fmt.Println("\n-- indexes")
	fmt.Println(migrate.AddIndex("users", []string{"email"}, migrate.UniqueIndex(), migrate.Where("deleted_at IS NULL")))
	fmt.Println(pg.AddIndex("docs", []string{"body"}, migrate.Using("gin")))
	fmt.Println(migrate.AddIndex("users", []string{"lower(email)"}))
	fmt.Println(migrate.DropIndex("index_users_on_email"))

	fmt.Println("\n-- foreign keys and references")
	fmt.Println(migrate.AddForeignKey("comments", "posts",
		migrate.OnDelete(migrate.Cascade), migrate.OnUpdate(migrate.Restrict)))
	fmt.Println(migrate.RemoveForeignKey("comments", "posts"))
	fmt.Println(migrate.AddReference("comments", "author",
		migrate.ReferenceIndex(), migrate.WithForeignKey(), migrate.ReferenceTable("users")))

	fmt.Println("\n-- bulk ChangeTable")
	fmt.Println(migrate.ChangeTable("users", func(t *migrate.AlterTable) {
		t.String("nickname", migrate.Limit(40))
		t.Boolean("verified", migrate.Default(false))
		t.Rename("bio", "about")
		t.Change("login_count", "BIGINT")
		t.Remove("avatar")
		t.Index([]string{"nickname"}, migrate.UniqueIndex())
	}))

	fmt.Println("\n-- constraints, views, join tables, sequences, pg types")
	fmt.Println(migrate.AddCheckConstraint("users", "login_count >= 0"))
	fmt.Println(migrate.AddUniqueConstraint("users", []string{"email"}))
	fmt.Println(migrate.CreateJoinTable("posts", "tags", nil))
	fmt.Println(migrate.JoinTableName("posts", "tags"))
	fmt.Println(migrate.CreateView("active_users", "SELECT * FROM users WHERE active", migrate.OrReplace()))
	fmt.Println(pg.CreateView("mv_users", "SELECT * FROM users", migrate.Materialized()))
	fmt.Println(pg.RefreshMaterializedView("mv_users"))
	fmt.Println(migrate.CreateSequence("order_no", migrate.SequenceStart(1000), migrate.SequenceIncrement(5)))
	fmt.Println(pg.EnableExtension("pgcrypto"))
	fmt.Println(pg.CreateEnum("role", []string{"admin", "member"}))
	fmt.Println(pg.AddEnumValue("role", "guest", migrate.AfterValue("member")))
	fmt.Println(migrate.TruncateTable("logs", "audits"))
	fmt.Println(migrate.ChangeColumnNull("users", "bio", false))
	fmt.Println(migrate.ChangeColumnDefaultRaw("users", "last_seen_at", "CURRENT_TIMESTAMP"))
	fmt.Println(migrate.RenameIndex("users", "index_users_on_email", "idx_users_email"))
}

// ---------------------------------------------------------------- section 2

// demoChangeMigrations builds two reversible migrations whose Down direction is
// derived automatically, and prints both directions.
func demoChangeMigrations() []migrate.Migration {
	users := migrate.ChangeWith(migrate.SQLite, 20240101, "create_users", func(r *migrate.ChangeRecorder) {
		r.CreateTable("users", func(t *migrate.Table) {
			t.String("email", migrate.NotNull())
			t.String("name")
			t.Integer("login_count", migrate.NotNull(), migrate.Default(0))
		})
		r.AddIndex("users", []string{"email"}, migrate.UniqueIndex())
	})

	posts := migrate.ChangeWith(migrate.SQLite, 20240102, "create_posts", func(r *migrate.ChangeRecorder) {
		r.CreateTable("posts", func(t *migrate.Table) {
			t.String("title", migrate.NotNull())
			t.Text("body")
			t.Boolean("published", migrate.NotNull(), migrate.Default(false))
			t.References("user", migrate.WithForeignKey(), migrate.ReferenceNotNull())
		})
		r.AddIndex("posts", []string{"user_id"})
	})

	// A plain Migration with hand-written UpSQL/DownSQL rendered by the DSL.
	sqlite := migrate.NewSchema(migrate.SQLite)
	bio := migrate.Migration{
		Version: 20240103,
		Name:    "add_users_bio",
		UpSQL:   sqlite.AddColumn("users", "bio", "TEXT"),
		DownSQL: sqlite.DropColumn("users", "bio"),
	}

	for _, m := range []migrate.Migration{users, posts, bio} {
		fmt.Printf("migration %d %-14s Up func=%t Down func=%t UpSQL=%q DownSQL=%q\n",
			m.Version, m.Name, m.Up != nil, m.Down != nil, oneLine(m.UpSQL), oneLine(m.DownSQL))
	}
	fmt.Println("\nHOLE: Change/ChangeWith return a Migration whose directions are opaque")
	fmt.Println("      closures (UpSQL/DownSQL stay empty) and ChangeRecorder has no")
	fmt.Println("      exported constructor or statement accessor, so the derived SQL cannot")
	fmt.Println("      be inspected, printed or unit-tested without executing it. The")
	fmt.Println("      equivalent statements have to be re-derived through Schema:")
	s := migrate.NewSchema(migrate.SQLite)
	fmt.Println("      up:   " + oneLine(s.CreateTable("users", func(t *migrate.Table) {
		t.String("email", migrate.NotNull())
	})))
	fmt.Println("      down: " + oneLine(s.DropTable("users")))

	return []migrate.Migration{users, posts, bio}
}

// goFuncMigration returns a migration whose directions are Go functions.
func goFuncMigration() migrate.Migration {
	return migrate.Migration{
		Version: 20240105,
		Name:    "create_published_posts_view",
		Up: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, migrate.NewSchema(migrate.SQLite).
				CreateView("published_posts", "SELECT id, title FROM posts WHERE published"))
			return err
		},
		Down: func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, migrate.NewSchema(migrate.SQLite).DropView("published_posts"))
			return err
		},
	}
}

// ---------------------------------------------------------------- section 3

func demoLoaders(tmp string) []migrate.Migration {
	sub, err := fs.Sub(embedded, "migrations")
	if err != nil {
		log.Fatal(err)
	}
	fromFS, err := migrate.LoadFS(sub)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("LoadFS(embed.FS) loaded %d migration(s)\n", len(fromFS))
	for _, m := range fromFS {
		fmt.Printf("  %d %s reversible=%t\n", m.Version, m.Name, m.DownSQL != "")
	}

	// The same files on a real directory via LoadDir.
	copyDir := filepath.Join(tmp, "migrations")
	if err := os.MkdirAll(copyDir, 0o755); err != nil {
		log.Fatal(err)
	}
	entries, _ := fs.ReadDir(sub, ".")
	for _, e := range entries {
		b, _ := fs.ReadFile(sub, e.Name())
		if err := os.WriteFile(filepath.Join(copyDir, e.Name()), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	// A stray file that does not match the naming pattern must be ignored.
	_ = os.WriteFile(filepath.Join(copyDir, "README.txt"), []byte("ignored"), 0o644)

	fromDir, err := migrate.LoadDir(copyDir)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("LoadDir(%q) loaded %d migration(s) (non-matching files ignored)\n",
		filepath.Base(copyDir), len(fromDir))
	fmt.Printf("  up SQL of %d:\n%s\n", fromDir[0].Version, indent(fromDir[0].UpSQL))
	return fromDir
}

// ---------------------------------------------------------------- section 4

func demoUp(ctx context.Context, mg *migrate.Migrator) {
	if err := mg.Up(ctx, 2); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Up(ctx, 2) applied the first two migrations")
	printStatus(ctx, mg)

	if err := mg.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("\nMigrate(ctx) applied the rest")
	printStatus(ctx, mg)

	// Migrate is idempotent.
	if err := mg.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Migrate(ctx) again -> no error (idempotent)")

	v, err := mg.Version(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Version(ctx) =", v)
	fmt.Println("Registered migrations:", len(mg.Migrations()))
}

func printStatus(ctx context.Context, mg *migrate.Migrator) {
	st, err := mg.Status(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("  VERSION    NAME                          APPLIED  AT")
	for _, s := range st {
		at := "-"
		if s.Applied {
			at = s.AppliedAt.Format("2006-01-02T15:04:05Z")
		}
		fmt.Printf("  %-10d %-29s %-8t %s\n", s.Version, s.Name, s.Applied, at)
	}
}

// ---------------------------------------------------------------- section 5

func demoSeeds(ctx context.Context, db *sql.DB) {
	seeder := migrate.NewSeeder(db, migrate.WithSeedTable("schema_seeds"))
	if err := seeder.EnsureTable(ctx); err != nil {
		log.Fatal(err)
	}

	run := func(label string) {
		err := seeder.Run(ctx, "base_users", func(ctx context.Context, tx *sql.Tx) error {
			if err := migrate.Execute(ctx, tx,
				"INSERT INTO users (email, name, login_count) VALUES (?, ?, ?)",
				"ada@example.com", "Ada", 3); err != nil {
				return err
			}
			return migrate.ExecuteAll(ctx, tx,
				"INSERT INTO users (email, name, login_count) VALUES ('grace@example.com', 'Grace', 7)",
				"INSERT INTO tags (label) VALUES ('go')")
		})
		if err != nil {
			log.Fatal(err)
		}
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("%s: users rows = %d\n", label, n)
	}
	run("seeder.Run first call ")
	run("seeder.Run second call")

	applied, err := seeder.Applied(ctx)
	if err != nil {
		log.Fatal(err)
	}
	names := make([]string, 0, len(applied))
	for name := range applied {
		names = append(names, name)
	}
	fmt.Println("seeder.Applied ->", names)

	if err := seeder.RunSQL(ctx, "extra_posts",
		"INSERT INTO posts (title, body, published, user_id) VALUES ('Hello', 'first', 1, 1)",
		"INSERT INTO posts (title, body, published, user_id) VALUES ('Draft', 'wip', 0, 2)",
	); err != nil {
		log.Fatal(err)
	}
	var pub int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM published_posts").Scan(&pub); err != nil {
		log.Fatal(err)
	}
	fmt.Println("rows visible through the migration-created view published_posts:", pub)
}

// ---------------------------------------------------------------- section 6

func demoRollbacks(ctx context.Context, mg *migrate.Migrator) {
	must := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}
	version := func() uint64 {
		v, err := mg.Version(ctx)
		must(err)
		return v
	}

	must(mg.Rollback(ctx, 1))
	fmt.Println("Rollback(ctx, 1)     -> version", version())

	must(mg.Down(ctx, 2))
	fmt.Println("Down(ctx, 2)         -> version", version())

	must(mg.MigrateTo(ctx, 20240105))
	fmt.Println("MigrateTo(20240105)  -> version", version())

	must(mg.MigrateTo(ctx, 20240102))
	fmt.Println("MigrateTo(20240102)  -> version", version())

	must(mg.Redo(ctx))
	fmt.Println("Redo(ctx)            -> version", version(), "(rolled back and re-applied 20240102)")

	must(mg.MigrateTo(ctx, 0))
	fmt.Println("MigrateTo(0)         -> version", version(), "(everything rolled back)")

	st, err := mg.Status(ctx)
	must(err)
	appliedCount := 0
	for _, s := range st {
		if s.Applied {
			appliedCount++
		}
	}
	fmt.Println("applied migrations after full rollback:", appliedCount)

	must(mg.Migrate(ctx))
	fmt.Println("Migrate(ctx)         -> version", version(), "(back up again)")
}

// ---------------------------------------------------------------- section 7

func demoErrors(ctx context.Context, tmp string) {
	// Duplicate versions are rejected at Register time.
	dup := migrate.New(openMemLike(tmp, "dup.db"))
	err := dup.Register(
		migrate.Migration{Version: 1, Name: "a", UpSQL: "SELECT 1"},
		migrate.Migration{Version: 1, Name: "b", UpSQL: "SELECT 1"},
	)
	fmt.Printf("Register duplicate version: errors.Is(err, ErrDuplicateVersion) = %t\n  %v\n",
		errors.Is(err, migrate.ErrDuplicateVersion), err)

	// A migration with no usable Up direction is invalid.
	err = migrate.New(openMemLike(tmp, "invalid.db")).Register(migrate.Migration{Version: 2, Name: "empty"})
	fmt.Printf("Register migration without Up: errors.Is(err, ErrInvalidMigration) = %t\n  %v\n",
		errors.Is(err, migrate.ErrInvalidMigration), err)

	// An unsafe bookkeeping table name is rejected.
	bad := migrate.New(openMemLike(tmp, "badtable.db"), migrate.WithTable("bad name; DROP TABLE x"))
	err = bad.EnsureSchemaTable(ctx)
	fmt.Printf("WithTable(\"bad name; DROP TABLE x\"): errors.Is(err, ErrInvalidTableName) = %t\n  %v\n",
		errors.Is(err, migrate.ErrInvalidTableName), err)

	// A failing migration halts the run, is rolled back and is NOT recorded.
	db := openMemLike(tmp, "failing.db")
	defer func() { _ = db.Close() }()
	mg := migrate.New(db)
	if err := mg.Register(
		migrate.Migration{
			Version: 10,
			Name:    "ok",
			UpSQL:   "CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)",
			DownSQL: "DROP TABLE widgets",
		},
		migrate.Migration{
			Version: 11,
			Name:    "half_broken",
			// First statement succeeds, second one is invalid: the whole
			// migration must roll back inside its transaction.
			UpSQL:   "INSERT INTO widgets (name) VALUES ('a'); INSERT INTO nope (x) VALUES (1)",
			DownSQL: "DELETE FROM widgets",
		},
		migrate.Migration{
			Version: 12,
			Name:    "never_reached",
			UpSQL:   "CREATE TABLE gadgets (id INTEGER PRIMARY KEY)",
			DownSQL: "DROP TABLE gadgets",
		},
	); err != nil {
		log.Fatal(err)
	}
	err = mg.Migrate(ctx)
	fmt.Printf("\nMigrate with a broken migration -> error:\n  %v\n", err)
	v, _ := mg.Version(ctx)
	fmt.Println("version after failure:", v, "(the broken migration was not recorded)")
	var widgets int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM widgets").Scan(&widgets); err != nil {
		log.Fatal(err)
	}
	fmt.Println("widget rows:", widgets, "(the partial INSERT was rolled back with the transaction)")
	var gadgets int
	fmt.Printf("table gadgets created? %t (the run halted before version 12)\n",
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gadgets").Scan(&gadgets) == nil)
	printStatus(ctx, mg)
	fmt.Println("NOTE: there is no dirty flag in the bookkeeping table; recovery is just")
	fmt.Println("      'fix the migration and re-run Migrate', which is safe because a")
	fmt.Println("      failed migration leaves no version row behind.")

	// A version present in the database with no registered migration.
	orphan := migrate.New(db)
	if err := orphan.Register(migrate.Migration{Version: 99, Name: "other", UpSQL: "SELECT 1"}); err != nil {
		log.Fatal(err)
	}
	err = orphan.Down(ctx, 1)
	fmt.Printf("\nDown with an applied-but-unregistered version: errors.Is(err, ErrMissingMigration) = %t\n  %v\n",
		errors.Is(err, migrate.ErrMissingMigration), err)
	st, _ := orphan.Status(ctx)
	for _, s := range st {
		if s.Name == "(unknown)" {
			fmt.Printf("Status reports the orphan version %d as %q\n", s.Version, s.Name)
		}
	}
}

// ---------------------------------------------------------------- section 8

func demoIrreversible(ctx context.Context, tmp string) {
	db := openMemLike(tmp, "irreversible.db")
	defer func() { _ = db.Close() }()

	m := migrate.ChangeWith(migrate.SQLite, 30240101, "backfill", func(r *migrate.ChangeRecorder) {
		r.CreateTable("notes", func(t *migrate.Table) { t.Text("body") })
		r.Execute("UPDATE notes SET body = 'x'") // raw Execute cannot be inverted
	})
	mg := migrate.New(db)
	if err := mg.Register(m); err != nil {
		log.Fatal(err)
	}
	if err := mg.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("applied a Change containing a raw Execute (which cannot be inverted)")

	err := mg.Rollback(ctx, 1)
	fmt.Printf("Rollback -> errors.Is(err, ErrIrreversibleMigration) = %t\n  %v\n",
		errors.Is(err, migrate.ErrIrreversibleMigration), err)
	v, _ := mg.Version(ctx)
	fmt.Println("version is unchanged after the failed rollback:", v)
	fmt.Println("HOLE: irreversibility is only detectable by *attempting* the rollback at")
	fmt.Println("      runtime. ChangeRecorder.Reversible() exists but ChangeRecorder cannot")
	fmt.Println("      be constructed from outside the package, so there is no way to")
	fmt.Println("      pre-flight a Change migration for reversibility.")
}

// ---------------------------------------------------------------- section 9

func demoDump() {
	dump := migrate.NewSchemaDump(migrate.Postgres, 20240105).
		EnableExtension("pgcrypto").
		CreateEnum("role", []string{"admin", "member"}).
		CreateTable("users", func(t *migrate.Table) {
			t.String("email", migrate.NotNull())
			t.JSONB("prefs")
		}).
		AddIndex("users", []string{"email"}, migrate.UniqueIndex()).
		AddUniqueConstraint("users", []string{"email"}).
		AddCheckConstraint("users", "email <> ''").
		CreateJoinTable("posts", "tags", nil).
		AddForeignKey("posts", "users", migrate.OnDelete(migrate.Cascade)).
		CreateView("active_users", "SELECT * FROM users")
	fmt.Println(dump.String())
	fmt.Println("dump.Version() =", dump.Version(), "statements =", len(dump.Statements()))
}

// ---------------------------------------------------------------- helpers

func open(path string) *sql.DB {
	db, err := sql.Open("sqlite", path+"?_time_format=sqlite")
	if err != nil {
		log.Fatal(err)
	}
	// SQLite tolerates only one writer; keep the pool at one connection.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		log.Fatal(err)
	}
	return db
}

func openMemLike(dir, name string) *sql.DB { return open(filepath.Join(dir, name)) }

func section(title string) {
	fmt.Printf("\n%s\n== %s\n%s\n", strings.Repeat("=", 74), title, strings.Repeat("=", 74))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}
