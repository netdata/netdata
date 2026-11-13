//! The netflow plugin's charts about its own operation: the values each chart
//! shows, as sampled into `NetflowChartsSnapshot`, and the metric sets that
//! declare the charts and record the values.

use netdata_plugin_metrics::{
    ChartDescriptor, ChartType, CumulativeCounter, Error, Gauge, Measurements, MetricSet,
    MetricSetBuilder, Settings,
};
use std::time::Duration;

/// Declares the values of one chart: a struct with one `u64` field per
/// dimension, and the chart's column enum, whose variants are named and
/// ordered as the fields, so each field name is a dimension id. `record` sets
/// the values on the chart's instrument: running totals for `totals`, current
/// values for `values`.
macro_rules! chart_values {
    (totals $values:ident, $columns:ident { $($field:ident),+ $(,)? }) => {
        chart_values!(@declare $values, $columns { $($field),+ });

        impl $values {
            fn record(&self, measurements: &mut Measurements<'_>, counter: CumulativeCounter<$columns>) {
                $(measurements.set_total(counter, $columns::$field, self.$field as f64);)+
            }
        }
    };
    (values $values:ident, $columns:ident { $($field:ident),+ $(,)? }) => {
        chart_values!(@declare $values, $columns { $($field),+ });

        impl $values {
            fn record(&self, measurements: &mut Measurements<'_>, gauge: Gauge<$columns>) {
                $(measurements.set_value(gauge, $columns::$field, self.$field as f64);)+
            }
        }
    };
    (@declare $values:ident, $columns:ident { $($field:ident),+ }) => {
        #[derive(Debug, Default, Clone, PartialEq)]
        pub(super) struct $values {
            $(pub(super) $field: u64,)+
        }

        // Variants named after the fields: the macro maps one to the other.
        #[allow(non_camel_case_types)]
        #[derive(Debug, Clone, Copy)]
        enum $columns {
            $($field,)+
        }

        impl netdata_plugin_metrics::Column for $columns {
            const NAMES: &'static [&'static str] = &[$(stringify!($field)),+];

            fn index(self) -> usize {
                self as usize
            }
        }
    };
}

chart_values! {
    totals InputPacketsMetrics, InputPacketsColumn {
        udp_received,
        kernel_dropped,
        empty,
    }
}

chart_values! {
    totals InputBytesMetrics, InputBytesColumn {
        udp_received,
    }
}

chart_values! {
    totals ProtocolPacketsMetrics, ProtocolPacketsColumn {
        netflow_v5,
        netflow_v7,
        netflow_v9,
        ipfix,
        sflow,
    }
}

chart_values! {
    totals FlowSetMetrics, FlowSetColumn {
        v9_data,
        v9_options_data,
        v9_templates,
        v9_options_templates,
        v9_missing_template,
        v9_ignored,
        ipfix_data,
        ipfix_options_data,
        ipfix_templates,
        ipfix_options_templates,
        ipfix_missing_template,
        ipfix_ignored,
    }
}

chart_values! {
    totals TemplateMetrics, TemplateColumn {
        v9_data,
        v9_options,
        ipfix_data,
        ipfix_options,
    }
}

chart_values! {
    totals FlowRecordMetrics, FlowRecordColumn {
        netflow_v5,
        netflow_v7,
        netflow_v9,
        ipfix,
    }
}

chart_values! {
    totals OptionsRecordMetrics, OptionsRecordColumn {
        netflow_v9,
        ipfix,
        sampling_data,
    }
}

chart_values! {
    totals SflowSampleMetrics, SflowSampleColumn {
        flow,
        counter,
        discarded_packet,
        rt_metric,
        rt_flow,
        unknown,
    }
}

chart_values! {
    totals DecoderExceptionMetrics, DecoderExceptionColumn {
        udp_receive_errors,
        udp_socket_setup_errors,
        parse_errors,
        missing_template_sets,
        disabled_protocol_packets,
        parser_source_evictions,
        partial_counter_records,
        decapsulation_failed_records,
        unsupported_data_sets,
        ipfix_zero_reverse_records,
    }
}

