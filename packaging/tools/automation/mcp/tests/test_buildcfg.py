from pathlib import Path

import pytest

from netdata_mcp import buildcfg


def _configure_dir(tmp: Path, build_type: str, *, marker: bool = True) -> Path:
    """Simulate a configured build dir with the given CMAKE_BUILD_TYPE."""
    bdir = tmp / "build"
    bdir.mkdir(parents=True, exist_ok=True)
    (bdir / "build.ninja").write_text("")
    (bdir / "CMakeCache.txt").write_text(f"CMAKE_BUILD_TYPE:STRING={build_type}\n")
    if marker:
        (bdir / ".mcp-managed").write_text("managed\n")
    return bdir


def test_build_dir_is_single_per_worktree(tmp_path):
    assert buildcfg.build_dir(str(tmp_path)) == tmp_path / "build"


def test_commands_target_the_single_build_dir(tmp_path):
    bdir = str(tmp_path / "build")
    assert buildcfg.build_command(str(tmp_path)) == ["ninja", "-C", bdir]
    assert buildcfg.install_command(str(tmp_path)) == ["ninja", "-C", bdir, "install"]
    cfg = buildcfg.configure_command(str(tmp_path), "debug")
    assert "-B" in cfg and bdir in cfg


def test_lock_and_log_paths(tmp_path):
    assert buildcfg.lock_key(str(tmp_path)) == str((tmp_path / "build").resolve())
    assert buildcfg.lock_file(str(tmp_path)) == (tmp_path / ".netdata-mcp-build.lock").resolve()
    assert buildcfg.log_path(str(tmp_path)) == tmp_path / "build" / ".netdata-build.log"
    assert buildcfg.compile_commands_path(str(tmp_path)) == tmp_path / "build" / "compile_commands.json"


def test_needs_configure_when_unconfigured(tmp_path):
    assert buildcfg.needs_configure(str(tmp_path), "debug") is True


def test_needs_configure_false_when_matching_profile(tmp_path):
    _configure_dir(tmp_path, "Debug")
    assert buildcfg.needs_configure(str(tmp_path), "debug") is False


def test_needs_configure_true_on_profile_switch(tmp_path):
    _configure_dir(tmp_path, "Debug")
    # optimized == RelWithDebInfo != Debug -> reconfigure
    assert buildcfg.needs_configure(str(tmp_path), "optimized") is True


def test_needs_configure_rejects_unknown_profile(tmp_path):
    with pytest.raises(ValueError):
        buildcfg.needs_configure(str(tmp_path), "bogus")


def test_assert_ownable_passes_when_absent_or_marked(tmp_path):
    buildcfg.assert_ownable(str(tmp_path))  # no build dir -> ok
    _configure_dir(tmp_path, "Debug", marker=True)
    buildcfg.assert_ownable(str(tmp_path))  # configured + marked -> ok


def test_assert_ownable_refuses_a_foreign_build(tmp_path):
    _configure_dir(tmp_path, "Release", marker=False)  # configured, no marker
    with pytest.raises(buildcfg.BuildDirNotOwned):
        buildcfg.assert_ownable(str(tmp_path))


def test_failed_configure_leaves_dir_recoverable(tmp_path):
    # The dir is claimed (marked) BEFORE configure, so a failed/killed configure
    # (cache written, no build.ninja) is recoverable: still owned, reconfigures.
    buildcfg.mark_owned(str(tmp_path))  # claim
    (tmp_path / "build" / "CMakeCache.txt").write_text("CMAKE_BUILD_TYPE:STRING=Debug\n")
    buildcfg.assert_ownable(str(tmp_path))  # marker present -> not refused
    assert buildcfg.needs_configure(str(tmp_path), "debug") is True  # no build.ninja -> reconfigure


def test_claim_build_dir_refuses_foreign_then_claims_ownable(tmp_path):
    _configure_dir(tmp_path, "Release", marker=False)  # foreign
    with pytest.raises(buildcfg.BuildDirNotOwned):
        buildcfg.claim_build_dir(str(tmp_path))
    # a fresh worktree: claim succeeds and stamps the marker
    fresh = tmp_path / "fresh"
    fresh.mkdir()
    buildcfg.claim_build_dir(str(fresh))
    assert (fresh / "build" / ".mcp-managed").is_file()


def test_mark_owned_stamps_marker(tmp_path):
    buildcfg.mark_owned(str(tmp_path))
    assert (tmp_path / "build" / ".mcp-managed").is_file()
    # and ownership now passes even with a cache present
    (tmp_path / "build" / "CMakeCache.txt").write_text("CMAKE_BUILD_TYPE:STRING=Debug\n")
    buildcfg.assert_ownable(str(tmp_path))
