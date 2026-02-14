mod output;

use anyhow::{Context, Result, bail};
use chrono::{Local, NaiveDate, NaiveDateTime, TimeZone};
use clap::Parser;
use journal_core::file::HashableObject;
use journal_core::file::mmap::Mmap;
use journal_core::{JournalFile, JournalReader, install_sigbus_handler};
use output::Entry;
use regex::Regex;
use std::collections::BTreeMap;
use std::io::{self, BufWriter, Write};
use std::path::Path;

#[derive(Parser)]
#[command(name = "jctl", about = "Query systemd journal files")]
struct Cli {
    /// Field filters (FIELD=VALUE), use + for OR between groups
    matches: Vec<String>,

    /// Use journal files from directory
    #[arg(short = 'D', long)]
    directory: Option<String>,

    /// Use a specific journal file
    #[arg(long)]
    file: Option<String>,

    /// Output format: short, verbose, json, cat
    #[arg(short, long, default_value = "short")]
    output: String,

    /// Show at most N entries
    #[arg(short = 'n', long)]
    lines: Option<usize>,

    /// Newest entries first
    #[arg(short, long)]
    reverse: bool,

    /// Show entries on or after TIME
    #[arg(short = 'S', long)]
    since: Option<String>,

    /// Show entries on or before TIME
    #[arg(short = 'U', long)]
    until: Option<String>,

    /// Filter by priority (0-7), show this priority and more important
    #[arg(short, long)]
    priority: Option<u8>,

    /// Grep MESSAGE field with regex
    #[arg(short, long)]
    grep: Option<String>,

    /// List unique values for FIELD
    #[arg(short = 'F', long)]
    field: Option<String>,

    /// List all field names
    #[arg(long)]
    fields: bool,

    /// Show journal file header info
    #[arg(long)]
    header: bool,

    /// Timestamps in UTC
    #[arg(long)]
    utc: bool,
}

fn parse_timestamp(s: &str) -> Result<u64> {
    // Raw integer → microseconds since epoch
    if let Ok(usec) = s.parse::<u64>() {
        return Ok(usec);
    }

    let s_lower = s.to_lowercase();

    if s_lower == "now" {
        let now = chrono::Utc::now().timestamp_micros() as u64;
        return Ok(now);
    }

    if s_lower == "today" {
        let today = Local::now().date_naive().and_hms_opt(0, 0, 0).unwrap();
        let dt = Local
            .from_local_datetime(&today)
            .single()
            .context("ambiguous local time for 'today'")?;
        return Ok(dt.timestamp_micros() as u64);
    }

    if s_lower == "yesterday" {
        let yesterday = Local::now()
            .date_naive()
            .pred_opt()
            .context("date underflow")?
            .and_hms_opt(0, 0, 0)
            .unwrap();
        let dt = Local
            .from_local_datetime(&yesterday)
            .single()
            .context("ambiguous local time for 'yesterday'")?;
        return Ok(dt.timestamp_micros() as u64);
    }

    // "YYYY-MM-DD HH:MM:SS"
    if let Ok(ndt) = NaiveDateTime::parse_from_str(s, "%Y-%m-%d %H:%M:%S") {
        let dt = Local
            .from_local_datetime(&ndt)
            .single()
            .context("ambiguous local time")?;
        return Ok(dt.timestamp_micros() as u64);
    }

    // "YYYY-MM-DD"
    if let Ok(nd) = NaiveDate::parse_from_str(s, "%Y-%m-%d") {
        let ndt = nd.and_hms_opt(0, 0, 0).unwrap();
        let dt = Local
            .from_local_datetime(&ndt)
            .single()
            .context("ambiguous local time")?;
        return Ok(dt.timestamp_micros() as u64);
    }

    // "N unit ago" — relative time
    if let Some(rest) = s_lower.strip_suffix(" ago") {
        let (num_str, unit) = rest
            .trim()
            .rsplit_once(' ')
            .with_context(|| format!("cannot parse relative time: {s:?}"))?;
        let n: i64 = num_str
            .parse()
            .with_context(|| format!("invalid number in relative time: {s:?}"))?;

        const USEC_PER_SEC: i64 = 1_000_000;
        let offset_usec = match unit {
            "second" | "seconds" | "sec" | "s" => n * USEC_PER_SEC,
            "minute" | "minutes" | "min" => n * 60 * USEC_PER_SEC,
            "hour" | "hours" | "hr" => n * 3600 * USEC_PER_SEC,
            "day" | "days" => n * 86400 * USEC_PER_SEC,
            "week" | "weeks" => n * 7 * 86400 * USEC_PER_SEC,
            "month" | "months" => n * 30 * 86400 * USEC_PER_SEC,
            "year" | "years" => n * 365 * 86400 * USEC_PER_SEC,
            _ => bail!("unknown time unit in relative time: {unit:?}"),
        };

        let now = chrono::Utc::now().timestamp_micros();
        return Ok((now - offset_usec) as u64);
    }

    bail!("cannot parse timestamp: {s:?}")
}

