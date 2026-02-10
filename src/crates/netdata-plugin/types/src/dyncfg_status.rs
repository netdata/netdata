#![allow(dead_code)]

use std::fmt;
use std::str::FromStr;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DynCfgStatus {
    None,
    Accepted,
    Running,
    Failed,
    Disabled,
    Orphan,
    Incomplete,
}

impl DynCfgStatus {
    /// Get the string name for this status
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

    /// Parse from string name
    pub fn from_name(name: &str) -> Option<Self> {
        match name {
            "none" => Some(Self::None),
            "accepted" => Some(Self::Accepted),
            "running" => Some(Self::Running),
            "failed" => Some(Self::Failed),
            "disabled" => Some(Self::Disabled),
            "orphan" => Some(Self::Orphan),
            "incomplete" => Some(Self::Incomplete),
            _ => None,
        }
    }

    /// Parse from byte slice
    pub fn from_slice(bytes: &[u8]) -> Option<Self> {
        let s = std::str::from_utf8(bytes).ok()?.trim();
        Self::from_name(s)
    }
}

impl Default for DynCfgStatus {
    fn default() -> Self {
        Self::None
    }
}

impl fmt::Display for DynCfgStatus {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

impl FromStr for DynCfgStatus {
    type Err = ();

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        Self::from_name(s).ok_or(())
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
    fn test_from_name() {
        assert_eq!(DynCfgStatus::from_name("none"), Some(DynCfgStatus::None));
        assert_eq!(DynCfgStatus::from_name("accepted"), Some(DynCfgStatus::Accepted));
        assert_eq!(DynCfgStatus::from_name("running"), Some(DynCfgStatus::Running));
        assert_eq!(DynCfgStatus::from_name("failed"), Some(DynCfgStatus::Failed));
        assert_eq!(DynCfgStatus::from_name("disabled"), Some(DynCfgStatus::Disabled));
        assert_eq!(DynCfgStatus::from_name("orphan"), Some(DynCfgStatus::Orphan));
        assert_eq!(DynCfgStatus::from_name("incomplete"), Some(DynCfgStatus::Incomplete));
        assert_eq!(DynCfgStatus::from_name("invalid"), None);
    }

    #[test]
    fn test_from_slice() {
        assert_eq!(DynCfgStatus::from_slice(b"running"), Some(DynCfgStatus::Running));
        assert_eq!(DynCfgStatus::from_slice(b"  failed  "), Some(DynCfgStatus::Failed));
        assert_eq!(DynCfgStatus::from_slice(b"invalid"), None);
        assert_eq!(DynCfgStatus::from_slice(&[0xFF, 0xFE]), None); // Invalid UTF-8
    }

    #[test]
    fn test_display() {
        assert_eq!(format!("{}", DynCfgStatus::None), "none");
        assert_eq!(format!("{}", DynCfgStatus::Accepted), "accepted");
        assert_eq!(format!("{}", DynCfgStatus::Running), "running");
    }

    #[test]
    fn test_from_str() {
        assert_eq!("none".parse::<DynCfgStatus>(), Ok(DynCfgStatus::None));
        assert_eq!("running".parse::<DynCfgStatus>(), Ok(DynCfgStatus::Running));
        assert_eq!("invalid".parse::<DynCfgStatus>(), Err(()));
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgStatus::default(), DynCfgStatus::None);
    }
}