import argparse
import importlib.util
import json
from pathlib import Path

import pytest

_spec = importlib.util.spec_from_file_location(
    "setup_mcp", Path(__file__).parent.parent / "scripts" / "setup_mcp.py"
)
setup_mcp = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(setup_mcp)

TD = Path("/repo/packaging/tools/automation/mcp")
CLAIM = {"NETDATA_CLAIM_TOKEN": "tok", "NETDATA_CLAIM_ROOMS": "room-1"}


def test_opencode_entry_shape():
    e = setup_mcp.opencode_entry(TD, CLAIM)
    assert e["type"] == "local"
    assert e["cwd"] == str(TD)
    assert e["command"] == ["uv", "run", "netdata-build-mcp", "--transport", "stdio"]
    assert e["enabled"] is True
    assert e["environment"] == CLAIM  # claim creds injected into the per-server env


def test_merge_adds_into_existing_config_without_touching_other_keys():
    config = {
        "theme": "dark",
        "mcp": {"other": {"type": "remote", "url": "https://x/mcp"}},
        "provider": {"k": {"options": {"apiKey": "SECRET"}}},
    }
    out = setup_mcp.merge_opencode_config(config, TD, CLAIM)
    # untouched
    assert out["theme"] == "dark"
    assert out["provider"]["k"]["options"]["apiKey"] == "SECRET"
    assert out["mcp"]["other"] == {"type": "remote", "url": "https://x/mcp"}
    # added
    assert out["mcp"]["netdata-build"]["cwd"] == str(TD)
    # input not mutated in place
    assert "netdata-build" not in config["mcp"]


def test_merge_is_idempotent():
    config = {"mcp": {}}
    once = setup_mcp.merge_opencode_config(config, TD, CLAIM)
    twice = setup_mcp.merge_opencode_config(once, TD, CLAIM)
    assert once == twice


def test_merge_creates_mcp_when_absent():
    out = setup_mcp.merge_opencode_config({"theme": "dark"}, TD, CLAIM)
    assert "netdata-build" in out["mcp"]
    assert out["theme"] == "dark"


def test_setup_opencode_writes_then_is_idempotent(tmp_path):
    cfg = tmp_path / "opencode.json"
    cfg.write_text(json.dumps({"mcp": {"keep": {"type": "remote", "url": "u"}}}), encoding="utf-8")
    setup_mcp.setup_opencode(TD, CLAIM, cfg_path=cfg)
    data = json.loads(cfg.read_text())
    assert data["mcp"]["keep"] == {"type": "remote", "url": "u"}  # preserved
    assert data["mcp"]["netdata-build"]["cwd"] == str(TD)
    assert data["mcp"]["netdata-build"]["environment"] == CLAIM
    mtime = cfg.stat().st_mtime_ns
    setup_mcp.setup_opencode(TD, CLAIM, cfg_path=cfg)  # second run: already current
    assert cfg.stat().st_mtime_ns == mtime  # no rewrite/churn


def test_setup_opencode_creates_file_when_absent(tmp_path):
    cfg = tmp_path / "sub" / "opencode.json"  # parent dir does not exist yet
    setup_mcp.setup_opencode(TD, CLAIM, cfg_path=cfg)
    assert json.loads(cfg.read_text())["mcp"]["netdata-build"]["cwd"] == str(TD)


def test_setup_opencode_clear_error_on_invalid_json(tmp_path):
    cfg = tmp_path / "opencode.json"
    cfg.write_text('{"mcp": {"x": 1},}', encoding="utf-8")  # trailing comma -> invalid
    with pytest.raises(RuntimeError, match="not valid JSON"):
        setup_mcp.setup_opencode(TD, CLAIM, cfg_path=cfg)


def test_claude_command_construction():
    cmd = setup_mcp.claude_command(TD, CLAIM)
    # server name comes BEFORE --env (which is variadic and would otherwise eat it)
    assert cmd[:6] == ["claude", "mcp", "add", "--scope", "user", "netdata-build"]
    sep = cmd.index("--")
    env_at = cmd.index("--env")
    assert cmd.index("netdata-build") < env_at < sep  # name, then --env KEY=val…, then --
    assert "NETDATA_CLAIM_TOKEN=tok" in cmd and "NETDATA_CLAIM_ROOMS=room-1" in cmd
    assert cmd[sep + 1:] == ["uv", "run", "--directory", str(TD), "netdata-build-mcp", "--transport", "stdio"]


