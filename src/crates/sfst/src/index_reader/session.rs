//! The per-file trace query session — the batch primitive of the
//! cross-source engine and the substrate of the single-file
//! [`IndexReader::trace_by_id`].
//!
//! One session per open file serves many trace-id lookups, decoding each
//! structure at most once while it stays loaded:
//!
//! - the TBLM bloom is decoded on open and tested per id (a corrupt bloom
//!   degrades to the exact lookup with ONE demoted warning per session,
//!   not one per lookup);
//! - a screened session ([`TraceFileSession::screened`], for a caller
//!   that looks up many ids in one file) whose file has no tight bloom (a
//!   file sealed at the former 5% target, or a missing or corrupt bloom)
//!   builds one from the lookup structures the first time it decodes them,
//!   so later false positives cost no decode;
//! - the lookup structures (TIDX + TRCE) decode on the first id the
//!   filters let through, the per-row columns (SPAN/PSPN/DURN/FLAG/DRAC +
//!   timestamps) only on the first lookup that finds rows, and a probe
//!   that finds nothing drops what it decoded;
//! - an owned session ([`TraceFileSession::open_owned`]) starts from the
//!   file's metadata view and opens the full reader (`PRIM`, for `_kind`
//!   and string resolution) only once a lookup finds rows;
//! - stream batches, EVNB/LNKB and the trace rollup (TRSU) decode lazily,
//!   once;
//! - KvId→string resolutions accumulate in a per-session cache; a value
//!   longer than [`CACHED_VALUE_MAX_BYTES`] is resolved whole for the span
//!   that needs it and not kept. Values are never cut: predicates and
//!   canonical selection see what was stored, in every layout.
//!
//! Two layers: the bloom and the screen stay for the session's life
//! (about 3 bytes per distinct trace id in the file: ~30 KB for 10k);
//! everything else is the LOADED layer, which
//! [`release`](SpanSource::release) drops. A caller holding many
//! sessions keeps only a few loaded
//! ([`LoadedSources`](crate::LoadedSources)); a released session decodes
//! again on its next use and answers the same.
//!
//! The session implements [`SpanSource`], so the shared combiner
//! ([`crate::trace_combine::combine`]) drives it: cheap [`SpanRef`]s up
//! front (no row materialization — `start_ns` from the timestamps column,
//! `kind` from the low-card `_kind` facet bitmaps), full spans only when
//! the merge accepts (or must tie-break) them.
//!
//! Fallibility is per-source, never a panic: a file contradicting its
//! own bounds raises `CorruptIndex` (position ≥ `record_count`, no
//! stream batch for a position, a row past its batch's length, a
//! missing timestamp or column value); the combiner records it in
//! `CombineOutcome::failures` and drops that source's remaining
//! candidates, and `trace_by_id` — one file being the whole result —
//! propagates it. The plan (`trace_plan.rs`) mirrors these re-checks on
//! its side (column lengths against each range). `span_refs` alone
//! raises `UnsetTraceId` — this module holds the variant's only raise
//! site; `trace_by_id` resolves UNSET to the documented empty before
//! any session is opened.

use std::collections::HashMap;

use crate::index_reader::{
    IndexReader, MetaReader, RollupRefOutcome, RollupRefResolver, TraceEvent, TraceLink, TraceSpan,
    kv_attr, kv_value,
};
use crate::reader::ChunkReader;
use crate::trace_combine::{SpanRef, SpanSource};
use crate::{RollupRow, TraceId, TraceIdBloom, TraceRollup};

/// The bloom gate, resolved once per session.
enum BloomGate {
    /// No bloom chunk: every id falls through to the exact lookup.
    Pass,
    /// The bloom failed to decode (warned once): every id falls through
    /// to the exact lookup, and a caller can see the file is damaged.
    Unreadable,
    Filter(crate::TraceIdBloom),
}