chart_values! {
    values DecoderScopeMetrics, DecoderScopeColumn {
        v9_sources,
        ipfix_sources,
        legacy_sources,
        namespaces,
        hydrated_sources,
    }
}

chart_values! {
    totals FlowRowMetrics, FlowRowColumn {
        decoded,
        classifier_filtered,
        journaled,
        write_failed,
    }
}

chart_values! {
    totals NselEventMetrics, NselEventColumn {
        update,
        create,
        teardown,
        denied,
        unsupported,
        malformed,
    }
}

chart_values! {
    totals NselRowMetrics, NselRowColumn {
        forward,
        reverse,
    }
}

chart_values! {
    totals NselExceptionMetrics, NselExceptionColumn {
        counterless_updates,
        partial_counter_directions,
        zero_responder,
    }
}

chart_values! {
    totals RawJournalOpsMetrics, RawJournalOpsColumn {
        entries_written,
        write_errors,
        sync_calls,
        sync_errors,
    }
}

chart_values! {
    totals RawJournalBytesMetrics, RawJournalBytesColumn {
        logical_written,
    }
}

chart_values! {
    totals MaterializedTierOpsMetrics, MaterializedTierOpsColumn {
        minute_1_rows,
        minute_5_rows,
        hour_1_rows,
        flushes,
        write_errors,
        sync_calls,
        sync_errors,
    }
}

chart_values! {
    totals MaterializedTierBytesMetrics, MaterializedTierBytesColumn {
        minute_1_logical_written,
        minute_5_logical_written,
        hour_1_logical_written,
    }
}

chart_values! {
    values TierCommitAgeMetrics, TierCommitAgeColumn {
        minute_1,
        minute_5,
        hour_1,
    }
}

chart_values! {
    values TierCommitDurationMetrics, TierCommitDurationColumn {
        minute_1,
        minute_5,
        hour_1,
    }
}

chart_values! {
    totals TierCommitBatchesMetrics, TierCommitBatchesColumn {
        minute_1,
        minute_5,
        hour_1,
    }
}

chart_values! {
    totals TierCommitStretchedMetrics, TierCommitStretchedColumn {
        minute_1,
        minute_5,
        hour_1,
    }
}

chart_values! {
    totals JournalIoOpsMetrics, JournalIoOpsColumn {
        decoder_state_persist_calls,
        decoder_state_write_errors,
        decoder_state_move_errors,
        facet_active_update_errors,
        facet_lifecycle_errors,
        facet_persist_errors,
    }
}

chart_values! {
    totals JournalIoBytesMetrics, JournalIoBytesColumn {
        decoder_state_persist_bytes,
    }
}

chart_values! {
    values OpenTierMetrics, OpenTierColumn {
        minute_1,
        minute_5,
        hour_1,
    }
}

chart_values! {
    values TierIndexEntryMetrics, TierIndexEntryColumn {
        hours,
        flows,
    }
}

chart_values! {
    values FacetValueMetrics, FacetValueColumn {
        total,
        exposed,
    }
}

chart_values! {
    values FacetFieldMetrics, FacetFieldColumn {
        populated,
        autocomplete,
    }
}

chart_values! {
    values MemoryResidentBytesMetrics, MemoryResidentBytesColumn {
        rss,
        hwm,
        rss_anon,
        rss_file,
        rss_shmem,
        anon_huge_pages,
    }
}

chart_values! {
    values MemoryResidentMappingBytesMetrics, MemoryResidentMappingBytesColumn {
        heap,
        anon_other,
        journal_raw,
        journal_1m,
        journal_5m,
        journal_1h,
        geoip_asn,
        geoip_geo,
        other_file,
        shmem,
    }
}

