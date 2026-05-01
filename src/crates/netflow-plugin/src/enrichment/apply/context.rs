use super::*;

#[derive(Debug)]
pub(super) struct EnrichmentContext {
    pub(super) exporter_name: String,
    pub(super) exporter_classification: ExporterClassification,
    pub(super) in_interface: InterfaceInfo,
    pub(super) out_interface: InterfaceInfo,
    pub(super) in_classification: InterfaceClassification,
    pub(super) out_classification: InterfaceClassification,
}

impl EnrichmentContext {
    pub(super) fn new(
        exporter_name: String,
        in_if: u32,
        out_if: u32,
        src_vlan: u16,
        dst_vlan: u16,
    ) -> Self {
        Self {
            exporter_name,
            exporter_classification: ExporterClassification::default(),
            in_interface: InterfaceInfo {
                index: in_if,
                vlan: src_vlan,
                ..Default::default()
            },
            out_interface: InterfaceInfo {
                index: out_if,
                vlan: dst_vlan,
                ..Default::default()
            },
            in_classification: InterfaceClassification::default(),
            out_classification: InterfaceClassification::default(),
        }
    }

    pub(super) fn exporter_info(&self, exporter_ip: &str) -> ExporterInfo {
        ExporterInfo {
            ip: exporter_ip.to_string(),
            name: self.exporter_name.clone(),
        }
    }
}

#[derive(Debug, Default)]
pub(super) struct ResolvedFlowContext {
    pub(super) source_network: Option<NetworkAttributes>,
    pub(super) dest_network: Option<NetworkAttributes>,
    pub(super) dest_routing: Option<StaticRoutingEntry>,
    pub(super) source_mask: u8,
    pub(super) dest_mask: u8,
    pub(super) source_as: u32,
    pub(super) dest_as: u32,
    pub(super) next_hop: Option<IpAddr>,
}
