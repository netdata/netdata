use std::collections::HashMap;
use std::collections::hash_map::Entry;
use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use bridge::config::AuthConfig;
use file_registry::{MonotonicClock, ServiceStream, TenantId};
use opentelemetry_proto::tonic::collector::logs::v1::{
    ExportLogsPartialSuccess, ExportLogsServiceRequest, ExportLogsServiceResponse,
    logs_service_server::LogsService,
};
use opentelemetry_proto::tonic::common::v1::any_value::Value;
use opentelemetry_proto::tonic::logs::v1::ResourceLogs;
use tonic::{Request, Response, Status};

use crate::arrow_bridge;
use crate::ledger_sender::LedgerSender;

/// Extract the `(service.namespace, service.name)` stream identity from a
/// `ResourceLogs`.
///
/// An absent attribute and an empty-string attribute collapse to the same
/// empty-string field — they identify the same stream (see
/// [`ServiceStream::ns_hash`]). The `ns_hash` is derived from the returned
/// [`ServiceStream`] at the point it is needed, not stored alongside it.
fn extract_stream(rl: &ResourceLogs) -> ServiceStream {
    let attrs = match rl.resource.as_ref() {
        Some(r) => &r.attributes,
        None => return ServiceStream::new("", ""),
    };

    let mut namespace: &str = "";
    let mut name: &str = "";

    for kv in attrs {
        match kv.key.as_str() {
            "service.namespace" => {
                if let Some(Value::StringValue(s)) =
                    kv.value.as_ref().and_then(|v| v.value.as_ref())
                {
                    namespace = s.as_str();
                }
            }
            "service.name" => {
                if let Some(Value::StringValue(s)) =
                    kv.value.as_ref().and_then(|v| v.value.as_ref())
                {
                    name = s.as_str();
                }
            }
            _ => {}
        }
    }

    ServiceStream::new(namespace, name)
}

/// Total number of log records carried by a single `ResourceLogs`.
fn count_log_records(rl: &ResourceLogs) -> usize {
    rl.scope_logs.iter().map(|sl| sl.log_records.len()).sum()
}

/// Compute the `(min, max)` log-data time range for a group of `ResourceLogs`.
///
/// Mirrors the OTel timestamp hierarchy used by wal-otap's shared
/// frame decode, so the WAL's accumulated range matches what the
/// indexer will eventually compute for the file's SFST summary:
///
/// 1. `time_unix_nano` (event time) when non-zero.
/// 2. `observed_time_unix_nano` when `time_unix_nano` is zero or missing.
/// 3. `ingestion_ns + row_idx` as the last-resort fallback, where
///    `row_idx` is the row's position across all `ResourceLogs` in the
///    group. This matches the indexer's tier-3 behaviour exactly.
///
/// # Panics
///
/// Panics if `rls` carries zero log records. Callers in `export()` are
/// guaranteed to hand in a non-empty group: `export()` filters
/// `ResourceLogs` with no records and short-circuits when no accepted
/// group remains, so by the time this function runs, every group has at
/// least one row.
fn compute_log_ts_range(
    rls: &[ResourceLogs],
    ingestion_ns: file_registry::TimestampNs,
) -> (file_registry::TimestampNs, file_registry::TimestampNs) {
    let mut min: Option<u64> = None;
    let mut max: Option<u64> = None;
    let mut row_idx: u64 = 0;

    for rl in rls {
        for sl in &rl.scope_logs {
            for lr in &sl.log_records {
                let ts = if lr.time_unix_nano != 0 {
                    lr.time_unix_nano
                } else if lr.observed_time_unix_nano != 0 {
                    lr.observed_time_unix_nano
                } else {
                    ingestion_ns.0 + row_idx
                };
                row_idx += 1;
                min = Some(min.map_or(ts, |m| m.min(ts)));
                max = Some(max.map_or(ts, |m| m.max(ts)));
            }
        }
    }

    let expect_msg = "at least one log record";
    (
        file_registry::TimestampNs(min.expect(expect_msg)),
        file_registry::TimestampNs(max.expect(expect_msg)),
    )
}