/// The file a session reads through. A borrowed full reader (search
/// shares one per file), or an owned view: the metadata view until a
/// lookup finds the trace, then the full reader, whose `PRIM` decode
/// only a file holding the trace pays for.
enum View<'r, 'a> {
    Borrowed(&'r IndexReader<'a>),
    Meta(Box<MetaReader<'a>>),
    Full(Box<IndexReader<'a>>),
    /// The upgrade to the full reader failed; its error went to the
    /// caller, and every later read fails too.
    Failed,
}

impl<'a> View<'_, 'a> {
    fn chunk(&self) -> Result<&ChunkReader<'a>, crate::Error> {
        match self {
            View::Borrowed(r) => Ok(&r.sfst),
            View::Meta(m) => Ok(&m.sfst),
            View::Full(r) => Ok(&r.sfst),
            View::Failed => Err(upgrade_failed()),
        }
    }

    fn record_count(&self) -> Result<u32, crate::Error> {
        match self {
            View::Borrowed(r) => Ok(r.summary().record_count),
            View::Meta(m) => Ok(m.summary().record_count),
            View::Full(r) => Ok(r.summary().record_count),
            View::Failed => Err(upgrade_failed()),
        }
    }

    /// Downgrade an owned full reader to its metadata view (a borrowed
    /// reader is the caller's; a failed upgrade stays failed).
    fn release(&mut self) {
        if matches!(self, View::Full(_)) {
            let View::Full(full) = std::mem::replace(self, View::Failed) else {
                unreachable!("matched above");
            };
            *self = View::Meta(Box::new((*full).into_meta_reader()));
        }
    }

    /// The full reader, upgrading an owned metadata view on first use.
    fn full(&mut self) -> Result<&IndexReader<'a>, crate::Error> {
        if matches!(self, View::Meta(_)) {
            let View::Meta(meta) = std::mem::replace(self, View::Failed) else {
                unreachable!("matched above");
            };
            *self = View::Full(Box::new((*meta).into_index_reader()?));
        }
        match self {
            View::Borrowed(r) => Ok(r),
            View::Full(r) => Ok(r),
            View::Meta(_) => unreachable!("upgraded above"),
            View::Failed => Err(upgrade_failed()),
        }
    }
}

/// The longest `key=value` string the session's string cache keeps. The
/// cache saves re-resolving values that repeat across spans (service
/// names, methods, routes); a long value rarely repeats, so it is resolved
/// for the span that needs it and dropped, which keeps the cache's size a
/// function of the file's short values, not of its longest ones.
const CACHED_VALUE_MAX_BYTES: usize = 4 * 1024;

fn upgrade_failed() -> crate::Error {
    crate::Error::CorruptIndex("trace session: the file failed to open fully".into())
}

/// A screened session's query-time filter (see
/// [`TraceFileSession::screened`]).
enum Screen {
    /// Not a screened session: the bloom alone decides.
    Off,
    /// To be built on the first lookup decode.
    Pending,
    /// Built; `None` when the file holds no set trace id.
    Built(Option<TraceIdBloom>),
    /// The lookup structures failed to decode; the exact lookup decodes
    /// them again and surfaces the error.
    Unavailable,
}

/// One file's rollup row and stats for one trace id
/// ([`TraceFileSession::rollup_part`]).
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum PartEvidence {
    /// The file has no rollup chunk.
    NoRollup,
    /// The file has the rollup but no stats (sealed before `TRSS`).
    NoStats,
    /// The file holds none of the trace's spans.
    Absent,
    Part(RollupRow, crate::StatsRow),
}

/// One file's trace-rollup evidence for one trace id
/// ([`TraceFileSession::rollup_row`]).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RollupEvidence {
    /// The file has no rollup chunk (written before rollups existed).
    NoRollup,
    /// The rollup holds no row for the trace: the file holds none of its
    /// spans.
    Absent,
    Row(RollupRow),
}

/// The per-row columns a found trace's spans read — decoded together,
/// once, on the first lookup that finds rows (the timestamps are their
/// own field, shared with discovery).
struct Rows {
    span_ids: crate::SpanIds,
    parents: crate::ParentSpanIds,
    durations: crate::Durations,
    flags: crate::Flags,
    dropped: crate::DroppedAttributeCounts,
}

