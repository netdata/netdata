use super::context::{EnrichmentContext, ResolvedFlowContext};
use super::*;

impl FlowEnricher {
    #[cfg(test)]
    pub(super) fn write_fields(
        &self,
        fields: &mut FlowFields,
        context: &EnrichmentContext,
        resolved: &ResolvedFlowContext,
    ) {
        self.write_field_network_enrichment(fields, resolved);

        fields.insert("EXPORTER_NAME", context.exporter_name.clone());
        fields.insert(
            "EXPORTER_GROUP",
            context.exporter_classification.group.clone(),
        );
        fields.insert(
            "EXPORTER_ROLE",
            context.exporter_classification.role.clone(),
        );
        fields.insert(
            "EXPORTER_SITE",
            context.exporter_classification.site.clone(),
        );
        fields.insert(
            "EXPORTER_REGION",
            context.exporter_classification.region.clone(),
        );
        fields.insert(
            "EXPORTER_TENANT",
            context.exporter_classification.tenant.clone(),
        );

        self.write_field_interface(
            fields,
            "IN_IF_NAME",
            "IN_IF_DESCRIPTION",
            "IN_IF_SPEED",
            "IN_IF_PROVIDER",
            "IN_IF_CONNECTIVITY",
            "IN_IF_BOUNDARY",
            &context.in_interface,
            &context.in_classification,
        );
        self.write_field_interface(
            fields,
            "OUT_IF_NAME",
            "OUT_IF_DESCRIPTION",
            "OUT_IF_SPEED",
            "OUT_IF_PROVIDER",
            "OUT_IF_CONNECTIVITY",
            "OUT_IF_BOUNDARY",
            &context.out_interface,
            &context.out_classification,
        );
    }

    #[cfg(test)]
    fn write_field_network_enrichment(
        &self,
        fields: &mut FlowFields,
        resolved: &ResolvedFlowContext,
    ) {
        fields.insert("SRC_MASK", resolved.source_mask.to_string());
        fields.insert("DST_MASK", resolved.dest_mask.to_string());
        fields.insert("SRC_AS", resolved.source_as.to_string());
        fields.insert("DST_AS", resolved.dest_as.to_string());
        fields.insert(
            "NEXT_HOP",
            resolved
                .next_hop
                .map(|addr| addr.to_string())
                .unwrap_or_default(),
        );
        write_network_attributes(
            fields,
            &SRC_KEYS,
            resolved.source_network.as_ref(),
            resolved.source_as,
        );
        write_network_attributes(
            fields,
            &DST_KEYS,
            resolved.dest_network.as_ref(),
            resolved.dest_as,
        );

        if let Some(dest_routing) = resolved.dest_routing.as_ref() {
            append_u32_list_field(fields, "DST_AS_PATH", &dest_routing.as_path);
            append_u32_list_field(fields, "DST_COMMUNITIES", &dest_routing.communities);
            append_large_communities_field(
                fields,
                "DST_LARGE_COMMUNITIES",
                &dest_routing.large_communities,
            );
        }
    }

    #[cfg(test)]
    fn write_field_interface(
        &self,
        fields: &mut FlowFields,
        name_key: &'static str,
        description_key: &'static str,
        speed_key: &'static str,
        provider_key: &'static str,
        connectivity_key: &'static str,
        boundary_key: &'static str,
        interface: &InterfaceInfo,
        classification: &InterfaceClassification,
    ) {
        fields.insert(name_key, classification.name.clone());
        fields.insert(description_key, classification.description.clone());
        if interface.speed > 0 {
            fields.insert(speed_key, interface.speed.to_string());
        } else {
            fields.remove(speed_key);
        }
        fields.insert(provider_key, classification.provider.clone());
        fields.insert(connectivity_key, classification.connectivity.clone());
        if classification.boundary != 0 {
            fields.insert(boundary_key, classification.boundary.to_string());
        } else {
            fields.remove(boundary_key);
        }
    }

    pub(super) fn write_record(
        &self,
        rec: &mut FlowRecord,
        context: EnrichmentContext,
        resolved: ResolvedFlowContext,
    ) {
        rec.src_mask = resolved.source_mask;
        rec.dst_mask = resolved.dest_mask;
        rec.src_as = resolved.source_as;
        rec.dst_as = resolved.dest_as;
        rec.next_hop = resolved.next_hop;

        write_network_attributes_record_src(rec, resolved.source_network.as_ref());
        write_network_attributes_record_dst(rec, resolved.dest_network.as_ref());

        if let Some(dest_routing) = resolved.dest_routing {
            append_u32_csv(&mut rec.dst_as_path, &dest_routing.as_path);
            append_u32_csv(&mut rec.dst_communities, &dest_routing.communities);
            append_large_communities_csv(
                &mut rec.dst_large_communities,
                &dest_routing.large_communities,
            );
        }

        rec.exporter_name = context.exporter_name;
        rec.exporter_group = context.exporter_classification.group;
        rec.exporter_role = context.exporter_classification.role;
        rec.exporter_site = context.exporter_classification.site;
        rec.exporter_region = context.exporter_classification.region;
        rec.exporter_tenant = context.exporter_classification.tenant;

        rec.in_if_name = context.in_classification.name;
        rec.in_if_description = context.in_classification.description;
        if context.in_interface.speed > 0 {
            rec.set_in_if_speed(context.in_interface.speed);
        } else {
            rec.clear_in_if_speed();
        }
        rec.in_if_provider = context.in_classification.provider;
        rec.in_if_connectivity = context.in_classification.connectivity;
        if context.in_classification.boundary != 0 {
            rec.set_in_if_boundary(context.in_classification.boundary);
        } else {
            rec.clear_in_if_boundary();
        }

        rec.out_if_name = context.out_classification.name;
        rec.out_if_description = context.out_classification.description;
        if context.out_interface.speed > 0 {
            rec.set_out_if_speed(context.out_interface.speed);
        } else {
            rec.clear_out_if_speed();
        }
        rec.out_if_provider = context.out_classification.provider;
        rec.out_if_connectivity = context.out_classification.connectivity;
        if context.out_classification.boundary != 0 {
            rec.set_out_if_boundary(context.out_classification.boundary);
        } else {
            rec.clear_out_if_boundary();
        }
    }
}
