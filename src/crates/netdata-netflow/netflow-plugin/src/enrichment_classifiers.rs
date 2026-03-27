#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
struct ExporterInfo {
    ip: String,
    name: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
struct InterfaceInfo {
    index: u32,
    name: String,
    description: String,
    speed: u64,
    vlan: u16,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct ExporterAndInterfaceInfo {
    exporter: ExporterInfo,
    interface: InterfaceInfo,
}

#[derive(Debug, Clone, Default)]
struct ExporterClassification {
    group: String,
    role: String,
    site: String,
    region: String,
    tenant: String,
    reject: bool,
}

impl ExporterClassification {
    fn is_empty(&self) -> bool {
        self.group.is_empty()
            && self.role.is_empty()
            && self.site.is_empty()
            && self.region.is_empty()
            && self.tenant.is_empty()
            && !self.reject
    }

    fn is_complete(&self) -> bool {
        !self.group.is_empty()
            && !self.role.is_empty()
            && !self.site.is_empty()
            && !self.region.is_empty()
            && !self.tenant.is_empty()
    }
}

#[derive(Debug, Clone, Default)]
struct InterfaceClassification {
    connectivity: String,
    provider: String,
    boundary: u8,
    reject: bool,
    name: String,
    description: String,
}

impl InterfaceClassification {
    fn is_empty(&self) -> bool {
        self.connectivity.is_empty()
            && self.provider.is_empty()
            && self.boundary == 0
            && !self.reject
            && self.name.is_empty()
            && self.description.is_empty()
    }
}

#[derive(Debug, Clone)]
struct ClassifierRule {
    expression: BoolExpr,
}

impl ClassifierRule {
    fn parse(rule: &str) -> Result<Self> {
        let expression = parse_boolean_expr(rule)?;
        Ok(Self { expression })
    }

    fn evaluate_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        self.expression.eval_exporter(exporter, classification)
    }

    fn evaluate_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        self.expression
            .eval_interface(exporter, interface, classification)
    }
}

#[derive(Debug, Clone)]
enum BoolExpr {
    Term(RuleTerm),
    And(Box<BoolExpr>, Box<BoolExpr>),
    Or(Box<BoolExpr>, Box<BoolExpr>),
    Not(Box<BoolExpr>),
}

impl BoolExpr {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            BoolExpr::Term(term) => term.eval_exporter(exporter, classification),
            BoolExpr::And(left, right) => {
                if !left.eval_exporter(exporter, classification)? {
                    return Ok(false);
                }
                right.eval_exporter(exporter, classification)
            }
            BoolExpr::Or(left, right) => {
                if left.eval_exporter(exporter, classification)? {
                    return Ok(true);
                }
                right.eval_exporter(exporter, classification)
            }
            BoolExpr::Not(inner) => Ok(!inner.eval_exporter(exporter, classification)?),
        }
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            BoolExpr::Term(term) => term.eval_interface(exporter, interface, classification),
            BoolExpr::And(left, right) => {
                if !left.eval_interface(exporter, interface, classification)? {
                    return Ok(false);
                }
                right.eval_interface(exporter, interface, classification)
            }
            BoolExpr::Or(left, right) => {
                if left.eval_interface(exporter, interface, classification)? {
                    return Ok(true);
                }
                right.eval_interface(exporter, interface, classification)
            }
            BoolExpr::Not(inner) => {
                Ok(!inner.eval_interface(exporter, interface, classification)?)
            }
        }
    }
}

#[derive(Debug, Clone)]
enum RuleTerm {
    Condition(ConditionExpr),
    Action(ActionExpr),
}

impl RuleTerm {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        match self {
            RuleTerm::Condition(condition) => condition.eval_exporter(exporter, classification),
            RuleTerm::Action(action) => action.eval_exporter(exporter, classification),
        }
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        match self {
            RuleTerm::Condition(condition) => {
                condition.eval_interface(exporter, interface, classification)
            }
            RuleTerm::Action(action) => action.eval_interface(exporter, interface, classification),
        }
    }
}

