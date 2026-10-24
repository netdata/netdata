use crate::decoder::normalize_direction_value;

pub(crate) fn default_exporter_name(exporter_ip: &str) -> String {
    exporter_ip
        .chars()
        .map(|ch| if ch.is_ascii_alphanumeric() { ch } else { '_' })
        .collect()
}

pub(crate) fn canonical_value<'a>(canonical: &'a str, raw_value: &'a str) -> &'a str {
    if canonical == "DIRECTION" {
        normalize_direction_value(raw_value)
    } else {
        raw_value
    }
}

pub(crate) const USEC_PER_SECOND: u64 = 1_000_000;
pub(crate) const USEC_PER_MILLISECOND: u64 = 1_000;

pub(crate) fn unix_seconds_to_usec(seconds: u64) -> u64 {
    seconds.saturating_mul(USEC_PER_SECOND)
}

pub(crate) fn netflow_v9_system_init_usec(export_time_seconds: u64, sys_uptime_millis: u64) -> u64 {
    unix_seconds_to_usec(export_time_seconds)
        .saturating_sub(sys_uptime_millis.saturating_mul(USEC_PER_MILLISECOND))
}

pub(crate) fn netflow_v9_uptime_millis_to_absolute_usec(
    system_init_usec: u64,
    switched_millis: u64,
) -> u64 {
    system_init_usec.saturating_add(switched_millis.saturating_mul(USEC_PER_MILLISECOND))
}