/// One collision: a request group whose stream identity doesn't match the
/// canonical stream already registered for its `ns_hash`.
#[derive(Debug, Clone, PartialEq, Eq)]
struct Collision {
    hash: u64,
    canonical: ServiceStream,
    rejected: ServiceStream,
    rejected_log_records: usize,
}

/// One group of `ResourceLogs` that share an exact [`ServiceStream`]. The
/// identifying stream lives in the `HashMap` key in [`group_by_stream`].
struct StreamGroup {
    log_record_count: usize,
    resource_logs: Vec<ResourceLogs>,
}

/// Group `ResourceLogs` by their [`ServiceStream`] identity.
///
/// In normal operation, all `ResourceLogs` in a request from a single
/// service share the same stream. Streams whose `ns_hash` collides (a real
/// xxhash64 collision, or a literal-empty vs absent field — the latter now
/// collapses to one stream) are reconciled later by the canonical-table check.
fn group_by_stream(resource_logs: Vec<ResourceLogs>) -> HashMap<ServiceStream, StreamGroup> {
    let mut groups: HashMap<ServiceStream, StreamGroup> = HashMap::new();
    for rl in resource_logs {
        let stream = extract_stream(&rl);
        let count = count_log_records(&rl);
        let group = groups.entry(stream).or_insert_with(|| StreamGroup {
            log_record_count: 0,
            resource_logs: Vec::new(),
        });
        group.log_record_count += count;
        group.resource_logs.push(rl);
    }
    groups
}

/// Result of running the collision check across a request's groups.
struct CollisionCheck {
    accepted: Vec<(u64, StreamGroup)>,
    collisions: Vec<Collision>,
}

/// Reconcile a request's groups against the canonical-stream table.
///
/// For each group:
/// - If the table has no entry for `ns_hash`, register the group's stream
///   as canonical and accept the group.
/// - If the entry matches the group's stream, accept.
/// - If the entry mismatches, reject the group as a collision and record
///   it for the response's `partial_success`.
///
/// Pure with respect to the I/O of the gRPC handler — the only side
/// effect is mutating the canonical table. Extracted from `export` so it
/// can be unit-tested without spinning up a writer or a tonic Request.
fn check_collisions(
    canonical: &mut HashMap<(TenantId, u64), ServiceStream>,
    tenant_id: &TenantId,
    groups: HashMap<ServiceStream, StreamGroup>,
) -> CollisionCheck {
    let mut accepted = Vec::new();
    let mut collisions = Vec::new();

    for (stream, group) in groups {
        let hash = stream.ns_hash();
        let key = (tenant_id.clone(), hash);
        match canonical.entry(key) {
            Entry::Vacant(e) => {
                e.insert(stream.clone());
                accepted.push((hash, group));
            }
            Entry::Occupied(e) if *e.get() == stream => {
                accepted.push((hash, group));
            }
            Entry::Occupied(e) => {
                collisions.push(Collision {
                    hash,
                    canonical: e.get().clone(),
                    rejected: stream,
                    rejected_log_records: group.log_record_count,
                });
            }
        }
    }

    CollisionCheck {
        accepted,
        collisions,
    }
}

/// Build the OTLP `partial_success` payload from the collision list.
///
/// Returns `None` when the list is empty (no collisions = full success);
/// otherwise builds a payload that reports the total rejected count and a
/// developer-facing message naming each colliding pair.
fn build_partial_success(collisions: &[Collision]) -> Option<ExportLogsPartialSuccess> {
    if collisions.is_empty() {
        return None;
    }
    let total: i64 = collisions
        .iter()
        .map(|c| c.rejected_log_records as i64)
        .sum();
    Some(ExportLogsPartialSuccess {
        rejected_log_records: total,
        error_message: format_collision_error(collisions),
    })
}

