import asyncio
import io
import urllib.error

from netdata_mcp import agentfn
from netdata_mcp.tools.otel_logs import build_payload, classify_status


def _payload(**kw):
    base = dict(
        info=False, after=None, before=None, query=None, selections=None,
        facets=None, histogram=None, direction=None, last=None, anchor=None, tenant=None,
    )
    base.update(kw)
    return build_payload(**base)


def test_build_payload_omits_unset_fields():
    assert _payload() == {}  # nothing set → function applies its own defaults


def test_build_payload_info_flag():
    assert _payload(info=True) == {"info": True}


def test_build_payload_includes_only_set_fields():
    p = _payload(
        after=1000, before=2000, query="error",
        selections={"level": ["error", "warn"]}, facets=["level", "host"],
        histogram="level", direction="forward", last=50, tenant="t1",
    )
    assert p == {
        "after": 1000, "before": 2000, "query": "error",
        "histogram": "level", "direction": "forward", "last": 50, "tenant": "t1",
        "selections": {"level": ["error", "warn"]}, "facets": ["level", "host"],
    }


def test_build_payload_skips_empty_selections_and_facets():
    # empty containers are falsy → omitted (an empty selections must not mean
    # "match nothing"; absence lets the function default apply)
    assert _payload(selections={}, facets=[]) == {}


def test_function_url():
    url = agentfn.function_url("http://127.0.0.1:19999", "otel-logs", 60)
    assert url == "http://127.0.0.1:19999/api/v3/function?function=otel-logs&timeout=60"
    # trailing slash on base is handled
    assert agentfn.function_url("http://127.0.0.1:19999/", "otel-logs", 30).startswith(
        "http://127.0.0.1:19999/api/v3/function?"
    )


class _FakeResp:
    status = 200

    def read(self):
        return b'{"ok": true}'

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


def _capture_request(monkeypatch):
    """Patch the agentfn opener to capture the urllib.Request it sends."""
    captured = {}

    def fake_open(req, timeout=None):
        captured["headers"] = dict(req.header_items())
        return _FakeResp()

    monkeypatch.setattr(agentfn._LOCAL_OPENER, "open", fake_open)
    return captured


def test_call_function_sends_bearer_when_given(monkeypatch):
    cap = _capture_request(monkeypatch)
    status, data, err = asyncio.run(
        agentfn.call_function("http://127.0.0.1:1", "otel-logs", {"info": True}, bearer="BEARER-xyz")
    )
    assert status == 200 and err is None
    # urllib title-cases header keys
    assert cap["headers"].get("Authorization") == "Bearer BEARER-xyz"


def test_call_function_anonymous_has_no_authorization(monkeypatch):
    cap = _capture_request(monkeypatch)
    asyncio.run(agentfn.call_function("http://127.0.0.1:1", "otel-logs", {"info": True}))
    assert "Authorization" not in cap["headers"]


class _RaisingResp:
    status = 200

    def read(self):
        raise OSError("connection reset mid-body")

    def __enter__(self):
        return self

    def __exit__(self, *a):
        return False


def test_call_function_never_raises_on_read_error(monkeypatch):
    # A body read that raises must come back as an error tuple, not propagate
    # (the tool's "never raises" contract).
    monkeypatch.setattr(agentfn._LOCAL_OPENER, "open", lambda req, timeout=None: _RaisingResp())
    status, data, err = asyncio.run(agentfn.call_function("http://127.0.0.1:1", "otel-logs", {}))
    assert data is None and err is not None and "failed" in err


def test_call_function_scrubs_bearer_from_error(monkeypatch):
    def boom(req, timeout=None):
        raise RuntimeError("upstream said Bearer BEARER-SECRET-123 is bad")

    monkeypatch.setattr(agentfn._LOCAL_OPENER, "open", boom)
    _, _, err = asyncio.run(
        agentfn.call_function("http://127.0.0.1:1", "otel-logs", {}, bearer="BEARER-SECRET-123")
    )
    assert "BEARER-SECRET-123" not in err and "<REDACTED>" in err


def test_call_function_extracts_status_from_http_error(monkeypatch):
    # A 412 (the access gate) arrives as an HTTPError carrying a JSON body — the
    # core path for this tool. _post must extract the code and parse the body.
    def http_error(req, timeout=None):
        raise urllib.error.HTTPError(
            req.full_url, 412, "Precondition Failed", {}, io.BytesIO(b'{"status": 412}')
        )

    monkeypatch.setattr(agentfn._LOCAL_OPENER, "open", http_error)
    status, data, err = asyncio.run(agentfn.call_function("http://127.0.0.1:1", "otel-logs", {}))
    assert status == 412 and err is None and data == {"status": 412}


def test_call_function_http_error_empty_body(monkeypatch):
    def http_error(req, timeout=None):
        raise urllib.error.HTTPError(req.full_url, 500, "err", {}, io.BytesIO(b""))

    monkeypatch.setattr(agentfn._LOCAL_OPENER, "open", http_error)
    status, data, err = asyncio.run(agentfn.call_function("http://127.0.0.1:1", "otel-logs", {}))
    assert status == 500 and data is None and err is None


def test_classify_status():
    assert classify_status(200) == (None, "ok")
    assert classify_status(204) == (None, "ok")
    err412, msg412 = classify_status(412)
    assert err412 is not None and "SIGNED_ID" in err412 and msg412 == err412
    err500, _ = classify_status(500)
    assert err500 is not None and "500" in err500
    err_none, _ = classify_status(None)
    assert err_none is not None  # unknown status is an error, not success
