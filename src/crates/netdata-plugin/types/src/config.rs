//! The dyncfg configuration-declaration payload: [`ConfigDeclaration`], the
//! struct a plugin fills in to register one dynamic configuration entry with
//! the agent over the pluginsd `CONFIG` line. Field vocabularies live in the
//! sibling `dyncfg_*` and `http_access` modules.
//!
//! The declaration travels in two serialized forms: the `configDeclaration`
//! member of a netdata schema (produced by `NetdataSchema::netdata_schema()`
//! in netdata-plugin-schema, parsed back here by `TryFrom<&serde_json::Value>`)
//! and the pluginsd `CONFIG` line (encoded by `Message::ConfigDeclaration` in
//! netdata-plugin-protocol, received by `pluginsd_config()` in
//! src/plugins.d/pluginsd_dyncfg.c). The wire form is plugin→agent only: the
//! protocol parser has no handler arm for the CONFIG keyword, so an inbound
//! line panics in `line_parser` instead of decoding.
use crate::{DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess};
use netdata_plugin_error::{NetdataPluginError, Result};
use std::convert::TryFrom;

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
    /// Serialized under the JSON key `type`; the underscore avoids the Rust
    /// keyword.
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

/// Parse a declaration from a netdata schema object (the whole
/// `{jsonSchema, uiSchema, configDeclaration}` value): reads its
/// `configDeclaration` member, the shape produced by
/// `NetdataSchema::netdata_schema()` in netdata-plugin-schema with the keys
/// `id`, `status`, `type`, `path`, `sourceType`, `source`, `cmds`,
/// `viewAccess`, `editAccess` — this impl is the inverse of that serializer.
///
/// Every key is required. A missing key, a value of the wrong JSON type, or a
/// `status`/`type`/`sourceType`/`cmds` string outside its vocabulary fails with
/// `NetdataPluginError::Schema` naming the key — unknown vocabulary names are
/// rejected here, whereas the agent's C parsers default them. Access values
/// must be unsigned integers; they are cast to u32 and masked to the known
/// `HttpAccess` bits, so oversized values are truncated rather than rejected.
impl TryFrom<&serde_json::Value> for ConfigDeclaration {
    type Error = NetdataPluginError;

    fn try_from(schema: &serde_json::Value) -> Result<Self> {
        let config_decl = schema
            .get("configDeclaration")
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing configDeclaration key"),
            })?;

        let id = config_decl
            .get("id")
            .and_then(|v| v.as_str())
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing id key in configDeclaration"),
            })?
            .to_string();

        let status = config_decl
            .get("status")
            .and_then(|v| v.as_str())
            .and_then(DynCfgStatus::from_name)
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing status key in configDeclaration"),
            })?;

        let type_ = config_decl
            .get("type")
            .and_then(|v| v.as_str())
            .and_then(DynCfgType::from_name)
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing type key in configDeclaration"),
            })?;

        let path = config_decl
            .get("path")
            .and_then(|v| v.as_str())
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing path key in configDeclaration"),
            })?
            .to_string();

        let source_type = config_decl
            .get("sourceType")
            .and_then(|v| v.as_str())
            .and_then(DynCfgSourceType::from_name)
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing sourceType key in configDeclaration"),
            })?;

        let source = config_decl
            .get("source")
            .and_then(|v| v.as_str())
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing source key in configDeclaration"),
            })?
            .to_string();

        let cmds = config_decl
            .get("cmds")
            .and_then(|v| v.as_str())
            .and_then(DynCfgCmds::from_str_multi)
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing cmds key in configDeclaration"),
            })?;

        let view_access = config_decl
            .get("viewAccess")
            .and_then(|v| v.as_u64())
            .map(|v| HttpAccess::from_u32(v as u32))
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing viewAccess key in configDeclaration"),
            })?;

        let edit_access = config_decl
            .get("editAccess")
            .and_then(|v| v.as_u64())
            .map(|v| HttpAccess::from_u32(v as u32))
            .ok_or(NetdataPluginError::Schema {
                message: String::from("Missing editAccess key in configDeclaration"),
            })?;

        Ok(ConfigDeclaration {
            id,
            status,
            type_,
            path,
            source_type,
            source,
            cmds,
            view_access,
            edit_access,
        })
    }
}
