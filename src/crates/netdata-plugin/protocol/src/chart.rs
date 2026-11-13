//! Chart-protocol encoding: the plugin→agent lines that define charts
//! (`CHART`, `DIMENSION`, `CLABEL`, `CLABEL_COMMIT`) and stream their values
//! (`BEGIN`, `SET`, `END`), written into any `bytes::BufMut`.
//!
//! This module is the single owner of the chart vocabulary and of the rules
//! that keep a line parseable by the agent (`src/plugins.d/pluginsd_parser.c`).
//! The agent splits each line into words with `quoted_strings_splitter`
//! (`src/libnetdata/line_splitter/line_splitter.h`): a field may be wrapped in
//! `'`, a backslash keeps itself and the next byte verbatim, and a line ends
//! at `\n`. Nothing can be escaped, so text fields are sanitized instead (see
//! [`write_quoted`]): a `'` inside a field would end it early and shift every
//! later word, a trailing `\` would swallow the closing quote, and a newline
//! would split the command. A malformed command is a protocol error that
//! stops the agent's parser and restarts or disables the plugin.
//!
//! Ids and option words are written verbatim; callers own their validity:
//! - a chart id is `type.id`, both parts non-empty (the agent rejects the line
//!   otherwise, `pluginsd_chart`), at most [`MAX_CHART_ID_BYTES`] long, and a
//!   plain word: no whitespace or `=` (the splitter ends the word there), no
//!   quote (one at the start of a word opens a quoted field) and no backslash
//!   (it takes the next byte with it). The agent's splitter would keep a
//!   quote inside a word; this crate refuses one anywhere, as one rule;
//! - a dimension id is a plain word too, and every `SET` names a dimension
//!   the chart declared (an unknown id is a protocol error,
//!   `pluginsd_acquire_dimension`).
//!
//! [`sanitize_chart_id`] and [`sanitize_dimension_id`] make an id valid when
//! its text is not the plugin's own.
//!
//! Debug builds assert both. `validate_chart_stream` (feature
//! `test-support`) checks whole streams in tests.

use std::borrow::Cow;

use bytes::BufMut;

/// Chart rendering type on the `CHART` line.
///
/// `Heatmap` is a rendering of bucket dimensions, not a histogram data type:
/// the agent stores each bucket as an ordinary dimension
/// (`src/database/rrdset-type.c`).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum ChartType {
    #[default]
    Line,
    Area,
    Stacked,
    Heatmap,
}

impl ChartType {
    /// The keyword the agent recognizes (`rrdset_type_id`).
    pub fn as_str(&self) -> &'static str {
        match self {
            ChartType::Line => "line",
            ChartType::Area => "area",
            ChartType::Stacked => "stacked",
            ChartType::Heatmap => "heatmap",
        }
    }
}

/// How the agent turns each collected `SET` value into a stored point.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum DimensionAlgorithm {
    /// Store the value as-is.
    #[default]
    Absolute,
    /// Store the rate of change since the previous value (counters).
    Incremental,
    /// Store the value as a percentage of the row total.
    PercentageOfAbsoluteRow,
    /// Store the rate as a percentage of the row's total rate.
    PercentageOfIncrementalRow,
}

impl DimensionAlgorithm {
    /// The keyword the agent recognizes (`rrd_algorithm_id`).
    pub fn as_str(&self) -> &'static str {
        match self {
            DimensionAlgorithm::Absolute => "absolute",
            DimensionAlgorithm::Incremental => "incremental",
            DimensionAlgorithm::PercentageOfAbsoluteRow => "percentage-of-absolute-row",
            DimensionAlgorithm::PercentageOfIncrementalRow => "percentage-of-incremental-row",
        }
    }
}

/// Options of a `CHART` line. The agent matches them by substring, and a
/// `CHART` line without `obsolete` revives an obsolete chart.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct ChartOptions {
    /// Mark the chart obsolete: the agent stops expecting updates and frees it
    /// after its cleanup delay.
    pub obsolete: bool,
    /// Keep the first collected point instead of dropping it during
    /// interpolation.
    pub store_first: bool,
    /// Hide the chart from the dashboard.
    pub hidden: bool,
}

/// Options of a `DIMENSION` line.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct DimensionOptions {
    /// Values are doubles, written with [`write_set_float`], not integers.
    pub float: bool,
    /// Hide the dimension from the chart.
    pub hidden: bool,
    /// Mark the dimension obsolete.
    pub obsolete: bool,
}

/// The source bits of a chart label (`RRDLABEL_SRC` in
/// `src/database/rrdlabels.h`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LabelSource {
    /// Found by automation (`RRDLABEL_SRC_AUTO`).
    Auto,
    /// Configured by the user (`RRDLABEL_SRC_CONFIG`).
    Config,
}

impl LabelSource {
    fn bits(self) -> u32 {
        match self {
            LabelSource::Auto => 1,
            LabelSource::Config => 2,
        }
    }
}

/// The fields of a `CHART` line.
#[derive(Debug, Clone, Copy)]
pub struct ChartLine<'a> {
    /// `type.id`.
    pub id: &'a str,
    /// Alternate name; empty for none.
    pub name: &'a str,
    pub title: &'a str,
    pub units: &'a str,
    pub family: &'a str,
    pub context: &'a str,
    pub chart_type: ChartType,
    /// Dashboard order; lower comes first. The agent reads it as an `int`.
    pub priority: i32,
    /// Collection interval in seconds, 1..=`i32::MAX`: the agent reads an
    /// `int` and takes 0 as its own default. Debug builds assert it.
    pub update_every: u64,
    pub options: ChartOptions,
}

/// The fields of a `DIMENSION` line.
#[derive(Debug, Clone, Copy)]
pub struct DimensionLine<'a> {
    /// The id `SET` lines reference.
    pub id: &'a str,
    /// Display name.
    pub name: &'a str,
    pub algorithm: DimensionAlgorithm,
    pub multiplier: i64,
    pub divisor: i64,
    pub options: DimensionOptions,
}

