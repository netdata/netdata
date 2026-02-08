use crate::plugin_config::{
    EnrichmentConfig, SamplingRateSetting, StaticExporterConfig, StaticInterfaceConfig,
};
use anyhow::{Context, Result};
use ipnet::IpNet;
use regex::Regex;
use std::collections::{BTreeMap, HashMap};
use std::net::IpAddr;
use std::str::FromStr;

#[derive(Debug, Clone)]
pub(crate) struct FlowEnricher {
    default_sampling_rate: PrefixMap<u64>,
    override_sampling_rate: PrefixMap<u64>,
    static_metadata: StaticMetadata,
    exporter_classifiers: Vec<ClassifierRule>,
    interface_classifiers: Vec<ClassifierRule>,
}

impl FlowEnricher {
    pub(crate) fn from_config(config: &EnrichmentConfig) -> Result<Option<Self>> {
        let default_sampling_rate = build_sampling_map(
            config.default_sampling_rate.as_ref(),
            "enrichment.default_sampling_rate",
        )?;
        let override_sampling_rate = build_sampling_map(
            config.override_sampling_rate.as_ref(),
            "enrichment.override_sampling_rate",
        )?;
        let static_metadata = StaticMetadata::from_config(config)?;

        let exporter_classifiers = config
            .exporter_classifiers
            .iter()
            .enumerate()
            .map(|(idx, rule)| {
                ClassifierRule::parse(rule).with_context(|| {
                    format!("invalid enrichment.exporter_classifiers[{idx}] rule: {rule}")
                })
            })
            .collect::<Result<Vec<_>>>()?;
        let interface_classifiers = config
            .interface_classifiers
            .iter()
            .enumerate()
            .map(|(idx, rule)| {
                ClassifierRule::parse(rule).with_context(|| {
                    format!("invalid enrichment.interface_classifiers[{idx}] rule: {rule}")
                })
            })
            .collect::<Result<Vec<_>>>()?;

        if default_sampling_rate.is_empty()
            && override_sampling_rate.is_empty()
            && static_metadata.is_empty()
            && exporter_classifiers.is_empty()
            && interface_classifiers.is_empty()
        {
            return Ok(None);
        }

        Ok(Some(Self {
            default_sampling_rate,
            override_sampling_rate,
            static_metadata,
            exporter_classifiers,
            interface_classifiers,
        }))
    }

