#!/usr/bin/env bash
#
# split-passport-strategies.sh — build standalone repo payloads for every
# passport strategy, from passport/strategies.json.
#
#   node --experimental-strip-types scripts/subrepo/gen-passport-manifest.ts
#   scripts/subrepo/split-passport-strategies.sh --layer 1          # stage
#   scripts/subrepo/split-passport-strategies.sh --layer 1 --push   # stage + push
#
# WHAT IT DOES, PER STRATEGY
#   1. copies passport/strategies/<name>/ into a staging repo
#   2. writes go.mod:  module github.com/malcolmston/passport-<name>
#                      require github.com/malcolmston/passport v<core>
#                      require each sibling from the manifest
#   3. rewrites every  github.com/malcolmston/passport/strategies/<sib>
#                 ->   github.com/malcolmston/passport-<sib>
#   4. adds LICENSE (from passport) and VERSION if absent
#   5. git init + commit; with --push, pushes to origin
#
# THE REPOSITORIES MUST ALREADY EXIST. This script does not create them: the
# session token used to build this tooling is scoped without `Administration:
# write`, so `POST /user/repos` returns 403. Create them first with a token that
# has repo-creation scope — the manifest emits the exact list:
#
#   node -e 'require("./passport/strategies.json").strategies
#     .forEach(s=>console.log(s.repo))' \
#     | xargs -n1 -I{} gh repo create {} --public \
#         --description "passport strategy — see github.com/malcolmston/passport"
#
# ORDER MATTERS. Layer 2 pins layer 1 by version, so layer 1 must be pushed AND
# TAGGED before layer 2 is staged, or the layer-2 modules resolve against a tag
# that does not exist yet. Run --layer 1, tag those repos, then --layer 2.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="$REPO_ROOT/passport/strategies.json"

# Stage OUTSIDE the repo. A staged module placed anywhere under REPO_ROOT inherits
# this repo's go.work, whose `use` block lists all 39 library submodules — so a
# `go build` in the staging dir fails with "cannot load module ../../algebra" for
# every submodule that is not checked out, and never even reaches the staged code.
# Outside the tree there is no workspace to inherit and the staged module builds
# exactly as it will once pushed. Override with SUBREPO_STAGE if you want it
# elsewhere, but keep it out of REPO_ROOT (or export GOWORK=off when building).
STAGE="${SUBREPO_STAGE:-${TMPDIR:-/tmp}/passport-subrepo-stage}"

LAYER=""
ONLY=""
PUSH=0
DRY=0

die() { echo "error: $*" >&2; exit 1; }

usage() {
  sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  exit "${1:-0}"
}

while [ $# -gt 0 ]; do
  case "$1" in
    --layer)   LAYER="${2:-}"; shift 2 ;;
    --only)    ONLY="${2:-}";  shift 2 ;;
    --push)    PUSH=1; shift ;;
    --dry-run) DRY=1;  shift ;;
    -h|--help) usage 0 ;;
    *) die "unknown flag: $1 (try --help)" ;;
  esac
done

[ -f "$MANIFEST" ] || die "missing $MANIFEST — run: node --experimental-strip-types scripts/subrepo/gen-passport-manifest.ts"
[ -d "$REPO_ROOT/passport/strategies" ] || die "passport submodule not checked out — run: git submodule update --init passport"
command -v node >/dev/null || die "node is required to read the manifest"
command -v jq   >/dev/null || die "jq is required"

CORE_VERSION="$(jq -r '.core.version' "$MANIFEST")"
CORE_MODULE="$(jq -r '.core.module'  "$MANIFEST")"

# Select the strategies to process, honouring --layer / --only.
select_names() {
  if [ -n "$ONLY" ]; then echo "$ONLY" | tr ',' '\n'; return; fi
  if [ -n "$LAYER" ]; then jq -r ".strategies[] | select(.layer==$LAYER) | .name" "$MANIFEST"; return; fi
  jq -r '.strategies[].name' "$MANIFEST"
}

