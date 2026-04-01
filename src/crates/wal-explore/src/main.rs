mod otap_dump;
mod otap_dump_index;
mod otap_frame;
mod otap_index;
mod otap_read;
mod otap_schema;
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
