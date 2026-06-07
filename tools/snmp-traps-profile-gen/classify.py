#!/usr/bin/env python3
"""
SOW-0034 - SNMP Trap Profile LLM Enrichment Pipeline.

Reads output/extracted.jsonl (produced by extract.py), submits each trap
to the LLM gateway with a 4-field classification prompt, validates the
response, and writes one JSON file per OID under output/enriched/.

Strict rules:
  * Endpoint-neutral: pass OpenAI-compatible endpoints explicitly with
    --endpoint, or set the SNMP_TRAP_PROFILE_GEN_* environment defaults.
  * No authentication header is managed by this helper; use an endpoint or
    gateway whose auth policy is handled outside committed defaults.
  * Concurrent in-flight slots are configurable per endpoint.
  * Schema-validated output; up to 3 LLM attempts with feedback in the
    retry prompt; mechanical regex fallback only if all 3 attempts fail.
  * Atomic per-OID file writes; resumable runs.
"""

from __future__ import annotations

import argparse
import asyncio
import contextlib
import hashlib
import json
import logging
import os
import re
import secrets
import sys
import tempfile
import time
import traceback
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional, Tuple

import httpx


# --------------------------------------------------------------------------
# Configuration
# --------------------------------------------------------------------------


LLM_BASE_URL = os.environ.get("SNMP_TRAP_PROFILE_GEN_LLM_BASE_URL", "http://127.0.0.1:8000/v1")
MODEL = os.environ.get("SNMP_TRAP_PROFILE_GEN_MODEL", "local-model")
DEFAULT_CONCURRENCY = int(os.environ.get("SNMP_TRAP_PROFILE_GEN_CONCURRENCY", "8"))
MAX_LLM_ATTEMPTS = 3
DESCRIPTION_MAX_BYTES = 1024


ALLOWED_CATEGORIES = (
    "state_change",
    "config_change",
    "security",
    "auth",
    "license",
    "mobility",
    "diagnostic",
    "unknown",
)

# Map every accepted severity to its syslog PRIORITY value.
SEVERITY_TO_PRIORITY: Dict[str, int] = {
    "emerg":   0,
    "alert":   1,
    "crit":    2,
    "err":     3,
    "warning": 4,
    "notice":  5,
    "info":    6,
    "debug":   7,
}
ALLOWED_SEVERITIES = tuple(SEVERITY_TO_PRIORITY.keys())

STANDARD_PLACEHOLDERS = (
    "SNMP_DEVICE_HOSTNAME",
    "SNMP_SOURCE_IP",
    "SNMP_TRAP_NAME",
    "SNMP_DEVICE_VENDOR",
)


BANNED_REGEX = re.compile(
    r"\b(world[- ]class|powerful|catastrophic|severe issue|critical issue|cutting[- ]edge)\b",
    re.IGNORECASE,
)
BACKOFF_RANDOM = secrets.SystemRandom()


# --------------------------------------------------------------------------
# System prompt (constant — prefix-cached by sglang across all requests)
# --------------------------------------------------------------------------


