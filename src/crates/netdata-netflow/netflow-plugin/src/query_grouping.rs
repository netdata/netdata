struct FlowRecord {
    timestamp_usec: u64,
    fields: BTreeMap<String, String>,
}

impl FlowRecord {
    fn new(timestamp_usec: u64, mut fields: BTreeMap<String, String>) -> Self {
        populate_virtual_fields(&mut fields);
        Self {
            timestamp_usec,
            fields,
        }
    }
}

#[derive(Debug, Clone, Copy, Default)]
struct FlowMetrics {
    bytes: u64,
    packets: u64,
}

impl FlowMetrics {
    fn add(&mut self, other: FlowMetrics) {
        self.bytes = self.bytes.saturating_add(other.bytes);
        self.packets = self.packets.saturating_add(other.packets);
    }

    fn to_value(self) -> Value {
        json!({
            "bytes": self.bytes,
            "packets": self.packets,
        })
    }

    fn to_map(self) -> HashMap<String, u64> {
        let mut m = HashMap::new();
        m.insert("bytes".to_string(), self.bytes);
        m.insert("packets".to_string(), self.packets);
        m
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum RecordHandle {
    JournalRealtime { tier: TierKind, timestamp_usec: u64 },
}

#[derive(Debug, Clone)]
struct CompactAggregatedFlow {
    flow_id: Option<IndexedFlowId>,
    group_field_ids: Option<Vec<u32>>,
    first_ts: u64,
    last_ts: u64,
    metrics: FlowMetrics,
    bucket_label: Option<&'static str>,
    folded_labels: Option<FoldedGroupedLabels>,
}

impl CompactAggregatedFlow {
    fn new(
        record: &FlowRecord,
        _handle: RecordHandle,
        metrics: FlowMetrics,
        flow_id: IndexedFlowId,
    ) -> Self {
        let mut entry = Self {
            flow_id: Some(flow_id),
            group_field_ids: None,
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            bucket_label: None,
            folded_labels: None,
        };
        entry.update_projected(record.timestamp_usec, metrics);
        entry
    }

    fn new_overflow() -> Self {
        Self::new_synthetic_bucket(OVERFLOW_BUCKET_LABEL)
    }

    fn new_other() -> Self {
        Self::new_synthetic_bucket(OTHER_BUCKET_LABEL)
    }

    fn new_synthetic_bucket(bucket_label: &'static str) -> Self {
        Self {
            flow_id: None,
            group_field_ids: None,
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            bucket_label: Some(bucket_label),
            folded_labels: Some(FoldedGroupedLabels::default()),
        }
    }

    fn new_projected(
        _handle: RecordHandle,
        group_field_ids: Vec<u32>,
        timestamp_usec: u64,
        metrics: FlowMetrics,
    ) -> Self {
        let mut entry = Self {
            flow_id: None,
            group_field_ids: Some(group_field_ids),
            first_ts: 0,
            last_ts: 0,
            metrics: FlowMetrics::default(),
            bucket_label: None,
            folded_labels: None,
        };
        entry.update_projected(timestamp_usec, metrics);
        entry
    }

    fn update(&mut self, record: &FlowRecord, metrics: FlowMetrics) {
        self.update_projected(record.timestamp_usec, metrics);
    }

    fn update_projected(&mut self, timestamp_usec: u64, metrics: FlowMetrics) {
        if self.first_ts == 0 || timestamp_usec < self.first_ts {
            self.first_ts = timestamp_usec;
        }
        if timestamp_usec > self.last_ts {
            self.last_ts = timestamp_usec;
        }
        self.metrics.add(metrics);
    }
}

#[derive(Debug, Default)]
struct CompactGroupOverflow {
    aggregate: Option<CompactAggregatedFlow>,
    dropped_records: u64,
}

struct CompactGroupAccumulator {
    index: FlowIndex,
    rows: Vec<CompactAggregatedFlow>,
    scratch_field_ids: Vec<u32>,
    overflow: CompactGroupOverflow,
}

impl CompactGroupAccumulator {
    fn new(group_by: &[String]) -> Result<Self> {
        let schema = group_by
            .iter()
            .map(|field| IndexFieldSpec::new(field.clone(), IndexFieldKind::Text));
        Ok(Self {
            index: FlowIndex::new(schema)
                .context("failed to build compact flow index for grouped query")?,
            rows: Vec::new(),
            scratch_field_ids: Vec::with_capacity(group_by.len()),
            overflow: CompactGroupOverflow::default(),
        })
    }

    fn grouped_total(&self) -> usize {
        self.rows.len()
    }
}

struct ProjectedFieldTable {
    value_ids: Vec<FastHashMap<String, u32>>,
    values: Vec<Vec<String>>,
}

impl ProjectedFieldTable {
    fn new(group_by: &[String]) -> Self {
        let mut value_ids = Vec::with_capacity(group_by.len());
        let mut values = Vec::with_capacity(group_by.len());
        for _ in group_by {
            value_ids.push(FastHashMap::from([(String::new(), 0)]));
            values.push(vec![String::new()]);
        }
        Self { value_ids, values }
    }

    fn find_field_value(&self, field_index: usize, value: &str) -> Option<u32> {
        self.value_ids
            .get(field_index)
            .and_then(|values| values.get(value).copied())
    }

    fn get_or_insert_field_value(&mut self, field_index: usize, value: &str) -> u32 {
        if let Some(field_id) = self.find_field_value(field_index, value) {
            return field_id;
        }

        let field_values = &mut self.values[field_index];
        let field_id = field_values.len() as u32;
        let owned = value.to_string();
        field_values.push(owned.clone());
        self.value_ids[field_index].insert(owned, field_id);
        field_id
    }

    fn field_value(&self, field_index: usize, field_id: u32) -> Option<&str> {
        self.values
            .get(field_index)
            .and_then(|values| values.get(field_id as usize))
            .map(String::as_str)
    }
}

struct ProjectedGroupAccumulator {
    fields: ProjectedFieldTable,
    rows: Vec<CompactAggregatedFlow>,
    row_indexes: FastHashMap<Vec<u32>, usize>,
    scratch_field_ids: Vec<u32>,
    overflow: CompactGroupOverflow,
}

impl ProjectedGroupAccumulator {
    fn new(group_by: &[String]) -> Self {
        Self {
            fields: ProjectedFieldTable::new(group_by),
            rows: Vec::new(),
            row_indexes: FastHashMap::default(),
            scratch_field_ids: Vec::with_capacity(group_by.len()),
            overflow: CompactGroupOverflow::default(),
        }
    }

    fn grouped_total(&self) -> usize {
        self.rows.len()
    }

    fn find_field_value(&self, field_index: usize, value: &str) -> Option<u32> {
        self.fields.find_field_value(field_index, value)
    }

    fn accumulate_projected(
        &mut self,
        group_by: &[String],
        timestamp_usec: u64,
        handle: RecordHandle,
        metrics: FlowMetrics,
        row_group_field_ids: &mut [Option<u32>],
        row_missing_values: &mut [Option<String>],
        max_groups: usize,
    ) -> Result<()> {
        anyhow::ensure!(
            row_group_field_ids.len() == row_missing_values.len(),
            "projected grouped row buffers are misaligned"
        );

        self.scratch_field_ids.clear();
        let mut needs_field_inserts = false;
        for field_index in 0..row_group_field_ids.len() {
            if let Some(field_id) = row_group_field_ids[field_index] {
                self.scratch_field_ids.push(field_id);
                continue;
            }

            if row_missing_values[field_index].is_some() {
                needs_field_inserts = true;
                break;
            }

            self.scratch_field_ids.push(0);
        }

        if !needs_field_inserts {
            if let Some(row_index) = self.row_indexes.get(self.scratch_field_ids.as_slice()) {
                if let Some(entry) = self.rows.get_mut(*row_index) {
                    entry.update_projected(timestamp_usec, metrics);
                    return Ok(());
                }
                anyhow::bail!(
                    "projected grouped index returned missing row for row slot {}",
                    row_index
                );
            }

            if self.grouped_total() >= max_groups {
                let entry = self
                    .overflow
                    .aggregate
                    .get_or_insert_with(CompactAggregatedFlow::new_overflow);
                self.overflow.dropped_records = self.overflow.dropped_records.saturating_add(1);
                let labels = projected_group_labels_from_fields(
                    &self.fields,
                    group_by,
                    row_group_field_ids,
                    row_missing_values,
                )?;
                merge_compact_projected_labels(entry, &labels);
                entry.update_projected(timestamp_usec, metrics);
                return Ok(());
            }

            let row_index = self.rows.len();
            self.row_indexes
                .insert(self.scratch_field_ids.clone(), row_index);
            self.rows.push(CompactAggregatedFlow::new_projected(
                handle,
                self.scratch_field_ids.clone(),
                timestamp_usec,
                metrics,
            ));
            return Ok(());
        }

        if self.grouped_total() >= max_groups {
            let entry = self
                .overflow
                .aggregate
                .get_or_insert_with(CompactAggregatedFlow::new_overflow);
            self.overflow.dropped_records = self.overflow.dropped_records.saturating_add(1);
            let labels = projected_group_labels_from_fields(
                &self.fields,
                group_by,
                row_group_field_ids,
                row_missing_values,
            )?;
            merge_compact_projected_labels(entry, &labels);
            entry.update_projected(timestamp_usec, metrics);
            return Ok(());
        }

        self.scratch_field_ids.clear();
        for field_index in 0..row_group_field_ids.len() {
            let field_id = if let Some(field_id) = row_group_field_ids[field_index] {
                field_id
            } else if let Some(value) = row_missing_values[field_index].take() {
                let field_id = self
                    .fields
                    .get_or_insert_field_value(field_index, value.as_str());
                row_group_field_ids[field_index] = Some(field_id);
                field_id
            } else {
                0
            };
            self.scratch_field_ids.push(field_id);
        }

        if let Some(row_index) = self.row_indexes.get(self.scratch_field_ids.as_slice()) {
            if let Some(entry) = self.rows.get_mut(*row_index) {
                entry.update_projected(timestamp_usec, metrics);
                return Ok(());
            }
            anyhow::bail!(
                "projected grouped index returned missing row for row slot {}",
                row_index
            );
        }

        let row_index = self.rows.len();
        self.row_indexes
            .insert(self.scratch_field_ids.clone(), row_index);
        self.rows.push(CompactAggregatedFlow::new_projected(
            handle,
            self.scratch_field_ids.clone(),
            timestamp_usec,
            metrics,
        ));
        Ok(())
    }
}

struct CompactBuildResult {
    flows: Vec<Value>,
    metrics: FlowMetrics,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
    overflow_records: u64,
}

#[derive(Debug, Clone, Default)]
struct FoldedGroupedLabels {
    values: BTreeMap<String, BTreeSet<String>>,
}

impl FoldedGroupedLabels {
    fn merge_labels(&mut self, labels: &BTreeMap<String, String>) {
        for (field, value) in labels {
            if field == "_bucket" {
                continue;
            }
            self.values
                .entry(field.clone())
                .or_default()
                .insert(value.clone());
        }
    }

    fn merge_folded(&mut self, other: &Self) {
        for (field, values) in &other.values {
            self.values
                .entry(field.clone())
                .or_default()
                .extend(values.iter().cloned());
        }
    }

    fn render_into(&self, labels: &mut BTreeMap<String, String>) {
        for (field, values) in &self.values {
            if values.is_empty() {
                continue;
            }

            let rendered = if values.len() == 1 {
                values.iter().next().cloned().unwrap_or_default()
            } else {
                format!("Other ({})", values.len())
            };
            labels.insert(field.clone(), rendered);
        }
    }
}

struct RankedCompactAggregates {
    rows: Vec<CompactAggregatedFlow>,
    other: Option<CompactAggregatedFlow>,
    truncated: bool,
    other_count: usize,
}

#[cfg(test)]
#[allow(dead_code)]
struct BuildResult {
    flows: Vec<Value>,
    metrics: FlowMetrics,
    returned: usize,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
}

struct RankedAggregates {
    rows: Vec<AggregatedFlow>,
    #[cfg(test)]
    other: Option<AggregatedFlow>,
    grouped_total: usize,
    truncated: bool,
    other_count: usize,
}

#[derive(Debug, Default)]
struct AggregatedFlow {
    labels: BTreeMap<String, String>,
    first_ts: u64,
    last_ts: u64,
    metrics: FlowMetrics,
    folded_labels: Option<FoldedGroupedLabels>,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct GroupKey(Vec<(String, String)>);

#[derive(Debug, Default)]
struct GroupOverflow {
    aggregate: Option<AggregatedFlow>,
    dropped_records: u64,
}

#[derive(Debug, Default)]
#[cfg(test)]
struct FacetFieldAccumulator {
    values: BTreeMap<String, FlowMetrics>,
    overflow_metrics: FlowMetrics,
    overflow_records: u64,
}

#[cfg(test)]
fn build_aggregated_flows(records: &[FlowRecord]) -> BuildResult {
    let default_group_by = DEFAULT_GROUP_BY_FIELDS
        .iter()
        .map(|field| (*field).to_string())
        .collect::<Vec<_>>();
    build_grouped_flows(
        records,
        &default_group_by,
        SortBy::Bytes,
        DEFAULT_QUERY_LIMIT,
    )
}

#[cfg(test)]
fn build_grouped_flows(
    records: &[FlowRecord],
    group_by: &[String],
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let mut aggregates: HashMap<GroupKey, AggregatedFlow> = HashMap::new();
    let mut overflow = GroupOverflow::default();
    for record in records {
        let metrics = metrics_from_fields(&record.fields);
        accumulate_grouped_record(
            record,
            metrics,
            group_by,
            &mut aggregates,
            &mut overflow,
            DEFAULT_GROUP_ACCUMULATOR_MAX_GROUPS,
        );
    }
    build_grouped_flows_from_aggregates(aggregates, overflow.aggregate, sort_by, limit)
}

#[cfg(test)]
fn build_grouped_flows_from_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> BuildResult {
    let ranked = rank_aggregates(aggregates, overflow, sort_by, limit);

    let mut totals = FlowMetrics::default();
    let mut flows = Vec::with_capacity(ranked.rows.len() + usize::from(ranked.other.is_some()));

    for agg in ranked.rows {
        totals.add(agg.metrics);
        flows.push(flow_value_from_aggregate(agg));
    }

    if let Some(other_agg) = ranked.other {
        totals.add(other_agg.metrics);
        flows.push(flow_value_from_aggregate(other_agg));
    }

    BuildResult {
        returned: flows.len(),
        flows,
        metrics: totals,
        grouped_total: ranked.grouped_total,
        truncated: ranked.truncated,
        other_count: ranked.other_count,
    }
}

fn rank_aggregates(
    aggregates: HashMap<GroupKey, AggregatedFlow>,
    overflow: Option<AggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
) -> RankedAggregates {
    let grouped_total = aggregates.len();
    let mut grouped: Vec<AggregatedFlow> = aggregates.into_values().collect();
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    #[cfg(test)]
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        #[cfg(test)]
        {
            other = Some(merge_other_bucket(rest));
        }
    }

    RankedAggregates {
        rows,
        #[cfg(test)]
        other,
        grouped_total,
        truncated,
        other_count,
    }
}

fn accumulate_grouped_record(
    record: &FlowRecord,
    metrics: FlowMetrics,
    group_by: &[String],
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let labels = labels_for_group(record, group_by);
    let key = group_key_from_labels(&labels);
    if let Some(entry) = aggregates.get_mut(&key) {
        update_aggregate_entry(entry, record, metrics);
        return;
    }

    if aggregates.len() >= max_groups {
        let entry = overflow
            .aggregate
            .get_or_insert_with(new_overflow_aggregate);
        overflow.dropped_records = overflow.dropped_records.saturating_add(1);
        merge_grouped_labels(entry, &labels);
        update_aggregate_entry(entry, record, metrics);
        return;
    }

    let mut entry = AggregatedFlow {
        labels,
        first_ts: record.timestamp_usec,
        last_ts: record.timestamp_usec,
        ..AggregatedFlow::default()
    };
    update_aggregate_entry(&mut entry, record, metrics);
    aggregates.insert(key, entry);
}

fn accumulate_compact_grouped_record(
    record: &FlowRecord,
    handle: RecordHandle,
    metrics: FlowMetrics,
    group_by: &[String],
    aggregates: &mut CompactGroupAccumulator,
    max_groups: usize,
) -> Result<()> {
    aggregates.scratch_field_ids.clear();
    let mut needs_field_inserts = false;

    for (field_index, field_name) in group_by.iter().enumerate() {
        let value = record
            .fields
            .get(field_name.as_str())
            .map(String::as_str)
            .unwrap_or_default();
        match aggregates
            .index
            .find_field_value(field_index, IndexFieldValue::Text(value))
            .context("failed to resolve grouped field value from compact query index")?
        {
            Some(field_id) => aggregates.scratch_field_ids.push(field_id),
            None => {
                needs_field_inserts = true;
                break;
            }
        }
    }

    if !needs_field_inserts {
        if let Some(flow_id) = aggregates
            .index
            .find_flow_by_field_ids(&aggregates.scratch_field_ids)
            .context("failed to resolve grouped tuple from compact query index")?
        {
            if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
                entry.update(record, metrics);
                return Ok(());
            }
            anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
        }
    }

    if aggregates.grouped_total() >= max_groups {
        let entry = aggregates
            .overflow
            .aggregate
            .get_or_insert_with(CompactAggregatedFlow::new_overflow);
        aggregates.overflow.dropped_records = aggregates.overflow.dropped_records.saturating_add(1);
        let labels = labels_for_group(record, group_by);
        merge_compact_projected_labels(entry, &labels);
        entry.update(record, metrics);
        return Ok(());
    }

    if needs_field_inserts {
        aggregates.scratch_field_ids.clear();
        for (field_index, field_name) in group_by.iter().enumerate() {
            let value = record
                .fields
                .get(field_name.as_str())
                .map(String::as_str)
                .unwrap_or_default();
            let field_id = aggregates
                .index
                .get_or_insert_field_value(field_index, IndexFieldValue::Text(value))
                .context("failed to intern grouped field value into compact query index")?;
            aggregates.scratch_field_ids.push(field_id);
        }
    }

    if let Some(flow_id) = aggregates
        .index
        .find_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to recheck grouped tuple from compact query index")?
    {
        if let Some(entry) = aggregates.rows.get_mut(flow_id as usize) {
            entry.update(record, metrics);
            return Ok(());
        }
        anyhow::bail!("compact query index returned missing flow row for flow id {flow_id}");
    }

    let flow_id = aggregates
        .index
        .insert_flow_by_field_ids(&aggregates.scratch_field_ids)
        .context("failed to store grouped tuple into compact query index")?;
    if flow_id as usize != aggregates.rows.len() {
        anyhow::bail!(
            "compact query index returned non-dense flow id {} for row slot {}",
            flow_id,
            aggregates.rows.len()
        );
    }
    aggregates
        .rows
        .push(CompactAggregatedFlow::new(record, handle, metrics, flow_id));
    Ok(())
}

fn synthetic_bucket_labels(bucket_label: &'static str) -> BTreeMap<String, String> {
    BTreeMap::from([(String::from("_bucket"), String::from(bucket_label))])
}

fn new_bucket_aggregate(bucket_label: &'static str) -> AggregatedFlow {
    AggregatedFlow {
        labels: synthetic_bucket_labels(bucket_label),
        folded_labels: Some(FoldedGroupedLabels::default()),
        ..AggregatedFlow::default()
    }
}

fn new_overflow_aggregate() -> AggregatedFlow {
    new_bucket_aggregate(OVERFLOW_BUCKET_LABEL)
}

fn update_aggregate_entry(entry: &mut AggregatedFlow, record: &FlowRecord, metrics: FlowMetrics) {
    if entry.first_ts == 0 || record.timestamp_usec < entry.first_ts {
        entry.first_ts = record.timestamp_usec;
    }
    if record.timestamp_usec > entry.last_ts {
        entry.last_ts = record.timestamp_usec;
    }
    entry.metrics.add(metrics);
}

fn labels_for_group(record: &FlowRecord, group_by: &[String]) -> BTreeMap<String, String> {
    let mut labels = BTreeMap::new();
    for field in group_by {
        labels.insert(
            field.clone(),
            normalized_record_field_value(record, field).into_owned(),
        );
    }
    labels
}

fn labels_for_compact_flow(
    index: &FlowIndex,
    group_by: &[String],
    flow_id: IndexedFlowId,
) -> Result<BTreeMap<String, String>> {
    let field_ids = index
        .flow_field_ids(flow_id)
        .context("missing compact flow field ids for grouped query result")?;
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let field_id = field_ids
            .get(field_index)
            .copied()
            .context("missing compact flow field id for grouped query result")?;
        let value = index
            .field_value(field_index, field_id)
            .map(compact_index_value_to_string)
            .context("missing compact flow field value for grouped query result")?;
        labels.insert(field_name.clone(), value);
    }
    Ok(labels)
}

