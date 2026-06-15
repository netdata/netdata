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


class _RaisingSession:
    async def call_tool(self, tool, arguments):
        raise McpError(ErrorData(code=-32602, message="Missing required parameter 'nodes'."))


async def test_call_forwards_protocol_error_without_raising():
    # a protocol-level rejection is raised as McpError by the client; forward it as text
    out = await agentmcp._call(_RaisingSession(), "list_functions", {})
    assert out.startswith("[agent /mcp error]") and "Missing required parameter 'nodes'" in out
