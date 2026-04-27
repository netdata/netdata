use super::*;

macro_rules! presence_field_methods {
    ($(($has:ident, $set:ident, $clear:ident, $field:ident : $ty:ty, $flag:ident, $default:expr)),* $(,)?) => {
        $(
            pub(crate) fn $has(&self) -> bool {
                self.presence.contains(FlowPresence::$flag)
            }

            pub(crate) fn $set(&mut self, value: $ty) {
                self.$field = value;
                self.presence.insert(FlowPresence::$flag);
            }
        )*
    };
}

impl FlowRecord {
    pub(crate) fn swap_presence_flags(&mut self, left: FlowPresence, right: FlowPresence) {
        let left_present = self.presence.contains(left);
        let right_present = self.presence.contains(right);
        self.presence.set(left, right_present);
        self.presence.set(right, left_present);
    }

    presence_field_methods!(
        (
            has_sampling_rate,
            set_sampling_rate,
            clear_sampling_rate,
            sampling_rate: u64,
            SAMPLING_RATE,
            0
        ),
        (has_etype, set_etype, clear_etype, etype: u16, ETYPE, 0),
        (
            has_direction,
            set_direction,
            clear_direction,
            direction: FlowDirection,
            DIRECTION,
            FlowDirection::Undefined
        ),
        (
            has_forwarding_status,
            set_forwarding_status,
            clear_forwarding_status,
            forwarding_status: u8,
            FORWARDING_STATUS,
            0
        ),
        (
            has_in_if_speed,
            set_in_if_speed,
            clear_in_if_speed,
            in_if_speed: u64,
            IN_IF_SPEED,
            0
        ),
        (
            has_out_if_speed,
            set_out_if_speed,
            clear_out_if_speed,
            out_if_speed: u64,
            OUT_IF_SPEED,
            0
        ),
        (
            has_in_if_boundary,
            set_in_if_boundary,
            clear_in_if_boundary,
            in_if_boundary: u8,
            IN_IF_BOUNDARY,
            0
        ),
        (
            has_out_if_boundary,
            set_out_if_boundary,
            clear_out_if_boundary,
            out_if_boundary: u8,
            OUT_IF_BOUNDARY,
            0
        ),
        (has_src_port, set_src_port, clear_src_port, src_port: u16, SRC_PORT, 0),
        (has_dst_port, set_dst_port, clear_dst_port, dst_port: u16, DST_PORT, 0),
        (has_src_vlan, set_src_vlan, clear_src_vlan, src_vlan: u16, SRC_VLAN, 0),
        (has_dst_vlan, set_dst_vlan, clear_dst_vlan, dst_vlan: u16, DST_VLAN, 0),
        (has_iptos, set_iptos, clear_iptos, iptos: u8, IPTOS, 0),
        (
            has_tcp_flags,
            set_tcp_flags,
            clear_tcp_flags,
            tcp_flags: u8,
            TCP_FLAGS,
            0
        ),
        (
            has_icmpv4_type,
            set_icmpv4_type,
            clear_icmpv4_type,
            icmpv4_type: u8,
            ICMPV4_TYPE,
            0
        ),
        (
            has_icmpv4_code,
            set_icmpv4_code,
            clear_icmpv4_code,
            icmpv4_code: u8,
            ICMPV4_CODE,
            0
        ),
        (
            has_icmpv6_type,
            set_icmpv6_type,
            clear_icmpv6_type,
            icmpv6_type: u8,
            ICMPV6_TYPE,
            0
        ),
        (
            has_icmpv6_code,
            set_icmpv6_code,
            clear_icmpv6_code,
            icmpv6_code: u8,
            ICMPV6_CODE,
            0
        ),
    );

    pub(crate) fn clear_direction(&mut self) {
        self.direction = FlowDirection::Undefined;
        self.presence.remove(FlowPresence::DIRECTION);
    }

    pub(crate) fn clear_in_if_speed(&mut self) {
        self.in_if_speed = 0;
        self.presence.remove(FlowPresence::IN_IF_SPEED);
    }

    pub(crate) fn clear_out_if_speed(&mut self) {
        self.out_if_speed = 0;
        self.presence.remove(FlowPresence::OUT_IF_SPEED);
    }

    pub(crate) fn clear_in_if_boundary(&mut self) {
        self.in_if_boundary = 0;
        self.presence.remove(FlowPresence::IN_IF_BOUNDARY);
    }

    pub(crate) fn clear_out_if_boundary(&mut self) {
        self.out_if_boundary = 0;
        self.presence.remove(FlowPresence::OUT_IF_BOUNDARY);
    }
}
