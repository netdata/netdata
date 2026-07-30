use super::*;

pub(crate) struct ProjectedScanPlan {
    pub(crate) capture_positions: FastHashMap<String, usize>,
    pub(crate) field_specs: Vec<ProjectedFieldSpec>,
    pub(crate) match_plan: Option<ProjectedFieldMatchPlan>,
    pub(crate) prefilter_matches: Vec<Vec<u8>>,
}

impl ProjectedScanPlan {
    pub(crate) fn for_group_totals(setup: &QuerySetup, request: &FlowsRequest) -> Self {
        Self::new(
            setup,
            request,
            &[ProjectedMetricField::Bytes, ProjectedMetricField::Packets],
        )
    }

    pub(crate) fn for_timeseries(
        setup: &QuerySetup,
        request: &FlowsRequest,
        sort_by: SortBy,
    ) -> Self {
        let metric = match sort_by {
            SortBy::Bytes => ProjectedMetricField::Bytes,
            SortBy::Packets => ProjectedMetricField::Packets,
        };
        Self::new(setup, request, &[metric])
    }

    fn new(
        setup: &QuerySetup,
        request: &FlowsRequest,
        metric_fields: &[ProjectedMetricField],
    ) -> Self {
        let mut captured_fields = request
            .selections
            .keys()
            .map(|field| field.to_ascii_uppercase())
            .collect::<Vec<_>>();
        captured_fields.sort_unstable();
        captured_fields.dedup();
        let capture_positions = captured_fields
            .iter()
            .cloned()
            .enumerate()
            .map(|(index, field)| (field, index))
            .collect::<FastHashMap<_, _>>();

        let mut field_specs = Vec::with_capacity(
            metric_fields.len() + setup.effective_group_by.len() + capture_positions.len() + 2,
        );
        for metric_field in metric_fields {
            let metric_key = match metric_field {
                ProjectedMetricField::Bytes => b"BYTES".as_slice(),
                ProjectedMetricField::Packets => b"PACKETS".as_slice(),
            };
            let spec_index = projected_field_spec_index(&mut field_specs, metric_key);
            field_specs[spec_index].targets.metric = Some(*metric_field);
        }
        for (index, field) in setup.effective_group_by.iter().enumerate() {
            let spec_index = projected_field_spec_index(&mut field_specs, field.as_bytes());
            field_specs[spec_index].targets.action.group_slot = Some(index);
        }
        for (field, field_index) in &capture_positions {
            let spec_index = projected_field_spec_index(&mut field_specs, field.as_bytes());
            field_specs[spec_index].targets.action.capture_slot = Some(*field_index);
        }

        let match_plan = ProjectedFieldMatchPlan::new(&field_specs);
        let prefilter_matches =
            build_prefilter_matches(&cursor_prefilter_pairs(&request.selections));

        Self {
            capture_positions,
            field_specs,
            match_plan,
            prefilter_matches,
        }
    }
}
