# examples/yaml

A runnable program that exercises the **published** `github.com/malcolmston/yaml`
module — a standard-library-only YAML 1.2 parser, codec and emitter — over
structs, maps, node trees and malformed input.

## Module version

The example consumes the module exactly as an outside user would: there is no
`replace` directive and no reference to the local working tree. The dependency
was added with `GOWORK=off go get github.com/malcolmston/yaml@latest`, which
resolved to the pseudo-version

```
github.com/malcolmston/yaml v0.0.0-20260725030041-44445a971bbd
```

The repository has no semver tags, hence the pseudo-version. Everything below
describes **that** published revision. The local `../../yaml` working tree is
newer and differs (it adds a UTF-8 well-formedness check and rewrites parts of
the emitter and the deviations document), so some holes recorded here may
already be fixed there.

## Run

```sh
cd examples/yaml
GOWORK=off go mod tidy
GOWORK=off go build ./...
GOWORK=off go run .
```

The program terminates on its own and prints twelve labelled sections.

## What it demonstrates

| Section | Feature |
| --- | --- |
| 1 | `Marshal` of a nested struct: `yaml:"name"`, `omitempty`, `flow`, `inline`, `-`, `time.Time`, `time.Duration`, multi-line strings as literal block scalars |
| 2 | `Unmarshal` back into the same struct type, with a `reflect.DeepEqual` check |
| 3 | Maps and `any`: ambiguous strings (`"yes"`, `"007"`, `"null"`) getting quoted, `[]byte` as `!!binary`, mixed sequences |
| 4 | Round-trip fidelity and idempotence: value stability, byte stability of `Marshal`∘`Unmarshal`, literal/folded block scalars, single-quote doubling, empty values, YAML 1.2 core schema (`yes` is a string) |
| 5 | `Encoder.SetIndent` |
| 6 | Anchors, aliases, merge keys (`<<`) with per-document overrides, anchors/aliases as seen on the `Node` tree, and the alias-bomb guard (`ErrAliasBomb`) |
| 7 | Multi-document input: `Decoder` to `io.EOF`, `%YAML` directive, `...` end markers, `Parse` returning one `DocumentNode` per document, `Unmarshal` seeing only the first |
| 8 | Multi-document output: `Encoder` with `---` separators, `Close`, `ErrClosed`, re-decoding the stream, encoding straight to `os.Stdout` |
| 9 | `Node` API: kind/tag/`ShortTag`/`LongTag`/line/column, comments, in-place rewriting and re-emitting, `Node.Decode` on a subtree, `Node.Encode`, `SetString`, `IsZero` |
| 10 | Explicit tags (`!!str`, `!!int`, `!!float`, `!!bool`, `!!null`, `!!binary`, `!!timestamp`), local tags (`!point`), verbatim tags (`!<uri>`), `%TAG` handle expansion, and tag round-tripping |
| 11 | Custom `Marshaler`/`Unmarshaler` (one returning a string, one returning a `*Node` with a custom tag), plus `time.Duration` text decoding |
| 12 | Error handling: `*SyntaxError` with line/column for eight malformed inputs, `ErrUnknownAnchor`, accumulated `*TypeError` with partial decode, `ErrInvalidUnmarshal`, `ErrUnsupportedType`, cyclic values, nesting limits, empty input, a failing `io.Reader` |

## Holes found

Nothing had to be commented out — the whole program compiles and runs. The
holes below are wrong *output*, not missing API. Four of them break round
tripping through `Node`, which is the library's headline feature.

### 1. A tag on a flow collection is emitted twice, producing invalid YAML

```
flow tag doubled: "q: !lst !lst [1, 2]" -> re-parse err=yaml: line 1: column 9: repeated tag
```

`Parse` → `Marshal` → `Parse` fails outright for any document containing a
tagged flow collection (`!point {x: 1}`, `!lst [1, 2]`). Block-style tagged
collections and tagged scalars are emitted correctly, so this is specific to the
flow path. Section 10 shows the whole tagged sample document failing to
re-parse for this reason.

### 2. Global/URI tags are emitted as bare URIs, silently corrupting the value

```
URI tag emitted bare: "v: tag:example.com,2000:thing value" re-parse err=<nil>
  value became kind=scalar tag="" value="tag:example.com,2000:thing value" (tag lost, scalar corrupted)
```

`Node.Tag` legitimately holds a resolved URI — that is what the parser puts
there for `!<tag:example.com,2000:thing>` and for a `%TAG`-expanded handle. The
emitter writes it out without any `!` syntax, so it becomes part of the plain
scalar. No error is reported; the data is just wrong on the way back in. The
correct output is the verbatim `!<...>` form (which the newer local tree
documents and implements). This also hits `%TAG` round trips:
`%TAG !e! tag:example.com,2000:` + `!e!widget 1` re-emits as
`thing: tag:example.com,2000:widget 1`.

### 3. `LineComment` is written verbatim, without the `#` that `writeComment` adds

