#![allow(dead_code)]

use std::fmt;
use std::str::FromStr;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DynCfgSourceType {
    Internal,
    Stock,
    User,
    Dyncfg,
    Discovered,
}

impl DynCfgSourceType {
    /// Get the string name for this source type
    pub fn name(&self) -> &'static str {
        match self {
            Self::Internal => "internal",
            Self::Stock => "stock",
            Self::User => "user",
            Self::Dyncfg => "dyncfg",
            Self::Discovered => "discovered",
        }
    }

    /// Parse from string name
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

    /// Parse from byte slice
    pub fn from_slice(bytes: &[u8]) -> Option<Self> {
        let s = std::str::from_utf8(bytes).ok()?.trim();
        Self::from_name(s)
    }
}

impl Default for DynCfgSourceType {
    fn default() -> Self {
        Self::Internal
    }
}

impl fmt::Display for DynCfgSourceType {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}", self.name())
    }
}

impl FromStr for DynCfgSourceType {
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
        assert_eq!(DynCfgSourceType::Internal.name(), "internal");
        assert_eq!(DynCfgSourceType::Stock.name(), "stock");
        assert_eq!(DynCfgSourceType::User.name(), "user");
        assert_eq!(DynCfgSourceType::Dyncfg.name(), "dyncfg");
        assert_eq!(DynCfgSourceType::Discovered.name(), "discovered");
    }

    #[test]
    fn test_from_name() {
        assert_eq!(DynCfgSourceType::from_name("internal"), Some(DynCfgSourceType::Internal));
        assert_eq!(DynCfgSourceType::from_name("stock"), Some(DynCfgSourceType::Stock));
        assert_eq!(DynCfgSourceType::from_name("user"), Some(DynCfgSourceType::User));
        assert_eq!(DynCfgSourceType::from_name("dyncfg"), Some(DynCfgSourceType::Dyncfg));
        assert_eq!(DynCfgSourceType::from_name("discovered"), Some(DynCfgSourceType::Discovered));
        assert_eq!(DynCfgSourceType::from_name("invalid"), None);
    }

    #[test]
    fn test_from_slice() {
        assert_eq!(DynCfgSourceType::from_slice(b"internal"), Some(DynCfgSourceType::Internal));
        assert_eq!(DynCfgSourceType::from_slice(b"  stock  "), Some(DynCfgSourceType::Stock));
        assert_eq!(DynCfgSourceType::from_slice(b"user"), Some(DynCfgSourceType::User));
        assert_eq!(DynCfgSourceType::from_slice(b"dyncfg"), Some(DynCfgSourceType::Dyncfg));
        assert_eq!(DynCfgSourceType::from_slice(b"discovered"), Some(DynCfgSourceType::Discovered));
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
        assert_eq!("internal".parse::<DynCfgSourceType>(), Ok(DynCfgSourceType::Internal));
        assert_eq!("stock".parse::<DynCfgSourceType>(), Ok(DynCfgSourceType::Stock));
        assert_eq!("user".parse::<DynCfgSourceType>(), Ok(DynCfgSourceType::User));
        assert_eq!("dyncfg".parse::<DynCfgSourceType>(), Ok(DynCfgSourceType::Dyncfg));
        assert_eq!("discovered".parse::<DynCfgSourceType>(), Ok(DynCfgSourceType::Discovered));
        assert_eq!("invalid".parse::<DynCfgSourceType>(), Err(()));
    }

    #[test]
    fn test_default() {
        assert_eq!(DynCfgSourceType::default(), DynCfgSourceType::Internal);
    }
}