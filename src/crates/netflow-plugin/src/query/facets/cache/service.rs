use super::*;

impl FlowQueryService {
    pub(crate) fn facet_vocabulary_payload(&self, request: &FlowsRequest) -> Result<Value> {
        let requested_fields = requested_facet_fields(request);
        let snapshot = self.facet_runtime.snapshot();

        Ok(build_facet_vocabulary_payload(
            &requested_fields,
            &request.selections,
            &snapshot.fields,
        ))
    }

    pub(crate) fn autocomplete_field_values(
        &self,
        request: &FlowsRequest,
    ) -> Result<FlowAutocompleteQueryOutput> {
        let field = request
            .normalized_autocomplete_field()
            .context("autocomplete mode requires a field")?;
        let term = request.normalized_autocomplete_term().to_string();
        let started = Instant::now();
        let values = self.facet_runtime.autocomplete(&field, &term)?;
        let elapsed = started.elapsed().as_millis() as u64;

        let rows = values
            .into_iter()
            .map(|value| {
                json!({
                    "value": value,
                    "name": presentation::field_value_name(&field, &value).unwrap_or_else(|| value.clone()),
                })
            })
            .collect::<Vec<_>>();
        let value_count = rows.len() as u64;

        Ok(FlowAutocompleteQueryOutput {
            agent_id: self.agent_id.clone(),
            field,
            term,
            values: rows,
            stats: HashMap::from([
                ("query_facet_autocomplete_ms".to_string(), elapsed),
                ("query_facet_autocomplete_values".to_string(), value_count),
            ]),
            warnings: None,
        })
    }
}
