use super::*;

#[derive(Debug, Clone)]
pub(crate) enum ValueExpr {
    StringLiteral(String),
    NumberLiteral(i64),
    Field(FieldExpr),
    List(Vec<ValueExpr>),
    Concat(Vec<ValueExpr>),
    Format {
        pattern: Box<ValueExpr>,
        args: Vec<ValueExpr>,
    },
}

impl ValueExpr {
    pub(crate) fn resolve(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<ResolvedValue> {
        match self {
            ValueExpr::StringLiteral(value) => Ok(ResolvedValue::String(value.clone())),
            ValueExpr::NumberLiteral(value) => Ok(ResolvedValue::Number(*value)),
            ValueExpr::Field(field) => field.resolve(
                exporter,
                interface,
                exporter_classification,
                interface_classification,
            ),
            ValueExpr::List(items) => {
                let mut resolved = Vec::with_capacity(items.len());
                for item in items {
                    resolved.push(item.resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?);
                }
                Ok(ResolvedValue::List(resolved))
            }
            ValueExpr::Concat(parts) => {
                let mut output = String::new();
                for part in parts {
                    let value = part.resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?;
                    output.push_str(&value.to_string_value());
                }
                Ok(ResolvedValue::String(output))
            }
            ValueExpr::Format { pattern, args } => {
                let pattern = pattern
                    .resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?
                    .to_string_value();
                let mut resolved_args = Vec::with_capacity(args.len());
                for arg in args {
                    resolved_args.push(arg.resolve(
                        exporter,
                        interface,
                        exporter_classification,
                        interface_classification,
                    )?);
                }
                Ok(ResolvedValue::String(format_with_percent_placeholders(
                    &pattern,
                    &resolved_args,
                )?))
            }
        }
    }

    pub(crate) fn is_string_expression(&self) -> bool {
        match self {
            ValueExpr::StringLiteral(_) => true,
            ValueExpr::NumberLiteral(_) => false,
            ValueExpr::Field(field) => field.is_string_field(),
            ValueExpr::List(_) => false,
            ValueExpr::Concat(parts) => parts.iter().all(ValueExpr::is_string_expression),
            ValueExpr::Format { .. } => true,
        }
    }
}
