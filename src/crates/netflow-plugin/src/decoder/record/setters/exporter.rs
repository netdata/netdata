use super::*;

pub(super) fn set_record_exporter_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "FLOW_VERSION" => true,
        "EXPORTER_IP" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.exporter_ip = Some(canonicalize_ip_addr(ip));
            }
            true
        }
        "EXPORTER_PORT" => {
            rec.exporter_port = value.parse().unwrap_or(rec.exporter_port);
            true
        }
        "EXPORTER_NAME" => {
            rec.exporter_name = value.to_string();
            true
        }
        "EXPORTER_GROUP" => {
            rec.exporter_group = value.to_string();
            true
        }
        "EXPORTER_ROLE" => {
            rec.exporter_role = value.to_string();
            true
        }
        "EXPORTER_SITE" => {
            rec.exporter_site = value.to_string();
            true
        }
        "EXPORTER_REGION" => {
            rec.exporter_region = value.to_string();
            true
        }
        "EXPORTER_TENANT" => {
            rec.exporter_tenant = value.to_string();
            true
        }
        "SAMPLING_RATE" => {
            rec.set_sampling_rate(value.parse().unwrap_or(0));
            true
        }
        "ETYPE" => {
            rec.set_etype(value.parse().unwrap_or(0));
            true
        }
        "FORWARDING_STATUS" => {
            rec.set_forwarding_status(value.parse().unwrap_or(0));
            true
        }
        "DIRECTION" => {
            let direction = FlowDirection::from_str_value(value);
            if direction == FlowDirection::Undefined {
                rec.clear_direction();
            } else {
                rec.set_direction(direction);
            }
            true
        }
        _ => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn direction_setter_clears_unrecognized_values() {
        let mut rec = FlowRecord::default();
        rec.set_direction(FlowDirection::Egress);

        assert!(set_record_exporter_field(&mut rec, "DIRECTION", ""));
        assert_eq!(rec.direction, FlowDirection::Undefined);
        assert!(!rec.has_direction());
    }

    #[test]
    fn direction_setter_keeps_numeric_direction_values() {
        let mut rec = FlowRecord::default();

        assert!(set_record_exporter_field(&mut rec, "DIRECTION", "0"));
        assert_eq!(rec.direction, FlowDirection::Ingress);
        assert!(rec.has_direction());
    }
}
