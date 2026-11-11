//! Linux: free memory from `/proc/meminfo` and the memory cgroups above this
//! process; a process's memory from `/proc/<pid>/status`.
//!
//! Free memory is the smaller of two numbers. The first is `MemAvailable`,
//! the kernel's own estimate of what new work can take without swapping. In a
//! container that number describes the host, so the second is the room left
//! under the memory limits of the cgroups this process is in: for each one
//! with a limit, the limit minus what the cgroup uses, where the inactive
//! page cache does not count as used (the kernel drops it first).

use std::path::{Path, PathBuf};

pub(super) fn free_memory() -> Option<u64> {
    free_memory_under(Path::new("/"))
}

pub(super) fn resident_memory(pid: u32) -> Option<u64> {
    let status = std::fs::read_to_string(format!("/proc/{pid}/status")).ok()?;

    kib_field(&status, "VmRSS:")
}

/// [`free_memory`] of the system whose `/proc` and `/sys` are under `root`.
fn free_memory_under(root: &Path) -> Option<u64> {
    let meminfo = std::fs::read_to_string(root.join("proc/meminfo")).ok()?;
    let available = kib_field(&meminfo, "MemAvailable:")?;

    Some(match cgroup_room(root) {
        Some(room) => room.min(available),
        None => available,
    })
}

/// The value of a `Name:   <n> kB` line of `text`, in bytes.
fn kib_field(text: &str, name: &str) -> Option<u64> {
    let kib = text
        .lines()
        .find_map(|line| line.strip_prefix(name))?
        .trim()
        .strip_suffix("kB")?
        .trim();

    Some(kib.parse::<u64>().ok()? * 1024)
}

/// The value of the `name <n>` line of a cgroup's `memory.stat`.
fn stat_field(stat: &str, name: &str) -> Option<u64> {
    stat.lines()
        .find_map(|line| line.strip_prefix(name)?.strip_prefix(' '))?
        .trim()
        .parse()
        .ok()
}

/// The contents of `dir/name` as a number. `None` when the file is absent
/// or holds no number, such as `max`.
fn number_in(dir: &Path, name: &str) -> Option<u64> {
    std::fs::read_to_string(dir.join(name))
        .ok()?
        .trim()
        .parse()
        .ok()
}

/// The room left under the memory limits of this process's cgroups, in
/// bytes: the smallest over the cgroups that have a limit. `None` when none
/// has one.
fn cgroup_room(root: &Path) -> Option<u64> {
    let membership = std::fs::read_to_string(root.join("proc/self/cgroup")).ok()?;

    membership
        .lines()
        .filter_map(|line| {
            // `<id>:<controllers>:<path>`. The unified hierarchy (v2) has id
            // 0 and no controllers; v1 names `memory` among its controllers.
            let mut fields = line.splitn(3, ':');
            let (id, controllers, path) = (fields.next()?, fields.next()?, fields.next()?);
            let path = path.trim_start_matches('/');

            if id == "0" && controllers.is_empty() {
                cgroup_v2_room(&root.join("sys/fs/cgroup"), path)
            } else if controllers.split(',').any(|name| name == "memory") {
                cgroup_v1_room(&root.join("sys/fs/cgroup/memory").join(path))
            } else {
                None
            }
        })
        .min()
}

/// The room under the limits of the v2 cgroup `path` and of every cgroup
/// above it, up to the hierarchy's `mount`.
fn cgroup_v2_room(mount: &Path, path: &str) -> Option<u64> {
    let mut dir: PathBuf = mount.join(path);
    let mut room: Option<u64> = None;

    loop {
        // A cgroup without a limit holds `max`, and the root has no file.
        if let Some(limit) = number_in(&dir, "memory.max") {
            let stat = std::fs::read_to_string(dir.join("memory.stat")).unwrap_or_default();
            let used = number_in(&dir, "memory.current")
                .unwrap_or(0)
                .saturating_sub(stat_field(&stat, "inactive_file").unwrap_or(0));
            let here = limit.saturating_sub(used);
            room = Some(room.map_or(here, |above| above.min(here)));
        }

        if dir == mount || !dir.pop() {
            return room;
        }
    }
}

/// The room under the limit of the v1 memory cgroup at `dir`. Its
/// `hierarchical_memory_limit` already is the smallest limit of the cgroups
/// above it. A cgroup without a limit reports a number near `i64::MAX`
/// there: its room is then far more than the kernel calls available, and
/// does not decide.
fn cgroup_v1_room(dir: &Path) -> Option<u64> {
    let stat = std::fs::read_to_string(dir.join("memory.stat")).ok()?;
    let limit = stat_field(&stat, "hierarchical_memory_limit")?;

    let used = number_in(dir, "memory.usage_in_bytes")?
        .saturating_sub(stat_field(&stat, "total_inactive_file").unwrap_or(0));
    Some(limit.saturating_sub(used))
}

#[cfg(test)]
mod tests {
    use super::*;

    const GIB: u64 = 1 << 30;
    const MIB: u64 = 1 << 20;

    /// Write `contents` to `root/path`, creating the directories above it.
    fn write(root: &Path, path: &str, contents: &str) {
        let file = root.join(path);
        std::fs::create_dir_all(file.parent().unwrap()).unwrap();
        std::fs::write(file, contents).unwrap();
    }

