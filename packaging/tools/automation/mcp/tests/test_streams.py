import asyncio
import subprocess

from netdata_mcp import streams


# ── command builders (pure) ─────────────────────────────────────────────────────
def test_synth_logs_cmd_basics():
    cmd = streams.synth_logs_cmd(
        "127.0.0.1:4317", count=25, field_cardinality=4, spacing_nanos=1_000_000_000,
        start_time_nanos=None, seed=0, tenant_id=None, batch_size=100,
        flush_interval_ms=300, connect_timeout_secs=30,
    )
    assert cmd[:7] == ["cargo", "run", "--quiet", "-p", "otel-streams", "--bin", "synth"]
    assert "--otel-endpoint" in cmd and "http://127.0.0.1:4317" in cmd
    assert cmd[cmd.index("--count") + 1] == "25"
    assert cmd[cmd.index("--field-cardinality") + 1] == "4"  # logs-only flag
    # omitted optionals stay out (synth applies its own defaults)
    assert "--start-time-nanos" not in cmd and "--tenant-id" not in cmd
    assert "--service-name" not in cmd and "--service-namespace" not in cmd


def test_synth_logs_cmd_includes_set_optionals():
    cmd = streams.synth_logs_cmd(
        "h:1", count=1, field_cardinality=1, spacing_nanos=0, start_time_nanos=123,
        seed=5, tenant_id="t1", batch_size=1, flush_interval_ms=1, connect_timeout_secs=1,
        service_name="api", service_namespace="prod",
    )
    assert cmd[cmd.index("--start-time-nanos") + 1] == "123"
    assert cmd[cmd.index("--tenant-id") + 1] == "t1"
    assert cmd[cmd.index("--seed") + 1] == "5"
    assert cmd[cmd.index("--service-name") + 1] == "api"
    assert cmd[cmd.index("--service-namespace") + 1] == "prod"


def test_synth_traces_cmd_basics():
    cmd = streams.synth_traces_cmd(
        "127.0.0.1:4317", count=25, spacing_nanos=1_000_000_000, duration_nanos=5_000_000,
        start_time_nanos=None, seed=0, tenant_id=None, batch_size=100,
        connect_timeout_secs=30,
    )
    assert cmd[:7] == ["cargo", "run", "--quiet", "-p", "otel-streams", "--bin", "synth-traces"]
    assert "--otel-endpoint" in cmd and "http://127.0.0.1:4317" in cmd
    assert cmd[cmd.index("--count") + 1] == "25"
    assert cmd[cmd.index("--duration-nanos") + 1] == "5000000"  # traces-only flag
    assert "--field-cardinality" not in cmd  # logs-only flag must NOT appear
    # synth-traces exports synchronously; the logs flush knob is not exposed/emitted
    assert "--flush-interval-ms" not in cmd
    assert "--start-time-nanos" not in cmd and "--tenant-id" not in cmd
    assert "--service-name" not in cmd and "--service-namespace" not in cmd


def test_synth_traces_cmd_includes_set_optionals():
    cmd = streams.synth_traces_cmd(
        "h:1", count=1, spacing_nanos=0, duration_nanos=7, start_time_nanos=123,
        seed=5, tenant_id="t1", batch_size=1, connect_timeout_secs=1,
        service_name="api", service_namespace="prod",
    )
    assert cmd[cmd.index("--start-time-nanos") + 1] == "123"
    assert cmd[cmd.index("--tenant-id") + 1] == "t1"
    assert cmd[cmd.index("--duration-nanos") + 1] == "7"
    assert cmd[cmd.index("--service-name") + 1] == "api"
    assert cmd[cmd.index("--service-namespace") + 1] == "prod"


def test_synth_cmds_forward_explicit_empty_service_identity():
    # An explicit "" is a present-but-empty identity (queryable empty value),
    # distinct from omitting it — both builders must FORWARD it, not drop it.
    # logs carries field_cardinality + flush_interval_ms; traces carries
    # duration_nanos (no flush knob) — signal-specific args go in `extra`.
    for builder, extra in (
        (streams.synth_logs_cmd, {"field_cardinality": 1, "flush_interval_ms": 1}),
        (streams.synth_traces_cmd, {"duration_nanos": 1}),
    ):
        cmd = builder(
            "h:1", count=1, spacing_nanos=0, start_time_nanos=None, seed=0,
            tenant_id=None, batch_size=1, connect_timeout_secs=1,
            service_name="", service_namespace="", **extra,
        )
        assert cmd[cmd.index("--service-name") + 1] == ""
        assert cmd[cmd.index("--service-namespace") + 1] == ""


def test_synth_cmds_omit_service_identity_when_none():
    # Omitted (None) → no flag → the bin applies its own default (absent ≠ empty).
    for builder, extra in (
        (streams.synth_logs_cmd, {"field_cardinality": 1, "flush_interval_ms": 1}),
        (streams.synth_traces_cmd, {"duration_nanos": 1}),
    ):
        cmd = builder(
            "h:1", count=1, spacing_nanos=0, start_time_nanos=None, seed=0,
            tenant_id=None, batch_size=1, connect_timeout_secs=1,
            service_name=None, service_namespace=None, **extra,
        )
        assert "--service-name" not in cmd and "--service-namespace" not in cmd


