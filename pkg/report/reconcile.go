// Package report turns recorded evidence into the reconciliation the product
// exists to produce: what the agent declared, what the recorder observed inside
// the same boundary, and what the agent says happened at the end (§34, §38).
//
// Two rules shape everything below. A derived state may only be one of the four
// the plan defines, because "the report is missing" and "the session ended
// unknown" are different facts and inventing a fifth invites a reader to
// over-read it. And no metric may be averaged across enforcement modes, since
// `enforce=off` observes calls that `enforce=all` refuses: one compliance number
// spanning both would describe no real system (§20, §36).
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"samebits.com/evidra/pkg/evidence"
)

// Schema names the artifact so a reader can tell which reconciliation rules wrote
// the numbers they are looking at.
const Schema = "evidra.summary.v2"

// Options selects the evidence to reconcile.
type Options struct {
	// Root is an evidence root holding recorder directories.
	Root string
	// Since drops events recorded before an instant; zero reads everything.
	Since time.Time
}

// Execution is one observed upstream call as the reconciliation layer sees it.
type Execution struct {
	ExecutionID             string    `json:"execution_id"`
	Tool                    string    `json:"tool"`
	ArgumentsHMAC           string    `json:"arguments_hmac"`
	Status                  string    `json:"status"`
	ResultFingerprintStatus string    `json:"result_fingerprint_status"`
	ErrorCode               string    `json:"error_code,omitempty"`
	DurationMS              int64     `json:"duration_ms"`
	StartedAt               time.Time `json:"started_at"`
	FinishedAt              time.Time `json:"finished_at"`
	// DeclaredReadOnly is the upstream's own annotation, recorded verbatim and
	// never verified: it is what the server claimed, not what the call did (§26).
	DeclaredReadOnly bool `json:"declared_read_only_unverified"`
	// Paired states whether the terminal event for this start was found.
	Paired bool `json:"paired"`
}

// Report is the agent's terminal declaration for an operation.
type Report struct {
	Status  string `json:"status"`
	Outcome string `json:"outcome"`
	Summary string `json:"summary,omitempty"`
}

// Operation is one prescription with everything the recorder attributed to it.
type Operation struct {
	OperationID      string `json:"operation_id"`
	SessionID        string `json:"session_id"`
	RecorderInstance string `json:"recorder_instance_id"`
	Objective        string `json:"objective,omitempty"`
	ExpectedOutcome  string `json:"expected_outcome,omitempty"`
	// State is one of §34's four derived states, never a fifth invented here.
	State      string      `json:"derived_state"`
	Report     *Report     `json:"report,omitempty"`
	Executions []Execution `json:"executions"`
	// BlockedAttemptsInSession is §38's `blocked_attempts_for_operation`, counted
	// over the session instead of the operation window. The distinction is not
	// pedantry: a block only happens when no operation is open, so an
	// operation-scoped block count is structurally always zero, and reporting it
	// would hide the difference between "nothing was attempted" and "five attempts
	// were refused". The session is the narrowest scope that exists.
	BlockedAttemptsInSession int      `json:"blocked_attempts_in_session"`
	Anomalies                []string `json:"anomalies,omitempty"`
	Facts                    []string `json:"reconciliation_facts,omitempty"`
	// SameFingerprintRecovery answers §38.B only when a comparison was possible.
	SameFingerprintRecovery      string `json:"same_fingerprint_recovery,omitempty"`
	RepeatedArgumentFingerprints int    `json:"repeated_canonical_argument_fingerprints"`

	// group is the cell this operation belongs to, kept so reconciliation never
	// has to guess which comparison domain a number came from.
	group string
	// declaredAt and reportedAt bound the operation's window for block
	// attribution (§38). They are recorded facts, not serialized fields: the
	// events themselves already carry the timestamps a reader can check.
	declaredAt time.Time
	reportedAt time.Time
	// execIDs holds the executions attributed to this operation while events are
	// being read. The start records must stay mutable until their terminal event
	// arrives, so they live in one index and an operation names them by id rather
	// than copying them into itself and losing the pairing.
	execIDs []string
}

