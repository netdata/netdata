use super::*;

#[derive(Debug, Clone)]
pub(crate) enum FieldExpr {
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
    pub(crate) fn parse(input: &str) -> Option<Self> {
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

    pub(crate) fn resolve(
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

    pub(crate) fn is_string_field(&self) -> bool {
        !matches!(
            self,
            FieldExpr::InterfaceIndex
                | FieldExpr::InterfaceSpeed
                | FieldExpr::InterfaceVlan
                | FieldExpr::CurrentInterfaceBoundary
        )
    }
}
