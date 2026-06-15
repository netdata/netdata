import pytest

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
    assert reg.set_otel("missing", OtelConfig(wal_max_log_entries=5)) is None
    reg.declare("a", "/wt", "debug")
    spec = reg.set_otel("a", OtelConfig(wal_max_log_entries=5, index_max_files=2))
    assert spec is not None
    assert spec.otel.wal_max_log_entries == 5
    assert spec.otel.index_max_files == 2
    assert reg.get("a").otel.wal_max_log_entries == 5


def test_re_declare_preserves_otel_config():
    reg = AgentRegistry()
    reg.declare("a", "/wt", "debug")
    reg.set_otel("a", OtelConfig(wal_max_log_entries=10))
    reg.declare("a", "/wt2", "optimized")  # idempotent update of worktree/profile
    assert reg.get("a").otel.wal_max_log_entries == 10  # otel survived


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


def test_get_unknown_returns_none():
    assert AgentRegistry().get("nope") is None