SYSTEM_PROMPT = """You are classifying SNMP traps for the Netdata observability platform.
For every trap supplied to you, you output exactly one JSON object — no commentary, no markdown fences.
The JSON object has three keys:

{
  "category":    "<one of the 8 canonical slugs>",
  "severity":    "<one of the 8 syslog severity slugs>",
  "description": "<one operator-facing description, max 1024 bytes, newlines OK>"
}

══════════════════════════════════════════════════════════════════════
FIELD 1 — category (pick exactly one slug from the 8 canonical):
══════════════════════════════════════════════════════════════════════

state_change   — A device entity transitioned between operational states.
                 Examples: linkDown / linkUp, coldStart / warmStart, BGP / OSPF / IS-IS
                 transitions, peer up / down, fan / PSU / temperature threshold crossed,
                 port admin / oper state change, redundancy state change.
                 NOT this if the trap reports CONFIGURATION being changed
                 (use config_change) or AUTHENTICATION attempts (use auth).

config_change  — The configuration of the device was modified or saved.
                 Examples: running-config edited, startup-config replaced, ACL added,
                 BGP peer added, license file replaced, audit events from change-management
                 workflows.
                 NOT this for OPERATIONAL state transitions (use state_change).

security       — Security violation or detection event with per-event identifying
                 information beyond simple authentication. Examples: port-security MAC
                 violations, DHCP-snooping drops, dynamic-ARP-inspection drops, ACL hits,
                 IPS / IDS signature matches, threat detection.
                 NOT this for plain authentication failures without further security
                 context (use auth).

auth           — Authentication-related event (failures or successes).
                 Examples: SNMP authentication failure, console / SSH login failures,
                 RADIUS / TACACS+ events, AAA accounting.

license        — Licensing or compliance event. Examples: license expired, license
                 expiring soon, feature unlocked, feature locked, compliance violation,
                 license entitlement change.

mobility       — Identity-move events with the actor named. Examples: MAC moved from
                 port A to port B, STP newRoot (which bridge), topology change with
                 cause. Use when the IDENTITY of the new / old actor is the
                 operator-relevant content.
                 NOT this for simple state changes without actor-identity emphasis
                 (use state_change).

diagnostic     — Vendor-specific diagnostic event with device-determined context that
                 doesn't fit other categories. Examples: reboot reason codes, module
                 insertion / removal with FRU info, RAID array events (rebuild / fail),
                 optical transceiver DOM events, internal process restart with reason.

unknown        — RESERVED. Use ONLY when, after honest analysis, no other slug fits.
                 Picking `unknown` is a signal to human curators that this trap's
                 domain is not yet in our taxonomy. It is NOT a way to skip
                 classification effort. Prefer the closest reasonable slug over
                 `unknown` when a defensible match exists.

══════════════════════════════════════════════════════════════════════
FIELD 2 — severity (pick exactly one, mapped to syslog PRIORITY):
══════════════════════════════════════════════════════════════════════

emerg   (PRIORITY=0) — System unusable. Extremely rare for SNMP traps.
                       Examples: total system halt, last redundancy lost with no
                       failover possible.

alert   (PRIORITY=1) — Immediate action required. Examples: hardware fault in
                       non-redundant component, security compromise detected,
                       loss of primary subsystem.

crit    (PRIORITY=2) — Critical condition. Service impact certain.
                       Examples: license expired (feature stops working), FRU
                       removed from running system, major hardware failure,
                       loss of last redundancy.

err     (PRIORITY=3) — Error condition. Functional impact likely.
                       Examples: BGP peer unreachable sustained, repeated auth
                       failures, interface error counter threshold crossed.

warning (PRIORITY=4) — Operator attention warranted; no certain service impact yet.
                       Examples: link down (could be planned), single auth failure,
                       port-security violation, fan-redundancy lost (still operating).

notice  (PRIORITY=5) — Normal but significant. Examples: config change saved, peer
                       added, role transition, license entitlement changed (not
                       expired), MAC moved (expected in dynamic networks).

info    (PRIORITY=6) — Informational; normal operation. Examples: link came up,
                       peer established, warm start, feature unlocked.

debug   (PRIORITY=7) — Debug-level. Almost never appropriate for an SNMP trap.
                       Use ONLY if the trap is explicitly described as diagnostic
                       debug output (very rare).

══════════════════════════════════════════════════════════════════════
FIELD 3 — description (operator-facing template):
══════════════════════════════════════════════════════════════════════

  • Max 1024 bytes. Multiple lines OK (newlines welcome where they aid clarity).
  • Use {varbind_name} placeholders for inline substitutions.
  • Use {SNMP_DEVICE_HOSTNAME} for the device identifier.
  • Use varbind names EXACTLY as supplied in the user prompt's varbinds list.
  • Do NOT invent varbind names that are not in the list.
  • Do NOT "fix" what look like typos or odd spellings in varbind names
    (e.g., ngev*, cpqDa*, cvm*, hpHttp*, sd_*). These are the exact symbol
    names the vendor defined in their MIB; copy them character-for-character
    from the varbinds list — never rewrite them to look more conventional.
  • Plain factual language. NO marketing or dramatic terms ("critical issue",
    "severe degradation", "world-class", "powerful"). State what happened.
  • Include the operator-relevant varbinds — those that identify WHAT was
    affected, not internal indexes alone (prefer ifDescr over ifIndex if both
    are present).
  • For traps with no varbinds, still reference {SNMP_DEVICE_HOSTNAME}.
  • A short single-line description is OK when the trap is simple. Use multiple
    lines (with newlines) when the trap has rich varbinds worth listing
    separately, or when investigation hints add real value.

══════════════════════════════════════════════════════════════════════
EXAMPLES (output JSON only — no fences, no commentary):
══════════════════════════════════════════════════════════════════════

[Input]
trap_name: linkDown
mib_module: IF-MIB
varbinds: ifIndex (INTEGER), ifAdminStatus (INTEGER), ifOperStatus (INTEGER)

[Output]
{
  "category":"state_change",
  "severity":"warning",
  "description":"Interface {ifIndex} down on {SNMP_DEVICE_HOSTNAME}\\n  admin: {ifAdminStatus}\\n  oper: {ifOperStatus}"
}

[Input]
trap_name: ccmCLIRunningConfigChanged
mib_module: CISCO-CONFIG-MAN-MIB
varbinds: ccmHistoryEventCommandSource (INTEGER), ccmHistoryEventConfigSource (INTEGER)
varbinds: ccmHistoryEventConfigDestination (INTEGER)

[Output]
{
  "category":"config_change",
  "severity":"notice",
  "description":"Cisco running-config changed on {SNMP_DEVICE_HOSTNAME}"
}

[Input]
trap_name: authenticationFailure
mib_module: SNMPv2-MIB
varbinds: (none)

[Output]
{"category":"auth","severity":"warning","description":"SNMP authentication failure on {SNMP_DEVICE_HOSTNAME}"}

[Input]
trap_name: ciscoPsmTrapSrvUnauthorized
mib_module: CISCO-PORT-SECURITY-MIB
varbinds: cpsIfViolationMacAddress (OctetString), cpsIfViolationVlan (INTEGER), ifIndex (INTEGER)

[Output]
{
  "category":"security",
  "severity":"warning",
  "description":"Port-security violation on {SNMP_DEVICE_HOSTNAME}\\n  MAC: {cpsIfViolationMacAddress}"
}

[Input]
trap_name: hh3cLicenseExpired
mib_module: HH3C-LICENSE-MIB
varbinds: hh3cLicenseFeatureName (OctetString), hh3cLicenseFeatureState (INTEGER)

[Output]
{
  "category":"license",
  "severity":"crit",
  "description":"License expired on {SNMP_DEVICE_HOSTNAME}\\n  feature: {hh3cLicenseFeatureName}"
}

[Input]
trap_name: ieee8021SpanningTreeNewRoot
mib_module: IEEE8021-SPANNING-TREE-MIB
varbinds: (none)

[Output]
{
  "category":"mobility",
  "severity":"notice",
  "description":"Device {SNMP_DEVICE_HOSTNAME} elected as new STP root bridge"
}

[Input]
trap_name: cefcFRURemoved
mib_module: CISCO-ENTITY-FRU-CONTROL-MIB
varbinds: entPhysicalDescr (OctetString), entPhysicalIndex (INTEGER)

[Output]
{
  "category":"diagnostic",
  "severity":"crit",
  "description":"FRU removed from {SNMP_DEVICE_HOSTNAME}\\n  description: {entPhysicalDescr}"
}
"""


