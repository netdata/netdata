//! Linux: free memory from `/proc/meminfo` and the memory cgroups above this
//! process; a process's memory from `/proc/<pid>/status`.
//!
//! Free memory is the smaller of two numbers. The first is `MemAvailable`,
//! the kernel's own estimate of what new work can take without swapping. In a
//! container that number describes the host, so the second is the room left
//! under the memory limits of the cgroups this process is in and of the
//! cgroups above them: for each one with a limit, the limit minus what the
//! cgroup uses, where the inactive page cache does not count as used (the
//! kernel drops it first). `/proc/self/mountinfo` tells where each cgroup
//! hierarchy is mounted and at which cgroup the mount starts.

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

/// A version of the memory cgroup.
#[derive(Clone, Copy)]
enum Version {
    V1,
    V2,
}

impl Version {
    /// The version of the hierarchy that a line of `/proc/self/cgroup`,
    /// `<id>:<controllers>:<path>`, places this process in. `None` for a v1
    /// hierarchy without the memory controller.
    fn of(id: &str, controllers: &str) -> Option<Self> {
        // The unified hierarchy (v2) has id 0 and no controllers; v1 names
        // `memory` among its controllers.
        if id == "0" && controllers.is_empty() {
            Some(Self::V2)
        } else if controllers.split(',').any(|name| name == "memory") {
            Some(Self::V1)
        } else {
            None
        }
    }

    /// Whether a mount of `filesystem` with these superblock `options` is
    /// this version's memory hierarchy.
    fn is_mount(self, filesystem: &str, options: &str) -> bool {
        match self {
            Self::V1 => filesystem == "cgroup" && options.split(',').any(|name| name == "memory"),
            Self::V2 => filesystem == "cgroup2",
        }
    }

    /// The file of a cgroup that holds its limit.
    fn limit_file(self) -> &'static str {
        match self {
            Self::V1 => "memory.limit_in_bytes",
            Self::V2 => "memory.max",
        }
    }

    /// The file of a cgroup that holds what it and the cgroups below it use.
    fn usage_file(self) -> &'static str {
        match self {
            Self::V1 => "memory.usage_in_bytes",
            Self::V2 => "memory.current",
        }
    }

    /// The line of a cgroup's `memory.stat` that holds the inactive page
    /// cache of it and of the cgroups below it.
    fn inactive_file_field(self) -> &'static str {
        match self {
            Self::V1 => "total_inactive_file",
            Self::V2 => "inactive_file",
        }
    }
}

/// The room left under the memory limits of this process's cgroups, in
/// bytes: the smallest over the cgroups that have a limit. `None` when none
/// has one, or when the cgroups cannot be found.
fn cgroup_room(root: &Path) -> Option<u64> {
    let membership = std::fs::read_to_string(root.join("proc/self/cgroup")).ok()?;
    let mountinfo = std::fs::read_to_string(root.join("proc/self/mountinfo")).ok()?;

    let mut room: Option<u64> = None;
    for line in membership.lines() {
        let mut fields = line.splitn(3, ':');
        let (Some(id), Some(controllers), Some(path)) =
            (fields.next(), fields.next(), fields.next())
        else {
            continue;
        };
        let Some(version) = Version::of(id, controllers) else {
            continue;
        };
        let Some((mount, cgroup)) = cgroup_dir(root, &mountinfo, version, path) else {
            continue;
        };

        if let Some(here) = room_under(version, &mount, cgroup) {
            room = Some(room.map_or(here, |other| other.min(here)));
        }
    }

    room
}

/// Where the cgroup that this process knows as `path` is in the filesystem:
/// the directory of its hierarchy's mount, and its own directory under it.
///
/// A mount need not start at the root of its hierarchy. A container that has
/// no cgroup namespace of its own knows its cgroup by the path it has on the
/// host, and has a mount that starts at that cgroup. So `mountinfo` tells
/// where the mount is and which cgroup it starts at, and the cgroup's
/// directory is the rest of `path` under it. `None` when no mount of the
/// hierarchy shows the cgroup.
fn cgroup_dir(
    root: &Path,
    mountinfo: &str,
    version: Version,
    path: &str,
) -> Option<(PathBuf, PathBuf)> {
    for line in mountinfo.lines() {
        // `<id> <parent> <device> <root> <mount point> <options> ... -
        // <filesystem> <source> <superblock options>`
        let Some((mount, filesystem)) = line.split_once(" - ") else {
            continue;
        };
        let mut filesystem = filesystem.split(' ');
        let (Some(kind), Some(_source), Some(options)) =
            (filesystem.next(), filesystem.next(), filesystem.next())
        else {
            continue;
        };
        if !version.is_mount(kind, options) {
            continue;
        }

        let mut mount = mount.split(' ');
        let (Some(mount_root), Some(mount_point)) = (mount.nth(3), mount.next()) else {
            continue;
        };
        let Ok(below_mount) = Path::new(path).strip_prefix(mount_root) else {
            continue;
        };
        let Ok(mount_point) = Path::new(mount_point).strip_prefix("/") else {
            continue;
        };

        let mount = root.join(mount_point);
        let cgroup = mount.join(below_mount);
        return Some((mount, cgroup));
    }

    None
}