fn resolve_path(path_str: &str) -> Result<std::path::PathBuf> {
    let path = Path::new(path_str);
    if path.is_absolute() {
        Ok(path.to_path_buf())
    } else {
        Ok(std::env::current_dir()?.join(path))
    }
}

fn open_journal_file(path: &Path) -> Result<JournalFile<Mmap>> {
    let repo_file = journal_core::repository::File::from_path(path)
        .with_context(|| format!("cannot parse journal path: {}", path.display()))?;
    let journal_file = JournalFile::<Mmap>::open(&repo_file, 8 * 1024 * 1024)
        .with_context(|| format!("cannot open journal file: {}", path.display()))?;
    Ok(journal_file)
}

fn cmd_header(journal_file: &JournalFile<Mmap>) -> Result<()> {
    let hdr = journal_file.journal_header_ref();
    let stdout = io::stdout();
    let mut w = BufWriter::new(stdout.lock());

    writeln!(
        w,
        "File ID:          {}",
        uuid::Uuid::from_bytes(hdr.file_id).as_hyphenated()
    )?;
    writeln!(
        w,
        "Machine ID:       {}",
        uuid::Uuid::from_bytes(hdr.machine_id).as_hyphenated()
    )?;
    writeln!(
        w,
        "Boot ID (tail):   {}",
        uuid::Uuid::from_bytes(hdr.tail_entry_boot_id).as_hyphenated()
    )?;
    writeln!(
        w,
        "Seqnum ID:        {}",
        uuid::Uuid::from_bytes(hdr.seqnum_id).as_hyphenated()
    )?;
    writeln!(w, "State:            {}", hdr.state)?;

    writeln!(w, "Compatible flags: {:#010x}", hdr.compatible_flags)?;
    writeln!(w, "Incompatible flags: {:#010x}", hdr.incompatible_flags)?;

    writeln!(w, "Header size:      {}", hdr.header_size)?;
    writeln!(w, "Arena size:       {}", hdr.arena_size)?;
    writeln!(w, "Objects:          {}", hdr.n_objects)?;
    writeln!(w, "Entries:          {}", hdr.n_entries)?;

    writeln!(w, "Head seqnum:      {}", hdr.head_entry_seqnum)?;
    writeln!(w, "Tail seqnum:      {}", hdr.tail_entry_seqnum)?;
    writeln!(w, "Head realtime:    {}", hdr.head_entry_realtime)?;
    writeln!(w, "Tail realtime:    {}", hdr.tail_entry_realtime)?;
    writeln!(w, "Tail monotonic:   {}", hdr.tail_entry_monotonic)?;

    w.flush()?;
    Ok(())
}

fn cmd_fields(journal_file: &JournalFile<Mmap>) -> Result<()> {
    let mut reader = JournalReader::<Mmap>::default();
    let stdout = io::stdout();
    let mut w = BufWriter::new(stdout.lock());

    while let Some(field_guard) = reader.fields_enumerate(journal_file)? {
        let payload = field_guard.raw_payload();
        if let Ok(s) = std::str::from_utf8(payload) {
            writeln!(w, "{s}")?;
        }
    }

    w.flush()?;
    Ok(())
}

fn cmd_field_values(journal_file: &JournalFile<Mmap>, field_name: &str) -> Result<()> {
    let mut reader = JournalReader::<Mmap>::default();
    reader.field_data_query_unique(journal_file, field_name.as_bytes())?;

    let stdout = io::stdout();
    let mut w = BufWriter::new(stdout.lock());
    let mut seen = std::collections::BTreeSet::new();

    while let Some(data_guard) = reader.field_data_enumerate(journal_file)? {
        let payload = if data_guard.is_compressed() {
            let mut buf = Vec::new();
            data_guard.decompress(&mut buf)?;
            buf
        } else {
            data_guard.raw_payload().to_vec()
        };

        // payload is FIELD=VALUE, extract value after '='
        if let Some(eq_pos) = payload.iter().position(|&b| b == b'=') {
            let value = &payload[eq_pos + 1..];
            if let Ok(s) = std::str::from_utf8(value) {
                if seen.insert(s.to_string()) {
                    writeln!(w, "{s}")?;
                }
            }
        }
    }

    w.flush()?;
    Ok(())
}

