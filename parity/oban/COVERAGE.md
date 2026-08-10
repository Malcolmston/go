# oban — upstream API inventory vs the Go port

- **Upstream**: [`sorentwo/oban`](https://github.com/sorentwo/oban), Elixir, pinned to
  **oban 2.23.1** (declared `{:oban, "~> 2.18"}`, resolved and locked by
  `elixir/mix.lock`; the runner is an escript built from `elixir/`).
- **Port**: `github.com/malcolmston/oban@v0.0.0-20260810111557-dd5f0675354a`,
  consumed as a published module (no `replace` directive).
- **Toolchain used**: Elixir 1.20.3 / Erlang OTP 29, Go 1.26.5.
- **Score**: see `parity.json`, rewritten by `go test`.

## What can and cannot be compared here

Oban is, in its own words, a *Postgres-backed* job processing system. Its queues,
staging, uniqueness, leadership, notifications, plugins, telemetry and the whole
execution engine are implemented as SQL against an `Ecto.Repo`. There is no
PostgreSQL in this environment and none was stood up, so **no live job execution
is compared**: every module that needs a repo or a running supervision tree is
marked `untested` with that reason, not guessed at.

What is genuinely comparable is the pure, database-free arithmetic:

| area | upstream module | comparable? |
| --- | --- | --- |
| cron parsing and next/last-run computation | `Oban.Cron.Expression` | yes — 133 cases |
| retry backoff arithmetic | `Oban.Backoff`, `Oban.Worker.backoff/1` | yes — 16 cases |
| job/option normalisation | `Oban.Job.new/2` | yes — 16 cases |
| unique-job keys and de-duplication | `Oban.Engines.Basic` (`unique_query/1` and friends) | **no** — upstream expresses uniqueness as an `Ecto.Query` dynamic evaluated by Postgres; there is no repo-free entry point, so this is `untested` rather than faked |
| queues, producers, staging, draining | `Oban`, `Oban.Queue.*`, `Oban.Engines.*` | no — needs Postgres |
| plugins (Cron, Lifeline, Pruner, Reindexer, Gossip, Repeater) | `Oban.Plugins.*` | no — all of them read/write the `oban_jobs` table |
| leadership, notifications, telemetry, migrations, testing helpers | `Oban.Peer(s)`, `Oban.Notifier(s)`, `Oban.Telemetry`, `Oban.Migration(s)`, `Oban.Testing` | no — needs Postgres and/or a live supervision tree |

Additional deliberate restrictions of the comparison, so that the numbers mean
something:

- **Timezone is fixed to `Etc/UTC` on both sides.** Upstream's `next_at/1` and
  `last_at/1` accept a timezone string and call `DateTime.now!/1`, which needs a
  tz database; this project does not depend on `tzdata`, and the Go port's
  `Schedule.Next` merely preserves the location of the instant it is given.
  Non-UTC and DST behaviour is therefore listed as `untested`, not as a match.
- **Every cron case carries an explicit reference instant** as an absolute UTC
  ISO-8601 string. Neither runner ever reads the wall clock, so the arities that
  default to "now" (`now?/1`, `next_at/1`, `last_at/1`) are out of scope.
- **All fire times are emitted as `YYYY-MM-DDTHH:MM:SSZ`** by both runners.
- **Backoff jitter.** Upstream's default worker backoff always adds jitter
  (`Oban.Backoff.jitter(mode: :inc, mult: 0.1)`) and there is no option to turn
  it off, so the *deterministic component* is compared instead: the Elixir runner
  calls upstream's own `Oban.Backoff.exponential/2` with exactly the option set
  `Oban.Worker.backoff/1` passes it (`mult: 1, max_pow: 100, min_pad: 15`) and
  re-states only the three-line `clamped_attempt` scaling, which is
  unobservable through the jittered public call. The jittered value itself is
  compared against its documented bounds `[base, base + 10%]` by the
  `worker-backoff-bounds-*` cases.
- **`Oban.Job.new/2` leaves `priority` as `nil`** in the changeset and relies on
  the `oban_jobs.priority` column default of `0`; the Elixir runner reports
  `nil` as `0` so that the comparison is about the option handling rather than
  about a database default. Timestamps are never compared, because upstream
  resolves `:schedule_in` against `DateTime.utc_now/0`.

## How the upstream inventory was produced

Mechanically, by reflection over the *installed* 2.23.1 package — not from the
README and not from memory:

```elixir
# run from parity/oban/elixir with MIX_ENV=prod mix run --no-start
Application.load(:oban)

for m <- Enum.sort(Application.spec(:oban, :modules)) do
  exports =
    m.module_info(:exports)
    |> Enum.reject(fn {f, _a} ->
      f in [:module_info, :__info__, :__struct__, :__impl__, :behaviour_info,
            :__protocol__, :__using__, :__after_compile__, :__opts__]
    end)
    |> Enum.reject(fn {f, _a} -> String.starts_with?(Atom.to_string(f), "-") end)
    |> Enum.sort()
    |> Enum.map(fn {f, a} -> "#{f}/#{a}" end)

  hidden? =
    match?({:docs_v1, _, _, _, h, _, _} when h in [:hidden, :none], Code.fetch_docs(m))

  IO.puts("#{inspect(m)}#{if hidden?, do: " [hidden]"}: #{Enum.join(exports, ", ")}")
end
```

That yields **70 modules and 621 exported functions/macros**; 41 of the modules
carry `@moduledoc false` and are marked *(hidden)* below. Compiler-generated
noise (`module_info`, `__info__`, `__struct__`, anonymous-function stubs) is
filtered out; Ecto-generated schema reflection (`__changeset__/0`,
`__schema__/1,2`) is kept in the table but marked `n/a`.

The Go side was enumerated with `GOWORK=off go doc -all github.com/malcolmston/oban`
over the same module version that `go.mod` pins.

A note on hidden modules: `Oban.Cron.Expression` and `Oban.Backoff` are both
`@moduledoc false`, i.e. private by convention. They are nevertheless the
comparison targets here, because they are the only places where Oban's
database-free arithmetic lives, and the Go port's `cron_parity_test.go` cites
`Oban.Cron.Expression` explicitly as the thing it mirrors.

## `Oban.Cron.Expression` *(hidden)* — 8 exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Oban.Cron.Expression.parse/1` | `oban.ParseCron`, `oban.ParseCronSpec` | differs | all 57 `cron_parse` cases: 46 `parse-*` + 11 `macro-*` (49/57 agree) | error-returning arity, the one the Go runner actually calls; same code path as `parse!/1`. Divergences below. |
| `Oban.Cron.Expression.parse!/1` | `oban.MustParseCron`, `oban.MustParseCronSpec` | differs | same as above | raising arity; upstream raises `ArgumentError`, the port panics |
| `Oban.Cron.Expression.now?/2` | `(*oban.Schedule).Matches` | differs | 17 `now-*` + `macro-reboot-now` (17/18 agree) | only `@reboot` diverges; the day-of-month/day-of-week AND rule agrees exactly |
| `Oban.Cron.Expression.now?/1` | — | untested | — | defaults to `DateTime.utc_now/0`; excluded as non-deterministic |
| `Oban.Cron.Expression.next_at/2` | `(*oban.Schedule).Next` | differs | 26 `next-*` + 8 `macro-*` + 9 `upcoming-*` (41/43 agree) | only `@reboot` and weekday `7` diverge |
| `Oban.Cron.Expression.next_at/1` | — | untested | — | timezone-string arity; needs a tz database (see above) |
| `Oban.Cron.Expression.last_at/2` | `(*oban.Schedule).Prev` | match | 14 `prev-*` + `macro-daily-prev` (15/15 agree) | |
| `Oban.Cron.Expression.last_at/1` | — | untested | — | timezone-string arity, and for `@reboot` it is derived from VM uptime, which is not reproducible |

Cron behaviour that **does** agree, verified case-by-case: `*`, single values,
lists, ranges, `range/step`, `*/step`, the `value/step` open-range form,
three-letter uppercase month and weekday names (including in range bounds),
minute/hour/day/month/weekday bounds checking, the `validate_days` rejection of
impossible day-month pairings (Feb 30, Apr 31), leap-day scheduling in both
directions, wrong field counts, inverted and out-of-range ranges, zero steps,
the `@hourly`/`@daily`/`@midnight`/`@weekly`/`@monthly`/`@yearly`/`@annually`
macros, sub-minute truncation of the reference instant, and — importantly — the
**day-field combination rule**. Both implementations **AND** day-of-month with
day-of-week; neither implements the Vixie-cron "either day field" OR rule. The
`now-both-days-*` and `next-both-days-*` cases pin this down: `0 0 13 * 5` does
*not* fire on Tuesday the 13th and does *not* fire on Friday the 6th in either
implementation.

### Real cron divergences

| # | input | upstream 2.23.1 | Go port | cases |
| --- | --- | --- | --- | --- |
| 1 | `0 0 * * 7` | rejected — the weekday range is `0..6` | accepted, `7` folded onto Sunday | `parse-dow-7`, `next-dow-7-alias` |
| 2 | `0 0 * * 1-7` | rejected — range end out of `0..6` | accepted | `parse-dow-range-to-7` |
| 3 | `0 0 * * sun`, `0 0 * jan *` | rejected — `trans_field/2` substitutes only the uppercase names | accepted, names are lower-cased before lookup | `parse-lowercase-dow`, `parse-lowercase-month` |
| 4 | `0 0 * * Mon` | rejected — mixed case is not substituted either | accepted | `parse-mixed-case-dow` |
| 5 | `*/100 * * * *` | rejected — a step is restricted to `1..99` by regex | accepted (step 100 on 0..59 yields just minute 0) | `parse-step-100` |
| 6 | `@reboot` | parses to `%Expression{reboot?: true}`; `now?/2` is unconditionally `true`, `next_at/2` returns `:unknown`, `last_at/2` is derived from VM uptime | not a known macro — rejected | `macro-reboot-parse`, `macro-reboot-now`, `macro-reboot-next` |
| 7 | `@DAILY` | rejected — the macro clause heads are literal lowercase strings | accepted, macros are matched case-insensitively | `macro-uppercase` |

Divergences 1–4 are a deliberate superset in the port: `cron.go` and
`cron_parity_test.go` both say so in prose ("Two upstream behaviours are
deliberately supersetted by this port … weekday 7 as an alias for Sunday and
lowercase three-letter month/weekday names"). They are **not** marked
`"deviation"` in the case files, because `HARNESS.md` requires a deviation to be
listed in the library's `API-DEVIATIONS.md` and that file does not exist in the
`oban` repo. They are scored as differences here. Divergences 5–7 are not
documented anywhere in the port.

Also worth recording, though not a divergence in behaviour: upstream's
`Oban.Cron.Expression` struct exposes its parsed field sets (`minutes`, `hours`,
`days`, `months`, `weekdays` as `MapSet`s) while the Go `Schedule` keeps them in
unexported bitmasks. The parsed sets are therefore compared *indirectly*,
through `now?`/`next_at`/`last_at`/`Upcoming` over explicit instants, rather than
by reading the fields out.

## `Oban.Backoff` *(hidden)* — 6 exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Oban.Backoff.exponential/2` | — | missing | `exp-attempt-1`, `exp-mult-100`, `exp-min-pad-10`, `exp-attempt-10`, `exp-attempt-11-clamped` | no Go counterpart: the port has no exponential helper parameterised by `mult`/`max_pow`/`min_pad`, and `ExponentialBackoff` cannot express `min_pad` at all. All five cases fail on the Go side. |
| `Oban.Backoff.exponential/1` | — | missing | — | default-options arity of the above |
| `Oban.Backoff.jitter/2` | `oban.ExponentialBackoff.Jitter` field | differs | `worker-backoff-jitter-mode` | upstream jitter has three modes (`:inc`/`:dec`/`:both`) and a multiplier; the port has a single `Jitter` fraction that only ever *subtracts*, i.e. it is `:dec`-shaped, and the default policy applies none at all |
| `Oban.Backoff.jitter/1` | — | missing | — | default-options arity |
| `Oban.Backoff.with_retry/1`, `with_retry/2` | — | missing | — | retries a *database* interaction on `DBConnection`/`Postgrex` errors; not applicable without a repo |

## `Oban.Worker` — 7 exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Oban.Worker.backoff/1` | `(*oban.ExponentialBackoff).Next` (the default when `Config.Backoff` is nil) | differs | `worker-backoff-1/2/3/5/10/20`, `worker-backoff-clamped-attempt`, `worker-backoff-bounds-1/5/20` (3/10 agree) | **the default retry schedule is not the same algorithm** — see below |
| `Oban.Worker.timeout/1` | `oban.Config.JobTimeout` / `WorkerOptions` | untested | — | upstream default is `:infinity` per job; the port has an engine-level per-attempt timeout. Different shape, no repo-free comparison. |
| `Oban.Worker.to_string/1` | — | missing | — | module-name normalisation; the port names workers with plain strings |
| `Oban.Worker.from_string/1` | `(*oban.Registry).Lookup` | untested | — | upstream resolves a worker *module* from a string; the port resolves a `Worker` from a registry, which is not the same operation |
| `Oban.Worker.merge_opts/2` | `oban.WorkerOptions` | untested | — | `use Oban.Worker` option merging has no analogue; Go workers are configured with a struct |
| `MACRO-Oban.Worker.__using__/2` | `oban.Worker` interface, `oban.WorkerFunc` | untested | — | `use`-macro code generation; not a comparable call |
| `MACRO-Oban.Worker.__after_compile__/3` | — | n/a | — | compile-time option validation |

### Real backoff divergence

Upstream's default worker backoff, in seconds, is
`15 + 2^clamped_attempt` (`Oban.Backoff.exponential(clamped, mult: 1, max_pow: 100, min_pad: 15)`),
plus up to 10% increasing jitter, where `clamped_attempt` is the attempt scaled
into `1..20` when `max_attempts > 20`. The port's default is
`ExponentialBackoff{}` = `1s * 2^(attempt-1)`, capped at 5 minutes, with no
jitter and no clamping of the attempt against `max_attempts`. Observed:

| attempt (max_attempts) | upstream, deterministic part | Go port |
| --- | --- | --- |
| 1 (20) | 17 s | 1 s |
| 2 (20) | 19 s | 2 s |
| 3 (20) | 23 s | 4 s |
| 5 (20) | 47 s | 16 s |
| 10 (20) | 1039 s | 300 s (cap) |
| 20 (20) | 1048591 s ≈ 12.1 days | 300 s (cap) |
| 25 (50) | 1039 s (attempt scaled to 10) | 300 s (cap; `max_attempts` ignored) |

So three separate differences: the formula (`15 + 2^n` vs `2^(n-1)` seconds), the
5-minute cap the port imposes and upstream does not, and the attempt clamping
against `max_attempts` that upstream does and the port does not. Only the
`worker-backoff-bounds-*` cases agree, and only because both sides trivially sit
inside their own bounds — upstream because its jitter is bounded at +10%, the
port because it applies no jitter.

## `Oban.Job` — 16 exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Oban.Job.new/2` | `oban.NewJob` + `oban.JobOption`s | differs | all 16 `job-*` cases (11/16 agree) | agrees on the default queue (`default`), default `max_attempts` (20), default state (`available`), explicit queue, explicit valid `max_attempts` and `priority`, `schedule_in` normalising the state to `scheduled`, args round-tripping (including nesting, key ordering, floats, booleans and nulls), the empty-worker rejection, and the unknown-option rejection. Diverges on validation, below. |
| `Oban.Job.new/1` | `oban.NewJob` | untested | — | args-only arity; the port always requires a worker name |
| `Oban.Job.to_map/1` | — | untested | — | changeset → insertable map; the Elixir runner uses `Ecto.Changeset.apply_action!/2` instead so that invalid changesets surface as `ok:false` |
| `Oban.Job.update/2` | `oban.ReplaceField` | untested | — | changeset update path used by `:replace`; not exercised without a repo |
| `Oban.Job.states/0` | `oban.State` constants | untested | — | the port has the same seven states as typed constants but no enumerating function |
| `Oban.Job.unique_states/1` | `oban` internal `unfinishedStates` | untested | — | the port's equivalent set is unexported, so there is nothing to call |
| `Oban.Job.validate_unique/1` | `oban.UniqueBy` | untested | — | option validation for uniqueness; the port validates nothing up front |
| `Oban.Job.warn_unique/1` | — | missing | — | deprecation warning for legacy unique options |
| `Oban.Job.validate_replace/1` | `oban.ReplaceField` | untested | — | |
| `Oban.Job.cast_period/1` | — | missing | — | `{5, :minutes}` → seconds; the port takes a `time.Duration` and needs no coercion |
| `Oban.Job.cast_unique_group/1` | — | missing | — | expands `:incomplete`/`:successful`/… state groups |
| `Oban.Job.format_attempt/1` | `oban.AttemptError` | missing | — | formats an `unsaved_error` into the `errors` array |
| `Oban.Job.query/1` | `oban.JobFilter` | untested | — | builds an `Ecto.Queryable`; Postgres-only for `args`/`meta` containment |
| `Oban.Job.__changeset__/0`, `__schema__/1`, `__schema__/2` | — | n/a | — | Ecto schema reflection |

### Real job divergences

| # | input | upstream 2.23.1 | Go port | cases |
| --- | --- | --- | --- | --- |
| 1 | `max_attempts: 0` / `-1` | rejected (`validate_number greater_than: 0`) | accepted, silently replaced by the default 20 | `job-max-attempts-zero`, `job-max-attempts-negative` |
| 2 | `priority: 10` / `-1` | rejected (`validate_number` to `0..9`) | accepted verbatim — `oban.NewJob` does no range check; the port exposes `oban.ValidatePriority` as a separate call the constructor never makes | `job-priority-10`, `job-priority-negative` |
| 3 | `state: "scheduled"` | accepted, `:state` is a permitted param | no such option on `oban.NewJob` | `job-explicit-state` |

Fields upstream has that the port's `Job` struct does not: `meta`, `tags`,
`attempted_by`, `cancelled_at`, `conf`, `conflict?`, `replace`, `unsaved_error`.
`tags` in particular has real normalisation logic upstream (trim, downcase,
reject empties, dedupe) with nothing to compare it against — the port only
offers a store-level `oban.Tags` helper. All `missing`.

## `Oban.Period` — 3 exports

| upstream symbol | Go symbol | status | cases | note |
| --- | --- | --- | --- | --- |
| `Oban.Period.to_seconds/1` | — | missing | — | `{5, :minutes}` → 300; the port uses `time.Duration` throughout |
| `MACRO-Oban.Period.is_seconds/2` | — | missing | — | guard macro |
| `MACRO-Oban.Period.is_valid_period/2` | — | missing | — | guard macro |

## Go-only symbols (`extra`)

The port is a superset in several places; none of these has an upstream
counterpart to compare against, so they carry no cases:

| Go symbol | note |
| --- | --- |
| `oban.Schedule.Upcoming` | n next fire times; the harness composes upstream `next_at/2` in a loop to reach parity for it (`upcoming-*` cases) |
| `oban.Schedule.String` | returns the normalised expression |
| `oban.ConstantBackoff`, `oban.LinearBackoff`, `oban.FibonacciBackoff`, `oban.BackoffFunc` | alternative retry policies; upstream documents constant/linear/squared/sidekiq shapes in prose only and ships none of them |
| `oban.NewExponentialBackoff` (seeded jitter source) | upstream jitter is unseedable `:rand` |
| `oban.ValidatePriority` | the 0..9 check upstream performs inline inside `Oban.Job.new/2` |
| `oban.InMemoryStore`, `oban.SQLStore`, `oban.ControlledStore`, `oban.Store` and the optional capability interfaces | upstream has no pluggable storage layer; it is Ecto/Postgres or nothing |
| `oban.Clock`, `oban.SystemClock` | injectable time; upstream calls `DateTime.utc_now/0` directly |
| `oban.Middleware`, `oban.Recorder`, `oban.RateLimiter`, `oban.Controller`, `oban.Supervisor` | Go-side engine plumbing |

## Appendix — the remaining 65 modules, all `untested`

Every module below needs PostgreSQL, an `Ecto.Repo`, or a live supervision
tree, so none of it is comparable in this environment. Listed for completeness,
with the export count from the reflection command above.

| upstream module | exports | Go symbol | status | reason |
| --- | --- | --- | --- | --- |
| `Inspect.Oban.Cron.Expression` *(hidden)* | 1 | — | untested | `Inspect` protocol impl |
| `Mix.Tasks.Oban.Install` | 1 | — | untested | Mix installer task |
| `Mix.Tasks.Oban.Install.Docs` *(hidden)* | 3 | — | untested | Mix task docs |
| `Oban` | 55 | `oban.Oban`, `oban.New`, … | untested | the whole public façade: `insert`, `insert_all`, `cancel_*`, `retry_*`, `pause_*`, `scale_queue`, `drain_queue`, `check_queue`, … all issue SQL |
| `Oban.Application` *(hidden)* | 2 | — | untested | OTP application callback |
| `Oban.Config` | 7 | `oban.Config` | untested | validates a repo, engine, notifier, peer and plugin list — none of which exists in the port's config |
| `Oban.CrashError` | 3 | — | untested | exception raised by the executor |
| `Oban.Cron` *(hidden)* | 5 | `oban.CronPlugin` | untested | schedules the periodic tick inside a running process |
| `Oban.Engine` | 30 | `oban.Store` | untested | the engine behaviour: fetch/complete/discard/snooze/retry, all SQL |
| `Oban.Engines.Basic` | 32 | `oban.SQLStore` | untested | Postgres engine, including `unique_query/1` — the unique-key logic named in the task, which has no repo-free entry point |
| `Oban.Engines.Dolphin` | 30 | — | untested | MySQL engine |
| `Oban.Engines.Inline` | 30 | `oban.Drain` | untested | executes inline but still through `Ecto` changesets and a repo |
| `Oban.Engines.Lite` | 30 | — | untested | SQLite engine |
| `Oban.Errors` *(hidden)* | 2 | — | untested | database-error macro list |
| `Oban.Harbor` *(hidden)* | 5 | — | untested | supervision helper |
| `Oban.JSON` *(hidden)* | 4 | — | untested | JSON shim |
| `Oban.Midwife` *(hidden)* | 7 | — | untested | starts/stops queue supervisors |
| `Oban.Migration` | 4 | — | untested | schema migration entry point |
| `Oban.Migrations` *(hidden)* | 2 | — | untested | migration dispatch |
| `Oban.Migrations.MyXQL` *(hidden)* | 3 | — | untested | MySQL migrations |
| `Oban.Migrations.Postgres` *(hidden)* | 5 | — | untested | Postgres migrations |
| `Oban.Migrations.Postgres.V01`–`V14` *(hidden)* | 2 each (28) | — | untested | versioned migration steps |
| `Oban.Migrations.SQLite` *(hidden)* | 3 | — | untested | SQLite migrations |
| `Oban.Notifier` | 15 | `oban.Telemetry`, `oban.EventHandler` | untested | pub/sub across nodes; upstream default rides Postgres `LISTEN/NOTIFY` |
| `Oban.Notifiers.Isolated` *(hidden)* | 12 | — | untested | test notifier |
| `Oban.Notifiers.PG` | 12 | — | untested | `:pg`-based notifier, needs distribution |
| `Oban.Nursery` *(hidden)* | 6 | — | untested | dynamic supervisor |
| `Oban.Peer` | 6 | — | untested | leader election |
| `Oban.Peers.Database` | 9 | — | untested | leader election via the database |
| `Oban.Peers.Global` | 9 | — | untested | leader election via `:global` |
| `Oban.Peers.Isolated` *(hidden)* | 9 | — | untested | test peer |
| `Oban.PerformError` | 3 | `oban.AttemptError` | untested | exception wrapper around a failed `perform/1` |
| `Oban.Plugin` | 4 | `oban.Plugin` | untested | plugin behaviour; every shipped plugin needs a repo |
| `Oban.Plugins.Cron` | 8 | `oban.CronPlugin` | untested | inserts the cron-scheduled jobs — the *scheduling arithmetic* it delegates to `Oban.Cron.Expression` **is** compared above; the insertion is not |
| `Oban.Plugins.Gossip` *(hidden)* | 2 | — | untested | deprecated |
| `Oban.Plugins.Lifeline` | 8 | `oban.Lifeline` | untested | rescues orphaned `executing` rows |
| `Oban.Plugins.Pruner` | 8 | `oban.Pruner` | untested | deletes old rows |
| `Oban.Plugins.Reindexer` | 8 | — | untested | `REINDEX CONCURRENTLY` |
| `Oban.Plugins.Repeater` *(hidden)* | 2 | — | untested | deprecated |
| `Oban.Queue.Drainer` *(hidden)* | 1 | `oban.Drain`, `oban.DrainQueue` | untested | drains through the engine |
| `Oban.Queue.Executor` *(hidden)* | 10 | `oban.PerformJob`, `oban.RunJob` | untested | records the outcome back to the database |
| `Oban.Queue.Producer` *(hidden)* | 9 | — | untested | polls the database for available jobs |
| `Oban.Queue.Supervisor` *(hidden)* | 3 | — | untested | per-queue supervision |
| `Oban.Queue.Watchman` *(hidden)* | 6 | — | untested | graceful shutdown |
| `Oban.Registry` | 8 | `oban.Registry` | untested | upstream registers *processes*; the port's `Registry` maps worker names to workers — same word, different job |
| `Oban.Repo` | 28 | — | untested | the `Ecto.Repo` wrapper; the entire reason Postgres is required |
| `Oban.Sonar` *(hidden)* | 8 | — | untested | cluster connectivity tracking |
| `Oban.Stager` *(hidden)* | 8 | — | untested | moves `scheduled` → `available` |
| `Oban.Telemetry` | 8 | `oban.Telemetry`, `oban.Event` | untested | `:telemetry` spans around database work |
| `Oban.Testing` | 30 | `oban.AssertEnqueued`, `oban.RefuteEnqueued`, `oban.CountJobs` | untested | every assertion queries the `oban_jobs` table |
| `Oban.TimeoutError` | 3 | — | untested | exception |
| `Oban.Validation` *(hidden)* | 8 | — | untested | option-schema validation |

## Counts

Symbol-level, over the whole upstream surface:

| status | symbols | note |
| --- | --- | --- |
| `match` | 1 | `Oban.Cron.Expression.last_at/2` |
| `differs` | 7 | `Expression.parse/1`, `parse!/1`, `now?/2`, `next_at/2`, `Backoff.jitter/2`, `Worker.backoff/1`, `Job.new/2` |
| `missing` | 13 | no Go counterpart at all: `Backoff.exponential/1,2`, `Backoff.jitter/1`, `Backoff.with_retry/1,2`, `Worker.to_string/1`, `Job.warn_unique/1`, `Job.cast_period/1`, `Job.cast_unique_group/1`, `Job.format_attempt/1`, and all 3 `Oban.Period` exports |
| `extra` | 8 groups | Go-only, listed above |
| `untested` | 596 | 581 exports across the 65 Postgres-bound modules, plus 15 of the 40 exports in the five in-scope modules |
| `n/a` | 4 | Ecto/compile-time generated (`Job.__changeset__/0`, `Job.__schema__/1,2`, `Worker.__after_compile__/3`) |

1 + 7 + 13 + 596 + 4 = 621, the full export count.

A symbol counts as `match` only when **every** case attributed to it agrees.
Two parity numbers, with their denominators stated explicitly:

- **Case pass rate: 136 / 165 = 82.4 %** — the denominator is every case in
  `cases/*.json` that both runners answered.
- **Symbol match rate: 1 / 8 = 12.5 %** — the denominator is the eight upstream
  symbols named in `upstreamFn` across the case files, i.e. the ones that
  actually have at least one case: `Expression.parse!/1` (`parse/1` shares its
  code path and is not counted twice), `Expression.now?/2`, `next_at/2`,
  `last_at/2`, `Backoff.exponential/2`, `Backoff.jitter/2`,
  `Worker.backoff/1`, `Job.new/2`. Seven of the eight have at least one
  disagreeing case, which is why the two numbers are so far apart: the cron
  divergences are a handful of input forms out of 133 cron cases, but they land
  on three different cron symbols.

Neither number should be read as "82 % of Oban is ported". 596 of Oban's 621
exports are untested here because they require PostgreSQL, and that is the
single most important fact in this file.