def test_claude_command_without_claim_has_no_env():
    cmd = setup_mcp.claude_command(TD, {})
    assert "--env" not in cmd
    sep = cmd.index("--")
    assert cmd[sep - 1] == "netdata-build"


def _args(**kw):
    base = {
        "claim_token": None, "claim_rooms": None, "claim_url": None,
        "cloud_token": None, "cloud_hostname": None,
    }
    base.update(kw)
    return argparse.Namespace(**base)


def test_resolve_claim_cli_beats_env():
    creds = setup_mcp.resolve_claim_creds(
        _args(claim_token="cli-tok", claim_rooms="cli-room"),
        {"NETDATA_CLAIM_TOKEN": "env-tok", "NETDATA_CLAIM_ROOMS": "env-room"},
    )
    assert creds == {"NETDATA_CLAIM_TOKEN": "cli-tok", "NETDATA_CLAIM_ROOMS": "cli-room"}


def test_resolve_claim_falls_back_to_env_and_trims():
    creds = setup_mcp.resolve_claim_creds(
        _args(),
        {"NETDATA_CLAIM_TOKEN": " env-tok ", "NETDATA_CLAIM_URL": "https://app.netdata.cloud"},
    )
    assert creds == {"NETDATA_CLAIM_TOKEN": "env-tok", "NETDATA_CLAIM_URL": "https://app.netdata.cloud"}
    assert "NETDATA_CLAIM_ROOMS" not in creds  # optional, unset -> omitted


def test_resolve_claim_fails_without_token():
    with pytest.raises(SystemExit, match="claim token required"):
        setup_mcp.resolve_claim_creds(_args(), {})
    with pytest.raises(SystemExit):  # whitespace-only counts as unset
        setup_mcp.resolve_claim_creds(_args(claim_token="   "), {})


def test_resolve_cloud_cli_beats_env():
    creds = setup_mcp.resolve_cloud_creds(
        _args(cloud_token="cli-ctok", cloud_hostname="cli.host"),
        {"NETDATA_CLOUD_TOKEN": "env-ctok", "NETDATA_CLOUD_HOSTNAME": "env.host"},
    )
    assert creds == {"NETDATA_CLOUD_TOKEN": "cli-ctok", "NETDATA_CLOUD_HOSTNAME": "cli.host"}


def test_resolve_cloud_falls_back_to_env_and_trims():
    creds = setup_mcp.resolve_cloud_creds(
        _args(), {"NETDATA_CLOUD_TOKEN": " env-ctok "}
    )
    assert creds == {"NETDATA_CLOUD_TOKEN": "env-ctok"}
    assert "NETDATA_CLOUD_HOSTNAME" not in creds  # optional, unset -> omitted


def test_resolve_cloud_fails_without_token():
    with pytest.raises(SystemExit, match="cloud token required"):
        setup_mcp.resolve_cloud_creds(_args(), {})
    with pytest.raises(SystemExit):  # whitespace-only counts as unset
        setup_mcp.resolve_cloud_creds(_args(cloud_token="   "), {})


def test_redact_masks_claim_and_cloud_values():
    s = ("Invalid: --env NETDATA_CLAIM_TOKEN=qzzwSECRET NETDATA_CLAIM_ROOMS=room-1 "
         "NETDATA_CLOUD_TOKEN=ckSECRET oops")
    out = setup_mcp._redact(s)
    assert "qzzwSECRET" not in out and "room-1" not in out and "ckSECRET" not in out
    assert "NETDATA_CLAIM_TOKEN=***" in out and "NETDATA_CLOUD_TOKEN=***" in out


def test_parse_args_defaults():
    a = setup_mcp.parse_args([])
    assert a.tool == "all"
    assert a.source_dir is None
    assert a.claim_token is None and a.claim_rooms is None and a.claim_url is None
    assert a.cloud_token is None and a.cloud_hostname is None