/// Write `CHART <id> '<name>' '<title>' '<units>' '<family>' '<context>'
/// <type> <priority> <update_every>[ <options>]`. The optional plugin and
/// module fields are never written: the agent fills in the plugin name.
pub fn write_chart(buf: &mut impl BufMut, chart: &ChartLine<'_>) {
    debug_assert!(
        is_valid_chart_id(chart.id),
        "invalid chart id {:?}",
        chart.id
    );
    debug_assert!(
        (1..=i32::MAX as u64).contains(&chart.update_every),
        "update_every {} is outside 1..=i32::MAX",
        chart.update_every
    );
    buf.put_slice(b"CHART ");
    buf.put_slice(chart.id.as_bytes());
    for text in [
        chart.name,
        chart.title,
        chart.units,
        chart.family,
        chart.context,
    ] {
        buf.put_u8(b' ');
        write_quoted(buf, text);
    }
    buf.put_u8(b' ');
    buf.put_slice(chart.chart_type.as_str().as_bytes());
    buf.put_u8(b' ');
    write_i64(buf, i64::from(chart.priority));
    buf.put_u8(b' ');
    write_u64(buf, chart.update_every);
    let options = chart.options;
    write_options(
        buf,
        &[
            (options.obsolete, "obsolete"),
            (options.store_first, "store_first"),
            (options.hidden, "hidden"),
        ],
    );
    buf.put_u8(b'\n');
}

/// Write `DIMENSION <id> '<name>' <algorithm> <multiplier> <divisor>[
/// <options>]`. It attaches to the chart of the preceding `CHART` line.
pub fn write_dimension(buf: &mut impl BufMut, dimension: &DimensionLine<'_>) {
    debug_assert!(
        is_valid_dimension_id(dimension.id),
        "invalid dimension id {:?}",
        dimension.id
    );
    buf.put_slice(b"DIMENSION ");
    buf.put_slice(dimension.id.as_bytes());
    buf.put_u8(b' ');
    write_quoted(buf, dimension.name);
    buf.put_u8(b' ');
    buf.put_slice(dimension.algorithm.as_str().as_bytes());
    buf.put_u8(b' ');
    write_i64(buf, dimension.multiplier);
    buf.put_u8(b' ');
    write_i64(buf, dimension.divisor);
    let options = dimension.options;
    write_options(
        buf,
        &[
            (options.float, "type=float"),
            (options.hidden, "hidden"),
            (options.obsolete, "obsolete"),
        ],
    );
    buf.put_u8(b'\n');
}

/// Write `CLABEL '<key>' '<value>' <source>`. Labels attach to the chart of
/// the preceding `CHART` line and take effect at [`write_clabel_commit`],
/// which also removes the chart's labels that were not re-sent. The agent
/// stores an empty value as `[none]`, so omit a label rather than send it
/// empty.
pub fn write_clabel(buf: &mut impl BufMut, key: &str, value: &str, source: LabelSource) {
    buf.put_slice(b"CLABEL ");
    write_quoted(buf, key);
    buf.put_u8(b' ');
    write_quoted(buf, value);
    buf.put_u8(b' ');
    write_u64(buf, u64::from(source.bits()));
    buf.put_u8(b'\n');
}

/// Write `CLABEL_COMMIT`. It must follow at least one `CLABEL`.
pub fn write_clabel_commit(buf: &mut impl BufMut) {
    buf.put_slice(b"CLABEL_COMMIT\n");
}

/// Write `BEGIN <chart_id> <microseconds>`, opening one update of the chart.
/// The agent reads `microseconds` as the time since the chart's previous
/// update; 0 makes it measure that on its own clock.
pub fn write_begin(buf: &mut impl BufMut, chart_id: &str, microseconds: u64) {
    debug_assert!(is_valid_chart_id(chart_id), "invalid chart id {chart_id:?}");
    buf.put_slice(b"BEGIN ");
    buf.put_slice(chart_id.as_bytes());
    buf.put_u8(b' ');
    write_u64(buf, microseconds);
    buf.put_u8(b'\n');
}

/// Write `SET <dimension_id> = <value>` for an integer dimension.
pub fn write_set_int(buf: &mut impl BufMut, dimension_id: &str, value: i64) {
    write_set_prefix(buf, dimension_id);
    buf.put_u8(b' ');
    write_i64(buf, value);
    buf.put_u8(b'\n');
}

/// Write `SET <dimension_id> = <value>` for a dimension declared with
/// [`DimensionOptions::float`]. The value is exact: `@` and the 64 bits of
/// the double in base64, most significant digit first, without padding, for
/// example `@D/4AAAAAAAA` for 1.5. The agent decodes it as it decodes its own
/// streaming (`str2ndd_encoded`, accepted on float dimensions since v2.10.0);
/// it is several times cheaper to write than the shortest decimal. A
/// non-finite value is written as an empty `SET`: no value this update.
pub fn write_set_float(buf: &mut impl BufMut, dimension_id: &str, value: f64) {
    if !value.is_finite() {
        write_set_empty(buf, dimension_id);
        return;
    }
    write_set_prefix(buf, dimension_id);
    buf.put_slice(b" @");
    // Normalize -0 so the agent never stores a sign on zero.
    let value = if value == 0.0 { 0.0 } else { value };
    write_base64(buf, value.to_bits());
    buf.put_u8(b'\n');
}

/// libnetdata's base64 digits (`base64_digits`, `src/libnetdata/buffer/buffer.c`).
const BASE64_DIGITS: &[u8; 64] =
    b"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";

/// Write `bits` in base64, most significant digit first, without leading
/// zero digits (one digit for 0), as libnetdata's
/// `print_uint64_base64_reversed` and `char_array_reverse` do.
fn write_base64(buf: &mut impl BufMut, mut bits: u64) {
    // 64 bits take at most 11 digits of 6 bits.
    let mut digits = [0u8; 11];
    let mut start = digits.len();
    loop {
        start -= 1;
        digits[start] = BASE64_DIGITS[(bits & 63) as usize];
        bits >>= 6;
        if bits == 0 {
            break;
        }
    }
    buf.put_slice(&digits[start..]);
}

/// Read a `SET` value of a float dimension as the agent reads it: `@` and
/// base64 bits (as [`write_set_float`] writes them), or a decimal. `None` for
/// anything else, including base64 digits that do not fit 64 bits.
#[cfg(any(test, feature = "test-support"))]
pub fn read_float_value(word: &str) -> Option<f64> {
    let Some(digits) = word.strip_prefix('@') else {
        return word.parse().ok();
    };
    if digits.is_empty() {
        return None;
    }
    let mut bits = 0u64;
    for digit in digits.bytes() {
        let value = BASE64_DIGITS.iter().position(|&d| d == digit)? as u64;
        if bits >> 58 != 0 {
            return None;
        }
        bits = bits << 6 | value;
    }
    Some(f64::from_bits(bits))
}

