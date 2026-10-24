use super::super::super::*;

pub(crate) fn sflow_agent_ip_addr(address: &Address) -> Option<IpAddr> {
    match address {
        Address::IPv4(ip) => Some(IpAddr::V4(*ip)),
        Address::IPv6(ip) => Some(IpAddr::V6(*ip)),
        Address::Unknown => None,
    }
}
