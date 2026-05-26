use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;
use std::time::Duration;

#[derive(Debug, Clone, Copy)]
pub(super) struct ProcSnapshot {
    pub(super) rss_bytes: u64,
    pub(super) rss_anon_bytes: u64,
    pub(super) rss_file_bytes: u64,
    pub(super) read_bytes: u64,
    pub(super) write_bytes: u64,
    pub(super) user_ticks: u64,
    pub(super) system_ticks: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(super) struct ResourceEnvelopeReport {
    pub(super) methodology: String,
    pub(super) layer: String,
    pub(super) profile: String,
    pub(super) protocol: String,
    pub(super) requested_flows_per_sec: u64,
    pub(super) achieved_flows_per_sec: f64,
    pub(super) cpu_percent_of_one_core: f64,
    pub(super) logical_write_bytes_per_sec: f64,
    pub(super) logical_entries_per_sec: f64,
    pub(super) read_bytes_per_sec: f64,
    pub(super) write_bytes_per_sec: f64,
    pub(super) final_rss_bytes: u64,
    pub(super) peak_rss_bytes: u64,
    pub(super) peak_rss_anon_bytes: u64,
    pub(super) peak_rss_file_bytes: u64,
    pub(super) warmup_secs: u64,
    pub(super) measurement_secs: u64,
    pub(super) record_pool_size: usize,
}

pub(super) fn take_proc_snapshot() -> ProcSnapshot {
    let status = fs::read_to_string("/proc/self/status")
        .unwrap_or_else(|err| panic!("read /proc/self/status: {err}"));
    let io = fs::read_to_string("/proc/self/io")
        .unwrap_or_else(|err| panic!("read /proc/self/io: {err}"));
    let stat = fs::read_to_string("/proc/self/stat")
        .unwrap_or_else(|err| panic!("read /proc/self/stat: {err}"));

    ProcSnapshot {
        rss_bytes: parse_status_kib(&status, "VmRSS") * 1024,
        rss_anon_bytes: parse_status_kib(&status, "RssAnon") * 1024,
        rss_file_bytes: parse_status_kib(&status, "RssFile") * 1024,
        read_bytes: parse_io_value(&io, "read_bytes"),
        write_bytes: parse_io_value(&io, "write_bytes"),
        user_ticks: parse_stat_ticks(&stat, 14),
        system_ticks: parse_stat_ticks(&stat, 15),
    }
}

pub(super) fn cpu_percent_of_one_core(
    start: ProcSnapshot,
    end: ProcSnapshot,
    elapsed: Duration,
) -> f64 {
    let cpu_ticks = end
        .user_ticks
        .saturating_add(end.system_ticks)
        .saturating_sub(start.user_ticks.saturating_add(start.system_ticks));
    let ticks_per_second = proc_ticks_per_second() as f64;
    let cpu_seconds = cpu_ticks as f64 / ticks_per_second;
    (cpu_seconds / elapsed.as_secs_f64()) * 100.0
}

pub(super) fn print_resource_report(report: &ResourceEnvelopeReport) {
    eprintln!();
    eprintln!("Layer:                  {}", report.layer);
    eprintln!("Profile:                {}", report.profile);
    eprintln!("Protocol:               {}", report.protocol);
    eprintln!(
        "  offered load:         {} flows/s",
        report.requested_flows_per_sec
    );
    eprintln!(
        "  achieved load:        {:.0} flows/s",
        report.achieved_flows_per_sec
    );
    eprintln!(
        "  CPU:                  {:.1}% of one core",
        report.cpu_percent_of_one_core
    );
    eprintln!(
        "  memory:               peak {:.2} MiB | final {:.2} MiB",
        bytes_to_mib(report.peak_rss_bytes),
        bytes_to_mib(report.final_rss_bytes)
    );
    eprintln!(
        "  memory split:         peak anon {:.2} MiB | peak file {:.2} MiB",
        bytes_to_mib(report.peak_rss_anon_bytes),
        bytes_to_mib(report.peak_rss_file_bytes)
    );
    eprintln!(
        "  disk I/O:             read {:.0} KiB/s | write {:.0} KiB/s",
        report.read_bytes_per_sec / 1024.0,
        report.write_bytes_per_sec / 1024.0
    );
    eprintln!(
        "  logical journal I/O:  {:.0} KiB/s | {:.0} entries/s",
        report.logical_write_bytes_per_sec / 1024.0,
        report.logical_entries_per_sec
    );
    eprintln!(
        "  duration:             warmup {}s | measure {}s | pool {}",
        report.warmup_secs, report.measurement_secs, report.record_pool_size
    );
}

pub(super) fn parse_child_report(output: &std::process::Output) -> ResourceEnvelopeReport {
    let stdout = String::from_utf8_lossy(&output.stdout);
    let stderr = String::from_utf8_lossy(&output.stderr);
    let combined = format!("{stdout}\n{stderr}");
    let json = combined
        .lines()
        .find_map(|line| {
            line.split_once("RESOURCE_BENCH_RESULT:")
                .map(|(_, json)| json)
        })
        .unwrap_or_else(|| panic!("resource bench child did not emit result\n{combined}"));
    serde_json::from_str(json)
        .unwrap_or_else(|err| panic!("parse resource bench result JSON: {err}\n{combined}"))
}

fn parse_status_kib(status: &str, key: &str) -> u64 {
    for line in status.lines() {
        if let Some(value) = line.strip_prefix(key) {
            return value
                .split_whitespace()
                .find_map(|part| part.parse::<u64>().ok())
                .unwrap_or(0);
        }
    }
    0
}

fn parse_io_value(io: &str, key: &str) -> u64 {
    for line in io.lines() {
        if let Some(value) = line.strip_prefix(&format!("{key}:")) {
            return value.trim().parse::<u64>().unwrap_or(0);
        }
    }
    0
}

fn parse_stat_ticks(stat: &str, field_index: usize) -> u64 {
    let after_comm = stat
        .rsplit_once(") ")
        .map(|(_, rest)| rest)
        .unwrap_or_else(|| panic!("unexpected /proc/self/stat format"));
    let fields: Vec<&str> = after_comm.split_whitespace().collect();
    fields
        .get(field_index.saturating_sub(3))
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
}

fn proc_ticks_per_second() -> i64 {
    let ticks = unsafe { libc::sysconf(libc::_SC_CLK_TCK) };
    if ticks <= 0 {
        panic!("sysconf(_SC_CLK_TCK) returned invalid value: {ticks}");
    }
    ticks
}

fn bytes_to_mib(bytes: u64) -> f64 {
    bytes as f64 / (1024.0 * 1024.0)
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(super) struct StorageFootprintSample {
    pub(super) elapsed_secs: u64,
    pub(super) raw_dir_bytes: u64,
    pub(super) minute_1_dir_bytes: u64,
    pub(super) minute_5_dir_bytes: u64,
    pub(super) hour_1_dir_bytes: u64,
    pub(super) total_disk_bytes: u64,
    pub(super) cumulative_io_write_bytes: u64,
    pub(super) cumulative_logical_bytes: u64,
    pub(super) cumulative_flows_ingested: u64,
    pub(super) rss_bytes: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(super) struct StorageFootprintReport {
    pub(super) protocol: String,
    pub(super) profile: String,
    pub(super) flows_per_sec: u64,
    pub(super) duration_secs: u64,
    pub(super) sample_interval_secs: u64,
    pub(super) samples: Vec<StorageFootprintSample>,
    pub(super) final_total_flows: u64,
    pub(super) final_disk_bytes: u64,
    pub(super) final_logical_bytes: u64,
    pub(super) final_io_write_bytes: u64,
}

#[allow(dead_code)]
pub(super) fn parse_storage_child_report(output: &std::process::Output) -> StorageFootprintReport {
    let stdout = String::from_utf8_lossy(&output.stdout);
    let stderr = String::from_utf8_lossy(&output.stderr);
    let combined = format!("{stdout}\n{stderr}");
    let json = combined
        .lines()
        .find_map(|line| {
            line.split_once("STORAGE_BENCH_RESULT:")
                .map(|(_, json)| json)
        })
        .unwrap_or_else(|| panic!("storage bench child did not emit result\n{combined}"));
    serde_json::from_str(json)
        .unwrap_or_else(|err| panic!("parse storage bench result JSON: {err}\n{combined}"))
}

pub(super) fn journal_dir_size_bytes(path: &Path) -> u64 {
    let mut total = 0_u64;
    if let Ok(entries) = fs::read_dir(path) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_dir() {
                total = total.saturating_add(journal_dir_size_bytes(&path));
            } else if let Ok(meta) = entry.metadata() {
                total = total.saturating_add(meta.len());
            }
        }
    }
    total
}