/// The optional event/link structures — decoded together, once, on the
/// first materialization (absent chunks mean the file's spans carry no
/// events/links — a correct empty, not a failure).
struct Extras {
    events: Option<crate::EventIndex>,
    links: Option<crate::LinkIndex>,
}

/// See the module docs. Open one per file; look up many ids.
pub struct TraceFileSession<'r, 'a> {
    view: View<'r, 'a>,
    bloom: BloomGate,
    screen: Screen,
    // The loaded layer, dropped by `release`.
    /// The trace-id index (TIDX): with `trace_ids`, the lookup pair.
    index: Option<crate::TraceIdIndex>,
    /// The per-row trace ids (TRCE), shared by lookups and discovery.
    trace_ids: Option<crate::TraceIds>,
    /// The per-row start timestamps (TIMS), shared by rows and discovery.
    timestamps: Option<crate::Timestamps>,
    rows: Option<Rows>,
    extras: Option<Extras>,
    /// Stream-batch cache, pre-sized to the file's batch count on the
    /// first `batch` call; each batch decodes on first touch.
    batches: Vec<Option<crate::StreamBatch>>,
    /// KvId → `key=value` string, accumulated across materializations.
    strings: HashMap<u32, String>,
    /// Row position → raw `_kind` int, filled per looked-up trace from
    /// the low-card facet bitmaps (no row access).
    kinds: HashMap<u32, i32>,
    rollup: Option<TraceRollup>,
    stats: Option<crate::TraceRollupStats>,
    resolver: Option<RollupRefResolver>,
    /// The caller's hook for when the session holds nothing decoded.
    on_release: Option<Box<dyn FnMut() + 'a>>,
}

impl<'r, 'a> TraceFileSession<'r, 'a> {
    /// Open a session over a full reader the caller keeps. Resolves the
    /// bloom gate (once; corrupt → one demoted warning + exact lookups);
    /// everything else decodes lazily.
    pub fn open(reader: &'r IndexReader<'a>) -> Self {
        Self::new(View::Borrowed(reader))
    }

    /// Open a session that owns its file's metadata view: the bloom
    /// decides first; a "maybe" decodes only the trace-id index and the
    /// trace column (dropped again when they find nothing); only a file
    /// that holds the trace decodes its row columns and upgrades to the
    /// full reader. A file the bloom rules out costs its summary,
    /// metadata and bloom.
    pub fn open_owned(meta: MetaReader<'a>) -> Self {
        Self::new(View::Meta(Box::new(meta)))
    }

    fn new(view: View<'r, 'a>) -> Self {
        let bloom = match view.chunk() {
            Ok(chunk) if chunk.has_trace_id_bloom() => match chunk.trace_id_bloom() {
                Ok(filter) => BloomGate::Filter(filter),
                Err(e) => {
                    // The bloom is a skip hint, not authoritative: a
                    // corrupt one must not make findable traces
                    // unfindable. One warning per session, however many
                    // ids the fan-out probes.
                    tracing::warn!(
                        "trace_id bloom unreadable; session falls back to exact lookups: {e}"
                    );
                    BloomGate::Unreadable
                }
            },
            _ => BloomGate::Pass,
        };
        Self {
            view,
            bloom,
            screen: Screen::Off,
            index: None,
            trace_ids: None,
            timestamps: None,
            rows: None,
            extras: None,
            batches: Vec::new(),
            strings: HashMap::new(),
            kinds: HashMap::new(),
            rollup: None,
            stats: None,
            resolver: None,
            on_release: None,
        }
    }

    /// Unless the file's sealed bloom is already tight, build a tight
    /// filter over the file's trace ids on the first lookup decode and
    /// screen every later id with it: for a caller looking up many ids in
    /// this file (search), whose false positives would otherwise each
    /// decode the lookup structures. A one-id lookup gains nothing from it.
    pub fn screened(mut self) -> Self {
        self.screen = match &self.bloom {
            BloomGate::Filter(bloom) if bloom.is_tight() => Screen::Off,
            BloomGate::Filter(_) | BloomGate::Pass | BloomGate::Unreadable => Screen::Pending,
        };
        self
    }

