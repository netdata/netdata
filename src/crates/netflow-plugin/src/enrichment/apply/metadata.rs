use super::context::EnrichmentContext;
use super::*;

impl FlowEnricher {
    pub(super) fn apply_static_metadata(
        &self,
        exporter_ip: IpAddr,
        context: &mut EnrichmentContext,
    ) {
        self.apply_static_lookup(
            exporter_ip,
            &mut context.in_interface,
            &mut context.in_classification,
            &mut context.exporter_name,
            &mut context.exporter_classification,
        );
        self.apply_static_lookup(
            exporter_ip,
            &mut context.out_interface,
            &mut context.out_classification,
            &mut context.exporter_name,
            &mut context.exporter_classification,
        );
    }

    fn apply_static_lookup(
        &self,
        exporter_ip: IpAddr,
        interface: &mut InterfaceInfo,
        classification: &mut InterfaceClassification,
        exporter_name: &mut String,
        exporter_classification: &mut ExporterClassification,
    ) {
        if interface.index == 0 {
            return;
        }
        let Some(lookup) = self.static_metadata.lookup(exporter_ip, interface.index) else {
            return;
        };

        *exporter_name = lookup.exporter.name.clone();
        exporter_classification.group = lookup.exporter.group.clone();
        exporter_classification.role = lookup.exporter.role.clone();
        exporter_classification.site = lookup.exporter.site.clone();
        exporter_classification.region = lookup.exporter.region.clone();
        exporter_classification.tenant = lookup.exporter.tenant.clone();

        interface.name = lookup.interface.name.clone();
        interface.description = lookup.interface.description.clone();
        interface.speed = lookup.interface.speed;
        classification.provider = lookup.interface.provider.clone();
        classification.connectivity = lookup.interface.connectivity.clone();
        classification.boundary = lookup.interface.boundary;
    }

    #[cfg(test)]
    pub(super) fn apply_sampling_rate_fields(&self, exporter_ip: IpAddr, fields: &mut FlowFields) {
        if let Some(sampling_rate) = self
            .override_sampling_rate
            .lookup(exporter_ip)
            .copied()
            .filter(|rate| *rate > 0)
        {
            fields.insert("SAMPLING_RATE", sampling_rate.to_string());
        }
        if parse_u64_field(fields, "SAMPLING_RATE") == 0 {
            if let Some(sampling_rate) = self
                .default_sampling_rate
                .lookup(exporter_ip)
                .copied()
                .filter(|rate| *rate > 0)
            {
                fields.insert("SAMPLING_RATE", sampling_rate.to_string());
            }
        }
    }

    pub(super) fn apply_sampling_rate_record(&self, exporter_ip: IpAddr, rec: &mut FlowRecord) {
        if let Some(sampling_rate) = self
            .override_sampling_rate
            .lookup(exporter_ip)
            .copied()
            .filter(|rate| *rate > 0)
        {
            rec.set_sampling_rate(sampling_rate);
        }
        if !rec.has_sampling_rate() {
            if let Some(sampling_rate) = self
                .default_sampling_rate
                .lookup(exporter_ip)
                .copied()
                .filter(|rate| *rate > 0)
            {
                rec.set_sampling_rate(sampling_rate);
            }
        }
    }

    pub(super) fn classify_context(
        &self,
        exporter_ip: &str,
        context: &mut EnrichmentContext,
    ) -> bool {
        let exporter_info = context.exporter_info(exporter_ip);
        if !self.classify_exporter(&exporter_info, &mut context.exporter_classification) {
            return false;
        }
        if !self.classify_interface(
            &exporter_info,
            &context.out_interface,
            &context.exporter_classification,
            &mut context.out_classification,
        ) {
            return false;
        }
        if !self.classify_interface(
            &exporter_info,
            &context.in_interface,
            &context.exporter_classification,
            &mut context.in_classification,
        ) {
            return false;
        }
        true
    }
}
