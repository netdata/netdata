use super::*;
use regex::Regex;

#[derive(Debug, Clone)]
pub(crate) enum ActionExpr {
    Reject,
    ClassifyExporter(ExporterTarget, ValueExpr),
    ClassifyExporterRegex(
        ExporterTarget,
        ValueExpr,
        ValueExpr,
        ValueExpr,
        Option<Regex>,
    ),
    ClassifyInterface(InterfaceTarget, ValueExpr),
    ClassifyInterfaceRegex(
        InterfaceTarget,
        ValueExpr,
        ValueExpr,
        ValueExpr,
        Option<Regex>,
    ),
    SetName(ValueExpr),
    SetDescription(ValueExpr),
    ClassifyExternal,
    ClassifyInternal,
}

impl ActionExpr {
    pub(crate) fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            ActionExpr::Reject => {
                classification.reject = true;
                Ok(false)
            }
            ActionExpr::ClassifyExporter(target, value) => {
                let value = value.resolve(Some(exporter), None, Some(classification), None)?;
                let slot = classification.exporter_target_mut(target);
                if slot.is_empty() {
                    *slot = normalize_classifier_value(&value.to_string_value());
                }
                Ok(true)
            }
            ActionExpr::ClassifyExporterRegex(target, input, pattern, template, compiled) => {
                let input = input
                    .resolve(Some(exporter), None, Some(classification), None)?
                    .to_string_value();
                let pattern = pattern
                    .resolve(Some(exporter), None, Some(classification), None)?
                    .to_string_value();
                let template = template
                    .resolve(Some(exporter), None, Some(classification), None)?
                    .to_string_value();

                let slot = classification.exporter_target_mut(target);
                if slot.is_empty() {
                    if let Some(mapped) =
                        apply_regex_template(&input, &pattern, &template, compiled.as_ref())?
                    {
                        *slot = normalize_classifier_value(&mapped);
                        return Ok(true);
                    }
                    return Ok(false);
                }
                Ok(true)
            }
            ActionExpr::ClassifyInterface(_, _)
            | ActionExpr::ClassifyInterfaceRegex(_, _, _, _, _)
            | ActionExpr::SetName(_)
            | ActionExpr::SetDescription(_)
            | ActionExpr::ClassifyExternal
            | ActionExpr::ClassifyInternal => anyhow::bail!("interface action in exporter rule"),
        }
    }

    pub(crate) fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            ActionExpr::Reject => {
                classification.reject = true;
                Ok(false)
            }
            ActionExpr::ClassifyInterface(target, value) => {
                let value =
                    value.resolve(Some(exporter), Some(interface), None, Some(classification))?;
                let slot = classification.interface_target_mut(target);
                if slot.is_empty() {
                    *slot = normalize_classifier_value(&value.to_string_value());
                }
                Ok(true)
            }
            ActionExpr::ClassifyInterfaceRegex(target, input, pattern, template, compiled) => {
                let input = input
                    .resolve(Some(exporter), Some(interface), None, Some(classification))?
                    .to_string_value();
                let pattern = pattern
                    .resolve(Some(exporter), Some(interface), None, Some(classification))?
                    .to_string_value();
                let template = template
                    .resolve(Some(exporter), Some(interface), None, Some(classification))?
                    .to_string_value();

                let slot = classification.interface_target_mut(target);
                if slot.is_empty() {
                    if let Some(mapped) =
                        apply_regex_template(&input, &pattern, &template, compiled.as_ref())?
                    {
                        *slot = normalize_classifier_value(&mapped);
                        return Ok(true);
                    }
                    return Ok(false);
                }
                Ok(true)
            }
            ActionExpr::SetName(value) => {
                if classification.name.is_empty() {
                    classification.name = value
                        .resolve(Some(exporter), Some(interface), None, Some(classification))?
                        .to_string_value();
                }
                Ok(true)
            }
            ActionExpr::SetDescription(value) => {
                if classification.description.is_empty() {
                    classification.description = value
                        .resolve(Some(exporter), Some(interface), None, Some(classification))?
                        .to_string_value();
                }
                Ok(true)
            }
            ActionExpr::ClassifyExternal => {
                if classification.boundary == 0 {
                    classification.boundary = 1;
                }
                Ok(true)
            }
            ActionExpr::ClassifyInternal => {
                if classification.boundary == 0 {
                    classification.boundary = 2;
                }
                Ok(true)
            }
            ActionExpr::ClassifyExporter(_, _)
            | ActionExpr::ClassifyExporterRegex(_, _, _, _, _) => {
                anyhow::bail!("exporter action in interface rule")
            }
        }
    }
}
