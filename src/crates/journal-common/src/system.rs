//! Host-identity lookups for the journal stack: machine ID, hostname, and
//! boot ID. Each lookup is a set of `#[cfg]`-selected per-platform variants —
//! Linux, macOS, and a stub returning `ErrorKind::Unsupported` everywhere
//! else — behind one `io::Result`-returning API, so the platform split stays
//! inside this module. All lookups do blocking I/O, including subprocess
//! spawns (`system_profiler`, `sysctl`), and nothing is cached: call once at
//! startup and pass the value around rather than re-querying per entry.
//!
//! Consumers (grep-verified): journal-log-writer imports the functions flat
//! via the crate-root re-export ([`crate::load_machine_id`],
//! [`crate::load_boot_id`]) — machine ID becomes the journal directory name
//! `<path>/<machine-id>` (`journal-log-writer/src/log/mod.rs`
//! `create_chain`) and boot ID resumes the per-boot monotonic tail
//! (`journal-log-writer/src/log/mod.rs` `Log::new`); ng-ingest calls
//! `journal_common::load_machine_id`/`load_boot_id` directly for its
//! file-registry identity (`ng-ingest/src/main.rs` `main` and
//! `ng-ingest/src/bin/traces.rs` `main`). [`crate::load_hostname`] is
//! re-exported at the root but no crate calls it.
//!
//! `src/crates/jf/journal_file/src/file.rs` (`read_host_file`,
//! `load_machine_id`, `load_boot_id`) carries a near-identical copy of the
//! same lookups (same `/host` fallback and macOS parsing, but `[u8; 16]` +
//! `JournalError` instead of `Uuid` + `io::Error`, and the Linux parsers
//! decode hex by hand instead of `Uuid::try_parse`: there the machine-ID
//! parser accepts only the 32-hex undashed form (`hex::decode` rejects
//! hyphens), while the boot-ID parser strips hyphens first); the two must
//! be edited in step by hand.

use std::io;

/// Reads a file from the host filesystem, falling back to `/host/<filename>`.
///
/// Tries `filename` as-is and retries under the `/host` prefix only when the
/// first read is `NotFound` — `/host` is the host-root prefix the Netdata
/// container runs with (`-s /host` in packaging/docker/run.sh). Any other
/// first-read error propagates unchanged and the `/host` path is never
/// tried; if the fallback read also fails, its error propagates as-is.
#[cfg(target_os = "linux")]
fn read_host_file(filename: &str) -> io::Result<String> {
    match std::fs::read_to_string(filename) {
        Ok(contents) => Ok(contents),
        Err(e) if e.kind() == io::ErrorKind::NotFound => {
            let filename = format!("/host/{}", filename);
            std::fs::read_to_string(filename)
        }
        Err(e) => Err(e),
    }
}

/// The host machine ID as a UUID.
///
/// Linux: reads `/etc/machine-id` through `read_host_file`, trims
/// whitespace, and parses it with `Uuid::try_parse`, which accepts systemd's
/// undashed 32-hex form as well as hyphenated UUIDs; unparseable content
/// becomes `ErrorKind::InvalidData`. The machine ID is stable across
/// reboots, unlike the boot ID.
///
/// macOS: spawns `system_profiler SPHardwareDataType` and hex-decodes the
/// stripped "Hardware UUID:" line into the UUID bytes. A wrong-length value,
/// a missing line, or a non-zero exit surfaces as `ErrorKind::NotFound`; a
/// 32-character value that is not hex is `ErrorKind::InvalidData`.
///
/// Other platforms: `ErrorKind::Unsupported`.
#[cfg(target_os = "linux")]
pub fn load_machine_id() -> io::Result<uuid::Uuid> {
    let content = read_host_file("/etc/machine-id")?;
    uuid::Uuid::try_parse(content.trim()).map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
}

#[cfg(target_os = "macos")]
pub fn load_machine_id() -> io::Result<uuid::Uuid> {
    use std::process::Command;

    let output = Command::new("system_profiler")
        .arg("SPHardwareDataType")
        .output()?;

    if output.status.success() {
        let output_str = String::from_utf8_lossy(&output.stdout);
        for line in output_str.lines() {
            if line.contains("Hardware UUID:") {
                if let Some(uuid_str) = line.split("Hardware UUID:").nth(1) {
                    let uuid_str = uuid_str.trim();
                    let hex_str: String = uuid_str.chars().filter(|c| *c != '-').collect();

                    if hex_str.len() == 32 {
                        let mut bytes = [0u8; 16];
                        for i in 0..16 {
                            let hex_pair = &hex_str[i * 2..i * 2 + 2];
                            bytes[i] = u8::from_str_radix(hex_pair, 16)
                                .map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))?;
                        }
                        return Ok(uuid::Uuid::from_bytes(bytes));
                    }
                }
            }
        }
    }

    Err(io::Error::new(
        io::ErrorKind::NotFound,
        "Could not find Hardware UUID",
    ))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