fn labels_for_projected_compact_flow(
    fields: &ProjectedFieldTable,
    group_by: &[String],
    field_ids: &[u32],
) -> Result<BTreeMap<String, String>> {
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let field_id = *field_ids
            .get(field_index)
            .context("missing projected compact flow field id for grouped query result")?;
        let value = fields
            .field_value(field_index, field_id)
            .context("missing projected compact flow field value for grouped query result")?;
        labels.insert(field_name.clone(), value.to_string());
    }
    Ok(labels)
}

fn projected_group_labels_from_fields(
    fields: &ProjectedFieldTable,
    group_by: &[String],
    row_group_field_ids: &[Option<u32>],
    row_missing_values: &[Option<String>],
) -> Result<BTreeMap<String, String>> {
    let mut labels = BTreeMap::new();
    for (field_index, field_name) in group_by.iter().enumerate() {
        let value = if let Some(field_id) = row_group_field_ids[field_index] {
            fields
                .field_value(field_index, field_id)
                .context("missing projected compact overflow field value")?
                .to_string()
        } else if let Some(value) = row_missing_values[field_index].as_ref() {
            value.clone()
        } else {
            String::new()
        };
        labels.insert(field_name.clone(), value);
    }
    Ok(labels)
}

#[cfg(test)]
fn merge_aggregate_grouped_labels(target: &mut AggregatedFlow, row: &AggregatedFlow) {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
    } else {
        folded.merge_labels(&row.labels);
    }
}

