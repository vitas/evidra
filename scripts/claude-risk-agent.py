#!/usr/bin/env python3
"""Claude CLI headless artifact risk assessor for Evidra benchmark experiments."""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run Claude headless artifact risk assessment")
    parser.add_argument("--model-id", required=True, help="Model id label, e.g. claude/sonnet")
    parser.add_argument("--artifact", default=os.getenv("EVIDRA_ARTIFACT_PATH", ""), help="Path to artifact file")
    parser.add_argument("--expected-json", default=os.getenv("EVIDRA_EXPECTED_JSON", ""), help="Path to expected.json")
    parser.add_argument("--output", default=os.getenv("EVIDRA_AGENT_OUTPUT", ""), help="Output JSON path")
    parser.add_argument(
        "--prompt-file",
        default=os.getenv(
            "EVIDRA_PROMPT_FILE",
            str(Path(__file__).resolve().parents[1] / "prompts/experiments/runtime/system_instructions.txt"),
        ),
        help="Prompt instructions file path",
    )
    parser.add_argument(
        "--cli-model",
        default=os.getenv("CLAUDE_HEADLESS_MODEL", ""),
        help="Claude CLI model alias override (e.g. sonnet, haiku, opus)",
    )
    parser.add_argument(
        "--raw-stream-out",
        default=os.getenv("EVIDRA_AGENT_RAW_STREAM", ""),
        help="Optional path to store raw Claude stream-json output",
    )
    return parser.parse_args()


def fail(msg: str) -> None:
    print(f"claude-risk-agent: FAIL {msg}", file=sys.stderr)
    raise SystemExit(1)


def read_text(path: str, what: str) -> str:
    if not path:
        fail(f"missing {what} path")
    p = Path(path)
    if not p.is_file():
        fail(f"{what} not found: {path}")
    return p.read_text(encoding="utf-8")


def write_text(path: str, content: str) -> None:
    if not path:
        return
    out = Path(path)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(content, encoding="utf-8")


def parse_contract_version(text: str) -> str:
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        if line.startswith("<!--") and line.endswith("-->"):
            line = line[4:-3].strip()
        if line.startswith("#"):
            line = line[1:].strip()
        if not line.lower().startswith("contract:"):
            return "unknown"
        value = line.split(":", 1)[1].strip()
        return value if value else "unknown"
    return "unknown"


def strip_contract_header(text: str) -> str:
    out = []
    skipped = False
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not skipped and (not line):
            continue
        if not skipped:
            probe = line
            if probe.startswith("<!--") and probe.endswith("-->"):
                probe = probe[4:-3].strip()
            if probe.startswith("#"):
                probe = probe[1:].strip()
            if probe.lower().startswith("contract:"):
                skipped = True
                continue
        skipped = True
        out.append(raw_line)
    return "\n".join(out).strip()


def load_expected(path: str) -> dict:
    if not path:
        return {}
    p = Path(path)
    if not p.is_file():
        return {}
    try:
        return json.loads(p.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}


def build_user_prompt(artifact_text: str, expected: dict) -> str:
    case_id = str(expected.get("case_id", "unknown"))
    category = str(expected.get("category", "unknown"))
    difficulty = str(expected.get("difficulty", "unknown"))
    return (
        "Assessment mode: classify this infrastructure artifact.\n"
        "Return ONLY JSON with keys predicted_risk_level and predicted_risk_details.\n"
        "Allowed predicted_risk_level values: low, medium, high, critical, unknown.\n"
        f"case_id={case_id}\n"
        f"category={category}\n"
        f"difficulty={difficulty}\n\n"
        "Artifact:\n"
        "-----BEGIN ARTIFACT-----\n"
        f"{artifact_text}\n"
        "-----END ARTIFACT-----\n"
    )


def resolve_claude_model(model_id: str, cli_model_override: str) -> str:
    if cli_model_override:
        return cli_model_override
    if model_id.startswith("claude/"):
        return model_id.split("/", 1)[1]
    if model_id.startswith("anthropic/"):
        low = model_id.lower()
        if "haiku" in low:
            return "haiku"
        if "sonnet" in low:
            return "sonnet"
        if "opus" in low:
            return "opus"
    return model_id