def test_stream_cmd_certstream_url_flag():
    cmd = streams.stream_cmd(
        "certstream", "h:1", url="ws://x/", collections=None, start=None, rate=None,
        tenant_id=None, batch_size=100, flush_interval_ms=1000,
    )
    assert cmd[:7] == ["cargo", "run", "--quiet", "-p", "otel-streams", "--bin", "certstream"]
    assert "--certstream-url" in cmd and "ws://x/" in cmd
    assert "--jetstream-url" not in cmd


def test_stream_cmd_jetstream_url_and_collections():
    cmd = streams.stream_cmd(
        "jetstream", "h:1", url="wss://y/", collections="app.bsky.feed.post", start=None,
        rate=None, tenant_id="t", batch_size=50, flush_interval_ms=500,
    )
    assert "--jetstream-url" in cmd and "wss://y/" in cmd
    assert cmd[cmd.index("--collections") + 1] == "app.bsky.feed.post"
    assert cmd[cmd.index("--tenant-id") + 1] == "t"


def test_stream_cmd_github_start_and_rate():
    cmd = streams.stream_cmd(
        "github", "h:1", url=None, collections=None, start="2024-06-01-12", rate=0,
        tenant_id=None, batch_size=100, flush_interval_ms=1000,
    )
    assert cmd[cmd.index("--start") + 1] == "2024-06-01-12"
    assert cmd[cmd.index("--rate") + 1] == "0"  # rate=0 (unlimited) must be emitted


# ── StreamRegistry lifecycle (real subprocess, harmless commands) ────────────────
def _crates(tmp_path):
    d = tmp_path / "src" / "crates"
    d.mkdir(parents=True)
    return str(tmp_path)


def test_registry_start_then_stop(tmp_path):
    async def run():
        reg = streams.StreamRegistry()
        wt = _crates(tmp_path)
        s = reg.start("ag", wt, "127.0.0.1:1", "certstream", ["sleep", "30"])
        assert s.state == "running" and reg.get(s.stream_id) is s
        await asyncio.sleep(0.2)  # let it actually spawn
        stopped = await reg.stop(s.stream_id, wait=3.0)
        assert stopped is not None and stopped.state == "stopped"
        assert stopped.returncode is not None and stopped.returncode < 0  # killed by signal

    asyncio.run(run())


def test_registry_marks_unexpected_exit_failed(tmp_path):
    async def run():
        reg = streams.StreamRegistry()
        wt = _crates(tmp_path)
        s = reg.start("ag", wt, "127.0.0.1:1", "github", ["sh", "-c", "exit 0"])
        await s._task  # daemon exits on its own → failure (we didn't stop it)
        assert s.state == "failed" and s.returncode == 0 and "exited" in (s.error or "")

    asyncio.run(run())


def test_registry_stop_unknown_returns_none():
    async def run():
        reg = streams.StreamRegistry()
        assert await reg.stop("nope") is None

    asyncio.run(run())


def test_registry_list_enumerates_streams(tmp_path):
    async def run():
        reg = streams.StreamRegistry()
        wt = _crates(tmp_path)
        assert reg.list() == []
        a = reg.start("ag", wt, "127.0.0.1:1", "certstream", ["sleep", "30"])
        b = reg.start("ag", wt, "127.0.0.1:1", "jetstream", ["sleep", "30"])
        ids = {s.stream_id for s in reg.list()}
        assert ids == {a.stream_id, b.stream_id}
        await reg.stop_all(wait=3.0)

    asyncio.run(run())


def test_run_synth_timeout_kills_and_reports(tmp_path):
    async def run():
        wt = _crates(tmp_path)
        rc, tail, err = await streams.run_synth(wt, ["sleep", "30"], timeout=1)
        assert rc is None and err is not None and "timed out" in err

    asyncio.run(run())


def _alive(marker: str) -> bool:
    return subprocess.run(["pgrep", "-f", f"sleep {marker}"], capture_output=True).returncode == 0


def test_run_synth_cancel_kills_child(tmp_path):
    # Outer cancellation must not orphan the shielded cargo/synth process group.
    async def run():
        wt = _crates(tmp_path)
        marker = "99887766554433"  # unique sleep duration to grep for
        task = asyncio.create_task(streams.run_synth(wt, ["sleep", marker], timeout=60))
        for _ in range(50):  # wait until the child is actually up
            await asyncio.sleep(0.1)
            if _alive(marker):
                break
        assert _alive(marker), "child never spawned"
        task.cancel()
        try:
            await task
        except asyncio.CancelledError:
            pass
        for _ in range(20):  # give the SIGKILL a moment to land
            if not _alive(marker):
                break
            await asyncio.sleep(0.1)
        assert not _alive(marker), "child was orphaned after outer cancel"

    asyncio.run(run())