fn merge_grouped_labels(target: &mut AggregatedFlow, labels: &BTreeMap<String, String>) {
    target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default)
        .merge_labels(labels);
}

fn merge_compact_grouped_labels(
    target: &mut CompactAggregatedFlow,
    group_by: &[String],
    index: &FlowIndex,
    row: &CompactAggregatedFlow,
) -> Result<()> {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
        return Ok(());
    }

    let flow_id = row
        .flow_id
        .context("missing compact flow id while folding grouped labels into synthetic row")?;
    let labels = labels_for_compact_flow(index, group_by, flow_id)?;
    folded.merge_labels(&labels);
    Ok(())
}

fn merge_projected_compact_grouped_labels(
    target: &mut CompactAggregatedFlow,
    group_by: &[String],
    fields: &ProjectedFieldTable,
    row: &CompactAggregatedFlow,
) -> Result<()> {
    let folded = target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default);
    if let Some(source) = &row.folded_labels {
        folded.merge_folded(source);
        return Ok(());
    }

    let field_ids = row.group_field_ids.as_ref().context(
        "missing projected compact field ids while folding grouped labels into synthetic row",
    )?;
    let labels = labels_for_projected_compact_flow(fields, group_by, field_ids)?;
    folded.merge_labels(&labels);
    Ok(())
}

