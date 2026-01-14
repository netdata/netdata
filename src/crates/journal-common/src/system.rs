//! System utilities for loading machine and boot identifiers.
//!
//! This module provides platform-specific functions to load system identifiers
//! that are used for journal file creation and identification.

use std::io;

/// Reads a file from the host filesystem, trying both the normal path and /host/ prefix.
///
/// This is useful when running in containers where the host filesystem may be mounted at /host.
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

/// Loads the machine ID from the system.
///
/// On Linux, this reads from `/etc/machine-id`.
/// On macOS, this uses `system_profiler` to get the hardware UUID.
/// On other platforms, this returns an error.
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

/// Loads the boot ID from the system.
///
/// On Linux, this reads from `/proc/sys/kernel/random/boot_id`.
/// On macOS, this derives a deterministic ID from the boot time.
/// On other platforms, this returns an error.
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
        // Parse "{ sec = 1753988677, usec = 131097 } Thu Jul 31 22:04:37 2025"
        // Extract sec and usec values
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
                // Create a deterministic UUID from boot time
                // Use sec in first 8 bytes, usec in next 4 bytes, pad remaining with zeros
                let mut bytes = [0u8; 16];
                bytes[0..8].copy_from_slice(&sec.to_be_bytes());
                bytes[8..12].copy_from_slice(&(usec as u32).to_be_bytes());
                // bytes[12..16] remain zero-filled for consistency
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
