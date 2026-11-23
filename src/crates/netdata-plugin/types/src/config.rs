//! The dyncfg configuration-declaration payload: [`ConfigDeclaration`], the
//! struct a plugin fills in to register one dynamic configuration entry with
//! the agent over the pluginsd `CONFIG` line. Field vocabularies live in the
//! sibling `dyncfg_*` and `http_access` modules.
//!
//! The declaration travels as the pluginsd `CONFIG` line (encoded by
//! `Message::ConfigDeclaration` in netdata-plugin-protocol, received by
//! `pluginsd_config()` in src/plugins.d/pluginsd_dyncfg.c). The wire form is
//! plugin→agent only: the protocol parser has no handler arm for the CONFIG
//! keyword, so an inbound line panics in `line_parser` instead of decoding.
use crate::{DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess};

/// A dyncfg configuration entry a plugin registers with the agent: the payload
/// of the pluginsd `CONFIG` line, encoded by `Message::ConfigDeclaration`
/// (netdata-plugin-protocol) as `CONFIG <id> CREATE <status> <type> <path>
/// <source_type> <source> <cmds> <view_access> <edit_access>` and parsed by
/// `pluginsd_config()` (src/plugins.d/pluginsd_dyncfg.c). On the wire,
/// `status`, `type` and `source_type` print their lowercase vocabulary names,
/// `cmds` prints pipe-separated (`get | schema`) and the access masks print as
/// `0x…` hex; `id`, `path` and `source` are quoted with `quote_if_needed`
/// (single quotes, embedded `'` escaped as `\'`).
///
/// Note: the codec sends the action word uppercase (`CREATE`) while the agent
/// compares it case-sensitively against `create`, so a declaration sent through
/// `MessageWriter` is logged as an unknown action instead of registered — see
/// the `Message::ConfigDeclaration` comment in the protocol crate.
///
/// Two agent-side normalizations apply at registration (`dyncfg_add_low_level`,
/// src/daemon/dyncfg/dyncfg.c): an empty access mask means the agent's default
/// permissions for that role rather than "no access", and the declared command
/// set is sanitized against `type` and `source_type`.
#[derive(Debug, Clone)]
pub struct ConfigDeclaration {
    /// Dyncfg id the config is registered under (e.g. `go.d:nginx`); must
    /// contain no whitespace or `'` — the agent rejects the declaration
    /// otherwise (`dyncfg_is_valid_id`, src/libnetdata/inicfg/dyncfg.c).
    pub id: String,
    /// Status the config is registered with (the agent stores it as-is).
    pub status: DynCfgStatus,
    /// The underscore avoids the Rust keyword.
    pub type_: DynCfgType,
    /// UI path the config is organized under (e.g. `/collectors`).
    pub path: String,
    /// Provenance kind of the config (see [`source`](Self::source)).
    pub source_type: DynCfgSourceType,
    /// Free-form provenance text (e.g. `Plugin-generated configuration`).
    pub source: String,
    /// Dyncfg commands the plugin can serve.
    pub cmds: DynCfgCmds,
    /// Access mask governing who may view the config.
    pub view_access: HttpAccess,
    /// Access mask governing who may edit the config.
    pub edit_access: HttpAccess,
}