    /// Call `hook` whenever the session ends up holding nothing decoded
    /// (a release, or a probe that dropped what it decoded): a caller
    /// that maps the file drops its pages there, since everything the
    /// session keeps is a copy.
    pub fn on_release(mut self, hook: impl FnMut() + 'a) -> Self {
        self.on_release = Some(Box::new(hook));
        self
    }

    /// The file's metadata (decoded once, cached by the reader).
    pub fn metadata(&self) -> Result<&crate::Metadata, crate::Error> {
        self.view.chunk()?.metadata()
    }

    /// The full reader, opened on first use for an owned session (its
    /// `PRIM` decode), for callers that compile plans or resolve values.
    pub fn reader(&mut self) -> Result<&IndexReader<'a>, crate::Error> {
        self.view.full()
    }

    /// Whether the file carries the rollup stats (`TRSS`): a TOC check.
    pub fn has_rollup_stats(&self) -> bool {
        self.view
            .chunk()
            .is_ok_and(|chunk| chunk.has_trace_rollup_stats())
    }

    /// The file's bloom failed to decode: lookups fall back to the exact
    /// path, and the file is damaged.
    pub fn bloom_unreadable(&self) -> bool {
        matches!(self.bloom, BloomGate::Unreadable)
    }

    /// Bloom (and built screen) test, decoding nothing: `false` is a
    /// definite miss; `true` means "maybe" (or no usable filter).
    pub fn might_contain(&self, trace_id: TraceId) -> bool {
        let bloom = match &self.bloom {
            BloomGate::Pass | BloomGate::Unreadable => true,
            BloomGate::Filter(bloom) => bloom.might_contain(trace_id),
        };
        bloom
            && match &self.screen {
                Screen::Built(Some(screen)) => screen.might_contain(trace_id),
                Screen::Built(None) => false,
                Screen::Off | Screen::Pending | Screen::Unavailable => true,
            }
    }

    /// [`might_contain`](Self::might_contain), building a screened
    /// session's screen first when this is the first id the bloom lets
    /// through. `false` is a definite miss. The build keeps the lookup
    /// structures only when the id passes, for the lookup that follows.
    pub fn screen(&mut self, trace_id: TraceId) -> bool {
        if !self.might_contain(trace_id) {
            return false;
        }
        if !matches!(self.screen, Screen::Pending) {
            return true;
        }
        let held = (self.index.is_some(), self.trace_ids.is_some());
        self.screen = match self.lookup() {
            Ok((index, trace_ids)) => Screen::Built(TraceIdBloom::build(index, trace_ids)),
            Err(_) => Screen::Unavailable,
        };
        let pass = self.might_contain(trace_id);
        if !pass {
            self.free_lookup(held);
        }
        pass
    }

    /// `trace_id`'s row in the file's trace rollup (`TRSU`), decoding the
    /// rollup on first use: it stays loaded while it answers rows, and a
    /// probe that finds nothing drops the rollup it decoded. Screen first:
    /// an id the filters rule out never needs it.
    pub fn rollup_row(&mut self, trace_id: TraceId) -> Result<RollupEvidence, crate::Error> {
        let decoded = self.rollup.is_none();
        if decoded {
            let chunk = self.view.chunk()?;
            if !chunk.has_trace_rollup() {
                return Ok(RollupEvidence::NoRollup);
            }
            self.rollup = Some(chunk.trace_rollup()?);
        }
        let rollup = self.rollup.as_ref().expect("decoded above");
        match rollup.row(trace_id) {
            Some(row) => Ok(RollupEvidence::Row(row)),
            None => {
                if decoded {
                    self.rollup = None;
                    self.emptied();
                }
                Ok(RollupEvidence::Absent)
            }
        }
    }

    /// `trace_id`'s rollup row and its stats (`TRSU` + `TRSS`), decoding
    /// both into the loaded layer (a caller bounding its loaded sessions
    /// touches its [`crate::LoadedSources`] after this). A probe that
    /// finds no row drops what it decoded.
    pub fn rollup_part(&mut self, trace_id: TraceId) -> Result<PartEvidence, crate::Error> {
        let decoded = (self.rollup.is_none(), self.stats.is_none());
        if decoded.0 {
            let chunk = self.view.chunk()?;
            if !chunk.has_trace_rollup() {
                return Ok(PartEvidence::NoRollup);
            }
            self.rollup = Some(chunk.trace_rollup()?);
        }
        let rollup = self.rollup.as_ref().expect("decoded above");
        let Some(index) = rollup.find(trace_id) else {
            if decoded.0 {
                self.rollup = None;
                self.emptied();
            }
            return Ok(PartEvidence::Absent);
        };
        if decoded.1 {
            let chunk = self.view.chunk()?;
            if !chunk.has_trace_rollup_stats() {
                return Ok(PartEvidence::NoStats);
            }
            let stats = chunk.trace_rollup_stats(rollup)?;
            self.stats = Some(stats);
        }
        let rollup = self.rollup.as_ref().expect("decoded above");
        let stats = self.stats.as_ref().expect("decoded above");
        Ok(PartEvidence::Part(
            rollup.row_at(index),
            stats.row_at(rollup, index),
        ))
    }

    /// Visit every row of the file's trace rollup (`TRSU`) — its trace
    /// id and copied-out row — in id order, decoding the rollup into the
    /// loaded layer (later [`rollup_row`](Self::rollup_row) calls reuse
    /// it; a caller bounding its loaded sessions touches its
    /// [`crate::LoadedSources`] after this). `Ok(None)` when the file has
    /// no rollup, else the number of rows visited.
    pub fn for_each_rollup_row(
        &mut self,
        mut visit: impl FnMut(TraceId, RollupRow),
    ) -> Result<Option<usize>, crate::Error> {
        if self.rollup.is_none() {
            let chunk = self.view.chunk()?;
            if !chunk.has_trace_rollup() {
                return Ok(None);
            }
            self.rollup = Some(chunk.trace_rollup()?);
        }
        let rollup = self.rollup.as_ref().expect("decoded above");
        for i in 0..rollup.len() {
            visit(rollup.trace_ids.get(i), rollup.row_at(i));
        }
        Ok(Some(rollup.len()))
    }

    /// Resolve a rollup root ref as a value of `field_name`, through the
    /// file's dictionaries (the full reader) and the session's
    /// [`RollupRefResolver`]. `Err` when the file cannot open fully.
    pub fn resolve_rollup_ref(
        &mut self,
        field_name: &str,
        kv_ref: u32,
    ) -> Result<RollupRefOutcome<'_>, crate::Error> {
        let reader = self.view.full()?;
        let resolver = self.resolver.get_or_insert_with(RollupRefResolver::new);
        Ok(resolver.resolve(reader, field_name, kv_ref))
    }

    /// Tell the caller when nothing decoded is left.
    fn emptied(&mut self) {
        if !self.is_loaded()
            && let Some(hook) = self.on_release.as_mut()
        {
            hook();
        }
    }

    /// `kind` per position from the low-card `_kind` facet bitmaps, for
    /// the positions not cached yet (materialize_field on a low field
    /// never touches stream batches). Absent field / value → 0 (files
    /// written before UNSPECIFIED was stored skip it). A corrupt
    /// multi-valued row takes the first value — deterministic, and
    /// impossible from the production flattener.
    fn load_kinds(&mut self, positions: &[u32]) -> Result<(), crate::Error> {
        let missing: Vec<u32> = positions
            .iter()
            .copied()
            .filter(|p| !self.kinds.contains_key(p))
            .collect();
        if missing.is_empty() {
            return Ok(());
        }
        let values = self.view.full()?.materialize_field("_kind", &missing)?;
        for (p, vals) in missing.iter().zip(values) {
            debug_assert!(
                vals.len() <= 1,
                "a row carries multiple _kind values (impossible from the flattener)"
            );
            let kind = vals.first().and_then(|v| v.parse().ok()).unwrap_or(0);
            self.kinds.insert(*p, kind);
        }
        Ok(())
    }

    /// The lookup structures, decoding them on first use.
    /// The lookup pair (TIDX + TRCE), decoding what is missing.
    fn lookup(&mut self) -> Result<(&crate::TraceIdIndex, &crate::TraceIds), crate::Error> {
        if self.index.is_none() {
            self.index = Some(self.view.chunk()?.trace_id_index()?);
        }
        self.trace_ids()?;
        Ok((
            self.index.as_ref().expect("just populated"),
            self.trace_ids.as_ref().expect("just populated"),
        ))
    }

    /// Free the parts of the lookup pair a lookup decoded that found
    /// nothing; `held` = which parts were loaded before it.
    fn free_lookup(&mut self, held: (bool, bool)) {
        if !held.0 {
            self.index = None;
        }
        if !held.1 {
            self.trace_ids = None;
        }
        self.emptied();
    }

    /// The file's per-row trace ids, decoding them on first use. Part of
    /// the loaded layer: a caller bounding its loaded sessions touches
    /// its [`crate::LoadedSources`] after this.
    pub fn trace_ids(&mut self) -> Result<&crate::TraceIds, crate::Error> {
        if self.trace_ids.is_none() {
            self.trace_ids = Some(self.view.chunk()?.trace_ids()?);
        }
        Ok(self.trace_ids.as_ref().expect("just populated"))
    }

    /// The file's per-row start timestamps, decoding them on first use.
    /// Part of the loaded layer, like [`trace_ids`](Self::trace_ids).
    pub fn timestamps(&mut self) -> Result<&crate::Timestamps, crate::Error> {
        if self.timestamps.is_none() {
            self.timestamps = Some(crate::Timestamps::new(self.view.chunk()?.timestamps()?));
        }
        Ok(self.timestamps.as_ref().expect("just populated"))
    }

    /// The rows holding `trace_id`, chronological (empty when the file
    /// holds none). An id the bloom or screen rules out costs no decode;
    /// otherwise the lookup pair (TIDX + TRCE) decodes on first use and
    /// stays while the id is found, so the session then reports itself
    /// loaded: a caller bounding its loaded sessions touches its
    /// [`crate::LoadedSources`] after this. A false positive frees what
    /// it decoded. The UNSET id is a request error, as in
    /// [`SpanSource::span_refs`].
    pub fn positions(&mut self, trace_id: TraceId) -> Result<Vec<u32>, crate::Error> {
        if trace_id.is_unset() {
            return Err(crate::Error::UnsetTraceId);
        }
        let held = (self.index.is_some(), self.trace_ids.is_some());
        if !self.screen(trace_id) {
            return Ok(Vec::new());
        }
        let (index, trace_ids) = self.lookup()?;
        let positions = index.positions(trace_id, trace_ids).to_vec();
        if positions.is_empty() {
            // A false positive: free what found nothing.
            self.free_lookup(held);
        }
        Ok(positions)
    }

    /// The per-row columns, decoding them on the first lookup that finds
    /// rows.
    fn rows(&mut self) -> Result<&Rows, crate::Error> {
        if self.rows.is_none() {
            let chunk = self.view.chunk()?;
            self.rows = Some(Rows {
                span_ids: chunk.span_ids()?,
                parents: chunk.parent_span_ids()?,
                durations: chunk.durations()?,
                flags: chunk.flags()?,
                dropped: chunk.dropped_attribute_counts()?,
            });
        }
        self.timestamps()?;
        Ok(self.rows.as_ref().expect("just populated"))
    }

    /// The event/link indexes, decoding them on the first
    /// materialization.
    fn extras(&mut self) -> Result<&Extras, crate::Error> {
        if self.extras.is_none() {
            let chunk = self.view.chunk()?;
            self.extras = Some(Extras {
                events: if chunk.has_event_index() {
                    Some(chunk.event_index()?)
                } else {
                    None
                },
                links: if chunk.has_link_index() {
                    Some(chunk.link_index()?)
                } else {
                    None
                },
            });
        }
        Ok(self.extras.as_ref().expect("just populated"))
    }

    /// The decoded stream batch containing `pos`.
    fn batch(&mut self, pos: u32) -> Result<&crate::StreamBatch, crate::Error> {
        let total = self.view.record_count()?;
        if pos >= total {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} >= record_count {total}"
            )));
        }
        if self.batches.is_empty() {
            self.batches = (0..crate::num_stream_batches(total))
                .map(|_| None)
                .collect();
        }
        let b = (pos / crate::stream_batch_size(total)) as usize;
        if self.batches.get(b).is_none() {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} maps to missing stream batch {b}"
            )));
        }
        if self.batches[b].is_none() {
            self.batches[b] = Some(self.view.chunk()?.stream_batch(b as u8)?);
        }
        Ok(self.batches[b].as_ref().expect("just populated"))
    }

    /// Resolve `ids` into the per-session cache, fetching only the missing
    /// ones from the file. Returns the ids of the long values it added,
    /// which the caller removes once its span is built.
    fn resolve(&mut self, ids: &mut Vec<u32>) -> Result<Vec<u32>, crate::Error> {
        ids.sort_unstable();
        ids.dedup();
        ids.retain(|id| !self.strings.contains_key(id));
        let mut long = Vec::new();
        if !ids.is_empty() {
            let resolved = self.view.full()?.resolve_kv_strings(ids)?;
            for (id, kv) in resolved {
                if kv.len() > CACHED_VALUE_MAX_BYTES {
                    long.push(id);
                }
                self.strings.insert(id, kv);
            }
        }
        Ok(long)
    }
}