    pub(crate) fn enrich_fields(&mut self, fields: &mut BTreeMap<String, String>) -> bool {
        let Some(exporter_ip) = parse_exporter_ip(fields) else {
            return true;
        };
        let exporter_ip_str = exporter_ip.to_string();
        let in_if = parse_u32_field(fields, "IN_IF");
        let out_if = parse_u32_field(fields, "OUT_IF");

        let mut exporter_name = String::new();
        let mut exporter_classification = ExporterClassification::default();
        let mut in_interface = InterfaceInfo {
            index: in_if,
            vlan: parse_u16_field(fields, "SRC_VLAN"),
            ..Default::default()
        };
        let mut out_interface = InterfaceInfo {
            index: out_if,
            vlan: parse_u16_field(fields, "DST_VLAN"),
            ..Default::default()
        };
        let mut in_classification = InterfaceClassification::default();
        let mut out_classification = InterfaceClassification::default();

        if in_if != 0
            && let Some(lookup) = self.static_metadata.lookup(exporter_ip, in_if)
        {
            exporter_name = lookup.exporter.name.clone();
            exporter_classification.group = lookup.exporter.group.clone();
            exporter_classification.role = lookup.exporter.role.clone();
            exporter_classification.site = lookup.exporter.site.clone();
            exporter_classification.region = lookup.exporter.region.clone();
            exporter_classification.tenant = lookup.exporter.tenant.clone();

            in_interface.name = lookup.interface.name.clone();
            in_interface.description = lookup.interface.description.clone();
            in_interface.speed = lookup.interface.speed;
            in_classification.provider = lookup.interface.provider.clone();
            in_classification.connectivity = lookup.interface.connectivity.clone();
            in_classification.boundary = lookup.interface.boundary;
        }

        if out_if != 0
            && let Some(lookup) = self.static_metadata.lookup(exporter_ip, out_if)
        {
            exporter_name = lookup.exporter.name.clone();
            exporter_classification.group = lookup.exporter.group.clone();
            exporter_classification.role = lookup.exporter.role.clone();
            exporter_classification.site = lookup.exporter.site.clone();
            exporter_classification.region = lookup.exporter.region.clone();
            exporter_classification.tenant = lookup.exporter.tenant.clone();

            out_interface.name = lookup.interface.name.clone();
            out_interface.description = lookup.interface.description.clone();
            out_interface.speed = lookup.interface.speed;
            out_classification.provider = lookup.interface.provider.clone();
            out_classification.connectivity = lookup.interface.connectivity.clone();
            out_classification.boundary = lookup.interface.boundary;
        }

        // Akvorado parity: reject records lacking interface identity.
        if in_if == 0 && out_if == 0 {
            return false;
        }
        // Akvorado parity: metadata lookup must yield an exporter name.
        if exporter_name.is_empty() {
            return false;
        }

        if let Some(sampling_rate) = self
            .override_sampling_rate
            .lookup(exporter_ip)
            .copied()
            .filter(|rate| *rate > 0)
        {
            fields.insert("SAMPLING_RATE".to_string(), sampling_rate.to_string());
        }
        if parse_u64_field(fields, "SAMPLING_RATE") == 0 {
            if let Some(sampling_rate) = self
                .default_sampling_rate
                .lookup(exporter_ip)
                .copied()
                .filter(|rate| *rate > 0)
            {
                fields.insert("SAMPLING_RATE".to_string(), sampling_rate.to_string());
            } else {
                // Akvorado parity: sampling rate is required after overrides/defaults.
                return false;
            }
        }

        let exporter_info = ExporterInfo {
            ip: exporter_ip_str.clone(),
            name: exporter_name.clone(),
        };
        if !self.classify_exporter(&exporter_info, &mut exporter_classification) {
            return false;
        }

        if !self.classify_interface(&exporter_info, &out_interface, &mut out_classification) {
            return false;
        }
        if !self.classify_interface(&exporter_info, &in_interface, &mut in_classification) {
            return false;
        }

        fields.insert("EXPORTER_NAME".to_string(), exporter_name);
        fields.insert("EXPORTER_GROUP".to_string(), exporter_classification.group);
        fields.insert("EXPORTER_ROLE".to_string(), exporter_classification.role);
        fields.insert("EXPORTER_SITE".to_string(), exporter_classification.site);
        fields.insert(
            "EXPORTER_REGION".to_string(),
            exporter_classification.region,
        );
        fields.insert(
            "EXPORTER_TENANT".to_string(),
            exporter_classification.tenant,
        );

        fields.insert("IN_IF_NAME".to_string(), in_classification.name);
        fields.insert(
            "IN_IF_DESCRIPTION".to_string(),
            in_classification.description,
        );
        fields.insert("IN_IF_SPEED".to_string(), in_interface.speed.to_string());
        fields.insert("IN_IF_PROVIDER".to_string(), in_classification.provider);
        fields.insert(
            "IN_IF_CONNECTIVITY".to_string(),
            in_classification.connectivity,
        );
        fields.insert(
            "IN_IF_BOUNDARY".to_string(),
            in_classification.boundary.to_string(),
        );

        fields.insert("OUT_IF_NAME".to_string(), out_classification.name);
        fields.insert(
            "OUT_IF_DESCRIPTION".to_string(),
            out_classification.description,
        );
        fields.insert("OUT_IF_SPEED".to_string(), out_interface.speed.to_string());
        fields.insert("OUT_IF_PROVIDER".to_string(), out_classification.provider);
        fields.insert(
            "OUT_IF_CONNECTIVITY".to_string(),
            out_classification.connectivity,
        );
        fields.insert(
            "OUT_IF_BOUNDARY".to_string(),
            out_classification.boundary.to_string(),
        );

        true
    }

