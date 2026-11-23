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
//!
//! The value also gates agent-side registration behavior: the declared
//! command set is sanitized against type and source type, and job
//! registration requires a registered template — see the variant docs
//! (dyncfg_sanitize_cmds, dyncfg_add_low_level,
//! src/daemon/dyncfg/dyncfg.c).
//!
//! `Default` has no callers outside this file's tests; the file-level
//! allow(dead_code) keeps it, as in the sibling dyncfg_* vocabulary modules.
#![allow(dead_code)]

use std::fmt;

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
    /// `Display` prints into the `CONFIG` line.
    pub fn name(&self) -> &'static str {
        match self {
            Self::Single => "single",
            Self::Template => "template",
            Self::Job => "job",
        }
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
    fn test_display() {
        assert_eq!(format!("{}", DynCfgType::Single), "single");
        assert_eq!(format!("{}", DynCfgType::Template), "template");
        assert_eq!(format!("{}", DynCfgType::Job), "job");
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgType::default(), DynCfgType::Single);
    }
}
