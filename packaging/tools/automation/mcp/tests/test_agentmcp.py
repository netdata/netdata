from mcp.shared.exceptions import McpError
from mcp.types import ErrorData

from netdata_mcp import agentmcp


class _Block:
    def __init__(self, text):
        self.text = text


class _Result:
    def __init__(self, content, is_error=False):
        self.content = content
        self.isError = is_error


class _Session:
    def __init__(self, result):
        self._result = result
        self.calls = []

    async def call_tool(self, tool, arguments):
        self.calls.append((tool, arguments))
        return self._result


def test_content_text_joins_text_blocks():
    assert agentmcp._content_text(_Result([_Block("a"), _Block("b")])) == "a\nb"
    assert agentmcp._content_text(_Result([])) == ""
    assert agentmcp._content_text(_Result(None)) == ""


async def test_call_forwards_and_returns_text():
    s = _Session(_Result([_Block('{"data": 1}')]))
    out = await agentmcp._call(s, "query_metrics", {"metric": "system.cpu"})
    assert out == '{"data": 1}'
    assert s.calls == [("query_metrics", {"metric": "system.cpu"})]


async def test_call_forwards_error_content_verbatim():
    # the agent returns tool-execution errors as content; we forward the text either way
    s = _Session(_Result([_Block("FUNCTION EXECUTION FAILED: bad param")], is_error=True))
    out = await agentmcp._call(s, "execute_function", {"function": "systemd-journal"})
    assert "FUNCTION EXECUTION FAILED" in out


class _DummyClient:
    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        return False


class _DummyStreamable:
    async def __aenter__(self):
        return (None, None, None)  # (read, write, get_session_id)

    async def __aexit__(self, *exc):
        return False


def _mcp_error_session(message):
    """A fake ClientSession whose call_tool raises a protocol-level McpError, so
    call_agent_tool's `except McpError` branch is exercised the way production hits it."""

    class _S:
        def __init__(self, *args):
            pass

        async def __aenter__(self):
            return self

        async def __aexit__(self, *exc):
            return False

        async def initialize(self):
            pass

        async def call_tool(self, tool, arguments):
            raise McpError(ErrorData(code=-32602, message=message))

    return _S


def _wire_session(monkeypatch, session_cls):
    monkeypatch.setattr(agentmcp, "create_mcp_http_client", lambda headers=None: _DummyClient())
    monkeypatch.setattr(agentmcp, "streamable_http_client", lambda url, http_client=None: _DummyStreamable())
    monkeypatch.setattr(agentmcp, "ClientSession", session_cls)


async def test_call_agent_tool_forwards_protocol_error_as_text(monkeypatch):
    # a protocol-level rejection surfaces as McpError from the /mcp call; forward it as text
    _wire_session(monkeypatch, _mcp_error_session("Missing required parameter 'nodes'."))
    out = await agentmcp.call_agent_tool("http://127.0.0.1:9", "list_functions", {}, bearer="tok-XYZ")
    assert out.startswith("[agent /mcp error]") and "Missing required parameter 'nodes'" in out


async def test_call_agent_tool_scrubs_bearer_from_protocol_error(monkeypatch):
    # the McpError path must honor the same HARD scrub contract as the transport path
    _wire_session(monkeypatch, _mcp_error_session("rejected; saw Bearer tok-XYZ"))
    out = await agentmcp.call_agent_tool("http://127.0.0.1:9", "list_functions", {}, bearer="tok-XYZ")
    assert out.startswith("[agent /mcp error]") and "tok-XYZ" not in out and "<REDACTED>" in out


async def test_call_agent_tool_attaches_bearer_via_sdk_factory(monkeypatch):
    # The bearer must reach the /mcp client as an Authorization header, and the
    # client must come from the SDK factory (so MCP timeout defaults are kept).
    captured = {}

    def fake_factory(headers=None):
        captured["headers"] = headers
        captured["client"] = _DummyClient()
        return captured["client"]

    def fake_streamable(url, http_client=None):
        captured["url"] = url
        captured["passed_client"] = http_client
        raise RuntimeError("stop after client wiring")

    monkeypatch.setattr(agentmcp, "create_mcp_http_client", fake_factory)
    monkeypatch.setattr(agentmcp, "streamable_http_client", fake_streamable)
    out = await agentmcp.call_agent_tool("http://127.0.0.1:9/", "list_functions", {}, bearer="tok-XYZ")
    assert captured["headers"] == {"Authorization": "Bearer tok-XYZ"}
    assert captured["url"] == "http://127.0.0.1:9/mcp"
    assert captured["passed_client"] is captured["client"]  # SDK client is the one used
    assert out.startswith("[could not reach agent /mcp")


async def test_call_agent_tool_scrubs_bearer_from_error(monkeypatch):
    # An exception whose text embeds the bearer must not leak it to tool output.
    monkeypatch.setattr(agentmcp, "create_mcp_http_client", lambda headers=None: _DummyClient())

    def fake_streamable(url, http_client=None):
        raise RuntimeError("transport blew up; header was Bearer tok-XYZ")

    monkeypatch.setattr(agentmcp, "streamable_http_client", fake_streamable)
    out = await agentmcp.call_agent_tool("http://127.0.0.1:9", "list_functions", {}, bearer="tok-XYZ")
    assert "tok-XYZ" not in out and "<REDACTED>" in out
