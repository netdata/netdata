import asyncio

import pytest

from netdata_mcp import bearer

SENTINEL = "sk-SENTINEL-cloud-token-zzz"


def setup_function(_fn):
    bearer._CACHE.clear()


@pytest.fixture(autouse=True)
def _clear_cloud_env(monkeypatch):
    # The suite runs in dev shells that may export these (the tool documents
    # NETDATA_CLOUD_TOKEN); clear them so cloud_token()/env defaults are tested
    # against a known-empty environment, not the developer's shell.
    monkeypatch.delenv("NETDATA_CLOUD_TOKEN", raising=False)
    monkeypatch.delenv("NETDATA_CLOUD_HOSTNAME", raising=False)


# ── env reading ───────────────────────────────────────────────────────────────
def test_cloud_token_reads_and_trims():
    assert bearer.cloud_token({"NETDATA_CLOUD_TOKEN": "  tok  "}) == "tok"
    assert bearer.cloud_token({}) == ""


def test_cloud_hostname_defaults():
    assert bearer.cloud_hostname({}) == bearer.DEFAULT_CLOUD_HOSTNAME
    assert bearer.cloud_hostname({"NETDATA_CLOUD_HOSTNAME": "eu.netdata.cloud"}) == "eu.netdata.cloud"


# ── expiry heuristic ───────────────────────────────────────────────────────────
def test_usable_until_seconds_applies_refresh_buffer():
    assert bearer._usable_until(10_000, now=0) == 10_000 - bearer._REFRESH_BUFFER_S


def test_usable_until_milliseconds_normalized():
    # 10_000_000_000_000 ms == 10_000_000_000 s
    assert bearer._usable_until(10_000_000_000_000, now=0) == 10_000_000_000 - bearer._REFRESH_BUFFER_S


def test_usable_until_zero_or_invalid_uses_fallback_window():
    assert bearer._usable_until(0, now=100) == 100 + bearer._FALLBACK_TTL_S
    assert bearer._usable_until("nope", now=100) == 100 + bearer._FALLBACK_TTL_S


def test_usable_until_short_ttl_floors_above_now_not_below():
    # A token expiring sooner than the refresh buffer must still cache briefly
    # (>= now) rather than landing in the past and re-minting every call — but
    # never past the actual expiry.
    now = 1_000_000.0
    horizon = bearer._usable_until(int(now) + 120, now=now)  # 2-min TTL
    assert now < horizon <= now + 120


# ── identity extraction ────────────────────────────────────────────────────────
def test_extract_identity_ok():
    data = {"agents": [{"mg": "MG", "nd": "ND", "cloud": {"claim_id": "CL"}}]}
    ident, err = bearer._extract_identity(data)
    assert err is None and ident == ("MG", "ND", "CL")


def test_extract_identity_unclaimed_is_incomplete():
    data = {"agents": [{"mg": "MG", "nd": "ND", "cloud": {}}]}
    ident, err = bearer._extract_identity(data)
    assert ident is None and "incomplete" in err


def test_extract_identity_no_agents():
    ident, err = bearer._extract_identity({"agents": []})
    assert ident is None and "agents[0]" in err


# ── resolve flow (with monkeypatched IO) ───────────────────────────────────────
def test_resolve_sync_caches_after_mint(monkeypatch):
    calls = {"mint": 0}
    monkeypatch.setattr(bearer, "_get_identity", lambda port, t: (("MG", "ND", "CL"), None))

    def fake_mint(nd, mg, claim, token, host, t):
        calls["mint"] += 1
        return {"token": "BEARER-1", "expiration": 0}, None

    monkeypatch.setattr(bearer, "_mint", fake_mint)

    tok, err = bearer._resolve_sync(123, "ctok", "app.netdata.cloud", 30, now=1000.0)
    assert tok == "BEARER-1" and err is None and calls["mint"] == 1
    # second call within the window is served from cache (no second mint)
    tok2, err2 = bearer._resolve_sync(123, "ctok", "app.netdata.cloud", 30, now=2000.0)
    assert tok2 == "BEARER-1" and err2 is None and calls["mint"] == 1