fn merge_compact_projected_labels(
    target: &mut CompactAggregatedFlow,
    labels: &BTreeMap<String, String>,
) {
    target
        .folded_labels
        .get_or_insert_with(FoldedGroupedLabels::default)
        .merge_labels(labels);
}

fn compact_index_value_to_string(value: IndexFieldValue<'_>) -> String {
    match value {
        IndexFieldValue::Text(value) => value.to_string(),
        IndexFieldValue::U8(value) => value.to_string(),
        IndexFieldValue::U16(value) => value.to_string(),
        IndexFieldValue::U32(value) => value.to_string(),
        IndexFieldValue::U64(value) => value.to_string(),
        IndexFieldValue::IpAddr(value) => value.to_string(),
    }
}

fn normalized_record_field_value<'a>(record: &'a FlowRecord, field: &str) -> Cow<'a, str> {
    Cow::Borrowed(
        record
            .fields
            .get(field)
            .map(String::as_str)
            .unwrap_or_default(),
    )
}

fn group_key_from_labels(labels: &BTreeMap<String, String>) -> GroupKey {
    GroupKey(
        labels
            .iter()
            .map(|(name, value)| (name.clone(), value.clone()))
            .collect(),
    )
}

#[cfg(test)]
fn accumulate_grouped_labels(
    labels: BTreeMap<String, String>,
    timestamp_usec: u64,
    metrics: FlowMetrics,
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let key = group_key_from_labels(&labels);
    if let Some(entry) = aggregates.get_mut(&key) {
        update_aggregate_entry_from_metrics(entry, timestamp_usec, metrics);
        return;
    }

    if aggregates.len() >= max_groups {
        let entry = overflow
            .aggregate
            .get_or_insert_with(new_overflow_aggregate);
        overflow.dropped_records = overflow.dropped_records.saturating_add(1);
        merge_grouped_labels(entry, &labels);
        update_aggregate_entry_from_metrics(entry, timestamp_usec, metrics);
        return;
    }

    let mut entry = AggregatedFlow {
        labels,
        first_ts: timestamp_usec,
        last_ts: timestamp_usec,
        ..AggregatedFlow::default()
    };
    update_aggregate_entry_from_metrics(&mut entry, timestamp_usec, metrics);
    aggregates.insert(key, entry);
}

