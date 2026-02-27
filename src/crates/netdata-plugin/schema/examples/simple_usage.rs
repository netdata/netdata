use netdata_plugin_schema::NetdataSchema;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

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