def test_resolve_sync_remints_when_expired(monkeypatch):
    monkeypatch.setattr(bearer, "_get_identity", lambda port, t: (("MG", "ND", "CL"), None))
    seq = iter([{"token": "B1", "expiration": 0}, {"token": "B2", "expiration": 0}])
    monkeypatch.setattr(bearer, "_mint", lambda *a: (next(seq), None))

    t1, _ = bearer._resolve_sync(1, "c", "h", 30, now=0.0)
    # jump past the fallback window → must re-mint
    t2, _ = bearer._resolve_sync(1, "c", "h", 30, now=bearer._FALLBACK_TTL_S + 1.0)
    assert t1 == "B1" and t2 == "B2"


def test_resolve_sync_remints_on_new_claim_id(monkeypatch):
    # Same machine_guid, rotated claim_id (re-claim) must miss the cache and
    # re-mint rather than serve the bearer minted under the old claim.
    ids = iter([("MG", "ND", "CL-old"), ("MG", "ND", "CL-new")])
    monkeypatch.setattr(bearer, "_get_identity", lambda port, t: (next(ids), None))
    seq = iter([{"token": "B-old", "expiration": 0}, {"token": "B-new", "expiration": 0}])
    monkeypatch.setattr(bearer, "_mint", lambda *a: (next(seq), None))

    t1, _ = bearer._resolve_sync(1, "c", "h", 30, now=0.0)
    t2, _ = bearer._resolve_sync(1, "c", "h", 30, now=1.0)  # within window, but new claim_id
    assert t1 == "B-old" and t2 == "B-new"


def test_resolve_sync_propagates_identity_error(monkeypatch):
    monkeypatch.setattr(bearer, "_get_identity", lambda port, t: (None, "not claimed"))
    tok, err = bearer._resolve_sync(1, "c", "h", 30, now=0.0)
    assert tok is None and err == "not claimed"


def test_resolve_bearer_without_cloud_token_errs():
    tok, err = asyncio.run(bearer.resolve_bearer(1, environ={}))
    assert tok is None and "NETDATA_CLOUD_TOKEN" in err


# ── token-safety contract ──────────────────────────────────────────────────────
def test_scrub_masks_secrets():
    out = bearer._scrub(f"oops {SENTINEL} and BEARER-xyz", SENTINEL, "BEARER-xyz")
    assert SENTINEL not in out and "BEARER-xyz" not in out


def test_scrub_handles_overlapping_secrets():
    # Longest-first ordering: the longer secret must not be left half-masked when
    # a shorter one is its prefix.
    out = bearer._scrub("leak abcxyz here", "abc", "abcxyz")
    assert "abcxyz" not in out and "abc" not in out and "xyz" not in out


def test_mint_sends_descriptive_user_agent(monkeypatch):
    # Cloud is behind Cloudflare, which 1010-bans the default urllib UA. The
    # mint MUST send a non-urllib User-Agent (regression guard).
    captured = {}

    class _Resp:
        def read(self):
            return b'{"token": "B", "expiration": 0}'

        def __enter__(self):
            return self

        def __exit__(self, *a):
            return False

    def fake_urlopen(req, timeout=None):
        captured["ua"] = req.get_header("User-agent")
        return _Resp()

    monkeypatch.setattr(bearer.urllib.request, "urlopen", fake_urlopen)
    parsed, err = bearer._mint("ND", "MG", "CL", "ctok", "app.netdata.cloud", 30)
    assert err is None and parsed["token"] == "B"
    assert captured["ua"] == bearer._USER_AGENT
    assert "urllib" not in (captured["ua"] or "").lower()


def test_mint_error_never_leaks_cloud_token(monkeypatch):
    # urlopen raising with the token in its message must come back scrubbed.
    def boom(*a, **k):
        raise RuntimeError(f"connection to host failed with creds {SENTINEL}")

    monkeypatch.setattr(bearer.urllib.request, "urlopen", boom)
    parsed, err = bearer._mint("ND", "MG", "CL", SENTINEL, "app.netdata.cloud", 30)
    assert parsed is None
    assert SENTINEL not in err and "<REDACTED>" in err
