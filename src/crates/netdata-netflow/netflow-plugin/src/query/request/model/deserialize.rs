use super::super::*;
use super::{FlowsRequest, RawFlowsRequest};
use crate::facet_catalog::facet_field_enabled;

impl<'de> Deserialize<'de> for FlowsRequest {
    fn deserialize<D>(deserializer: D) -> std::result::Result<Self, D::Error>
    where
        D: Deserializer<'de>,
    {
        let mut raw = RawFlowsRequest::deserialize(deserializer)?;
        let view = match raw.view {
            Some(view) => {
                raw.selections.remove("VIEW");
                view
            }
            None => take_selection_view(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_default(),
        };
        let group_by = match raw.group_by {
            Some(group_by) => {
                raw.selections.remove("GROUP_BY");
                group_by
            }
            None => take_selection_group_by(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_else(default_group_by),
        };
        let sort_by = match raw.sort_by {
            Some(sort_by) => {
                raw.selections.remove("SORT_BY");
                sort_by
            }
            None => take_selection_sort_by(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_default(),
        };
        let top_n = match raw.top_n {
            Some(top_n) => {
                raw.selections.remove("TOP_N");
                top_n
            }
            None => take_selection_top_n(&mut raw.selections)
                .transpose()
                .map_err(D::Error::custom)?
                .unwrap_or_default(),
        };
        let mode = raw.mode.unwrap_or_default();
        let field = raw.field.as_mut().and_then(|field| {
            let normalized = field.trim().to_ascii_uppercase();
            (!normalized.is_empty()).then_some(normalized)
        });

        if matches!(mode, super::RequestMode::Autocomplete) {
            let Some(field) = field.as_deref() else {
                return Err(D::Error::custom(
                    "autocomplete mode requires a supported `field` parameter",
                ));
            };
            if !facet_field_enabled(field) {
                return Err(D::Error::custom(format!(
                    "unsupported autocomplete field `{field}`"
                )));
            }
        }

        validate_selection_fields(&raw.selections).map_err(D::Error::custom)?;

        Ok(Self {
            mode,
            view,
            after: raw.after,
            before: raw.before,
            query: raw.query,
            selections: raw.selections,
            facets: raw.facets,
            group_by,
            sort_by,
            top_n,
            field,
            term: raw.term.trim().to_string(),
        })
    }
}
