use super::value::{one_string_arg, three_string_args};
use super::*;

fn compile_literal_regex(value: &ValueExpr, context: &str) -> Result<Option<regex::Regex>> {
    match value {
        ValueExpr::StringLiteral(pattern) => regex::Regex::new(pattern).map(Some).map_err(|err| {
            anyhow::anyhow!("invalid regex '{pattern}' in {context} expression: {err}")
        }),
        _ => Ok(None),
    }
}

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
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Group,
                arg1,
                arg2,
                arg3,
                compiled,
            ))
        }
        "ClassifyRoleRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Role,
                arg1,
                arg2,
                arg3,
                compiled,
            ))
        }
        "ClassifySiteRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Site,
                arg1,
                arg2,
                arg3,
                compiled,
            ))
        }
        "ClassifyRegionRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Region,
                arg1,
                arg2,
                arg3,
                compiled,
            ))
        }
        "ClassifyTenantRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyExporterRegex(
                ExporterTarget::Tenant,
                arg1,
                arg2,
                arg3,
                compiled,
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
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyInterfaceRegex(
                InterfaceTarget::Provider,
                arg1,
                arg2,
                arg3,
                compiled,
            ))
        }
        "ClassifyConnectivityRegex" => {
            let (arg1, arg2, arg3) = three_string_args(name, args)?;
            let compiled = compile_literal_regex(&arg2, name)?;
            Ok(ActionExpr::ClassifyInterfaceRegex(
                InterfaceTarget::Connectivity,
                arg1,
                arg2,
                arg3,
                compiled,
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
