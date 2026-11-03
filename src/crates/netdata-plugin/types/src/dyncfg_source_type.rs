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
//! - `configDeclaration` JSON: the `sourceType` string, read by
//!   types::config and emitted by netdata-plugin-schema, which also accepts
//!   it as the `x-config-source-type` schema attribute.
//!
//! Parsing is strict here (`from_name` rejects unknown words), while the
//! agent's C parser defaults unknown or empty words to `internal` — a
//! misspelled word passes the agent but fails schema/JSON parsing.
//!
//! `from_slice`, `Default` and `FromStr` have no callers outside this
//! file's tests; the file-level allow(dead_code) keeps them, as in the
//! sibling dyncfg_* vocabulary modules.
#![allow(dead_code)]

use std::fmt;
use std::str::FromStr;

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
    /// `Display` prints into the `CONFIG` line and what
    /// netdata-plugin-schema writes into the `sourceType` field of a
    /// `configDeclaration`.
    pub fn name(&self) -> &'static str {
        match self {
            Self::Internal => "internal",
            Self::Stock => "stock",
            Self::User => "user",
            Self::Dyncfg => "dyncfg",
            Self::Discovered => "discovered",
        }
    }

    /// Strict inverse of `name()`: exact lowercase match, no trimming, None
    /// for unknown words. Used by the `configDeclaration` parser in
    /// types::config and by netdata-plugin-schema's `x-config-*` attribute
    /// reader.
    pub fn from_name(name: &str) -> Option<Self> {
        match name {
            "internal" => Some(Self::Internal),
            "stock" => Some(Self::Stock),
            "user" => Some(Self::User),
            "dyncfg" => Some(Self::Dyncfg),
            "discovered" => Some(Self::Discovered),
            _ => None,
        }
    }

    /// `from_name` over UTF-8 bytes, trimming surrounding whitespace first;
    /// None for invalid UTF-8 or unknown words.
    pub fn from_slice(bytes: &[u8]) -> Option<Self> {
        let s = std::str::from_utf8(bytes).ok()?.trim();
        Self::from_name(s)
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

impl FromStr for DynCfgSourceType {
    type Err = ();

    /// `str::parse` support, delegating to `from_name`; failures carry no
    /// detail (`Err(())`).
    fn from_str(s: &str) -> Result<Self, Self::Err> {
        Self::from_name(s).ok_or(())
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
    fn test_from_name() {
        assert_eq!(
            DynCfgSourceType::from_name("internal"),
            Some(DynCfgSourceType::Internal)
        );
        assert_eq!(
            DynCfgSourceType::from_name("stock"),
            Some(DynCfgSourceType::Stock)
        );
        assert_eq!(
            DynCfgSourceType::from_name("user"),
            Some(DynCfgSourceType::User)
        );
        assert_eq!(
            DynCfgSourceType::from_name("dyncfg"),
            Some(DynCfgSourceType::Dyncfg)
        );
        assert_eq!(
            DynCfgSourceType::from_name("discovered"),
            Some(DynCfgSourceType::Discovered)
        );
        assert_eq!(DynCfgSourceType::from_name("invalid"), None);
    }

    #[test]
    fn test_from_slice() {
        assert_eq!(
            DynCfgSourceType::from_slice(b"internal"),
            Some(DynCfgSourceType::Internal)
        );
        assert_eq!(
            DynCfgSourceType::from_slice(b"  stock  "),
            Some(DynCfgSourceType::Stock)
        );
        assert_eq!(
            DynCfgSourceType::from_slice(b"user"),
            Some(DynCfgSourceType::User)
        );
        assert_eq!(
            DynCfgSourceType::from_slice(b"dyncfg"),
            Some(DynCfgSourceType::Dyncfg)
        );
        assert_eq!(
            DynCfgSourceType::from_slice(b"discovered"),
            Some(DynCfgSourceType::Discovered)
        );
        assert_eq!(DynCfgSourceType::from_slice(b"invalid"), None);
        assert_eq!(DynCfgSourceType::from_slice(&[0xFF, 0xFE]), None); // Invalid UTF-8
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
    fn test_from_str() {
        assert_eq!(
            "internal".parse::<DynCfgSourceType>(),
            Ok(DynCfgSourceType::Internal)
        );
        assert_eq!(
            "stock".parse::<DynCfgSourceType>(),
            Ok(DynCfgSourceType::Stock)
        );
        assert_eq!(
            "user".parse::<DynCfgSourceType>(),
            Ok(DynCfgSourceType::User)
        );
        assert_eq!(
            "dyncfg".parse::<DynCfgSourceType>(),
            Ok(DynCfgSourceType::Dyncfg)
        );
        assert_eq!(
            "discovered".parse::<DynCfgSourceType>(),
            Ok(DynCfgSourceType::Discovered)
        );
        assert_eq!("invalid".parse::<DynCfgSourceType>(), Err(()));
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgSourceType::default(), DynCfgSourceType::Internal);
    }
}
