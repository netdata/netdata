#![allow(dead_code)]

use std::fmt;
use std::str::FromStr;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DynCfgType {
    Single,
    Template,
    Job,
}

impl DynCfgType {
    /// Get the string name for this type
    pub fn name(&self) -> &'static str {
        match self {
            Self::Single => "single",
            Self::Template => "template",
            Self::Job => "job",
        }
    }

    /// Parse from string name
    pub fn from_name(name: &str) -> Option<Self> {
        match name {
            "single" => Some(Self::Single),
            "template" => Some(Self::Template),
            "job" => Some(Self::Job),
            _ => None,
        }
    }

    /// Parse from byte slice
    pub fn from_slice(bytes: &[u8]) -> Option<Self> {
        let s = std::str::from_utf8(bytes).ok()?.trim();
        Self::from_name(s)
    }
}

impl Default for DynCfgType {
    fn default() -> Self {
        Self::Single
    }
}

impl fmt::Display for DynCfgType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

impl FromStr for DynCfgType {
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