/// `stream` as text with every `@` base64 `SET` value written as a decimal
/// (Rust's `Display`), so tests can assert on readable values. Other lines,
/// and values that do not decode, stay as they are.
#[cfg(any(test, feature = "test-support"))]
pub fn readable_chart_stream(stream: &[u8]) -> String {
    let text = std::str::from_utf8(stream).expect("chart stream is UTF-8");
    let mut out = String::with_capacity(text.len());
    for line in text.split_inclusive('\n') {
        let decoded = line
            .strip_prefix("SET ")
            .and_then(|rest| rest.split_once(" = @"))
            .and_then(|(id, word)| {
                let word = word.strip_suffix('\n').unwrap_or(word);
                read_float_value(&format!("@{word}")).map(|value| (id, value))
            });
        match decoded {
            Some((id, value)) => {
                out.push_str(&format!("SET {id} = {value}"));
                if line.ends_with('\n') {
                    out.push('\n');
                }
            }
            None => out.push_str(line),
        }
    }
    out
}

/// Write `SET <dimension_id> =`: the dimension gets no value this update
/// and the agent stores a gap.
pub fn write_set_empty(buf: &mut impl BufMut, dimension_id: &str) {
    write_set_prefix(buf, dimension_id);
    buf.put_u8(b'\n');
}

/// Write `END <unix_seconds>`, closing the update opened by [`write_begin`].
///
/// The agent uses this time only to place a chart's first point; after that
/// it advances the chart by the `BEGIN` durations, re-synced to its own wall
/// clock whenever the chart is (re)defined (`RRDSET_FLAG_SYNC_CLOCK`,
/// `src/database/rrdset-collection.c`). That suits plugins that collect "now".
/// For updates of past times use [`write_end_timed`].
pub fn write_end(buf: &mut impl BufMut, unix_seconds: u64) {
    buf.put_slice(b"END ");
    write_u64(buf, unix_seconds);
    buf.put_u8(b'\n');
}

/// Write `END <unix_seconds> 0 1`: close the update and make the agent place
/// it at `unix_seconds`. The third word asks the agent to derive the time
/// since the previous update from this time, which is how its internal
/// collectors finish an update (`rrdset_done` → `rrdset_timed_done` with the
/// pending flag, `src/database/rrdset-collection.c`). Times must increase per
/// chart. Combine with [`ChartOptions::store_first`], so the chart's clock is
/// aligned to whole seconds and the first point is kept: then each update is
/// stored exactly at its time, without interpolation.
pub fn write_end_timed(buf: &mut impl BufMut, unix_seconds: u64) {
    buf.put_slice(b"END ");
    write_u64(buf, unix_seconds);
    buf.put_slice(b" 0 1\n");
}

/// Write `text` as one single-quoted field the agent splits back into the
/// same word. Bytes the splitter would misread are replaced, never escaped
/// (escapes keep their backslash on the agent side):
/// - `'` becomes `"` (it would close the field);
/// - `\` becomes `/`, as the agent's own label and chart-name sanitizers do
///   (a trailing one would swallow the closing quote);
/// - ASCII control characters become a space (`\n` would end the command).
pub fn write_quoted(buf: &mut impl BufMut, text: &str) {
    buf.put_u8(b'\'');
    let bytes = text.as_bytes();
    let mut start = 0;
    for (i, &b) in bytes.iter().enumerate() {
        let replacement = match b {
            b'\'' => b'"',
            b'\\' => b'/',
            0x00..=0x1f | 0x7f => b' ',
            _ => continue,
        };
        buf.put_slice(&bytes[start..i]);
        buf.put_u8(replacement);
        start = i + 1;
    }
    buf.put_slice(&bytes[start..]);
    buf.put_u8(b'\'');
}

/// Packs chart-protocol text into chunks under a byte budget, for a channel
/// that limits the size of one message.
///
/// A chunk holds whole blocks. A block is what one call of [`Chunker::block`]
/// writes: one chart definition, one update (`BEGIN`..`END`) or one obsolete
/// mark. So each chunk can be written to the agent on its own, and the chunks
/// in order are the text. A block larger than the budget gets a chunk of its
/// own.
pub struct Chunker {
    max_chunk: usize,
    chunks: Vec<Vec<u8>>,
    current: Vec<u8>,
    block: Vec<u8>,
}

impl Chunker {
    /// A packer of chunks of at most `max_chunk` bytes; `usize::MAX` for one
    /// chunk.
    pub fn new(max_chunk: usize) -> Self {
        Self {
            max_chunk,
            chunks: Vec::new(),
            current: Vec::new(),
            block: Vec::new(),
        }
    }

    /// Add one block, written by `write`; a chunk never splits a block.
    pub fn block(&mut self, write: impl FnOnce(&mut Vec<u8>)) {
        self.block.clear();
        write(&mut self.block);
        if !self.current.is_empty() && self.current.len() + self.block.len() > self.max_chunk {
            self.chunks.push(std::mem::take(&mut self.current));
        }
        self.current.extend_from_slice(&self.block);
    }

    /// The chunks, in the order to write them; none is empty.
    pub fn finish(mut self) -> Vec<Vec<u8>> {
        if !self.current.is_empty() {
            self.chunks.push(self.current);
        }
        self.chunks
    }
}

/// The longest chart id the agent keeps whole. It builds the chart's key
/// with a bounded print into `RRD_ID_LENGTH_MAX` (1200) bytes, the last one
/// for the terminator (`rrdset_create_custom`,
/// `src/database/rrdset-index-id.c`), so a longer id is stored cut short. A
/// later `BEGIN` carries the whole id, finds no chart, and is a protocol
/// error.
pub const MAX_CHART_ID_BYTES: usize = 1199;

/// The agent's limit for a label key (`RRDLABELS_MAX_NAME_LENGTH`,
/// src/database/rrdlabels.h). The agent applies it to the text its sanitizer
/// writes, not to the text it is sent: the sanitizer skips leading spaces
/// and keeps one space of a run (`text_sanitize`,
/// src/libnetdata/sanitizers/utf8-sanitizer.c), and the last byte of the
/// limit is its terminator. So this is close to what the agent keeps of a
/// key, not the same for every text.
pub const MAX_LABEL_KEY_BYTES: usize = 200;

