//! The dyncfg type vocabulary: what kind of configuration entry a
//! registration declares. A word list shared across the wire, mirrored by
//! the agent's `DYNCFG_TYPE` enum and name table
//! (src/libnetdata/inicfg/dyncfg.h, dyncfg.c).
//!
//! Rendering:
//! - pluginsd wire: the `<type>` word of
//!   `CONFIG <id> CREATE <status> <type> <path> <source_type> <source>
//!   <cmds> <view_access> <edit_access>`, printed lowercase via `Display`
//!   by the netdata-plugin-protocol encoder and parsed by pluginsd_config
//!   (src/plugins.d/pluginsd_dyncfg.c) through `dyncfg_type2id`.
//! - `configDeclaration` JSON: the `type` string, read by types::config and
//!   emitted by netdata-plugin-schema, which also accepts it as the
//!   `x-config-type` schema attribute.
//!
//! Parsing is strict here (`from_name` rejects unknown words), while the
//! agent's C parser defaults unknown or empty words to `single` — a
//! misspelled word passes the agent but fails schema/JSON parsing.
//!
//! The value also gates agent-side registration behavior: the declared
//! command set is sanitized against type and source type, and job
//! registration requires a registered template — see the variant docs
//! (dyncfg_sanitize_cmds, dyncfg_add_low_level,
//! src/daemon/dyncfg/dyncfg.c).
//!
//! `from_slice`, `Default` and `FromStr` have no callers outside this
//! file's tests; the file-level allow(dead_code) keeps them, as in the
//! sibling dyncfg_* vocabulary modules.
#![allow(dead_code)]

use std::fmt;
use std::str::FromStr;

/// The kind of configuration entry a dyncfg registration declares.
/// Carried by [`ConfigDeclaration`](crate::ConfigDeclaration) as `type_`
/// and rendered on the wire by `Display` (the CONFIG-line and JSON
/// contracts are in the module docs).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DynCfgType {
    /// A standalone entry registered under its own id — no template behind
    /// it and no jobs hanging off it. Sanitization strips `add` and
    /// `remove` from its command set: `add` is template-only, `remove` is
    /// reserved for dyncfg-sourced jobs (dyncfg_sanitize_cmds,
    /// src/daemon/dyncfg/dyncfg.c).
    Single,
    /// A factory of jobs: registering a template is the precondition for
    /// registering jobs under it, with job ids of the form
    /// `<template-id>:<name>`. Its command set always carries `add` and
    /// never `get`/`update` (templates have no data of their own), and
    /// registering it replays the saved values of its dyncfg-sourced jobs
    /// (dyncfg_sanitize_cmds, dyncfg_send_updates,
    /// src/daemon/dyncfg/dyncfg.c).
    Template,
    /// One instance created from a template, with id
    /// `<template-id>:<name>`: registration is rejected unless a template
    /// is registered under the id's prefix before its last `:`
    /// (`dyncfg_job_has_registered_template`, src/daemon/dyncfg/dyncfg.c).
    /// `remove` is allowed only for dyncfg-sourced jobs
    /// (dyncfg_sanitize_cmds).
    Job,
}

impl DynCfgType {
    /// The canonical lowercase vocabulary word for this value: what
    /// `Display` prints into the `CONFIG` line and what
    /// netdata-plugin-schema writes into the `type` field of a
    /// `configDeclaration`.
    pub fn name(&self) -> &'static str {
        match self {
            Self::Single => "single",
            Self::Template => "template",
            Self::Job => "job",
        }
    }

    /// Strict inverse of `name()`: exact lowercase match, no trimming, None
    /// for unknown words. Used by the `configDeclaration` parser in
    /// types::config and by netdata-plugin-schema's `x-config-*` attribute
    /// reader.
    pub fn from_name(name: &str) -> Option<Self> {
        match name {
            "single" => Some(Self::Single),
            "template" => Some(Self::Template),
            "job" => Some(Self::Job),
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

impl Default for DynCfgType {
    /// The agent's fallback: dyncfg_type2id maps missing or unknown words
    /// to DYNCFG_TYPE_SINGLE (src/libnetdata/inicfg/dyncfg.c).
    fn default() -> Self {
        Self::Single
    }
}

impl fmt::Display for DynCfgType {
    /// Prints the vocabulary word (name()) — the form embedded in the
    /// pluginsd `CONFIG` line.
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

impl FromStr for DynCfgType {
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
        assert_eq!(DynCfgType::Single.name(), "single");
        assert_eq!(DynCfgType::Template.name(), "template");
        assert_eq!(DynCfgType::Job.name(), "job");
    }

    #[test]
    fn test_from_name() {
        assert_eq!(DynCfgType::from_name("single"), Some(DynCfgType::Single));
        assert_eq!(
            DynCfgType::from_name("template"),
            Some(DynCfgType::Template)
        );
        assert_eq!(DynCfgType::from_name("job"), Some(DynCfgType::Job));
        assert_eq!(DynCfgType::from_name("invalid"), None);
    }

    #[test]
    fn test_from_slice() {
        assert_eq!(DynCfgType::from_slice(b"single"), Some(DynCfgType::Single));
        assert_eq!(
            DynCfgType::from_slice(b"  template  "),
            Some(DynCfgType::Template)
        );
        assert_eq!(DynCfgType::from_slice(b"job"), Some(DynCfgType::Job));
        assert_eq!(DynCfgType::from_slice(b"invalid"), None);
        assert_eq!(DynCfgType::from_slice(&[0xFF, 0xFE]), None); // Invalid UTF-8
    }

    #[test]
    fn test_display() {
        assert_eq!(format!("{}", DynCfgType::Single), "single");
        assert_eq!(format!("{}", DynCfgType::Template), "template");
        assert_eq!(format!("{}", DynCfgType::Job), "job");
    }

    #[test]
    fn test_from_str() {
        assert_eq!("single".parse::<DynCfgType>(), Ok(DynCfgType::Single));
        assert_eq!("template".parse::<DynCfgType>(), Ok(DynCfgType::Template));
        assert_eq!("job".parse::<DynCfgType>(), Ok(DynCfgType::Job));
        assert_eq!("invalid".parse::<DynCfgType>(), Err(()));
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgType::default(), DynCfgType::Single);
    }
}
