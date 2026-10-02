// Build script for the protocol crate: writes `OUT_DIR/tokens.rs`, whose
// `COMMAND_MAP` static is the pluginsd keyword table that `src/line_parser.rs`
// includes at compile time (`include!(concat!(env!("OUT_DIR"), "/tokens.rs"))`).
//
// `COMMAND_MAP` maps every pluginsd wire keyword — the first word of a line,
// looked up byte-exact — to the `Token` variant `parse_normal_line` dispatches
// on, so keyword resolution is a single phf lookup.
//
// The table spans both wire directions. It mirrors the pluginsd keyword
// vocabulary the agent side maintains: the wire words in
// `src/libnetdata/functions_evloop/functions_evloop.h` (`PLUGINSD_KEYWORD_*` /
// `PLUGINSD_CALL_*`) and the agent's parse table
// `src/plugins.d/gperf-hashtable.h`. It is not an exact mirror of the parse
// table: the `PLUGIN_KEEPALIVE` and `FUNCTION_DEL` lines the agent's parser
// accepts are not listed here.
//
// Sync contract with the `Token` enum in line_parser.rs: exactly one entry per
// variant, in enum-declaration order, both sides maintained by hand. Entry
// keys and values are rendered verbatim into the generated file, so a variant
// renamed without updating its entry here fails to compile in line_parser.rs.
//
// Mapping a keyword does not make the decoder understand it:
// `parse_normal_line` converts only the tokens it matches — the chart and
// function-protocol subsets — to `Command`s; any other mapped token hits its
// `panic!` arm when that keyword arrives, and a keyword missing from this
// table parses as `Command::Unknown` and is dropped by the message layer. A
// `Token` variant added without an entry therefore leaves its keyword
// silently unrecognized.
//
// No `cargo:rerun-if-*` directives are emitted, so cargo falls back to
// rerunning this script whenever any file in the package changes.
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
