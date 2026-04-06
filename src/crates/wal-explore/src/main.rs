mod otap_batch_ref;
mod otap_batch_split;
mod otap_compact;
mod otap_dump;
mod otap_no_bitmaps;
mod otap_pcodec;
mod otap_strip_keys;
mod otap_dump_index;
mod otap_frame;
mod otap_index;
mod otap_read;
mod otap_schema;
mod otap_sections;
mod process_frame;

use std::path::PathBuf;

use clap::{Parser, Subcommand};

#[derive(Parser)]
#[command(name = "wal-explore")]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Print log entries from an Arrow-format WAL file.
    Dump {
        /// Path to WAL file.
        file: PathBuf,
    },
    /// Print the Arrow schema of each batch type from the first frame.
    Schema {
        /// Path to WAL file.
        file: PathBuf,
    },
    /// Read and inspect an .sfst index file.
    Read {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Print reconstructed log entries from an .sfst index file.
    DumpIndex {
        /// Path to .sfst file.
        file: PathBuf,
        /// Max log entries to print (default: all).
        #[arg(short = 'n', long)]
        limit: Option<u32>,
    },
    /// Print section sizes of an .sfst index file.
    Sections {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Estimate size reduction from compact bitmap descriptors.
    CompactEstimate {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Measure overhead of splitting stream entries into batches.
    BatchSplitEstimate {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Estimate savings from replacing high-card bitmaps with batch references.
    BatchRefEstimate {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Estimate savings from dropping bitmaps in high-card chunks.
    NoBitmapsEstimate {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Estimate savings from stripping field name prefix in high-card chunks.
    StripKeysEstimate {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Compare stream entry compression: zstd vs pcodec.
    PcodecEstimate {
        /// Path to .sfst file.
        file: PathBuf,
    },
    /// Build a roaring bitmap index for every key=value pair.
    Index {
        /// Path to WAL file.
        file: PathBuf,
        /// Max log entries to index (default: all).
        #[arg(short = 'n', long)]
        limit: Option<u32>,
        /// Fields with fewer unique values than this go into the primary FST.
        #[arg(long, default_value_t = 100)]
        cardinality_threshold: u32,
    },
}

fn main() {
    let cli = Cli::parse();

    match cli.command {
        Command::Dump { file } => {
            if let Err(e) = otap_dump::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::Schema { file } => {
            if let Err(e) = otap_schema::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::Read { file } => {
            if let Err(e) = otap_read::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::Sections { file } => {
            if let Err(e) = otap_sections::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::CompactEstimate { file } => {
            if let Err(e) = otap_compact::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::BatchSplitEstimate { file } => {
            if let Err(e) = otap_batch_split::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::BatchRefEstimate { file } => {
            if let Err(e) = otap_batch_ref::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::NoBitmapsEstimate { file } => {
            if let Err(e) = otap_no_bitmaps::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::StripKeysEstimate { file } => {
            if let Err(e) = otap_strip_keys::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::PcodecEstimate { file } => {
            if let Err(e) = otap_pcodec::run(&file) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::DumpIndex { file, limit } => {
            if let Err(e) = otap_dump_index::run(&file, limit) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }
        }
        Command::Index {
            file,
            limit,
            cardinality_threshold,
        } => {
            if let Err(e) = otap_index::run(&file, limit, cardinality_threshold) {
                eprintln!("{}: {e}", file.display());
                std::process::exit(1);
            }

            otap_index::print_rss();
        }
    }
}
