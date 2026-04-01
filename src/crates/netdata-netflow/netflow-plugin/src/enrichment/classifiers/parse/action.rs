use super::value::{one_string_arg, three_string_args};
use super::*;

pub(super) fn parse_action(name: &str, args: &[String]) -> Result<ActionExpr> {
    match name {
        "Reject" => {
            if !args.is_empty() {
                anyhow::bail!("Reject() does not accept arguments");
            }
            Ok(ActionExpr::Reject)
        }
        "Classify" | "ClassifyGroup" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Group,
            one_string_arg(name, args)?,
        )),
        "ClassifyRole" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Role,
            one_string_arg(name, args)?,
        )),
        "ClassifySite" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Site,
            one_string_arg(name, args)?,
        )),
        "ClassifyRegion" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Region,
            one_string_arg(name, args)?,
        )),
        "ClassifyTenant" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Tenant,
            one_string_arg(name, args)?,
        )),
        "ClassifyRegex" | "ClassifyGroupRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Group,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyRoleRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Role,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifySiteRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Site,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyRegionRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Region,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyTenantRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Tenant,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyProvider" => Ok(ActionExpr::ClassifyInterface(
            InterfaceTarget::Provider,
            one_string_arg(name, args)?,
        )),
        "ClassifyConnectivity" => Ok(ActionExpr::ClassifyInterface(
            InterfaceTarget::Connectivity,
            one_string_arg(name, args)?,
        )),
        "ClassifyProviderRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyInterfaceRegex(
                InterfaceTarget::Provider,
                arg1,
                arg2,
                arg3,
            ))
        }
        "ClassifyConnectivityRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            Ok(ActionExpr::ClassifyInterfaceRegex(
                InterfaceTarget::Connectivity,
                arg1,
                arg2,
                arg3,
            ))
        }
        "SetName" => Ok(ActionExpr::SetName(one_string_arg(name, args)?)),
        "SetDescription" => Ok(ActionExpr::SetDescription(one_string_arg(name, args)?)),
        "ClassifyExternal" => {
            if !args.is_empty() {
                anyhow::bail!("ClassifyExternal() does not accept arguments");
            }
            Ok(ActionExpr::ClassifyExternal)
        }
        "ClassifyInternal" => {
            if !args.is_empty() {
                anyhow::bail!("ClassifyInternal() does not accept arguments");
            }
            Ok(ActionExpr::ClassifyInternal)
        }
        _ => anyhow::bail!("unsupported classifier action '{name}'"),
    }
}
