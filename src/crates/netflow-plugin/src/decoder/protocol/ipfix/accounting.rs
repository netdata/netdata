use super::super::*;

pub(crate) fn account_ipfix_flowset(body: &IPFixFlowSetBody, stats: &mut DecodeStats) {
    match body {
        IPFixFlowSetBody::Template(_) | IPFixFlowSetBody::V9Template(_) => {
            stats.ipfix_template_sets += 1;
            stats.ipfix_data_templates += 1;
        }
        IPFixFlowSetBody::Templates(templates) => {
            stats.ipfix_template_sets += 1;
            stats.ipfix_data_templates += templates.len() as u64;
        }
        IPFixFlowSetBody::V9Templates(templates) => {
            stats.ipfix_template_sets += 1;
            stats.ipfix_data_templates += templates.len() as u64;
        }
        IPFixFlowSetBody::OptionsTemplate(_) | IPFixFlowSetBody::V9OptionsTemplate(_) => {
            stats.ipfix_options_template_sets += 1;
            stats.ipfix_options_templates += 1;
        }
        IPFixFlowSetBody::OptionsTemplates(templates) => {
            stats.ipfix_options_template_sets += 1;
            stats.ipfix_options_templates += templates.len() as u64;
        }
        IPFixFlowSetBody::V9OptionsTemplates(templates) => {
            stats.ipfix_options_template_sets += 1;
            stats.ipfix_options_templates += templates.len() as u64;
        }
        IPFixFlowSetBody::Data(data) => {
            stats.ipfix_data_sets += 1;
            stats.ipfix_records += data.fields.len() as u64;
        }
        IPFixFlowSetBody::V9Data(data) => {
            stats.ipfix_data_sets += 1;
            stats.ipfix_records += data.fields.len() as u64;
            stats.unsupported_data_sets += 1;
        }
        IPFixFlowSetBody::OptionsData(data) => {
            stats.ipfix_options_data_sets += 1;
            stats.ipfix_options_records += data.fields.len() as u64;
        }
        IPFixFlowSetBody::V9OptionsData(data) => {
            stats.ipfix_options_data_sets += 1;
            stats.ipfix_options_records += data.fields.len() as u64;
        }
        IPFixFlowSetBody::NoTemplate(_) => {
            stats.ipfix_missing_template_sets += 1;
            stats.missing_template_sets += 1;
        }
        IPFixFlowSetBody::Empty => stats.ipfix_ignored_sets += 1,
    }
}

pub(crate) fn account_ipfix_packet(packet: &IPFix, stats: &mut DecodeStats) {
    for flowset in &packet.flowsets {
        account_ipfix_flowset(&flowset.body, stats);
    }
}