// Cell is one comparison domain: enforcement mode, upstream and digest key.
// Cells are reported side by side and never merged.
type Cell struct {
	Group          string `json:"group"`
	EnforceMode    string `json:"enforce_mode"`
	UpstreamID     string `json:"upstream_id"`
	ServerName     string `json:"server_name,omitempty"`
	DigestKeyID    string `json:"digest_key_id"`
	Stores         int    `json:"stores"`
	Records        int    `json:"records"`
	ChainValid     bool   `json:"cryptographic_chain"`
	SignatureValid bool   `json:"signature"`
	Coverage       string `json:"evidence_coverage"`

	Operations              []Operation `json:"operations"`
	UnpairedExecutions      int         `json:"unpaired_executions"`
	UnprescribedExecutions  int         `json:"unprescribed_executions"`
	BlockedAttempts         int         `json:"blocked_unprescribed_attempts"`
	SessionsWithNoPrescribe int         `json:"sessions_with_no_prescribe"`
	SessionsWithAttempts    int         `json:"sessions_with_upstream_attempts"`
	SessionsPrescribedFirst int         `json:"sessions_prescribed_before_first_attempt"`

	// VoluntaryPrescriptionCoverage is defined only for `enforce=off`; in
	// `enforce=all` prescription is not voluntary, so the field stays empty
	// rather than reporting a number that means nothing.
	VoluntaryPrescriptionCoverage string `json:"voluntary_prescription_coverage,omitempty"`
	FirstAttemptCompliance        string `json:"first_attempt_protocol_compliance,omitempty"`

	DegradedWindows int   `json:"degraded_window_count"`
	DegradedMS      int64 `json:"degraded_window_duration_ms"`
}

// Summary is the whole reconciliation, plus what a reader must know to avoid
// misreading it.
type Summary struct {
	Schema      string    `json:"schema"`
	GeneratedAt time.Time `json:"generated_at"`
	Root        string    `json:"root"`
	Cells       []Cell    `json:"cells"`
	Notes       []string  `json:"notes"`
}

// anomaly and fact vocabulary from §38. Named here so a change in wording is a
// change to one constant, not to a string that readers of the code may trust.
const (
	AnomalyClaimedAchievedWithoutExecution = "CLAIMED_ACHIEVED_WITHOUT_OBSERVED_EXECUTION_IN_SCOPE"
	FactAchievedWithDeclaredReadOnlyOnly   = "ACHIEVED_WITH_ONLY_DECLARED_READ_ONLY_EXECUTIONS_IN_SCOPE"

	stateReported           = "reported"
	stateReplacedWithoutRpt = "replaced_without_report"
	stateInterruptedSession = "interrupted_session"
	stateLifecycleUnknown   = "lifecycle_unknown"

	recoveryYes          = "later_success_with_same_fingerprint"
	recoveryNo           = "no_later_same_fingerprint_success"
	recoveryInsufficient = "insufficient_fingerprint_data"
)

// opKey identifies an operation within one recorder instance. Operations are not
// merged across recorders: a new process is a new observation boundary, and the
// plan's non-omission claim reaches only as far as one recorder saw (§21).
type opKey struct{ recorder, operationID string }

type blockFact struct {
	sessionID string
	at        time.Time
}

// sessionFacts is what one session proves about protocol use, gathered while the
// events are in order so "first attempt" does not have to be re-derived later.
type sessionFacts struct {
	prescribed   bool
	sawAttempt   bool
	prescribedOk bool
}

// Reconcile reads the evidence root and produces the summary.
// starts keeps every execution record by id so a terminal event can update the
// same record its start created.
type startsIndex map[string]*Execution