# --------------------------------------------------------------------------
# Prompt building (user prompt — variable per request)
# --------------------------------------------------------------------------


def truncate(s: Optional[str], n: int) -> str:
    if not s:
        return ""
    s = " ".join(s.split())
    return s if len(s) <= n else s[: n - 3] + "..."


def render_varbind_line(vb: Dict[str, Any]) -> str:
    name = vb.get("name") or "?"
    syntax = vb.get("syntax") or ""
    enum = vb.get("enum")
    enum_summary = ""
    if isinstance(enum, dict) and enum:
        items = list(enum.items())[:6]
        enum_summary = " enum=" + ",".join(f"{v}={k}" for k, v in items)
        if len(enum) > 6:
            enum_summary += ",..."
    desc = truncate(vb.get("description"), 200)
    return f"    - {name} ({syntax}{enum_summary}) - {desc}"


def build_user_prompt(trap: Dict[str, Any]) -> str:
    oid = trap.get("oid") or ""
    name = trap.get("name") or ""
    mib = trap.get("mib") or ""
    mib_desc = truncate(trap.get("mib_description"), 240)
    trap_desc = truncate(trap.get("trap_description"), 800)
    trap_ref = truncate(trap.get("trap_reference"), 80)

    lines = [
        "Classify this SNMP trap. Output JSON only.",
        "",
        f"trap_oid:         {oid}",
        f"trap_name:        {name}",
        f"mib_module:       {mib}",
        f"mib_purpose:      {mib_desc}",
        f"trap_description: {trap_desc}",
    ]
    if trap_ref:
        lines.append(f"trap_reference:   {trap_ref}")
    # Show ONLY varbinds with BOTH ``oid`` and ``syntax`` set. That is the
    # exact filter ``emit.py`` uses to admit a varbind into the shipped
    # YAML's file-level ``varbinds:`` table. Any varbind the LLM references
    # here will therefore survive to disk; descriptions cannot reference a
    # name that emit will silently drop.
    all_vbs = trap.get("varbinds") or []
    resolved_vbs = [
        vb for vb in all_vbs
        if isinstance(vb, dict) and vb.get("oid") and vb.get("syntax")
    ]
    if resolved_vbs:
        lines.append("varbinds:")
        for vb in resolved_vbs:
            lines.append(render_varbind_line(vb))
    else:
        lines.append("varbinds: (none)")
    return "\n".join(lines)


def build_retry_prompt(trap: Dict[str, Any], previous_failure_reason: str, previous_response: str) -> str:
    """User prompt augmented with feedback from the previous failed attempt."""
    base = build_user_prompt(trap)
    return (
        "Your previous attempt failed validation. "
        f"Reason: {previous_failure_reason}\n"
        f"Previous output (first 300 chars): {previous_response[:300]!r}\n"
        "\n"
        "Re-classify the trap below. Output ONLY the JSON object as instructed in the system prompt "
        "— no commentary, no markdown fences.\n"
        "\n"
        f"{base}"
    )


# --------------------------------------------------------------------------
# Mechanical fallback (last resort if all LLM attempts fail validation)
# --------------------------------------------------------------------------