/// The agent's limit for a label value (`RRDLABELS_MAX_VALUE_LENGTH`,
/// src/database/rrdlabels.h), applied as for [`MAX_LABEL_KEY_BYTES`].
pub const MAX_LABEL_VALUE_BYTES: usize = 800;

/// True when `id` is a valid chart id: `type.id` with both parts non-empty,
/// no byte the agent's splitter treats as a separator or quote, not
/// starting with `SLOT:` (the agent would read the word as a slot number,
/// `pluginsd_parse_rrd_slot`), and at most [`MAX_CHART_ID_BYTES`] long.
pub fn is_valid_chart_id(id: &str) -> bool {
    if id.len() > MAX_CHART_ID_BYTES {
        return false;
    }

    match id.split_once('.') {
        Some((kind, rest)) => !kind.is_empty() && !rest.is_empty() && is_plain_word(id),
        None => false,
    }
}

/// True when `id` is a valid dimension id: non-empty, with no byte the
/// agent's splitter treats as a separator or quote, and not starting with
/// `SLOT:`.
pub fn is_valid_dimension_id(id: &str) -> bool {
    !id.is_empty() && is_plain_word(id)
}

fn is_plain_word(word: &str) -> bool {
    !word.starts_with("SLOT:") && !word.bytes().any(splits_or_quotes)
}

/// True for a byte the agent's splitter treats as a separator or a quote, and
/// for `=`, which separates a `SET` line's dimension from its value.
fn splits_or_quotes(byte: u8) -> bool {
    matches!(byte, b'\'' | b'"' | b'\\' | b'=' | 0x00..=0x20 | 0x7f)
}

/// A valid chart id made from `id`: `id` itself when it is valid
/// ([`is_valid_chart_id`]). Otherwise every byte the agent's splitter would
/// misread becomes `_`, a leading `SLOT:` becomes `SLOT_`, an empty type or
/// id part becomes `_` (`cpu` becomes `cpu._`), and what is past
/// [`MAX_CHART_ID_BYTES`] is cut. Ids that differ only in replaced or cut
/// bytes become one id: a caller that needs them apart keeps its ids
/// shorter than the limit.
///
/// For ids built from text the plugin does not control, such as a metric name
/// a sender chose.
pub fn sanitize_chart_id(id: &str) -> Cow<'_, str> {
    if is_valid_chart_id(id) {
        return Cow::Borrowed(id);
    }

    // Room for the `.` and the `_` that an empty part may add.
    let mut word = plain_word(id);
    let mut end = word.len().min(MAX_CHART_ID_BYTES - 2);
    while !word.is_char_boundary(end) {
        end -= 1;
    }
    word.truncate(end);

    let (kind, rest) = word.split_once('.').unwrap_or((&word, ""));
    let or_placeholder = |part| if str::is_empty(part) { "_" } else { part };
    Cow::Owned(format!("{}.{}", or_placeholder(kind), or_placeholder(rest)))
}

/// A valid dimension id made from `id`: `id` itself when it is valid
/// ([`is_valid_dimension_id`]). Otherwise every byte the agent's splitter
/// would misread becomes `_`, a leading `SLOT:` becomes `SLOT_`, and an empty
/// id becomes `_`. Ids that differ only in replaced bytes become one id.
pub fn sanitize_dimension_id(id: &str) -> Cow<'_, str> {
    if is_valid_dimension_id(id) {
        return Cow::Borrowed(id);
    }

    let word = plain_word(id);
    Cow::Owned(if word.is_empty() {
        "_".to_string()
    } else {
        word
    })
}

/// `text` with every byte of [`splits_or_quotes`] replaced by `_` and a
/// leading `SLOT:` turned into `SLOT_`.
fn plain_word(text: &str) -> String {
    let mut word: String = text
        .chars()
        .map(|c| match u8::try_from(c) {
            Ok(byte) if splits_or_quotes(byte) => '_',
            _ => c,
        })
        .collect();
    if word.starts_with("SLOT:") {
        word.replace_range(4..5, "_");
    }
    word
}

fn write_set_prefix(buf: &mut impl BufMut, dimension_id: &str) {
    debug_assert!(
        is_valid_dimension_id(dimension_id),
        "invalid dimension id {dimension_id:?}"
    );
    buf.put_slice(b"SET ");
    buf.put_slice(dimension_id.as_bytes());
    buf.put_slice(b" =");
}

/// Write the enabled option words as one single-quoted field. The quotes are
/// required: the agent's splitter also separates words at `=` outside quotes,
/// so a bare `type=float` would arrive as two words.
fn write_options(buf: &mut impl BufMut, options: &[(bool, &str)]) {
    if !options.iter().any(|(enabled, _)| *enabled) {
        return;
    }
    buf.put_slice(b" '");
    let mut first = true;
    for (enabled, word) in options {
        if !enabled {
            continue;
        }
        if !first {
            buf.put_u8(b' ');
        }
        buf.put_slice(word.as_bytes());
        first = false;
    }
    buf.put_u8(b'\'');
}

fn write_i64(buf: &mut impl BufMut, value: i64) {
    buf.put_slice(itoa::Buffer::new().format(value).as_bytes());
}

fn write_u64(buf: &mut impl BufMut, value: u64) {
    buf.put_slice(itoa::Buffer::new().format(value).as_bytes());
}

#[cfg(any(test, feature = "test-support"))]
pub use validate::validate_chart_stream;

#[cfg(any(test, feature = "test-support"))]
mod validate {
    use super::read_float_value;
    use std::collections::{HashMap, HashSet};

    #[derive(Default)]
    struct ChartState {
        int_dimensions: HashSet<String>,
        float_dimensions: HashSet<String>,
        /// Last defined with the `obsolete` option.
        obsolete: bool,
        last_end: Option<u64>,
    }

    enum Scope {
        None,
        /// After `CHART`: `DIMENSION` and `CLABEL` lines attach here.
        Definition {
            chart: String,
            labels: usize,
        },
        /// After `BEGIN`: `SET` lines until `END`.
        Update {
            chart: String,
            set: HashSet<String>,
        },
    }

