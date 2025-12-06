//! # Netdata Schema Generation Library
//!
//! This library provides functionality to generate Netdata-compatible JSON schemas
//! with UI annotations and configuration declarations from Rust types annotated with schemars attributes.
//!
//! ## Basic Usage
//!
//! ```rust
//! use schemars::JsonSchema;
//! use netdata_plugin_schema::NetdataSchema;
//!
//! #[derive(Clone, Debug, JsonSchema)]
//! #[schemars(
//!     extend("x-ui-flavour" = "tabs"),
//!     extend("x-config-id" = "my_plugin:my_config"),
//!     extend("x-config-path" = "/collectors")
//! )]
//! struct MyConfig {
//!     #[schemars(
//!         title = "Server URL",
//!         extend("x-ui-help" = "Enter the server URL"),
//!         extend("x-ui-placeholder" = "https://example.com")
//!     )]
//!     url: String,
//! }
//!
//! // NetdataSchema is automatically implemented for all JsonSchema types
//!
//! let netdata_schema = MyConfig::netdata_schema();
//! println!("{}", serde_json::to_string_pretty(&netdata_schema).unwrap());
//!
//! // Config declaration is included in the schema if x-config-* metadata is present
//! if let Some(config_decl) = netdata_schema.get("configDeclaration") {
//!     println!("Config ID: {}", config_decl["id"]);
//! }
//! ```

use schemars::transform::{Transform, transform_subschemas};
use schemars::{JsonSchema, Schema, SchemaGenerator, generate::SchemaSettings};
use serde_json::{Map, Value};

// Re-export types for convenience
pub use netdata_plugin_types::{
    ConfigDeclaration, DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess,
};

/// Transform that collects UI schema information from x-ui-* extensions
/// and removes them from the JSON schema, collecting them separately
#[derive(Default)]
struct CollectUISchema {
    ui_schema: Map<String, Value>,
    current_path: Vec<String>,
}

impl Transform for CollectUISchema {
    fn transform(&mut self, schema: &mut Schema) {
        let Some(obj) = schema.as_object_mut() else {
            return;
        };

        // Collect UI extensions from current schema
        let mut ui_props = Map::new();
        let mut keys_to_remove = Vec::new();

        for (key, value) in obj.iter() {
            if let Some(ui_key) = key.strip_prefix("x-ui-") {
                ui_props.insert(format!("ui:{}", ui_key), value.clone());
                keys_to_remove.push(key.clone());
            } else if key == "x-sensitive" && value == &Value::Bool(true) {
                ui_props.insert(
                    "ui:widget".to_string(),
                    Value::String("password".to_string()),
                );
                keys_to_remove.push(key.clone());
            }
        }

        // Remove the x-ui-* extensions from the JSON schema
        for key in keys_to_remove {
            obj.remove(&key);
        }

        // If we have UI properties, add them to the UI schema at the current path
        if !ui_props.is_empty() {
            let ui_path = if self.current_path.is_empty() {
                ".".to_string()
            } else {
                self.current_path.join(".")
            };

            if ui_path == "." {
                // Root level - merge into root UI schema
                for (key, value) in ui_props {
                    self.ui_schema.insert(key, value);
                }
            } else {
                self.ui_schema.insert(ui_path, Value::Object(ui_props));
            }
        }

        // Handle properties recursively
        if let Some(properties) = obj.get_mut("properties").and_then(|v| v.as_object_mut()) {
            for (prop_name, prop_schema) in properties.iter_mut() {
                if let Ok(schema_ref) = prop_schema.try_into() {
                    self.current_path.push(prop_name.clone());
                    self.transform(schema_ref);
                    self.current_path.pop();
                }
            }
        }

        // Handle definitions recursively
        if let Some(definitions) = obj.get_mut("definitions").and_then(|v| v.as_object_mut()) {
            for (def_name, def_schema) in definitions.iter_mut() {
                if let Ok(schema_ref) = def_schema.try_into() {
                    self.current_path.push(def_name.clone());
                    self.transform(schema_ref);
                    self.current_path.pop();
                }
            }
        }

        // Handle other subschemas
        transform_subschemas(self, schema);
    }
}

/// Transform that collects config declaration information from x-config-* extensions
#[derive(Default)]
struct CollectConfigDeclaration {
    config_declaration: Option<ConfigDeclaration>,
}

