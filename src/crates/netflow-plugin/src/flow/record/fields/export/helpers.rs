use std::net::IpAddr;

/// Format a MAC address as lowercase colon-separated hex.
pub(super) fn format_mac(mac: &[u8; 6]) -> String {
    format!(
        "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
        mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]
    )
}

pub(super) fn opt_ip_to_string(ip: Option<IpAddr>) -> String {
    match ip {
        Some(addr) => addr.to_string(),
        None => String::new(),
    }
}

/// Format prefix as "IP/mask" (CIDR) or just "IP" if mask is 0.
pub(super) fn format_prefix(ip: Option<IpAddr>, mask: u8) -> String {
    match ip {
        Some(addr) if mask > 0 => format!("{}/{}", addr, mask),
        Some(addr) => addr.to_string(),
        None => String::new(),
    }
}