/// Format collision details for `ExportLogsPartialSuccess::error_message`.
fn format_collision_error(collisions: &[Collision]) -> String {
    fn show(s: &str) -> &str {
        if s.is_empty() { "<none>" } else { s }
    }

    let parts: Vec<String> = collisions
        .iter()
        .map(|c| {
            format!(
                "ns_hash={:#x}: rejected ({}/{}) collides with canonical ({}/{}) ({} log records dropped)",
                c.hash,
                show(&c.rejected.namespace),
                show(&c.rejected.name),
                show(&c.canonical.namespace),
                show(&c.canonical.name),
                c.rejected_log_records,
            )
        })
        .collect();
    format!(
        "{} ns_hash collision{} detected; rename one of the colliding (service.namespace, service.name) pairs to dedupe: {}",
        collisions.len(),
        if collisions.len() == 1 { "" } else { "s" },
        parts.join("; "),
    )
}

fn extract_tenant_id(
    metadata: &tonic::metadata::MetadataMap,
    auth: &AuthConfig,
) -> Result<TenantId, Status> {
    if !auth.enabled {
        return Ok(TenantId::default_tenant());
    }
    let value = metadata
        .get(AuthConfig::TENANT_HEADER)
        .ok_or_else(|| Status::unauthenticated("missing tenant header"))?;
    let tenant = value
        .to_str()
        .map_err(|_| Status::invalid_argument("tenant header must be valid UTF-8"))?;
    // The id policy (strict: becomes a directory name) lives on the
    // type; this layer only maps the reason onto the transport error.
    TenantId::validate_ingest(tenant).map_err(Status::invalid_argument)
}

pub struct NetdataLogsService {
    writers: Mutex<HashMap<TenantId, wal::Writer>>,
    /// Canonical [`ServiceStream`] per `(tenant, ns_hash)`. First write
    /// wins; subsequent writes whose stream doesn't match are rejected via
    /// `partial_success`. In-memory only — on restart the table is empty and
    /// the first write of a tenant's stream re-establishes the canonical
    /// stream.
    canonical: Mutex<HashMap<(TenantId, u64), ServiceStream>>,
    /// Process-wide monotonic clock. Provides the per-frame `ingestion_ns`
    /// stamped on disk by the WAL writer and the same value the indexer
    /// will use as its tier-3 fallback for log rows missing both
    /// `time_unix_nano` and `observed_time_unix_nano`. Sharing a single
    /// clock across all tenants and streams keeps `ingestion_ns`
    /// monotonic globally within this process.
    clock: Mutex<MonotonicClock>,
    sender: LedgerSender,
    wal_base_dir: PathBuf,
    wal_config: bridge::config::WalConfig,
    seq: Arc<wal::SeqAllocator>,
    auth: AuthConfig,
}

impl NetdataLogsService {
    pub fn new(
        sender: LedgerSender,
        wal_base_dir: PathBuf,
        wal_config: bridge::config::WalConfig,
        seq: Arc<wal::SeqAllocator>,
        auth: AuthConfig,
    ) -> Self {
        Self {
            writers: Mutex::new(HashMap::new()),
            canonical: Mutex::new(HashMap::new()),
            clock: Mutex::new(MonotonicClock::new()),
            sender,
            wal_base_dir,
            wal_config,
            seq,
            auth,
        }
    }

    fn resolve_wal_config(&self, tenant_id: &str) -> wal::Config {
        let rotation =
            bridge::config::RotationConfig::resolve(&self.wal_config.rotation, tenant_id);
        wal::Config {
            rotation: wal::RotationConfig {
                max_log_entries: rotation.max_log_entries,
                max_file_size: file_registry::ByteSize(rotation.max_file_size.as_u64()),
                max_duration: Some(rotation.max_file_duration),
            },
            crc_enabled: self.wal_config.crc_enabled,
            compression_enabled: self.wal_config.compression_enabled,
        }
    }
}

