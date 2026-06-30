use super::*;
mod context;
mod metadata;
mod resolve;
mod write;

use context::EnrichmentContext;

impl FlowEnricher {
    #[cfg(test)]
    pub(crate) fn enrich_fields(&mut self, fields: &mut FlowFields) -> bool {
        let Some(exporter_ip) = parse_exporter_ip(fields) else {
            return true;
        };
        let exporter_ip_str = exporter_ip.to_string();
        let mut context = EnrichmentContext::new(
            fields
                .get("EXPORTER_NAME")
                .cloned()
                .filter(|value| !value.is_empty())
                .unwrap_or_else(|| exporter_ip_str.clone()),
            parse_u32_field(fields, "IN_IF"),
            parse_u32_field(fields, "OUT_IF"),
            parse_u16_field(fields, "SRC_VLAN"),
            parse_u16_field(fields, "DST_VLAN"),
        );

        self.apply_static_metadata(exporter_ip, &mut context);
        self.apply_sampling_rate_fields(exporter_ip, fields);
        if !self.classify_context(&exporter_ip_str, &mut context) {
            return false;
        }

        let resolved = self.resolve_flow_context(
            exporter_ip,
            parse_ip_field(fields, "SRC_ADDR"),
            parse_ip_field(fields, "DST_ADDR"),
            parse_ip_field(fields, "NEXT_HOP"),
            parse_u8_field(fields, "SRC_MASK"),
            parse_u8_field(fields, "DST_MASK"),
            parse_u32_field(fields, "SRC_AS"),
            parse_u32_field(fields, "DST_AS"),
        );
        self.write_fields(fields, &context, &resolved);

        true
    }

    /// Enrich a FlowRecord in place. Same logic as enrich_fields but operates
    /// on native typed fields — no string parsing or formatting on the hot path.
    pub(crate) fn enrich_record(&mut self, rec: &mut FlowRecord) -> bool {
        let Some(exporter_ip) = rec.exporter_ip else {
            return true;
        };
        let exporter_ip_str = exporter_ip.to_string();
        let mut context = EnrichmentContext::new(
            if rec.exporter_name.is_empty() {
                exporter_ip_str.clone()
            } else {
                rec.exporter_name.clone()
            },
            rec.in_if,
            rec.out_if,
            rec.src_vlan,
            rec.dst_vlan,
        );

        self.apply_static_metadata(exporter_ip, &mut context);
        self.apply_sampling_rate_record(exporter_ip, rec);
        if !self.classify_context(&exporter_ip_str, &mut context) {
            return false;
        }

        let resolved = self.resolve_flow_context(
            exporter_ip,
            rec.src_addr,
            rec.dst_addr,
            rec.next_hop,
            rec.src_mask,
            rec.dst_mask,
            rec.src_as,
            rec.dst_as,
        );
        self.write_record(rec, context, resolved);

        true
    }
}
