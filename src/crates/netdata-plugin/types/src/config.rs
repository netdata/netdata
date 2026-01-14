use crate::{DynCfgCmds, DynCfgSourceType, DynCfgStatus, DynCfgType, HttpAccess};
use netdata_plugin_error::{NetdataPluginError, Result};
use std::convert::TryFrom;

#[derive(Debug, Clone)]
pub struct ConfigDeclaration {
    pub id: String,
    pub status: DynCfgStatus,
    pub type_: DynCfgType,
    pub path: String,
    pub source_type: DynCfgSourceType,
    pub source: String,
    pub cmds: DynCfgCmds,
    pub view_access: HttpAccess,
    pub edit_access: HttpAccess,
}

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
