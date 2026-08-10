# `yaml` parity coverage

Port under test: **`github.com/malcolmston/yaml v0.1.0`** (resolved by
`GOWORK=off go get github.com/malcolmston/yaml@latest`; consumed as a published
module, no `replace`).

Two upstreams, because the thing being ported is a *specification with a
conformance corpus*, not just a library:

| upstream | pin | role |
| --- | --- | --- |
| [`yaml/yaml-test-suite`](https://github.com/yaml/yaml-test-suite) | branch `main`, commit `da267a5c4782e7361e82889e76c0dc7df0e1e870` (2025-12-25) | the **corpus**: 351 files, 402 tests, each with an input document, a must-fail flag and often an expected JSON value |
| [`yaml`](https://www.npmjs.com/package/yaml) (JavaScript) | `yaml@2.6.1` (`node/package.json`, `node/package-lock.json`) | the **live oracle**: a second opinion on everything the corpus does not state — decoded values, emitter output, round-trip stability |

So this directory reports two different numbers, and they are not the same
question:

* **parity** — do the port and the JavaScript oracle answer identically?
* **conformance** — does each side answer the way the test suite says? Reported
  per side, split into accept-cases, must-fail cases and to-JSON cases.

Both are machine-generated into `parity.json` by `go test`. Nothing in
`cases/*.json` states an expected result.

```
GOWORK=off go test ./parity/yaml/
```

## Score

| | cases | match | mismatch | deviations | parity |
| --- | --- | --- | --- | --- | --- |
| **total** | 1071 | 990 | 74 | 7 | **93.05 %** |
| `suite-accept` | 308 | 304 | 4 | 0 | 98.70 % |
| `suite-fail` | 94 | 92 | 2 | 0 | 97.87 % |
| `suite-json` | 279 | 273 | 5 | 1 | 98.20 % |
| `suite-reemit` | 308 | 262 | 45 | 1 | 85.34 % |
| `emitter-bugs` | 23 | 8 | 13 | 2 | 38.10 % |
| `schema-1-1-vs-1-2` | 24 | 17 | 4 | 3 | 80.95 % |
| `emit-values` | 18 | 17 | 1 | 0 | 94.44 % |
| `stream` | 17 | 17 | 0 | 0 | 100.00 % |

Parity is over the cases actually compared (`match + mismatch`); the 7
deviations are excluded, and each is listed in
[`yaml/API-DEVIATIONS.md`](../../yaml/API-DEVIATIONS.md).

### Conformance against the test suite

| bucket | cases | port | `yaml@2.6.1` |
| --- | --- | --- | --- |
| accept (suite says the document is well-formed) | 308 | **308 (100.00 %)** | 307 (99.68 %) |
| must-fail (suite says it must be rejected) | 94 | **94 (100.00 %)** | 92 (97.87 %) |
| to-JSON (suite gives the expected value) | 274 | **273 (99.64 %)** | 270 (98.54 %) |

The port is *more* conformant than the reference JavaScript implementation on
every bucket. Its single to-JSON miss is `565N`, which it documents (it
base64-decodes `!!binary`, so it cannot reproduce the suite's undecoded text);
that matches the claim in its own `API-DEVIATIONS.md`. The oracle's misses are
`2JQS` (rejects duplicate keys the suite accepts), `9MMA`/`SF5V` (accepts two
must-fail directive documents), and `2XXW`/`J7PZ`/`M7A3`.

274 of the 279 to-JSON cases are scored: five suite tests carry an empty `json`
field and so state nothing to compare against.

Read `parity.json` for the per-case audit trail: `conformance.<bucket>.goFails`
and `.nodeFails` list the suite ids each side answers differently.

## Confirmed emitter defects

Every item below is an observed round-trip failure, reproduced by a named case.
The top finding is **(A): the port emits YAML that the port itself cannot
re-parse.** Case ids in `cases/emitter-bugs.json` unless noted.

### A. Output does not re-parse — 11 cases

| what | cases | emitted | error on re-parse |
| --- | --- | --- | --- |
| a tag on a **flow** collection is written twice | `emit-flow-seq-tag`, `emit-flow-map-tag`, `emit-flow-seq-tag-nested` | `!lst !lst [1, 2]` | `repeated tag` |
| an **anchor** on a flow collection, or on an explicit key, is written twice | `reemit-CN3R`, `reemit-6BFJ` | `&flowseq &flowseq [...]` | `repeated anchor` |
| an **alias used as an implicit key** is written with no space before the colon, so the parser reads the colon as part of the anchor name | `reemit-E76Z`, `reemit-X38W`, `reemit-26DV` | `*b: *a` | `unknown anchor "b:"` |
| a **global/URI tag on the document root** is written as a bare URI on its own line, which destroys the document structure | `reemit-C4HZ`, `reemit-UGM3` | `tag:clarkevans.com,2002:shape` then indented items | `unexpected content after document root` |
| a folded block scalar whose first line is more-indented is re-emitted as a literal block with a broken indent | `reemit-F6MC` | `b: \|` + blank lines + mixed indent | `unexpected content after document root` |

A tag or anchor on a *block* collection survives (`emit-block-seq-tag-control`
matches), so the doubling is specific to flow style.

### B. Output re-parses but no longer decodes — 13 cases

`reemit-4FJ6`, `reemit-6PBE`, `reemit-9MMW`, `reemit-KK5P`, `reemit-LX3P`,
`reemit-M2N8-00/01`, `reemit-M5DY`, `reemit-Q9WF`, `reemit-RZP5`, `reemit-SBG9`,
`reemit-V9D5`, `reemit-XW4D`, plus `schema-complex-key`.

The port cannot decode a **complex (collection) mapping key** at all: `Parse`
accepts `? [a, b]\n: v`, but `(*Node).Decode` into `any` fails with
`invalid map key: []interface {}`. Re-emitting also *converts* block complex
keys into flow (`? - a\n  - b` becomes `? [a, b]`), so the shape changes even
where it can be read. The oracle decodes all of these.

### C. Round trip silently changes the value — 12 cases

| what | cases |
| --- | --- |
| a global/URI tag is emitted as a **bare URI**, so it comes back as part of a plain string: `!<tag:example.com,2000:app/foo> bar` becomes `tag:example.com,2000:app/foo bar` | `emit-verbatim-global-tag`, `emit-tag-directive-global`, `emit-tag-directive-secondary`, `reemit-CC74`, `reemit-Z9M4`, `reemit-6CK3`, `reemit-P76L` |
| `!!timestamp` is **dropped** on emit, so a timestamp comes back as a string | `emit-timestamp-tag`, `emit-timestamp-tag-spaced` |
| the string `"\n"` is emitted as a literal block with an empty body (`\|` then a blank line) and reads back as `""` — **data loss** | `marshal-multiline`, `reemit-K858`, `reemit-JEF9-01`, `reemit-JEF9-02` |

The bare-URI emission directly contradicts the port's own
`API-DEVIATIONS.md` § *"Tags are emitted verbatim when they have no shorthand"*,
which promises the `!<...>` form.

### D. The document grows on every round trip — 4 cases

`emit-foot-comment-growth`, `emit-foot-comment-growth-seq`, `reemit-F8F9`,
`reemit-JHB9`. A foot comment is duplicated each time: `a: 1\n# foot\n` becomes
`a: 1\n# foot\n# foot\n` after one emit and eight `# foot` lines after three.
Unbounded growth. A head comment is stable
(`emit-head-comment-roundtrip` matches).

### E. `LineComment` is written without its `#` — 2 cases

`emit-line-comment`, `emit-line-comment-string`. Setting
`Node.LineComment = "why"` on a mapping value and marshalling gives `a: 1 why`,
which re-parses as the **string** `"1 why"`. The oracle writes `a: 1 #why` and
reads back `1`. Silent data corruption from a documented field.

### F. Ill-formed UTF-8 is accepted — 1 case

`utf8-illformed-accept` (with `utf8-illformed-is-illformed` asserting the
fixture). `yaml.Parse([]byte{'a', ':', ' ', 0xff, 0xfe, '\n'})` returns no
error; the bytes survive as U+FFFD and are re-emitted as `a: '��'`.
This contradicts `API-DEVIATIONS.md` § *"Ill-formed UTF-8 is a syntax error"*,
which states the port rejects it. It cannot be scored as a parity mismatch: a
JavaScript string cannot hold ill-formed UTF-8, so the oracle never sees the
same input (`utf8-illformed-reemit` is marked a deviation for that reason).

### Where the *oracle* is the unstable side

Six cases mismatch because `yaml@2.6.1` grows or rewrites and the port does not:
`reemit-MUS6-02/03/04/05/06`, `reemit-PW8X`, `reemit-SM9W-00`, `reemit-6XDY`,
`reemit-DK95-07` — mostly an empty document written as `---\n\n` gaining a blank
line per round, and anchors on empty scalars being re-laid-out.

## Other divergences found

| what | case | port | `yaml@2.6.1` |
| --- | --- | --- | --- |
| duplicate mapping keys | `schema-dup-keys`, `accept-2JQS` | accepted (last wins) | rejected — the port is over-permissive, though the suite sides with the port on `2JQS` |
| uppercase hex prefix `0XC` | `schema-int-hex` | resolved to `12` | left a string (the 1.2 core schema only has `0x`) — the port is over-permissive |
| `!!bool yes` | `schema-explicit-tags` | decode error | the string `"yes"` |
| `!!omap` | `json-J7PZ`, `reemit-J7PZ` | a sequence of single-pair maps (the suite agrees) | an object |
| `!!set` | `json-2XXW`, `reemit-2XXW` | a mapping with `null` values (the suite agrees), but re-emitted as `k: null` rather than `? k` | a `Set`, which the harness cannot normalise |
| an empty document (`...` or a comment only) | `accept-HWV9`, `accept-QT73`, `accept-M7A3` | dropped from the stream | kept as `null`; on `M7A3` the suite agrees with the port |
| tags on empty scalars | `reemit-FH7J` | `!!str ""` key re-emitted so it reads back as `null` | preserved |
| `!!binary` | `parse-binary-tag`, `emit-binary-tag`, `json-565N` | base64-decoded to a string (documented deviation) | `Uint8Array` |
| merge keys `<<` | `schema-merge-key` | resolved (documented deviation) | kept as a literal `<<` key unless `{merge: true}` |
| alias bomb | `schema-alias-bomb` | allowed up to `1<<21` nodes (documented deviation) | rejected outright |
| integers beyond 2^53 | `schema-big-int` | full precision (deviation: JavaScript cannot) | rounded |

YAML 1.1-vs-1.2 resolution agrees everywhere else: `yes`/`no`/`on`/`off`/`y`/`n`
stay strings on both sides, `~`/`null`/`Null`/`NULL`/empty are all null, `017`
is `17` (not octal), `0o17` is `15`, `0b1010` and `1_000` and `12:30` and
`190:20:30` and `inf`/`nan`/`Infinity` are strings, `.inf`/`-.Inf`/`.NAN` are
the float specials. 17 of 24 `schema-1-1-vs-1-2` cases match, 3 are deviations,
4 are the divergences above.

## Upstream API inventory — `yaml@2.6.1`

Derived mechanically:

```
cd node && node -e "console.log(Object.keys(require('yaml')).sort().join('\n'))"
```

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `yaml.parse` | `yaml.Unmarshal` | match | `unmarshal-*` (6) | |
| `yaml.parseAllDocuments` | `yaml.Parse`, `yaml.NewDecoder`/`Decoder.Decode` | differs | `accept-*` (308), `fail-*` (94), `decode-stream-*` (5) | port drops empty documents; port accepts duplicate keys |
| `yaml.parseDocument` | `yaml.Parse` (first document) | differs | `reemit-*` (308), `emit-*` | see the emitter defects above |
| `yaml.stringify` | `yaml.Marshal`, `Encoder.Encode` | differs | `marshal-*` (18), `encode-stream-*` (6) | `"\n"` round-trips to `""` |
| `yaml.Document` | `yaml.Node` (`DocumentNode`) | differs | `reemit-*`, `comment` cases | |
| `yaml.Scalar` | `yaml.Node` (`ScalarNode`) | differs | `reemit-*` | tag/anchor/comment emission |
| `yaml.YAMLMap` | `yaml.Node` (`MappingNode`) | differs | `reemit-*` | complex keys cannot be decoded |
| `yaml.YAMLSeq` | `yaml.Node` (`SequenceNode`) | differs | `reemit-*` | |
| `yaml.Alias` | `yaml.Node` (`AliasNode`), `Node.Alias` | differs | `reemit-E76Z`, `reemit-X38W`, `unmarshal-anchors` | alias-as-key emitted without a space |
| `yaml.Pair` | — | missing | — | the port stores a mapping as alternating `Node.Content`, with no key/value pair type |
| `yaml.Schema` | — | missing | — | the schema is fixed to YAML 1.2 core; not selectable |
| `yaml.Composer` | — | missing | — | no separately exposed event-to-tree stage |
| `yaml.Parser` | — | missing | — | no exposed event parser |
| `yaml.Lexer` | — | missing | — | no exposed lexer |
| `yaml.CST` | — | missing | — | no concrete syntax tree / lossless source model |
| `yaml.LineCounter` | `Node.Line`, `Node.Column` | missing | — | positions are on the node; no offset-to-line utility |
| `yaml.visit` | — | missing | — | no tree visitor (`Node.Content` is walked by hand) |
| `yaml.visitAsync` | — | missing | — | |
| `yaml.YAMLError` | `yaml.ErrSyntax` and friends | untested | — | only *whether* a call failed is compared, never the message (per `HARNESS.md`) |
| `yaml.YAMLParseError` | `*yaml.SyntaxError` | untested | — | carries line/column on both sides; not compared |
| `yaml.YAMLWarning` | — | missing | — | the port has no recoverable-warning channel; it either fails or succeeds |
| `yaml.isNode` | `Node.IsZero` (nearest) | untested | — | Go uses the concrete `*Node` type, so no predicate is needed |
| `yaml.isDocument` | `Node.Kind == DocumentNode` | untested | — | |
| `yaml.isMap` | `Node.Kind == MappingNode` | untested | — | |
| `yaml.isSeq` | `Node.Kind == SequenceNode` | untested | — | |
| `yaml.isScalar` | `Node.Kind == ScalarNode` | untested | — | |
| `yaml.isAlias` | `Node.Kind == AliasNode` | untested | — | |
| `yaml.isPair` | — | missing | — | no pair type |
| `yaml.isCollection` | `Node.Kind & (SequenceNode\|MappingNode)` | untested | — | `Kind` is a bit set for exactly this |

Upstream totals: 29 symbols — 1 `match`, 8 `differs`, 11 `missing`,
9 `untested`, 0 `extra`. Over the 9 symbols actually compared: **1 match,
8 differs = 11.1 %**. That number is misleading on its own and is not the
headline: `yaml@2.6.1` exposes a document-object model (CST, visitors, pluggable
schemas, a pair type) that the port deliberately does not have, and the port's
API is modelled on `gopkg.in/yaml.v3` instead. Every `differs` above is a real
defect listed in the sections above, not a shape difference.

## Port API inventory — `github.com/malcolmston/yaml v0.1.0`

Derived mechanically:

```
GOWORK=off go doc -all github.com/malcolmston/yaml
```

| Go symbol | status | cases | note |
| --- | --- | --- | --- |
| `yaml.Marshal` | differs | `marshal-*` (18), `reemit-*` (308) | defects A–E |
| `yaml.Unmarshal` | match | `unmarshal-*` (6) | |
| `yaml.Parse` | differs | `accept-*` (308), `fail-*` (94) | drops empty documents; accepts duplicate keys; accepts ill-formed UTF-8 |
| `yaml.NewDecoder` | match | `decode-stream-*` (5) | |
| `(*Decoder).Decode` | match | `decode-stream-*` (5) | including the `io.EOF` terminator |
| `yaml.NewEncoder` | match | `encode-stream-*` (6) | |
| `(*Encoder).Encode` | match | `encode-stream-*` (6) | |
| `(*Encoder).Close` | match | `encode-stream-*` (6) | |
| `(*Encoder).SetIndent` | match | `encode-stream-one-2`, `encode-stream-one-4` | value survives at both indents |
| `(*Node).Decode` | differs | `json-*` (279), `reemit-*` | cannot decode collection keys (13 cases) |
| `(*Node).Encode` | differs | `emit-line-comment*`, `emit-head-comment`, `emit-foot-comment` | see defect E |
| `yaml.Node.Kind` | untested | — | not observable through a JSON value |
| `yaml.Node.Tag` | differs | defect A/C tables | doubled in flow style, emitted bare when global, dropped for `!!timestamp` |
| `yaml.Node.Value` | untested | — | compared only through the decoded value |
| `yaml.Node.Anchor` | differs | `reemit-CN3R`, `reemit-6BFJ`, `reemit-E76Z` | doubled in flow style; alias key emitted without a space |
| `yaml.Node.Alias` | differs | `reemit-X38W`, `reemit-E76Z` | |
| `yaml.Node.Content` | untested | — | structural; compared only through the decoded value |
| `yaml.Node.Style` | differs | `emit-flow-*`, `reemit-K858`, `reemit-JEF9-01/02` | `FlowStyle` + tag doubles the tag; `LiteralStyle` loses a lone `"\n"` |
| `yaml.Node.Line` / `.Column` | untested | — | positions are not part of any compared value |
| `yaml.Node.HeadComment` | untested | `emit-head-comment`, `emit-head-comment-roundtrip` | the cases pass, but a head comment does not change the decoded value, so they cannot prove the text is right |
| `yaml.Node.LineComment` | differs | `emit-line-comment`, `emit-line-comment-string` | defect E — written with no `#` |
| `yaml.Node.FootComment` | differs | `emit-foot-comment-growth*`, `reemit-F8F9`, `reemit-JHB9` | defect D — duplicated per round trip |
| `(*Node).ShortTag` | untested | — | no oracle counterpart in `yaml@2` |
| `(*Node).LongTag` | untested | — | |
| `(*Node).IsZero` | untested | — | |
| `(*Node).SetString` | untested | — | |
| `(Kind).String` | untested | — | Go-only, no counterpart |
| `yaml.Kind` + `DocumentNode`/`SequenceNode`/`MappingNode`/`ScalarNode`/`AliasNode` | untested | — | not observable through a JSON value |
| `yaml.Style` + `TaggedStyle`/`FlowStyle` | differs | `emit-flow-*` | |
| `yaml.Style` + `DoubleQuotedStyle`/`SingleQuotedStyle` | match | every `double`- and `single`-tagged suite case re-emits (33/33 and 6/6) | |
| `yaml.Style` + `FoldedStyle` | differs | `reemit-F6MC` | a folded scalar with a more-indented first line re-emits as an unparseable literal block (21/25 folded cases re-emit) |
| `yaml.Style` + `LiteralStyle` | differs | `reemit-K858`, `reemit-JEF9-01/02`, `marshal-multiline` | a lone `"\n"` becomes `""` (31/36 literal cases re-emit) |
| `NullTag`/`BoolTag`/`IntTag`/`FloatTag`/`StringTag` | match | `schema-*`, `json-*` | resolution agrees with the oracle and the suite |
| `SequenceTag`/`MappingTag` | match | `json-*` | |
| `BinaryTag` | differs | `parse-binary-tag`, `json-565N` | documented deviation (decoded) |
| `TimestampTag` | differs | `emit-timestamp-tag*`, `parse-timestamp-tag` | dropped on emit |
| `MergeTag` | differs | `schema-merge-key` | documented deviation (resolved by default) |
| `ErrAliasBomb` | differs | `schema-alias-bomb` | documented deviation (higher threshold than the oracle) |
| `ErrSyntax` | untested | — | only failure/success is compared, not the sentinel |
| `ErrUnknownAnchor` | untested | — | observed in defect A messages, not asserted |
| `ErrTypeMismatch` | untested | — | |
| `ErrUnsupportedType` | untested | — | |
| `ErrInvalidUnmarshal` | untested | — | not reachable from a JSON-Lines case |
| `ErrClosed` | untested | — | |
| `*SyntaxError` (`.Line`, `.Column`, `.Msg`, `.Error`, `.Unwrap`) | untested | — | positions are not compared |
| `*TypeError` (`.Errors`, `.Error`, `.Unwrap`) | untested | — | |
| `yaml.Marshaler` | untested | — | Go-only extension point; no oracle counterpart |
| `yaml.Unmarshaler` | untested | — | Go-only extension point |

Port totals: 48 rows — 11 `match`, 17 `differs`, 0 `missing`, 20 `untested`,
0 `extra`. Nothing is `missing` because the port *is* the thing being scored, and
nothing is `extra` because the whole exported surface is listed. Over the 28
symbols actually compared: **11 match, 17 differs = 39.3 %**.

The two percentages answer different questions and should be quoted together
with the case-level number: **93.05 % of 1064 compared cases agree**, the port
is **100 % conformant** on the suite's accept and must-fail corpora, and 17 of
the 28 exported symbols the harness can compare have at least one confirmed
defect — concentrated entirely in the emitter and in complex-key decoding. The
parser is the strong half; the emitter is the weak one.

## Suite coverage by section

The test suite has no chapters; it tags each test. Every tagged test is in the
corpus, so this is a coverage table by construction — the columns show
match/total per case shape.

| suite tag | accept | must-fail | to-JSON | re-emit | mismatches |
| --- | --- | --- | --- | --- | --- |
| `spec` | 113/114 | — | 105/108 | 100/114 | 18 |
| `mapping` | 95/96 | 30/30 | 77/79 | 79/96 | 20 |
| `scalar` | 83/83 | 10/10 | 82/82 | 81/83 | 2 |
| `whitespace` | 76/76 | 16/16 | 74/74 | 72/76 | 4 |
| `sequence` | 59/59 | 23/23 | 50/50 | 48/59 | 11 |
| `flow` | 60/60 | 19/19 | 49/49 | 52/60 | 8 |
| `literal` | 36/36 | 3/3 | 36/36 | 31/36 | 5 |
| `double` | 33/33 | 12/12 | 33/33 | 33/33 | 0 |
| `tag` | 35/35 | 5/5 | 33/34 | 27/35 | 9 |
| `indent` | 28/28 | 13/13 | 28/28 | 26/28 | 2 |
| `comment` | 29/30 | 9/9 | 26/27 | 25/30 | 7 |
| `1.3-err` | 29/30 | — | 26/27 | 27/30 | 5 |
| `folded` | 25/25 | 4/4 | 23/23 | 21/25 | 4 |
| `error` | — | 72/74 | — | — | 2 |
| `1.3-mod` | 22/22 | — | 21/21 | 21/22 | 1 |
| `header` | 19/19 | 4/4 | 19/19 | 15/19 | 4 |
| `anchor` | 18/18 | 7/7 | 15/15 | 14/18 | 4 |
| `explicit-key` | 22/22 | — | 12/13 | 15/22 | 8 |
| `directive` | 15/15 | 8/10 | 15/15 | 8/15 | 9 |
| `alias` | 16/16 | 2/2 | 14/14 | 11/16 | 5 |
| `upto-1.2` | 12/12 | — | 12/12 | 12/12 | 0 |
| `footer` | 5/8 | 5/5 | 5/8 | 6/8 | 8 |
| `unknown-tag` | 9/9 | 1/1 | 7/9 | 3/9 | 8 |
| `empty-key` | 11/12 | — | 1/1 | 9/12 | 4 |
| `edge` | 9/9 | 1/1 | 5/5 | 7/9 | 2 |
| `local-tag` | 8/8 | — | 8/8 | 6/8 | 2 |
| `single` | 6/6 | 3/3 | 6/6 | 6/6 | 0 |
| `complex-key` | 8/8 | — | — | 0/8 | 8 |
| `libyaml-err` | 4/4 | — | 4/4 | 4/4 | 0 |
| `empty` | 2/2 | — | 2/2 | 2/2 | 0 |
| `simple` | 2/2 | — | 2/2 | 2/2 | 0 |
| `duplicate-key` | 0/1 | — | — | 0/1 | 2 |
| `document` | — | 1/1 | — | — | 0 |

A test carries several tags, so the rows overlap. `complex-key` (0/8 on
re-emit), `directive` (8/15) and `unknown-tag` (3/9) are the weakest sections;
`double`, `single`, `upto-1.2` and `libyaml-err` are clean throughout.

One suite file is excluded: `ZYU8` carries `skip: true` with the suite's own
note that its documents are "valid according to the 1.2 productions but not at
all usefully valid" and that processors should not be encouraged to support
them. 402 tests remain, from 350 files.

## How the harness works

```
parity/yaml/
├── cases/                     # 8 generated case files, 1071 cases
├── node/
│   ├── gen-cases.mjs          # the generator: suite checkout -> cases/*.json
│   ├── run.mjs                # oracle runner (yaml@2.6.1)
│   └── package.json           # pins yaml@2.6.1
├── go/run.go                  # port runner (github.com/malcolmston/yaml v0.1.0)
├── parity_test.go             # drives both, compares, writes parity.json
├── COVERAGE.md
└── parity.json
```

Regenerate the corpus after a suite bump:

```
git clone https://github.com/yaml/yaml-test-suite /tmp/yaml-test-suite
cd parity/yaml/node && npm install && node gen-cases.mjs /tmp/yaml-test-suite
```

then update `suiteRev` in `parity_test.go` to the new commit — the test refuses
to run against case files whose `upstream` pin does not match, so the score can
never be attributed to the wrong corpus.

### Case shapes

| `fn` | args | what it answers |
| --- | --- | --- |
| `parse` | document | acceptance: `ok:false` when the side rejects it; `{docs: n}` otherwise |
| `parseJSON` | document | one normalised value per document |
| `reemit` | document, n | emit the first document's tree `n` times, re-parsing between rounds: `{emitOK, reparseOK, decodeOK, stable, grew, value}` |
| `marshal` | value | emit a value and read it back: `{emitOK, reparseOK, stable, value}` |
| `comment` | value, kind, text | attach a head/line/foot comment to a mapping value, emit, read back |
| `unmarshal` | document | single-document decode (`yaml.Unmarshal` / `yaml.parse`) |
| `decodeStream` | document | stream decode (`Decoder` / `parseAllDocuments`) |
| `encodeStream` | values, indent | stream encode at an indent, then read back |
| `wellFormedUTF8` | document | asserts a fixture really is ill-formed UTF-8 |

A document argument is a JSON string, or `{"hex": "..."}` for bytes that are not
valid UTF-8 and therefore cannot be written as a JSON string.

### Value normalisation

Both runners project a decoded YAML value into the same JSON shape, so a
mismatch is always about YAML and never about how Go and JavaScript happen to
spell things:

* mapping keys are sorted; a non-string key becomes the JSON text of its
  normalised form (`1` → `"1"`, `[1,2]` → `"[1,2]"`);
* `NaN`/`±Inf` become `".nan"`/`".inf"`/`"-.inf"`, which JSON has no literals for;
* bytes become `{"!!binary": "<lowercase hex>"}`;
* a timestamp becomes `{"!!timestamp": "<ISO 8601, UTC, milliseconds>"}` —
  *tagged*, so an emitter that drops `!!timestamp` cannot pass by producing the
  equal plain string;
* the harness compares all JSON numbers as `float64`, so `1` and `1.0` agree.

For **conformance** only, the `!!binary`/`!!timestamp` wrappers are stripped and
whole floats are compared as integers, because the suite's `json` field has no
notion of either.

Known normalisation limits, both symmetric and both noted where they bite: a
`!!set` becomes `{}` on the JavaScript side (a `Set` has no enumerable entries),
and ill-formed UTF-8 reaches the oracle latin1-decoded because a JavaScript
string cannot hold it.

### Runner contract additions

Beyond `HARNESS.md`, a response may carry a `"detail"` object. It is **never
compared** — the harness only prints it next to a mismatch — and is what makes
the emitter findings above readable: it holds the YAML each side actually
emitted, and the error it failed to re-parse with. Both runners catch every
throw/panic into `ok:false` and keep reading; neither exits on a failing case.

Case files carry three extra fields so the conformance score can exist:
`expect` (`"accept"`/`"fail"`), `suite` (the suite's test id), `suiteTags`, and
`suiteJSON` (the suite's expected JSON text).

### Skips

`go test` skips, never fails, when `node` is absent, when `npm install` cannot
provide `yaml@2.6.1`, or when `cases/` is empty. The corpus **was** fetched for
this report — `yaml/yaml-test-suite` at `da267a5c` — so nothing here is a
hand-built fallback.
