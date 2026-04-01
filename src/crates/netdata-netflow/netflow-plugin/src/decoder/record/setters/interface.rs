use super::*;

pub(super) fn set_record_interface_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "IN_IF" => {
            let v: u32 = value.parse().unwrap_or(0);
            if rec.in_if == 0 {
                rec.in_if = v;
            }
            true
        }
        "OUT_IF" => {
            let v: u32 = value.parse().unwrap_or(0);
            if rec.out_if == 0 {
                rec.out_if = v;
            }
            true
        }
        "IN_IF_NAME" => {
            rec.in_if_name = value.to_string();
            true
        }
        "OUT_IF_NAME" => {
            rec.out_if_name = value.to_string();
            true
        }
        "IN_IF_DESCRIPTION" => {
            rec.in_if_description = value.to_string();
            true
        }
        "OUT_IF_DESCRIPTION" => {
            rec.out_if_description = value.to_string();
            true
        }
        "IN_IF_SPEED" => {
            rec.set_in_if_speed(value.parse().unwrap_or(0));
            true
        }
        "OUT_IF_SPEED" => {
            rec.set_out_if_speed(value.parse().unwrap_or(0));
            true
        }
        "IN_IF_PROVIDER" => {
            rec.in_if_provider = value.to_string();
            true
        }
        "OUT_IF_PROVIDER" => {
            rec.out_if_provider = value.to_string();
            true
        }
        "IN_IF_CONNECTIVITY" => {
            rec.in_if_connectivity = value.to_string();
            true
        }
        "OUT_IF_CONNECTIVITY" => {
            rec.out_if_connectivity = value.to_string();
            true
        }
        "IN_IF_BOUNDARY" => {
            rec.set_in_if_boundary(value.parse().unwrap_or(0));
            true
        }
        "OUT_IF_BOUNDARY" => {
            rec.set_out_if_boundary(value.parse().unwrap_or(0));
            true
        }
        _ => false,
    }
}