    fn classify_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> bool {
        // Akvorado parity: metadata-provided classification has priority.
        if !classification.is_empty() {
            return !classification.reject;
        }
        if self.exporter_classifiers.is_empty() {
            return true;
        }

        for rule in &self.exporter_classifiers {
            if rule.evaluate_exporter(exporter, classification).is_err() {
                break;
            }
            if classification.is_complete() {
                break;
            }
        }

        !classification.reject
    }

    fn classify_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> bool {
        // Akvorado parity: metadata-provided classification has priority.
        if !classification.is_empty() {
            classification.name = interface.name.clone();
            classification.description = interface.description.clone();
            return !classification.reject;
        }

        if self.interface_classifiers.is_empty() {
            classification.name = interface.name.clone();
            classification.description = interface.description.clone();
            return true;
        }

        for rule in &self.interface_classifiers {
            if rule
                .evaluate_interface(exporter, interface, classification)
                .is_err()
            {
                break;
            }
            if !classification.connectivity.is_empty()
                && !classification.provider.is_empty()
                && classification.boundary != 0
            {
                break;
            }
        }

        if classification.name.is_empty() {
            classification.name = interface.name.clone();
        }
        if classification.description.is_empty() {
            classification.description = interface.description.clone();
        }

        !classification.reject
    }
}

#[derive(Debug, Clone, Default)]
struct ExporterInfo {
    ip: String,
    name: String,
}

#[derive(Debug, Clone, Default)]
struct InterfaceInfo {
    index: u32,
    name: String,
    description: String,
    speed: u64,
    vlan: u16,
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
    terms: Vec<RuleTerm>,
}

impl ClassifierRule {
    fn parse(rule: &str) -> Result<Self> {
        let raw_terms = split_top_level(rule, "&&");
        if raw_terms.is_empty() {
            anyhow::bail!("empty classifier rule");
        }
        let terms = raw_terms
            .into_iter()
            .map(|term| parse_rule_term(term.trim()))
            .collect::<Result<Vec<_>>>()?;
        Ok(Self { terms })
    }

