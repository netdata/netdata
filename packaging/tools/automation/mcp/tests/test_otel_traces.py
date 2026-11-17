import pytest
from mcp.server.fastmcp import FastMCP

from netdata_mcp.tools import otel_traces
from netdata_mcp.tools.otel_traces import Condition, build_payload

_CHIP = Condition(key="name", op="=", values=["GET /"])


def test_build_payload():
    cases = {
        "list defaults": ("list", None, {}, {}),
        "list full": (
            "list", "t1",
            dict(after=-600, before=0, last=20, anchor="c1", conditions=[_CHIP], text=["err"], skip_overview=True),
            {
                "after": -600, "before": 0, "last": 20, "anchor": "c1",
                "conditions": [{"key": "name", "op": "=", "values": ["GET /"]}],
                "text": ["err"], "skip_overview": True, "tenant": "t1",
            },
        ),
        "info drops tenant": ("info", "t1", {}, {"info": {}}),
        "trace": (
            "trace", None, dict(trace_id="ab" * 16, after=1, before=2),
            {"trace": {"id": "ab" * 16, "after": 1, "before": 2}},
        ),
        "suggest values": (
            "suggest", "t1", dict(key="name", term="GE", max_items=5),
            {"suggest": {"key": "name", "term": "GE", "max_items": 5}, "tenant": "t1"},
        ),
        "suggest tier": ("suggest", None, dict(tier="low"), {"suggest": {"tier": "low"}}),
        "explore histogram flag": ("explore", None, dict(histogram=True), {"explore": {"histogram": {}}}),
        "explore implied sections": (
            "explore", None,
            dict(after=-300, histogram_stack="kind", rows_limit=10, rows_columns=["name"], text=["abc"]),
            {"explore": {
                "after": -300, "text": ["abc"],
                "histogram": {"stack": "kind"}, "rows": {"limit": 10, "columns": ["name"]},
            }},
        ),
        "explore false flags ask nothing": (
            "explore", None, dict(histogram=False, rows=False), {"explore": {}},
        ),
    }
    for name, (mode, tenant, params, want) in cases.items():
        assert build_payload(mode, tenant, params) == want, name


def test_build_payload_refuses_params_of_other_modes():
    cases = {
        "list": ("list", dict(trace_id="x", rows=True), "mode 'list' does not take: rows, trace_id"),
        "info": ("info", dict(after=1), "mode 'info' does not take: after"),
        "suggest": ("suggest", dict(conditions=[_CHIP]), "mode 'suggest' does not take: conditions"),
        "trace": ("trace", dict(last=5), "mode 'trace' does not take: last"),
    }
    for name, (mode, params, msg) in cases.items():
        with pytest.raises(ValueError) as exc:
            build_payload(mode, None, params)
        assert str(exc.value) == msg, name


async def test_tool_registered():
    mcp = FastMCP("t")
    otel_traces.register(mcp)
    tool = next(t for t in await mcp.list_tools() if t.name == "netdata_agent_otel_traces")
    assert tool.inputSchema["required"] == ["agent_id"]
    assert tool.inputSchema["properties"]["mode"]["default"] == "list"