#[cfg(test)]
fn update_aggregate_entry_from_metrics(
    entry: &mut AggregatedFlow,
    timestamp_usec: u64,
    metrics: FlowMetrics,
) {
    if entry.first_ts == 0 || timestamp_usec < entry.first_ts {
        entry.first_ts = timestamp_usec;
    }
    if timestamp_usec > entry.last_ts {
        entry.last_ts = timestamp_usec;
    }
    entry.metrics.add(metrics);
}

#[cfg(test)]
fn open_tier_row_labels(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
) -> BTreeMap<String, String> {
    let mut labels = BTreeMap::new();
    for field in group_by {
        labels.insert(
            field.clone(),
            open_tier_row_field_value(row, tier_flow_indexes, field).unwrap_or_default(),
        );
    }
    labels
}

#[cfg(test)]
fn sampled_metrics_from_open_tier_row(row: &OpenTierRow, _: &TierFlowIndexStore) -> FlowMetrics {
    FlowMetrics {
        bytes: row.metrics.bytes,
        packets: row.metrics.packets,
    }
}

#[cfg(test)]
fn sampled_metric_value_from_open_tier_row(
    sort_by: SortBy,
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
) -> u64 {
    sort_by.metric(sampled_metrics_from_open_tier_row(row, tier_flow_indexes))
}

