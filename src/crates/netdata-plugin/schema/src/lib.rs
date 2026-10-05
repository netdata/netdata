//! Turns a schemars-generated JSON Schema for a Rust type into Netdata's
//! config-form schema payload: [`NetdataSchema::netdata_schema`] generates a
//! draft-07 schema for the type and reworks it into the
//! `{jsonSchema, uiSchema, configDeclaration}` object — the shape the go.d
//! side ships as `config_schema.json` (Go readers take exactly the
//! `jsonSchema` and `uiSchema` members:
//! src/go/plugin/framework/vnodes/schema.go and
//! src/go/plugin/scripts.d/collector/native/internal/configform/form.go;
//! `configDeclaration` has no Go reader — it is this crate's dyncfg add-on).
//! The trait is blanket-implemented for every `JsonSchema` type, so no derive
//! is needed; examples/simple_usage.rs is the working end-to-end example.
//!
//! `netdata_schema()` runs two private schemars `Transform`s in sequence over
//! the generated root schema: `CollectUISchema` over the whole tree, then
//! `CollectConfigDeclaration` over the root object only (no recursion).
//! Extensions are stripped from the returned `jsonSchema` as they are
//! collected:
//!
//! - `x-ui-*` (any JSON value) → the matching `ui:*` key in `uiSchema`,
//!   stored under the dotted property/definition path of the node carrying
//!   the extension; extensions on the root merge their keys directly into
//!   `uiSchema`. A non-root path holds one object that is replaced — not
//!   merged — by a later collection at the same path, so extensions found
//!   both on a node and inside its subschema positions (`items`, `oneOf`,
//!   …) do not accumulate; positions walked only through
//!   `transform_subschemas` have no path pushed, so their extensions land on
//!   the enclosing node's path.
//! - `x-sensitive`, exactly `Bool(true)` → `ui:widget: "password"` at the
//!   same node; any other value stays in the schema.
//! - `x-config-*` (root object only) → one dyncfg `ConfigDeclaration`,
//!   serialized into `configDeclaration`; see netdata-plugin-types (the
//!   `config` module), whose `TryFrom<&serde_json::Value> for
//!   ConfigDeclaration` is the inverse of the serializer here.
//!
//! Boolean schema values convert in the walks but have no object, so they
//! are no-ops; other non-object values fail `try_into` and are skipped.
//!
//! Failures are panics, not `Result`s: an unknown `x-config-*` attribute
//! name panics in `CollectConfigDeclaration`, and
//! `ConfigDeclarationBuilder::build` unwraps every declaration field, so an
//! absent attribute, an unsupported value type, or a `type`/`status`/
//! `source-type` word outside its vocabulary panics there too — a
//! declaration is only ever produced complete. Only `x-config-cmds` is
//! lenient: an unparsable command list becomes the empty `DynCfgCmds` set.
//!
//! Direct consumers (grep-verified): otel-legacy-logs takes the `HttpAccess`
//! re-export (src/crates/otel-legacy-logs/src/handler.rs); bridge and rt
//! declare the workspace dependency but reference nothing from this crate in
//! their current sources; nothing outside the crate calls `netdata_schema()`
//! except examples/simple_usage.rs.

use schemars::transform::{Transform, transform_subschemas};
use schemars::{JsonSchema, Schema, SchemaGenerator, generate::SchemaSettings};
use serde_json::{Map, Value};

// Re-exports of the netdata-plugin-types vocabulary consumed through this
// crate (otel-legacy-logs reads HttpAccess from here).
pub use netdata_plugin_types::{
    ConfigDeclaration, DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess,
};

/// Collects `x-ui-*` extensions (plus `x-sensitive: true`) into a `uiSchema`
/// map keyed by dotted property path, stripping them from the schema as it
/// walks the whole tree.
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

        // x-ui-* keys become ui:<suffix>; x-sensitive counts only when exactly Bool(true).
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

        // Consumed keys are stripped; a non-true x-sensitive stays in the schema.
        for key in keys_to_remove {
            obj.remove(&key);
        }

        // Store collected keys under the node's dotted path; the root merges into uiSchema itself.
        if !ui_props.is_empty() {
            let ui_path = if self.current_path.is_empty() {
                ".".to_string()
            } else {
                self.current_path.join(".")
            };

            if ui_path == "." {
                // Root: merge the keys directly into uiSchema rather than nesting under ".".
                for (key, value) in ui_props {
                    self.ui_schema.insert(key, value);
                }
            } else {
                self.ui_schema.insert(ui_path, Value::Object(ui_props));
            }
        }

        // Property names extend the dotted uiSchema path.
        if let Some(properties) = obj.get_mut("properties").and_then(|v| v.as_object_mut()) {
            for (prop_name, prop_schema) in properties.iter_mut() {
                if let Ok(schema_ref) = prop_schema.try_into() {
                    self.current_path.push(prop_name.clone());
                    self.transform(schema_ref);
                    self.current_path.pop();
                }
            }
        }

        // Same walk for named definitions.
        if let Some(definitions) = obj.get_mut("definitions").and_then(|v| v.as_object_mut()) {
            for (def_name, def_schema) in definitions.iter_mut() {
                if let Ok(schema_ref) = def_schema.try_into() {
                    self.current_path.push(def_name.clone());
                    self.transform(schema_ref);
                    self.current_path.pop();
                }
            }
        }

        // Remaining positions (items, oneOf, additionalProperties, ...) get no path push:
        // extensions found there land on the enclosing node's path.
        transform_subschemas(self, schema);
    }
}