pub fn load_machine_id() -> io::Result<uuid::Uuid> {
    Err(io::Error::new(
        io::ErrorKind::Unsupported,
        "Machine ID loading not supported on this platform",
    ))
}

/// The system hostname.
///
/// Linux and macOS: `nix::unistd::gethostname()`; a failed lookup wraps the
/// `nix` errno as `ErrorKind::Other`, and a hostname that is not valid
/// UTF-8 is rejected as `ErrorKind::InvalidData` (the raw bytes are dropped).
/// This is the only user of the crate's `nix` `hostname` feature (enabled on
/// the `nix` dependency in `Cargo.toml`); currently no crate outside this
/// module calls it.
///
/// Other platforms: `ErrorKind::Unsupported`.
#[cfg(any(target_os = "linux", target_os = "macos"))]
pub fn load_hostname() -> io::Result<String> {
    let hostname =
        nix::unistd::gethostname().map_err(|e| io::Error::new(io::ErrorKind::Other, e))?;
    hostname
        .into_string()
        .map_err(|_| io::Error::new(io::ErrorKind::InvalidData, "hostname is not valid UTF-8"))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
pub fn load_hostname() -> io::Result<String> {
    Err(io::Error::new(
        io::ErrorKind::Unsupported,
        "Hostname loading not supported on this platform",
    ))
}

/// A boot identifier: a UUID that stays the same until the next reboot.
///
/// Linux: reads `/proc/sys/kernel/random/boot_id` — the per-boot UUID
/// assigned by the kernel — directly, without `read_host_file`'s `/host`
/// fallback; whitespace is trimmed and the content parsed with
/// `Uuid::try_parse` (`ErrorKind::InvalidData` if it does not parse).
///
/// macOS: derives the UUID deterministically from the boot time reported by
/// `sysctl -n kern.boottime` (see the byte layout in the function body); any
/// failure to read or parse it — including a non-zero exit — becomes
/// `ErrorKind::NotFound`.
///
/// Other platforms: `ErrorKind::Unsupported`.
#[cfg(target_os = "linux")]
pub fn load_boot_id() -> io::Result<uuid::Uuid> {
    let content = std::fs::read_to_string("/proc/sys/kernel/random/boot_id")?;
    uuid::Uuid::try_parse(content.trim()).map_err(|e| io::Error::new(io::ErrorKind::InvalidData, e))
}

#[cfg(target_os = "macos")]
pub fn load_boot_id() -> io::Result<uuid::Uuid> {
    use std::process::Command;

    let output = Command::new("sysctl")
        .arg("-n")
        .arg("kern.boottime")
        .output()?;

    if output.status.success() {
        let output_str = String::from_utf8_lossy(&output.stdout);
        // Sample output: "{ sec = 1753988677, usec = 131097 } Thu Jul 31 22:04:37 2025"
        // The +6/+7 slice offsets below are the lengths of the "sec = " / "usec = " prefixes.
        if let (Some(sec_start), Some(usec_start)) =
            (output_str.find("sec = "), output_str.find("usec = "))
        {
            let sec_str = &output_str[sec_start + 6..];
            let sec_end = sec_str.find(',').unwrap_or(sec_str.len());
            let sec_str = &sec_str[..sec_end].trim();

            let usec_str = &output_str[usec_start + 7..];
            let usec_end = usec_str.find(' ').unwrap_or(usec_str.len());
            let usec_str = &usec_str[..usec_end].trim();

            if let (Ok(sec), Ok(usec)) = (sec_str.parse::<u64>(), usec_str.parse::<u64>()) {
                // Deterministic UUID from the boot time: big-endian seconds in bytes
                // 0..8, big-endian microseconds in bytes 8..12, zero-padded bytes 12..16.
                let mut bytes = [0u8; 16];
                bytes[0..8].copy_from_slice(&sec.to_be_bytes());
                bytes[8..12].copy_from_slice(&(usec as u32).to_be_bytes());
                // usec is sub-second, so the u32 cast above is lossless for real boottime values.
                return Ok(uuid::Uuid::from_bytes(bytes));
            }
        }
    }

    Err(io::Error::new(
        io::ErrorKind::NotFound,
        "Could not parse boot time",
    ))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
pub fn load_boot_id() -> io::Result<uuid::Uuid> {
    Err(io::Error::new(
        io::ErrorKind::Unsupported,
        "Boot ID loading not supported on this platform",
    ))
}
