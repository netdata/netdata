use super::*;

fn direction_to_u8(direction: FlowDirection) -> u8 {
    match direction {
        FlowDirection::Ingress => 0,
        FlowDirection::Egress => 1,
        FlowDirection::Undefined => 2,
    }
}

pub(crate) fn direction_from_u8(value: u8) -> FlowDirection {
    match value {
        0 => FlowDirection::Ingress,
        1 => FlowDirection::Egress,
        _ => FlowDirection::Undefined,
    }
}

pub(crate) fn direction_as_rollup_value(direction: FlowDirection) -> u8 {
    direction_to_u8(direction)
}