CATEGORY_KEYWORDS: List[Tuple[str, str]] = [
    ("auth",          r"auth(enticat|oriz|n)|login|logoff|logout|password|credential|aaaserver"),
    (
        "security",
        r"violation|intrus|attack|tamper|spoof|firewall|acl[- ]?(hit|deny|drop)|"
        r"ipsec|portsec|port[-_ ]?security|antivirus",
    ),
    ("license",       r"licens|entitle|expir|grace[- ]?period"),
    (
        "config_change",
        r"config(uration)?(chang|sav|updat|commit|rollback|loaded|written)|"
        r"runningconfig|startupconfig|configmgmt|configurationchange",
    ),
    (
        "mobility",
        r"macmov|macnotif|macaddressmoved|stproot|stpnewroot|stptopology|"
        r"topologychange|hsrp|vrrp",
    ),
    (
        "diagnostic",
        r"reboot|reset|powerfail|psu|powersupply|\bfan\b|temperat|optical|sfp|"
        r"xcvr|raid|battery|moduleinsert|moduleremov|hot[- ]?swap|sensoralarm|"
        r"envmon|hardwarefault",
    ),
    (
        "state_change",
        r"linkdown|linkup|coldstart|warmstart|bgp(establish|backward)|"
        r"ospf(if|nbr)stat|state[- ]?(chang|trans)",
    ),
]


def mechanical_fallback(trap: Dict[str, Any]) -> Dict[str, Any]:
    name = (trap.get("name") or "").strip()
    description_text = trap.get("trap_description") or ""
    haystack = f"{name} {description_text}".lower()
    category = "unknown"
    for cat, pat in CATEGORY_KEYWORDS:
        if re.search(pat, haystack, re.IGNORECASE):
            category = cat
            break
    # Severity default mapped to 8-level set; conservative info/warning/crit only.
    severity = "info"
    if category in ("auth", "security"):
        severity = "warning"
    elif category == "state_change":
        if re.search(r"down|fail|stop|backward|critic", haystack, re.IGNORECASE):
            severity = "warning"
    elif category == "diagnostic":
        if re.search(r"fail|alarm|critic|fault|degrad", haystack, re.IGNORECASE):
            severity = "warning"
    elif category == "license":
        if re.search(r"expir", haystack, re.IGNORECASE):
            severity = "crit"
    template = "{SNMP_TRAP_NAME} from {SNMP_DEVICE_HOSTNAME} ({SNMP_SOURCE_IP})"
    return {
        "category": category,
        "severity": severity,
        "description": template,
    }


# --------------------------------------------------------------------------
# Response validation
# --------------------------------------------------------------------------


# Permissive: match any {non-empty-non-whitespace} token. We do NOT restrict
# to MIB-symbol form because the validator must REJECT bad placeholders the
# model might have invented (e.g. hyphenated names that pysmi can't produce
# because SMIv2 symbols use underscores or camelCase only).
PLACEHOLDER_RE = re.compile(r"\{([^{}\s]+)\}")


def extract_placeholders(s: str) -> List[str]:
    # Strip optional ``.suffix`` so ``{ifOperStatus.raw}`` validates as
    # ``ifOperStatus``.
    return [m.split(".", 1)[0] for m in PLACEHOLDER_RE.findall(s)]


def normalize_response_text(response_text: str) -> str:
    text = response_text.strip()
    # Strip optional ```json fences a chatty model might add.
    if text.startswith("```"):
        text = text.strip("`")
        text = re.sub(r"^json\s*", "", text, flags=re.IGNORECASE)
        text = text.strip("`").strip()
    # Some models prepend explanation; try to locate a JSON object.
    if not text.startswith("{"):
        m = re.search(r"\{.*\}", text, re.DOTALL)
        if m:
            text = m.group(0)
    return text


def parse_response_object(text: str) -> Tuple[Optional[Dict[str, Any]], Optional[str]]:
    try:
        obj = json.loads(text)
    except Exception as exc:
        return None, f"JSON parse failed: {exc}"
    if not isinstance(obj, dict):
        return None, "response is not a JSON object"
    return obj, None


def validate_response_keys(obj: Dict[str, Any]) -> Optional[str]:
    keys = set(obj.keys())
    required = {"category", "severity", "description"}
    missing = required - keys
    extra = keys - required
    if missing:
        return f"missing required keys: {sorted(missing)}"
    if extra:
        return f"unexpected extra keys: {sorted(extra)}"
    return None


def validate_response_values(obj: Dict[str, Any], varbind_names: List[str]) -> Optional[str]:
    if obj["category"] not in ALLOWED_CATEGORIES:
        return f"category not allowed: {obj['category']!r} (expected one of {ALLOWED_CATEGORIES})"
    if obj["severity"] not in ALLOWED_SEVERITIES:
        return f"severity not allowed: {obj['severity']!r} (expected one of {ALLOWED_SEVERITIES})"
    desc = obj["description"]
    if not isinstance(desc, str):
        return "description not a string"
    desc_bytes = len(desc.encode("utf-8"))
    if desc_bytes > DESCRIPTION_MAX_BYTES:
        return f"description too long ({desc_bytes} bytes; max {DESCRIPTION_MAX_BYTES})"
    if BANNED_REGEX.search(desc):
        return "description contains banned phrase"
    allowed = set(varbind_names) | set(STANDARD_PLACEHOLDERS)
    for ph in extract_placeholders(desc):
        if ph not in allowed:
            return f"unknown placeholder {{{ph}}} (not in varbinds {sorted(varbind_names)[:6]}... or standard set)"
    return None


