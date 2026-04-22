use super::*;

fn set_record_interface_id(slot: &mut u32, value: &str) {
    let parsed = value.parse().unwrap_or(0);

    // Multiple exporter fields collapse into the same canonical interface key.
    // Keep the first non-zero identity observed during normal decode; callers
    // that need to replace it must use override_record_field().
    if *slot == 0 {
        *slot = parsed;
    }
}

pub(super) fn set_record_interface_field(rec: &mut FlowRecord, key: &str, value: &str) -> bool {
    match key {
        "IN_IF" => {
            set_record_interface_id(&mut rec.in_if, value);
            true
        }
        "OUT_IF" => {
            set_record_interface_id(&mut rec.out_if, value);
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