chart_values! {
    values MemoryAllocatorBytesMetrics, MemoryAllocatorBytesColumn {
        heap_in_use,
        heap_free,
        heap_arena,
        mmap_in_use,
        releasable,
    }
}

chart_values! {
    values MemoryAccountedBytesMetrics, MemoryAccountedBytesColumn {
        facet_archived,
        facet_active,
        facet_active_contributions,
        facet_published,
        facet_archived_paths,
        tier_indexes,
        open_tiers,
        geoip_asn,
        geoip_geo,
        unaccounted,
    }
}

chart_values! {
    values MemoryTierIndexBytesMetrics, MemoryTierIndexBytesColumn {
        row_storage,
        field_stores,
        flow_lookup,
        schema,
        index_keys,
        scratch_field_ids,
    }
}

/// A netflow chart in its dashboard group (`family`), at its place among the
/// netflow charts (`priority`).
fn chart(
    context: &str,
    title: &str,
    units: &str,
    chart_type: ChartType,
    family: &str,
    priority: i32,
) -> ChartDescriptor {
    ChartDescriptor {
        title: title.into(),
        units: units.into(),
        family: family.into(),
        context: context.into(),
        chart_type,
        priority,
    }
}

/// The charts sampled every second.
pub(super) struct OperationCharts {
    pub(super) set: MetricSet,
    input_packets: CumulativeCounter<InputPacketsColumn>,
    input_bytes: CumulativeCounter<InputBytesColumn>,
    protocol_packets: CumulativeCounter<ProtocolPacketsColumn>,
    flow_sets: CumulativeCounter<FlowSetColumn>,
    templates: CumulativeCounter<TemplateColumn>,
    flow_records: CumulativeCounter<FlowRecordColumn>,
    options_records: CumulativeCounter<OptionsRecordColumn>,
    sflow_samples: CumulativeCounter<SflowSampleColumn>,
    decoder_exceptions: CumulativeCounter<DecoderExceptionColumn>,
    decoder_scopes: Gauge<DecoderScopeColumn>,
    flow_rows: CumulativeCounter<FlowRowColumn>,
    nsel_events: CumulativeCounter<NselEventColumn>,
    nsel_rows: CumulativeCounter<NselRowColumn>,
    nsel_exceptions: CumulativeCounter<NselExceptionColumn>,
    raw_journal_ops: CumulativeCounter<RawJournalOpsColumn>,
    raw_journal_bytes: CumulativeCounter<RawJournalBytesColumn>,
    materialized_tier_ops: CumulativeCounter<MaterializedTierOpsColumn>,
    materialized_tier_bytes: CumulativeCounter<MaterializedTierBytesColumn>,
    tier_commit_age: Gauge<TierCommitAgeColumn>,
    tier_commit_duration: Gauge<TierCommitDurationColumn>,
    tier_commit_batches: CumulativeCounter<TierCommitBatchesColumn>,
    tier_commit_stretched: CumulativeCounter<TierCommitStretchedColumn>,
    journal_io_ops: CumulativeCounter<JournalIoOpsColumn>,
    journal_io_bytes: CumulativeCounter<JournalIoBytesColumn>,
    open_tiers: Gauge<OpenTierColumn>,
    tier_index_entries: Gauge<TierIndexEntryColumn>,
    facet_values: Gauge<FacetValueColumn>,
    facet_fields: Gauge<FacetFieldColumn>,
}

