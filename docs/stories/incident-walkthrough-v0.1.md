# Incident Walkthrough: Degraded after Deploy (v0.1)

This walkthrough demonstrates how to investigate a production degradation after a deployment using Evidra v0.1.
It is written as a practical exercise focused on safe conclusions and audit-ready output.

## 1) Situation

A production deployment occurred in Argo CD for `payments-api`.
Within minutes, application health changed to `Degraded`.

Known:
- Service health degraded after a deploy window.
- Argo CD completed at least one sync operation.

Unknown:
- Which exact revision was deployed.
- Who initiated the operation.
- Whether degradation was observed after deploy start.
- Whether rollback occurred.

## 2) Investigation goal

Answer the following:
- What changed in production?
- Who initiated the deploy?
- Where was it deployed (cluster and namespace)?
- When did deploy start and finish?
- Was degradation observed after deploy start?
- Was rollback performed?
- What evidence can be exported for incident review and audit?

## 3) Environment assumptions

- Argo CD is managing the target application.
- Evidra extension is installed and visible in Argo CD.
- Incident window is within the last 4 hours.
- Application has correlation annotations, for example:
  - `evidra.io/change-id: CHG-48219`
  - `evidra.io/ticket: OPS-914`

## 4) Step-by-step walkthrough

1. Open Argo CD -> Evidra Explorer.
- Action: Sign in to Argo CD and open the `Evidra` tab.
- Evidra shows: Explorer with filters, Changes list, and investigation panel.
- Safe conclusion: Investigation context is ready.
- Not safe: No conclusions about cause yet.

2. Narrow search by application and time range.
- Action: Select subject/application for `payments-api`, set a recent time range, click `Find changes`.
- Evidra shows: Filtered list of candidate Changes in the window.
- Safe conclusion: Candidate operations are constrained by scope and time.
- Not safe: First visible Change is not automatically the incident trigger.

3. Identify the relevant Change (deploy operation).
- Action: Select the Change near incident start time.
- Evidra shows: Selected Change Summary with status and timestamps.
- Safe conclusion: This is the operation selected for investigation.
- Not safe: Temporal proximity alone does not prove causation.

4. Open Change Summary and extract key facts.
- Action: Read the summary block and copy fields if needed.
- Evidra shows:
  - `change_id`
  - revision/commit
  - cluster and namespace
  - initiator identity
  - operation start/end
  - external change id / ticket (if present)
- Safe conclusion: This is the authoritative v0.1 snapshot for what/where/who/when.
- Not safe: External annotation presence does not validate approval policy by itself.

5. Inspect the narrative timeline.
- Action: Review ordered events around deploy and health transitions.
- Evidra shows:
  - deploy start and completion
  - health transitions
  - first degraded timestamp when available
- Safe conclusion: Event ordering and timestamps support sequence analysis.
- Not safe: Sequence does not prove root cause.

6. Determine degradation-after-deploy status.
- Action: Check the summary indicator for post-deploy degradation.
- Evidra shows: `Degradation observed after deploy` (only when criteria match).
- Safe conclusion: Degradation was observed after deployment started.
- Not safe: Evidra does not claim deploy caused degradation.

7. Check for rollback or recovery Change.
- Action: Search the same app/time window for subsequent rollback/recovery operations.
- Evidra shows: Later Change entries and timeline outcomes.
- Safe conclusion: You can confirm whether rollback/recovery occurred and when.
- Not safe: Absence of a rollback Change does not mean no remediation happened outside Argo CD.

8. Export evidence pack (JSON).
- Action: Click export for the selected Change.
- Evidra shows: Export artifact identifier or downloadable pack.
- Safe conclusion: Export captures deterministic investigation evidence for sharing.
- Not safe: Export is not a full compliance workflow record by itself.

9. Attach/share permalink and export in incident record.
- Action: Copy Change permalink and attach export to incident ticket/postmortem.
- Evidra shows: Stable Change link and exported evidence metadata.
- Safe conclusion: Reviewers can reopen the same investigation context consistently.
- Not safe: Permalink alone is not immutable storage outside your retention policy.

## 5) Evidence pack contents

Typical redacted structure:

```json
{
  "change_id": "chg_...",
  "generated_at": "2026-02-18T12:34:56Z",
  "source": "argocd",
  "application": "payments-api",
  "cluster": "prod-eu",
  "namespace": "payments",
  "revision": "abc1234",
  "initiator": "ops-user",
  "result": "succeeded",
  "post_deploy_degradation": {
    "observed": true,
    "first_timestamp": "2026-02-18T12:18:03Z"
  },
  "timeline": [
    { "type": "argo.sync.started", "timestamp": "..." },
    { "type": "argo.sync.finished", "timestamp": "..." },
    { "type": "argo.health.changed", "timestamp": "..." }
  ]
}
```

## 6) Outcome summary

At the end of this walkthrough, the on-call reviewer can state:
- what revision was deployed and where,
- who initiated the operation,
- when degradation was first observed after deploy,
- whether rollback/recovery occurred in Argo CD history,
- which evidence export and permalink were attached to incident artifacts.

## 7) Common pitfalls

- Confusing `No results` with `Not searched yet`.
  - Mitigation: Use explicit empty-state text and run `Find changes` after filter updates.
- Assuming causality from timeline order.
  - Mitigation: Use observational wording only; avoid root-cause claims from sequence alone.
- Timezone mismatch between operators.
  - Mitigation: Confirm timestamps and incident window in consistent timezone/UTC.
- Retries creating similar operations.
  - Mitigation: Compare `change_id`, operation timing, and revision together.
- Truncated IDs copied from UI labels.
  - Mitigation: Use copy actions in summary for full IDs/revisions/permalink.
- Stale results after changing filters.
  - Mitigation: Re-run search and verify selected Change still matches current filters.

## 8) v0.1 limitations

- No direct Jira/ITSM lookup.
- No monitoring or alerting features.
- No cross-tool correlation beyond Argo CD data plus annotations.
- No policy enforcement workflow.

## 9) How to reproduce the demo locally

Interactive sandbox:

```bash
cp .env.example .env
make evidra-demo
```

Open:
- `http://localhost:8080/ui/`
 - `https://localhost:8081/` (Argo CD with Evidra extension, use printed admin password)

Run deterministic case suite:

```bash
make evidra-demo-test
```
