"""Mint and cache Netdata Cloud per-agent bearer tokens, for calling
access-gated agent functions (e.g. ``otel-logs``, which requires
``SIGNED_ID | SAME_SPACE | SENSITIVE_DATA``). Transport-free: no MCP-server
imports.

A local agent grants an anonymous caller only ``HTTP_ACCESS_ANONYMOUS_DATA``,
so gated functions return HTTP 412. To satisfy ``SIGNED_ID`` we present an
``Authorization: Bearer <token>`` minted by Netdata Cloud for the (claimed)
agent — the same flow as the ``query-netdata-agents`` skill:

  1. read the agent's identity from ``GET /api/v3/info`` (``agents[0].mg`` =
     machine_guid, ``.nd`` = node_id, ``.cloud.claim_id``);
  2. ``GET https://<hostname>/api/v2/bearer_get_token?node_id&machine_guid&claim_id``
     with the *Cloud* token in the Authorization header;
  3. cache the returned bearer in-process until shortly before it expires.

Token-safety contract (HARD requirement):
  * ``NETDATA_CLOUD_TOKEN`` and every minted bearer are secrets. This module
    NEVER logs them, NEVER returns them in user/tool output, and NEVER places
    them on a command line — they travel only in an Authorization header and
    the in-process cache.
  * Error strings are scrubbed of both secrets before being returned.
  * Callers receive a bearer only to hand to ``agentfn.call_function``; they
    MUST NOT echo it into a tool result.
"""

from __future__ import annotations

import asyncio
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Mapping
from dataclasses import dataclass

CLOUD_TOKEN_ENV = "NETDATA_CLOUD_TOKEN"
CLOUD_HOSTNAME_ENV = "NETDATA_CLOUD_HOSTNAME"
DEFAULT_CLOUD_HOSTNAME = "app.netdata.cloud"

# Cloud (app.netdata.cloud) sits behind Cloudflare, whose managed WAF rules
# reject the default urllib User-Agent with "error code: 1010" (banned browser
# signature) before auth is even checked. Send a plain descriptive UA — verified
# to pass — so the mint reaches the API.
_USER_AGENT = "netdata-build-mcp/1.0"

# Re-mint when within this many seconds of a real expiry (matches the cloud
# frontend's 1h buffer). When Cloud returns expiration=0, fall back to a fixed
# window from the mint time — the agent issues ~3h bearers, so 2h leaves margin.
_REFRESH_BUFFER_S = 3600
_FALLBACK_TTL_S = 7200
# Floor so a short-lived token (real expiry < the refresh buffer) still caches
# briefly instead of re-minting on every call; never cached past actual expiry.
_MIN_CACHE_S = 60

# Loopback-only opener for the agent /api/v3/info probe: never via a proxy.
_LOCAL_OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def cloud_token(environ: Mapping[str, str] | None = None) -> str:
    """The Cloud REST token from the env, or ``""`` when unset/blank."""
    src = os.environ if environ is None else environ
    return (src.get(CLOUD_TOKEN_ENV) or "").strip()


def cloud_hostname(environ: Mapping[str, str] | None = None) -> str:
    """The Cloud REST host, defaulting to ``app.netdata.cloud``."""
    src = os.environ if environ is None else environ
    return (src.get(CLOUD_HOSTNAME_ENV) or "").strip() or DEFAULT_CLOUD_HOSTNAME


@dataclass
class _Cached:
    token: str
    usable_until: float  # unix seconds; cache is valid while now < usable_until


# In-memory cache, no secrets on disk. Keyed by (machine_guid, claim_id) so a
# re-claim (new claim_id, same machine_guid) misses and re-mints rather than
# serving a bearer minted under the old claim. A mint lock serializes concurrent
# first-calls so we mint once.
_CACHE: dict[tuple[str, str], _Cached] = {}
_LOCK = asyncio.Lock()


def _scrub(text: str, *secrets: str) -> str:
    """Mask any secret substring so it cannot leak through an error string."""
    out = text
    # Longest-first so a secret that contains another isn't left half-masked.
    for s in sorted((s for s in secrets if s), key=len, reverse=True):
        out = out.replace(s, "<REDACTED>")
    return out


def _usable_until(expiration: object, now: float) -> float:
    """Effective cache horizon from Cloud's ``expiration`` (s or ms, or 0)."""
    try:
        exp = int(expiration)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        exp = 0
    if exp <= 0:
        return now + _FALLBACK_TTL_S
    if exp > 1_000_000_000_000:  # milliseconds
        exp //= 1000
    # Refresh an hour before expiry, but cache at least _MIN_CACHE_S so a
    # short-lived token doesn't re-mint on every call — and never past expiry.
    return min(float(exp), max(exp - _REFRESH_BUFFER_S, now + _MIN_CACHE_S))


