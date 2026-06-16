//! Offline inspector for OTel logs stored in Netdata's WAL/SFST files.
//!
//! The CLI is a thin shell over the wire-neutral `sfsq` query engine: it
//! resolves the WAL/SFST directories ([`config`]), discovers query sources off
//! disk ([`discover`]), builds a neutral query ([`query`]), runs the engine,
//! and formats the result ([`output`]). No running agent is required.

pub mod config;
pub mod discover;
pub mod output;
pub mod query;

use std::sync::Arc;
use std::sync::atomic::AtomicUsize;

use sfsq::logs::{LogSource, LogsData, LogsQuery, run};
use tokio_util::sync::CancellationToken;

/// Run the (synchronous) engine over `sources`. Thin wrapper that supplies the
/// non-cancelled token and a throwaway progress counter the CLI does not
/// surface.
pub fn run_query(sources: Vec<LogSource>, query: LogsQuery) -> LogsData {
    run(
        sources,
        query,
        CancellationToken::new(),
        Arc::new(AtomicUsize::new(0)),
    )
}
