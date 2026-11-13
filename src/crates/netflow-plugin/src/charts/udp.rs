use crate::ingest::IngestMetrics;
#[cfg(any(target_os = "linux", test))]
use std::collections::{HashMap, HashSet};
#[cfg(target_os = "linux")]
use std::sync::atomic::Ordering;

/// Each listener socket's drop counter in `/proc/net/udp` or `/proc/net/udp6`
/// text, by inode; the first line of an inode wins. The kernel prints a 32-bit
/// counter, so it wraps.
#[cfg(any(target_os = "linux", test))]
pub(crate) fn parse_udp_socket_drops(
    contents: &str,
    listener_inodes: &HashSet<u64>,
) -> HashMap<u64, u32> {
    let mut drops = HashMap::with_capacity(listener_inodes.len());

    for line in contents.lines().skip(1) {
        let mut columns = line.split_ascii_whitespace();
        let (Some(inode), Some(socket_drops)) = (columns.nth(9), columns.nth(2)) else {
            continue;
        };
        let (Ok(inode), Ok(socket_drops)) = (inode.parse::<u64>(), socket_drops.parse::<u32>())
        else {
            continue;
        };
        if listener_inodes.contains(&inode) {
            drops.entry(inode).or_insert(socket_drops);
        }
    }

    drops
}

/// The listener sockets' drop counters as last read. `udp_kernel_drops` grows
/// by what they grow by, so it stays a running total across a wrap of a 32-bit
/// counter and across sockets that come and go.
#[derive(Debug, Default)]
pub(super) struct UdpKernelDrops {
    counters: HashMap<u64, u32>,
}

impl UdpKernelDrops {
    /// What the counters in `current` grew by since they were last read. A
    /// socket read for the first time counts whole: the plugin created it, so
    /// its counter started at 0. Sockets that are no longer listeners are
    /// forgotten; a listener missing from `current` keeps its last counter.
    #[cfg(any(target_os = "linux", test))]
    fn growth(&mut self, current: &HashMap<u64, u32>, listener_inodes: &HashSet<u64>) -> u64 {
        let mut growth = 0_u64;
        for (&inode, &counter) in current {
            let previous = self.counters.insert(inode, counter).unwrap_or(0);
            growth = growth.saturating_add(u64::from(counter.wrapping_sub(previous)));
        }
        self.counters
            .retain(|inode, _| listener_inodes.contains(inode));
        growth
    }
}

#[cfg(target_os = "linux")]
pub(super) fn sample_udp_kernel_drops(metrics: &IngestMetrics, drops: &mut UdpKernelDrops) {
    let listener_inodes: HashSet<u64> = metrics.udp_listener_socket_inodes().into_iter().collect();
    if listener_inodes.is_empty() {
        return;
    }

    let mut current = HashMap::with_capacity(listener_inodes.len());
    for path in ["/proc/net/udp", "/proc/net/udp6"] {
        let Ok(contents) = std::fs::read_to_string(path) else {
            continue;
        };
        current.extend(parse_udp_socket_drops(&contents, &listener_inodes));
    }

    let growth = drops.growth(&current, &listener_inodes);
    metrics
        .udp_kernel_drops
        .fetch_add(growth, Ordering::Relaxed);
}

#[cfg(not(target_os = "linux"))]
pub(super) fn sample_udp_kernel_drops(_metrics: &IngestMetrics, _drops: &mut UdpKernelDrops) {}

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

        assert_eq!(
            parse_udp_socket_drops(PROC_NET_UDP, &listeners),
            HashMap::from([(111, 7)])
        );
    }

    #[test]
    fn does_not_count_one_socket_twice() {
        let duplicated = format!("{PROC_NET_UDP}{}", PROC_NET_UDP.lines().nth(1).unwrap());
        let listeners = HashSet::from([111_u64]);

        assert_eq!(
            parse_udp_socket_drops(&duplicated, &listeners),
            HashMap::from([(111, 7)])
        );
    }

    #[test]
    fn growth_follows_the_counters_across_a_wrap_and_listener_changes() {
        let mut drops = UdpKernelDrops::default();
        let listeners = HashSet::from([111_u64, 222_u64]);

        // First read: whole counters.
        let first = HashMap::from([(111, 7), (222, u32::MAX - 1)]);
        assert_eq!(
            drops.growth(&first, &listeners),
            7 + u64::from(u32::MAX - 1)
        );

        // 222 wraps: 2 to reach 0, then 3 more.
        let wrapped = HashMap::from([(111, 7), (222, 3)]);
        assert_eq!(drops.growth(&wrapped, &listeners), 5);

        // 111 missing from this read keeps its counter for the next.
        let missing = HashMap::from([(222, 3)]);
        assert_eq!(drops.growth(&missing, &listeners), 0);
        let back = HashMap::from([(111, 10), (222, 3)]);
        assert_eq!(drops.growth(&back, &listeners), 3);

        // A new listener socket counts whole; a closed one is forgotten.
        let replaced = HashSet::from([111_u64, 333_u64]);
        let new_socket = HashMap::from([(111, 10), (333, 4)]);
        assert_eq!(drops.growth(&new_socket, &replaced), 4);
        assert_eq!(drops.counters, HashMap::from([(111, 10), (333, 4)]));
    }
}