def validate(
    response_text: str,
    varbind_names: List[str],
) -> Tuple[Optional[Dict[str, Any]], Optional[str]]:
    """Parse and validate model JSON.

    Returns ``(record, None)`` on success or ``(None, reason)`` on failure.
    """
    if not response_text:
        return None, "empty response"
    obj, reason = parse_response_object(normalize_response_text(response_text))
    if reason is not None:
        return None, reason
    reason = validate_response_keys(obj)
    if reason is None:
        reason = validate_response_values(obj, varbind_names)
    return (obj, None) if reason is None else (None, reason)


# --------------------------------------------------------------------------
# LLM client
# --------------------------------------------------------------------------


@dataclass
class LLMConfig:
    base_url: str
    model: str
    concurrency: int
    timeout_s: float = 300.0   # generous; some reasoning-style models can take 30-60s/request
    http_retries: int = 2      # retry on HTTP 5xx / 429 / network errors
    max_tokens: int = 800      # bump higher for reasoning models (qwen, deepseek-r1, ...)
    # Extra JSON fields merged into the chat-completion payload (e.g.
    # ``{"chat_template_kwargs": {"enable_thinking": false}}`` to disable
    # Qwen's reasoning chain).
    extra_body: Dict[str, Any] = None  # type: ignore[assignment]


async def call_llm(
    client: httpx.AsyncClient,
    cfg: LLMConfig,
    user_prompt: str,
) -> str:
    """Make a single LLM call.

    This helper intentionally does not inject authentication headers. Point it
    at an OpenAI-compatible endpoint or gateway whose auth policy is handled
    outside committed defaults.

    Returns the model's text response, or raises on persistent HTTP failure.
    """
    payload = {
        "model": cfg.model,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": user_prompt},
        ],
        "temperature": 0.0,
        "max_tokens": cfg.max_tokens,
    }
    if cfg.extra_body:
        payload.update(cfg.extra_body)
    last_exc: Optional[Exception] = None
    for attempt in range(cfg.http_retries + 1):
        try:
            r = await client.post(
                f"{cfg.base_url}/chat/completions",
                json=payload,
                timeout=cfg.timeout_s,
            )
            if r.status_code >= 500 or r.status_code == 429:
                raise RuntimeError(f"HTTP {r.status_code}: {r.text[:200]}")
            r.raise_for_status()
            data = r.json()
            choices = data.get("choices") or []
            if not choices:
                raise RuntimeError("no choices in response")
            content = choices[0].get("message", {}).get("content")
            if content is None or not isinstance(content, str):
                # Treat empty/null content as a retryable failure.
                raise RuntimeError(f"empty or non-string content (type={type(content).__name__})")
            return content
        except Exception as exc:
            last_exc = exc
            if attempt >= cfg.http_retries:
                raise
            backoff = min(8.0, 0.5 * (2 ** attempt)) + BACKOFF_RANDOM.uniform(0, 0.4)
            await asyncio.sleep(backoff)
    assert last_exc is not None
    raise last_exc


# --------------------------------------------------------------------------
# Per-OID workflow
# --------------------------------------------------------------------------


def oid_to_filename(oid: str) -> str:
    return oid.replace(".", "_") + ".json"


def deterministic_sample_key(seed: int, value: str) -> str:
    return hashlib.sha256(f"{seed}:{value}".encode("utf-8")).hexdigest()


def atomic_write(path: str, content: str) -> None:
    d = os.path.dirname(path)
    os.makedirs(d, exist_ok=True)
    fd, tmp = tempfile.mkstemp(prefix=".tmp.", dir=d)
    try:
        with os.fdopen(fd, "w") as f:
            f.write(content)
        os.replace(tmp, path)
    finally:
        if os.path.exists(tmp):
            try:
                os.remove(tmp)
            except OSError as exc:
                logging.debug("failed to remove temporary file %s: %s", tmp, exc)


def write_json(path: str, obj: Any) -> None:
    with open(path, "w") as f:
        json.dump(obj, f, indent=2)


def read_json_list(path: str) -> List[Dict[str, Any]]:
    if not os.path.exists(path):
        return []
    try:
        with open(path) as f:
            data = json.load(f)
    except Exception as exc:
        logging.debug("ignoring unreadable JSON list %s: %s", path, exc)
        return []
    if not isinstance(data, list):
        logging.debug("ignoring non-list JSON content in %s", path)
        return []
    return data


