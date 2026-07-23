use crate::ingest::IngestMetrics;
#[cfg(any(target_os = "linux", test))]
use std::collections::HashSet;
#[cfg(target_os = "linux")]
use std::sync::atomic::Ordering;

#[cfg(any(target_os = "linux", test))]
fn parse_udp_socket_drops(contents: &str, listener_inodes: &HashSet<u64>) -> (u64, HashSet<u64>) {
    let mut drops = 0_u64;
    let mut found = HashSet::with_capacity(listener_inodes.len());

    for line in contents.lines().skip(1) {
        let mut columns = line.split_ascii_whitespace();
        let (Some(inode), Some(socket_drops)) = (columns.nth(9), columns.nth(2)) else {
            continue;
        };
        let (Ok(inode), Ok(socket_drops)) = (inode.parse::<u64>(), socket_drops.parse::<u64>())
        else {
            continue;
        };
        if listener_inodes.contains(&inode) && found.insert(inode) {
            drops = drops.saturating_add(socket_drops);
        }
    }

    (drops, found)
}

#[cfg(target_os = "linux")]
pub(super) fn sample_udp_kernel_drops(metrics: &IngestMetrics) {
    let listener_inodes: HashSet<u64> = metrics.udp_listener_socket_inodes().into_iter().collect();
    if listener_inodes.is_empty() {
        return;
    }

    let mut drops = 0_u64;
    let mut found = HashSet::with_capacity(listener_inodes.len());
    for path in ["/proc/net/udp", "/proc/net/udp6"] {
        let Ok(contents) = std::fs::read_to_string(path) else {
            continue;
        };
        let (file_drops, file_found) = parse_udp_socket_drops(&contents, &listener_inodes);
        drops = drops.saturating_add(file_drops);
        found.extend(file_found);
    }

    if found.len() == listener_inodes.len() {
        metrics.udp_kernel_drops.store(drops, Ordering::Relaxed);
    }
}

#[cfg(not(target_os = "linux"))]
pub(super) fn sample_udp_kernel_drops(_metrics: &IngestMetrics) {}

#[cfg(test)]
mod tests {
    use super::*;

    const PROC_NET_UDP: &str = "\
  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops
  11: 0100007F:0801 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 111 2 0000000000000000 7
  12: 0100007F:0802 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 222 2 0000000000000000 9
  13: 0100007F:0803 00000000:0000 07 00000000:00000000 00:00000000 00000000  1000        0 broken 2 0000000000000000 5
malformed
";

    #[test]
    fn parses_only_exact_listener_socket_inodes() {
        let listeners = HashSet::from([111_u64, 333_u64]);
        let (drops, found) = parse_udp_socket_drops(PROC_NET_UDP, &listeners);

        assert_eq!(drops, 7);
        assert_eq!(found, HashSet::from([111]));
    }

    #[test]
    fn does_not_count_one_socket_twice() {
        let duplicated = format!("{PROC_NET_UDP}{}", PROC_NET_UDP.lines().nth(1).unwrap());
        let listeners = HashSet::from([111_u64]);
        let (drops, found) = parse_udp_socket_drops(&duplicated, &listeners);

        assert_eq!(drops, 7);
        assert_eq!(found, HashSet::from([111]));
    }
}