#[derive(Debug, Clone)]
enum ConditionExpr {
    Literal(bool),
    Equals(ValueExpr, ValueExpr),
    NotEquals(ValueExpr, ValueExpr),
    Greater(ValueExpr, ValueExpr),
    GreaterOrEqual(ValueExpr, ValueExpr),
    Less(ValueExpr, ValueExpr),
    LessOrEqual(ValueExpr, ValueExpr),
    In(ValueExpr, ValueExpr),
    Contains(ValueExpr, ValueExpr),
    StartsWith(ValueExpr, ValueExpr),
    EndsWith(ValueExpr, ValueExpr),
    Matches(ValueExpr, ValueExpr),
}

impl ConditionExpr {
    fn eval_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &ExporterClassification,
    ) -> Result<bool> {
        self.eval_with_context(Some(exporter), None, Some(classification), None)
    }

    fn eval_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &InterfaceClassification,
    ) -> Result<bool> {
        self.eval_with_context(Some(exporter), Some(interface), None, Some(classification))
    }

    fn eval_with_context(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<bool> {
        match self {
            ConditionExpr::Literal(value) => Ok(*value),
            ConditionExpr::Equals(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value() == right.to_string_value())
            }
            ConditionExpr::NotEquals(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value() != right.to_string_value())
            }
            ConditionExpr::Greater(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '>'"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '>'"))?;
                Ok(left_num > right_num)
            }
            ConditionExpr::GreaterOrEqual(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '>='"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '>='"))?;
                Ok(left_num >= right_num)
            }
            ConditionExpr::Less(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '<'"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '<'"))?;
                Ok(left_num < right_num)
            }
            ConditionExpr::LessOrEqual(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left_num = left
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("left operand is not numeric for '<='"))?;
                let right_num = right
                    .to_i64()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not numeric for '<='"))?;
                Ok(left_num <= right_num)
            }
            ConditionExpr::In(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let left = left.to_string_value();
                let members = right
                    .as_list()
                    .ok_or_else(|| anyhow::anyhow!("right operand is not a list for 'in'"))?;
                Ok(members
                    .iter()
                    .any(|candidate| candidate.to_string_value() == left))
            }
            ConditionExpr::Contains(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value().contains(&right.to_string_value()))
            }
            ConditionExpr::StartsWith(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value().starts_with(&right.to_string_value()))
            }
            ConditionExpr::EndsWith(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                Ok(left.to_string_value().ends_with(&right.to_string_value()))
            }
            ConditionExpr::Matches(left, right) => {
                let left = left.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let right = right.resolve(
                    exporter,
                    interface,
                    exporter_classification,
                    interface_classification,
                )?;
                let pattern = right.to_string_value();
                let regex = Regex::new(&pattern)
                    .with_context(|| format!("invalid regex '{pattern}' for 'matches'"))?;
                Ok(regex.is_match(&left.to_string_value()))
            }
        }
    }
}

#[derive(Debug, Clone)]
enum ActionExpr {
    Reject,
    ClassifyExporter(ExporterTarget, ValueExpr),
    ClassifyExporterRegex(ExporterTarget, ValueExpr, ValueExpr, ValueExpr),
    ClassifyInterface(InterfaceTarget, ValueExpr),
    ClassifyInterfaceRegex(InterfaceTarget, ValueExpr, ValueExpr, ValueExpr),
    SetName(ValueExpr),
    SetDescription(ValueExpr),
    ClassifyExternal,
    ClassifyInternal,
}

impl ActionExpr {
    fn eval_exporter(
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
            ActionExpr::ClassifyExporterRegex(target, input, pattern, template) => {
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
                    if let Some(mapped) = apply_regex_template(&input, &pattern, &template)? {
                        *slot = normalize_classifier_value(&mapped);
                        return Ok(true);
                    }
                    return Ok(false);
                }
                Ok(true)
            }
            ActionExpr::ClassifyInterface(_, _)
            | ActionExpr::ClassifyInterfaceRegex(_, _, _, _)
            | ActionExpr::SetName(_)
            | ActionExpr::SetDescription(_)
            | ActionExpr::ClassifyExternal
            | ActionExpr::ClassifyInternal => anyhow::bail!("interface action in exporter rule"),
        }
    }

    fn eval_interface(
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
            ActionExpr::ClassifyInterfaceRegex(target, input, pattern, template) => {
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
                    if let Some(mapped) = apply_regex_template(&input, &pattern, &template)? {
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
            ActionExpr::ClassifyExporter(_, _) | ActionExpr::ClassifyExporterRegex(_, _, _, _) => {
                anyhow::bail!("exporter action in interface rule")
            }
        }
    }
}

