#![allow(dead_code)]

//! The agent HTTP access bitmask: [`HttpAccess`], the 11-bit (0x7FF) mask
//! carried by function declarations, function calls and dyncfg config
//! declarations. Bit layout, wire rendering and legacy role mappings mirror
//! the agent's HTTP_ACCESS enum and helpers bit-for-bit
//! (src/libnetdata/user-auth/http-access.h, http-access.c), so u32 values
//! interoperate.
//!
//! The mask is a requirement set, not a grant: the agent grants a request
//! only when the user's granted access contains every bit the endpoint
//! requires (http_access_user_has_enough_access_level_for_endpoint,
//! src/libnetdata/user-auth/http-access.h). What the mask means depends on
//! direction:
//! - `FunctionDeclaration.access`: the access a caller needs. An empty mask
//!   adds no requirement (the agent's subset check passes for every user).
//! - `FunctionCall.access`: the calling user's granted access, sent by the
//!   agent after it has enforced the requirement; no Rust consumer re-checks
//!   it.
//! - `ConfigDeclaration` view/edit access: the permissions needed to view or
//!   edit the entry. An empty mask is replaced at registration by the agent's
//!   defaults, not read as "no access" (dyncfg_add_low_level,
//!   src/daemon/dyncfg/dyncfg.c).
//!
//! Rendering:
//! - pluginsd wire: `Display` prints `0x…` lowercase hex, matching the C
//!   HTTP_ACCESS_FORMAT ("0x%" PRIx32). netdata-plugin-protocol uses it for
//!   the access word of the FUNCTION and FUNCTION_PAYLOAD lines (omitted
//!   when the mask is `None`; the agent maps a missing word to the empty
//!   mask, http-access.c) and for the view/edit words of the CONFIG line.
//! - serde/JSON: serializes as the raw u32 bit pattern (a decimal number)
//!   and deserializes dropping unknown bits — the form
//!   FunctionDeclaration/FunctionCall carry through serde; the
//!   configDeclaration viewAccess/editAccess members are decimal u32 numbers
//!   too, written by netdata-plugin-schema and read back by types::config.
//!
//! Parsing is lax, mirroring the agent's strtoull-based helpers:
//! [`HttpAccess::from_slice`] accepts hex (with or without `0x`) and the
//! legacy role words any/all, member(s), admin(s) — the words the agent's
//! declaration parser also accepts (http_access_from_hex_mapping_old_roles,
//! called by that parser in src/plugins.d/pluginsd_functions.c) — mapping
//! anything else, including invalid UTF-8, to the empty mask.
//! [`HttpAccess::from_hex`] is the strict half: it returns `None` on non-hex
//! words where the C strtoull would partially decode them ("7zz" → 0x7).
//!
//! `has`, `as_u32` and `from_hex` have no callers outside this file's tests;
//! the file carries the same file-level allow(dead_code) as the sibling
//! dyncfg_* vocabulary modules.
use bitflags::bitflags;
use std::fmt;

bitflags! {
    /// The access bits a Netdata user holds or an endpoint requires, matching
    /// the agent's HTTP_ACCESS enum (src/libnetdata/user-auth/http-access.h)
    /// bit-for-bit. Wire, JSON and parsing contracts are in the module docs.
    #[derive(Debug, Clone, Copy, PartialEq, Eq, Default, Hash)]
    pub struct HttpAccess: u32 {
        /// The user is authenticated.
        const SIGNED_ID = 1 << 0;
        /// The Netdata Cloud user and this agent are in the same space.
        const SAME_SPACE = 1 << 1;
        /// The space is on a commercial plan.
        const COMMERCIAL_SPACE = 1 << 2;
        /// Non-identifying data may be read (Netdata Cloud `room:Read`).
        const ANONYMOUS_DATA = 1 << 3;
        /// Sensitive data may be read (Netdata Cloud `agent:ViewSensitiveData`).
        const SENSITIVE_DATA = 1 << 4;
        /// Dyncfg entries may be viewed (Netdata Cloud `agent:ReadDynCfg`).
        const VIEW_AGENT_CONFIG = 1 << 5;
        /// Dyncfg entries may be edited (Netdata Cloud `agent:EditDynCfg`).
        const EDIT_AGENT_CONFIG = 1 << 6;
        /// Notification config may be viewed (Netdata Cloud `agent:ViewNotificationsConfig`).
        const VIEW_NOTIFICATIONS_CONFIG = 1 << 7;
        /// Notification config may be edited (Netdata Cloud `agent:EditNotificationsConfig`).
        const EDIT_NOTIFICATIONS_CONFIG = 1 << 8;
        /// Alert-silencing rules may be viewed (Netdata Cloud `space:GetSystemSilencingRules`).
        const VIEW_ALERTS_SILENCING = 1 << 9;
        /// Alert-silencing rules may be edited (Netdata Cloud `space:CreateSystemSilencingRule`).
        const EDIT_ALERTS_SILENCING = 1 << 10;
    }
}