def run_claude(system_prompt: str, user_prompt: str, cli_model: str) -> str:
    cmd = [
        "claude",
        "-p",
        user_prompt,
        "--output-format",
        "stream-json",
        "--verbose",
        "--model",
        cli_model,
        "--append-system-prompt",
        system_prompt,
    ]
    proc = subprocess.run(  # noqa: S603
        cmd,
        stdin=subprocess.DEVNULL,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        stderr_line = (proc.stderr or "").strip().splitlines()
        if stderr_line:
            fail(f"claude command failed: {stderr_line[0]}")
        fail(f"claude command failed with exit code {proc.returncode}")
    return proc.stdout or ""


def extract_text_from_stream(stream_text: str) -> str:
    pieces: list[str] = []
    for raw_line in stream_text.splitlines():
        line = raw_line.strip()
        if not line:
            continue
        try:
            evt = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(evt, dict):
            continue
        evt_type = evt.get("type")
        if evt_type == "text" and isinstance(evt.get("text"), str):
            pieces.append(evt["text"])
        if evt_type == "assistant":
            msg = evt.get("message", {})
            if isinstance(msg, dict):
                for block in msg.get("content", []) or []:
                    if isinstance(block, dict) and block.get("type") == "text" and isinstance(block.get("text"), str):
                        pieces.append(block["text"])
        if evt_type == "content_block_start":
            block = evt.get("content_block", {})
            if isinstance(block, dict) and block.get("type") == "text" and isinstance(block.get("text"), str):
                pieces.append(block["text"])
        if evt_type == "content_block_delta":
            delta = evt.get("delta", {})
            if isinstance(delta, dict) and isinstance(delta.get("text"), str):
                pieces.append(delta["text"])
        if evt_type == "result":
            result = evt.get("result")
            if isinstance(result, str):
                pieces.append(result)
            elif isinstance(result, dict):
                pieces.append(json.dumps(result, ensure_ascii=True))
    return "\n".join(pieces).strip()


def extract_json(text: str) -> dict:
    txt = text.strip()
    try:
        data = json.loads(txt)
        if isinstance(data, dict):
            return data
    except json.JSONDecodeError:
        pass

    # Scan for the first valid JSON object inside mixed text or multi-event streams.
    decoder = json.JSONDecoder()
    for idx, ch in enumerate(txt):
        if ch != "{":
            continue
        try:
            data, _ = decoder.raw_decode(txt[idx:])
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict):
            return data

    for match in re.finditer(r"\{[^{}]*\}", txt, flags=re.DOTALL):
        try:
            data = json.loads(match.group(0))
        except json.JSONDecodeError:
            continue
        if isinstance(data, dict):
            return data
    return {}


def normalize_output(raw: dict) -> dict:
    level = str(raw.get("predicted_risk_level", raw.get("risk_level", "unknown")) or "unknown").strip().lower()
    allowed_levels = {"low", "medium", "high", "critical", "unknown"}
    if level not in allowed_levels:
        level = "unknown"

    details = raw.get("predicted_risk_details", raw.get("predicted_risk_tags", raw.get("risk_tags", [])))
    if not isinstance(details, list):
        details = []
    clean_details = sorted({str(x).strip() for x in details if str(x).strip()})
    return {"predicted_risk_level": level, "predicted_risk_details": clean_details}


def main() -> None:
    args = parse_args()

    if not args.output:
        fail("missing output path (pass --output or set EVIDRA_AGENT_OUTPUT)")

    artifact_text = read_text(args.artifact, "artifact")
    prompt_text = read_text(args.prompt_file, "prompt file")
    expected = load_expected(args.expected_json)

    contract_version = parse_contract_version(prompt_text)
    system_prompt = strip_contract_header(prompt_text)
    user_prompt = build_user_prompt(artifact_text, expected)
    cli_model = resolve_claude_model(args.model_id, args.cli_model)

    stream = run_claude(system_prompt=system_prompt, user_prompt=user_prompt, cli_model=cli_model)
    write_text(args.raw_stream_out, stream)

    extracted = extract_text_from_stream(stream)
    if not extracted:
        fail("no parseable text events found in Claude stream output")

    parsed = extract_json(extracted)
    if not parsed:
        fail("could not parse JSON object from Claude stream output")

    out = normalize_output(parsed)
    out["prompt_contract_version"] = contract_version
    out["model_id"] = args.model_id
    out["claude_model"] = cli_model

    output_path = Path(args.output)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(json.dumps(out, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