    fn evaluate_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        let mut accepted = true;
        for term in &self.terms {
            if !accepted {
                break;
            }
            accepted = term.eval_exporter(exporter, classification)?;
        }
        Ok(accepted)
    }

    fn evaluate_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        let mut accepted = true;
        for term in &self.terms {
            if !accepted {
                break;
            }
            accepted = term.eval_interface(exporter, interface, classification)?;
        }
        Ok(accepted)
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
    Equals(ValueExpr, ValueExpr),
    Greater(ValueExpr, ValueExpr),
    StartsWith(ValueExpr, ValueExpr),
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
                Ok(left.to_i64().unwrap_or(0) > right.to_i64().unwrap_or(0))
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
                if slot.is_empty()
                    && let Some(mapped) = apply_regex_template(&input, &pattern, &template)?
                {
                    *slot = normalize_classifier_value(&mapped);
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
                if slot.is_empty()
                    && let Some(mapped) = apply_regex_template(&input, &pattern, &template)?
                {
                    *slot = normalize_classifier_value(&mapped);
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
}

#[derive(Debug, Clone)]
enum ResolvedValue {
    String(String),
    Number(i64),
}

impl ResolvedValue {
    fn to_string_value(&self) -> String {
        match self {
            ResolvedValue::String(value) => value.clone(),
            ResolvedValue::Number(value) => value.to_string(),
        }
    }

    fn to_i64(&self) -> Option<i64> {
        match self {
            ResolvedValue::String(value) => value.parse::<i64>().ok(),
            ResolvedValue::Number(value) => Some(*value),
        }
    }
}

fn parse_rule_term(term: &str) -> Result<RuleTerm> {
    let term = term.trim();
    if term.is_empty() {
        anyhow::bail!("empty rule term");
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
            one_arg(name, args)?,
        )),
        "ClassifyRole" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Role,
            one_arg(name, args)?,
        )),
        "ClassifySite" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Site,
            one_arg(name, args)?,
        )),
        "ClassifyRegion" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Region,
            one_arg(name, args)?,
        )),
        "ClassifyTenant" => Ok(ActionExpr::ClassifyExporter(
            ExporterTarget::Tenant,
            one_arg(name, args)?,
        )),
        "ClassifyRegex" | "ClassifyGroupRegex" => Ok(ActionExpr::ClassifyExporterRegex(
            ExporterTarget::Group,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "ClassifyRoleRegex" => Ok(ActionExpr::ClassifyExporterRegex(
            ExporterTarget::Role,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "ClassifySiteRegex" => Ok(ActionExpr::ClassifyExporterRegex(
            ExporterTarget::Site,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "ClassifyRegionRegex" => Ok(ActionExpr::ClassifyExporterRegex(
            ExporterTarget::Region,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "ClassifyTenantRegex" => Ok(ActionExpr::ClassifyExporterRegex(
            ExporterTarget::Tenant,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "ClassifyProvider" => Ok(ActionExpr::ClassifyInterface(
            InterfaceTarget::Provider,
            one_arg(name, args)?,
        )),
        "ClassifyConnectivity" => Ok(ActionExpr::ClassifyInterface(
            InterfaceTarget::Connectivity,
            one_arg(name, args)?,
        )),
        "ClassifyProviderRegex" => Ok(ActionExpr::ClassifyInterfaceRegex(
            InterfaceTarget::Provider,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "ClassifyConnectivityRegex" => Ok(ActionExpr::ClassifyInterfaceRegex(
            InterfaceTarget::Connectivity,
            three_args(name, args)?.0,
            three_args(name, args)?.1,
            three_args(name, args)?.2,
        )),
        "SetName" => Ok(ActionExpr::SetName(one_arg(name, args)?)),
        "SetDescription" => Ok(ActionExpr::SetDescription(one_arg(name, args)?)),
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

fn parse_value_expr(input: &str) -> Result<ValueExpr> {
    let input = input.trim();
    if input.is_empty() {
        anyhow::bail!("empty expression");
    }

    if let Some(string) = parse_quoted_string(input) {
        return Ok(ValueExpr::StringLiteral(string));
    }
    if let Ok(number) = input.parse::<i64>() {
        return Ok(ValueExpr::NumberLiteral(number));
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

    Ok(ValueExpr::StringLiteral(input.to_string()))
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
    let inner = &input[1..input.len() - 1];
    Some(inner.replace("\\\"", "\""))
}

fn split_top_level(input: &str, sep: &str) -> Vec<String> {
    let mut parts = Vec::new();
    let mut start = 0_usize;
    let mut depth = 0_i32;
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
            '(' => depth += 1,
            ')' => depth -= 1,
            _ => {}
        }

        if depth == 0 && bytes[i..].starts_with(sep_bytes) {
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
    let mut depth = 0_i32;
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
            '(' => depth += 1,
            ')' => depth -= 1,
            _ => {}
        }

        if depth == 0 && bytes[i..].starts_with(sep_bytes) {
            let left = &input[..i];
            let right = &input[i + sep.len()..];
            return Some((left, right));
        }
        i += 1;
    }

    None
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

#[derive(Debug, Clone, Default)]
struct StaticMetadata {
    exporters: PrefixMap<StaticExporter>,
}

impl StaticMetadata {
    fn from_config(config: &EnrichmentConfig) -> Result<Self> {
        let mut exporters = PrefixMap::default();
        for (prefix, cfg) in &config.metadata_static.exporters {
            let parsed_prefix = parse_prefix(prefix)
                .with_context(|| format!("invalid metadata exporter prefix '{prefix}'"))?;
            exporters.insert(parsed_prefix, StaticExporter::from_config(cfg));
        }
        Ok(Self { exporters })
    }

    fn is_empty(&self) -> bool {
        self.exporters.is_empty()
    }

    fn lookup(&self, exporter_ip: IpAddr, if_index: u32) -> Option<StaticMetadataLookup<'_>> {
        let exporter = self.exporters.lookup(exporter_ip)?;
        let interface = exporter.lookup_interface(if_index)?;
        Some(StaticMetadataLookup {
            exporter,
            interface,
        })
    }
}

struct StaticMetadataLookup<'a> {
    exporter: &'a StaticExporter,
    interface: &'a StaticInterface,
}

#[derive(Debug, Clone, Default)]
struct StaticExporter {
    name: String,
    region: String,
    role: String,
    tenant: String,
    site: String,
    group: String,
    default_interface: StaticInterface,
    interfaces_by_index: HashMap<u32, StaticInterface>,
    skip_missing_interfaces: bool,
}

impl StaticExporter {
    fn from_config(config: &StaticExporterConfig) -> Self {
        let mut interfaces_by_index = HashMap::new();
        for (if_index, interface) in &config.if_indexes {
            interfaces_by_index.insert(*if_index, StaticInterface::from_config(interface));
        }

        Self {
            name: config.name.clone(),
            region: config.region.clone(),
            role: config.role.clone(),
            tenant: config.tenant.clone(),
            site: config.site.clone(),
            group: config.group.clone(),
            default_interface: StaticInterface::from_config(&config.default),
            interfaces_by_index,
            skip_missing_interfaces: config.skip_missing_interfaces,
        }
    }

    fn lookup_interface(&self, if_index: u32) -> Option<&StaticInterface> {
        if let Some(interface) = self.interfaces_by_index.get(&if_index) {
            return Some(interface);
        }
        if self.skip_missing_interfaces {
            return None;
        }
        Some(&self.default_interface)
    }
}

#[derive(Debug, Clone, Default)]
struct StaticInterface {
    name: String,
    description: String,
    speed: u64,
    provider: String,
    connectivity: String,
    boundary: u8,
}

impl StaticInterface {
    fn from_config(config: &StaticInterfaceConfig) -> Self {
        Self {
            name: config.name.clone(),
            description: config.description.clone(),
            speed: config.speed,
            provider: config.provider.clone(),
            connectivity: config.connectivity.clone(),
            boundary: config.boundary,
        }
    }
}

#[derive(Debug, Clone)]
struct PrefixMapEntry<T> {
    prefix: IpNet,
    value: T,
}

#[derive(Debug, Clone, Default)]
struct PrefixMap<T> {
    entries: Vec<PrefixMapEntry<T>>,
}

impl<T> PrefixMap<T> {
    fn insert(&mut self, prefix: IpNet, value: T) {
        self.entries.push(PrefixMapEntry { prefix, value });
    }

    fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    fn lookup(&self, address: IpAddr) -> Option<&T> {
        let mut best: Option<(&T, u8)> = None;

        for entry in &self.entries {
            if !entry.prefix.contains(&address) {
                continue;
            }

            let prefix_len = entry.prefix.prefix_len();
            if let Some((_, current_len)) = best
                && prefix_len <= current_len
            {
                continue;
            }

            best = Some((&entry.value, prefix_len));
        }

        best.map(|(value, _)| value)
    }
}

fn build_sampling_map(
    sampling: Option<&SamplingRateSetting>,
    field_name: &str,
) -> Result<PrefixMap<u64>> {
    let mut out = PrefixMap::default();
    let Some(sampling) = sampling else {
        return Ok(out);
    };

    match sampling {
        SamplingRateSetting::Single(rate) => {
            out.insert(parse_prefix("0.0.0.0/0")?, *rate);
            out.insert(parse_prefix("::/0")?, *rate);
        }
        SamplingRateSetting::PerPrefix(entries) => {
            for (prefix, rate) in entries {
                let parsed_prefix = parse_prefix(prefix)
                    .with_context(|| format!("{field_name}: invalid sampling prefix '{prefix}'"))?;
                out.insert(parsed_prefix, *rate);
            }
        }
    }

    Ok(out)
}

fn parse_prefix(prefix: &str) -> Result<IpNet> {
    IpNet::from_str(prefix).with_context(|| format!("invalid prefix '{prefix}'"))
}

fn parse_exporter_ip(fields: &BTreeMap<String, String>) -> Option<IpAddr> {
    fields
        .get("EXPORTER_IP")
        .and_then(|value| value.parse::<IpAddr>().ok())
}

fn parse_u16_field(fields: &BTreeMap<String, String>, key: &str) -> u16 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u16>().ok())
        .unwrap_or(0)
}

fn parse_u32_field(fields: &BTreeMap<String, String>, key: &str) -> u32 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u32>().ok())
        .unwrap_or(0)
}

fn parse_u64_field(fields: &BTreeMap<String, String>, key: &str) -> u64 {
    fields
        .get(key)
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::plugin_config::{StaticExporterConfig, StaticInterfaceConfig, StaticMetadataConfig};

    #[test]
    fn enricher_is_disabled_when_configuration_is_empty() {
        let cfg = EnrichmentConfig::default();
        let enricher = FlowEnricher::from_config(&cfg).expect("build enricher");
        assert!(enricher.is_none());
    }

    #[test]
    fn static_sampling_override_uses_most_specific_prefix() {
        let cfg = EnrichmentConfig {
            default_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([(
                "192.0.2.0/24".to_string(),
                100_u64,
            )]))),
            override_sampling_rate: Some(SamplingRateSetting::PerPrefix(BTreeMap::from([
                ("192.0.2.0/24".to_string(), 500_u64),
                ("192.0.2.128/25".to_string(), 1000_u64),
            ]))),
            metadata_static: metadata_config_for_192(),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.142", 10, 20, 0, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("SAMPLING_RATE").map(String::as_str),
            Some("1000")
        );
    }

    #[test]
    fn static_metadata_populates_exporter_and_interface_fields() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 250, 10, 300);
        assert!(enricher.enrich_fields(&mut fields));

        assert_eq!(
            fields.get("EXPORTER_NAME").map(String::as_str),
            Some("edge-router")
        );
        assert_eq!(
            fields.get("EXPORTER_GROUP").map(String::as_str),
            Some("blue")
        );
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("eu")
        );
        assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("Gi10"));
        assert_eq!(fields.get("OUT_IF_NAME").map(String::as_str), Some("Gi20"));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("transit-a")
        );
        assert_eq!(
            fields.get("OUT_IF_CONNECTIVITY").map(String::as_str),
            Some("peering")
        );
        assert_eq!(fields.get("IN_IF_BOUNDARY").map(String::as_str), Some("1"));
        assert_eq!(fields.get("OUT_IF_BOUNDARY").map(String::as_str), Some("2"));
        assert_eq!(fields.get("IN_IF_SPEED").map(String::as_str), Some("1000"));
        assert_eq!(
            fields.get("OUT_IF_SPEED").map(String::as_str),
            Some("10000")
        );
    }

    #[test]
    fn exporter_classifier_assigns_region() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![
                r#"Exporter.Name startsWith "edge" && ClassifyRegion("EU West")"#.to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("EXPORTER_REGION").map(String::as_str),
            Some("euwest")
        );
    }

    #[test]
    fn exporter_classifier_reject_drops_flow() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_exporter_classification(),
            exporter_classifiers: vec![r#"Reject()"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn interface_classifier_sets_provider_and_renames_with_format() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![
                r#"Interface.Index == 10 && ClassifyProvider("Transit-101") && SetName("eth10")"#
                    .to_string(),
                r#"Interface.VLAN > 200 && SetName(Format("%s.%d", Interface.Name, Interface.VLAN))"#
                    .to_string(),
            ],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(enricher.enrich_fields(&mut fields));
        assert_eq!(
            fields.get("IN_IF_PROVIDER").map(String::as_str),
            Some("transit-101")
        );
        assert_eq!(fields.get("IN_IF_NAME").map(String::as_str), Some("eth10"));
        assert_eq!(
            fields.get("OUT_IF_NAME").map(String::as_str),
            Some("Gi20.300")
        );
    }

    #[test]
    fn interface_classifier_reject_drops_flow() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_without_interface_classification(),
            interface_classifiers: vec![r#"Reject()"#.to_string()],
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");
        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);

        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn parity_drops_flow_when_metadata_is_missing() {
        let cfg = EnrichmentConfig {
            default_sampling_rate: Some(SamplingRateSetting::Single(1000)),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 100, 10, 300);
        assert!(!enricher.enrich_fields(&mut fields));
    }

    #[test]
    fn parity_drops_flow_when_sampling_rate_is_missing() {
        let cfg = EnrichmentConfig {
            metadata_static: metadata_config_for_192(),
            ..Default::default()
        };
        let mut enricher = FlowEnricher::from_config(&cfg)
            .expect("build enricher")
            .expect("enricher must be enabled");

        let mut fields = base_fields("192.0.2.10", 10, 20, 0, 10, 300);
        assert!(!enricher.enrich_fields(&mut fields));
    }

    fn metadata_config_for_192() -> StaticMetadataConfig {
        StaticMetadataConfig {
            exporters: BTreeMap::from([(
                "192.0.2.0/24".to_string(),
                StaticExporterConfig {
                    name: "edge-router".to_string(),
                    region: "eu".to_string(),
                    role: "peering".to_string(),
                    tenant: "tenant-a".to_string(),
                    site: "par".to_string(),
                    group: "blue".to_string(),
                    default: StaticInterfaceConfig {
                        name: "Default0".to_string(),
                        description: "Default interface".to_string(),
                        speed: 1000,
                        provider: String::new(),
                        connectivity: String::new(),
                        boundary: 0,
                    },
                    if_indexes: BTreeMap::from([
                        (
                            10_u32,
                            StaticInterfaceConfig {
                                name: "Gi10".to_string(),
                                description: "10th interface".to_string(),
                                speed: 1000,
                                provider: "transit-a".to_string(),
                                connectivity: "transit".to_string(),
                                boundary: 1,
                            },
                        ),
                        (
                            20_u32,
                            StaticInterfaceConfig {
                                name: "Gi20".to_string(),
                                description: "20th interface".to_string(),
                                speed: 10000,
                                provider: "ix".to_string(),
                                connectivity: "peering".to_string(),
                                boundary: 2,
                            },
                        ),
                    ]),
                    skip_missing_interfaces: false,
                },
            )]),
        }
    }

    fn metadata_config_without_exporter_classification() -> StaticMetadataConfig {
        let mut cfg = metadata_config_for_192();
        for exporter in cfg.exporters.values_mut() {
            exporter.region.clear();
            exporter.role.clear();
            exporter.site.clear();
            exporter.group.clear();
            exporter.tenant.clear();
        }
        cfg
    }

    fn metadata_config_without_interface_classification() -> StaticMetadataConfig {
        let mut cfg = metadata_config_without_exporter_classification();
        for exporter in cfg.exporters.values_mut() {
            for iface in exporter.if_indexes.values_mut() {
                iface.provider.clear();
                iface.connectivity.clear();
                iface.boundary = 0;
            }
        }
        cfg
    }

    fn base_fields(
        exporter_ip: &str,
        in_if: u32,
        out_if: u32,
        sampling_rate: u64,
        src_vlan: u16,
        dst_vlan: u16,
    ) -> BTreeMap<String, String> {
        BTreeMap::from([
            ("EXPORTER_IP".to_string(), exporter_ip.to_string()),
            ("IN_IF".to_string(), in_if.to_string()),
            ("OUT_IF".to_string(), out_if.to_string()),
            ("SAMPLING_RATE".to_string(), sampling_rate.to_string()),
            ("SRC_VLAN".to_string(), src_vlan.to_string()),
            ("DST_VLAN".to_string(), dst_vlan.to_string()),
            ("EXPORTER_NAME".to_string(), String::new()),
        ])
    }
}
