//! The dyncfg source-type vocabulary: where a configuration entry's current
//! value came from. A word list shared across the wire, mirrored by the
//! agent's `DYNCFG_SOURCE_TYPE` enum and name table
//! (src/libnetdata/inicfg/dyncfg.h, dyncfg.c) and by the go.d framework's
//! confgroup `Type*` constants (src/go/plugin/framework/confgroup/config.go).
//!
//! Rendering:
//! - pluginsd wire: the `<source_type>` word of
//!   `CONFIG <id> CREATE <status> <type> <path> <source_type> <source>
//!   <cmds> <view_access> <edit_access>`, printed lowercase via `Display` by
//!   the netdata-plugin-protocol encoder and parsed by pluginsd_config
//!   (src/plugins.d/pluginsd_dyncfg.c) through `dyncfg_source_type2id`.
//!
//! `Default` has no callers outside this file's tests; the file-level
//! allow(dead_code) keeps it, as in the sibling dyncfg_* vocabulary modules.
#![allow(dead_code)]

use std::fmt;

/// Where a dyncfg entry's current value came from.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DynCfgSourceType {
    /// Configuration defined within Netdata itself, not from a config file
    /// or a dyncfg edit.
    Internal,
    /// Value from Netdata's stock (shipped) configuration files.
    Stock,
    /// Value from user-provided configuration files.
    User,
    /// Created or edited through DynCfg itself. The agent replays these
    /// saved values to the plugin on registration and only allows REMOVE
    /// for dyncfg-sourced jobs (src/daemon/dyncfg/dyncfg.c).
    Dyncfg,
    /// Jobs produced by service discovery (go.d confgroup
    /// `TypeDiscovered`); part of the vocabulary, but no daemon code
    /// assigns it today.
    Discovered,
}

impl DynCfgSourceType {
    /// The canonical lowercase vocabulary word for this value: what
    /// `Display` prints into the `CONFIG` line.
    pub fn name(&self) -> &'static str {
        match self {
            Self::Internal => "internal",
            Self::Stock => "stock",
            Self::User => "user",
            Self::Dyncfg => "dyncfg",
            Self::Discovered => "discovered",
        }
    }
}

impl Default for DynCfgSourceType {
    /// The agent's fallback: dyncfg_source_type2id maps missing or unknown
    /// words to DYNCFG_SOURCE_TYPE_INTERNAL.
    fn default() -> Self {
        Self::Internal
    }
}

impl fmt::Display for DynCfgSourceType {
    /// Prints the vocabulary word (name()) — the form embedded in the
    /// pluginsd `CONFIG` line.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_name() {
        assert_eq!(DynCfgSourceType::Internal.name(), "internal");
        assert_eq!(DynCfgSourceType::Stock.name(), "stock");
        assert_eq!(DynCfgSourceType::User.name(), "user");
        assert_eq!(DynCfgSourceType::Dyncfg.name(), "dyncfg");
        assert_eq!(DynCfgSourceType::Discovered.name(), "discovered");
    }

    #[test]
    fn test_display() {
        assert_eq!(format!("{}", DynCfgSourceType::Internal), "internal");
        assert_eq!(format!("{}", DynCfgSourceType::Stock), "stock");
        assert_eq!(format!("{}", DynCfgSourceType::User), "user");
        assert_eq!(format!("{}", DynCfgSourceType::Dyncfg), "dyncfg");
        assert_eq!(format!("{}", DynCfgSourceType::Discovered), "discovered");
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgSourceType::default(), DynCfgSourceType::Internal);
    }
}