impl OperationCharts {
    /// Declares the charts, in dashboard order, at a 1 s collection interval.
    pub(super) fn new() -> Result<Self, Error> {
        let mut builder = MetricSetBuilder::new(Settings::default())?;
        Ok(Self {
            input_packets: builder.cumulative_counter(chart(
                "netdata.netflow.input_packets",
                "Netflow UDP Input Packets",
                "packets/s",
                ChartType::Line,
                "input",
                160_000,
            ))?,
            input_bytes: builder.cumulative_counter(chart(
                "netdata.netflow.input_bytes",
                "Netflow Input Bytes",
                "bytes/s",
                ChartType::Line,
                "input",
                160_001,
            ))?,
            protocol_packets: builder.cumulative_counter(chart(
                "netdata.netflow.protocol_packets",
                "Netflow Protocol Packets",
                "packets/s",
                ChartType::Stacked,
                "input",
                160_002,
            ))?,
            flow_sets: builder.cumulative_counter(chart(
                "netdata.netflow.flow_sets",
                "Netflow Sets",
                "sets/s",
                ChartType::Stacked,
                "decoding",
                160_100,
            ))?,
            templates: builder.cumulative_counter(chart(
                "netdata.netflow.templates",
                "Netflow Template Definitions",
                "templates/s",
                ChartType::Stacked,
                "decoding",
                160_101,
            ))?,
            flow_records: builder.cumulative_counter(chart(
                "netdata.netflow.flow_records",
                "Netflow Decoded Data Records",
                "records/s",
                ChartType::Stacked,
                "decoding",
                160_102,
            ))?,
            options_records: builder.cumulative_counter(chart(
                "netdata.netflow.options_records",
                "Netflow Options Records",
                "records/s",
                ChartType::Stacked,
                "decoding",
                160_103,
            ))?,
            sflow_samples: builder.cumulative_counter(chart(
                "netdata.netflow.sflow_samples",
                "sFlow Samples",
                "samples/s",
                ChartType::Stacked,
                "decoding",
                160_104,
            ))?,
            decoder_exceptions: builder.cumulative_counter(chart(
                "netdata.netflow.decoder_exceptions",
                "Netflow Decoder Exceptions",
                "events/s",
                ChartType::Line,
                "decoding",
                160_105,
            ))?,
            decoder_scopes: builder.gauge(chart(
                "netdata.netflow.decoder_scopes",
                "Netflow Decoder Scope Counts",
                "scopes",
                ChartType::Line,
                "decoding",
                160_106,
            ))?,
            flow_rows: builder.cumulative_counter(chart(
                "netdata.netflow.flow_rows",
                "Netflow Row Pipeline",
                "rows/s",
                ChartType::Line,
                "decoding",
                160_107,
            ))?,
            nsel_events: builder.cumulative_counter(chart(
                "netdata.netflow.nsel_events",
                "Cisco NSEL Events",
                "records/s",
                ChartType::Stacked,
                "cisco nsel",
                160_200,
            ))?,
            nsel_rows: builder.cumulative_counter(chart(
                "netdata.netflow.nsel_rows",
                "Cisco NSEL Rows",
                "rows/s",
                ChartType::Stacked,
                "cisco nsel",
                160_201,
            ))?,
            nsel_exceptions: builder.cumulative_counter(chart(
                "netdata.netflow.nsel_exceptions",
                "Cisco NSEL Exceptions",
                "events/s",
                ChartType::Line,
                "cisco nsel",
                160_202,
            ))?,
            raw_journal_ops: builder.cumulative_counter(chart(
                "netdata.netflow.raw_journal_ops",
                "Netflow Raw Journal Operations",
                "ops/s",
                ChartType::Line,
                "storage",
                160_300,
            ))?,
            raw_journal_bytes: builder.cumulative_counter(chart(
                "netdata.netflow.raw_journal_bytes",
                "Netflow Raw Journal Logical Bytes",
                "bytes/s",
                ChartType::Line,
                "storage",
                160_301,
            ))?,
            materialized_tier_ops: builder.cumulative_counter(chart(
                "netdata.netflow.materialized_tier_ops",
                "Netflow Materialized Tier Operations",
                "ops/s",
                ChartType::Line,
                "storage",
                160_302,
            ))?,
            materialized_tier_bytes: builder.cumulative_counter(chart(
                "netdata.netflow.materialized_tier_bytes",
                "Netflow Materialized Tier Logical Bytes",
                "bytes/s",
                ChartType::Stacked,
                "storage",
                160_303,
            ))?,
            tier_commit_age: builder.gauge(chart(
                "netdata.netflow.tier_commit_age",
                "Netflow Tier Commit Age",
                "seconds",
                ChartType::Line,
                "storage",
                160_304,
            ))?,
            tier_commit_duration: builder.gauge(chart(
                "netdata.netflow.tier_commit_duration",
                "Netflow Tier Commit Duration",
                "microseconds",
                ChartType::Line,
                "storage",
                160_305,
            ))?,
            tier_commit_batches: builder.cumulative_counter(chart(
                "netdata.netflow.tier_commit_batches",
                "Netflow Tier Commit Batches",
                "batches/s",
                ChartType::Line,
                "storage",
                160_306,
            ))?,
            tier_commit_stretched: builder.cumulative_counter(chart(
                "netdata.netflow.tier_commit_stretched",
                "Netflow Tier Commit Stretched Windows",
                "events/s",
                ChartType::Line,
                "storage",
                160_307,
            ))?,
            journal_io_ops: builder.cumulative_counter(chart(
                "netdata.netflow.journal_io_ops",
                "Netflow Journal Auxiliary I/O Operations",
                "ops/s",
                ChartType::Line,
                "storage",
                160_308,
            ))?,
            journal_io_bytes: builder.cumulative_counter(chart(
                "netdata.netflow.journal_io_bytes",
                "Netflow Journal Auxiliary I/O Bytes",
                "bytes/s",
                ChartType::Line,
                "storage",
                160_309,
            ))?,
            open_tiers: builder.gauge(chart(
                "netdata.netflow.open_tiers",
                "Netflow Open Tier Rows",
                "rows",
                ChartType::Stacked,
                "state",
                160_400,
            ))?,
            tier_index_entries: builder.gauge(chart(
                "netdata.netflow.tier_index_entries",
                "Netflow Tier Index Entries",
                "entries",
                ChartType::Line,
                "state",
                160_401,
            ))?,
            facet_values: builder.gauge(chart(
                "netdata.netflow.facet_values",
                "Netflow Facet Values",
                "values",
                ChartType::Line,
                "state",
                160_402,
            ))?,
            facet_fields: builder.gauge(chart(
                "netdata.netflow.facet_fields",
                "Netflow Facet Fields",
                "fields",
                ChartType::Line,
                "state",
                160_403,
            ))?,
            set: builder.build()?,
        })
    }