#[tonic::async_trait]
impl LogsService for NetdataLogsService {
    #[tracing::instrument(skip_all, fields(received_logs))]
    async fn export(
        &self,
        request: Request<ExportLogsServiceRequest>,
    ) -> Result<Response<ExportLogsServiceResponse>, Status> {
        let tenant_id = extract_tenant_id(request.metadata(), &self.auth)?;
        let req = request.into_inner();

        // Drop ResourceLogs that carry zero log records.
        let resource_logs: Vec<ResourceLogs> = req
            .resource_logs
            .into_iter()
            .filter(|rl| count_log_records(rl) > 0)
            .collect();

        if resource_logs.is_empty() {
            return Ok(Response::new(ExportLogsServiceResponse {
                partial_success: None,
            }));
        }

        // Group, then run the collision check.
        let groups = group_by_stream(resource_logs);
        let CollisionCheck {
            accepted,
            collisions,
        } = {
            let mut canonical = self.canonical.lock().unwrap();
            check_collisions(&mut canonical, &tenant_id, groups)
        };

        for c in &collisions {
            tracing::warn!(
                tenant = %tenant_id,
                hash = c.hash,
                "ns_hash collision: rejecting {} log records",
                c.rejected_log_records,
            );
        }

        // No-op short-circuit: every group was rejected as a collision,
        // so there's nothing to write.
        if accepted.is_empty() {
            return Ok(Response::new(ExportLogsServiceResponse {
                partial_success: build_partial_success(&collisions),
            }));
        }

        let mut writers = self.writers.lock().unwrap();
        let writer = if let Some(w) = writers.get_mut(&tenant_id) {
            w
        } else {
            let path = self.wal_base_dir.join(tenant_id.as_str());
            let wal_config = self.resolve_wal_config(tenant_id.as_str());
            let w = wal::Writer::new(&path, wal_config, Arc::clone(&self.seq)).map_err(|e| {
                tracing::error!(%e, tenant = %tenant_id, "failed to create WAL writer");
                Status::internal("WAL writer creation failed")
            })?;
            writers.entry(tenant_id.clone()).or_insert(w)
        };

        for (ns_hash, group) in accepted {
            // One clock tick per frame. The same value flows into the
            // WAL frame header (`ingestion_ns`), into the indexer's
            // tier-3 fallback during indexing, and into
            // `compute_log_ts_range` so the WAL summary matches the
            // eventual SFST summary numerically.
            let ingestion_ns = self.clock.lock().unwrap().now_ns();
            let (log_min_ts, log_max_ts) = compute_log_ts_range(&group.resource_logs, ingestion_ns);
            let (data, count) = arrow_bridge::encode(group.resource_logs).map_err(|e| {
                tracing::error!(%e, "failed to encode Arrow");
                Status::internal("Arrow encode error")
            })?;

            writer
                .write_frame(ns_hash, &data, count, ingestion_ns, log_min_ts, log_max_ts)
                .map_err(|e| {
                    tracing::error!(%e, "failed to write WAL entry");
                    Status::internal("WAL write error")
                })?;
        }

        writer.sync_all().map_err(|e| {
            tracing::error!(%e, "failed to sync WAL");
            Status::internal("WAL sync error")
        })?;

        let events = writer.take_all_events();
        self.sender.send_events(tenant_id, events);

        Ok(Response::new(ExportLogsServiceResponse {
            partial_success: build_partial_success(&collisions),
        }))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use opentelemetry_proto::tonic::common::v1::{AnyValue, KeyValue};
    use opentelemetry_proto::tonic::logs::v1::{LogRecord, ScopeLogs};
    use opentelemetry_proto::tonic::resource::v1::Resource;

    /// Build a `ResourceLogs` with `log_count` records, each carrying the
    /// timestamps from the corresponding `(time_unix_nano, observed_time_unix_nano)`
    /// pair in `tss`.
    fn rl_with_ts(namespace: Option<&str>, name: Option<&str>, tss: &[(u64, u64)]) -> ResourceLogs {
        let mut base = rl(namespace, name, 0);
        let records: Vec<LogRecord> = tss
            .iter()
            .map(|&(t, ot)| LogRecord {
                time_unix_nano: t,
                observed_time_unix_nano: ot,
                ..LogRecord::default()
            })
            .collect();
        base.scope_logs[0].log_records = records;
        base
    }

    fn rl(namespace: Option<&str>, name: Option<&str>, log_count: usize) -> ResourceLogs {
        let mut attrs = Vec::new();
        if let Some(ns) = namespace {
            attrs.push(KeyValue {
                key: "service.namespace".to_string(),
                value: Some(AnyValue {
                    value: Some(Value::StringValue(ns.to_string())),
                }),
            });
        }
        if let Some(n) = name {
            attrs.push(KeyValue {
                key: "service.name".to_string(),
                value: Some(AnyValue {
                    value: Some(Value::StringValue(n.to_string())),
                }),
            });
        }
        ResourceLogs {
            resource: Some(Resource {
                attributes: attrs,
                dropped_attributes_count: 0,
                entity_refs: Vec::new(),
            }),
            scope_logs: vec![ScopeLogs {
                scope: None,
                log_records: (0..log_count).map(|_| LogRecord::default()).collect(),
                schema_url: String::new(),
            }],
            schema_url: String::new(),
        }
    }

    fn tenant() -> TenantId {
        TenantId::from("t1")
    }

    /// All compute_log_ts_range tests use a fixed ingestion_ns far
    /// outside the range of "real" timestamps in the fixtures so it's
    /// easy to tell tier-3 fallback values apart from the rows' own.
    const TEST_INGESTION_NS: u64 = 1_000_000;

    fn ingestion() -> file_registry::TimestampNs {
        file_registry::TimestampNs(TEST_INGESTION_NS)
    }

    #[test]
    fn compute_log_ts_range_uses_time_unix_nano_first() {
        let rls = vec![rl_with_ts(
            Some("prod"),
            Some("api"),
            &[(100, 999), (200, 999), (50, 999)],
        )];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(50));
        assert_eq!(max, file_registry::TimestampNs(200));
    }