    /// Split one line (without its `\n`) the way the agent's
    /// `quoted_strings_splitter` with `isspace_map_pluginsd` splits it:
    /// separators are space, `\t`, `\r`, `\f`, `\v` and `=` outside quotes; a
    /// word may open with `'` or `"` and closes at the same quote; a backslash
    /// keeps itself and the next byte. The agent splits the line with its
    /// `\n` still attached and stops at a NUL, so a line ending inside a
    /// quote or in a backslash, or holding a NUL, would read differently
    /// there: those are errors here. The crate's `WordIterator` differs on
    /// purpose (see its docs), so the validator carries its own port.
    fn agent_words(line: &[u8]) -> Result<Vec<Vec<u8>>, &'static str> {
        if line.contains(&0) {
            return Err("NUL byte");
        }
        let is_space = |b: u8| matches!(b, b' ' | b'\t' | b'\r' | 0x0c | 0x0b | b'=');
        let mut words = Vec::new();
        let mut i = 0;
        let skip_to_word = |i: &mut usize, quote: &mut u8| {
            while *i < line.len() && is_space(line[*i]) {
                *i += 1;
            }
            if *i < line.len() && (line[*i] == b'\'' || line[*i] == b'"') {
                *quote = line[*i];
                *i += 1;
            }
        };
        let mut quote = 0u8;
        skip_to_word(&mut i, &mut quote);
        if i == line.len() {
            return if quote == 0 {
                Ok(words)
            } else {
                Err("unterminated quote")
            };
        }
        let mut word = Vec::new();
        while i < line.len() {
            let b = line[i];
            if b == b'\\' {
                if i + 1 == line.len() {
                    return Err("trailing backslash");
                }
                word.extend_from_slice(&line[i..i + 2]);
                i += 2;
            } else if (quote != 0 && b == quote) || (quote == 0 && is_space(b)) {
                // The C code turns a closing quote into a separator.
                quote = 0;
                i += 1;
                words.push(std::mem::take(&mut word));
                skip_to_word(&mut i, &mut quote);
                if i == line.len() {
                    return if quote == 0 {
                        Ok(words)
                    } else {
                        Err("unterminated quote")
                    };
                }
            } else {
                word.push(b);
                i += 1;
            }
        }
        if quote != 0 {
            return Err("unterminated quote");
        }
        words.push(word);
        Ok(words)
    }

    /// Check a chart-protocol byte stream against the agent's grammar: every
    /// line `\n`-terminated and split as the agent splits it; known keywords
    /// with the expected word counts; valid ids; `int` priority and
    /// update_every; `DIMENSION`/`CLABEL` only after `CHART`;
    /// `CLABEL_COMMIT` only after a `CLABEL`; `BEGIN` only for a defined chart
    /// that is not obsolete; `SET` only inside `BEGIN`..`END`, once per
    /// dimension, only for a declared dimension, with float values (decimal or
    /// `@` base64, [`read_float_value`]) only on float dimensions; `END <t>` or `END <t> 0 1` with `t` strictly
    /// increasing per chart. Returns the first problem with its 1-based line
    /// number.
    pub fn validate_chart_stream(stream: &[u8]) -> Result<(), String> {
        let Some(body) = stream.strip_suffix(b"\n") else {
            return if stream.is_empty() {
                Ok(())
            } else {
                Err("stream does not end with a newline".to_string())
            };
        };
        let mut charts: HashMap<String, ChartState> = HashMap::new();
        let mut scope = Scope::None;
        for (index, line) in body.split(|&b| b == b'\n').enumerate() {
            let fail = |msg: String| Err(format!("line {}: {msg}", index + 1));
            let words: Vec<String> = match agent_words(line) {
                Ok(words) => words
                    .into_iter()
                    .map(|w| String::from_utf8_lossy(&w).into_owned())
                    .collect(),
                Err(problem) => return fail(problem.to_string()),
            };
            let Some((keyword, args)) = words.split_first() else {
                return fail("empty line".to_string());
            };
            match keyword.as_str() {
                "CHART" => {
                    if let Scope::Update { .. } = scope {
                        return fail("CHART inside BEGIN..END".to_string());
                    }
                    if args.len() != 9 && args.len() != 10 {
                        return fail(format!("CHART has {} arguments", args.len()));
                    }
                    let id = &args[0];
                    if !super::is_valid_chart_id(id) {
                        return fail(format!("invalid chart id {id:?}"));
                    }
                    if !["line", "area", "stacked", "heatmap"].contains(&args[6].as_str()) {
                        return fail(format!("unknown chart type {:?}", args[6]));
                    }
                    if args[7].parse::<i32>().is_err()
                        || args[8].parse::<i32>().map_or(true, |u| u <= 0)
                    {
                        return fail("priority or update_every is not a valid int".to_string());
                    }
                    let state = charts.entry(id.clone()).or_default();
                    state.obsolete = args.get(9).is_some_and(|o| o.contains("obsolete"));
                    scope = Scope::Definition {
                        chart: id.clone(),
                        labels: 0,
                    };
                }
                "DIMENSION" => {
                    let Scope::Definition { chart, .. } = &scope else {
                        return fail("DIMENSION outside a chart definition".to_string());
                    };
                    if args.len() != 5 && args.len() != 6 {
                        return fail(format!("DIMENSION has {} arguments", args.len()));
                    }
                    let id = &args[0];
                    if !super::is_valid_dimension_id(id) {
                        return fail(format!("invalid dimension id {id:?}"));
                    }
                    if args[3].parse::<i64>().is_err() || args[4].parse::<i64>().is_err() {
                        return fail("multiplier or divisor is not an integer".to_string());
                    }
                    let float = args.get(5).is_some_and(|o| o.contains("type=float"));
                    let state = charts.get_mut(chart).expect("defined by CHART");
                    if float {
                        state.int_dimensions.remove(id);
                        state.float_dimensions.insert(id.clone());
                    } else {
                        state.float_dimensions.remove(id);
                        state.int_dimensions.insert(id.clone());
                    }
                }
                "CLABEL" => {
                    let Scope::Definition { labels, .. } = &mut scope else {
                        return fail("CLABEL outside a chart definition".to_string());
                    };
                    if args.len() != 3 || args[2].parse::<u32>().is_err() {
                        return fail(
                            "CLABEL needs a key, a value and a numeric source".to_string(),
                        );
                    }
                    *labels += 1;
                }
                "CLABEL_COMMIT" => {
                    let Scope::Definition { labels, .. } = &mut scope else {
                        return fail("CLABEL_COMMIT outside a chart definition".to_string());
                    };
                    if *labels == 0 {
                        return fail("CLABEL_COMMIT without CLABEL".to_string());
                    }
                    *labels = 0;
                }
                "BEGIN" => {
                    if let Scope::Update { .. } = scope {
                        return fail("BEGIN inside BEGIN..END".to_string());
                    }
                    if args.len() != 2 || args[1].parse::<u64>().is_err() {
                        return fail("BEGIN needs a chart id and microseconds".to_string());
                    }
                    match charts.get(&args[0]) {
                        None => return fail(format!("BEGIN for undefined chart {:?}", args[0])),
                        Some(state) if state.obsolete => {
                            return fail(format!("BEGIN for obsolete chart {:?}", args[0]));
                        }
                        Some(_) => {}
                    }
                    scope = Scope::Update {
                        chart: args[0].clone(),
                        set: HashSet::new(),
                    };
                }
                "SET" => {
                    let Scope::Update { chart, set } = &mut scope else {
                        return fail("SET outside BEGIN..END".to_string());
                    };
                    // The agent reads `SET <dimension> = <value>` as two or
                    // three words: `=` is a separator.
                    if args.is_empty() || args.len() > 2 {
                        return fail("SET must read SET <dimension> = [value]".to_string());
                    }
                    let id = &args[0];
                    if !set.insert(id.clone()) {
                        return fail(format!("second SET for {id:?} in one update"));
                    }
                    let state = &charts[chart.as_str()];
                    let value = args.get(1);
                    if state.float_dimensions.contains(id) {
                        if value.is_some_and(|v| read_float_value(v).is_none_or(|f| !f.is_finite()))
                        {
                            return fail(format!("SET {id} has an invalid float value"));
                        }
                    } else if state.int_dimensions.contains(id) {
                        if value.is_some_and(|v| v.parse::<i64>().is_err()) {
                            return fail(format!("SET {id} has a non-integer value"));
                        }
                    } else {
                        return fail(format!("SET for undeclared dimension {id:?} of {chart:?}"));
                    }
                }
                "END" => {
                    let Scope::Update { chart, .. } = &scope else {
                        return fail("END without BEGIN".to_string());
                    };
                    let timed = args.len() == 3 && args[1] == "0" && args[2] == "1";
                    let time = args.first().and_then(|t| t.parse::<u64>().ok());
                    let (Some(time), true) = (time, args.len() == 1 || timed) else {
                        return fail("END must read END <time> or END <time> 0 1".to_string());
                    };
                    let state = charts.get_mut(chart).expect("checked by BEGIN");
                    if state.last_end.is_some_and(|last| time <= last) {
                        return fail(format!("END time {time} does not advance for {chart:?}"));
                    }
                    state.last_end = Some(time);
                    scope = Scope::None;
                }
                other => return fail(format!("unexpected keyword {other:?}")),
            }
        }
        if let Scope::Update { chart, .. } = scope {
            return Err(format!("stream ends inside BEGIN..END of {chart:?}"));
        }
        Ok(())
    }

    #[cfg(test)]
    #[test]
    fn splits_like_the_agent() {
        let words = |s: &str| -> Result<Vec<String>, &'static str> {
            agent_words(s.as_bytes()).map(|words| {
                words
                    .into_iter()
                    .map(|w| String::from_utf8(w).unwrap())
                    .collect()
            })
        };
        assert_eq!(words("SET d = 1").unwrap(), ["SET", "d", "1"]);
        assert_eq!(words("SET d =").unwrap(), ["SET", "d"]);
        assert_eq!(words("A type=float").unwrap(), ["A", "type", "float"]);
        assert_eq!(words("A 'type=float x'").unwrap(), ["A", "type=float x"]);
        assert_eq!(words("A '' 'b'").unwrap(), ["A", "", "b"]);
        assert_eq!(words("A 'it\"s' b").unwrap(), ["A", "it\"s", "b"]);
        assert_eq!(words(r"A 'x\' b'").unwrap(), ["A", r"x\' b"]);
        assert_eq!(words("SET d = '"), Err("unterminated quote"));
        assert_eq!(words("A 'b"), Err("unterminated quote"));
        assert_eq!(words(r"A 'b\"), Err("trailing backslash"));
        assert_eq!(words("A\0B"), Err("NUL byte"));
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn line(f: impl FnOnce(&mut Vec<u8>)) -> String {
        let mut buf = Vec::new();
        f(&mut buf);
        String::from_utf8(buf).unwrap()
    }

    fn chart<'a>(options: ChartOptions) -> ChartLine<'a> {
        ChartLine {
            id: "otel.derived_metrics.requests_x",
            name: "",
            title: "Requests",
            units: "requests/s",
            family: "requests",
            context: "otel.derived_metrics.requests",
            chart_type: ChartType::Stacked,
            priority: 70000,
            update_every: 1,
            options,
        }
    }

    #[test]
    fn chart_line_without_options() {
        assert_eq!(
            line(|b| write_chart(b, &chart(ChartOptions::default()))),
            "CHART otel.derived_metrics.requests_x '' 'Requests' 'requests/s' 'requests' \
             'otel.derived_metrics.requests' stacked 70000 1\n"
        );
    }

    #[test]
    fn chart_line_options_are_one_quoted_field() {
        let one = ChartOptions {
            obsolete: true,
            ..Default::default()
        };
        assert!(line(|b| write_chart(b, &chart(one))).ends_with(" stacked 70000 1 'obsolete'\n"));
        let two = ChartOptions {
            obsolete: true,
            store_first: true,
            hidden: false,
        };
        assert!(
            line(|b| write_chart(b, &chart(two)))
                .ends_with(" stacked 70000 1 'obsolete store_first'\n")
        );
    }

    #[test]
    fn dimension_lines() {
        let mut dim = DimensionLine {
            id: "error",
            name: "error",
            algorithm: DimensionAlgorithm::Absolute,
            multiplier: 1,
            divisor: 1,
            options: DimensionOptions::default(),
        };
        assert_eq!(
            line(|b| write_dimension(b, &dim)),
            "DIMENSION error 'error' absolute 1 1\n"
        );
        dim.options.float = true;
        assert_eq!(
            line(|b| write_dimension(b, &dim)),
            "DIMENSION error 'error' absolute 1 1 'type=float'\n"
        );
        dim.options.obsolete = true;
        dim.algorithm = DimensionAlgorithm::Incremental;
        assert_eq!(
            line(|b| write_dimension(b, &dim)),
            "DIMENSION error 'error' incremental 1 1 'type=float obsolete'\n"
        );
    }

    #[test]
    fn labels() {
        assert_eq!(
            line(|b| {
                write_clabel(
                    b,
                    "resource.attributes.service.name",
                    "checkout",
                    LabelSource::Auto,
                );
                write_clabel_commit(b);
            }),
            "CLABEL 'resource.attributes.service.name' 'checkout' 1\nCLABEL_COMMIT\n"
        );
        assert_eq!(
            line(|b| write_clabel(b, "k", "v", LabelSource::Config)),
            "CLABEL 'k' 'v' 2\n"
        );
    }

    #[test]
    fn update_block() {
        assert_eq!(
            line(|b| {
                write_begin(b, "a.b", 1_000_000);
                write_set_int(b, "x", -3);
                write_set_float(b, "y", 0.1);
                write_set_float(b, "z", f64::NAN);
                write_set_empty(b, "w");
                write_end(b, 1_700_000_000);
            }),
            "BEGIN a.b 1000000\nSET x = -3\nSET y = @D+5mZmZmZma\nSET z =\nSET w =\nEND 1700000000\n"
        );
        assert_eq!(line(|b| write_end_timed(b, 7)), "END 7 0 1\n");
    }

    #[test]
    fn float_values_are_ieee754_base64() {
        let set = |v: f64| line(|b| write_set_float(b, "d", v));
        // Expected words computed independently from libnetdata's algorithm.
        assert_eq!(set(1.0), "SET d = @D/wAAAAAAAA\n");
        assert_eq!(set(1.5), "SET d = @D/4AAAAAAAA\n");
        assert_eq!(set(0.1), "SET d = @D+5mZmZmZma\n");
        assert_eq!(set(-7.0), "SET d = @MAcAAAAAAAA\n");
        assert_eq!(set(f64::MAX), "SET d = @H/v////////\n");
        assert_eq!(set(5e-324), "SET d = @B\n");
        assert_eq!(set(0.0), "SET d = @A\n");
        assert_eq!(set(-0.0), "SET d = @A\n");
        assert_eq!(set(f64::INFINITY), "SET d =\n");
        assert_eq!(set(f64::NAN), "SET d =\n");
    }

    #[test]
    fn float_values_round_trip() {
        for value in [
            1e-7,
            2.5e10,
            0.0005,
            -0.5,
            123.456,
            f64::MIN_POSITIVE,
            f64::MIN,
            f64::EPSILON,
            9_007_199_254_740_993.0,
        ] {
            let text = line(|b| write_set_float(b, "d", value));
            let word = text.strip_prefix("SET d = ").unwrap().trim_end();
            assert_eq!(
                read_float_value(word).map(f64::to_bits),
                Some(value.to_bits()),
                "{value}"
            );
        }
        assert_eq!(read_float_value("1.5"), Some(1.5));
        assert_eq!(read_float_value("@"), None);
        assert_eq!(read_float_value("@A!"), None);
        // Leading zero digits are only zeros; more than 64 bits do not fit.
        assert_eq!(read_float_value("@AAAAAAAAAAAAA"), Some(0.0));
        assert_eq!(read_float_value("@BAAAAAAAAAAA"), None);
        assert_eq!(read_float_value("@QAAAAAAAAAA"), None);
    }

    #[test]
    fn readable_streams_show_decimals() {
        let stream = line(|b| {
            write_begin(b, "a.b", 1_000_000);
            write_set_float(b, "x", 1.5);
            write_set_float(b, "y", f64::NAN);
            write_set_int(b, "z", 7);
            write_end(b, 1);
        });
        assert_eq!(
            readable_chart_stream(stream.as_bytes()),
            "BEGIN a.b 1000000\nSET x = 1.5\nSET y =\nSET z = 7\nEND 1\n"
        );
    }

    #[test]
    fn quoted_fields_are_sanitized() {
        let quoted = |s: &str| line(|b| write_quoted(b, s));
        assert_eq!(quoted("plain text"), "'plain text'");
        assert_eq!(quoted("it's"), "'it\"s'");
        assert_eq!(quoted(r"C:\path\"), "'C:/path/'");
        assert_eq!(quoted("a\nb\rc\td"), "'a b c d'");
        assert_eq!(quoted("Ελλάδα"), "'Ελλάδα'");
        assert_eq!(quoted(""), "''");
    }

    #[test]
    fn id_validation() {
        assert!(is_valid_chart_id("a.b"));
        assert!(is_valid_chart_id("a.b.c"));
        assert!(!is_valid_chart_id("ab"));
        assert!(!is_valid_chart_id(".b"));
        assert!(!is_valid_chart_id("a."));
        assert!(!is_valid_chart_id("a.b c"));
        assert!(!is_valid_chart_id("a.b'"));
        assert!(is_valid_dimension_id("0.005"));
        assert!(is_valid_dimension_id("+Inf"));
        assert!(!is_valid_dimension_id(""));
        assert!(!is_valid_dimension_id("a=b"));
        assert!(!is_valid_dimension_id("SLOT:1"));
        assert!(!is_valid_chart_id("SLOT:1.a"));
    }

    #[test]
    fn a_chunk_holds_whole_blocks_under_the_budget() {
        let mut chunker = Chunker::new(10);
        for block in ["aaaa", "bbbb", "cccc", "a block of 22 bytes...", "dd", ""] {
            chunker.block(|buf| buf.extend_from_slice(block.as_bytes()));
        }

        let chunks: Vec<String> = chunker
            .finish()
            .into_iter()
            .map(|chunk| String::from_utf8(chunk).unwrap())
            .collect();
        // Two blocks fit, the third opens a chunk; the oversized block is
        // alone; an empty block adds nothing.
        assert_eq!(chunks, ["aaaabbbb", "cccc", "a block of 22 bytes...", "dd"]);

        assert!(Chunker::new(10).finish().is_empty());
    }

    #[test]
    fn sanitized_ids() {
        let chart = |id: &str| sanitize_chart_id(id).into_owned();
        assert_eq!(chart("system.cpu"), "system.cpu");
        assert_eq!(chart("my metric.42"), "my_metric.42");
        assert_eq!(chart("it's=odd\\\n.1"), "it_s_odd__.1");
        assert_eq!(chart("cpu"), "cpu._");
        assert_eq!(chart(".42"), "_.42");
        assert_eq!(chart("cpu."), "cpu._");
        assert_eq!(chart(""), "_._");
        assert_eq!(chart("SLOT:1.a"), "SLOT_1.a");
        assert_eq!(chart("μετρική.1"), "μετρική.1");
        // An id of the longest length is kept; a longer one is cut, on a
        // character boundary.
        let longest = format!("a.{}", "b".repeat(MAX_CHART_ID_BYTES - 2));
        assert_eq!(chart(&longest), longest);
        assert!(!is_valid_chart_id(&format!("{longest}b")));
        assert_eq!(
            chart(&format!("{longest}b")),
            format!("a.{}", "b".repeat(MAX_CHART_ID_BYTES - 4))
        );
        let greek = chart(&"μ".repeat(MAX_CHART_ID_BYTES));
        assert!(is_valid_chart_id(&greek), "{} bytes", greek.len());
        assert!(greek.ends_with("μ._"));

        let dimension = |id: &str| sanitize_dimension_id(id).into_owned();
        assert_eq!(dimension("0.005"), "0.005");
        assert_eq!(dimension("+Inf"), "+Inf");
        assert_eq!(dimension("GET /users"), "GET_/users");
        assert_eq!(dimension("a=b"), "a_b");
        assert_eq!(dimension(""), "_");
        assert_eq!(dimension("SLOT:1"), "SLOT_1");
    }

    /// Every string up to four characters over an alphabet that holds each
    /// kind of troublesome byte.
    #[test]
    fn a_sanitized_id_is_valid_and_a_valid_id_is_unchanged() {
        const ALPHABET: [char; 10] = ['a', '.', ' ', '\'', '"', '\\', '=', '\n', ':', 'μ'];
        let mut texts = vec![String::new()];
        for _ in 0..4 {
            let longer: Vec<String> = texts
                .iter()
                .flat_map(|text| ALPHABET.iter().map(move |c| format!("{text}{c}")))
                .collect();
            texts.extend(longer);
        }
        texts.extend(["SLOT:1.a", "SLOT:", "SLOT:.", "S.LOT:"].map(String::from));
        // Around the longest chart id, with a multi-byte character and a dot
        // at every place the cut can land on.
        for length in MAX_CHART_ID_BYTES - 4..=MAX_CHART_ID_BYTES + 4 {
            texts.push(format!("a.{}", "b".repeat(length - 2)));
            texts.push(format!("{}μ", "b".repeat(length - 2)));
            texts.push(format!("{}.", "b".repeat(length - 1)));
            texts.push(format!("{}.μμ", "b".repeat(length - 3)));
        }

        for text in &texts {
            let chart_id = sanitize_chart_id(text);
            assert!(is_valid_chart_id(&chart_id), "{text:?} -> {chart_id:?}");
            assert_eq!(is_valid_chart_id(text), chart_id == *text, "{text:?}");

            let dimension_id = sanitize_dimension_id(text);
            assert!(
                is_valid_dimension_id(&dimension_id),
                "{text:?} -> {dimension_id:?}"
            );
            assert_eq!(
                is_valid_dimension_id(text),
                dimension_id == *text,
                "{text:?}"
            );
        }
    }

    mod validator {
        use super::*;

        fn valid_stream() -> Vec<u8> {
            let mut b = Vec::new();
            write_chart(&mut b, &chart(ChartOptions::default()));
            write_clabel(&mut b, "k", "it's \\ odd\n", LabelSource::Auto);
            write_clabel_commit(&mut b);
            write_dimension(
                &mut b,
                &DimensionLine {
                    id: "ok",
                    name: "ok",
                    algorithm: DimensionAlgorithm::Absolute,
                    multiplier: 1,
                    divisor: 1,
                    options: DimensionOptions {
                        float: true,
                        ..Default::default()
                    },
                },
            );
            write_begin(&mut b, "otel.derived_metrics.requests_x", 1_000_000);
            write_set_float(&mut b, "ok", 1.5);
            write_end(&mut b, 10);
            write_begin(&mut b, "otel.derived_metrics.requests_x", 1_000_000);
            write_set_empty(&mut b, "ok");
            write_end_timed(&mut b, 11);
            b
        }

        #[test]
        fn accepts_encoder_output() {
            assert_eq!(validate_chart_stream(&valid_stream()), Ok(()));
            assert_eq!(validate_chart_stream(b""), Ok(()));
        }

        #[test]
        fn rejects_grammar_violations() {
            let cases: &[(&str, &str)] = &[
                ("CHART a.b 'x' line 1 1\n", "CHART has"),
                // A stray quote shifts every later word.
                (
                    "CHART a.b '' 'it's' 'u' 'f' 'c' line 1 1\n",
                    "unknown chart type",
                ),
                (
                    "DIMENSION d 'd' absolute 1 1\n",
                    "outside a chart definition",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nCLABEL_COMMIT\n",
                    "without CLABEL",
                ),
                ("BEGIN a.b 1\n", "undefined chart"),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nBEGIN a.b 1\nSET d = 1\nEND 1\n",
                    "undeclared dimension",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nDIMENSION d 'd' absolute 1 1\n\
                     BEGIN a.b 1\nSET d = 1.5\nEND 1\n",
                    "non-integer",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nDIMENSION d 'd' absolute 1 1 'type=float'\n\
                     BEGIN a.b 1\nSET d = @D/4!\nEND 1\n",
                    "invalid float value",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nDIMENSION d 'd' absolute 1 1\n\
                     BEGIN a.b 1\nEND 2\nBEGIN a.b 1\nEND 2\n",
                    "does not advance",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nBEGIN a.b 1\n",
                    "ends inside",
                ),
                ("CHART a.b '' 't' 'u' 'f' 'c' line 1 1", "newline"),
                ("CHART a.b '' 't' 'u' 'f' 'c' bar 1 1\n", "chart type"),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 99999999999 1\n",
                    "valid int",
                ),
                (
                    "CHART SLOT:1 '' 't' 'u' 'f' 'c' line 1 1\n",
                    "invalid chart id",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1 'obsolete'\nBEGIN a.b 1\n",
                    "obsolete chart",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nDIMENSION d 'd' absolute 1 1\n\
                     BEGIN a.b 1\nSET d = 1\nSET d = 2\nEND 1\n",
                    "second SET",
                ),
                (
                    "CHART a.b '' 't' 'u' 'f' 'c' line 1 1\nBEGIN a.b 1\nEND 1 0\n",
                    "END must read",
                ),
            ];
            for (stream, expected) in cases {
                let err = validate_chart_stream(stream.as_bytes()).expect_err(stream);
                assert!(err.contains(expected), "{stream:?}: {err}");
            }
        }
    }
}
