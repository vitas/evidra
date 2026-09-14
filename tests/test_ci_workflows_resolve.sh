#!/usr/bin/env bash
set -uo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Every target a CI workflow names must exist.
#
# Why this exists: `ci-vnext.yml` ran `go test -race ./internal/lifecycle/...` after that
# package was deleted in the §43 prune, and `ci.yml` called four Make targets, five shell
# guards and a prompts directory that no longer exist. Both workflows were edited for pages
# at a time while their contents drifted, and the first anyone found out was a red check on a
# pushed branch. A workflow step pointing at a deleted path is the same object as a checklist
# row pointing at a function that no longer compiles: it is not evidence of anything, it just
# looks like it.

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[[ -f tests/vnext-workflows.txt ]] || fail "tests/vnext-workflows.txt is missing"

problems=0
note() {
  printf '  %s\n' "$*"
  problems=$((problems + 1))
}

# --- structural sanity: mappings that must not be empty --------------------------------------
# GitHub accepts these files as YAML and rejects them as workflows, so loading them proves
# nothing: a key with only comments under it parses to null, and a null `env:` or `jobs:` fails
# the run with no jobs and no useful log. This was caught by a red check on a pushed branch
# rather than by a local one, which is the whole reason this guard exists.
for wf in .github/workflows/*.yml; do
  empties="$(python3 - "$wf" <<'PYEOF'
import re, sys
must = re.compile(r'^([ ]*)(env|with|jobs|steps|permissions|outputs):[ ]*$')
lines = open(sys.argv[1], encoding='utf-8').read().split('\n')
out = []
for i, line in enumerate(lines):
    m = must.match(line)
    if not m:
        continue
    ind = len(m.group(1))
    for nxt in lines[i + 1:]:
        if not nxt.strip() or nxt.lstrip().startswith('#'):
            continue
        if len(nxt) - len(nxt.lstrip()) <= ind:
            out.append('%d:%s' % (i + 1, line.strip()))
        break
print('\n'.join(out))
PYEOF
)"
  while IFS= read -r hit; do
    [[ -n "$hit" ]] && note "$wf: line ${hit%%:*} is a mapping that must have entries but has none: ${hit#*:}"
  done <<<"$empties"
done

# --- every workflow on disk is declared ----------------------------------------------------
while IFS= read -r file; do
  base="$(basename "$file")"
  grep -q "^${base}	" tests/vnext-workflows.txt || note "$base exists but is not declared in tests/vnext-workflows.txt"
done < <(find .github/workflows -maxdepth 1 -name '*.yml' | sort)

while IFS=$'\t' read -r name state marker; do
  [[ -n "${name:-}" && ! "$name" =~ ^# ]] || continue
  [[ -f ".github/workflows/$name" ]] || note "$name is declared but the workflow file is missing"
  [[ "$state" == "enabled" || "$state" == "disabled" ]] || note "$name has unknown state '$state'"
  if [[ "$state" == "disabled" && "$marker" != "none" ]]; then
    grep -q -- "$marker" ".github/workflows/$name" 2>/dev/null ||
      note "$name is declared disabled but does not contain its refusal marker '$marker'"
  fi
done < <(grep -v '^#' tests/vnext-workflows.txt)

# --- resolve references inside enabled workflows -------------------------------------------
resolve_go_path() {
  local p="$1"
  p="${p%/...}"
  p="${p%/}"
  [[ "$p" == "." || "$p" == "./" ]] && return 0
  [[ -d "$p" ]] && return 0
  [[ -f "$p" ]] && return 0
  [[ -f "${p}.go" ]] && return 0
  return 1
}

while IFS=$'\t' read -r name state _marker; do
  [[ -n "${name:-}" && ! "$name" =~ ^# ]] || continue
  [[ "$state" == "enabled" ]] || continue
  wf=".github/workflows/$name"
  # Only executable content is scanned. Prose in comments legitimately names paths that were
  # deleted - the §43 notes say so out loud - and `make` appears in ordinary English too.
  body="$(awk '{ if ($0 ~ /^[[:space:]]*#/) next; print }' "$wf")"

  # make <target>
  for target in $(printf '%s\n' "$body" | grep -oE 'make +[a-z][a-z0-9_-]*' | awk '{print $2}' | sort -u); do
    grep -qE "^${target}:" Makefile || note "$wf: 'make $target' but Makefile has no such target"
  done

  # bash scripts/<x>.sh, bash tests/<x>.sh
  for script in $(printf '%s\n' "$body" | grep -oE '(bash|sh|python3) +(scripts|tests|tools)/[A-Za-z0-9_./-]+' | awk '{print $2}' | sort -u); do
    [[ -f "$script" ]] || note "$wf: runs $script which does not exist"
    [[ -x "$script" ]] || note "$wf: runs $script which is not executable"
  done

  # go test / go vet / go build package patterns given literally
  for pattern in $(printf '%s\n' "$body" | grep -oE '\./[A-Za-z0-9_./-]+\.\.\.?' | sed 's/\.\.\.$//' | sort -u); do
    resolve_go_path "$pattern" || note "$wf: names $pattern which is not a directory in this module"
  done

  # config files named in steps (go-version-file, path: inputs that are repo paths)
  for cfg in $(printf '%s\n' "$body" | grep -oE '(go-version-file|cache-dependency-path): +[A-Za-z0-9_./-]+' | awk '{print $2}' | sort -u); do
    [[ -f "$cfg" ]] || note "$wf: requires $cfg which does not exist"
  done

  # -run 'A|B|C' filters: a pattern matching no test reports success while testing nothing
  while IFS= read -r spec; do
    pkg="$(printf '%s\n' "$spec" | awk '{print $1}')"
    pattern="$(printf '%s\n' "$spec" | sed "s/^${pkg} //; s/^'//; s/'$//")"
    resolve_go_path "$pkg" || continue
    matches="$(go test -list "$pattern" "$pkg" 2>/dev/null | grep -c '^Test' || true)"
    [[ "${matches:-0}" -gt 0 ]] || note "$wf: -run '$pattern' over $pkg matches no tests (vacuous green)"
  done < <(printf '%s\n' "$body" | grep -oE "\-run +'[^']+' +\./[A-Za-z0-9_./-]+" | sed "s/-run +//" | awk '{print $2" "$1}' | sort -u)
done < <(grep -v '^#' tests/vnext-workflows.txt)

# --- release configuration that is not a workflow but is what a release runs ---------------
if [[ -f .goreleaser.yaml ]]; then
  for main in $(grep -oE 'main: +\./[A-Za-z0-9_./-]+' .goreleaser.yaml | awk '{print $2}' | sort -u); do
    resolve_go_path "${main%/}" || note ".goreleaser.yaml: builds $main which does not exist"
  done
fi

if ((problems > 0)); then
  fail "$problems unresolved CI reference(s) - fix the reference or the code, not this check"
fi

echo "PASS: every CI workflow reference resolves"
