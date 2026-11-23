from pathlib import Path

import pytest

from netdata_mcp import runtime
from netdata_mcp.agents import AgentRegistry
from netdata_mcp.runtime import OtelConfig


def test_declare_registers_and_get_returns_it():
    reg = AgentRegistry()
    spec = reg.declare("parent", "/wt", "optimized")
    assert spec.agent_id == "parent"
    assert spec.worktree == "/wt"
    assert spec.profile == "optimized"
    assert reg.get("parent") is spec
    assert spec.otel == OtelConfig()  # defaults until configured


def test_set_otel_stores_config_and_unknown_returns_none():
    reg = AgentRegistry()
    assert reg.set_otel("missing", OtelConfig(extra_yaml="x: 1\n")) is None
    reg.declare("a", "/wt", "debug")
    cfg = OtelConfig(extra_yaml="auth: {enabled: true}\n")
    spec = reg.set_otel("a", cfg)
    assert spec is not None and spec.otel == cfg
    assert reg.get("a").otel == cfg


def test_re_declare_preserves_otel_config():
    reg = AgentRegistry()
    reg.declare("a", "/wt", "debug")
    reg.set_otel("a", OtelConfig(extra_yaml="x: 2\n"))
    reg.declare("a", "/wt2", "optimized")  # idempotent update of worktree/profile
    assert reg.get("a").otel == OtelConfig(extra_yaml="x: 2\n")  # otel survived


def test_declare_is_idempotent_and_updates_spec():
    reg = AgentRegistry()
    first = reg.declare("a", "/wt", "debug")
    again = reg.declare("a", "/wt2", "optimized")
    assert again is first  # same object, addressed by id
    assert again.worktree == "/wt2"
    assert again.profile == "optimized"


def test_declare_rejects_unknown_profile():
    reg = AgentRegistry()
    with pytest.raises(ValueError):
        reg.declare("a", "/wt", "nonexistent")


def test_declare_rejects_unsafe_agent_id():
    reg = AgentRegistry()
    with pytest.raises(ValueError):
        reg.declare("../x", "/wt", "debug")


def test_declare_rejects_id_too_long_for_socket_paths(monkeypatch):
    monkeypatch.setattr(runtime.Path, "home", classmethod(lambda cls: Path("/h")))
    reg = AgentRegistry()
    assert reg.declare("x" * 46, "/wt", "debug").agent_id == "x" * 46
    with pytest.raises(ValueError):
        reg.declare("x" * 47, "/wt", "debug")


def test_get_unknown_returns_none():
    assert AgentRegistry().get("nope") is None


def test_declare_pins_port_and_re_declare_without_it_unpins():
    reg = AgentRegistry()
    assert reg.declare("a", "/wt", "debug").port is None  # auto-assigned by default
    assert reg.declare("a", "/wt", "debug", port=19999).port == 19999
    assert reg.declare("a", "/wt", "debug").port is None  # declare replaces, like worktree/profile


@pytest.mark.parametrize("port", [0, 65536, -1])
def test_declare_rejects_out_of_range_port(port):
    with pytest.raises(ValueError):
        AgentRegistry().declare("a", "/wt", "debug", port=port)
