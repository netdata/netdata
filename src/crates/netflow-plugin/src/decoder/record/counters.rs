use super::*;

#[derive(Debug, Clone, Copy)]
enum CounterMember {
    IncomingBytes,
    IncomingPackets,
    PostBytes,
    PostPackets,
}

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
struct CounterPair {
    bytes: Option<u64>,
    packets: Option<u64>,
}

impl CounterPair {
    fn observe_bytes(&mut self, value: Option<u64>) {
        if let Some(value) = value.filter(|value| *value > 0) {
            self.bytes = Some(value);
        }
    }

    fn observe_packets(&mut self, value: Option<u64>) {
        if let Some(value) = value.filter(|value| *value > 0) {
            self.packets = Some(value);
        }
    }

    fn is_complete(self) -> bool {
        self.bytes.is_some() && self.packets.is_some()
    }

    fn is_partial(self) -> bool {
        !self.is_complete() && (self.bytes.is_some() || self.packets.is_some())
    }

    fn write_to(self, rec: &mut FlowRecord) {
        rec.bytes = self.bytes.unwrap_or(0);
        rec.packets = self.packets.unwrap_or(0);
        rec.raw_bytes = rec.bytes;
        rec.raw_packets = rec.packets;
    }
}

/// Temporary per-record state for ordinary incoming/post-observation counters.
/// It never becomes part of the stored flow schema.
#[derive(Debug, Default)]
pub(crate) struct OrdinaryCounterSelector {
    incoming: CounterPair,
    post: CounterPair,
    whole_flow_field_present: bool,
    sampled_frame: Option<CounterPair>,
}

impl OrdinaryCounterSelector {
    pub(crate) fn observe_v9(&mut self, field: V9Field, value: &FieldValue) -> bool {
        let member = match field {
            V9Field::InBytes => CounterMember::IncomingBytes,
            V9Field::InPkts => CounterMember::IncomingPackets,
            V9Field::OutBytes => CounterMember::PostBytes,
            V9Field::OutPkts => CounterMember::PostPackets,
            _ => return false,
        };
        self.observe(member, field_value_unsigned(value));
        true
    }

    pub(crate) fn observe_ipfix(&mut self, field: &IPFixField, value: &FieldValue) -> bool {
        let member = match field {
            IPFixField::IANA(IANAIPFixField::OctetDeltaCount) => CounterMember::IncomingBytes,
            IPFixField::IANA(IANAIPFixField::PacketDeltaCount) => CounterMember::IncomingPackets,
            IPFixField::IANA(IANAIPFixField::PostOctetDeltaCount) => CounterMember::PostBytes,
            IPFixField::IANA(IANAIPFixField::PostPacketDeltaCount) => CounterMember::PostPackets,
            _ => return false,
        };
        self.observe(member, field_value_unsigned(value));
        true
    }

    pub(crate) fn observe_sampled_frame(&mut self, bytes: u64) {
        if bytes > 0 {
            self.sampled_frame = Some(CounterPair {
                bytes: Some(bytes),
                packets: Some(1),
            });
        }
    }

    pub(crate) fn observe_other_whole_flow_field(&mut self) {
        self.whole_flow_field_present = true;
    }

    /// Resolve the canonical pair. Returns true only when the selected
    /// whole-flow family is partial.
    pub(crate) fn apply_to_record(&self, rec: &mut FlowRecord) -> bool {
        let selected = if self.incoming.is_complete() {
            Some(self.incoming)
        } else if self.post.is_complete() {
            Some(self.post)
        } else if self.incoming.is_partial() {
            Some(self.incoming)
        } else if self.post.is_partial() {
            Some(self.post)
        } else {
            None
        };

        if let Some(selected) = selected {
            selected.write_to(rec);
            return selected.is_partial();
        }

        if !self.whole_flow_field_present
            && rec.bytes == 0
            && rec.packets == 0
            && let Some(sampled_frame) = self.sampled_frame
        {
            sampled_frame.write_to(rec);
        }

        false
    }

    fn observe(&mut self, member: CounterMember, value: Option<u64>) {
        self.whole_flow_field_present = true;
        match member {
            CounterMember::IncomingBytes => self.incoming.observe_bytes(value),
            CounterMember::IncomingPackets => self.incoming.observe_packets(value),
            CounterMember::PostBytes => self.post.observe_bytes(value),
            CounterMember::PostPackets => self.post.observe_packets(value),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Clone, Copy)]
    enum FamilyState {
        Empty,
        Bytes,
        Packets,
        Complete,
    }

    fn value(value: u64) -> FieldValue {
        FieldValue::DataNumber(DataNumber::U64(value))
    }

    fn observe_family(selector: &mut OrdinaryCounterSelector, incoming: bool, state: FamilyState) {
        let (bytes_field, packets_field, bytes, packets) = if incoming {
            (V9Field::InBytes, V9Field::InPkts, 11, 2)
        } else {
            (V9Field::OutBytes, V9Field::OutPkts, 23, 4)
        };
        match state {
            FamilyState::Empty => {}
            FamilyState::Bytes => {
                selector.observe_v9(bytes_field, &value(bytes));
            }
            FamilyState::Packets => {
                selector.observe_v9(packets_field, &value(packets));
            }
            FamilyState::Complete => {
                selector.observe_v9(bytes_field, &value(bytes));
                selector.observe_v9(packets_field, &value(packets));
            }
        }
    }

