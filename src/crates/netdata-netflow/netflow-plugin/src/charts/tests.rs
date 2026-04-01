use super::*;

#[test]
fn chart_metadata_uses_honest_contexts_and_units() {
    let raw_bytes = RawJournalBytesMetrics::chart_metadata();
    assert_eq!(raw_bytes.context, "netdata.netflow.raw_journal_bytes");
    assert_eq!(raw_bytes.family, "netflow");
    assert_eq!(raw_bytes.units, "bytes/s");

    let raw_ops = RawJournalOpsMetrics::chart_metadata();
    assert_eq!(raw_ops.context, "netdata.netflow.raw_journal_ops");
    assert_eq!(raw_ops.family, "netflow");
    assert_eq!(raw_ops.units, "ops/s");

    let open = OpenTierMetrics::chart_metadata();
    assert_eq!(open.context, "netdata.netflow.open_tiers");
    assert_eq!(open.family, "netflow");
    assert_eq!(open.units, "rows");
}

#[test]
fn snapshot_collects_current_metric_totals_and_open_rows() {
    let metrics = IngestMetrics::default();
    metrics.udp_packets_received.store(11, Ordering::Relaxed);
    metrics.udp_bytes_received.store(22, Ordering::Relaxed);
    metrics.journal_entries_written.store(33, Ordering::Relaxed);
    metrics.raw_journal_syncs.store(44, Ordering::Relaxed);
    metrics
        .raw_journal_logical_bytes
        .store(55, Ordering::Relaxed);
    metrics
        .minute_1_entries_written
        .store(66, Ordering::Relaxed);
    metrics.minute_5_logical_bytes.store(77, Ordering::Relaxed);
    metrics
        .decoder_state_persist_calls
        .store(88, Ordering::Relaxed);
    metrics
        .decoder_state_persist_bytes
        .store(99, Ordering::Relaxed);

    let snapshot = NetflowChartsSnapshot::collect(&metrics, (1, 2, 0));
    assert_eq!(snapshot.input_packets.udp_received, 11);
    assert_eq!(snapshot.input_bytes.udp_received, 22);
    assert_eq!(snapshot.raw_journal_ops.entries_written, 33);
    assert_eq!(snapshot.raw_journal_ops.sync_calls, 44);
    assert_eq!(snapshot.raw_journal_bytes.logical_written, 55);
    assert_eq!(snapshot.materialized_tier_ops.minute_1_rows, 66);
    assert_eq!(
        snapshot.materialized_tier_bytes.minute_5_logical_written,
        77
    );
    assert_eq!(snapshot.journal_io_ops.decoder_state_persist_calls, 88);
    assert_eq!(snapshot.journal_io_bytes.decoder_state_persist_bytes, 99);
    assert_eq!(snapshot.open_tiers.minute_1, 1);
    assert_eq!(snapshot.open_tiers.minute_5, 2);
    assert_eq!(snapshot.open_tiers.hour_1, 0);
}