func Reconcile(opts Options) (Summary, error) {
	reports, err := evidence.VerifyRoot(opts.Root, opts.Since)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{
		Schema:      Schema,
		GeneratedAt: time.Now().UTC(),
		Root:        opts.Root,
		Notes: []string{
			"cells are comparison domains: counts are never comparable across enforcement modes, upstreams or digest keys",
			"cryptographic_chain and evidence_coverage are separate statements: a valid chain can still be incomplete",
			"declared objectives and report summaries are agent text stored readably; observed arguments and results are keyed fingerprints",
			"annotations come from the upstream server and are unverified",
		},
	}
	cells := map[string]*Cell{}
	ops := map[opKey]*Operation{}
	opOrder := []opKey{}
	blocks := map[string][]blockFact{}
	starts := startsIndex{}

	for _, rep := range reports {
		cell := cellFor(cells, &rep)
		events, _, err := evidence.ReadStore(rep.Dir, opts.Since)
		if err != nil {
			return Summary{}, fmt.Errorf("report: read %s: %w", rep.Dir, err)
		}
		if err := ingest(cell, rep, events, ops, &opOrder, blocks, starts); err != nil {
			return Summary{}, err
		}
	}
	if err := finalize(cells, ops, opOrder, blocks, starts); err != nil {
		return Summary{}, err
	}
	// Operations attach to their cell during finalize, so the ordered view is
	// built after it, not before.
	sum.Cells = orderedCells(cells)
	if len(sum.Cells) == 0 {
		sum.Notes = append(sum.Notes, "no recorder directories with events were found under this root")
	}
	return sum, nil
}

func cellFor(cells map[string]*Cell, rep *evidence.StoreReport) *Cell {
	key := evidence.GroupKey(evidence.Meta{
		UpstreamID: rep.UpstreamID, EnforceMode: rep.EnforceMode, DigestKeyID: rep.DigestKeyID,
	})
	c, ok := cells[key]
	if !ok {
		c = &Cell{
			Group: key, EnforceMode: rep.EnforceMode, UpstreamID: rep.UpstreamID,
			ServerName: rep.ServerName, DigestKeyID: rep.DigestKeyID,
			ChainValid: true, SignatureValid: true, Coverage: "complete",
		}
		cells[key] = c
	}
	c.Stores++
	c.Records += rep.Records
	// A cell claims validity only if every store in it does.
	c.ChainValid = c.ChainValid && rep.ChainValid
	c.SignatureValid = c.SignatureValid && rep.SignatureValid
	if rep.Coverage != "complete" {
		c.Coverage = "degraded"
	}
	return c
}

// pendingExecutions waits for a terminal event keyed by execution_id.
type pendingExecutions map[string]*Execution

func ingest(cell *Cell, rep evidence.StoreReport, events []evidence.Event,
	ops map[opKey]*Operation, opOrder *[]opKey, blocks map[string][]blockFact, starts startsIndex) error {
	pending := pendingExecutions{}
	sessions := map[string]*sessionFacts{}
	for i := range events {
		ev := &events[i]
		switch ev.EventType {
		case evidence.EventOperationPrescribed:
			declareOp(cell, ops, opOrder, opKey{rep.RecorderInstanceID, ev.OperationID}, ev, sessions)
		case evidence.EventOperationReported:
			applyReport(ops, opKey{rep.RecorderInstanceID, ev.OperationID}, ev)
		case evidence.EventOperationReplaced:
			applyReplace(ops, rep.RecorderInstanceID, ev)
		case evidence.EventExecutionStarted:
			noteAttempt(sessions, ev, true)
			applyStart(cell, ops, opKey{rep.RecorderInstanceID, ev.OperationID}, ev, pending, starts)
		case evidence.EventExecutionFinished:
			applyFinish(ops, ev, pending)
		case evidence.EventProtocolViolation:
			noteAttempt(sessions, ev, false)
			applyBlock(cell, ev, blocks)
		case evidence.EventRecorderDegraded:
			applyDegraded(cell, ev)
		case evidence.EventRecorderStopped:
			markStopped(ops, rep.RecorderInstanceID)
		}
	}
	cell.UnpairedExecutions += len(pending)
	for _, f := range sessions {
		if !f.prescribed {
			cell.SessionsWithNoPrescribe++
		}
		if f.sawAttempt {
			cell.SessionsWithAttempts++
			if f.prescribedOk {
				cell.SessionsPrescribedFirst++
			}
		}
	}
	return nil
}

