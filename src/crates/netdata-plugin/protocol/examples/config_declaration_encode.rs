//! Example: encode a dyncfg [`ConfigDeclaration`] to stdout with
//! [`MessageWriter::send`] — the `CONFIG <id> CREATE ...` line a plugin
//! sends the agent to register a configuration entry (`pluginsd_config()`
//! in src/plugins.d/pluginsd_dyncfg.c).
//!
//! Run:
//! `cargo run -p netdata-plugin-protocol --example config_declaration_encode`
//!
//! It prints one pluginsd line to stdout:
//!
//! ```text
//! CONFIG go.d:nginx CREATE accepted template /collectors internal 'whatever internal source' 'schema | add | enable | disable' 0x0 0x0
//! ```
//!
//! This exercises only the plugin→agent encode side: the decoder never
//! produces [`Message::ConfigDeclaration`] — `CONFIG` is in the line
//! parser's keyword map but has no decode arm, so an inbound CONFIG line
//! would hit the unhandled-token panic in `line_parser` (unreachable via
//! the agent→plugin stream today) — and the message is a single line with
//! no payload block.
use netdata_plugin_protocol::{
    ConfigDeclaration, DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess,
};
use netdata_plugin_protocol::{Message, MessageWriter};

#[tokio::main]
async fn main() {
    let config_declaration = ConfigDeclaration {
        id: "go.d:nginx".to_string(),
        status: DynCfgStatus::Accepted,
        type_: DynCfgType::Template,
        path: "/collectors".to_string(),
        source_type: DynCfgSourceType::Internal,
        source: "whatever internal source".to_string(),
        cmds: DynCfgCmds::SCHEMA | DynCfgCmds::ADD | DynCfgCmds::ENABLE | DynCfgCmds::DISABLE,
        view_access: HttpAccess::empty(),
        edit_access: HttpAccess::empty(),
    };

    let message = Message::ConfigDeclaration(Box::new(config_declaration));

    let stdout = tokio::io::stdout();
    let mut writer = MessageWriter::new(stdout);

    match writer.send(message).await {
        Ok(()) => {}
        Err(e) => {
            eprintln!("Error sending message: {}", e);
        }
    }
}
