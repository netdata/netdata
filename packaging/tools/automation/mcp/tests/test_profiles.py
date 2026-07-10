import typing

import pytest

from netdata_mcp import buildcfg, profiles
from netdata_mcp.tools._common import Profile

_BDIR = "/tmp/wt/build"


def test_profile_literal_matches_profiles():
    """The tool's Profile Literal must stay in sync with profiles.PROFILES."""
    assert set(typing.get_args(Profile)) == set(profiles.PROFILE_NAMES)


def test_only_debug_and_optimized():
    assert set(profiles.PROFILE_NAMES) == {"debug", "optimized"}


def test_plugin_on_off_sets_are_disjoint():
    # both are unpacked into each profile; an accidental key in both would make
    # the result depend on implicit dict-merge order.
    assert set(profiles._PLUGINS_ON) & set(profiles._PLUGINS_OFF) == set()


def test_cmake_args_structure():
    args = profiles.cmake_args("/tmp/wt", "debug", profiles.PROFILES["debug"], _BDIR)
    assert args[0] == "cmake"
    assert args[1:3] == ["-G", "Ninja"]
    assert "-S" in args and "/tmp/wt" in args
    assert "-B" in args and _BDIR in args
    assert any(a.startswith("-DCMAKE_INSTALL_PREFIX=") for a in args)
    assert "-DCMAKE_BUILD_TYPE=Debug" in args


def test_cflags_keys_are_remapped_not_literal():
    args = profiles.cmake_args("/tmp/wt", "debug", profiles.PROFILES["debug"], _BDIR)
    assert any(a.startswith("-DCMAKE_C_FLAGS=") for a in args)
    # the special CFLAGS key must never leak as a literal -DCFLAGS=
    assert not any(a.startswith("-DCFLAGS=") for a in args)


def test_cxx_only_warn_flag_is_in_cxxflags_not_cflags():
    # The C++-only template suppression must reach CMAKE_CXX_FLAGS and must NOT
    # be in CMAKE_C_FLAGS (where GCC's C compiler rejects it).
    for name in ("debug", "optimized"):
        args = profiles.cmake_args("/tmp/wt", name, profiles.PROFILES[name], _BDIR)
        cflags = next(a for a in args if a.startswith("-DCMAKE_C_FLAGS="))
        cxxflags = next(a for a in args if a.startswith("-DCMAKE_CXX_FLAGS="))
        assert "missing-template-arg-list" not in cflags
        assert "missing-template-arg-list" in cxxflags


def test_cmake_args_sets_clang_compiler():
    # clang is required for netdata's Rust plugins (--ld-path=wild is clang-only).
    args = profiles.cmake_args("/tmp/wt", "debug", profiles.PROFILES["debug"], _BDIR)
    assert "-DCMAKE_C_COMPILER=clang" in args
    assert "-DCMAKE_CXX_COMPILER=clang++" in args


def test_both_profiles_disable_heavy_plugins_and_enable_common_ones():
    for name in ("debug", "optimized"):
        p = profiles.PROFILES[name]
        # heavy → off
        for off in ("ENABLE_PLUGIN_NETFLOW", "ENABLE_PLUGIN_GO",
                    "ENABLE_PLUGIN_EBPF", "ENABLE_ML"):
            assert p[off] == "Off", f"{off} should be Off in {name}"
        # curated common → on
        for on in ("ENABLE_PLUGIN_SYSTEMD_JOURNAL", "ENABLE_PLUGIN_LOCAL_LISTENERS",
                   "ENABLE_PLUGIN_DEBUGFS"):
            assert p[on] == "On", f"{on} should be On in {name}"


def test_both_profiles_build_the_otel_plugin():
    # The otel plugin is the deliberate heavy-but-on exception: this tool exists
    # for OTel-logs development, so every build must include it.
    for name in ("debug", "optimized"):
        assert profiles.PROFILES[name]["ENABLE_PLUGIN_OTEL"] == "On"


def test_no_dead_signal_viewer_flag():
    # ENABLE_PLUGIN_OTEL_SIGNAL_VIEWER is no longer a CMake option; it must not
    # be reintroduced into either profile (would draw an unused-variable warning).
    for name in ("debug", "optimized"):
        assert "ENABLE_PLUGIN_OTEL_SIGNAL_VIEWER" not in profiles.PROFILES[name]


def test_profiles_differ_only_by_build_type_and_checks():
    assert profiles.PROFILES["debug"]["CMAKE_BUILD_TYPE"] == "Debug"
    assert profiles.PROFILES["optimized"]["CMAKE_BUILD_TYPE"] == "RelWithDebInfo"


def test_configure_command_rejects_unknown_profile():
    with pytest.raises(ValueError):
        buildcfg.configure_command("/tmp/wt", "nonexistent")


def test_install_prefix_is_per_worktree():
    p = profiles.install_prefix("/home/u/repos/nd")
    assert p.endswith("/nd/netdata")  # one install per worktree, no profile component