/// The room under the limits of the cgroup at `dir` and of every cgroup
/// above it, up to the `mount` of its hierarchy: for each one with a limit,
/// the limit minus what the cgroup uses without its inactive page cache.
///
/// A v2 cgroup without a limit holds `max`, which is no number, and the root
/// of the hierarchy has no such file. A v1 cgroup without a limit holds a
/// number near `i64::MAX`: its room is then far more than the kernel calls
/// available, and does not decide.
fn room_under(version: Version, mount: &Path, mut dir: PathBuf) -> Option<u64> {
    let mut room: Option<u64> = None;

    loop {
        if let Some(limit) = number_in(&dir, version.limit_file()) {
            let stat = std::fs::read_to_string(dir.join("memory.stat")).unwrap_or_default();
            let inactive_file = stat_field(&stat, version.inactive_file_field()).unwrap_or(0);
            let used = number_in(&dir, version.usage_file())
                .unwrap_or(0)
                .saturating_sub(inactive_file);
            let here = limit.saturating_sub(used);
            room = Some(room.map_or(here, |above| above.min(here)));
        }

        if dir == mount || !dir.pop() {
            return room;
        }
    }
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
        write(
            root.path(),
            "proc/self/mountinfo",
            "30 23 0:26 / /sys/fs/cgroup rw,nosuid,nodev,noexec shared:4 - cgroup2 cgroup2 rw,nsdelegate\n\
             35 30 0:31 / /sys/fs/cgroup/memory rw,nosuid,nodev,noexec shared:9 - cgroup cgroup rw,memory\n",
        );
        root
    }

    /// Write the files of the v1 memory cgroup at `root/cgroup`.
    fn write_v1(root: &Path, cgroup: &str, limit: u64, usage: u64, inactive_file: u64) {
        write(
            root,
            &format!("{cgroup}/memory.limit_in_bytes"),
            &format!("{limit}\n"),
        );
        write(
            root,
            &format!("{cgroup}/memory.usage_in_bytes"),
            &format!("{usage}\n"),
        );
        write(
            root,
            &format!("{cgroup}/memory.stat"),
            &format!(
                "cache 0\ntotal_inactive_file {inactive_file}\nhierarchical_memory_limit {limit}\n"
            ),
        );
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
        write_v1(
            root.path(),
            "sys/fs/cgroup/memory/docker/abc",
            GIB,
            600 * MIB,
            100 * MIB,
        );

        assert_eq!(free_memory_under(root.path()), Some(524 * MIB));
    }

    #[test]
    fn a_v1_limit_of_a_container_is_found_at_its_mount() {
        // A container without a cgroup namespace of its own, as Docker
        // starts them on a v1 host: the process sees the path its cgroup has
        // on the host, and a mount that starts at that cgroup.
        let root = system(
            8 * 1024 * 1024,
            "12:cpu,cpuacct:/docker/abc\n7:memory:/docker/abc\n",
        );
        write(
            root.path(),
            "proc/self/mountinfo",
            "39 35 0:30 /docker/abc /sys/fs/cgroup/cpu,cpuacct ro,nosuid master:8 - cgroup cgroup rw,cpu,cpuacct\n\
             40 35 0:31 /docker/abc /sys/fs/cgroup/memory ro,nosuid master:9 - cgroup cgroup rw,memory\n",
        );
        write_v1(
            root.path(),
            "sys/fs/cgroup/memory",
            GIB,
            600 * MIB,
            100 * MIB,
        );

        assert_eq!(free_memory_under(root.path()), Some(524 * MIB));
    }

    #[test]
    fn the_tightest_of_the_v1_cgroups_above_decides() {
        let root = system(8 * 1024 * 1024, "7:memory:/pod/container\n");
        // The process's own cgroup has the looser limit of the two, and the
        // cgroup above it is used by others too.
        write_v1(
            root.path(),
            "sys/fs/cgroup/memory/pod/container",
            2 * GIB,
            0,
            0,
        );
        write_v1(root.path(), "sys/fs/cgroup/memory/pod", GIB, 400 * MIB, 0);
        // What the kernel says of the limits above a v1 cgroup, in the file
        // of the cgroup itself.
        write(
            root.path(),
            "sys/fs/cgroup/memory/pod/container/memory.stat",
            &format!("total_inactive_file 0\nhierarchical_memory_limit {GIB}\n"),
        );

        assert_eq!(free_memory_under(root.path()), Some(624 * MIB));
    }

    #[test]
    fn a_v1_cgroup_without_a_limit_does_not_count() {
        let root = system(8 * 1024 * 1024, "7:memory:/\n");
        write_v1(
            root.path(),
            "sys/fs/cgroup/memory",
            9_223_372_036_854_771_712,
            123_456,
            0,
        );

        assert_eq!(free_memory_under(root.path()), Some(8 * GIB));
    }

    #[test]
    fn a_cgroup_that_no_mount_shows_has_no_known_limit() {
        let root = system(8 * 1024 * 1024, "0::/pod/container\n");
        // The only mount starts at another cgroup, and a directory with a
        // limit sits where the path would be under it.
        write(
            root.path(),
            "proc/self/mountinfo",
            "30 23 0:26 /other /sys/fs/cgroup rw,nosuid - cgroup2 cgroup2 rw\n",
        );
        write(
            root.path(),
            "sys/fs/cgroup/pod/container/memory.max",
            &format!("{}\n", GIB),
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
