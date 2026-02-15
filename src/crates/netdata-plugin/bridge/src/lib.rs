//! Bridge types for communication between a plugin supervisor and its
//! worker subprocesses over ferryboat IPC.
//!
//! These types are protocol-agnostic — workers never see pluginsd. The
//! supervisor translates between the agent's pluginsd protocol and these
//! typed messages.
//!
//! ## Handshake protocol
//!
//! 1. Worker connects to the supervisor's IPC socket.
//! 2. Supervisor sends `Configure` with the worker's resolved config.
//! 3. Worker initializes, then sends `Ready` with zero or more function declarations.
//! 4. Supervisor registers the declarations and enters its main loop.

pub mod config;

use netdata_plugin_types::{FunctionDeclaration, FunctionResult};
use serde::{Deserialize, Serialize};

use config::PluginConfig;

// --- Ingestor subprocess ---

/// Messages sent from the supervisor to the ingestor worker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum IngestorRequest {
    /// Configuration for the worker (sent once after connection).
    Configure(PluginConfig),
    /// Execute a function.
    Call {
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    },
    /// Cancel a running function.
    Cancel { transaction: String },
    /// Shut down gracefully.
    Shutdown,
}

/// Messages sent from the ingestor worker back to the supervisor.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum IngestorResponse {
    /// Worker is initialized and ready. Carries zero or more function declarations.
    Ready {
        declarations: Vec<FunctionDeclaration>,
    },
    /// Function completed.
    Result(FunctionResult),
    /// Progress update for a running function.
    Progress {
        transaction: String,
        done: usize,
        total: usize,
    },
    /// Raw chart protocol data to proxy to the agent's stdout.
    ChartData { payload: Vec<u8> },
}

// --- Ledger subprocess ---

/// Messages sent from the supervisor to the ledger worker.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LedgerRequest {
    /// Configuration for the worker (sent once after connection).
    Configure(PluginConfig),
    /// Execute a function.
    Call {
        transaction: String,
        timeout: u32,
        name: String,
        args: Vec<String>,
        payload: Option<Vec<u8>>,
    },
    /// Cancel a running function.
    Cancel { transaction: String },
    /// Shut down gracefully.
    Shutdown,
}

/// Messages sent from the ledger worker back to the supervisor.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LedgerResponse {
    /// Worker is initialized and ready. Carries zero or more function declarations.
    Ready {
        declarations: Vec<FunctionDeclaration>,
    },
    /// Function completed.
    Result(FunctionResult),
    /// Progress update for a running function.
    Progress {
        transaction: String,
        done: usize,
        total: usize,
    },
}
