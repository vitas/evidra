#!/usr/bin/env python3
"""Structural checks over GitHub Actions workflow files that plain YAML cannot express.

Why a separate program: these rules parse YAML-shaped documents, and embedding them in a bash
heredoc turned out to be untestable - the first version of the job-graph check silently failed
to install itself, and the guard reported PASS while a dangling `needs:` reference sat in the
file it was supposed to read. A check that cannot be run by hand is a check that cannot be
shown to fail.

Rules
-----
1. A mapping that must carry entries (`env`, `with`, `jobs`, `steps`, `permissions`,
   `outputs`) may not be followed only by comments. GitHub accepts that as YAML and rejects it
   as a workflow, producing a run with zero jobs and no log. Triggers such as `pull_request:`
   and `workflow_dispatch:` are exempt: an empty trigger is legal.
2. Every `needs:` target must name a job defined in the same file. Deleting a job without
   searching for `needs:` references to it is the same mistake as a CI step naming a deleted
   package, except the pipeline then fails in a way nobody can read.
3. The needs graph must be acyclic: a cycle is another zero-job rejection.
"""
import re
import sys

MUST_HAVE_ENTRIES = ("env", "with", "jobs", "steps", "permissions", "outputs")
JOB_KEY = re.compile(r"^  ([A-Za-z0-9_-]+):\s*$")
NEEDS = re.compile(r"^\s+needs:\s*\[?([^\]]*)\]?\s*$")
TOP_LEVEL = re.compile(r"^[A-Za-z]")


def indent(line: str) -> int:
    return len(line) - len(line.lstrip())


def job_names(lines):
    names = set()
    inside = False
    for line in lines:
        if re.match(r"^jobs:\s*$", line):
            inside = True
            continue
        if inside and TOP_LEVEL.match(line):
            inside = False
        if not inside:
            continue
        m = JOB_KEY.match(line)
        if m:
            names.add(m.group(1))
    return names


def check_empty_mappings(lines, path):
    problems = []
    for i, line in enumerate(lines):
        m = re.match(r"^([ ]*)(" + "|".join(MUST_HAVE_ENTRIES) + r"):[ ]*$", line)
        if not m:
            continue
        ind = len(m.group(1))
        for nxt in lines[i + 1:]:
            if not nxt.strip() or nxt.lstrip().startswith("#"):
                continue
            if indent(nxt) <= ind:
                problems.append(f"{path}: line {i + 1}: '{m.group(2)}:' has no entries under it")
            break
        else:
            # Nothing but comments and blank lines to the end of the file: also empty.
            problems.append(f"{path}: line {i + 1}: '{m.group(2)}:' ends the file with no entries")
    return problems


def check_needs(lines, path):
    problems = []
    defined = job_names(lines)
    graph = {}
    inside = False
    for i, line in enumerate(lines):
        if re.match(r"^jobs:\s*$", line):
            inside = True
            continue
        if inside and TOP_LEVEL.match(line):
            inside = False
        if not inside:
            continue
        job = None
        m = JOB_KEY.match(line)
        if m:
            job = m.group(1)
        nm = NEEDS.match(line)
        if not nm:
            continue
        owner = job or current_job(lines, i)
        targets = [t.strip().strip("'\"") for t in nm.group(1).split(",") if t.strip()]
        if owner:
            graph.setdefault(owner, []).extend(targets)
        for target in targets:
            if target not in defined:
                problems.append(
                    f"{path}: line {i + 1}: job '{owner}' needs '{target}', which this file does not define"
                )
    problems.extend(check_cycles(graph, path))
    return problems


def current_job(lines, index):
    for i in range(index, -1, -1):
        m = JOB_KEY.match(lines[i])
        if m:
            return m.group(1)
    return None


def check_cycles(graph, path):
    problems = []
    state = {}

    def visit(node, trail):
        if state.get(node) == 1:
            problems.append(f"{path}: needs cycle: {' -> '.join(trail + [node])}")
            return
        if state.get(node) == 2:
            return
        state[node] = 1
        for nxt in graph.get(node, []):
            if nxt in graph:
                visit(nxt, trail + [node])
        state[node] = 2

    for node in graph:
        visit(node, [])
    return problems


def main(argv):
    problems = []
    for path in argv:
        with open(path, encoding="utf-8") as handle:
            lines = handle.read().split("\n")
        problems.extend(check_empty_mappings(lines, path))
        problems.extend(check_needs(lines, path))
    for problem in problems:
        print(problem)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