impl SpanSource for TraceFileSession<'_, '_> {
    fn span_refs(&mut self, trace_id: TraceId) -> Result<Vec<SpanRef>, crate::Error> {
        // The batch-primitive contract: an UNSET id in a candidate set is
        // a request error (TIDX omits UNSET while a tail scan could serve
        // it — accepting it here would make lookups layout-dependent).
        // `positions` enforces it, screens, and frees a false positive.
        let positions = self.positions(trace_id)?;
        if positions.is_empty() {
            return Ok(Vec::new());
        }

        self.load_kinds(&positions)?;

        self.rows()?;
        let kinds = &self.kinds;
        let cols = self.rows.as_ref().expect("populated above");
        let timestamps = self.timestamps.as_ref().expect("rows load them");
        positions
            .into_iter()
            .map(|pos| {
                let start_ns = timestamps.at(pos).ok_or_else(|| {
                    crate::Error::CorruptIndex(format!(
                        "trace session: position {pos} has no timestamp"
                    ))
                })?;
                Ok(SpanRef {
                    position: pos,
                    start_ns,
                    span_id: cols.span_ids.get(pos as usize),
                    kind: kinds.get(&pos).copied().unwrap_or(0),
                })
            })
            .collect()
    }

    fn materialize(&mut self, pos: u32) -> Result<TraceSpan, crate::Error> {
        // Row tokens first (also validates the position), then the
        // event/link refs — everything this span needs resolves through
        // the session cache in one pass.
        let local = (pos % crate::stream_batch_size(self.view.record_count()?)) as usize;
        let batch = self.batch(pos)?;
        // A CRC-valid but structurally short batch must surface as
        // `CorruptIndex`, never an out-of-bounds panic (the same check
        // the reader's `materialize_rows` applies).
        if local >= batch.num_rows() {
            return Err(crate::Error::CorruptIndex(format!(
                "trace session: position {pos} row {local} >= batch length {}",
                batch.num_rows()
            )));
        }
        let row_ids: Vec<crate::KvId> = batch.row(local).collect();
        let extras = self.extras()?;
        let has_events = extras.events.is_some();
        let has_links = extras.links.is_some();
        let mut needed: Vec<u32> = row_ids.iter().map(|kv| kv.0).collect();
        if let Some(ev) = &extras.events {
            needed.extend(ev.all_refs_for_row(pos).map(|id| id.0));
        }
        if let Some(lk) = &extras.links {
            needed.extend(lk.all_refs_for_row(pos).map(|id| id.0));
        }
        let long = self.resolve(&mut needed)?;

        // Row facets: the `events.`/`links.` tokens exist for search; in
        // the reconstructed span they appear structured instead, so drop
        // the flat duplicates (only when the structure is present).
        let fields: Vec<(String, String)> = row_ids
            .iter()
            .map(|kv| {
                let s = self.strings.get(&kv.0).map(String::as_str).unwrap_or("");
                match s.split_once('=') {
                    Some((k, v)) => (k.to_string(), v.to_string()),
                    None => (s.to_string(), String::new()),
                }
            })
            .filter(|(k, _)| {
                !((has_events && k.starts_with("events."))
                    || (has_links && k.starts_with("links.")))
            })
            .collect();

        let extras = self.extras.as_ref().expect("populated above by extras()");
        let events: Vec<TraceEvent> = match &extras.events {
            Some(ev) => ev
                .events_for_row(pos)
                .map(|e| TraceEvent {
                    time_unix_nano: e.time_unix_nano,
                    name: kv_value(&self.strings, e.name),
                    dropped_attributes_count: e.dropped_attributes_count,
                    attributes: e
                        .attr_refs
                        .iter()
                        .map(|&id| kv_attr(&self.strings, id, "events.attributes."))
                        .collect(),
                })
                .collect(),
            None => Vec::new(),
        };
        let links: Vec<TraceLink> = match &extras.links {
            Some(lk) => lk
                .links_for_row(pos)
                .map(|l| TraceLink {
                    trace_id: l.trace_id,
                    span_id: l.span_id,
                    trace_state: String::from_utf8_lossy(l.trace_state).into_owned(),
                    flags: l.flags,
                    dropped_attributes_count: l.dropped_attributes_count,
                    attributes: l
                        .attr_refs
                        .iter()
                        .map(|&id| kv_attr(&self.strings, id, "links.attributes."))
                        .collect(),
                })
                .collect(),
            None => Vec::new(),
        };
        let dropped_events_count = extras
            .events
            .as_ref()
            .map_or(0, |ev| ev.row_dropped_count(pos));
        let dropped_links_count = extras
            .links
            .as_ref()
            .map_or(0, |lk| lk.row_dropped_count(pos));
        // Every string of this span is built: long values leave the cache.
        for id in long {
            self.strings.remove(&id);
        }

        // `kind` from the facet, as `span_refs` reads it (a released
        // session lost the cached values).
        self.load_kinds(&[pos])?;
        let kind = self.kinds.get(&pos).copied().unwrap_or(0);

        self.rows()?;
        let cols = self.rows.as_ref().expect("populated above");
        let timestamps = self.timestamps.as_ref().expect("rows load them");
        let i = pos as usize;
        let bounds = |what: &str| {
            crate::Error::CorruptIndex(format!("trace session: {what} missing at position {pos}"))
        };
        Ok(TraceSpan {
            span_id: cols.span_ids.get(i),
            parent_span_id: cols.parents.get(i),
            start_ns: timestamps.at(pos).ok_or_else(|| bounds("timestamp"))?,
            duration_ns: *cols.durations.0.get(i).ok_or_else(|| bounds("duration"))?,
            kind,
            flags: *cols.flags.0.get(i).ok_or_else(|| bounds("flags"))?,
            dropped_attributes_count: *cols.dropped.0.get(i).ok_or_else(|| bounds("dropped"))?,
            dropped_events_count,
            dropped_links_count,
            fields,
            events,
            links,
        })
    }

    /// Whether the loaded layer holds anything (see the module docs).
    fn is_loaded(&self) -> bool {
        matches!(self.view, View::Full(_))
            || self.index.is_some()
            || self.trace_ids.is_some()
            || self.timestamps.is_some()
            || self.rows.is_some()
            || self.extras.is_some()
            || !self.batches.is_empty()
            || !self.strings.is_empty()
            || !self.kinds.is_empty()
            || self.rollup.is_some()
            || self.stats.is_some()
            || self.resolver.is_some()
    }

    /// Drop the loaded layer, keeping the bloom and the screen: an owned
    /// full reader goes back to its metadata view, and everything decodes
    /// again on next use, with the same answers.
    fn release(&mut self) {
        self.view.release();
        self.index = None;
        self.trace_ids = None;
        self.timestamps = None;
        self.rows = None;
        self.extras = None;
        self.batches = Vec::new();
        self.strings = HashMap::new();
        self.kinds = HashMap::new();
        self.rollup = None;
        self.stats = None;
        self.resolver = None;
        self.emptied();
    }
}