    #[test]
    fn compute_log_ts_range_falls_back_to_observed() {
        let rls = vec![rl_with_ts(
            Some("prod"),
            Some("api"),
            &[(0, 100), (0, 300), (0, 200)],
        )];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(100));
        assert_eq!(max, file_registry::TimestampNs(300));
    }

    #[test]
    fn compute_log_ts_range_synthesizes_fallback_per_row() {
        // Two rows with explicit timestamps, two without. The two
        // missing rows must get `ingestion_ns + row_idx` (their position
        // across the whole iteration), exactly mirroring the indexer's
        // tier-3 fallback. row_idx values: 0, 1, 2, 3.
        // Rows: (0,0)→fallback at 1_000_000+0
        //       (100,0)→100
        //       (0,0)→fallback at 1_000_000+2
        //       (50,0)→50
        let rls = vec![rl_with_ts(
            Some("prod"),
            Some("api"),
            &[(0, 0), (100, 0), (0, 0), (50, 0)],
        )];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(50));
        assert_eq!(max, file_registry::TimestampNs(TEST_INGESTION_NS + 2));
    }

    #[test]
    fn compute_log_ts_range_all_missing_uses_ingestion_plus_row_idx() {
        // No row carries a timestamp. The synthesized fallbacks span
        // [ingestion_ns, ingestion_ns + N - 1] for N rows.
        let rls = vec![rl_with_ts(Some("prod"), Some("api"), &[(0, 0); 5])];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(TEST_INGESTION_NS));
        assert_eq!(max, file_registry::TimestampNs(TEST_INGESTION_NS + 4));
    }

    #[test]
    #[should_panic(expected = "at least one log record")]
    fn compute_log_ts_range_panics_on_empty_input() {
        // The function's precondition is documented and codified at
        // every call site (see the `debug_assert!` in `export()`). The
        // panic guards against silent breakage if a future caller ever
        // hands in an empty group.
        let rls: Vec<ResourceLogs> = Vec::new();
        let _ = compute_log_ts_range(&rls, ingestion());
    }

    #[test]
    #[should_panic(expected = "at least one log record")]
    fn compute_log_ts_range_panics_when_all_resource_logs_have_no_records() {
        // Same precondition: a group of `ResourceLogs` whose `scope_logs`
        // are all empty also has zero log records and must not reach
        // this function.
        let rls = vec![rl(Some("prod"), Some("api"), 0)];
        let _ = compute_log_ts_range(&rls, ingestion());
    }

    #[test]
    fn compute_log_ts_range_mixes_time_and_observed_per_row() {
        // Row 1 uses time_unix_nano=200; row 2 falls back to observed=50.
        let rls = vec![rl_with_ts(
            Some("prod"),
            Some("api"),
            &[(200, 999), (0, 50)],
        )];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(50));
        assert_eq!(max, file_registry::TimestampNs(200));
    }

    #[test]
    fn compute_log_ts_range_aggregates_across_resource_logs() {
        let rls = vec![
            rl_with_ts(Some("a"), Some("b"), &[(100, 0), (200, 0)]),
            rl_with_ts(Some("c"), Some("d"), &[(50, 0), (300, 0)]),
        ];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(50));
        assert_eq!(max, file_registry::TimestampNs(300));
    }

    #[test]
    fn compute_log_ts_range_row_idx_continues_across_resource_logs() {
        // Indexer tier-3 increments row_idx across all rows in the
        // group, not per-ResourceLogs. The fallback row in the second
        // ResourceLogs must use row_idx = 2 (after two rows in rls[0]).
        let rls = vec![
            rl_with_ts(Some("a"), Some("b"), &[(100, 0), (200, 0)]),
            rl_with_ts(Some("c"), Some("d"), &[(0, 0)]),
        ];
        let (min, max) = compute_log_ts_range(&rls, ingestion());
        assert_eq!(min, file_registry::TimestampNs(100));
        assert_eq!(max, file_registry::TimestampNs(TEST_INGESTION_NS + 2));
    }

    #[test]
    fn extract_stream_pulls_namespace_and_name_from_resource_attrs() {
        let s = extract_stream(&rl(Some("prod"), Some("api"), 0));
        assert_eq!(s, ServiceStream::new("prod", "api"));
        assert_eq!(
            s.ns_hash(),
            file_registry::compute_ns_hash(Some("prod"), Some("api"))
        );
    }

    #[test]
    fn extract_stream_handles_missing_attrs() {
        let s = extract_stream(&rl(None, None, 0));
        assert_eq!(s, ServiceStream::new("", ""));
        assert_eq!(s.ns_hash(), file_registry::compute_ns_hash(None, None));
    }

    #[test]
    fn extract_stream_merges_missing_and_empty() {
        // Absent and empty-string attributes identify the same stream:
        // both collapse to an empty-field ServiceStream and the same
        // ns_hash, so the canonical table treats them as one.
        let absent = extract_stream(&rl(None, None, 0));
        let empty = extract_stream(&rl(Some(""), Some(""), 0));
        assert_eq!(absent, empty);
        assert_eq!(absent, ServiceStream::new("", ""));
        assert_eq!(absent.ns_hash(), empty.ns_hash());
    }

    #[test]
    fn group_merges_resource_logs_with_identical_stream() {
        let groups = group_by_stream(vec![
            rl(Some("prod"), Some("api"), 3),
            rl(Some("prod"), Some("api"), 5),
            rl(Some("prod"), Some("worker"), 1),
        ]);
        assert_eq!(groups.len(), 2);
        let api = groups.get(&ServiceStream::new("prod", "api")).unwrap();
        assert_eq!(api.log_record_count, 8);
        assert_eq!(api.resource_logs.len(), 2);
    }

    #[test]
    fn first_write_establishes_canonical_pair() {
        let mut canonical = HashMap::new();
        let groups = group_by_stream(vec![rl(Some("prod"), Some("api"), 4)]);
        let r = check_collisions(&mut canonical, &tenant(), groups);
        assert_eq!(r.accepted.len(), 1);
        assert!(r.collisions.is_empty());
        let stream = canonical.get(&(tenant(), r.accepted[0].0)).unwrap();
        assert_eq!(stream, &ServiceStream::new("prod", "api"));
    }

    #[test]
    fn matching_subsequent_writes_pass_through() {
        let mut canonical = HashMap::new();
        let r1 = check_collisions(
            &mut canonical,
            &tenant(),
            group_by_stream(vec![rl(Some("prod"), Some("api"), 1)]),
        );
        assert!(r1.collisions.is_empty());
        let r2 = check_collisions(
            &mut canonical,
            &tenant(),
            group_by_stream(vec![rl(Some("prod"), Some("api"), 7)]),
        );
        assert!(r2.collisions.is_empty());
        assert_eq!(r2.accepted.len(), 1);
        assert_eq!(r2.accepted[0].1.log_record_count, 7);
    }

    #[test]
    fn synthetic_collision_is_rejected() {
        // Two genuinely different streams hashing to the same u64 is
        // impossible to construct naturally — xxhash64 collisions are
        // vanishingly rare. We simulate one by pre-seeding the canonical
        // table at the *rejected* stream's own hash slot with a different
        // canonical stream (deliberately not that slot's natural occupant),
        // as if an earlier colliding stream had claimed it. The incoming
        // group then hashes to the occupied slot with a mismatching identity
        // and must be rejected — exercising the Occupied-mismatch arm.
        let mut canonical = HashMap::new();
        let rejected = ServiceStream::new("staging", "api");
        let hash = rejected.ns_hash();
        let canonical_stream = ServiceStream::new("prod", "api");
        canonical.insert((tenant(), hash), canonical_stream.clone());

        let mut groups = HashMap::new();
        groups.insert(
            rejected.clone(),
            StreamGroup {
                log_record_count: 12,
                resource_logs: Vec::new(),
            },
        );

        let r = check_collisions(&mut canonical, &tenant(), groups);
        assert!(r.accepted.is_empty());
        assert_eq!(r.collisions.len(), 1);
        let c = &r.collisions[0];
        assert_eq!(c.hash, hash);
        assert_eq!(c.canonical, canonical_stream);
        assert_eq!(c.rejected, rejected);
        assert_eq!(c.rejected_log_records, 12);
    }

    #[test]
    fn collision_does_not_overwrite_canonical_pair() {
        let mut canonical = HashMap::new();
        let rejected = ServiceStream::new("staging", "api");
        let hash = rejected.ns_hash();
        let canonical_stream = ServiceStream::new("prod", "api");
        canonical.insert((tenant(), hash), canonical_stream.clone());

        let mut groups = HashMap::new();
        groups.insert(
            rejected,
            StreamGroup {
                log_record_count: 1,
                resource_logs: Vec::new(),
            },
        );

        let _ = check_collisions(&mut canonical, &tenant(), groups);
        // The original canonical stream must remain unchanged.
        assert_eq!(
            canonical.get(&(tenant(), hash)).unwrap(),
            &canonical_stream
        );
    }

    #[test]
    fn tenants_have_independent_canonical_tables() {
        let mut canonical = HashMap::new();
        let t1 = TenantId::from("t1");
        let t2 = TenantId::from("t2");
        let groups_t1 = group_by_stream(vec![rl(Some("prod"), Some("api"), 1)]);
        let r1 = check_collisions(&mut canonical, &t1, groups_t1);
        assert!(r1.collisions.is_empty());

        // The same stream (hence the same ns_hash) in a different tenant
        // must be accepted as fresh, not flagged as a collision — the
        // tenant id is part of the canonical-table key.
        let groups_t2 = group_by_stream(vec![rl(Some("prod"), Some("api"), 1)]);
        let r2 = check_collisions(&mut canonical, &t2, groups_t2);
        assert!(r2.collisions.is_empty());
        assert_eq!(r2.accepted.len(), 1);
        assert_eq!(r1.accepted[0].0, r2.accepted[0].0);
    }

    /// Build a `NetdataLogsService` rooted at `wal_dir`. The
    /// `LedgerSender` points at a path that intentionally won't accept a
    /// connection — `send_events` is fire-and-forget over an internal
    /// channel, so the unconnected sender doesn't block the service. The
    /// background reconnect task gets dropped at end of test along with
    /// the tokio runtime.
    fn test_service(wal_dir: std::path::PathBuf) -> NetdataLogsService {
        let socket = format!("/tmp/netdata-ingestor-test-{}.sock", std::process::id());
        let sender = LedgerSender::new(&socket);

        let mut rotation = HashMap::new();
        rotation.insert(
            "default".to_string(),
            bridge::config::RotationEntry {
                max_file_size: Some(bytesize::ByteSize::mb(64)),
                max_log_entries: Some(100_000),
                max_file_duration: Some(std::time::Duration::from_secs(3600)),
            },
        );
        let wal_config = bridge::config::WalConfig {
            dir: wal_dir.clone(),
            crc_enabled: true,
            compression_enabled: true,
            rotation,
        };

        NetdataLogsService::new(
            sender,
            wal_dir,
            wal_config,
            Arc::new(wal::SeqAllocator::ephemeral(0)),
            bridge::config::AuthConfig::default(),
        )
    }

    #[tokio::test]
    async fn empty_request_creates_no_wal_files() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        let req = Request::new(ExportLogsServiceRequest {
            resource_logs: vec![],
        });
        let resp = service.export(req).await.unwrap();

        let entries: Vec<_> = std::fs::read_dir(&wal_dir).unwrap().collect();
        assert!(
            entries.is_empty(),
            "no tenant directory should be created on a fully empty request"
        );
        assert!(resp.into_inner().partial_success.is_none());
    }

    #[tokio::test]
    async fn empty_resource_logs_create_no_wal_files() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // Two ResourceLogs with stream identity but zero log records.
        // Pre-fix, each of these would have claimed a canonical pair and
        // produced a WAL file with a zero-entry frame.
        let req = Request::new(ExportLogsServiceRequest {
            resource_logs: vec![
                rl(Some("prod"), Some("api"), 0),
                rl(Some("prod"), Some("worker"), 0),
            ],
        });
        let resp = service.export(req).await.unwrap();

        let entries: Vec<_> = std::fs::read_dir(&wal_dir).unwrap().collect();
        assert!(
            entries.is_empty(),
            "empty ResourceLogs must not create any WAL files"
        );
        assert!(resp.into_inner().partial_success.is_none());
    }

    #[tokio::test]
    async fn empty_resource_logs_do_not_claim_canonical_pair() {
        let tmp = tempfile::tempdir().unwrap();
        let wal_dir = tmp.path().to_path_buf();
        let service = test_service(wal_dir.clone());

        // First request: empty ResourceLogs for `(prod, api)`. Pre-fix,
        // this would claim the canonical pair for that ns_hash.
        let _ = service
            .export(Request::new(ExportLogsServiceRequest {
                resource_logs: vec![rl(Some("prod"), Some("api"), 0)],
            }))
            .await
            .unwrap();

        // The canonical table should be empty — no claim was made.
        let canonical = service.canonical.lock().unwrap();
        assert!(
            canonical.is_empty(),
            "empty request must not have claimed a canonical (ns, name) pair"
        );
    }

    #[test]
    fn error_message_describes_each_collision() {
        let collisions = vec![
            Collision {
                hash: 0x1,
                canonical: ServiceStream::new("prod", "api"),
                rejected: ServiceStream::new("staging", "api"),
                rejected_log_records: 3,
            },
            Collision {
                hash: 0x2,
                canonical: ServiceStream::new("", "worker"),
                rejected: ServiceStream::new("dev", "worker"),
                rejected_log_records: 1,
            },
        ];
        let msg = format_collision_error(&collisions);
        assert!(msg.contains("2 ns_hash collisions"));
        assert!(msg.contains("prod"));
        assert!(msg.contains("staging"));
        // An empty (absent) field renders as `<none>`.
        assert!(msg.contains("<none>"));
        assert!(msg.contains("3 log records"));
    }
}
