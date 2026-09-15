package main

// Read-side evidence commands: read what a recorder stored and say what
// it means. Both are thin over pkg/evidence and pkg/report on purpose. The
// reconciliation rules belong to the library, so an agent, a script and a human
// at a terminal cannot drift into three different definitions of the same metric.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vitas/evidra/pkg/evidence"
	"github.com/vitas/evidra/pkg/report"
)

// evidenceTarget is the resolved input for both commands.
type evidenceTarget struct {
	dir   string
	since time.Time
}

func parseEvidenceArgs(name string, args []string, stderr io.Writer) (*flag.FlagSet, evidenceTarget, bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dirFlag := fs.String("dir", "", "Evidence root holding recorder directories (default: $EVIDRA_EVIDENCE_DIR)")
	sinceFlag := fs.String("since", "", "Only events recorded after this instant: 7d, 36h, or RFC 3339")
	if err := fs.Parse(args); err != nil {
		return fs, evidenceTarget{}, false
	}
	dir := *dirFlag
	if dir == "" {
		dir = os.Getenv("EVIDRA_EVIDENCE_DIR")
	}
	if dir == "" {
		return fs, evidenceTarget{}, true
	}
	since, err := evidence.ParseSince(*sinceFlag, time.Now().UTC())
	if err != nil {
		fmt.Fprintf(stderr, "invalid --since: %v\n", err)
		return fs, evidenceTarget{}, false
	}
	return fs, evidenceTarget{dir: dir, since: since}, true
}

// cmdSummarize reconciles declared, observed and reported evidence and prints the
// terminal summary §35 asks for. It also writes summary.json, because the terminal
// view is for a person reading once and the JSON is for the comparison that comes
// later.
func cmdSummarize(args []string, stdout, stderr io.Writer) int {
	_, target, ok := parseEvidenceArgs("summarize", args, stderr)
	if !ok {
		return 2
	}
	if target.dir == "" {
		fmt.Fprintln(stderr, "summarize: --dir <evidence-root> is required (or set EVIDRA_EVIDENCE_DIR)")
		return 2
	}
	sum, err := report.Reconcile(report.Options{Root: target.dir, Since: target.since})
	if err != nil {
		fmt.Fprintf(stderr, "summarize: %v\n", err)
		return 1
	}
	raw, err := sum.JSON()
	if err != nil {
		fmt.Fprintf(stderr, "summarize: encode summary: %v\n", err)
		return 1
	}
	path := filepath.Join(target.dir, "summary.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		fmt.Fprintf(stderr, "summarize: write %s: %v\n", path, err)
		return 1
	}
	printSummary(stdout, sum)
	fmt.Fprintf(stdout, "\nsummary written to %s\n", path)
	return 0
}

// cmdVerifyChain reports cryptographic validity and evidence coverage as the two
// different statements they are. A chain can verify perfectly and still describe a
// window in which nothing could be recorded; saying "valid" alone would let a
// degraded session read as a clean one (§18).
func cmdVerifyChain(args []string, stdout, stderr io.Writer) int {
	_, target, ok := parseEvidenceArgs("verify", args, stderr)
	if !ok {
		return 2
	}
	if target.dir == "" {
		fmt.Fprintln(stderr, "verify: --dir <evidence-root> is required (or set EVIDRA_EVIDENCE_DIR)")
		return 2
	}
	reports, err := evidence.VerifyRoot(target.dir, target.since)
	if err != nil {
		fmt.Fprintf(stderr, "verify: %v\n", err)
		return 1
	}
	if len(reports) == 0 {
		fmt.Fprintf(stderr, "verify: no recorder directories under %s\n", target.dir)
		return 1
	}
	bad, empty := 0, 0
	for _, r := range reports {
		if r.Records == 0 {
			empty++
		}
		if !r.ChainValid || !r.SignatureValid {
			bad++
		}
		fmt.Fprintf(stdout, "%s records=%d seq=%d..%d chain=%s signature=%s coverage=%s mode=%s upstream=%s digest_key=%s\n",
			filepath.Base(r.Dir), r.Records, r.FirstSeq, r.LastSeq,
			validWord(r.ChainValid), validWord(r.SignatureValid), r.Coverage,
			r.EnforceMode, r.UpstreamID, r.DigestKeyID)
		for _, f := range r.Findings {
			fmt.Fprintf(stdout, "    finding: %s\n", f)
		}
	}
	if bad > 0 {
		fmt.Fprintf(stderr, "verify: %d of %d recorder stores failed cryptographic validation\n", bad, len(reports))
		return 1
	}
	if empty == len(reports) {
		// Nothing in the window: saying "valid" here would let a reader take an
		// empty query as a verified session.
		fmt.Fprintf(stderr, "verify: %d recorder stores, none with records in the requested window\n", len(reports))
		return 1
	}
	fmt.Fprintf(stdout, "verify: %d recorder stores, chains and signatures valid\n", len(reports))
	return 0
}

func validWord(v bool) string {
	if v {
		return "valid"
	}
	return "INVALID"
}

// printSummary is the human-facing artifact Gate C measures: the point is that a
// person changes their understanding of a session after reading it, so the finding
// lines come before the counts.
func printSummary(w io.Writer, sum report.Summary) {
	fmt.Fprintf(w, "evidra summary (%s) root=%s\n", sum.Schema, sum.Root)
	if len(sum.Cells) == 0 {
		fmt.Fprintln(w, "  no recorder directories with events found")
		return
	}
	for _, line := range sum.Findings() {
		fmt.Fprintln(w, "  "+line)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  notes:")
	for _, n := range sum.Notes {
		fmt.Fprintf(w, "    - %s\n", n)
	}
}
