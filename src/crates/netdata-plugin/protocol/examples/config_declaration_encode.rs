use netdata_plugin_protocol::{
    ConfigDeclaration, DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess,
};
use netdata_plugin_protocol::{Message, MessageWriter};

#[tokio::main]
async fn main() {
    // Create a sample ConfigDeclaration
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

    // Create message from the config declaration
    let message = Message::ConfigDeclaration(Box::new(config_declaration));

    // Create message writer for stdout
    let stdout = tokio::io::stdout();
    let mut writer = MessageWriter::new(stdout);

    // Send the message to stdout
    match writer.send(message).await {
        Ok(()) => {
            // Message sent successfully
        }
        Err(e) => {
            eprintln!("Error sending message: {}", e);
        }
    }
}