#[cfg(test)]
fn accumulate_open_tier_timeseries_grouped_record(
    row: &OpenTierRow,
    tier_flow_indexes: &TierFlowIndexStore,
    group_by: &[String],
    aggregates: &mut HashMap<GroupKey, AggregatedFlow>,
    overflow: &mut GroupOverflow,
    max_groups: usize,
) {
    let labels = open_tier_row_labels(row, tier_flow_indexes, group_by);
    let metrics = sampled_metrics_from_open_tier_row(row, tier_flow_indexes);
    accumulate_grouped_labels(
        labels,
        row.timestamp_usec,
        metrics,
        aggregates,
        overflow,
        max_groups,
    );
}

fn compare_aggregated(a: &AggregatedFlow, b: &AggregatedFlow, sort_by: SortBy) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
}

fn compare_compact_aggregated(
    a: &CompactAggregatedFlow,
    b: &CompactAggregatedFlow,
    sort_by: SortBy,
) -> Ordering {
    sort_by
        .metric(b.metrics)
        .cmp(&sort_by.metric(a.metrics))
        .then_with(|| b.metrics.bytes.cmp(&a.metrics.bytes))
        .then_with(|| b.metrics.packets.cmp(&a.metrics.packets))
}

fn rank_compact_aggregates(
    aggregates: Vec<CompactAggregatedFlow>,
    overflow: Option<CompactAggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
    group_by: &[String],
    index: &FlowIndex,
) -> Result<RankedCompactAggregates> {
    let mut grouped = aggregates;
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_compact_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        other = Some(merge_other_compact_bucket(rest, group_by, index)?);
    }

    Ok(RankedCompactAggregates {
        rows,
        other,
        truncated,
        other_count,
    })
}

