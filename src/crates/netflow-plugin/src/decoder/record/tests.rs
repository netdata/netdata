use super::*;
use std::net::{IpAddr, Ipv4Addr, Ipv6Addr};

#[test]
fn swap_directional_record_fields_swaps_directional_flow_record_state() {
    let mut record = FlowRecord {
        src_addr: Some(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1))),
        dst_addr: Some(IpAddr::V6(Ipv6Addr::new(
            0x2001, 0xdb8, 0, 0, 0, 0, 0, 0x20,
        ))),
        src_prefix: Some(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 0))),
        dst_prefix: Some(IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 0))),
        src_mask: 24,
        dst_mask: 64,
        src_port: 12345,
        dst_port: 443,
        src_as: 64512,
        dst_as: 64496,
        src_as_name: "upstream-src".to_string(),
        dst_as_name: "upstream-dst".to_string(),
        src_net_name: "src-net".to_string(),
        dst_net_name: "dst-net".to_string(),
        src_net_role: "src-role".to_string(),
        dst_net_role: "dst-role".to_string(),
        src_net_site: "src-site".to_string(),
        dst_net_site: "dst-site".to_string(),
        src_net_region: "src-region".to_string(),
        dst_net_region: "dst-region".to_string(),
        src_net_tenant: "src-tenant".to_string(),
        dst_net_tenant: "dst-tenant".to_string(),
        src_country: "GR".to_string(),
        dst_country: "US".to_string(),
        src_geo_city: "Athens".to_string(),
        dst_geo_city: "New York".to_string(),
        src_geo_state: "Attica".to_string(),
        dst_geo_state: "NY".to_string(),
        src_geo_latitude: "37.9838".to_string(),
        dst_geo_latitude: "40.7128".to_string(),
        src_geo_longitude: "23.7275".to_string(),
        dst_geo_longitude: "-74.0060".to_string(),
        src_addr_nat: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 5))),
        dst_addr_nat: Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 6))),
        src_port_nat: 40000,
        dst_port_nat: 50000,
        in_if: 10,
        out_if: 20,
        in_if_name: "xe-0/0/0".to_string(),
        out_if_name: "xe-0/0/1".to_string(),
        in_if_description: "ingress uplink".to_string(),
        out_if_description: "egress uplink".to_string(),
        in_if_provider: "src-isp".to_string(),
        out_if_provider: "dst-isp".to_string(),
        in_if_connectivity: "private".to_string(),
        out_if_connectivity: "public".to_string(),
        src_mac: [0, 1, 2, 3, 4, 5],
        dst_mac: [6, 7, 8, 9, 10, 11],
        dst_vlan: 202,
        out_if_speed: 10_000_000_000,
        in_if_boundary: 1,
        ..Default::default()
    };

    record.set_src_vlan(101);
    record.set_in_if_speed(1_000_000_000);
    record.set_out_if_boundary(2);

    swap_directional_record_fields(&mut record);

    assert_eq!(
        record.src_addr,
        Some(IpAddr::V6(Ipv6Addr::new(
            0x2001, 0xdb8, 0, 0, 0, 0, 0, 0x20
        )))
    );
    assert_eq!(
        record.dst_addr,
        Some(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 1)))
    );
    assert_eq!(
        record.src_prefix,
        Some(IpAddr::V6(Ipv6Addr::new(0x2001, 0xdb8, 0, 0, 0, 0, 0, 0)))
    );
    assert_eq!(
        record.dst_prefix,
        Some(IpAddr::V4(Ipv4Addr::new(192, 0, 2, 0)))
    );
    assert_eq!(record.src_mask, 64);
    assert_eq!(record.dst_mask, 24);
    assert_eq!(record.src_port, 443);
    assert_eq!(record.dst_port, 12345);
    assert_eq!(record.src_as, 64496);
    assert_eq!(record.dst_as, 64512);
    assert_eq!(record.src_as_name, "upstream-dst");
    assert_eq!(record.dst_as_name, "upstream-src");
    assert_eq!(record.src_net_name, "dst-net");
    assert_eq!(record.dst_net_name, "src-net");
    assert_eq!(record.src_net_role, "dst-role");
    assert_eq!(record.dst_net_role, "src-role");
    assert_eq!(record.src_net_site, "dst-site");
    assert_eq!(record.dst_net_site, "src-site");
    assert_eq!(record.src_net_region, "dst-region");
    assert_eq!(record.dst_net_region, "src-region");
    assert_eq!(record.src_net_tenant, "dst-tenant");
    assert_eq!(record.dst_net_tenant, "src-tenant");
    assert_eq!(record.src_country, "US");
    assert_eq!(record.dst_country, "GR");
    assert_eq!(record.src_geo_city, "New York");
    assert_eq!(record.dst_geo_city, "Athens");
    assert_eq!(record.src_geo_state, "NY");
    assert_eq!(record.dst_geo_state, "Attica");
    assert_eq!(record.src_geo_latitude, "40.7128");
    assert_eq!(record.dst_geo_latitude, "37.9838");
    assert_eq!(record.src_geo_longitude, "-74.0060");
    assert_eq!(record.dst_geo_longitude, "23.7275");
    assert_eq!(
        record.src_addr_nat,
        Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 6)))
    );
    assert_eq!(
        record.dst_addr_nat,
        Some(IpAddr::V4(Ipv4Addr::new(10, 0, 0, 5)))
    );
    assert_eq!(record.src_port_nat, 50000);
    assert_eq!(record.dst_port_nat, 40000);
    assert_eq!(record.src_vlan, 202);
    assert_eq!(record.dst_vlan, 101);
    assert!(!record.has_src_vlan());
    assert!(record.has_dst_vlan());
    assert_eq!(record.src_mac, [6, 7, 8, 9, 10, 11]);
    assert_eq!(record.dst_mac, [0, 1, 2, 3, 4, 5]);
    assert_eq!(record.in_if, 20);
    assert_eq!(record.out_if, 10);
    assert_eq!(record.in_if_name, "xe-0/0/1");
    assert_eq!(record.out_if_name, "xe-0/0/0");
    assert_eq!(record.in_if_description, "egress uplink");
    assert_eq!(record.out_if_description, "ingress uplink");
    assert_eq!(record.in_if_speed, 10_000_000_000);
    assert_eq!(record.out_if_speed, 1_000_000_000);
    assert!(!record.has_in_if_speed());
    assert!(record.has_out_if_speed());
    assert_eq!(record.in_if_provider, "dst-isp");
    assert_eq!(record.out_if_provider, "src-isp");
    assert_eq!(record.in_if_connectivity, "public");
    assert_eq!(record.out_if_connectivity, "private");
    assert_eq!(record.in_if_boundary, 2);
    assert_eq!(record.out_if_boundary, 1);
    assert!(record.has_in_if_boundary());
    assert!(!record.has_out_if_boundary());
}

#[test]
fn set_record_field_keeps_first_non_zero_interface_ids_until_override() {
    let mut record = FlowRecord::default();

    set_record_field(&mut record, "IN_IF", "17");
    set_record_field(&mut record, "IN_IF", "23");
    set_record_field(&mut record, "OUT_IF", "0");
    set_record_field(&mut record, "OUT_IF", "42");
    override_record_field(&mut record, "OUT_IF", "99");

    assert_eq!(record.in_if, 17);
    assert_eq!(record.out_if, 99);
}
