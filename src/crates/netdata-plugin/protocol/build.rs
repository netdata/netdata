use std::env;
use std::fs::File;
use std::io::{BufWriter, Write};
use std::path::Path;

fn main() {
    let path = Path::new(&env::var("OUT_DIR").unwrap()).join("tokens.rs");
    let mut file = BufWriter::new(File::create(&path).unwrap());

    writeln!(
        &mut file,
        "static COMMAND_MAP: phf::Map<&'static [u8], Token> = {};",
        phf_codegen::Map::<&[u8]>::new()
            .entry(b"CHART", "Token::Chart")
            .entry(b"CHART_DEFINITION_END", "Token::ChartDefinitionEnd")
            .entry(b"DIMENSION", "Token::Dimension")
            .entry(b"BEGIN", "Token::Begin")
            .entry(b"END", "Token::End")
            .entry(b"SET", "Token::Set")
            .entry(b"FLUSH", "Token::Flush")
            .entry(b"DISABLE", "Token::Disable")
            .entry(b"VARIABLE", "Token::Variable")
            .entry(b"LABEL", "Token::Label")
            .entry(b"OVERWRITE", "Token::Overwrite")
            .entry(b"CLABEL", "Token::Clabel")
            .entry(b"CLABEL_COMMIT", "Token::ClabelCommit")
            .entry(b"EXIT", "Token::Exit")
            .entry(b"BEGIN2", "Token::Begin2")
            .entry(b"SET2", "Token::Set2")
            .entry(b"END2", "Token::End2")
            .entry(b"HOST_DEFINE", "Token::HostDefine")
            .entry(b"HOST_DEFINE_END", "Token::HostDefineEnd")
            .entry(b"HOST_LABEL", "Token::HostLabel")
            .entry(b"HOST", "Token::Host")
            .entry(b"REPLAY_CHART", "Token::ReplayChart")
            .entry(b"RBEGIN", "Token::Rbegin")
            .entry(b"RSET", "Token::Rset")
            .entry(b"RDSTATE", "Token::RdState")
            .entry(b"RSSTATE", "Token::RsState")
            .entry(b"REND", "Token::Rend")
            .entry(b"FUNCTION", "Token::Function")
            .entry(b"FUNCTION_RESULT_BEGIN", "Token::FunctionResultBegin")
            .entry(b"FUNCTION_RESULT_END", "Token::FunctionResultEnd")
            .entry(b"FUNCTION_PAYLOAD", "Token::FunctionPayloadBegin")
            .entry(b"FUNCTION_PAYLOAD_END", "Token::FunctionPayloadEnd")
            .entry(b"FUNCTION_CANCEL", "Token::FunctionCancel")
            .entry(b"FUNCTION_PROGRESS", "Token::FunctionProgress")
            .entry(b"QUIT", "Token::Quit")
            .entry(b"CONFIG", "Token::Config")
            .entry(b"NODE_ID", "Token::NodeId")
            .entry(b"CLAIMED_ID", "Token::ClaimedId")
            .entry(b"JSON", "Token::Json")
            .entry(b"JSON_PAYLOAD_END", "Token::JsonPayloadEnd")
            .entry(b"STREAM_PATH", "Token::StreamPath")
            .entry(b"ML_MODEL", "Token::MlModel")
            .entry(b"TRUST_DURATIONS", "Token::TrustDurations")
            .entry(b"DYNCFG_ENABLE", "Token::DynCfg")
            .entry(b"DYNCFG_REGISTER_MODULE", "Token::DynCfgRegisterModule")
            .entry(b"DYNCFG_REGISTER_JOB", "Token::DynCfgRegisterJob")
            .entry(b"DYNCFG_RESET", "Token::DynCfgReset")
            .entry(b"REPORT_JOB_STATUS", "Token::ReportJobStatus")
            .entry(b"DELETE_JOB", "Token::DeleteJob")
            .build()
    )
    .unwrap();
}