// noteAttempt records that a session made its first move upstream, and whether a
// prescription was open when it did. Blocks never open an operation, so a session
// whose first attempt was blocked is not compliant even if it later behaved.
func noteAttempt(sessions map[string]*sessionFacts, ev *evidence.Event, executed bool) {
	f := sessions[ev.SessionID]
	if f == nil {
		f = &sessionFacts{}
		sessions[ev.SessionID] = f
	}
	if f.sawAttempt {
		return
	}
	f.sawAttempt = true
	f.prescribedOk = executed && ev.OperationID != ""
}

func declareOp(cell *Cell, ops map[opKey]*Operation, opOrder *[]opKey, key opKey,
	ev *evidence.Event, sessions map[string]*sessionFacts) {
	f := sessions[ev.SessionID]
	if f == nil {
		f = &sessionFacts{}
		sessions[ev.SessionID] = f
	}
	f.prescribed = true
	p, err := ev.DecodePayload()
	if err != nil {
		return
	}
	declared := p.(*evidence.PrescribedPayload)
	op := &Operation{
		OperationID: key.operationID, SessionID: ev.SessionID, RecorderInstance: key.recorder,
		Objective: declared.Objective, ExpectedOutcome: declared.ExpectedOutcome,
		State: stateLifecycleUnknown, group: cell.Group, declaredAt: ev.RecordedAt,
	}
	ops[key] = op
	*opOrder = append(*opOrder, key)
}

func applyReport(ops map[opKey]*Operation, key opKey, ev *evidence.Event) {
	op, ok := ops[key]
	if !ok {
		return
	}
	p, err := ev.DecodePayload()
	if err != nil {
		return
	}
	r := p.(*evidence.ReportedPayload)
	op.Report = &Report{Status: r.Status, Outcome: r.Outcome, Summary: r.Summary}
	op.State = stateReported
	op.reportedAt = ev.RecordedAt
}

func applyReplace(ops map[opKey]*Operation, recorder string, ev *evidence.Event) {
	p, err := ev.DecodePayload()
	if err != nil {
		return
	}
	r := p.(*evidence.ReplacedPayload)
	old, ok := ops[opKey{recorder, r.ReplacedOperationID}]
	if !ok {
		return
	}
	// A replaced operation keeps whatever report it did have; only a superseded
	// operation that never reported becomes replaced_without_report (§34).
	if old.Report == nil {
		old.State = stateReplacedWithoutRpt
	}
}

func applyStart(cell *Cell, ops map[opKey]*Operation, key opKey, ev *evidence.Event, pending pendingExecutions, starts startsIndex) {
	p, err := ev.DecodePayload()
	if err != nil {
		return
	}
	s := p.(*evidence.ExecutionStartedPayload)
	ex := &Execution{
		ExecutionID: s.ExecutionID, Tool: s.Tool, ArgumentsHMAC: s.ArgumentsHMAC,
		StartedAt: s.StartedAt, Status: "started", DeclaredReadOnly: declaredReadOnly(s.Annotations),
	}
	if ev.OperationID == "" && cell.EnforceMode == "off" {
		// Derived class, not an event type (§16): in observe-only mode an
		// unprescribed execution is legitimate traffic, so it is counted and not
		// flagged.
		cell.UnprescribedExecutions++
	}
	if op, ok := ops[key]; ok {
		op.execIDs = append(op.execIDs, s.ExecutionID)
	}
	// Two indexes on purpose: `pending` holds starts still waiting for a terminal
	// event and loses entries as they pair, while `starts` keeps every execution
	// record so an operation can be assembled after pairing. Deleting from both
	// would leave a fully reconciled session with nothing to show.
	starts[s.ExecutionID] = ex
	pending[s.ExecutionID] = ex
}

