"""Build-directory and configure-state helpers.

Transport-free: no MCP imports. A worktree given to this tool is treated as
LLM-dedicated: the tool owns its single ``<worktree>/build/`` tree (the standard
cmake location, which is why clangd finds ``build/compile_commands.json`` with no
help). A `.mcp-managed` marker guards against clobbering a build a user created
themselves.
"""

from __future__ import annotations

from pathlib import Path

from . import profiles

_MARKER = ".mcp-managed"


class BuildDirNotOwned(Exception):
    """``<worktree>/build/`` exists but wasn't created by this tool — refuse to
    clobber it. The worktree must be dedicated to LLM runs (no manual build)."""


def is_worktree(worktree: str) -> bool:
    return Path(worktree, "CMakeLists.txt").is_file()


def validate_profile(profile: str) -> None:
    if profile not in profiles.PROFILES:
        raise ValueError(f"Unknown profile {profile!r}; choose from {', '.join(profiles.PROFILE_NAMES)}")


def build_dir(worktree: str) -> Path:
    """The worktree's single build tree (standard cmake location)."""
    return Path(worktree) / "build"


def _cache_value(worktree: str, key: str) -> str | None:
    """The value of a ``KEY:TYPE=value`` entry in the build dir's cache, or None."""
    cache = build_dir(worktree) / "CMakeCache.txt"
    if not cache.is_file():
        return None
    prefix = f"{key}:"
    for line in cache.read_text(errors="replace").splitlines():
        if line.startswith(prefix):
            parts = line.split("=", 1)
            if len(parts) < 2:
                return None  # malformed (KEY:TYPE with no =value) → treat as absent
            return parts[1].strip() or None
    return None


def _configured_build_type(worktree: str) -> str | None:
    """The CMAKE_BUILD_TYPE recorded in the build dir's cache, or None."""
    return _cache_value(worktree, "CMAKE_BUILD_TYPE")


def _configured_install_prefix(worktree: str) -> str | None:
    """The CMAKE_INSTALL_PREFIX recorded in the build dir's cache, or None."""
    return _cache_value(worktree, "CMAKE_INSTALL_PREFIX")


def needs_configure(worktree: str, profile: str) -> bool:
    """True if the build dir isn't configured, is configured for a different
    profile (build-type switch), or has a stale install prefix — each requires a
    (re)configure.

    The install-prefix check guards a state divergence: the run path always
    launches from ``profiles.install_prefix(worktree)``, but ``ninja install``
    writes to whatever ``CMAKE_INSTALL_PREFIX`` is baked into the cache. A stale
    cached prefix (e.g. from an older tool layout) would install fresh binaries
    to one tree while the agent launches a stale binary from another. cmake
    reconfigures in place, re-baking the correct prefix on the next install.
    """
    validate_profile(profile)
    if not (build_dir(worktree) / "build.ninja").is_file():
        return True
    if _configured_build_type(worktree) != profiles.PROFILES[profile]["CMAKE_BUILD_TYPE"]:
        return True
    cached_prefix = _configured_install_prefix(worktree)
    expected_prefix = profiles.install_prefix(worktree)
    # Missing cache entry → reconfigure to be safe. Compare path-normalized to
    # avoid spurious reconfigures from cosmetic spelling/trailing-slash diffs.
    return cached_prefix is None or Path(cached_prefix).resolve() != Path(expected_prefix).resolve()


def assert_ownable(worktree: str) -> None:
    """Refuse if the build dir looks like a user's own build (configured, no marker)."""
    bdir = build_dir(worktree)
    if (bdir / "CMakeCache.txt").is_file() and not (bdir / _MARKER).is_file():
        raise BuildDirNotOwned(
            f"{bdir} already exists and was not created by this tool. Point the tool at a "
            "worktree dedicated to LLM runs (no manual cmake build), or remove that build dir."
        )


def mark_owned(worktree: str) -> None:
    """Stamp the ownership marker (idempotent). Call BEFORE configure to claim the
    dir, so a failed/killed configure leaves it recoverable (marked), not bricked."""
    bdir = build_dir(worktree)
    bdir.mkdir(parents=True, exist_ok=True)
    (bdir / _MARKER).write_text("managed by netdata-build-mcp\n", encoding="utf-8")


def claim_build_dir(worktree: str) -> None:
    """Verify the build dir is ours (or absent), then claim it. Raises
    BuildDirNotOwned on a foreign build dir. The single entry point every build
    path uses before touching `build/`.

    Called before the build-dir file lock; it's an idempotent ownership stamp,
    not the write-exclusion mechanism (the file lock serialises the actual
    cmake/ninja), so the brief check->stamp window is harmless.
    """
    assert_ownable(worktree)
    mark_owned(worktree)


def configure_command(worktree: str, profile: str) -> list[str]:
    validate_profile(profile)
    return profiles.cmake_args(worktree, profile, profiles.PROFILES[profile], build_dir(worktree))


def build_command(worktree: str) -> list[str]:
    return ["ninja", "-C", str(build_dir(worktree))]


def install_command(worktree: str) -> list[str]:
    # `install` depends on the build target, so this builds-then-installs
    # incrementally (cheap if already built+installed). Needed to run an agent.
    return ["ninja", "-C", str(build_dir(worktree)), "install"]


def lock_key(worktree: str) -> str:
    """Resource key guarding concurrent writes to the worktree's build dir."""
    return str(build_dir(worktree).resolve())


def lock_file(worktree: str) -> Path:
    """Cross-process lockfile for the build dir. A sibling at the worktree root
    (not inside `build/`) so a `rm -rf build/` mid-build can't drop a lock another
    process is holding; the path is gitignored. Resolved (like lock_key) so
    different spellings of the worktree map to one lockfile."""
    return (Path(worktree) / ".netdata-mcp-build.lock").resolve()


def log_path(worktree: str) -> Path:
    """Full build log (overwritten each run); lives in the build dir (gitignored)."""
    return build_dir(worktree) / ".netdata-build.log"


def compile_commands_path(worktree: str) -> Path:
    """The compile DB cmake emits (CMAKE_EXPORT_COMPILE_COMMANDS is on globally).

    At the standard `<worktree>/build/`, clangd discovers it natively — no symlink.
    """
    return build_dir(worktree) / "compile_commands.json"
