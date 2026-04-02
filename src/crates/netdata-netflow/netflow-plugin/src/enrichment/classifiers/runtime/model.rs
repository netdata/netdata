use super::eval::BoolExpr;
use super::parse_boolean_expr;
use super::*;

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub(crate) struct ExporterInfo {
    pub(crate) ip: String,
    pub(crate) name: String,
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Hash)]
pub(crate) struct InterfaceInfo {
    pub(crate) index: u32,
    pub(crate) name: String,
    pub(crate) description: String,
    pub(crate) speed: u64,
    pub(crate) vlan: u16,
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub(crate) struct ExporterAndInterfaceInfo {
    pub(crate) exporter: ExporterInfo,
    pub(crate) interface: InterfaceInfo,
}

#[derive(Debug, Clone, Default)]
pub(crate) struct ExporterClassification {
    pub(crate) group: String,
    pub(crate) role: String,
    pub(crate) site: String,
    pub(crate) region: String,
    pub(crate) tenant: String,
    pub(crate) reject: bool,
}

impl ExporterClassification {
    pub(crate) fn is_empty(&self) -> bool {
        self.group.is_empty()
            && self.role.is_empty()
            && self.site.is_empty()
            && self.region.is_empty()
            && self.tenant.is_empty()
            && !self.reject
    }

    pub(crate) fn is_complete(&self) -> bool {
        !self.group.is_empty()
            && !self.role.is_empty()
            && !self.site.is_empty()
            && !self.region.is_empty()
            && !self.tenant.is_empty()
    }

    pub(crate) fn exporter_target_mut(&mut self, target: &ExporterTarget) -> &mut String {
        match target {
            ExporterTarget::Group => &mut self.group,
            ExporterTarget::Role => &mut self.role,
            ExporterTarget::Site => &mut self.site,
            ExporterTarget::Region => &mut self.region,
            ExporterTarget::Tenant => &mut self.tenant,
        }
    }
}

#[derive(Debug, Clone, Default)]
pub(crate) struct InterfaceClassification {
    pub(crate) connectivity: String,
    pub(crate) provider: String,
    pub(crate) boundary: u8,
    pub(crate) reject: bool,
    pub(crate) name: String,
    pub(crate) description: String,
}

impl InterfaceClassification {
    pub(crate) fn is_empty(&self) -> bool {
        self.connectivity.is_empty()
            && self.provider.is_empty()
            && self.boundary == 0
            && !self.reject
            && self.name.is_empty()
            && self.description.is_empty()
    }

    pub(crate) fn interface_target_mut(&mut self, target: &InterfaceTarget) -> &mut String {
        match target {
            InterfaceTarget::Provider => &mut self.provider,
            InterfaceTarget::Connectivity => &mut self.connectivity,
        }
    }
}

#[derive(Debug, Clone)]
pub(crate) struct ClassifierRule {
    pub(crate) expression: BoolExpr,
}

impl ClassifierRule {
    pub(crate) fn parse(rule: &str) -> Result<Self> {
        let expression = parse_boolean_expr(rule)?;
        Ok(Self { expression })
    }

    pub(crate) fn evaluate_exporter(
        &self,
        exporter: &ExporterInfo,
        classification: &mut ExporterClassification,
    ) -> Result<bool> {
        self.expression.eval_exporter(exporter, classification)
    }

    pub(crate) fn evaluate_interface(
        &self,
        exporter: &ExporterInfo,
        interface: &InterfaceInfo,
        exporter_classification: &ExporterClassification,
        classification: &mut InterfaceClassification,
    ) -> Result<bool> {
        self.expression
            .eval_interface(exporter, interface, exporter_classification, classification)
    }
}

#[derive(Debug, Clone, Copy)]
pub(crate) enum ExporterTarget {
    Group,
    Role,
    Site,
    Region,
    Tenant,
}

#[derive(Debug, Clone, Copy)]
pub(crate) enum InterfaceTarget {
    Provider,
    Connectivity,
}