def _extract_identity(data: object) -> tuple[tuple[str, str, str] | None, str | None]:
    """Pull ``(machine_guid, node_id, claim_id)`` from a parsed /api/v3/info
    object (``agents[0]``: ``mg``/``nd``/``cloud.claim_id``), or ``(None, error)``.
    claim_id is present only once the agent is claimed."""
    agents = data.get("agents") if isinstance(data, dict) else None
    agent = agents[0] if isinstance(agents, list) and agents else None
    if not isinstance(agent, dict):
        return None, "no agents[0] in /api/v3/info"
    cloud = agent.get("cloud")
    mg = agent.get("mg")
    nd = agent.get("nd")
    claim = cloud.get("claim_id") if isinstance(cloud, dict) else None
    if not (mg and nd and claim):
        return None, ("agent identity incomplete (mg/nd/claim_id) — is it claimed "
                      "and cloud_connected? check netdata_run_status")
    return (str(mg), str(nd), str(claim)), None


def _get_identity(port: int, timeout: int) -> tuple[tuple[str, str, str] | None, str | None]:
    """``((machine_guid, node_id, claim_id), None)`` from the agent's
    /api/v3/info, or ``(None, error)``."""
    url = f"http://127.0.0.1:{port}/api/v3/info"
    try:
        with _LOCAL_OPENER.open(url, timeout=timeout) as resp:
            data = json.loads(resp.read())
    except Exception as exc:
        return None, f"could not read {url}: {exc!r}"
    return _extract_identity(data)


def _mint(node_id: str, mg: str, claim: str, token: str, hostname: str, timeout: int):
    """Mint a bearer via Cloud. Returns ``(parsed_json, None)`` or ``(None, error)``.
    The Cloud token rides in the Authorization header only — never the URL."""
    q = urllib.parse.urlencode({"node_id": node_id, "machine_guid": mg, "claim_id": claim})
    url = f"https://{hostname}/api/v2/bearer_get_token?{q}"
    req = urllib.request.Request(
        url, method="GET",
        headers={"Authorization": f"Bearer {token}", "User-Agent": _USER_AGENT},
    )
    try:
        # Cloud is off-host: use the default opener (proxy honored if configured).
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
    except urllib.error.HTTPError as exc:
        try:  # reading the error body can itself raise (IncompleteRead/OSError)
            body = exc.read()[:200].decode("utf-8", "replace")
        except Exception:
            body = "<error body unavailable>"
        return None, _scrub(f"Cloud mint HTTP {exc.code}: {body}", token)
    except Exception as exc:
        return None, _scrub(f"Cloud mint request failed: {exc!r}", token)
    try:
        parsed = json.loads(raw)
    except json.JSONDecodeError:
        return None, "Cloud mint returned non-JSON"
    if not isinstance(parsed, dict) or not parsed.get("token"):
        return None, "Cloud mint response had no token"
    return parsed, None


def _resolve_sync(port: int, token: str, hostname: str, timeout: int, now: float):
    identity, err = _get_identity(port, timeout)
    if identity is None:
        return None, err
    mg, nd, claim = identity
    key = (mg, claim)

    cached = _CACHE.get(key)
    if cached is not None and now < cached.usable_until:
        return cached.token, None

    parsed, err = _mint(nd, mg, claim, token, hostname, timeout)
    if parsed is None:
        return None, err
    minted = str(parsed["token"])
    _CACHE[key] = _Cached(token=minted, usable_until=_usable_until(parsed.get("expiration"), now))
    return minted, None


async def resolve_bearer(
    port: int, *, timeout: int = 30, now: float | None = None,
    environ: Mapping[str, str] | None = None,
) -> tuple[str | None, str | None]:
    """Resolve a Cloud bearer for the agent on ``port``: ``(token, None)`` on
    success, ``(None, error)`` otherwise. Reads ``NETDATA_CLOUD_TOKEN`` itself;
    returns a clear error when it is unset (the caller decides whether to fall
    back to an anonymous call). Never raises."""
    token = cloud_token(environ)
    if not token:
        return None, f"{CLOUD_TOKEN_ENV} is not set"
    host = cloud_hostname(environ)
    when = float(now) if now is not None else time.time()
    async with _LOCK:
        return await asyncio.to_thread(_resolve_sync, port, token, host, timeout, when)
