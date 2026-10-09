//! Example for the netdata-plugin-schema crate: a struct annotated with
//! schemars attributes is turned into the three-part Netdata schema payload
//! (`jsonSchema`, `uiSchema`, `configDeclaration`) and printed as pretty
//! JSON. Generation goes through `NetdataSchema::netdata_schema()`
//! (blanket-implemented for every `JsonSchema` type); the struct itself is
//! never instantiated.
//!
//! Run from the Rust workspace root (`src/crates`):
//!
//! ```text
//! cargo run -p netdata-plugin-schema --example simple_usage
//! ```
//!
//! What the attributes become (the two collector transforms in the schema
//! crate's `lib.rs`):
//! - `x-ui-*` extensions are renamed `ui:*` and moved out of `jsonSchema`
//!   into `uiSchema` — struct-level ones merge into the root map,
//!   field-level ones sit under their schema path (`host`, `port`);
//!   `uiOptions.fullPage` is added by default.
//! - the nine `x-config-*` extensions are collected from the root schema
//!   only (then removed from `jsonSchema`) and emitted as
//!   `configDeclaration`, the JSON form of
//!   `netdata_plugin_types::ConfigDeclaration` (`types::config`) that
//!   `TryFrom<&serde_json::Value>` parses back and netdata-plugin-protocol
//!   re-encodes onto the pluginsd `CONFIG` line. Field-level `x-config-*`
//!   attributes are never read and pass through into `jsonSchema`.
//!
//! Gotchas:
//! - every `x-config-*` attribute is required and its value must have the
//!   expected JSON type: unknown keys panic in the collector; missing,
//!   unrecognized or wrong-typed values panic at the builder's
//!   `Option::unwrap` — except an unrecognized `cmds` word, which silently
//!   degrades to the empty set.
//! - the vocabulary words must match `DynCfgType`, `DynCfgStatus` and
//!   `DynCfgSourceType` exactly, lowercase and untrimmed (`single`,
//!   `running` and `stock` here); `schema|get|update` is parsed by
//!   `DynCfgCmds::from_str_multi` and rendered back in fixed flag order
//!   (`get | schema | update`), so the input order is not preserved.
//! - `view-access`/`edit-access` of `0` is the empty access mask, which
//!   the agent replaces with its role defaults at registration — not "no
//!   access" (`http_access`/`config` in netdata-plugin-types).
//! - the tab field lists in `x-ui-options` name properties that do not
//!   exist on the struct; nothing validates them, so the emitted
//!   `uiSchema` references absent `jsonSchema` properties.
use netdata_plugin_schema::NetdataSchema;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

// Only the `JsonSchema` derive feeds schema generation; the serde and
// `Clone`/`Debug` derives are unused in this example.
#[derive(Clone, Debug, JsonSchema, Serialize, Deserialize)]
#[schemars(
    title = "Web Server Configuration",
    description = "Configuration for a simple web server",
    extend("x-ui-flavour" = "tabs"),
    extend("x-ui-options" = {
        "tabs": [
            {
                "title": "Server Settings",
                "fields": ["host", "port", "workers"]
            },
            {
                "title": "Security",
                "fields": ["enable_tls", "tls_cert_path", "api_key"]
            }
        ]
    }),
    extend("x-config-id" = "demo_plugin:my_config"),
    extend("x-config-path" = "/collectors"),
    extend("x-config-type" = "single"),
    extend("x-config-status" = "running"),
    extend("x-config-source-type" = "stock"),
    extend("x-config-source" = "Plugin-generated configuration"),
    extend("x-config-cmds" = "schema|get|update"),
    extend("x-config-view-access" = 0),
    extend("x-config-edit-access" = 0),
)]
struct WebServerConfig {
    #[schemars(
        title = "Host Address",
        description = "The IP address to bind the server to",
        example = "0.0.0.0",
        extend("x-ui-help" = "Use 0.0.0.0 to bind to all interfaces"),
        extend("x-ui-placeholder" = "127.0.0.1")
    )]
    host: String,

    #[schemars(
        title = "Port",
        description = "TCP port number",
        range(min = 1, max = 65535),
        example = 8080,
        extend("x-ui-help" = "Choose an available port number"),
        extend("x-ui-placeholder" = "8080")
    )]
    port: u16,
}

fn main() {
    let schema = WebServerConfig::netdata_schema();
    println!("{}", serde_json::to_string_pretty(&schema).unwrap());
}