/// Serializes the raw u32 bit pattern as a plain number, the form
/// FunctionDeclaration/FunctionCall carry through serde.
impl serde::Serialize for HttpAccess {
    fn serialize<S: serde::Serializer>(&self, serializer: S) -> Result<S::Ok, S::Error> {
        self.bits().serialize(serializer)
    }
}

/// Reads a u32 and drops undefined bits via `from_bits_truncate`, so flags
/// unknown to this build are ignored rather than rejected.
impl<'de> serde::Deserialize<'de> for HttpAccess {
    fn deserialize<D: serde::Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        let bits = u32::deserialize(deserializer)?;
        Ok(Self::from_bits_truncate(bits))
    }
}

impl HttpAccess {
    /// All 11 defined bits (0x7FF) — the agent's HTTP_ACCESS_ALL, and the
    /// mask every parser and conversion here clamps values to.
    pub const ALL: Self = Self::from_bits_truncate(0x7FF);

    /// The mask substituted for the legacy role words `any`/`all`; the
    /// MAP_OLD_* trio mirrors the agent's HTTP_ACCESS_MAP_OLD_* macros
    /// (src/libnetdata/user-auth/http-access.h).
    pub const MAP_OLD_ANY: Self = Self::ANONYMOUS_DATA;

    /// The mask substituted for the legacy role words `member`/`members`.
    pub const MAP_OLD_MEMBER: Self = Self::SIGNED_ID
        .union(Self::SAME_SPACE)
        .union(Self::ANONYMOUS_DATA)
        .union(Self::SENSITIVE_DATA);

    /// The mask substituted for the legacy role words `admin`/`admins`.
    pub const MAP_OLD_ADMIN: Self = Self::SIGNED_ID
        .union(Self::SAME_SPACE)
        .union(Self::ANONYMOUS_DATA)
        .union(Self::SENSITIVE_DATA)
        .union(Self::VIEW_AGENT_CONFIG)
        .union(Self::EDIT_AGENT_CONFIG);

    /// Parse an access mask from hex text: optional `0x` prefix, hex digits,
    /// surrounding whitespace allowed; an empty string is the empty mask;
    /// unknown bits are masked off; other text is `None` (a leading `+`
    /// before the digits still parses, as `u32::from_str_radix` allows).
    /// The strict half of [`Self::from_slice`]'s wire-word parsing.
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

    /// Parse a pluginsd access word (raw wire bytes): the legacy role names
    /// `any`/`all`, `member`/`members`, `admin`/`admins` map to the MAP_OLD_*
    /// masks, anything else goes through [`Self::from_hex`]. Invalid UTF-8
    /// and unparseable words yield the empty mask, like the C parsers'
    /// fallback to HTTP_ACCESS_NONE. Used by netdata-plugin-protocol's parser
    /// on the FUNCTION (declaration and call) and FUNCTION_PAYLOAD lines.
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

    /// True when every bit of `other` is set in `self`; a thin alias for
    /// bitflags' `contains`, unused outside this file's tests.
    pub fn has(&self, other: Self) -> bool {
        self.contains(other)
    }

    /// The raw u32 bit pattern, the form the agent stores and prints.
    pub fn as_u32(&self) -> u32 {
        self.bits()
    }

    /// Build from raw bits, dropping undefined bits (`from_bits_truncate`).
    /// Used for the configDeclaration viewAccess/editAccess values
    /// (types::config, netdata-plugin-schema) and `FunctionDeclaration::new`'s
    /// empty default.
    pub fn from_u32(value: u32) -> Self {
        Self::from_bits_truncate(value & Self::ALL.bits())
    }
}

/// Delegates to [`HttpAccess::from_u32`]; the bit layout matches the agent's
/// HTTP_ACCESS enum, so agent-side u32 values interoperate.
impl From<u32> for HttpAccess {
    fn from(value: u32) -> Self {
        Self::from_u32(value)
    }
}

/// The raw bit pattern — what netdata-plugin-schema writes into the
/// configDeclaration viewAccess/editAccess JSON members.
impl From<HttpAccess> for u32 {
    fn from(access: HttpAccess) -> Self {
        access.bits()
    }
}

/// Prints `0x…` lowercase hex, the empty mask as `0x0` — the form matching
/// the agent's HTTP_ACCESS_FORMAT ("0x%" PRIx32) that netdata-plugin-protocol
/// puts in the pluginsd access words.
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
        let access = HttpAccess::from_u32(0x9);
        assert_eq!(access.as_u32(), 0x9);

        let access: HttpAccess = 0x9u32.into();
        assert_eq!(access, HttpAccess::from_bits_truncate(0x9));

        let value: u32 = HttpAccess::SIGNED_ID.into();
        assert_eq!(value, 1);

        // Test that values beyond ALL are masked
        let access = HttpAccess::from_u32(0xFFFF);
        assert_eq!(access.as_u32(), 0x7FF);
    }
}