# Rewrite sibling imports in-place: every legacy sibling path becomes its split
# module path. Built from the manifest so a strategy that gains a new sibling edge
# needs no change here.
#
# One sed program applied once per file, rather than a grep+sed per rule. The
# grep form was not just slow (154 rules x 154 strategies = ~23k subprocesses) but
# WRONG under `set -euo pipefail`: grep exits 1 when a rule does not match the file,
# pipefail promotes that to the pipeline's status, and set -e aborts the run. Since
# nearly every rule misses nearly every file, that killed the script on the first
# strategy. sed exits 0 whether or not it substitutes, so this form has no such trap.
#
# Both sides of each rule are anchored on the enclosing quotes, so a shorter path
# can never clobber a longer one (oauth1 does not match "…/oauth1twitter").
rewrite_imports() {
  local dir="$1"
  local -a rules=()
  local legacy split
  while IFS=$'\t' read -r legacy split; do
    rules+=("s|\"${legacy}\"|\"${split}\"|g")
  done < <(jq -r '.strategies[] | [.legacyImport, .module] | @tsv' "$MANIFEST")

  local program
  program="$(printf '%s;' "${rules[@]}")"

  local f
  while IFS= read -r f; do
    sed -i "$program" "$f"
  done < <(find "$dir" -name '*.go' -type f)
}

stage_one() {
  local name="$1"
  local repo module dest
  repo="$(jq -r --arg n "$name" '.strategies[]|select(.name==$n)|.repo'   "$MANIFEST")"
  module="$(jq -r --arg n "$name" '.strategies[]|select(.name==$n)|.module' "$MANIFEST")"
  [ -n "$repo" ] && [ "$repo" != "null" ] || die "strategy not in manifest: $name"

  local src="$REPO_ROOT/passport/strategies/$name"
  [ -d "$src" ] || die "missing source: $src"
  dest="$STAGE/$name"

  if [ "$DRY" = 1 ]; then
    printf '%-22s -> %-42s (layer %s, %s files)\n' "$name" "$module" \
      "$(jq -r --arg n "$name" '.strategies[]|select(.name==$n)|.layer' "$MANIFEST")" \
      "$(ls "$src" | wc -l | tr -d ' ')"
    return
  fi

  rm -rf "$dest"; mkdir -p "$dest"
  cp -R "$src"/. "$dest"/

  # go.mod: core first, then siblings, matching the manifest's requires order.
  {
    echo "module $module"
    echo
    echo "go 1.23"
    echo
    echo "require ("
    jq -r --arg n "$name" --arg core "$CORE_MODULE" --arg cv "$CORE_VERSION" '
      .strategies[] | select(.name==$n) | .requires[] |
      if . == $core then "\t\($core) \($cv)" else "\t\(.) v0.1.0" end
    ' "$MANIFEST"
    echo ")"
  } > "$dest/go.mod"

  rewrite_imports "$dest"

  [ -f "$dest/LICENSE" ] || cp "$REPO_ROOT/passport/LICENSE" "$dest/LICENSE"
  [ -f "$dest/VERSION" ] || echo "0.1.0" > "$dest/VERSION"

  (
    cd "$dest"
    git init -q -b main
    git add -A
    git -c user.name="github-actions[bot]" \
        -c user.email="github-actions[bot]@users.noreply.github.com" \
        commit -qm "feat: split $name strategy out of malcolmston/passport

Extracted from passport/strategies/$name at core $CORE_VERSION.
Import path changes from github.com/malcolmston/passport/strategies/$name
to $module — a separate repository cannot serve the old path."
    git remote add origin "https://github.com/$repo"
  )

  if [ "$PUSH" = 1 ]; then
    ( cd "$dest" && git push -u origin main ) \
      || die "push failed for $repo — does the repository exist? (this script does not create them)"
    echo "pushed  $repo"
  else
    echo "staged  $name -> $dest ($module)"
  fi
}

names="$(select_names)"
count="$(echo "$names" | grep -c . || true)"
echo "passport strategy split: $count strategies, core $CORE_VERSION, stage=$STAGE"
[ "$PUSH" = 1 ] && echo "MODE: push (repositories must already exist)" || echo "MODE: stage only"
echo

while read -r n; do [ -n "$n" ] && stage_one "$n"; done <<< "$names"

echo
echo "done: $count strategies"
[ "$PUSH" = 0 ] && [ "$DRY" = 0 ] && echo "review $STAGE, then re-run with --push"
exit 0