/// Reads the root schema's `x-config-*` attributes into one `ConfigDeclaration`
/// (netdata-plugin-types) and strips them from the schema. Runs once over the
/// root — no recursion — so `x-config-*` left on nested subschemas is neither
/// read nor removed. The attribute vocabulary and the serialized
/// `configDeclaration` member are the netdata-plugin-types `config` module's
/// contract; its `TryFrom<&serde_json::Value> for ConfigDeclaration` is the
/// inverse of the serializer in `netdata_schema`.
#[derive(Default)]
struct CollectConfigDeclaration {
    config_declaration: Option<ConfigDeclaration>,
}

impl Transform for CollectConfigDeclaration {
    fn transform(&mut self, schema: &mut Schema) {
        let Some(obj) = schema.as_object_mut() else {
            return;
        };

        // Only the keys of the schema object passed in are read; nothing recurses.
        let mut config_props = ConfigDeclarationBuilder::default();
        let mut keys_to_remove = Vec::new();

        for (key, value) in obj.iter() {
            if let Some(config_key) = key.strip_prefix("x-config-") {
                // Known names with the wrong value type reach the `unknown`
                // arm of the other type's match and panic; values of neither
                // type are dropped here and surface later as missing fields
                // in `build`.
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

        for key in keys_to_remove {
            obj.remove(&key);
        }

        // Unconditional: built even when no x-config-* attribute was present.
        self.config_declaration = Some(config_props.build());
    }
}

/// x-config-* attribute accumulator; all nine fields are required, and
/// `build` unwraps each one.
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

/// Parses an `x-config-cmds` value with `DynCfgCmds::from_str_multi`; a list
/// containing an unknown command name falls back to the empty set rather
/// than failing.
fn parse_cmds_string(cmds_str: &str) -> DynCfgCmds {
    DynCfgCmds::from_str_multi(cmds_str).unwrap_or_else(DynCfgCmds::empty)
}

/// Private knobs for `netdata_schema`; only `Default` is ever constructed,
/// so `fullPage` is always emitted and draft-07 always used.
#[derive(Debug, Clone)]
struct NetdataSchemaConfig {
    /// Adds `uiSchema.uiOptions.fullPage: true` when set; ibm.d docgen emits
    /// the same key (src/go/plugin/ibm.d/docgen/main.go), while the go.d
    /// renderer treats top-level `uiOptions` as dead
    /// (src/go/plugin/go.d/collector/config_schema_test.go).
    full_page: bool,
    /// Draft-07 settings; the returned `jsonSchema` carries the matching
    /// `$schema` keyword.
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

/// One-call artifact generation for a `JsonSchema` type: [`Self::netdata_schema`]
/// returns the `{jsonSchema, uiSchema, configDeclaration}` payload described
/// in the module docs. Blanket-implemented below, so no derive is needed.
pub trait NetdataSchema: JsonSchema {
    /// Generates a draft-07 schema for `Self`, strips the consumed extensions
    /// out of the returned `jsonSchema`, and assembles the payload. Panics on
    /// an unknown or incomplete `x-config-*` set.
    fn netdata_schema() -> serde_json::Value
    where
        Self: Sized,
    {
        let config = NetdataSchemaConfig::default();
        let generator = SchemaGenerator::new(config.schema_settings.clone());
        let mut json_schema = generator.into_root_schema_for::<Self>();

        let mut ui_collector = CollectUISchema::default();
        ui_collector.transform(&mut json_schema);

        let mut config_collector = CollectConfigDeclaration::default();
        config_collector.transform(&mut json_schema);

        let mut ui_schema = ui_collector.ui_schema;

        if config.full_page {
            ui_schema.insert(
                "uiOptions".to_string(),
                serde_json::json!({
                    "fullPage": true
                }),
            );
        }

        let mut result = serde_json::json!({
            "jsonSchema": json_schema,
            "uiSchema": ui_schema
        });

        // Always present after a successful transform (build() panics otherwise); member
        // keys per netdata-plugin-types' `config` module (the TryFrom inverse).
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

/// Blanket impl: every `JsonSchema` type gets [`NetdataSchema`], so
/// `T::netdata_schema()` needs no derive.
impl<T> NetdataSchema for T where T: JsonSchema {}