Head and foot comments go through `writeComment`, which inserts `"# "` when the
text does not already start with `#`. The three `LineComment` emission sites do
not. So the natural

```go
node.LineComment = "note"
```

emits `k: v note`, which parses back as the string `v note` — a silent value
corruption from setting a comment. `HeadComment = "note"` on the same tree works
fine. The example works around it by writing `"# note"` by hand (see the
`// HOLE:` notes in `nodeAPI` and in `Celsius.MarshalYAML`); a caller has no way
to know which fields need the prefix and which do not.

### 4. Foot comments are duplicated on every round trip

```
  # trailing block
  # trailing block
  foot comment emitted 2 times (source had 1)
```

A trailing comment block is attached to *both* the block collection and the
enclosing `DocumentNode`, and the emitter writes both. Each `Parse`/`Marshal`
cycle therefore grows the file, which makes the comment-preserving round trip —
explicitly advertised in `doc.go` — unusable for anything automated.

### 5. `!!timestamp` is dropped on output, degrading `time.Time` to `string`

```
timestamp round trip: time.Time -> "t: 2026-08-10T12:00:00Z" -> string
```

The tag is not re-emitted and a plain `2026-08-10T12:00:00Z` resolves to `!!str`
under the YAML 1.2 core schema, so `any` → YAML → `any` is not stable for
timestamps. (Decoding into an explicit `time.Time` field is fine — section 1/2
round-trips `created` correctly.)

### 6. Ill-formed UTF-8 is accepted silently

```
ill-formed UTF-8: unmarshal err=<nil> value="\xff\xfe" re-emitted="a: '\xff\xfe'\n"
```

libyaml, PyYAML and js-yaml all reject a stream that is not valid Unicode text.
Here the invalid bytes pass through the parser into the Go string and are
written back out inside a single-quoted scalar, so `Marshal` emits something
that is not valid YAML text either. Neither the published `API-DEVIATIONS.md`
nor `doc.go` mentions this. (The newer local tree adds a `checkUTF8` pass, so
this looks like a known and since-fixed gap.)

### 7. Line comments on a scalar mapping value are dropped on parse

Section 9 parses

```yaml
name: widget   # the widget's name
```

and reports `line-comment=""` for the key. The emitter has the same restriction
from the other side: a key's `LineComment` is only written when the value moves
onto its own line ("A comment on the key line has to come after the value"). The
net effect is that the most common comment placement in real YAML files is not
preserved. `doc.go` calls comment handling "best-effort", so this is disclosed,
but it is the single biggest limitation for config-rewriting use cases.

### 8. `ErrUnknownAnchor` is not a `*SyntaxError` and its message is doubled

```
undefined alias  err=yaml: line 1: yaml: unknown anchor "nope"
  Is(ErrSyntax)=false as *SyntaxError=false
```

`errors.Is(err, yaml.ErrUnknownAnchor)` does work, but this is a malformed-input
failure with a known position that does *not* satisfy `errors.Is(err,
ErrSyntax)` and does not expose `Line`/`Column` through `*SyntaxError` like
every other parse failure — so a caller reporting positions has to special-case
it. The `"yaml: "` prefix is also emitted twice.

### 9. Documented YAML 1.2 behaviour worth flagging to users

Not bugs; correct per the deviations document, but they will surprise anyone
migrating:

* `yes`/`no`/`on`/`off`/`y`/`n` are strings, `12:30` is a string, `1_000` is a
  string, `017` is decimal 17.
* On output the Go string `"yes"` is written as the **plain** scalar `yes`
  (round-trips correctly here, but a YAML 1.1 reader will see a boolean).
* Mapping keys are emitted sorted, so key order from the source is not
  preserved when going through Go values (going through `Node` does preserve it).
* Default indent is 4 spaces, and block sequences under a mapping key are
  indented rather than flush.
* `>` folded scalars are re-emitted as `|` literals when that is exact.

## What was easy

The struct codec is solid: `inline`, `flow`, `omitempty`, `-`, `time.Time`,
`time.Duration`, `[]byte`/`!!binary`, `encoding.TextUnmarshaler`, `Marshaler`
returning either a plain value or a `*Node`, and `Unmarshaler` errors
propagating verbatim all worked first try. Merge keys with per-document
overrides, scalar and collection anchors, multi-document streams with `%YAML`
and `...`, `Decoder` to `io.EOF`, `Encoder` with `---` separators and
`ErrClosed`, `SetIndent`, `Node.Decode`/`Node.Encode`/`SetString`, and the whole
error surface (`*SyntaxError` line/column, accumulated `*TypeError` with partial
decode preserved, `ErrInvalidUnmarshal`, `ErrUnsupportedType`, cycle detection,
alias-bomb and nesting guards) all behaved as documented. Round-tripping
*values* is stable and `Marshal` is idempotent; it is round-tripping *nodes*
(tags and comments) that is broken. The module has zero dependencies and
downloads and builds cleanly.
