use super::*;

pub(super) fn set_record_exporter_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "FLOW_VERSION" => true,
        "EXPORTER_IP" => {
            if let Ok(ip) = value.parse::<IpAddr>() {
                rec.exporter_ip = Some(ip);
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
            let normalized = FlowDirection::from_str_value(value);
            if value == DIRECTION_UNDEFINED {
                rec.clear_direction();
            } else {
                rec.set_direction(normalized);
            }
            true
        }
        _ => false,
    }
}