#[derive(Debug, Clone, Copy)]
enum ExporterTarget {
    Group,
    Role,
    Site,
    Region,
    Tenant,
}

#[derive(Debug, Clone, Copy)]
enum InterfaceTarget {
    Provider,
    Connectivity,
}

impl ExporterClassification {
    fn exporter_target_mut(&mut self, target: &ExporterTarget) -> &mut String {
        match target {
            ExporterTarget::Group => &mut self.group,
            ExporterTarget::Role => &mut self.role,
            ExporterTarget::Site => &mut self.site,
            ExporterTarget::Region => &mut self.region,
            ExporterTarget::Tenant => &mut self.tenant,
        }
    }
}

impl InterfaceClassification {
    fn interface_target_mut(&mut self, target: &InterfaceTarget) -> &mut String {
        match target {
            InterfaceTarget::Provider => &mut self.provider,
            InterfaceTarget::Connectivity => &mut self.connectivity,
        }
    }
}

#[derive(Debug, Clone)]
enum ValueExpr {
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
    fn resolve(
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
                )))
            }
        }
    }

    fn is_string_expression(&self) -> bool {
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

#[derive(Debug, Clone)]
enum FieldExpr {
    ExporterIp,
    ExporterName,
    InterfaceIndex,
    InterfaceName,
    InterfaceDescription,
    InterfaceSpeed,
    InterfaceVlan,
    CurrentExporterGroup,
    CurrentExporterRole,
    CurrentExporterSite,
    CurrentExporterRegion,
    CurrentExporterTenant,
    CurrentInterfaceConnectivity,
    CurrentInterfaceProvider,
    CurrentInterfaceBoundary,
    CurrentInterfaceName,
    CurrentInterfaceDescription,
}

impl FieldExpr {
    fn parse(input: &str) -> Option<Self> {
        match input {
            "Exporter.IP" => Some(Self::ExporterIp),
            "Exporter.Name" => Some(Self::ExporterName),
            "Interface.Index" => Some(Self::InterfaceIndex),
            "Interface.Name" => Some(Self::InterfaceName),
            "Interface.Description" => Some(Self::InterfaceDescription),
            "Interface.Speed" => Some(Self::InterfaceSpeed),
            "Interface.VLAN" => Some(Self::InterfaceVlan),
            "CurrentClassification.Group" => Some(Self::CurrentExporterGroup),
            "CurrentClassification.Role" => Some(Self::CurrentExporterRole),
            "CurrentClassification.Site" => Some(Self::CurrentExporterSite),
            "CurrentClassification.Region" => Some(Self::CurrentExporterRegion),
            "CurrentClassification.Tenant" => Some(Self::CurrentExporterTenant),
            "CurrentClassification.Connectivity" => Some(Self::CurrentInterfaceConnectivity),
            "CurrentClassification.Provider" => Some(Self::CurrentInterfaceProvider),
            "CurrentClassification.Boundary" => Some(Self::CurrentInterfaceBoundary),
            "CurrentClassification.Name" => Some(Self::CurrentInterfaceName),
            "CurrentClassification.Description" => Some(Self::CurrentInterfaceDescription),
            _ => None,
        }
    }

    fn resolve(
        &self,
        exporter: Option<&ExporterInfo>,
        interface: Option<&InterfaceInfo>,
        exporter_classification: Option<&ExporterClassification>,
        interface_classification: Option<&InterfaceClassification>,
    ) -> Result<ResolvedValue> {
        match self {
            FieldExpr::ExporterIp => Ok(ResolvedValue::String(
                exporter.map(|exp| exp.ip.clone()).unwrap_or_default(),
            )),
            FieldExpr::ExporterName => Ok(ResolvedValue::String(
                exporter.map(|exp| exp.name.clone()).unwrap_or_default(),
            )),
            FieldExpr::InterfaceIndex => Ok(ResolvedValue::Number(
                interface.map(|iface| iface.index as i64).unwrap_or(0),
            )),
            FieldExpr::InterfaceName => Ok(ResolvedValue::String(
                interface
                    .map(|iface| iface.name.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::InterfaceDescription => Ok(ResolvedValue::String(
                interface
                    .map(|iface| iface.description.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::InterfaceSpeed => Ok(ResolvedValue::Number(
                interface.map(|iface| iface.speed as i64).unwrap_or(0),
            )),
            FieldExpr::InterfaceVlan => Ok(ResolvedValue::Number(
                interface.map(|iface| iface.vlan as i64).unwrap_or(0),
            )),
            FieldExpr::CurrentExporterGroup => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.group.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterRole => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.role.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterSite => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.site.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterRegion => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.region.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentExporterTenant => Ok(ResolvedValue::String(
                exporter_classification
                    .map(|classification| classification.tenant.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceConnectivity => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.connectivity.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceProvider => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.provider.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceBoundary => Ok(ResolvedValue::Number(
                interface_classification
                    .map(|classification| classification.boundary as i64)
                    .unwrap_or(0),
            )),
            FieldExpr::CurrentInterfaceName => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.name.clone())
                    .unwrap_or_default(),
            )),
            FieldExpr::CurrentInterfaceDescription => Ok(ResolvedValue::String(
                interface_classification
                    .map(|classification| classification.description.clone())
                    .unwrap_or_default(),
            )),
        }
    }

    fn is_string_field(&self) -> bool {
        !matches!(
            self,
            FieldExpr::InterfaceIndex
                | FieldExpr::InterfaceSpeed
                | FieldExpr::InterfaceVlan
                | FieldExpr::CurrentInterfaceBoundary
        )
    }
}

#[derive(Debug, Clone)]
enum ResolvedValue {
    String(String),
    Number(i64),
    List(Vec<ResolvedValue>),
}

impl ResolvedValue {
    fn to_string_value(&self) -> String {
        match self {
            ResolvedValue::String(value) => value.clone(),
            ResolvedValue::Number(value) => value.to_string(),
            ResolvedValue::List(values) => values
                .iter()
                .map(ResolvedValue::to_string_value)
                .collect::<Vec<_>>()
                .join(","),
        }
    }

    fn to_i64(&self) -> Option<i64> {
        match self {
            ResolvedValue::String(value) => value.parse::<i64>().ok(),
            ResolvedValue::Number(value) => Some(*value),
            ResolvedValue::List(_) => None,
        }
    }

    fn as_list(&self) -> Option<&[ResolvedValue]> {
        match self {
            ResolvedValue::List(values) => Some(values.as_slice()),
            _ => None,
        }
    }
}

fn parse_boolean_expr(input: &str) -> Result<BoolExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty classifier rule");
    }
    parse_or_expr(input)
}

fn parse_or_expr(input: &str) -> Result<BoolExpr> {
    let parts = split_top_level(input, "||");
    let parts = if parts.len() > 1 {
        parts
    } else {
        split_top_level_keyword(input, "or")
    };
    if parts.len() > 1 {
        let mut iter = parts.into_iter();
        let first = parse_and_expr(
            iter.next()
                .expect("split_top_level for '||' must return non-empty parts")
                .trim(),
        )?;
        return iter.try_fold(first, |left, part| {
            Ok(BoolExpr::Or(
                Box::new(left),
                Box::new(parse_and_expr(part.trim())?),
            ))
        });
    }
    parse_and_expr(input)
}

fn parse_and_expr(input: &str) -> Result<BoolExpr> {
    let parts = split_top_level(input, "&&");
    let parts = if parts.len() > 1 {
        parts
    } else {
        split_top_level_keyword(input, "and")
    };
    if parts.len() > 1 {
        let mut iter = parts.into_iter();
        let first = parse_unary_expr(
            iter.next()
                .expect("split_top_level for '&&' must return non-empty parts")
                .trim(),
        )?;
        return iter.try_fold(first, |left, part| {
            Ok(BoolExpr::And(
                Box::new(left),
                Box::new(parse_unary_expr(part.trim())?),
            ))
        });
    }
    parse_unary_expr(input)
}

fn parse_unary_expr(input: &str) -> Result<BoolExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty classifier expression");
    }

    if let Some(rest) = input.strip_prefix('!') {
        return Ok(BoolExpr::Not(Box::new(parse_unary_expr(rest)?)));
    }
    if let Some(rest) = strip_keyword_prefix(input, "not") {
        return Ok(BoolExpr::Not(Box::new(parse_unary_expr(rest)?)));
    }

    if let Some(inner) = strip_outer_parentheses(input) {
        return parse_boolean_expr(inner);
    }

    Ok(BoolExpr::Term(parse_rule_term(input)?))
}

fn parse_rule_term(term: &str) -> Result<RuleTerm> {
    let term = term.trim();
    if term.is_empty() {
        anyhow::bail!("empty rule term");
    }

    if term == "true" {
        return Ok(RuleTerm::Condition(ConditionExpr::Literal(true)));
    }
    if term == "false" {
        return Ok(RuleTerm::Condition(ConditionExpr::Literal(false)));
    }

    if let Some((name, args)) = parse_function_call(term) {
        let action = parse_action(&name, &args)?;
        return Ok(RuleTerm::Action(action));
    }

    if let Some((left, right)) = split_once_top_level(term, " startsWith ") {
        return Ok(RuleTerm::Condition(ConditionExpr::StartsWith(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " matches ") {
        let left = parse_value_expr(left.trim())?;
        let right = parse_value_expr(right.trim())?;
        validate_literal_regex_value(&right, "matches")?;
        return Ok(RuleTerm::Condition(ConditionExpr::Matches(left, right)));
    }
    if let Some((left, right)) = split_once_top_level(term, " contains ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Contains(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " endsWith ") {
        return Ok(RuleTerm::Condition(ConditionExpr::EndsWith(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " != ") {
        return Ok(RuleTerm::Condition(ConditionExpr::NotEquals(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " >= ") {
        return Ok(RuleTerm::Condition(ConditionExpr::GreaterOrEqual(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " <= ") {
        return Ok(RuleTerm::Condition(ConditionExpr::LessOrEqual(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level_keyword(term, "in") {
        return Ok(RuleTerm::Condition(ConditionExpr::In(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " == ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Equals(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " > ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Greater(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }
    if let Some((left, right)) = split_once_top_level(term, " < ") {
        return Ok(RuleTerm::Condition(ConditionExpr::Less(
            parse_value_expr(left.trim())?,
            parse_value_expr(right.trim())?,
        )));
    }

    anyhow::bail!("unsupported rule term: {term}")
}

fn parse_action(name: &str, args: &[String]) -> Result<ActionExpr> {
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

fn one_arg(name: &str, args: &[String]) -> Result<ValueExpr> {
    if args.len() != 1 {
        anyhow::bail!("{name}() expects exactly 1 argument");
    }
    parse_value_expr(args[0].trim())
}

fn three_args(name: &str, args: &[String]) -> Result<(ValueExpr, ValueExpr, ValueExpr)> {
    if args.len() != 3 {
        anyhow::bail!("{name}() expects exactly 3 arguments");
    }
    Ok((
        parse_value_expr(args[0].trim())?,
        parse_value_expr(args[1].trim())?,
        parse_value_expr(args[2].trim())?,
    ))
}

fn one_string_arg(name: &str, args: &[String]) -> Result<ValueExpr> {
    let value = one_arg(name, args)?;
    if !value.is_string_expression() {
        anyhow::bail!("{name}() expects a string argument");
    }
    Ok(value)
}

fn three_string_args(name: &str, args: &[String]) -> Result<(ValueExpr, ValueExpr, ValueExpr)> {
    let (arg1, arg2, arg3) = three_args(name, args)?;
    if !arg1.is_string_expression() || !arg2.is_string_expression() || !arg3.is_string_expression()
    {
        anyhow::bail!("{name}() expects string arguments");
    }
    validate_literal_regex_value(&arg2, name)?;
    Ok((arg1, arg2, arg3))
}

fn validate_literal_regex_value(value: &ValueExpr, context: &str) -> Result<()> {
    if let ValueExpr::StringLiteral(pattern) = value {
        Regex::new(pattern)
            .with_context(|| format!("invalid regex '{pattern}' in {context} expression"))?;
    }
    Ok(())
}

fn parse_value_expr(input: &str) -> Result<ValueExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty expression");
    }

    let plus_parts = split_top_level(input, "+");
    if plus_parts.len() > 1 {
        let mut parts = Vec::with_capacity(plus_parts.len());
        for part in plus_parts {
            parts.push(parse_value_expr(part.trim())?);
        }
        return Ok(ValueExpr::Concat(parts));
    }

    if let Some(string) = parse_quoted_string(input) {
        return Ok(ValueExpr::StringLiteral(string));
    }
    if let Ok(number) = input.parse::<i64>() {
        return Ok(ValueExpr::NumberLiteral(number));
    }
    if let Some(items) = parse_array_literal(input)? {
        return Ok(ValueExpr::List(items));
    }
    if let Some(field) = FieldExpr::parse(input) {
        return Ok(ValueExpr::Field(field));
    }
    if let Some((name, args)) = parse_function_call(input)
        && name == "Format"
    {
        if args.is_empty() {
            anyhow::bail!("Format() expects at least one argument");
        }
        let pattern = parse_value_expr(args[0].trim())?;
        let mut fmt_args = Vec::new();
        for arg in args.iter().skip(1) {
            fmt_args.push(parse_value_expr(arg.trim())?);
        }
        return Ok(ValueExpr::Format {
            pattern: Box::new(pattern),
            args: fmt_args,
        });
    }

    anyhow::bail!("unsupported value expression: {input}")
}

fn parse_array_literal(input: &str) -> Result<Option<Vec<ValueExpr>>> {
    let input = input.trim();
    if !input.starts_with('[') || !input.ends_with(']') {
        return Ok(None);
    }

    if !is_wrapped_by_top_level_delimiters(input, '[', ']') {
        return Ok(None);
    }

    let inner = input[1..input.len() - 1].trim();
    if inner.is_empty() {
        return Ok(Some(Vec::new()));
    }

    let mut values = Vec::new();
    for item in split_top_level(inner, ",") {
        values.push(parse_value_expr(item.trim())?);
    }
    Ok(Some(values))
}

fn parse_function_call(input: &str) -> Option<(String, Vec<String>)> {
    let input = input.trim();
    if !input.ends_with(')') {
        return None;
    }
    let open = input.find('(')?;
    let name = input[..open].trim();
    if name.is_empty() {
        return None;
    }
    let args_raw = &input[open + 1..input.len() - 1];
    let args = if args_raw.trim().is_empty() {
        Vec::new()
    } else {
        split_top_level(args_raw, ",")
    };
    Some((name.to_string(), args))
}

fn parse_quoted_string(input: &str) -> Option<String> {
    if input.len() < 2 || !input.starts_with('"') || !input.ends_with('"') {
        return None;
    }
    serde_json::from_str::<String>(input).ok()
}

fn strip_outer_parentheses(input: &str) -> Option<&str> {
    let input = input.trim();
    if !is_wrapped_by_top_level_delimiters(input, '(', ')') {
        return None;
    }
    Some(input[1..input.len() - 1].trim())
}

fn split_top_level(input: &str, sep: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let sep_bytes = sep.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(sep_bytes)
        {
            parts.push(input[start..i].trim().to_string());
            i += sep.len();
            start = i;
            continue;
        }
        i += 1;
    }

    parts.push(input[start..].trim().to_string());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

fn split_once_top_level<'a>(input: &'a str, sep: &str) -> Option<(&'a str, &'a str)> {
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let sep_bytes = sep.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(sep_bytes)
        {
            let left = &input[..i];
            let right = &input[i + sep.len()..];
            return Some((left, right));
        }
        i += 1;
    }

    None
}

fn split_top_level_keyword(input: &str, keyword: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let keyword_bytes = keyword.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(keyword_bytes)
            && is_keyword_boundary(input, i, keyword_bytes.len())
        {
            parts.push(input[start..i].trim().to_string());
            i += keyword.len();
            start = i;
            continue;
        }

        i += 1;
    }

    parts.push(input[start..].trim().to_string());
    parts.into_iter().filter(|part| !part.is_empty()).collect()
}

fn split_once_top_level_keyword<'a>(input: &'a str, keyword: &str) -> Option<(&'a str, &'a str)> {
    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let bytes = input.as_bytes();
    let keyword_bytes = keyword.as_bytes();
    let mut i = 0_usize;

    while i < bytes.len() {
        let ch = bytes[i] as char;
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            i += 1;
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        if paren_depth == 0
            && bracket_depth == 0
            && brace_depth == 0
            && bytes[i..].starts_with(keyword_bytes)
            && is_keyword_boundary(input, i, keyword_bytes.len())
        {
            let left = &input[..i];
            let right = &input[i + keyword.len()..];
            return Some((left, right));
        }
        i += 1;
    }
    None
}

fn strip_keyword_prefix<'a>(input: &'a str, keyword: &str) -> Option<&'a str> {
    let input = input.trim_start();
    let keyword_bytes = keyword.as_bytes();
    if !input.as_bytes().starts_with(keyword_bytes) {
        return None;
    }
    if !is_keyword_boundary(input, 0, keyword.len()) {
        return None;
    }
    Some(input[keyword.len()..].trim_start())
}

fn is_keyword_boundary(input: &str, start: usize, len: usize) -> bool {
    let before_ok = if start == 0 {
        true
    } else {
        input[..start]
            .chars()
            .next_back()
            .map(|ch| !is_identifier_char(ch))
            .unwrap_or(true)
    };
    let after_index = start + len;
    let after_ok = if after_index >= input.len() {
        true
    } else {
        input[after_index..]
            .chars()
            .next()
            .map(|ch| !is_identifier_char(ch))
            .unwrap_or(true)
    };
    before_ok && after_ok
}

fn is_identifier_char(ch: char) -> bool {
    ch.is_ascii_alphanumeric() || ch == '_' || ch == '.'
}

fn is_wrapped_by_top_level_delimiters(input: &str, open: char, close: char) -> bool {
    let input = input.trim();
    if !input.starts_with(open) || !input.ends_with(close) {
        return false;
    }

    let mut paren_depth = 0_i32;
    let mut bracket_depth = 0_i32;
    let mut brace_depth = 0_i32;
    let mut in_string = false;
    let mut escaped = false;
    let chars: Vec<(usize, char)> = input.char_indices().collect();

    for (idx, ch) in chars.iter().copied() {
        if in_string {
            if escaped {
                escaped = false;
            } else if ch == '\\' {
                escaped = true;
            } else if ch == '"' {
                in_string = false;
            }
            continue;
        }

        match ch {
            '"' => in_string = true,
            '(' => paren_depth += 1,
            ')' => paren_depth -= 1,
            '[' => bracket_depth += 1,
            ']' => bracket_depth -= 1,
            '{' => brace_depth += 1,
            '}' => brace_depth -= 1,
            _ => {}
        }

        let current_depth = match open {
            '(' => paren_depth,
            '[' => bracket_depth,
            '{' => brace_depth,
            _ => return false,
        };
        if current_depth == 0 && idx < input.len() - ch.len_utf8() {
            return false;
        }
        if paren_depth < 0 || bracket_depth < 0 || brace_depth < 0 {
            return false;
        }
    }

    paren_depth == 0 && bracket_depth == 0 && brace_depth == 0 && !in_string && !escaped
}

fn normalize_classifier_value(input: &str) -> String {
    input
        .to_ascii_lowercase()
        .chars()
        .filter(|ch| ch.is_ascii_alphanumeric() || *ch == '.' || *ch == '+' || *ch == '-')
        .collect()
}

fn apply_regex_template(input: &str, pattern: &str, template: &str) -> Result<Option<String>> {
    let regex = Regex::new(pattern).with_context(|| format!("invalid regex '{pattern}'"))?;
    if let Some(captures) = regex.captures(input) {
        let mut output = String::new();
        captures.expand(template, &mut output);
        return Ok(Some(output));
    }
    Ok(None)
}

fn format_with_percent_placeholders(pattern: &str, args: &[ResolvedValue]) -> String {
    let mut out = String::new();
    let chars: Vec<char> = pattern.chars().collect();
    let mut idx = 0_usize;
    let mut arg_idx = 0_usize;

    while idx < chars.len() {
        let ch = chars[idx];
        if ch == '%' && idx + 1 < chars.len() {
            let spec = chars[idx + 1];
            match spec {
                's' | 'v' => {
                    if let Some(arg) = args.get(arg_idx) {
                        out.push_str(&arg.to_string_value());
                        arg_idx += 1;
                    }
                    idx += 2;
                    continue;
                }
                'd' => {
                    if let Some(arg) = args.get(arg_idx) {
                        out.push_str(&arg.to_i64().unwrap_or(0).to_string());
                        arg_idx += 1;
                    }
                    idx += 2;
                    continue;
                }
                '%' => {
                    out.push('%');
                    idx += 2;
                    continue;
                }
                _ => {}
            }
        }
        out.push(ch);
        idx += 1;
    }

    out
}
