use super::super::*;
use super::{FlowsRequest, RequestMode, SortBy, ViewMode};

impl FlowsRequest {
    pub(crate) fn is_autocomplete_mode(&self) -> bool {
        matches!(self.mode, RequestMode::Autocomplete)
    }

    pub(crate) fn normalized_view(&self) -> &'static str {
        match self.view {
            ViewMode::TableSankey => "table-sankey",
            ViewMode::TimeSeries => "timeseries",
            ViewMode::CountryMap => "country-map",
            ViewMode::StateMap => "state-map",
            ViewMode::CityMap => "city-map",
        }
    }

    pub(crate) fn is_timeseries_view(&self) -> bool {
        matches!(self.view, ViewMode::TimeSeries)
    }

    pub(crate) fn is_country_map_view(&self) -> bool {
        matches!(self.view, ViewMode::CountryMap)
    }

    pub(crate) fn is_state_map_view(&self) -> bool {
        matches!(self.view, ViewMode::StateMap)
    }

    pub(crate) fn is_city_map_view(&self) -> bool {
        matches!(self.view, ViewMode::CityMap)
    }

    pub(crate) fn normalized_sort_by(&self) -> SortBy {
        self.sort_by
    }

    pub(crate) fn normalized_group_by(&self) -> Vec<String> {
        self.group_by.clone()
    }

    pub(crate) fn normalized_facets(&self) -> Option<Vec<String>> {
        let raw = self.facets.as_ref()?;
        let mut out = Vec::new();
        let mut seen = HashSet::new();

        for field in raw {
            let normalized = field.trim().to_ascii_uppercase();
            if normalized.is_empty() || !facet_field_requested(normalized.as_str()) {
                continue;
            }
            if seen.insert(normalized.clone()) {
                out.push(normalized);
            }
        }

        (!out.is_empty()).then_some(out)
    }

    pub(crate) fn normalized_top_n(&self) -> usize {
        self.top_n.as_usize()
    }

    pub(crate) fn normalized_autocomplete_field(&self) -> Option<String> {
        self.field
            .as_ref()
            .map(|field| field.trim().to_ascii_uppercase())
            .filter(|field| !field.is_empty())
    }

    pub(crate) fn normalized_autocomplete_term(&self) -> &str {
        self.term.as_str()
    }
}
