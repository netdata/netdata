use super::super::*;

const NSEL_FLOW_ID: u16 = 148;
const NSEL_INITIATOR_OCTETS: u16 = 231;
const NSEL_RESPONDER_OCTETS: u16 = 232;
const NSEL_FIREWALL_EVENT: u16 = 233;
const NSEL_INITIATOR_PACKETS: u16 = 298;
const NSEL_RESPONDER_PACKETS: u16 = 299;
const NSEL_EXTENDED_FIREWALL_EVENT: u16 = 33_002;
const NSEL_LEGACY_FIREWALL_EVENT: u16 = 40_005;

#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub(crate) struct NselDecodeStats {
    pub(crate) records: u64,
    pub(crate) update_records: u64,
    pub(crate) create_records: u64,
    pub(crate) teardown_records: u64,
    pub(crate) denied_records: u64,
    pub(crate) unsupported_event_records: u64,
    pub(crate) malformed_records: u64,
    pub(crate) counterless_update_records: u64,
    pub(crate) partial_counter_records: u64,
    pub(crate) zero_responder_records: u64,
    pub(crate) forward_rows: u64,
    pub(crate) reverse_rows: u64,
}

impl NselDecodeStats {
    pub(crate) fn merge_into(self, stats: &mut DecodeStats) {
        stats.nsel_records = stats.nsel_records.saturating_add(self.records);
        stats.nsel_update_records = stats
            .nsel_update_records
            .saturating_add(self.update_records);
        stats.nsel_create_records = stats
            .nsel_create_records
            .saturating_add(self.create_records);
        stats.nsel_teardown_records = stats
            .nsel_teardown_records
            .saturating_add(self.teardown_records);
        stats.nsel_denied_records = stats
            .nsel_denied_records
            .saturating_add(self.denied_records);
        stats.nsel_unsupported_event_records = stats
            .nsel_unsupported_event_records
            .saturating_add(self.unsupported_event_records);
        stats.nsel_malformed_records = stats
            .nsel_malformed_records
            .saturating_add(self.malformed_records);
        stats.nsel_counterless_update_records = stats
            .nsel_counterless_update_records
            .saturating_add(self.counterless_update_records);
        stats.nsel_partial_counter_records = stats
            .nsel_partial_counter_records
            .saturating_add(self.partial_counter_records);
        stats.nsel_zero_responder_records = stats
            .nsel_zero_responder_records
            .saturating_add(self.zero_responder_records);
        stats.nsel_forward_rows = stats.nsel_forward_rows.saturating_add(self.forward_rows);
        stats.nsel_reverse_rows = stats.nsel_reverse_rows.saturating_add(self.reverse_rows);
    }
}

#[derive(Debug, Clone, Copy, Default)]
struct SingletonU64 {
    present: bool,
    value: Option<u64>,
    malformed: bool,
}

impl SingletonU64 {
    fn observe(&mut self, value: Option<u64>) {
        self.present = true;
        let Some(value) = value else {
            self.malformed = true;
            return;
        };
        if self.value.is_some_and(|current| current != value) {
            self.malformed = true;
            return;
        }
        self.value = Some(value);
    }
}

#[derive(Debug, Clone, Copy, Default)]
struct DirectionalCounters {
    bytes: SingletonU64,
    packets: SingletonU64,
}

impl DirectionalCounters {
    fn present(self) -> bool {
        self.bytes.present || self.packets.present
    }

    fn partial(self) -> bool {
        self.bytes.present != self.packets.present
    }

    fn malformed(self) -> bool {
        self.bytes.malformed || self.packets.malformed
    }

    fn values(self) -> (u64, u64) {
        (
            self.bytes.value.unwrap_or(0),
            self.packets.value.unwrap_or(0),
        )
    }
}

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct NselRecordState {
    event: SingletonU64,
    flow_id: SingletonU64,
    initiator: DirectionalCounters,
    responder: DirectionalCounters,
}

impl NselRecordState {
    pub(crate) fn observe(&mut self, field: V9Field, value: &FieldValue) -> bool {
        let field_id = u16::from(field);
        match field_id {
            NSEL_FLOW_ID => self.flow_id.observe(field_value_unsigned(value)),
            NSEL_INITIATOR_OCTETS => {
                self.initiator.bytes.observe(field_value_unsigned(value));
            }
            NSEL_RESPONDER_OCTETS => {
                self.responder.bytes.observe(field_value_unsigned(value));
            }
            NSEL_FIREWALL_EVENT => self.event.observe(firewall_event_value(value)),
            NSEL_INITIATOR_PACKETS => {
                self.initiator.packets.observe(field_value_unsigned(value));
            }
            NSEL_RESPONDER_PACKETS => {
                self.responder.packets.observe(field_value_unsigned(value));
            }
            NSEL_LEGACY_FIREWALL_EVENT => {
                self.event.observe(field_value_unsigned(value));
            }
            NSEL_EXTENDED_FIREWALL_EVENT => {}
            _ => return false,
        }
        true
    }