fn open_session(cli: &Cli) -> Result<journal_session::JournalSession> {
    if let Some(ref file_path) = cli.file {
        let abs_path = resolve_path(file_path)?;
        return journal_session::JournalSession::from_files(vec![abs_path])
            .with_context(|| format!("failed to open journal file: {file_path}"));
    }

    if let Some(ref dir_path) = cli.directory {
        let abs_path = resolve_path(dir_path)?;
        let path_str = abs_path
            .to_str()
            .context("directory path is not valid UTF-8")?;
        return journal_session::JournalSession::open(path_str)
            .with_context(|| format!("failed to open journal directory: {dir_path}"));
    }

    // Default: scan standard journal directories.
    for dir in &["/var/log/journal", "/run/log/journal"] {
        if Path::new(dir).is_dir() {
            return journal_session::JournalSession::open(dir)
                .with_context(|| format!("failed to open {dir}"));
        }
    }

    bail!("no journal directory found; specify --directory or --file")
}

fn main() -> Result<()> {
    install_sigbus_handler().context("failed to install SIGBUS handler")?;

    let cli = Cli::parse();

    // Dispatch special commands (single-file, use journal-core directly).
    if cli.header || cli.fields || cli.field.is_some() {
        let file_path = cli
            .file
            .as_deref()
            .context("--header, --fields, and -F require --file")?;
        let abs_path = resolve_path(file_path)?;
        let journal_file = open_journal_file(&abs_path)?;

        if cli.header {
            return cmd_header(&journal_file);
        }
        if cli.fields {
            return cmd_fields(&journal_file);
        }
        if let Some(ref field_name) = cli.field {
            return cmd_field_values(&journal_file, field_name);
        }
    }

    // Main iteration path — use journal-session.
    let direction = if cli.reverse {
        journal_session::Direction::Backward
    } else {
        journal_session::Direction::Forward
    };

    let since_usec = cli.since.as_deref().map(parse_timestamp).transpose()?;
    let until_usec = cli.until.as_deref().map(parse_timestamp).transpose()?;

    let session = open_session(&cli)?;
    let mut builder = session.cursor_builder().direction(direction);

    if let Some(usec) = since_usec {
        builder = builder.since(usec);
    }
    if let Some(usec) = until_usec {
        builder = builder.until(usec);
    }

    for m in &cli.matches {
        if m == "+" {
            builder = builder.add_disjunction();
        } else if m.contains('=') {
            builder = builder.add_match(m.as_bytes());
        } else {
            bail!("invalid match filter (expected FIELD=VALUE or +): {m:?}");
        }
    }

    let mut cursor = builder.build()?;

    // Compile --grep regex
    let grep_re = cli
        .grep
        .as_deref()
        .map(|pat| Regex::new(pat).with_context(|| format!("invalid regex: {pat:?}")))
        .transpose()?;

    let stdout = io::stdout();
    let mut w = BufWriter::new(stdout.lock());
    let mut count = 0usize;
    let limit = cli.lines.unwrap_or(usize::MAX);

    while cursor.step()? {
        if count >= limit {
            break;
        }

        let realtime_usec = cursor.realtime_usec();
        let monotonic_usec = cursor.monotonic_usec();
        let boot_id = cursor.boot_id();

        // Collect fields from payloads.
        let mut fields = BTreeMap::new();
        let mut payloads = cursor.payloads()?;
        while let Some(data) = payloads.next()? {
            if let Some(eq_pos) = data.iter().position(|&b| b == b'=') {
                let key = &data[..eq_pos];
                let value = &data[eq_pos + 1..];
                if let Ok(key_str) = std::str::from_utf8(key) {
                    fields.insert(key_str.to_string(), value.to_vec());
                }
            }
        }

        // Priority filter
        if let Some(max_priority) = cli.priority {
            if let Some(prio_val) = fields.get("PRIORITY") {
                if let Ok(prio_str) = std::str::from_utf8(prio_val) {
                    if let Ok(p) = prio_str.parse::<u8>() {
                        if p > max_priority {
                            continue;
                        }
                    }
                }
            }
        }

        // Grep filter
        if let Some(ref re) = grep_re {
            let message = fields
                .get("MESSAGE")
                .and_then(|v| std::str::from_utf8(v).ok())
                .unwrap_or("");
            if !re.is_match(message) {
                continue;
            }
        }

        let entry = Entry {
            realtime_usec,
            monotonic_usec,
            boot_id,
            fields,
        };

        match cli.output.as_str() {
            "short" => output::format_short(&entry, cli.utc, &mut w)?,
            "verbose" => output::format_verbose(&entry, cli.utc, &mut w)?,
            "json" => output::format_json(&entry, &mut w)?,
            "cat" => output::format_cat(&entry, &mut w)?,
            other => {
                bail!("unknown output format: {other:?} (expected: short, verbose, json, cat)")
            }
        }

        count += 1;
    }

    w.flush()?;
    Ok(())
}
