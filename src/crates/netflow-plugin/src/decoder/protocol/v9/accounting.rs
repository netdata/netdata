use super::super::*;

pub(crate) fn account_v9_flowset(body: &V9FlowSetBody, stats: &mut DecodeStats) {
    match body {
        V9FlowSetBody::Template(templates) => {
            stats.v9_template_sets += 1;
            stats.v9_data_templates += templates.templates.len() as u64;
        }
        V9FlowSetBody::OptionsTemplate(templates) => {
            stats.v9_options_template_sets += 1;
            stats.v9_options_templates += templates.templates.len() as u64;
        }
        V9FlowSetBody::Data(data) => {
            stats.v9_data_sets += 1;
            stats.netflow_v9_records += data.fields.len() as u64;
        }
        V9FlowSetBody::OptionsData(data) => {
            stats.v9_options_data_sets += 1;
            stats.v9_options_records += data.fields.len() as u64;
        }
        V9FlowSetBody::NoTemplate(_) => {
            stats.v9_missing_template_sets += 1;
            stats.missing_template_sets += 1;
        }
        V9FlowSetBody::Empty => stats.v9_ignored_sets += 1,
    }
}

pub(crate) fn account_v9_packet(packet: &V9, stats: &mut DecodeStats) {
    for flowset in &packet.flowsets {
        account_v9_flowset(&flowset.body, stats);
    }
}