def read_trap_records(jsonl_path: str) -> List[Dict[str, Any]]:
    traps: List[Dict[str, Any]] = []
    with open(jsonl_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            rec = None
            try:
                rec = json.loads(line)
            except Exception as exc:
                logging.debug("skipping malformed extracted trap record: %s", exc)
            if rec is not None and rec.get("oid"):
                traps.append(rec)
    return traps


def known_enriched_filenames(out_enriched_dir: str) -> set:
    if not os.path.isdir(out_enriched_dir):
        return set()
    return {
        fn
        for fn in os.listdir(out_enriched_dir)
        if fn.endswith(".json")
    }


def sample_traps(traps: List[Dict[str, Any]], sample_size: int, sample_seed: int) -> List[Dict[str, Any]]:
    if not sample_size or sample_size >= len(traps):
        return traps

    by_mib: Dict[str, List[Dict[str, Any]]] = {}
    for trap in traps:
        by_mib.setdefault(trap.get("mib") or "", []).append(trap)
    keys = sorted(by_mib.keys(), key=lambda k: deterministic_sample_key(sample_seed, k))
    for key in keys:
        by_mib[key].sort(
            key=lambda trap: deterministic_sample_key(
                sample_seed,
                f"{trap.get('oid') or ''}:{trap.get('name') or ''}",
            )
        )

    flat = []
    i = 0
    while len(flat) < sample_size and any(by_mib[key] for key in keys):
        key = keys[i % len(keys)]
        if by_mib[key]:
            flat.append(by_mib[key].pop())
        i += 1
    return flat


async def enrich_one(
    trap: Dict[str, Any],
    cfg: LLMConfig,
    client: httpx.AsyncClient,
    sem: asyncio.Semaphore,
) -> Tuple[Dict[str, Any], Optional[Dict[str, Any]]]:
    """Enrich one trap with schema validation and retry feedback.

    Uses up to three attempts, falling back to mechanical defaults if all fail.

    Returns ``(record, failure_log_entry_or_None)``.
    """
    # Validator only accepts placeholders that reference varbinds that
    # ``emit.py`` will admit to the file-level table (oid AND syntax).
    # Mirrors the filter in build_user_prompt() so the LLM, the validator,
    # and the emitter all agree on what's legally referenceable.
    varbind_names = [
        vb.get("name")
        for vb in (trap.get("varbinds") or [])
        if (
            isinstance(vb, dict)
            and vb.get("name")
            and vb.get("oid")
            and vb.get("syntax")
        )
    ]
    attempts_log: List[Dict[str, Any]] = []
    chosen: Optional[Dict[str, Any]] = None
    source: Optional[str] = None
    last_failure_reason: Optional[str] = None
    last_response_text: str = ""

    async with sem:
        for attempt_idx in range(MAX_LLM_ATTEMPTS):
            if attempt_idx == 0:
                prompt = build_user_prompt(trap)
            else:
                prompt = build_retry_prompt(trap, last_failure_reason or "unknown", last_response_text)
            try:
                resp = await call_llm(client, cfg, prompt)
            except Exception as exc:
                attempts_log.append({"attempt": attempt_idx + 1, "kind": "http_failure", "reason": f"http: {exc}"})
                last_failure_reason = f"http: {exc}"
                last_response_text = ""
                continue
            # Defensive: even if call_llm somehow returns None, we record and retry.
            if not isinstance(resp, str):
                attempts_log.append({
                    "attempt": attempt_idx + 1,
                    "kind": "empty_response",
                    "reason": f"got {type(resp).__name__}",
                })
                last_failure_reason = f"empty response (got {type(resp).__name__})"
                last_response_text = ""
                continue
            last_response_text = resp
            ok, reason = validate(resp, varbind_names)
            if ok is not None:
                chosen = ok
                source = f"llm:{cfg.model}:attempt-{attempt_idx + 1}"
                attempts_log.append({"attempt": attempt_idx + 1, "kind": "ok"})
                break
            attempts_log.append({
                "attempt": attempt_idx + 1,
                "kind": "validation_failure",
                "reason": reason,
                "response_preview": resp[:300],
            })
            last_failure_reason = reason

    if chosen is None:
        chosen = mechanical_fallback(trap)
        source = "fallback:mechanical"

    priority = SEVERITY_TO_PRIORITY.get(chosen["severity"])
    record = {
        "oid": trap.get("oid"),
        "name": trap.get("name"),
        "mib": trap.get("mib"),
        "category": chosen["category"],
        "severity": chosen["severity"],
        "priority": priority,
        "description": chosen["description"],
        "varbinds": trap.get("varbinds") or [],
        "trap_description": trap.get("trap_description"),
        "trap_reference": trap.get("trap_reference"),
        "trap_status": trap.get("trap_status"),
        "mib_description": trap.get("mib_description"),
        "enrichment_source": source,
        "enrichment_attempts": attempts_log,
    }
    failure_log = None
    if source == "fallback:mechanical":
        failure_log = {
            "oid": trap.get("oid"),
            "name": trap.get("name"),
            "mib": trap.get("mib"),
            "attempts": attempts_log,
        }
    return record, failure_log


@dataclass
class EnrichmentRunStats:
    total: int
    progress_every: int
    failures: List[Dict[str, Any]]
    n_done: int = 0
    n_via_mech: int = 0
    t0: float = 0.0
    n_via_llm: List[int] = field(default_factory=lambda: [0, 0, 0])
    n_by_endpoint: Dict[str, int] = field(default_factory=dict)

    def report(self) -> Dict[str, Any]:
        return {
            "total": self.total,
            "done": self.n_done,
            "via_llm_attempt_1": self.n_via_llm[0],
            "via_llm_attempt_2": self.n_via_llm[1],
            "via_llm_attempt_3": self.n_via_llm[2],
            "via_mechanical_fallback": self.n_via_mech,
            "by_endpoint": self.n_by_endpoint,
            "elapsed_seconds": time.time() - self.t0,
        }


def record_enrichment_result(
    record: Dict[str, Any],
    failure_log: Optional[Dict[str, Any]],
    model: str,
    stats: EnrichmentRunStats,
) -> None:
    if failure_log is not None:
        stats.failures.append(failure_log)
    src = record.get("enrichment_source") or ""
    if src.startswith("llm:"):
        m = re.search(r"attempt-(\d+)$", src)
        if m:
            attempt = int(m.group(1)) - 1
            if 0 <= attempt < 3:
                stats.n_via_llm[attempt] += 1
        stats.n_by_endpoint[model] = stats.n_by_endpoint.get(model, 0) + 1
    else:
        stats.n_via_mech += 1
    stats.n_done += 1


def log_enrichment_progress(stats: EnrichmentRunStats) -> None:
    if not stats.progress_every or stats.n_done % stats.progress_every != 0:
        return
    dt = time.time() - stats.t0
    rate = stats.n_done / dt if dt > 0 else 0
    eta = (stats.total - stats.n_done) / rate if rate > 0 else 0
    by_ep = " ".join(f"{m}={c}" for m, c in stats.n_by_endpoint.items())
    logging.info(
        "progress %d/%d (a1=%d a2=%d a3=%d mech=%d) rate=%.1f/s eta=%.0fs %s",
        stats.n_done, stats.total,
        stats.n_via_llm[0], stats.n_via_llm[1], stats.n_via_llm[2], stats.n_via_mech,
        rate, eta, by_ep,
    )


async def enrich_worker(
    queue: asyncio.Queue,
    out_enriched: str,
    ep: LLMConfig,
    client: httpx.AsyncClient,
    ep_sem: asyncio.Semaphore,
    stats: EnrichmentRunStats,
) -> None:
    while True:
        try:
            trap = queue.get_nowait()
        except asyncio.QueueEmpty:
            return
        try:
            record, failure_log = await enrich_one(trap, ep, client, ep_sem)
            oid = record["oid"]
            if oid:
                path = os.path.join(out_enriched, oid_to_filename(oid))
                await asyncio.to_thread(
                    atomic_write,
                    path,
                    json.dumps(record, separators=(",", ":"), ensure_ascii=False) + "\n",
                )
                record_enrichment_result(record, failure_log, ep.model, stats)
                log_enrichment_progress(stats)
        except Exception as exc:
            logging.warning("worker(%s) error on trap %s: %s", ep.model, trap.get("oid"), exc)
        finally:
            queue.task_done()


async def run_async(
    traps: List[Dict[str, Any]],
    out_dir: str,
    endpoints: List[LLMConfig],
    progress_every: int,
) -> Dict[str, Any]:
    """Dispatch traps across one-or-more LLM endpoints.

    Each endpoint runs `endpoint.concurrency` worker tasks. Workers pull
    from a shared queue, so faster endpoints naturally absorb more work.
    """
    out_enriched = os.path.join(out_dir, "enriched")
    os.makedirs(out_enriched, exist_ok=True)

    failures_path = os.path.join(out_dir, "llm-failures.json")
    failures = await asyncio.to_thread(read_json_list, failures_path)
    stats = EnrichmentRunStats(
        total=len(traps),
        progress_every=progress_every,
        failures=failures,
        t0=time.time(),
        n_by_endpoint={ep.model: 0 for ep in endpoints},
    )

    queue: asyncio.Queue = asyncio.Queue()
    for t in traps:
        queue.put_nowait(t)

    # One httpx.AsyncClient per endpoint (independent connection pool +
    # keep-alives sized to the endpoint's concurrency).
    async with contextlib.AsyncExitStack() as stack:
        ep_workers: List[asyncio.Task] = []
        for ep in endpoints:
            limits = httpx.Limits(max_connections=ep.concurrency + 8,
                                  max_keepalive_connections=ep.concurrency + 8)
            client = await stack.enter_async_context(httpx.AsyncClient(limits=limits, timeout=ep.timeout_s))
            ep_sem = asyncio.Semaphore(ep.concurrency)
            for _ in range(ep.concurrency):
                ep_workers.append(asyncio.create_task(enrich_worker(queue, out_enriched, ep, client, ep_sem, stats)))
        await asyncio.gather(*ep_workers, return_exceptions=False)

    await asyncio.to_thread(write_json, failures_path, failures)
    return stats.report()


# --------------------------------------------------------------------------
# Trap loading
# --------------------------------------------------------------------------


def load_traps(
    jsonl_path: str,
    out_enriched_dir: str,
    only_oids: Optional[List[str]] = None,
    sample_size: int = 0,
    sample_seed: int = 7,
    force: bool = False,
) -> List[Dict[str, Any]]:
    traps = read_trap_records(jsonl_path)
    if only_oids:
        wanted = set(only_oids)
        traps = [t for t in traps if t.get("oid") in wanted]
    if not force:
        present = known_enriched_filenames(out_enriched_dir)
        traps = [t for t in traps if oid_to_filename(t["oid"]) not in present]
    return sample_traps(traps, sample_size, sample_seed)


def main() -> int:
    parser = argparse.ArgumentParser(description="LLM-enrich extracted SNMP trap records.")
    parser.add_argument("--in-jsonl", default="output/extracted.jsonl")
    parser.add_argument("--out-dir", default="output")
    parser.add_argument("--sample", type=int, default=0,
                        help="Process a stratified random sample of N traps then stop.")
    parser.add_argument("--sample-seed", type=int, default=7)
    parser.add_argument("--oids", nargs="*", default=None,
                        help="Restrict to these OIDs (overrides --sample).")
    parser.add_argument("--concurrency", type=int, default=DEFAULT_CONCURRENCY,
                        help=f"Single-endpoint slots when --endpoint is not used (default {DEFAULT_CONCURRENCY}).")
    parser.add_argument("--model", default=MODEL)
    parser.add_argument("--base-url", default=LLM_BASE_URL)
    parser.add_argument("--max-tokens", type=int, default=800,
                        help="LLM max_tokens for single-endpoint mode (bump for reasoning models).")
    parser.add_argument("--endpoint", action="append", default=None,
                        help=("Add an LLM endpoint to the dispatch pool. Repeatable. "
                              "Format: base_url|model|concurrency|max_tokens "
                              "e.g. 'http://127.0.0.1:8000/v1|local-model|8|800'. "
                              "When --endpoint is given, --base-url/--model/--concurrency are ignored."))
    parser.add_argument("--force", action="store_true",
                        help="Re-process even OIDs that already have output files.")
    parser.add_argument("--log-level", default="INFO")
    parser.add_argument("--progress-every", type=int, default=20)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=getattr(logging, args.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(message)s",
    )

    endpoints: List[LLMConfig] = []
    if args.endpoint:
        for spec in args.endpoint:
            parts = spec.split("|")
            if len(parts) not in (3, 4):
                logging.error("invalid --endpoint spec %r; expected base_url|model|concurrency[|max_tokens]", spec)
                return 2
            base_url, model, conc_s = parts[0], parts[1], parts[2]
            max_tokens = int(parts[3]) if len(parts) == 4 else 800
            # Auto-disable reasoning chains on Qwen "thinker" variants —
            # the thinker chain blows wall-clock per request and we don't
            # need reasoning for this classification workload (small
            # 3-field JSON output, deterministic).
            extra_body: Optional[Dict[str, Any]] = None
            if "qwen" in model.lower() and "nothinker" not in model.lower():
                extra_body = {"chat_template_kwargs": {"enable_thinking": False}}
            endpoints.append(LLMConfig(
                base_url=base_url,
                model=model,
                concurrency=int(conc_s),
                max_tokens=max_tokens,
                extra_body=extra_body,
            ))
    else:
        endpoints.append(LLMConfig(
            base_url=args.base_url,
            model=args.model,
            concurrency=args.concurrency,
            max_tokens=args.max_tokens,
        ))

    out_enriched = os.path.join(args.out_dir, "enriched")
    traps = load_traps(
        jsonl_path=args.in_jsonl,
        out_enriched_dir=out_enriched,
        only_oids=args.oids,
        sample_size=args.sample,
        sample_seed=args.sample_seed,
        force=args.force,
    )
    logging.info("loaded %d traps to process (from %s)", len(traps), args.in_jsonl)
    for ep in endpoints:
        logging.info("endpoint: %s model=%s concurrency=%d max_tokens=%d",
                     ep.base_url, ep.model, ep.concurrency, ep.max_tokens)
    total_slots = sum(ep.concurrency for ep in endpoints)
    logging.info("total in-flight slots across %d endpoints: %d", len(endpoints), total_slots)
    if args.dry_run:
        return 0
    if not traps:
        logging.info("nothing to do")
        return 0

    report = asyncio.run(run_async(
        traps=traps,
        out_dir=args.out_dir,
        endpoints=endpoints,
        progress_every=args.progress_every,
    ))
    report_path = os.path.join(args.out_dir, "enrichment-report.json")
    with open(report_path, "w") as f:
        json.dump(report, f, indent=2)
    print(json.dumps(report, indent=2))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        print("interrupted", file=sys.stderr)
        sys.exit(130)
    except Exception:
        traceback.print_exc()
        sys.exit(1)
