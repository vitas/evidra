#!/usr/bin/env bash

if [[ "$MODE" != "local-mcp" ]]; then
  skip "chain" "chain test currently runs only in local-mcp mode"
  return
fi

reset_evidence

prescribe_args=$(jq -n --arg raw "$(cat "$REPO_ROOT/tests/inspector/fixtures/safe-nginx-deployment.yaml")" '{
  actor: {type:"agent", id:"chain-agent", origin:"mcp"},
  tool: "kubectl",
  operation: "apply",
  raw_artifact: $raw
}')

prescribe_body=$(call_prescribe "$prescribe_args") || {
  fail "chain/prescribe" "call failed"
  return
}

if [[ "$(echo "$prescribe_body" | jq -r '.ok')" != "true" ]]; then
  fail "chain/prescribe_ok" "expected ok=true"
  return
fi
pass "chain/prescribe_ok"

prescription_id=$(echo "$prescribe_body" | jq -r '.prescription_id // empty')
if [[ -z "$prescription_id" ]]; then
  fail "chain/prescription_id" "empty"
  return
fi
pass "chain/prescription_id_present"

artifact_digest=$(echo "$prescribe_body" | jq -r '.artifact_digest // empty')
report_args=$(jq -n --arg pid "$prescription_id" --arg ad "$artifact_digest" '{
  prescription_id: $pid,
  verdict: "success",
  exit_code: 0,
  artifact_digest: $ad,
  actor: {type:"agent", id:"chain-agent", origin:"mcp"}
}')

report_body=$(call_report "$report_args") || {
  fail "chain/report" "call failed"
  return
}

if [[ "$(echo "$report_body" | jq -r '.ok')" != "true" ]]; then
  fail "chain/report_ok" "expected ok=true"
  return
fi
pass "chain/report_ok"

report_id=$(echo "$report_body" | jq -r '.report_id // empty')
if [[ -z "$report_id" ]]; then
  fail "chain/report_id" "empty"
  return
fi
pass "chain/report_id_present"

presc_event=$(call_get_event "$prescription_id") || {
  fail "chain/get_prescribe_event" "call failed"
  return
}
if [[ "$(echo "$presc_event" | jq -r '.ok')" != "true" ]]; then
  fail "chain/get_prescribe_event_ok" "expected ok=true"
else
  pass "chain/get_prescribe_event_ok"
fi
if [[ "$(echo "$presc_event" | jq -r '.entry.type // empty')" == "prescribe" ]]; then
  pass "chain/prescribe_entry_type"
else
  fail "chain/prescribe_entry_type" "expected prescribe"
fi

report_event=$(call_get_event "$report_id") || {
  fail "chain/get_report_event" "call failed"
  return
}
if [[ "$(echo "$report_event" | jq -r '.ok')" != "true" ]]; then
  fail "chain/get_report_event_ok" "expected ok=true"
else
  pass "chain/get_report_event_ok"
fi
if [[ "$(echo "$report_event" | jq -r '.entry.type // empty')" == "report" ]]; then
  pass "chain/report_entry_type"
else
  fail "chain/report_entry_type" "expected report"
fi