    /// A system root whose `MemAvailable` is `available_kib`.
    fn system(available_kib: u64, membership: &str) -> tempfile::TempDir {
        let root = tempfile::tempdir().unwrap();
        write(
            root.path(),
            "proc/meminfo",
            &format!(
                "MemTotal:       32801264 kB\nMemFree:          600000 kB\nMemAvailable:   {available_kib} kB\n"
            ),
        );
        write(root.path(), "proc/self/cgroup", membership);
        root
    }

    #[test]
    fn without_a_cgroup_limit_free_memory_is_what_the_kernel_calls_available() {
        let root = system(8 * 1024 * 1024, "0::/user.slice/app.scope\n");
        write(
            root.path(),
            "sys/fs/cgroup/user.slice/app.scope/memory.max",
            "max\n",
        );
        write(root.path(), "sys/fs/cgroup/user.slice/memory.max", "max\n");

        assert_eq!(free_memory_under(root.path()), Some(8 * GIB));
    }

    #[test]
    fn a_v2_limit_leaves_the_room_under_it_without_inactive_page_cache() {
        let root = system(8 * 1024 * 1024, "0::/pod/container\n");
        let cgroup = "sys/fs/cgroup/pod/container";
        write(
            root.path(),
            &format!("{cgroup}/memory.max"),
            &format!("{}\n", 2 * GIB),
        );
        write(
            root.path(),
            &format!("{cgroup}/memory.current"),
            &format!("{}\n", GIB),
        );
        write(
            root.path(),
            &format!("{cgroup}/memory.stat"),
            &format!(
                "anon {}\nfile {}\ninactive_file {}\nactive_file 0\n",
                700 * MIB,
                324 * MIB,
                300 * MIB
            ),
        );

        // 2 GiB - (1 GiB - 300 MiB).
        assert_eq!(free_memory_under(root.path()), Some(GIB + 300 * MIB));
    }

    #[test]
    fn the_tightest_of_the_v2_cgroups_above_decides() {
        let root = system(8 * 1024 * 1024, "0::/pod/container\n");
        // The process's own cgroup has the looser limit of the two.
        write(
            root.path(),
            "sys/fs/cgroup/pod/container/memory.max",
            &format!("{}\n", 2 * GIB),
        );
        write(
            root.path(),
            "sys/fs/cgroup/pod/container/memory.current",
            "0\n",
        );
        write(
            root.path(),
            "sys/fs/cgroup/pod/memory.max",
            &format!("{}\n", GIB),
        );
        write(
            root.path(),
            "sys/fs/cgroup/pod/memory.current",
            &format!("{}\n", 400 * MIB),
        );

        assert_eq!(free_memory_under(root.path()), Some(624 * MIB));
    }

    #[test]
    fn a_limit_on_the_root_of_a_cgroup_namespace_counts() {
        // Inside a container the process sees its cgroup as the root.
        let root = system(8 * 1024 * 1024, "0::/\n");
        write(
            root.path(),
            "sys/fs/cgroup/memory.max",
            &format!("{}\n", GIB),
        );
        write(
            root.path(),
            "sys/fs/cgroup/memory.current",
            &format!("{}\n", 100 * MIB),
        );

        assert_eq!(free_memory_under(root.path()), Some(924 * MIB));
    }

    #[test]
    fn the_kernels_number_wins_when_it_is_the_smaller() {
        let root = system(512 * 1024, "0::/pod\n");
        write(
            root.path(),
            "sys/fs/cgroup/pod/memory.max",
            &format!("{}\n", 4 * GIB),
        );
        write(root.path(), "sys/fs/cgroup/pod/memory.current", "0\n");

        assert_eq!(free_memory_under(root.path()), Some(512 * MIB));
    }

    #[test]
    fn a_v1_limit_leaves_the_room_under_it() {
        let root = system(
            8 * 1024 * 1024,
            "12:cpu,cpuacct:/docker/abc\n7:memory:/docker/abc\n1:name=systemd:/docker/abc\n",
        );
        let cgroup = "sys/fs/cgroup/memory/docker/abc";
        write(
            root.path(),
            &format!("{cgroup}/memory.stat"),
            &format!(
                "cache 0\ntotal_inactive_file {}\nhierarchical_memory_limit {}\n",
                100 * MIB,
                GIB
            ),
        );
        write(
            root.path(),
            &format!("{cgroup}/memory.usage_in_bytes"),
            &format!("{}\n", 600 * MIB),
        );

        assert_eq!(free_memory_under(root.path()), Some(524 * MIB));
    }

    #[test]
    fn a_v1_cgroup_without_a_limit_does_not_count() {
        let root = system(8 * 1024 * 1024, "7:memory:/\n");
        write(
            root.path(),
            "sys/fs/cgroup/memory/memory.stat",
            "hierarchical_memory_limit 9223372036854771712\n",
        );
        write(
            root.path(),
            "sys/fs/cgroup/memory/memory.usage_in_bytes",
            "123456\n",
        );

        assert_eq!(free_memory_under(root.path()), Some(8 * GIB));
    }

    #[test]
    fn free_memory_is_unknown_without_meminfo() {
        let root = tempfile::tempdir().unwrap();

        assert_eq!(free_memory_under(root.path()), None);
    }

    #[test]
    fn this_process_has_memory_and_the_system_has_some_free() {
        assert!(resident_memory(std::process::id()).is_some_and(|bytes| bytes > MIB));
        assert!(free_memory().is_some_and(|bytes| bytes > MIB));
        assert_eq!(resident_memory(u32::MAX), None);
    }
}
