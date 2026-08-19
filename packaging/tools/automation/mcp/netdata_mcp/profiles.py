"""Profile definitions mapping profile names to cmake -D options.

Two profiles, differing only in build type/flags; both share one curated plugin
set: the common system-monitoring features ON, the heavy-to-build and rarely-used
ones OFF. The otel plugin is the deliberate exception — kept ON despite its build
cost because this tool exists for OTel-logs development. Each profile is a dict of
CMAKE_CACHE variables, values exactly as they appear in ``-DVAR:TYPE=VALUE``.
Special keys ``CFLAGS``/``CXXFLAGS`` append to ``CMAKE_C_FLAGS``/``CMAKE_CXX_FLAGS``.
"""

from pathlib import Path

# ── curated plugin/feature set (shared by both profiles) ─────────────────────
# Common, useful, cheap-to-build features kept ON. (apps, cgroups, cgroup-network,
# network-viewer, dbengine, dashboard default ON in CMake and are left enabled.)
_PLUGINS_ON = {
    "ENABLE_PLUGIN_SYSTEMD_JOURNAL": "On",
    "ENABLE_PLUGIN_SYSTEMD_UNITS": "On",
    "ENABLE_NETDATA_JOURNAL_FILE_READER": "On",
    "ENABLE_PLUGIN_LOCAL_LISTENERS": "On",  # service auto-discovery
    "ENABLE_PLUGIN_DEBUGFS": "On",          # extra kernel metrics
    # Heavy to build (pulls the Rust/arrow cone), but kept ON: this tool
    # targets OTel-logs development, so the otel plugin must always be present
    # to build/run/verify against.
    "ENABLE_PLUGIN_OTEL": "On",
}

# Disabled: heavy to build (Rust/Go/eBPF) or rarely useful on a local repro box.
_PLUGINS_OFF = {
    # heavy build
    "ENABLE_PLUGIN_NETFLOW": "Off",
    "ENABLE_PLUGIN_GO": "Off",
    "ENABLE_PLUGIN_EBPF": "Off",
    "ENABLE_ML": "Off",
    # rare / niche
    "ENABLE_PLUGIN_PYTHON": "Off",
    "ENABLE_PLUGIN_SCRIPTS": "Off",
    "ENABLE_PLUGIN_CHARTS": "Off",
    "ENABLE_PLUGIN_FREEIPMI": "Off",
    "ENABLE_PLUGIN_CUPS": "Off",
    "ENABLE_PLUGIN_NFACCT": "Off",
    "ENABLE_PLUGIN_PERF": "Off",
    "ENABLE_PLUGIN_SLABINFO": "Off",
    "ENABLE_PLUGIN_XENSTAT": "Off",
    # exporters / optional
    "ENABLE_EXPORTER_MONGODB": "Off",
    "ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE": "Off",
    "ENABLE_MIMALLOC": "Off",
    "ENABLE_WEBRTC": "Off",
    "ENABLE_SENTRY": "Off",
}

# ── warning flags ─────────────────────────────────────────────────────────
# Shared C/C++ flags, plus a C++-only suffix. The template-arg-list warning is a
# GCC 11+ C++ diagnostic; passing it as a C flag makes GCC emit an "unrecognized
# command-line option" note, so it belongs in CXXFLAGS only.
_WARN_FLAGS = "-Wall -Wextra -Wchar-subscripts -fno-omit-frame-pointer"
_CXX_WARN_FLAGS = f"{_WARN_FLAGS} -Wno-missing-template-arg-list-after-template-kw"

_INTERNAL_CHECKS = "-DNETDATA_INTERNAL_CHECKS=1"


PROFILES = {
    "debug": {
        "CMAKE_BUILD_TYPE": "Debug",
        "USE_LTO": "Off",
        "CFLAGS": f"{_WARN_FLAGS} {_INTERNAL_CHECKS}",
        "CXXFLAGS": f"{_CXX_WARN_FLAGS} {_INTERNAL_CHECKS}",
        **_PLUGINS_OFF,
        **_PLUGINS_ON,
    },

    "optimized": {
        "CMAKE_BUILD_TYPE": "RelWithDebInfo",
        "USE_LTO": "Off",
        "CFLAGS": _WARN_FLAGS,
        "CXXFLAGS": _CXX_WARN_FLAGS,
        **_PLUGINS_OFF,
        **_PLUGINS_ON,
    },
}

PROFILE_NAMES = tuple(PROFILES.keys())


def install_prefix(worktree_path: str) -> str:
    """Install prefix for a worktree (one install per worktree, profile-agnostic)."""
    wt_name = Path(worktree_path).resolve().name
    return str(Path.home() / "opt" / "netdata-builds" / wt_name / "netdata")


def cmake_args(worktree_path: str, profile: str, opts: dict[str, str], build_dir: str | Path) -> list[str]:
    """Build the complete cmake command-line from a profile dict.

    ``build_dir`` is the worktree's single build tree; the caller (buildcfg) owns
    its layout.
    """
    args = [
        "cmake",
        "-G", "Ninja",
        "-S", worktree_path,
        "-B", str(build_dir),
        # clang is required: netdata's Rust plugins drive the linker with
        # `--ld-path=wild`, a clang driver flag that gcc/cc rejects.
        "-DCMAKE_C_COMPILER=clang",
        "-DCMAKE_CXX_COMPILER=clang++",
        f"-DCMAKE_INSTALL_PREFIX={install_prefix(worktree_path)}",
    ]
    for k, v in opts.items():
        if k == "CFLAGS":
            args.append(f"-DCMAKE_C_FLAGS={v}")
        elif k == "CXXFLAGS":
            args.append(f"-DCMAKE_CXX_FLAGS={v}")
        else:
            args.append(f"-D{k}={v}")
    return args
