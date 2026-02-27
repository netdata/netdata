#![allow(dead_code)]

use bitflags::bitflags;
use std::fmt;
use std::str::FromStr;

bitflags! {
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
    /// Get the string name for a single command flag
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

    /// Parse a single command name to its flag
    pub fn from_cmd_name(name: &str) -> Option<Self> {
        match name.trim() {
            "get" => Some(Self::GET),
            "schema" => Some(Self::SCHEMA),
            "update" => Some(Self::UPDATE),
            "add" => Some(Self::ADD),
            "test" => Some(Self::TEST),
            "remove" => Some(Self::REMOVE),
            "enable" => Some(Self::ENABLE),
            "disable" => Some(Self::DISABLE),
            "restart" => Some(Self::RESTART),
            "userconfig" => Some(Self::USERCONFIG),
            _ => None,
        }
    }

    /// Parse from string with space or pipe-separated command names
    /// Examples: "get | schema | restart", "get schema restart", "get|schema|restart"
    pub fn from_str_multi(s: &str) -> Option<Self> {
        let s = s.trim();
        if s.is_empty() {
            return Some(Self::empty());
        }

        let mut result = Self::empty();

        // Split by both spaces and pipes, then filter out empty parts
        for part in s.split([' ', '|']) {
            let part = part.trim();
            if part.is_empty() {
                continue;
            }

            match Self::from_cmd_name(part) {
                Some(flag) => result |= flag,
                None => return None, // Invalid command name
            }
        }

        Some(result)
    }

    /// Parse from byte slice
    pub fn from_slice(bytes: &[u8]) -> Option<Self> {
        let s = std::str::from_utf8(bytes).ok()?;
        Self::from_str_multi(s)
    }

    /// Convert to a space-separated string representation
    pub fn to_space_separated(&self) -> String {
        if self.is_empty() {
            return String::new();
        }

        let mut parts = Vec::new();

        // Check each flag in order
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

    /// Convert to a pipe-separated string representation
    pub fn to_pipe_separated(&self) -> String {
        if self.is_empty() {
            return String::new();
        }

        let mut parts = Vec::new();

        // Check each flag in order
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

impl fmt::Display for DynCfgCmds {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.to_pipe_separated())
    }
}

impl FromStr for DynCfgCmds {
    type Err = ();

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        Self::from_str_multi(s).ok_or(())
    }
}

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
    fn test_from_cmd_name() {
        assert_eq!(DynCfgCmds::from_cmd_name("get"), Some(DynCfgCmds::GET));
        assert_eq!(
            DynCfgCmds::from_cmd_name("schema"),
            Some(DynCfgCmds::SCHEMA)
        );
        assert_eq!(
            DynCfgCmds::from_cmd_name("userconfig"),
            Some(DynCfgCmds::USERCONFIG)
        );
        assert_eq!(
            DynCfgCmds::from_cmd_name("  restart  "),
            Some(DynCfgCmds::RESTART)
        );
        assert_eq!(DynCfgCmds::from_cmd_name("invalid"), None);
    }

    #[test]
    fn test_from_str_multi_pipe_separated() {
        let result = DynCfgCmds::from_str_multi("get | schema | restart").unwrap();
        assert_eq!(
            result,
            DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART
        );

        let result = DynCfgCmds::from_str_multi("get|schema|restart").unwrap();
        assert_eq!(
            result,
            DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART
        );
    }

    #[test]
    fn test_from_str_multi_space_separated() {
        let result = DynCfgCmds::from_str_multi("get schema restart").unwrap();
        assert_eq!(
            result,
            DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART
        );

        let result = DynCfgCmds::from_str_multi("  get   schema   restart  ").unwrap();
        assert_eq!(
            result,
            DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART
        );
    }

    #[test]
    fn test_from_str_multi_mixed_separators() {
        let result = DynCfgCmds::from_str_multi("get | schema restart").unwrap();
        assert_eq!(
            result,
            DynCfgCmds::GET | DynCfgCmds::SCHEMA | DynCfgCmds::RESTART
        );
    }

    #[test]
    fn test_from_str_multi_single_command() {
        let result = DynCfgCmds::from_str_multi("get").unwrap();
        assert_eq!(result, DynCfgCmds::GET);
    }

    #[test]
    fn test_from_str_multi_empty() {
        let result = DynCfgCmds::from_str_multi("").unwrap();
        assert_eq!(result, DynCfgCmds::empty());

        let result = DynCfgCmds::from_str_multi("   ").unwrap();
        assert_eq!(result, DynCfgCmds::empty());
    }

    #[test]
    fn test_from_str_multi_invalid() {
        assert_eq!(DynCfgCmds::from_str_multi("get | invalid | schema"), None);
        assert_eq!(DynCfgCmds::from_str_multi("invalid"), None);
    }

    #[test]
    fn test_from_slice() {
        let result = DynCfgCmds::from_slice(b"get | schema").unwrap();
        assert_eq!(result, DynCfgCmds::GET | DynCfgCmds::SCHEMA);

        assert_eq!(DynCfgCmds::from_slice(&[0xFF, 0xFE]), None); // Invalid UTF-8
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
    fn test_from_str_trait() {
        let cmds: DynCfgCmds = "get | schema".parse().unwrap();
        assert_eq!(cmds, DynCfgCmds::GET | DynCfgCmds::SCHEMA);

        assert!("invalid".parse::<DynCfgCmds>().is_err());
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