    fn expected(incoming: FamilyState, post: FamilyState) -> (u64, u64, bool) {
        use FamilyState::*;
        match (incoming, post) {
            (Complete, _) => (11, 2, false),
            (_, Complete) => (23, 4, false),
            (Bytes, _) => (11, 0, true),
            (Packets, _) => (0, 2, true),
            (Empty, Bytes) => (23, 0, true),
            (Empty, Packets) => (0, 4, true),
            (Empty, Empty) => (0, 0, false),
        }
    }

    #[test]
    fn selection_truth_table_is_family_order_independent() {
        use FamilyState::*;
        let states = [Empty, Bytes, Packets, Complete];

        for incoming in states {
            for post in states {
                let expected = expected(incoming, post);
                for incoming_first in [true, false] {
                    let mut selector = OrdinaryCounterSelector::default();
                    if incoming_first {
                        observe_family(&mut selector, true, incoming);
                        observe_family(&mut selector, false, post);
                    } else {
                        observe_family(&mut selector, false, post);
                        observe_family(&mut selector, true, incoming);
                    }
                    let mut rec = FlowRecord::default();
                    let partial = selector.apply_to_record(&mut rec);
                    assert_eq!(
                        (rec.bytes, rec.packets, partial),
                        expected,
                        "incoming_first={incoming_first}"
                    );
                }
            }
        }
    }

    #[test]
    fn exported_zero_is_unavailable_but_blocks_sampled_frame_fallback() {
        let mut selector = OrdinaryCounterSelector::default();
        selector.observe_v9(V9Field::InBytes, &value(0));
        selector.observe_v9(V9Field::InPkts, &value(0));
        selector.observe_sampled_frame(96);
        let mut rec = FlowRecord::default();

        assert!(!selector.apply_to_record(&mut rec));
        assert_eq!((rec.bytes, rec.packets), (0, 0));
    }

    #[test]
    fn exported_zero_follows_complete_and_partial_precedence() {
        let mut complete_post = OrdinaryCounterSelector::default();
        complete_post.observe_v9(V9Field::InBytes, &value(0));
        complete_post.observe_v9(V9Field::InPkts, &value(0));
        complete_post.observe_v9(V9Field::OutBytes, &value(23));
        complete_post.observe_v9(V9Field::OutPkts, &value(4));
        let mut rec = FlowRecord::default();
        assert!(!complete_post.apply_to_record(&mut rec));
        assert_eq!((rec.bytes, rec.packets), (23, 4));

        let mut partial_incoming = OrdinaryCounterSelector::default();
        partial_incoming.observe_v9(V9Field::InBytes, &value(11));
        partial_incoming.observe_v9(V9Field::InPkts, &value(0));
        let mut rec = FlowRecord::default();
        assert!(partial_incoming.apply_to_record(&mut rec));
        assert_eq!((rec.bytes, rec.packets), (11, 0));

        let mut partial_post = OrdinaryCounterSelector::default();
        partial_post.observe_v9(V9Field::OutBytes, &value(0));
        partial_post.observe_v9(V9Field::OutPkts, &value(4));
        let mut rec = FlowRecord::default();
        assert!(partial_post.apply_to_record(&mut rec));
        assert_eq!((rec.bytes, rec.packets), (0, 4));
    }

    #[test]
    fn partial_whole_flow_family_is_not_filled_from_sampled_frame() {
        let mut selector = OrdinaryCounterSelector::default();
        selector.observe_sampled_frame(96);
        selector.observe_v9(V9Field::InBytes, &value(11));
        let mut rec = FlowRecord::default();

        assert!(selector.apply_to_record(&mut rec));
        assert_eq!((rec.bytes, rec.packets), (11, 0));
        assert_eq!((rec.raw_bytes, rec.raw_packets), (11, 0));
    }

    #[test]
    fn sampled_frame_is_used_only_without_whole_flow_counters() {
        let mut selector = OrdinaryCounterSelector::default();
        selector.observe_sampled_frame(96);
        let mut rec = FlowRecord::default();

        assert!(!selector.apply_to_record(&mut rec));
        assert_eq!((rec.bytes, rec.packets), (96, 1));
        assert_eq!((rec.raw_bytes, rec.raw_packets), (96, 1));
    }

    #[test]
    fn selected_raw_pair_is_copied_before_sampling() {
        let mut selector = OrdinaryCounterSelector::default();
        selector.observe_v9(V9Field::InBytes, &value(11));
        selector.observe_v9(V9Field::InPkts, &value(2));
        let mut rec = FlowRecord::default();
        rec.set_sampling_rate(10);

        assert!(!selector.apply_to_record(&mut rec));
        finalize_record(&mut rec);

        assert_eq!((rec.raw_bytes, rec.raw_packets), (11, 2));
        assert_eq!((rec.bytes, rec.packets), (110, 20));
    }

    #[test]
    fn nsel_and_reverse_counter_families_are_not_captured() {
        let mut selector = OrdinaryCounterSelector::default();
        assert!(!selector.observe_ipfix(
            &IPFixField::IANA(IANAIPFixField::InitiatorOctets),
            &value(11)
        ));
        assert!(!selector.observe_ipfix(
            &IPFixField::IANA(IANAIPFixField::ResponderOctets),
            &value(23)
        ));
        assert!(!selector.observe_ipfix(
            &IPFixField::ReverseInformationElement(
                ReverseInformationElement::ReverseOctetDeltaCount
            ),
            &value(7)
        ));
    }
}
