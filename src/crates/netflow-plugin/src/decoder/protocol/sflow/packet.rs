use super::*;

pub(crate) fn extract_sflow_flows(
    source: SocketAddr,
    datagram: SFlowDatagram,
    decapsulation_mode: DecapsulationMode,
    timestamp_source: TimestampSource,
    input_realtime_usec: u64,
    emit_rows: bool,
    stats: &mut DecodeStats,
) -> Vec<DecodedFlow> {
    let exporter_ip_override = sflow_agent_ip_addr(&datagram.agent_address);
    let source_realtime_usec = timestamp_source.select(input_realtime_usec, None, None);
    let need_decap = !decapsulation_mode.is_none();

    let mut flows = Vec::new();
    for sample in datagram.samples {
        match sample.sample_data {
            SampleData::FlowSample(sample_data) => {
                stats.sflow_flow_samples += 1;
                if !emit_rows {
                    continue;
                }
                let mut in_if = if sample_data.input.is_single() {
                    Some(sample_data.input.value())
                } else {
                    None
                };
                let mut out_if = if sample_data.output.is_single() {
                    Some(sample_data.output.value())
                } else {
                    None
                };
                let forwarding_status = if sample_data.output.is_discarded() {
                    128
                } else {
                    0
                };

                if in_if == Some(SFLOW_INTERFACE_LOCAL) {
                    in_if = Some(0);
                }
                if out_if == Some(SFLOW_INTERFACE_LOCAL) {
                    out_if = Some(0);
                }

                if let Some(flow) = build_sflow_flow(
                    source,
                    exporter_ip_override,
                    sample_data.sampling_rate,
                    in_if,
                    out_if,
                    forwarding_status,
                    &sample_data.flow_records,
                    source_realtime_usec,
                    decapsulation_mode,
                    need_decap,
                ) {
                    flows.push(flow);
                } else {
                    stats.decapsulation_failed_records += 1;
                }
            }
            SampleData::FlowSampleExpanded(sample_data) => {
                stats.sflow_flow_samples += 1;
                if !emit_rows {
                    continue;
                }
                let mut in_if = if sample_data.input.format == SFLOW_INTERFACE_FORMAT_INDEX {
                    Some(sample_data.input.value)
                } else {
                    None
                };
                let mut out_if = if sample_data.output.format == SFLOW_INTERFACE_FORMAT_INDEX {
                    Some(sample_data.output.value)
                } else {
                    None
                };
                let forwarding_status =
                    if sample_data.output.format == SFLOW_INTERFACE_FORMAT_DISCARD {
                        128
                    } else {
                        0
                    };

                if in_if == Some(SFLOW_INTERFACE_LOCAL) {
                    in_if = Some(0);
                }
                if out_if == Some(SFLOW_INTERFACE_LOCAL) {
                    out_if = Some(0);
                }

                if let Some(flow) = build_sflow_flow(
                    source,
                    exporter_ip_override,
                    sample_data.sampling_rate,
                    in_if,
                    out_if,
                    forwarding_status,
                    &sample_data.flow_records,
                    source_realtime_usec,
                    decapsulation_mode,
                    need_decap,
                ) {
                    flows.push(flow);
                } else {
                    stats.decapsulation_failed_records += 1;
                }
            }
            SampleData::CountersSample(_) | SampleData::CountersSampleExpanded(_) => {
                stats.sflow_counter_samples += 1;
            }
            SampleData::DiscardedPacket(_) => {
                stats.sflow_discarded_samples += 1;
            }
            SampleData::RtMetric { .. } => {
                stats.sflow_rt_metric_samples += 1;
            }
            SampleData::RtFlow { .. } => {
                stats.sflow_rt_flow_samples += 1;
            }
            SampleData::Unknown { .. } => {
                stats.sflow_unknown_samples += 1;
            }
        }
    }

    flows
}
