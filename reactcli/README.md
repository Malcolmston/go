# reactcli

The `react` command — the toolchain for
[`github.com/malcolmston/react`](../react), React 19's programming model in Go.

```sh
go install github.com/malcolmston/reactcli@latest
```

The binary is called `react`. It is a **separate module** from the library on
purpose: the library is standard-library-only and stays that way, while the CLI
is free to depend on [cobra](https://github.com/spf13/cobra) and
[chalk](../chalk).

## What it does

```sh
react new blog          # scaffold a project
react add tailwind      # wire in ecosystem tooling
react dev               # rebuild and reload on save
react build             # render to static HTML
react doctor            # find react mistakes the compiler cannot catch
react gen component Card --prop title:string --state
```

## The CLI does not own your project

A react project is an ordinary Go module with a `main` package that renders your
tree and writes HTML. The CLI runs that program; it never loads your code as a
plugin.

```sh
go run . -out dist      # exactly what `react build` does
```

So a compile error or a panic in a component is reported by the Go toolchain,
with a real file and line and no wrapper layer in between. And a project whose
author uninstalls the CLI still builds.

## Integrations, without Node

`react add tailwind` installs the **Tailwind standalone binary** — one
self-contained executable — into `.react/bin` and records the resolved version
in `react.json`. There is no `package.json`, no `node_modules`, and no
JavaScript runtime anywhere in the project.

Two properties worth knowing:

- **Versions are pinned, not resolved.** `latest` is resolved exactly once, at
  install time, and the concrete version is written to the manifest. `react
  install` restores that version on another machine. A toolchain that quietly
  upgrades itself between machines is a toolchain that produces different output
  on each.
- **Downloads are checksum-verified.** The release's `sha256sums.txt` is fetched
  and the digest checked before the file is moved into place, so a partial or
  tampered binary never reaches the path a build will execute. If a release
  publishes no checksum, the CLI says so out loud rather than skipping
  verification silently.

Tailwind finds class names in Go source the same way it finds them in JSX: it
scans for candidate substrings rather than parsing a language, so class names in
`react.Props{"className": "flex gap-2"}` are picked up normally. The generated
`styles/app.css` carries an `@source "./**/*.go"` directive to point it at Go
files. The JSX caveat carries over too — a name assembled at runtime
(`"bg-" + colour`) is invisible to the scanner.

Only tools that publish a standalone binary can be wired up this way. That is a
real limit, and `react add --list` shows what is actually available rather than
implying the whole npm ecosystem is one command away.

## `react doctor`

The reason the CLI is worth having. Every check is for code that **compiles
cleanly and then misbehaves**:

| Check | What it catches |
| ----- | --------------- |
| `conditional-hook` | A hook behind an `if`, loop or `switch`. Hook state is matched by call order, so a hook that doesn't run every render hands every later hook the wrong slot. |
| `hook-in-closure` | A hook inside an effect body, callback or goroutine, where no component owns it. |
| `unconverted-component` | A function with a component's signature declared as a plain `func`. Dispatch is by named type, so it renders as **nothing at all** — no error, just a missing subtree. |
| `missing-key` | An element built in a loop with no `key`. Keyless siblings are matched by position, so inserting at the front moves every item's state onto its neighbour. |

`react.Use` is deliberately exempt from the hook-order checks: React 19's
`use()` is defined to be callable conditionally and in loops, and this port
honours that by never giving it a hook slot.

Analysis is syntactic — `go/parser`, no type checking — which keeps it instant
and dependency-free at the cost of reasoning about names rather than types.
`missing-key` is a warning rather than an error for exactly that reason: it can
only see literal props maps.

It exits non-zero on any error-level finding, so it drops into CI as is.
`--strict` fails on warnings too.

## `react dev`

Watches the project, rebuilds on save, and serves the output. Open pages reload
themselves through a server-sent events stream injected into HTML **responses** —
nothing is written into your source, and the build output on disk stays byte-for-byte
what `react build` produces.

Saves are debounced: one editor save commonly produces several filesystem
events, and format-on-save can touch a whole package at once.

A failed build leaves the last good page on screen rather than replacing it with
a blank document.

## `react.json`

```json
{
  "name": "blog",
  "entry": ".",
  "out": "dist",
  "static": "public",
  "integrations": {
    "tailwind": {
      "version": "v4.1.5",
      "input": "styles/app.css",
      "output": "dist/assets/app.css"
    }
  }
}
```

Deliberately small. Everything derivable from the Go toolchain — the module
path, the dependency set, the Go version — is absent, because duplicating
`go.mod` into a second file is how the two drift apart. Unknown keys are an
error, not a silent no-op: a config typo that does nothing is far more expensive
to find than one that fails immediately.

## Generated code

`react add` owns exactly one file inside your project, `app/integrations.go`,
and rewrites it **wholesale** from the manifest rather than patching your
sources. Adding or removing an integration therefore cannot mangle hand-written
code, and the file's contents are always exactly a function of `react.json`.

## Status

`0.1.0`. Known gaps:

- `react new` writes a `go.mod` requiring a published version of the react
  library. Until that version is tagged, add a `replace` pointing at a local
  checkout.
- Only the Tailwind integration exists so far.
- The download path has not been exercised against the live GitHub API in
  automated tests; the checksum and pinning logic is unit-reviewed, not
  integration-tested.
