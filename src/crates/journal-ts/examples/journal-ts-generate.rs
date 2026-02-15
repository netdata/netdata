//! Generate a `.ts` sidecar file from a systemd journal file.
//!
//! Uses the same two-phase timestamp extraction as the file indexer:
//!
//! 1. **Source field phase**: iterate DATA objects for `_SOURCE_REALTIME_TIMESTAMP`,
//!    parse each timestamp value, expand the InlinedCursor to entry offsets.
//! 2. **Realtime fallback phase**: for entries not covered by the source field,
//!    read `entry.header.realtime` from the entry object.
//!
//! Usage:
//!     cargo run -p journal-ts --example journal-ts-generate -- -i /path/to/journal-file -o /path/to/output.ts

use std::collections::HashSet;
use std::fs;
use std::num::NonZeroU64;
use std::path::PathBuf;
use std::time::Instant;

use clap::Parser;
use journal_core::file::Mmap;
use journal_core::{JournalFile, OpenJournalFile};
use journal_ts::TimestampOffsetsWriter;

const SOURCE_FIELD: &[u8] = b"_SOURCE_REALTIME_TIMESTAMP";

#[derive(Parser)]
#[command(about = "Generate a .ts sidecar from a journal file")]
struct Args {
    /// Path to the input journal file.
    #[arg(short, long)]
    input: PathBuf,

    /// Path to write the .ts output file.
    #[arg(short, long)]
    output: PathBuf,
}

/// Parse a u64 timestamp from a DATA object's "FIELD=value" payload.
fn parse_timestamp(field_name: &[u8], payload: &[u8]) -> Option<u64> {
    // Payload format: "FIELD_NAME=1234567890\n" or "FIELD_NAME=1234567890"
    if !payload.starts_with(field_name) {
        return None;
    }
    let rest = &payload[field_name.len()..];
    let rest = rest.strip_prefix(b"=")?;
    let s = std::str::from_utf8(rest).ok()?;
    s.trim().parse::<u64>().ok()
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();

    let repo_file = journal_core::repository::File::from_path(&args.input)
        .ok_or_else(|| format!("not a valid journal file path: {}", args.input.display()))?;

    // Load hash tables — required for field_data_objects().
    let jf: JournalFile<Mmap> = OpenJournalFile::new(8 * 1024 * 1024)
        .load_hash_tables()
        .open(&repo_file)?;

    let header = jf.journal_header_ref();
    let n_entries = header.n_entries as usize;
    eprintln!(
        "journal: {} entries, time range [{}, {}]",
        n_entries, header.head_entry_realtime, header.tail_entry_realtime,
    );

    // ── Phase 1: source field timestamps ───────────────────────────

    let t0 = Instant::now();
    let bump = bumpalo::Bump::new();
    let mut writer = TimestampOffsetsWriter::with_capacity_in(n_entries, &bump);
    let mut source_offsets: HashSet<NonZeroU64> = HashSet::with_capacity(n_entries);

    let field_iter = jf.field_data_objects(SOURCE_FIELD)?;
    let mut cursor_offsets: Vec<NonZeroU64> = Vec::new();

    for data_object_result in field_iter {
        let data_object = match data_object_result {
            Ok(d) => d,
            Err(_) => continue,
        };

        let Some(timestamp) = parse_timestamp(SOURCE_FIELD, data_object.raw_payload()) else {
            continue;
        };

        let Some(ic) = data_object.inlined_cursor() else {
            continue;
        };

        // Expand the cursor to all entry offsets that share this DATA value.
        cursor_offsets.clear();
        if ic.collect_offsets(&jf, &mut cursor_offsets).is_err() {
            continue;
        }

        for &entry_offset in &cursor_offsets {
            writer.push(timestamp, entry_offset.get());
            source_offsets.insert(entry_offset);
        }
    }

    let source_count = writer.len();
    eprintln!(
        "phase 1: {} entries from source field in {:.2?}",
        source_count,
        t0.elapsed(),
    );

    // ── Phase 2: realtime fallback ─────────────────────────────────

    let t1 = Instant::now();
    let mut entry_offsets: Vec<NonZeroU64> = Vec::with_capacity(n_entries);
    jf.entry_offsets(&mut entry_offsets)?;

    let mut fallback_count = 0usize;
    for &entry_offset in &entry_offsets {
        if source_offsets.contains(&entry_offset) {
            continue;
        }

        let entry = jf.entry_ref(entry_offset)?;
        let realtime = entry.header.realtime;
        writer.push(realtime, entry_offset.get());
        fallback_count += 1;
    }

    eprintln!(
        "phase 2: {} entries from realtime fallback in {:.2?}",
        fallback_count,
        t1.elapsed(),
    );

    eprintln!(
        "total: {} entries ({} source + {} fallback)",
        writer.len(),
        source_count,
        fallback_count,
    );

    // ── Write the .ts file (HTIM + HCNT + EOFF) ──────────────────

    let t2 = Instant::now();
    let mut buf = Vec::new();
    writer.write_to(&mut buf)?;
    fs::write(&args.output, &buf)?;

    eprintln!(
        "wrote {} ({} bytes) in {:.2?}",
        args.output.display(),
        buf.len(),
        t2.elapsed(),
    );

    Ok(())
}