func applyFinish(ops map[opKey]*Operation, ev *evidence.Event, pending pendingExecutions) {
	p, err := ev.DecodePayload()
	if err != nil {
		return
	}
	f := p.(*evidence.ExecutionFinishedPayload)
	ex, ok := pending[f.ExecutionID]
	if !ok {
		return
	}
	ex.Status = string(f.Status)
	ex.ResultFingerprintStatus = f.ResultFingerprintStatus
	ex.ErrorCode = f.ErrorCode
	ex.DurationMS = f.DurationMS
	ex.FinishedAt = f.FinishedAt
	ex.Paired = true
	delete(pending, f.ExecutionID)
}

func applyBlock(cell *Cell, ev *evidence.Event, blocks map[string][]blockFact) {
	cell.BlockedAttempts++
	blocks[ev.SessionID] = append(blocks[ev.SessionID], blockFact{sessionID: ev.SessionID, at: ev.RecordedAt})
}

func applyDegraded(cell *Cell, ev *evidence.Event) {
	p, err := ev.DecodePayload()
	if err != nil {
		return
	}
	d := p.(*evidence.DegradedPayload)
	cell.DegradedWindows++
	if !d.WindowEnd.After(d.WindowStart) {
		return
	}
	cell.DegradedMS += d.WindowEnd.Sub(d.WindowStart).Milliseconds()
}

// markStopped applies §34's third state: an operation still open when the
// recorder stopped with a known reason is an interrupted session, not a crash and
// not a silent success.
func markStopped(ops map[opKey]*Operation, recorder string) {
	for key, op := range ops {
		if key.recorder != recorder || op.Report != nil {
			continue
		}
		if op.State == stateLifecycleUnknown {
			op.State = stateInterruptedSession
		}
	}
}

func findExec(starts startsIndex, id string) (*Execution, bool) {
	ex, ok := starts[id]
	return ex, ok
}

func finalize(cells map[string]*Cell, ops map[opKey]*Operation, order []opKey,
	blocks map[string][]blockFact, starts startsIndex) error {
	for _, key := range order {
		op := ops[key]
		for _, id := range op.execIDs {
			if ex, ok := findExec(starts, id); ok {
				op.Executions = append(op.Executions, *ex)
			}
		}
		sort.Slice(op.Executions, func(i, j int) bool {
			return op.Executions[i].StartedAt.Before(op.Executions[j].StartedAt)
		})
		annotate(op, blocks[op.SessionID])
		cell, ok := cells[op.group]
		if !ok {
			continue
		}
		cell.Operations = append(cell.Operations, *op)
	}
	for _, c := range cells {
		if c.EnforceMode == "off" {
			c.VoluntaryPrescriptionCoverage = voluntaryCoverage(c)
		}
		if c.EnforceMode == "all" {
			// §36: first-attempt compliance is only meaningful where
			// prescription was required.
			c.FirstAttemptCompliance = fmt.Sprintf("%d/%d", c.SessionsPrescribedFirst, c.SessionsWithAttempts)
		}
		sort.Slice(c.Operations, func(i, j int) bool {
			return c.Operations[i].OperationID < c.Operations[j].OperationID
		})
	}
	return nil
}

// annotate adds §38's findings to one operation.
func annotate(op *Operation, sessionBlocks []blockFact) {
	op.RepeatedArgumentFingerprints = repeatedFingerprints(op.Executions)
	if op.Report == nil || !strings.EqualFold(op.Report.Outcome, "achieved") {
		return
	}
	if len(op.Executions) == 0 {
		op.Anomalies = append(op.Anomalies, AnomalyClaimedAchievedWithoutExecution)
		op.BlockedAttemptsInSession = len(sessionBlocks)
	}
	if errorsPresent(op.Executions) {
		op.SameFingerprintRecovery = fingerprintRecovery(op.Executions)
	}
	if onlyDeclaredReadOnly(op.Executions) {
		op.Facts = append(op.Facts, FactAchievedWithDeclaredReadOnlyOnly)
	}
}

