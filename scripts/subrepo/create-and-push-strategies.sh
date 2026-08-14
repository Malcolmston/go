#!/usr/bin/env bash
#
# create-and-push-strategies.sh — create every passport strategy repository and
# push its content, in dependency order.
#
# Run this LOCALLY. It needs a credential that can create repositories, which the
# CI/agent environment cannot hold: that environment authenticates as a GitHub App
# installation, and POST /user/repos rejects installation tokens outright
# ("Resource not accessible by integration"). It is not a scope that can be
# granted — the endpoint takes OAuth tokens and PATs only, and GitHub Apps can
# only create repos under an ORGANISATION (POST /orgs/{org}/repos). malcolmston is
# a user account, so a PAT is the only route.
#
# CREDENTIAL HANDLING
#   Create a fine-grained PAT at
#     https://github.com/settings/personal-access-tokens/new
#   with resource owner `malcolmston` and Administration: Read and write.
#
#   Pass it in the environment, never as an argument — argv is world-readable via
#   `ps` on most systems, and shells persist it to history:
#
#     read -rs GH_PAT && export GH_PAT     # prompts without echoing
#     scripts/subrepo/create-and-push-strategies.sh --layer 1
#
#   Revoke the token when the migration is done. If a token is ever pasted into a
#   chat, an issue, a log or a commit, treat it as compromised and rotate it —
#   exposure is not undone by deleting the message.
#
# USAGE
#   scripts/subrepo/create-and-push-strategies.sh --layer 1 [--dry-run]
#   scripts/subrepo/create-and-push-strategies.sh --layer 1 --tag v0.1.0
#   scripts/subrepo/create-and-push-strategies.sh --layer 2
#
# ORDER
#   Layer 2 pins layer 1 by version, so the sequence is:
#     1. --layer 1              create + push the 31 base strategies
#     2. --layer 1 --tag v0.1.0 tag them so layer 2 can resolve the pin
#     3. --layer 2              create + push the remaining 123
#
#   Running --layer 2 before layer 1 is tagged produces modules whose requires
#   point at tags that do not exist yet.
#
# IDEMPOTENT: a repository that already exists is reused, not treated as an error,
# so a partial run can simply be re-run.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="$REPO_ROOT/passport/strategies.json"
STAGE="${SUBREPO_STAGE:-${TMPDIR:-/tmp}/passport-subrepo-stage}"
API="https://api.github.com"

LAYER=""
TAG=""
DRY=0

die() { echo "error: $*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --layer)   LAYER="${2:-}"; shift 2 ;;
    --tag)     TAG="${2:-}";   shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) sed -n '2,45p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown flag: $1" ;;
  esac
done

TOKEN="${GH_PAT:-${GITHUB_TOKEN:-}}"
[ -n "$TOKEN" ] || die "set GH_PAT (or GITHUB_TOKEN) in the environment — see the header"
[ -f "$MANIFEST" ] || die "missing $MANIFEST — run gen-passport-manifest.ts first"
command -v jq   >/dev/null || die "jq is required"
command -v curl >/dev/null || die "curl is required"
[ -n "$LAYER" ] || die "--layer 1 or --layer 2 is required (ordering is not optional)"

names="$(jq -r ".strategies[] | select(.layer==$LAYER) | .name" "$MANIFEST")"
count="$(echo "$names" | grep -c . || true)"

echo "layer $LAYER: $count strategies"
[ "$DRY" = 1 ] && echo "MODE: dry run (no repos created, nothing pushed)"
echo

# Create the repo unless it already exists. `gh` is used when present because it
# handles enterprise hosts and auth quirks; curl is the dependency-free fallback.
create_repo() {
  local repo="$1" desc="$2" name="${1#*/}"
  if curl -fsS -o /dev/null -H "Authorization: Bearer $TOKEN" "$API/repos/$repo" 2>/dev/null; then
    echo "  exists  $repo"
    return 0
  fi
  if [ "$DRY" = 1 ]; then echo "  create  $repo (dry run)"; return 0; fi

  curl -fsS -o /dev/null -X POST "$API/user/repos" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Accept: application/vnd.github+json" \
    -d "$(jq -nc --arg n "$name" --arg d "$desc" \
          '{name:$n, description:$d, private:false, auto_init:false, has_issues:true}')" \
    || die "could not create $repo — is the PAT scoped to Administration: Read and write?"
  echo "  created $repo"
}

# Stage, then push. Staging is delegated so there is exactly one implementation of
# the go.mod/import rewrite, shared with the stage-only workflow.
while read -r name; do
  [ -n "$name" ] || continue
  repo="$(jq -r --arg n "$name" '.strategies[]|select(.name==$n)|.repo' "$MANIFEST")"
  echo "$name:"
  create_repo "$repo" "passport strategy '$name' — see github.com/malcolmston/passport"

  if [ "$DRY" = 1 ]; then echo; continue; fi

  "$REPO_ROOT/scripts/subrepo/split-passport-strategies.sh" --only "$name" >/dev/null
  (
    cd "$STAGE/$name"
    git remote set-url origin "https://x-access-token:${TOKEN}@github.com/${repo}.git"
    git push -q -u origin main
    if [ -n "$TAG" ]; then
      git tag -f "$TAG"
      git push -q -f origin "$TAG"
    fi
    # Do not leave the credential embedded in the staged checkout's config.
    git remote set-url origin "https://github.com/${repo}"
  )
  echo "  pushed  $repo${TAG:+ @ $TAG}"
  echo
done <<< "$names"

echo "done: layer $LAYER, $count strategies"
if [ "$LAYER" = 1 ] && [ -z "$TAG" ]; then
  echo
  echo "NEXT: tag layer 1 before staging layer 2, or layer 2's requires will point"
  echo "      at tags that do not exist:"
  echo "        $0 --layer 1 --tag v0.1.0"
fi