    fn malformed(self) -> bool {
        !self.event.present
            || self.event.malformed
            || self.flow_id.malformed
            || self.initiator.malformed()
            || self.responder.malformed()
    }
}

pub(crate) struct NselProjection {
    pub(crate) forward: Option<DecodedFlow>,
    pub(crate) reverse: Option<DecodedFlow>,
    pub(crate) stats: NselDecodeStats,
}

pub(crate) fn is_nsel_template(fields: &[PersistedV9TemplateField]) -> bool {
    let mut modern_event = false;
    let mut legacy_event = false;
    let mut extended_event = false;
    for field in fields {
        match field.field_type {
            NSEL_FIREWALL_EVENT => modern_event = true,
            NSEL_LEGACY_FIREWALL_EVENT => legacy_event = true,
            NSEL_EXTENDED_FIREWALL_EVENT => extended_event = true,
            _ => {}
        }
    }
    extended_event && (modern_event || legacy_event)
}

pub(crate) fn project_nsel_record(
    mut record: FlowRecord,
    state: NselRecordState,
    input_realtime_usec: u64,
) -> NselProjection {
    let mut stats = NselDecodeStats {
        records: 1,
        ..Default::default()
    };
    if state.malformed() {
        stats.malformed_records = 1;
        return empty_projection(stats);
    }

    match state.event.value {
        Some(1) => {
            stats.create_records = 1;
            return empty_projection(stats);
        }
        Some(2) => {
            stats.teardown_records = 1;
            return empty_projection(stats);
        }
        Some(3) => {
            stats.denied_records = 1;
            return empty_projection(stats);
        }
        Some(5) => stats.update_records = 1,
        Some(_) | None => {
            stats.unsupported_event_records = 1;
            return empty_projection(stats);
        }
    }

    if !state.initiator.present() && !state.responder.present() {
        stats.counterless_update_records = 1;
        return empty_projection(stats);
    }
    stats.partial_counter_records =
        u64::from(state.initiator.partial()) + u64::from(state.responder.partial());

    // ASA NSEL update counters are already the interval contributions. They
    // must not inherit a sampling rate learned from unrelated v9 options.
    record.sampling_rate = 1;
    let forward = state.initiator.present().then(|| {
        let (bytes, packets) = state.initiator.values();
        set_nsel_counters(&mut record, bytes, packets);
        finalize_record(&mut record);
        stats.forward_rows = 1;
        DecodedFlow {
            record: record.clone(),
            source_realtime_usec: Some(input_realtime_usec),
        }
    });

    let reverse = state.responder.present().then(|| {
        let (bytes, packets) = state.responder.values();
        if bytes == 0 && packets == 0 {
            stats.zero_responder_records = 1;
            return None;
        }
        let mut reverse = record;
        swap_directional_record_fields(&mut reverse);
        set_nsel_counters(&mut reverse, bytes, packets);
        finalize_record(&mut reverse);
        stats.reverse_rows = 1;
        Some(DecodedFlow {
            record: reverse,
            source_realtime_usec: Some(input_realtime_usec),
        })
    });

    NselProjection {
        forward,
        reverse: reverse.flatten(),
        stats,
    }
}

fn empty_projection(stats: NselDecodeStats) -> NselProjection {
    NselProjection {
        forward: None,
        reverse: None,
        stats,
    }
}

fn firewall_event_value(value: &FieldValue) -> Option<u64> {
    match value {
        FieldValue::FirewallEvent(event) => Some(u64::from(u8::from(*event))),
        _ => field_value_unsigned(value),
    }
}

fn set_nsel_counters(record: &mut FlowRecord, bytes: u64, packets: u64) {
    record.bytes = bytes;
    record.packets = packets;
    record.raw_bytes = bytes;
    record.raw_packets = packets;
}

#[cfg(test)]
mod tests {
    use super::*;

    fn field(field_type: u16) -> PersistedV9TemplateField {
        PersistedV9TemplateField {
            field_type,
            field_length: 1,
        }
    }

    #[test]
    fn template_signature_requires_event_and_cisco_extension() {
        assert!(is_nsel_template(&[
            field(NSEL_FIREWALL_EVENT),
            field(NSEL_EXTENDED_FIREWALL_EVENT),
        ]));
        assert!(is_nsel_template(&[
            field(NSEL_LEGACY_FIREWALL_EVENT),
            field(NSEL_EXTENDED_FIREWALL_EVENT),
        ]));
        assert!(!is_nsel_template(&[field(NSEL_FIREWALL_EVENT)]));
        assert!(!is_nsel_template(&[field(NSEL_EXTENDED_FIREWALL_EVENT)]));
    }
}
