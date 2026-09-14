# vNext history scrub — what was removed from the branch's own history, and how to resolve an old hash

**Why this document exists.** Before the first push of `vnext/mcp-recorder`, a commit in this
branch's history was found to carry 54 untracked scratch files under `tmp/` and 5 under
`examples/` — including 53 JPGs of a sales-review deck. They were never intended to be
committed: a blanket `git add -A` swept them into the §43 step-2 commit (pre-scrub `cf7e2cb`,
now `d99717a`), and `24ccd48` (pre-scrub `c79f3ee`) untracked them
again, which left them in the commit's tree and therefore in every clone of the branch
forever. Because the branch had never been pushed, this was the last moment at which the
removal cost nobody anything. The removal was done before the push, on instruction.

**What was done.** One commit was rewritten — the one that added the files — and its
descendants replayed on top of it:

```
git checkout --detach <cf7e2cb>          # the commit that added the scratch files
                                         # (<pre-scrub ids: the old objects exist only until pruning)
git rm -r --cached tmp examples
git commit --amend --no-edit             # -> d99717a, same message, same author, same DCO trailer
git rebase --onto d99717a <cf7e2cb> vnext/mcp-recorder   # 27 descendants replayed
```

`git filter-repo` was tried first and rejected: it re-encoded history back to the root, so
857 commits changed hash, the merge-base with `main` moved 800 commits backwards, and a PR
would have shown 857 commits where 51 exist. The narrow rewrite changes 28 commits and leaves
shared history with `main` byte-identical.

**What did not change, and how that was checked.** Every claim below was run against both
histories, not argued:

| Property | Check | Result |
|---|---|---|
| File content at the branch tip | `git rev-parse HEAD^{tree}` before/after | identical (`b70935a4…`) |
| Number of commits ahead of `main` | `git rev-list --count main..HEAD` | 51 before, 51 after |
| Shared history with `main` | `git merge-base main HEAD` | `fe213d12` both |
| No commit added or lost | subject multiset diff of both logs | empty |
| Scratch paths gone | `git rev-list --objects HEAD \| grep -cE " (tmp\|examples)/"` | 0 |
| DCO intact on every commit | trailer scan over `main..HEAD` | 0 unsigned |

The working tree at the tip of the branch also carries none of those paths, so nothing a
reader can build, run or read from this branch is different. What is different is only which
object ids the last 28 commits have.

**How to resolve a hash cited in an older document.** Documents written before the scrub cite
the old ids; they are not wrong, they are historical. Map them with the table below, or
re-derive any single one by subject:

```
git log --format='%h %s' vnext/mcp-recorder | grep 'the subject you have'
```

## Old → new

| Old (pre-scrub) | New | Subject |
|---|---|---|
| `cf7e2cb` | `d99717a` | refactor(vnext): §43 step 2 — the hosted chain leaves the module, ui… |
| `c79f3ee` | `24ccd48` | fix(vnext): stop tracking tmp/ and examples/, which a blanket git ad… |
| `318bcd5` | `b19ea59` | refactor(vnext): §43 step 3 — evidra-mcp serves only the merged endp… |
| `6c28a28` | `446fed4` | refactor(vnext): §43 step 4 — the legacy MCP server, its harnesses a… |
| `3292230` | `57a79a2` | refactor(vnext): §43 step 5 — the pre-vNext relay, its heuristics an… |
| `b508052` | `935de3d` | refactor(vnext): §43 step 6 — the analysis engine, prompt contracts … |
| `3fadc59` | `8d45418` | refactor(vnext): §43 step 6b — pkg/evidence keeps only the v2 model |
| `bce335f` | `c7b02e6` | docs(vnext): §43 step 7 — architecture and agent guidance describe t… |
| `4e72f7c` | `a043fac` | docs(vnext): state the README gap instead of papering over it |
| `6e9b79e` | `b6af110` | feat(vnext): §47 in-band evidra_report feedback, counted from observ… |
| `b642513` | `6d3b13a` | fix(vnext): the runner refuses to measure a binary older than its so… |
| `84377c5` | `d8cd8eb` | docs(vnext): second probe of the in-band feedback, recorded as not_m… |
| `f03b39e` | `95da55a` | feat(vnext): experiment runs record binary provenance; analytics inv… |
| `d94bf79` | `7f20209` | refactor(vnext): split the analytics invariant checker to satisfy it… |
| `ab44fee` | `f322cb7` | fix(vnext): memoize source state; correct the second probe's task-su… |
| `d97cda6` | `888a0b3` | feat(vnext): reconciliation view per operation; harness invariants a… |
| `28450a9` | `8779d01` | docs(vnext): the revised Gate A bars meet the official set, and the … |
| `3a11406` | `5185d87` | docs(vnext): full implementation report — findings, negative results… |
| `2709897` | `94f7ab0` | docs(vnext): correct the commit count in the implementation report |
| `aeefcbd` | `8ec2092` | docs(vnext): agreement fix in the report header |
| `76bdee5` | `655470b` | fix(vnext): delete the pre-vNext normative specs and the guards that… |
| `3476147` | `a794916` | fix(vnext): restore executable bits on the guards that remain |
| `6185191` | `c670887` | docs(vnext): the prune document becomes a record, with the ordering … |
| `160fef2` | `8fe49b9` | docs(vnext): §43 states the criterion it was executed under instead … |
| `8be8721` | `fe924ab` | docs(vnext): scope the headline claim, the false-record term, and th… |
| `816e1ac` | `af1c076` | fix(vnext): finish the docs sweep — stale "current" refs deleted, ba… |
| `4afa9cf` | `64480a6` | docs(vnext): stop writing commit counts in prose without the command… |
| `d479134` | `57dd0e2` | docs(vnext): the prune record stops describing future work, and the … |

**A number in this file that was wrong when written.** The report and CHANGELOG both said
the module was pruned "40 → 12 packages". It is 9 (`go list ./... | wc -l`, CI floor 8). The 12
was measured mid-prune and never re-derived, which is the same failure the commit-count rule
above describes: a figure quoted in prose without the command that produces it becomes a
false statement at the next commit, and a reader has no way to tell.

**One thing this does not fix.** The scratch files existed only in this branch, so the removal
is complete: `main` never had them (verified with `git log --full-history main -- tmp examples`
→ no commits), and they never reached the remote. Had the branch already been pushed, no
local rewrite would have removed them from GitHub's object storage — that would have needed
the site's own history-support process, and the blobs would have stayed retrievable by SHA in
the meantime. That is the entire reason this happened before the first push rather than after.