    /// Sets each chart's values from `snapshot`.
    pub(super) fn record(
        &self,
        measurements: &mut Measurements<'_>,
        snapshot: &super::NetflowChartsSnapshot,
    ) {
        snapshot
            .input_packets
            .record(measurements, self.input_packets);
        snapshot.input_bytes.record(measurements, self.input_bytes);
        snapshot
            .protocol_packets
            .record(measurements, self.protocol_packets);
        snapshot.flow_sets.record(measurements, self.flow_sets);
        snapshot.templates.record(measurements, self.templates);
        snapshot
            .flow_records
            .record(measurements, self.flow_records);
        snapshot
            .options_records
            .record(measurements, self.options_records);
        snapshot
            .sflow_samples
            .record(measurements, self.sflow_samples);
        snapshot
            .decoder_exceptions
            .record(measurements, self.decoder_exceptions);
        snapshot
            .decoder_scopes
            .record(measurements, self.decoder_scopes);
        snapshot.flow_rows.record(measurements, self.flow_rows);
        snapshot.nsel_events.record(measurements, self.nsel_events);
        snapshot.nsel_rows.record(measurements, self.nsel_rows);
        snapshot
            .nsel_exceptions
            .record(measurements, self.nsel_exceptions);
        snapshot
            .raw_journal_ops
            .record(measurements, self.raw_journal_ops);
        snapshot
            .raw_journal_bytes
            .record(measurements, self.raw_journal_bytes);
        snapshot
            .materialized_tier_ops
            .record(measurements, self.materialized_tier_ops);
        snapshot
            .materialized_tier_bytes
            .record(measurements, self.materialized_tier_bytes);
        snapshot
            .tier_commit_age
            .record(measurements, self.tier_commit_age);
        snapshot
            .tier_commit_duration
            .record(measurements, self.tier_commit_duration);
        snapshot
            .tier_commit_batches
            .record(measurements, self.tier_commit_batches);
        snapshot
            .tier_commit_stretched
            .record(measurements, self.tier_commit_stretched);
        snapshot
            .journal_io_ops
            .record(measurements, self.journal_io_ops);
        snapshot
            .journal_io_bytes
            .record(measurements, self.journal_io_bytes);
        snapshot.open_tiers.record(measurements, self.open_tiers);
        snapshot
            .tier_index_entries
            .record(measurements, self.tier_index_entries);
        snapshot
            .facet_values
            .record(measurements, self.facet_values);
        snapshot
            .facet_fields
            .record(measurements, self.facet_fields);
    }
}

