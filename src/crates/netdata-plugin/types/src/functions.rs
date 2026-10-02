//! Pluginsd Function payload types for Netdata's plugin protocol.
//!
//! The six structs below are the Function protocol's data model, shared across
//! the netdata-plugin crates: netdata-plugin-protocol maps them onto the wire
//! (`tokio_codec` encodes, `message_parser` decodes, hand-rolled rather than
//! via the serde derives), the `rt` and `bridge` engines run a [`FunctionCall`]
//! through a `FunctionHandler` into a [`FunctionResult`], and the otel plugin's
//! supervisor forwards worker declarations and results to the agent. The agent
//! side of every contract here is `pluginsd_function*()` in
//! src/plugins.d/pluginsd_functions.c.
use serde::{Deserialize, Serialize};

use crate::HttpAccess;

/// A function declaration, plugin-to-agent: registers one callable function in
/// the agent's registry. Wire:
/// `FUNCTION [GLOBAL] name timeout help [tags [access [priority [version]]]]`,
/// strictly positional, so optional fields can only be dropped from the end,
/// which is why the trailing fields nest as Options. name, timeout and help
/// are required; the agent rejects a declaration missing any of them.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionDeclaration {
    /// Leading `GLOBAL` wire word. Chart-scoped functions no longer exist, so
    /// the agent registers the function host-wide either way
    /// (pluginsd_function in src/plugins.d/pluginsd_functions.c); the flag
    /// only shapes the encoded line.
    pub global: bool,
    /// Function name: the agent's registry key for the declaration and the
    /// dispatch key for incoming calls.
    pub name: String,
    /// Default timeout in seconds the agent applies when a caller doesn't set
    /// one; a missing or non-positive word falls back to the agent's own
    /// default (10s).
    pub timeout: u32,
    /// Required help text, published with the function in the agent's
    /// function catalog.
    pub help: String,
    /// Optional catalog tags word, e.g. "logs".
    pub tags: Option<String>,
    /// Access bits a caller must hold to call the function; an empty set
    /// imposes no requirement (the agent's check is
    /// `(caller & declared) == declared`).
    pub access: Option<HttpAccess>,
    /// Catalog metadata the agent publishes with the function; non-positive
    /// values are normalized to NRPC_PRIORITY_DEFAULT (100) agent-side.
    pub priority: Option<u32>,
    /// Catalog metadata the agent passes through; absent on the wire it
    /// defaults to NRPC_VERSION_DEFAULT (0).
    pub version: Option<u32>,
}

impl FunctionDeclaration {
    /// Base declaration every in-tree handler starts from and then adjusts:
    /// timeout 10s (the agent's PLUGINS_FUNCTIONS_TIMEOUT_DEFAULT), tags
    /// "logs", an empty access set, priority 200, version 1, not global.
    pub fn new(name: &str, help: &str) -> Self {
        Self {
            name: String::from(name),
            help: String::from(help),
            timeout: 10,
            tags: Some("logs".to_string()),
            access: Some(HttpAccess::from_u32(0)),
            priority: Some(200),
            version: Some(1),
            global: false,
        }
    }
}

/// A function call, agent-to-plugin: one invocation of a declared function.
/// Wire: `FUNCTION transaction timeout "name [args ...]" [access] [source]`
/// inline, or the same header as a `FUNCTION_PAYLOAD ... [content_type]`
/// block whose body appends into `payload` (that trailing content-type word
/// has no field here). transaction, timeout and the quoted name word are
/// required. Sender: pluginsd_calls_insert_cb() in
/// src/plugins.d/pluginsd_functions.c.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionCall {
    /// Agent-generated call id (a UUID string); echo it verbatim on every
    /// result, progress report and cancel for this call.
    pub transaction: String,
    /// Seconds the agent will still wait: its remaining deadline at send
    /// time, not the declaration's default. Not enforced here; the agent
    /// cancels overdue calls with FUNCTION_CANCEL.
    pub timeout: u32,
    /// Function name; opens the quoted "name args..." wire word.
    pub name: String,
    /// The rest of that quoted word, split on spaces (empty when there are
    /// none).
    pub args: Vec<String>,
    /// The caller's access bits as the agent sent them (hex word); None when
    /// the word is missing.
    pub access: Option<HttpAccess>,
    /// Free-form caller description; the agent always sends the word, using
    /// "" when it has none, so None only means the word was absent.
    pub source: Option<String>,
    /// Call body: `Some` for FUNCTION_PAYLOAD calls, whose block body
    /// appends into it; `None` for inline FUNCTION calls (in-tree handlers
    /// then parse an empty JSON object).
    pub payload: Option<Vec<u8>>,
}

/// A function result, plugin-to-agent: completes one call. Wire:
/// `FUNCTION_RESULT_BEGIN transaction status format expires`, then the raw
/// `payload` bytes, then `FUNCTION_RESULT_END` alone on its line. All four
/// header words are required and unquoted, so the transaction must not
/// contain spaces (the agent treats it as a UUID).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionResult {
    /// Call id echoed back from the FunctionCall being completed.
    pub transaction: String,
    /// HTTP status code; the agent rewrites a non-positive one to its
    /// backend-error code (591, HTTP_RESP_BACKEND_RESPONSE_INVALID).
    pub status: u32,
    /// Content type of `payload`; the agent maps an unknown or empty word to
    /// text/plain (content_type_string2id).
    pub format: String,
    /// Unix epoch seconds. The agent serves the response cacheable only while
    /// this is in the future, so 0 (or a past value) means no-cache. In-tree
    /// engines send now+2 on success and 0 on errors.
    pub expires: u64,
    /// Raw response body; the Encoder appends a \n when the bytes don't end
    /// with one, so FUNCTION_RESULT_END stays alone on its line.
    pub payload: Vec<u8>,
}

/// A cancellation, agent-to-plugin: stop work on one call. Wire:
/// `FUNCTION_CANCEL transaction`. The agent sends it when a caller cancels
/// the call or its deadline passes (pluginsd_calls_garbage_collect).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionCancel {
    /// Call id whose work should stop; in-tree runtimes trip the handler's
    /// cancellation token with it.
    pub transaction: String,
}

/// A progress query relayed to the plugin, agent-to-plugin. Wire:
/// `FUNCTION_PROGRESS transaction`. The agent sends it when a progress query
/// arrives for the call (nrpc_call_progress in src/nrpc/nrpc-calls.c), which
/// also extends that call's deadline by 10s
/// (FUNCTIONS_EXTENDED_TIME_ON_PROGRESS_UT). A plugin may re-report progress
/// in response; the in-tree runtimes push progress proactively and ignore
/// the request.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionProgressRequest {
    /// Call id to re-report progress for.
    pub transaction: String,
}

/// A progress report, plugin-to-agent. Wire:
/// `FUNCTION_PROGRESS transaction done all`, all three words required.
/// done/all are cumulative unit counts of one call, not increments and not
/// percentages; each engine picks the scale (rt reports raw counters, bridge
/// reports percent as done/100). The agent drops zero-valued updates
/// (query_progress_functions_update in src/libnetdata/query_progress).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FunctionProgressResponse {
    /// Call id the report belongs to.
    pub transaction: String,
    /// Work units completed so far.
    pub done: usize,
    /// Total work units.
    pub all: usize,
}
