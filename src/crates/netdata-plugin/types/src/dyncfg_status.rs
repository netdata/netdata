//! The dyncfg status vocabulary: what state a dynamic-configuration entry is
//! in, as reported by its owning plugin or derived by the agent itself (for
//! example marking unclaimed entries orphan). A word list shared across the
//! wire, mirrored by the agent's `DYNCFG_STATUS` enum and name table
//! (src/libnetdata/inicfg/dyncfg.h, dyncfg.c).
//!
//! Rendering:
//! - pluginsd wire: the `<status>` word of
//!   `CONFIG <id> CREATE <status> <type> <path> <source_type> <source>
//!   <cmds> <view_access> <edit_access>`, printed lowercase via `Display` by
//!   the netdata-plugin-protocol encoder and parsed by pluginsd_config
//!   (src/plugins.d/pluginsd_dyncfg.c) through `dyncfg_status2id`. The same
//!   word also carries `CONFIG <id> status` update lines, which C plugins
//!   emit via `functions_evloop_dyncfg_status`
//!   (src/libnetdata/functions_evloop/functions_evloop.c); the Rust protocol
//!   crate has no message variant for that action.
//!
//! `Default` has no callers outside this file's tests; the file-level
//! allow(dead_code) keeps it, as in the sibling dyncfg_* vocabulary modules.
#![allow(dead_code)]

use std::fmt;

/// The lifecycle state of a dyncfg configuration entry, as carried in
/// [`ConfigDeclaration::status`](crate::ConfigDeclaration::status) and by
/// `CONFIG <id> status` update lines.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DynCfgStatus {
    /// Nothing reported: value 0 and the agent's parse fallback —
    /// `dyncfg_status2id` maps missing, empty and unknown words here.
    /// Stored as-is at registration, but `dyncfg_status_low_level` rejects
    /// it on `CONFIG <id> status` updates (src/daemon/dyncfg/dyncfg.c).
    None,
    /// The plugin has accepted the configuration but does not run it. The
    /// agent derives it from successful add/update responses other than 200
    /// (running) and 298 (disabled).
    Accepted,
    /// The plugin runs the accepted configuration; the agent derives it
    /// from a 200 add/update response.
    Running,
    /// The plugin fails to run the accepted configuration. Like incomplete,
    /// the agent never assigns it — only plugins report it.
    Failed,
    /// The configuration is disabled by a user; the agent also sets it when
    /// a plugin acknowledges a disable command.
    Disabled,
    /// No plugin has claimed this configuration. The agent assigns it
    /// itself when the registered function is no longer available, and an
    /// orphaned entry accepts only REMOVE (src/daemon/dyncfg/dyncfg-tree.c).
    Orphan,
    /// A special kind of failed configuration. In the agent's vocabulary and
    /// counted in its config tree summary, but no agent code assigns it
    /// today — only plugins report it.
    Incomplete,
}

impl DynCfgStatus {
    /// The canonical lowercase vocabulary word for this value: what
    /// `Display` prints into the `CONFIG` line.
    pub fn name(&self) -> &'static str {
        match self {
            Self::None => "none",
            Self::Accepted => "accepted",
            Self::Running => "running",
            Self::Failed => "failed",
            Self::Disabled => "disabled",
            Self::Orphan => "orphan",
            Self::Incomplete => "incomplete",
        }
    }
}

impl Default for DynCfgStatus {
    /// The agent's fallback: dyncfg_status2id maps missing, empty or unknown
    /// words to DYNCFG_STATUS_NONE.
    fn default() -> Self {
        Self::None
    }
}

impl fmt::Display for DynCfgStatus {
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
        assert_eq!(DynCfgStatus::None.name(), "none");
        assert_eq!(DynCfgStatus::Accepted.name(), "accepted");
        assert_eq!(DynCfgStatus::Running.name(), "running");
        assert_eq!(DynCfgStatus::Failed.name(), "failed");
        assert_eq!(DynCfgStatus::Disabled.name(), "disabled");
        assert_eq!(DynCfgStatus::Orphan.name(), "orphan");
        assert_eq!(DynCfgStatus::Incomplete.name(), "incomplete");
    }

    #[test]
    fn test_display() {
        assert_eq!(format!("{}", DynCfgStatus::None), "none");
        assert_eq!(format!("{}", DynCfgStatus::Accepted), "accepted");
        assert_eq!(format!("{}", DynCfgStatus::Running), "running");
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgStatus::default(), DynCfgStatus::None);
    }
}