/// The memory diagnostics charts, sampled every `charts.memory_diagnostics.interval`.
pub(super) struct MemoryCharts {
    pub(super) set: MetricSet,
    memory_resident_bytes: Gauge<MemoryResidentBytesColumn>,
    memory_resident_mapping_bytes: Gauge<MemoryResidentMappingBytesColumn>,
    memory_allocator_bytes: Gauge<MemoryAllocatorBytesColumn>,
    memory_accounted_bytes: Gauge<MemoryAccountedBytesColumn>,
    memory_tier_index_bytes: Gauge<MemoryTierIndexBytesColumn>,
}

impl MemoryCharts {
    /// Declares the charts at `interval`, which must be whole seconds.
    pub(super) fn new(interval: Duration) -> Result<Self, Error> {
        let mut builder = MetricSetBuilder::new(Settings {
            collection_interval: interval,
            ..Settings::default()
        })?;
        Ok(Self {
            memory_resident_bytes: builder.gauge(chart(
                "netdata.netflow.memory_resident_bytes",
                "Netflow Process Memory",
                "bytes",
                ChartType::Line,
                "memory",
                160_500,
            ))?,
            memory_resident_mapping_bytes: builder.gauge(chart(
                "netdata.netflow.memory_resident_mapping_bytes",
                "Netflow Resident Mapping Breakdown",
                "bytes",
                ChartType::Stacked,
                "memory",
                160_501,
            ))?,
            memory_allocator_bytes: builder.gauge(chart(
                "netdata.netflow.memory_allocator_bytes",
                "Netflow Allocator Memory",
                "bytes",
                ChartType::Line,
                "memory",
                160_502,
            ))?,
            memory_accounted_bytes: builder.gauge(chart(
                "netdata.netflow.memory_accounted_bytes",
                "Netflow Accounted Memory",
                "bytes",
                ChartType::Stacked,
                "memory",
                160_503,
            ))?,
            memory_tier_index_bytes: builder.gauge(chart(
                "netdata.netflow.memory_tier_index_bytes",
                "Netflow Tier Index Memory Breakdown",
                "bytes",
                ChartType::Stacked,
                "memory",
                160_504,
            ))?,
            set: builder.build()?,
        })
    }

    /// Sets each chart's values from `snapshot`.
    pub(super) fn record(
        &self,
        measurements: &mut Measurements<'_>,
        snapshot: &super::NetflowChartsSnapshot,
    ) {
        snapshot
            .memory_resident_bytes
            .record(measurements, self.memory_resident_bytes);
        snapshot
            .memory_resident_mapping_bytes
            .record(measurements, self.memory_resident_mapping_bytes);
        snapshot
            .memory_allocator_bytes
            .record(measurements, self.memory_allocator_bytes);
        snapshot
            .memory_accounted_bytes
            .record(measurements, self.memory_accounted_bytes);
        snapshot
            .memory_tier_index_bytes
            .record(measurements, self.memory_tier_index_bytes);
    }
}