fn rank_projected_compact_aggregates(
    aggregates: Vec<CompactAggregatedFlow>,
    overflow: Option<CompactAggregatedFlow>,
    sort_by: SortBy,
    limit: usize,
    group_by: &[String],
    fields: &ProjectedFieldTable,
) -> Result<RankedCompactAggregates> {
    let mut grouped = aggregates;
    if let Some(overflow_row) = overflow {
        grouped.push(overflow_row);
    }
    grouped.sort_by(|a, b| compare_compact_aggregated(a, b, sort_by));

    let limit = sanitize_explicit_limit(limit);
    let truncated = grouped.len() > limit;
    let mut other_count = 0usize;
    let mut rows = grouped;
    let mut other = None;
    if truncated {
        let rest = rows.split_off(limit);
        other_count = rest.len();
        other = Some(merge_other_projected_compact_bucket(
            rest, group_by, fields,
        )?);
    }

    Ok(RankedCompactAggregates {
        rows,
        other,
        truncated,
        other_count,
    })
}

#[cfg(test)]
fn merge_other_bucket(rows: Vec<AggregatedFlow>) -> AggregatedFlow {
    let mut other = new_bucket_aggregate(OTHER_BUCKET_LABEL);

    for row in rows {
        merge_aggregate_grouped_labels(&mut other, &row);
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    other
}

fn merge_other_compact_bucket(
    rows: Vec<CompactAggregatedFlow>,
    group_by: &[String],
    index: &FlowIndex,
) -> Result<CompactAggregatedFlow> {
    let mut other = CompactAggregatedFlow::new_other();
    for row in rows {
        merge_compact_grouped_labels(&mut other, group_by, index, &row)?;
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    Ok(other)
}

fn merge_other_projected_compact_bucket(
    rows: Vec<CompactAggregatedFlow>,
    group_by: &[String],
    fields: &ProjectedFieldTable,
) -> Result<CompactAggregatedFlow> {
    let mut other = CompactAggregatedFlow::new_other();
    for row in rows {
        merge_projected_compact_grouped_labels(&mut other, group_by, fields, &row)?;
        if other.first_ts == 0 || row.first_ts < other.first_ts {
            other.first_ts = row.first_ts;
        }
        if row.last_ts > other.last_ts {
            other.last_ts = row.last_ts;
        }
        other.metrics.add(row.metrics);
    }
    Ok(other)
}

fn synthetic_aggregate_from_compact(agg: CompactAggregatedFlow) -> Result<AggregatedFlow> {
    let bucket_label = agg
        .bucket_label
        .context("missing bucket label for synthetic compact aggregate")?;

    Ok(AggregatedFlow {
        labels: synthetic_bucket_labels(bucket_label),
        first_ts: agg.first_ts,
        last_ts: agg.last_ts,
        metrics: agg.metrics,
        folded_labels: agg.folded_labels,
    })
}

fn flow_value_from_aggregate(agg: AggregatedFlow) -> Value {
    let mut flow_obj = Map::new();
    let mut labels = agg.labels;
    if let Some(folded_labels) = &agg.folded_labels {
        folded_labels.render_into(&mut labels);
    }
    flow_obj.insert("key".to_string(), json!(labels));
    flow_obj.insert("metrics".to_string(), agg.metrics.to_value());
    Value::Object(flow_obj)
}
