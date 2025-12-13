#![allow(dead_code)]

use bitflags::bitflags;
use std::fmt;

bitflags! {
    #[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Hash)]
    pub struct HttpAccess: u32 {
        const SIGNED_ID = 1 << 0;
        const SAME_SPACE = 1 << 1;
        const COMMERCIAL_SPACE = 1 << 2;
        const ANONYMOUS_DATA = 1 << 3;
        const SENSITIVE_DATA = 1 << 4;
        const VIEW_AGENT_CONFIG = 1 << 5;
        const EDIT_AGENT_CONFIG = 1 << 6;
        const VIEW_NOTIFICATIONS_CONFIG = 1 << 7;
        const EDIT_NOTIFICATIONS_CONFIG = 1 << 8;
        const VIEW_ALERTS_SILENCING = 1 << 9;
        const EDIT_ALERTS_SILENCING = 1 << 10;
    }
}

impl HttpAccess {
    pub const ALL: Self = Self::from_bits_truncate(0x7FF);

    // Old role mappings
    pub const MAP_OLD_ANY: Self = Self::ANONYMOUS_DATA;

    pub const MAP_OLD_MEMBER: Self = Self::SIGNED_ID
        .union(Self::SAME_SPACE)
        .union(Self::ANONYMOUS_DATA)
        .union(Self::SENSITIVE_DATA);

    pub const MAP_OLD_ADMIN: Self = Self::SIGNED_ID
        .union(Self::SAME_SPACE)
        .union(Self::ANONYMOUS_DATA)
        .union(Self::SENSITIVE_DATA)
        .union(Self::VIEW_AGENT_CONFIG)
        .union(Self::EDIT_AGENT_CONFIG);

    pub fn from_hex(s: &str) -> Option<Self> {
        let s = s.trim();
        if s.is_empty() {
            return Some(Self::empty());
        }

        let s = s.strip_prefix("0x").unwrap_or(s);
        u32::from_str_radix(s, 16)
            .ok()
            .map(|v| Self::from_bits_truncate(v & Self::ALL.bits()))
    }

    pub fn from_slice(bytes: &[u8]) -> Self {
        let s = std::str::from_utf8(bytes).unwrap_or("").trim();
        if s.is_empty() {
            return Self::empty();
        }

        match s {
            "any" | "all" => Self::MAP_OLD_ANY,
            "member" | "members" => Self::MAP_OLD_MEMBER,
            "admin" | "admins" => Self::MAP_OLD_ADMIN,
            _ => Self::from_hex(s).unwrap_or_else(Self::empty),
        }
    }

    pub fn has(&self, other: Self) -> bool {
        self.contains(other)
    }

    pub fn as_u32(&self) -> u32 {
        self.bits()
    }

    pub fn from_u32(value: u32) -> Self {
        Self::from_bits_truncate(value & Self::ALL.bits())
    }
}

impl From<u32> for HttpAccess {
    fn from(value: u32) -> Self {
        Self::from_u32(value)
    }
}

impl From<HttpAccess> for u32 {
    fn from(access: HttpAccess) -> Self {
        access.bits()
    }
}

impl fmt::Display for HttpAccess {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "0x{:x}", self.bits())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_from_hex() {
        assert_eq!(
            HttpAccess::from_hex("0x9"),
            Some(HttpAccess::from_bits_truncate(0x9))
        );
        assert_eq!(HttpAccess::from_hex("7ff"), Some(HttpAccess::ALL));
        assert_eq!(HttpAccess::from_hex(""), Some(HttpAccess::empty()));
    }

    #[test]
    fn test_from_hex_mapping_old_roles() {
        assert_eq!(HttpAccess::from_slice(b"any"), HttpAccess::MAP_OLD_ANY);
        assert_eq!(HttpAccess::from_slice(b"all"), HttpAccess::MAP_OLD_ANY);
        assert_eq!(
            HttpAccess::from_slice(b"member"),
            HttpAccess::MAP_OLD_MEMBER
        );
        assert_eq!(
            HttpAccess::from_slice(b"members"),
            HttpAccess::MAP_OLD_MEMBER
        );
        assert_eq!(HttpAccess::from_slice(b"admin"), HttpAccess::MAP_OLD_ADMIN);
        assert_eq!(HttpAccess::from_slice(b"admins"), HttpAccess::MAP_OLD_ADMIN);
        assert_eq!(HttpAccess::from_slice(b"0x7ff"), HttpAccess::ALL);
        assert_eq!(HttpAccess::from_slice(b""), HttpAccess::empty());
    }

    #[test]
    fn test_has() {
        let access = HttpAccess::from_hex("0x9").unwrap();
        assert!(access.has(HttpAccess::SIGNED_ID));
        assert!(access.has(HttpAccess::ANONYMOUS_DATA));
        assert!(!access.has(HttpAccess::SENSITIVE_DATA));
    }

    #[test]
    fn test_u32_conversion() {
        // Test from_u32 and as_u32
        let access = HttpAccess::from_u32(0x9);
        assert_eq!(access.as_u32(), 0x9);

        // Test From traits
        let access: HttpAccess = 0x9u32.into();
        assert_eq!(access, HttpAccess::from_bits_truncate(0x9));

        let value: u32 = HttpAccess::SIGNED_ID.into();
        assert_eq!(value, 1);

        // Test that values beyond ALL are masked
        let access = HttpAccess::from_u32(0xFFFF);
        assert_eq!(access.as_u32(), 0x7FF);
    }
}
