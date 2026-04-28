use std::net::IpAddr;

/// Parse a MAC address from "xx:xx:xx:xx:xx:xx" string.
pub(crate) fn parse_mac(s: &str) -> [u8; 6] {
    let mut mac = [0u8; 6];
    let parts: Vec<&str> = s.split(':').collect();
    if parts.len() == 6 {
        for (i, part) in parts.iter().enumerate() {
            mac[i] = u8::from_str_radix(part, 16).unwrap_or(0);
        }
    }
    mac
}

/// Parse "IP/mask" or "IP" back to just the IP address.
pub(crate) fn parse_prefix_ip(s: &str) -> Option<IpAddr> {
    if s.is_empty() {
        return None;
    }

    let ip_part = s.split('/').next().unwrap_or(s);
    ip_part.parse::<IpAddr>().ok()
}
