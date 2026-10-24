import asyncio

from netdata_mcp.runner import LogBuffer, drain_all, escalate_cancel, run_command, run_phases


def test_logbuffer_offset_and_truncation():
    b = LogBuffer(max_lines=3)
    for i in range(5):
        b.append(f"l{i}")  # l0, l1 evicted; window = l2, l3, l4
    assert b.total == 5

    sl = b.read(0)
    assert sl.truncated is True
    assert sl.text == "l2\nl3\nl4"
    assert sl.next_offset == 5

    # reading at the head returns nothing new and is not truncated
    sl2 = b.read(5)
    assert sl2.text == ""
    assert sl2.truncated is False
    assert sl2.next_offset == 5


def test_logbuffer_incremental_read_after_more_lines():
    b = LogBuffer(max_lines=10)
    b.append("a")
    b.append("b")
    sl = b.read(0)
    assert sl.text == "a\nb"
    b.append("c")
    sl2 = b.read(sl.next_offset)
    assert sl2.text == "c"
    assert sl2.truncated is False


def test_logbuffer_tail():
    b = LogBuffer()
    for i in range(5):
        b.append(f"l{i}")
    assert b.tail(2) == "l3\nl4"
    assert b.tail(0) == ""


async def test_run_command_captures_merged_output_and_returncode():
    b = LogBuffer()
    rc = await run_command(["sh", "-c", "echo out; echo err 1>&2; exit 3"], cwd=".", sink=b.append)
    assert rc == 3
    text = b.read(0).text
    assert "out" in text and "err" in text


async def test_run_command_sink_receives_every_line():
    lines: list[str] = []
    rc = await run_command(["sh", "-c", "printf 'a\\nb\\nc\\n'"], cwd=".", sink=lines.append)
    assert rc == 0
    assert lines == ["a", "b", "c"]


async def test_run_command_on_spawn_receives_proc():
    seen = {}
    await run_command(["sh", "-c", "true"], cwd=".", sink=lambda _l: None, on_spawn=lambda p: seen.setdefault("pid", p.pid))
    assert "pid" in seen


async def test_run_command_env_is_merged_over_parent():
    # the child sees the supplied var AND still inherits the parent env (PATH)
    lines: list[str] = []
    rc = await run_command(
        ["sh", "-c", 'echo "$PROBE_X $([ -n "$PATH" ] && echo PATHSET)"'],
        cwd=".", sink=lines.append, env={"PROBE_X": "hello"},
    )
    assert rc == 0 and lines == ["hello PATHSET"]


class _FakeWork:
    """Minimal Cancellable: a task that ends only when signalled, recording
    whether SIGTERM (request_cancel) or the SIGKILL escalation (force_kill) did it."""

    def __init__(self, *, dies_on_sigterm: bool) -> None:
        self._dies_on_sigterm = dies_on_sigterm
        self._stop = asyncio.Event()
        self.term = False
        self.killed = False
        self._task = asyncio.get_running_loop().create_task(self._stop.wait())

    @property
    def done(self) -> bool:
        return self._task.done()

    def request_cancel(self) -> None:
        self.term = True
        if self._dies_on_sigterm:
            self._stop.set()

    def force_kill(self) -> None:
        self.killed = True
        self._stop.set()


async def test_escalate_cancel_stops_on_sigterm():
    w = _FakeWork(dies_on_sigterm=True)
    await escalate_cancel(w, wait=1.0)
    assert w.term and not w.killed and w.done  # SIGTERM was enough, no escalation


async def test_escalate_cancel_escalates_to_sigkill():
    w = _FakeWork(dies_on_sigterm=False)
    await escalate_cancel(w, wait=0.2)
    assert w.term and w.killed and w.done  # ignored SIGTERM -> SIGKILL reaped it


async def test_escalate_cancel_is_noop_when_already_done():
    w = _FakeWork(dies_on_sigterm=True)
    w.force_kill()  # finish it out of band
    await w._task
    w.term = w.killed = False
    await escalate_cancel(w, wait=1.0)
    assert not w.term and not w.killed  # already done -> untouched


async def test_drain_all_handles_both_signal_responses():
    polite = _FakeWork(dies_on_sigterm=True)
    stubborn = _FakeWork(dies_on_sigterm=False)
    await drain_all([polite, stubborn], wait=0.2)
    assert polite.done and stubborn.done
    assert polite.term and stubborn.term  # both got SIGTERM first
    assert not polite.killed  # died on SIGTERM
    assert stubborn.killed    # needed the SIGKILL escalation


async def test_drain_all_empty_is_noop():
    await drain_all([], wait=1.0)  # must not raise


class _FakeHost:
    """Minimal PhaseHost: tracks the phase loop's side effects on the host."""

    def __init__(self) -> None:
        self.current_phase: str | None = None
        self._cancelled = False
        self._proc = None

    def _set_proc(self, proc) -> None:
        self._proc = proc


async def test_run_phases_runs_all_and_succeeds():
    host = _FakeHost()
    lines: list[str] = []
    out = await run_phases(
        host, [("a", ["sh", "-c", "echo A"], "."), ("b", ["sh", "-c", "echo B"], ".")], lines.append
    )
    assert out.status == "succeeded"
    assert host.current_phase == "b" and host._proc is None  # advanced through both, proc cleared
    assert any("A" in line for line in lines) and any("B" in line for line in lines)


async def test_run_phases_stops_at_first_failure():
    host = _FakeHost()
    out = await run_phases(
        host, [("a", ["sh", "-c", "exit 5"], "."), ("b", ["sh", "-c", "echo nope"], ".")], lambda _l: None
    )
    assert out.status == "failed" and out.phase == "a" and out.returncode == 5
    assert host.current_phase == "a"  # the second phase never ran


async def test_run_phases_cancel_before_any_phase():
    host = _FakeHost()
    host._cancelled = True
    out = await run_phases(host, [("a", ["sh", "-c", "echo x"], ".")], lambda _l: None)
    assert out.status == "cancelled" and out.phase == "a" and out.returncode is None


async def test_run_phases_cancel_after_a_phase_completes():
    # cancel arrives during the first phase -> the post-command check ends it,
    # carrying that phase's exit code; the next phase never runs.
    host = _FakeHost()
    calls: list[str] = []

    def sink(line: str) -> None:
        calls.append(line)
        host._cancelled = True

    out = await run_phases(
        host, [("a", ["sh", "-c", "true"], "."), ("b", ["sh", "-c", "echo nope"], ".")], sink
    )
    assert out.status == "cancelled" and out.phase == "a" and out.returncode == 0
    assert not any("nope" in line for line in calls)  # phase b never ran


async def test_run_phases_empty_list_respects_cancellation():
    # post-loop guard: an empty phase list must still honor an already-set cancel
    # rather than falsely reporting success.
    host = _FakeHost()
    host._cancelled = True
    out = await run_phases(host, [], lambda _l: None)
    assert out.status == "cancelled" and out.phase is None


async def test_run_phases_empty_list_succeeds_when_not_cancelled():
    host = _FakeHost()
    out = await run_phases(host, [], lambda _l: None)
    assert out.status == "succeeded"
