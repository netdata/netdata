import asyncio
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

from netdata_mcp.locks import BuildLockCancelled, build_dir_lock


async def _hold(lockfile: Path, marker: Path) -> None:
    async with build_dir_lock(lockfile):
        with open(marker, "a") as f:
            f.write("S\n")
        await asyncio.sleep(0.3)
        with open(marker, "a") as f:
            f.write("E\n")


async def test_build_dir_lock_aborts_on_cancel_check(tmp_path):
    # A waiter whose cancel_check is true must abandon the wait with
    # BuildLockCancelled instead of blocking until the holder releases.
    lockfile = tmp_path / ".netdata-mcp-build.lock"
    entered = False
    async with build_dir_lock(lockfile):  # hold it
        with pytest.raises(BuildLockCancelled):
            async with build_dir_lock(lockfile, cancel_check=lambda: True):
                entered = True
    assert entered is False


async def test_build_dir_lock_serializes_coroutines(tmp_path):
    # Two fresh locks on the same file (separate fds) must mutually exclude even
    # in one process -> the two critical sections don't interleave.
    lockfile = tmp_path / ".netdata-mcp-build.lock"
    marker = tmp_path / "order.txt"
    marker.write_text("")
    await asyncio.gather(_hold(lockfile, marker), _hold(lockfile, marker))
    assert marker.read_text().split() == ["S", "E", "S", "E"]


# Runs our async lock helper in a child process; two children contending on one
# lockfile prove cross-process exclusion (the reason for a file lock over an
# asyncio.Lock). Only an interleaved S,S,E,E (no locking) fails the assertion;
# if the children happen to run sequentially the order is still S,E,S,E.
_CHILD = textwrap.dedent(
    """
    import asyncio, sys
    from pathlib import Path
    from netdata_mcp.locks import build_dir_lock

    lockfile, marker = Path(sys.argv[1]), Path(sys.argv[2])

    async def main():
        async with build_dir_lock(lockfile):
            with open(marker, "a") as f: f.write("S\\n")
            await asyncio.sleep(0.5)
            with open(marker, "a") as f: f.write("E\\n")

    asyncio.run(main())
    """
)


def test_build_dir_lock_serializes_across_processes(tmp_path):
    lockfile = tmp_path / ".netdata-mcp-build.lock"
    marker = tmp_path / "order.txt"
    marker.write_text("")
    procs = [
        subprocess.Popen([sys.executable, "-c", _CHILD, str(lockfile), str(marker)])
        for _ in range(2)
    ]
    for p in procs:
        assert p.wait(timeout=30) == 0
    assert marker.read_text().split() == ["S", "E", "S", "E"]
