use crate::{presentation, query};

use super::model::{RequiredParam, RequiredParamOption};

const FLOWS_ACCEPTED_PARAMS: &[&str] = &[
    "mode",
    "view",
    "after",
    "before",
    "query",
    "term",
    "field",
    "selections",
    "facets",
    "group_by",
    "sort_by",
    "top_n",
];

pub(crate) fn flows_required_params(
    view: &str,
    group_by: &[String],
    sort_by: query::SortBy,
    top_n: usize,
) -> Vec<RequiredParam> {
    let ordered_group_by_options = ordered_group_by_options(group_by);
    let mut params = vec![RequiredParam {
        id: "view".to_string(),
        name: "View".to_string(),
        kind: "select".to_string(),
        options: vec![
            RequiredParamOption {
                id: "table-sankey".to_string(),
                name: "Table / Sankey".to_string(),
                default_selected: view == "table-sankey",
            },
            RequiredParamOption {
                id: "timeseries".to_string(),
                name: "Time-Series".to_string(),
                default_selected: view == "timeseries",
            },
            RequiredParamOption {
                id: "country-map".to_string(),
                name: "Country-Map".to_string(),
                default_selected: view == "country-map",
            },
            RequiredParamOption {
                id: "state-map".to_string(),
                name: "State-Map".to_string(),
                default_selected: view == "state-map",
            },
            RequiredParamOption {
                id: "city-map".to_string(),
                name: "City-Map".to_string(),
                default_selected: view == "city-map",
            },
        ],
        help: "Select the flow view to render.".to_string(),
    }];

    if !matches!(view, "country-map" | "state-map" | "city-map") {
        params.push(RequiredParam {
            id: "group_by".to_string(),
            name: "Group By".to_string(),
            kind: "multiselect".to_string(),
            options: ordered_group_by_options
                .iter()
                .map(|field| RequiredParamOption {
                    id: field.clone(),
                    name: presentation::field_display_name(field),
                    default_selected: group_by.iter().any(|selected| selected == field),
                })
                .collect(),
            help: "Select up to 10 tuple fields used to group and rank flows.".to_string(),
        });
    }

    params.push(RequiredParam {
        id: "sort_by".to_string(),
        name: "Sort By".to_string(),
        kind: "select".to_string(),
        options: vec![
            RequiredParamOption {
                id: "bytes".to_string(),
                name: "Bytes".to_string(),
                default_selected: sort_by == query::SortBy::Bytes,
            },
            RequiredParamOption {
                id: "packets".to_string(),
                name: "Packets".to_string(),
                default_selected: sort_by == query::SortBy::Packets,
            },
        ],
        help: "Choose the metric used to rank top groups and the other bucket.".to_string(),
    });
    params.push(RequiredParam {
        id: "top_n".to_string(),
        name: "Top N".to_string(),
        kind: "select".to_string(),
        options: vec![
            RequiredParamOption {
                id: "25".to_string(),
                name: "25".to_string(),
                default_selected: top_n == 25,
            },
            RequiredParamOption {
                id: "50".to_string(),
                name: "50".to_string(),
                default_selected: top_n == 50,
            },
            RequiredParamOption {
                id: "100".to_string(),
                name: "100".to_string(),
                default_selected: top_n == 100,
            },
            RequiredParamOption {
                id: "200".to_string(),
                name: "200".to_string(),
                default_selected: top_n == 200,
            },
            RequiredParamOption {
                id: "500".to_string(),
                name: "500".to_string(),
                default_selected: top_n == 500,
            },
        ],
        help: "Choose how many grouped tuples the backend returns.".to_string(),
    });

    params
}

fn ordered_group_by_options(group_by: &[String]) -> Vec<String> {
    let supported = query::supported_group_by_fields();
    let mut ordered = Vec::with_capacity(supported.len());

    for selected in group_by {
        if supported.iter().any(|field| field == selected)
            && !ordered.iter().any(|field| field == selected)
        {
            ordered.push(selected.clone());
        }
    }

    for field in supported {
        if !ordered.iter().any(|selected| selected == field) {
            ordered.push(field.clone());
        }
    }

    ordered
}

pub(super) fn accepted_params() -> &'static [&'static str] {
    FLOWS_ACCEPTED_PARAMS
}
