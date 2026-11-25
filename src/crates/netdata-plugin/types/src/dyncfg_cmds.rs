#![allow(dead_code)]
//! The dyncfg command vocabulary: [`DynCfgCmds`], the bitflag set naming the
//! operations a dynamic-configuration entry supports. It is the `cmds` field
//! of [`ConfigDeclaration`](crate::ConfigDeclaration), emitted as one word of
//! the pluginsd CONFIG line.
//!
//! Wire form: lowercase names, pipe-separated ("get | schema") — what
//! netdata-plugin-protocol's CONFIG encoder puts in the `cmds` word (run
//! through the encoder's `quote_if_needed`, which quotes it when it contains
//! spaces). The agent parses that word with
//! `dyncfg_cmds2id` (src/libnetdata/inicfg/dyncfg.c), which splits on spaces
//! and silently ignores the "|" tokens, so it reads both the Rust
//! pipe-separated form and the C agent's own space-separated form
//! (`dyncfg_cmds2buffer`) identically. The bit layout mirrors the agent's
//! `DYNCFG_CMDS` enum (src/libnetdata/inicfg/dyncfg.h) bit-for-bit, so the
//! u32 conversions interoperate.
//!
//! Registration caveat, outside this type: the CONFIG encoder sends the
//! action word uppercase ("CREATE"), while the agent matches it
//! case-sensitively against lowercase "create" (pluginsd_config,
//! src/plugins.d/pluginsd_dyncfg.c), so those declarations are logged as an
//! unknown action and never registered — see the protocol crate's encoder
//! (tokio_codec).

use bitflags::bitflags;
use std::fmt;

bitflags! {
    /// Rendered on the wire by `Display` / [`Self::to_pipe_separated`]; the
    /// wire contract is in the module docs.
    #[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Hash)]
    pub struct DynCfgCmds: u32 {
        const GET = 1 << 0;
        const SCHEMA = 1 << 1;
        const UPDATE = 1 << 2;
        const ADD = 1 << 3;
        const TEST = 1 << 4;
        const REMOVE = 1 << 5;
        const ENABLE = 1 << 6;
        const DISABLE = 1 << 7;
        const RESTART = 1 << 8;
        const USERCONFIG = 1 << 9;
    }
}

impl DynCfgCmds {
    /// Wire name of a single command flag, lowercase as the agent's `cmd_map`
    /// spells it (src/libnetdata/inicfg/dyncfg.c). `None` unless exactly that
    /// one flag is set — multi-flag sets and the empty set have no single name.
    pub fn flag_name(flag: Self) -> Option<&'static str> {
        match flag {
            Self::GET => Some("get"),
            Self::SCHEMA => Some("schema"),
            Self::UPDATE => Some("update"),
            Self::ADD => Some("add"),
            Self::TEST => Some("test"),
            Self::REMOVE => Some("remove"),
            Self::ENABLE => Some("enable"),
            Self::DISABLE => Some("disable"),
            Self::RESTART => Some("restart"),
            Self::USERCONFIG => Some("userconfig"),
            _ => None,
        }
    }

    /// Render space-separated ("get schema restart"), the form the C agent
    /// uses for the `cmds` word of its own CONFIG lines
    /// (`dyncfg_cmds2buffer`). Names come out in the fixed flag order of the
    /// array below, so the output for a given set is deterministic; the
    /// empty set renders as "".
    pub fn to_space_separated(&self) -> String {
        if self.is_empty() {
            return String::new();
        }

        let mut parts = Vec::new();

        for &flag in &[
            Self::GET,
            Self::SCHEMA,
            Self::UPDATE,
            Self::ADD,
            Self::TEST,
            Self::REMOVE,
            Self::ENABLE,
            Self::DISABLE,
            Self::RESTART,
            Self::USERCONFIG,
        ] {
            if self.contains(flag) {
                if let Some(name) = Self::flag_name(flag) {
                    parts.push(name);
                }
            }
        }

        parts.join(" ")
    }

    /// Render pipe-separated ("get | schema") — the form [`std::fmt::Display`]
    /// prints. Names come out in the fixed flag order of the array below,
    /// like [`Self::to_space_separated`]; the empty set renders as "".
    pub fn to_pipe_separated(&self) -> String {
        if self.is_empty() {
            return String::new();
        }

        let mut parts = Vec::new();

        for &flag in &[
            Self::GET,
            Self::SCHEMA,
            Self::UPDATE,
            Self::ADD,
            Self::TEST,
            Self::REMOVE,
            Self::ENABLE,
            Self::DISABLE,
            Self::RESTART,
            Self::USERCONFIG,
        ] {
            if self.contains(flag) {
                if let Some(name) = Self::flag_name(flag) {
                    parts.push(name);
                }
            }
        }

        parts.join(" | ")
    }
}

/// Prints pipe-separated — the wire `cmds` word (see the module docs).
impl fmt::Display for DynCfgCmds {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.to_pipe_separated())
    }
}

/// Build from raw bits; unknown bits are silently dropped by
/// `from_bits_truncate`.
impl From<u32> for DynCfgCmds {
    fn from(value: u32) -> Self {
        Self::from_bits_truncate(value)
    }
}

impl From<DynCfgCmds> for u32 {
    fn from(cmds: DynCfgCmds) -> Self {
        cmds.bits()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_flag_name() {
        assert_eq!(DynCfgCmds::flag_name(DynCfgCmds::GET), Some("get"));
        assert_eq!(DynCfgCmds::flag_name(DynCfgCmds::SCHEMA), Some("schema"));
        assert_eq!(
            DynCfgCmds::flag_name(DynCfgCmds::USERCONFIG),
            Some("userconfig")
        );
        assert_eq!(
            DynCfgCmds::flag_name(DynCfgCmds::GET | DynCfgCmds::SCHEMA),
            None
        );
    }

    #[test]
    fn test_to_space_separated() {
        let cmds = DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART;
        assert_eq!(cmds.to_space_separated(), "get schema restart");

        assert_eq!(DynCfgCmds::empty().to_space_separated(), "");
        assert_eq!(DynCfgCmds::GET.to_space_separated(), "get");
    }

    #[test]
    fn test_to_pipe_separated() {
        let cmds = DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART;
        assert_eq!(cmds.to_pipe_separated(), "get | schema | restart");

        assert_eq!(DynCfgCmds::empty().to_pipe_separated(), "");
        assert_eq!(DynCfgCmds::GET.to_pipe_separated(), "get");
    }

    #[test]
    fn test_display() {
        let cmds = DynCfgCmds::GET | DynCfgCmds::SCHEMA;
        assert_eq!(format!("{}", cmds), "get | schema");
    }

    #[test]
    fn test_u32_conversion() {
        let cmds = DynCfgCmds::GET | DynCfgCmds::SCHEMA; // bits 0 and 1 = 3
        let value: u32 = cmds.into();
        assert_eq!(value, 3);

        let cmds_from_u32: DynCfgCmds = 3u32.into();
        assert_eq!(cmds_from_u32, cmds);
    }

    #[test]
    fn test_bitflags_operations() {
        let cmds = DynCfgCmds::GET | DynCfgCmds::SCHEMA;

        assert!(cmds.contains(DynCfgCmds::GET));
        assert!(cmds.contains(DynCfgCmds::SCHEMA));
        assert!(!cmds.contains(DynCfgCmds::UPDATE));

        assert!(cmds.intersects(DynCfgCmds::GET | DynCfgCmds::UPDATE));
        assert!(!cmds.intersects(DynCfgCmds::UPDATE | DynCfgCmds::ADD));
    }
}