impl Transform for CollectConfigDeclaration {
    fn transform(&mut self, schema: &mut Schema) {
        let Some(obj) = schema.as_object_mut() else {
            return;
        };

        // Only process root-level schema (where config declarations should be)
        let mut config_props = ConfigDeclarationBuilder::default();
        let mut keys_to_remove = Vec::new();

        for (key, value) in obj.iter() {
            if let Some(config_key) = key.strip_prefix("x-config-") {
                if let Some(str_value) = value.as_str() {
                    match config_key {
                        "id" => config_props.id = Some(str_value.to_string()),
                        "path" => config_props.path = Some(str_value.to_string()),
                        "source" => config_props.source = Some(str_value.to_string()),
                        "type" => config_props.type_ = DynCfgType::from_name(str_value),
                        "status" => config_props.status = DynCfgStatus::from_name(str_value),
                        "source-type" => {
                            config_props.source_type = DynCfgSourceType::from_name(str_value)
                        }
                        "cmds" => config_props.cmds = Some(parse_cmds_string(str_value)),
                        unknown => {
                            panic!("Unknown config declaration attribute: {}", unknown);
                        }
                    }
                } else if let Some(int_value) = value.as_u64() {
                    match config_key {
                        "view-access" => {
                            config_props.view_access = Some(HttpAccess::from_u32(int_value as u32))
                        }
                        "edit-access" => {
                            config_props.edit_access = Some(HttpAccess::from_u32(int_value as u32))
                        }
                        unknown => {
                            panic!("Unknown config declaration attribute: {}", unknown);
                        }
                    }
                }
                keys_to_remove.push(key.clone());
            }
        }

        // Remove the x-config-* extensions from the JSON schema
        for key in keys_to_remove {
            obj.remove(&key);
        }

        // Build config declaration
        self.config_declaration = Some(config_props.build());
    }
}

#[derive(Debug, Default)]
struct ConfigDeclarationBuilder {
    id: Option<String>,
    status: Option<DynCfgStatus>,
    type_: Option<DynCfgType>,
    path: Option<String>,
    source_type: Option<DynCfgSourceType>,
    source: Option<String>,
    cmds: Option<DynCfgCmds>,
    view_access: Option<HttpAccess>,
    edit_access: Option<HttpAccess>,
}

impl ConfigDeclarationBuilder {
    fn build(self) -> ConfigDeclaration {
        ConfigDeclaration {
            id: self.id.unwrap(),
            status: self.status.unwrap(),
            type_: self.type_.unwrap(),
            path: self.path.unwrap(),
            source_type: self.source_type.unwrap(),
            source: self.source.unwrap(),
            cmds: self.cmds.unwrap(),
            view_access: self.view_access.unwrap(),
            edit_access: self.edit_access.unwrap(),
        }
    }
}

/// Parse command string like "schema|get|update" into DynCfgCmds flags
fn parse_cmds_string(cmds_str: &str) -> DynCfgCmds {
    // Use the existing parsing functionality from DynCfgCmds
    DynCfgCmds::from_str_multi(cmds_str).unwrap_or_else(DynCfgCmds::empty)
}

/// Configuration for Netdata schema generation
#[derive(Debug, Clone)]
struct NetdataSchemaConfig {
    /// Whether to include the full page UI option
    full_page: bool,
    /// JSON Schema settings to use
    schema_settings: SchemaSettings,
}

impl Default for NetdataSchemaConfig {
    fn default() -> Self {
        Self {
            full_page: true,
            schema_settings: SchemaSettings::draft07(),
        }
    }
}

/// Trait for types that can generate Netdata-compatible schemas with UI and config declarations
pub trait NetdataSchema: JsonSchema {
    /// Generate a comprehensive Netdata-compatible schema with jsonSchema, uiSchema, and configDeclaration
    fn netdata_schema() -> serde_json::Value
    where
        Self: Sized,
    {
        let config = NetdataSchemaConfig::default();
        let generator = SchemaGenerator::new(config.schema_settings.clone());
        let mut json_schema = generator.into_root_schema_for::<Self>();

        // Apply our UI schema collector transform
        let mut ui_collector = CollectUISchema::default();
        ui_collector.transform(&mut json_schema);

        // Apply our config declaration collector
        let mut config_collector = CollectConfigDeclaration::default();
        config_collector.transform(&mut json_schema);

        // Create the UI schema from collected information
        let mut ui_schema = ui_collector.ui_schema;

        // Add default UI options
        if config.full_page {
            ui_schema.insert(
                "uiOptions".to_string(),
                serde_json::json!({
                    "fullPage": true
                }),
            );
        }

        // Build the result object
        let mut result = serde_json::json!({
            "jsonSchema": json_schema,
            "uiSchema": ui_schema
        });

        // Add config declaration if present
        if let Some(config_decl) = config_collector.config_declaration {
            result["configDeclaration"] = serde_json::json!({
                "id": config_decl.id,
                "status": config_decl.status.name(),
                "type": config_decl.type_.name(),
                "path": config_decl.path,
                "sourceType": config_decl.source_type.name(),
                "source": config_decl.source,
                "cmds": config_decl.cmds.to_pipe_separated(),
                "viewAccess": u32::from(config_decl.view_access),
                "editAccess": u32::from(config_decl.edit_access)
            });
        }

        result
    }
}

/// Blanket implementation for all JsonSchema types
impl<T> NetdataSchema for T where T: JsonSchema {}