func repeatedFingerprints(execs []Execution) int {
	seen := map[string]int{}
	for _, ex := range execs {
		if ex.ArgumentsHMAC == "" {
			continue
		}
		seen[ex.Tool+"\x00"+ex.ArgumentsHMAC]++
	}
	var repeats int
	for _, n := range seen {
		if n > 1 {
			repeats++
		}
	}
	return repeats
}

func errorsPresent(execs []Execution) bool {
	for _, ex := range execs {
		if ex.Status == string(evidence.ExecutionError) {
			return true
		}
	}
	return false
}

// fingerprintRecovery implements §38.B. Absence of a same-fingerprint success is
// only reportable when both sides of the comparison had a fingerprint: another
// successful action may have solved the problem, and a missing key is not proof
// either way.
func fingerprintRecovery(execs []Execution) string {
	var sawUncomparable bool
	for i, first := range execs {
		if first.Status != string(evidence.ExecutionError) {
			continue
		}
		if first.ArgumentsHMAC == "" {
			sawUncomparable = true
			continue
		}
		found := false
		for _, later := range execs[i+1:] {
			if later.Status != string(evidence.ExecutionSuccess) {
				continue
			}
			if later.ArgumentsHMAC == "" {
				sawUncomparable = true
				continue
			}
			if later.Tool == first.Tool && later.ArgumentsHMAC == first.ArgumentsHMAC {
				found = true
			}
		}
		if !found {
			if sawUncomparable {
				return recoveryInsufficient
			}
			return recoveryNo
		}
	}
	if sawUncomparable {
		return recoveryInsufficient
	}
	return recoveryYes
}

func onlyDeclaredReadOnly(execs []Execution) bool {
	if len(execs) == 0 {
		return false
	}
	for _, ex := range execs {
		if !ex.DeclaredReadOnly {
			return false
		}
	}
	return true
}

func declaredReadOnly(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var ann struct {
		ReadOnlyHint *bool `json:"readOnlyHint"`
	}
	if err := json.Unmarshal(raw, &ann); err != nil {
		return false
	}
	return ann.ReadOnlyHint != nil && *ann.ReadOnlyHint
}

// voluntaryCoverage is only meaningful where prescription was optional.
func voluntaryCoverage(c *Cell) string {
	var withOp int
	for _, op := range c.Operations {
		if op.group == c.Group {
			withOp += len(op.Executions)
		}
	}
	if c.UnprescribedExecutions+withOp == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", withOp, withOp+c.UnprescribedExecutions)
}

func orderedCells(cells map[string]*Cell) []Cell {
	keys := make([]string, 0, len(cells))
	for k := range cells {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]Cell, 0, len(keys))
	for _, k := range keys {
		out = append(out, *cells[k])
	}
	return out
}

// JSON renders the summary for disk.
func (s Summary) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// Findings returns the human-readable one-line-per-finding view used by the
// terminal summary.
func (s Summary) Findings() []string {
	var out []string
	for _, c := range s.Cells {
		out = append(out, fmt.Sprintf("cell %s: %d records, chain=%v coverage=%s, %d operations, %d blocked attempts",
			c.Group, c.Records, boolWord(c.ChainValid), c.Coverage, len(c.Operations), c.BlockedAttempts))
		for _, op := range c.Operations {
			out = append(out, fmt.Sprintf("  %s %s executions=%d report=%s %s",
				op.OperationID, op.State, len(op.Executions), reportWord(op), strings.Join(append(op.Anomalies, op.Facts...), " ")))
		}
	}
	return out
}

func boolWord(v bool) string {
	if v {
		return "valid"
	}
	return "INVALID"
}

func reportWord(op Operation) string {
	if op.Report == nil {
		return "none"
	}
	return op.Report.Status + "/" + op.Report.Outcome
}
