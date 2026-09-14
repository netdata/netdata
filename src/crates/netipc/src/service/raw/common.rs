pub(super) const SERVER_POLL_TIMEOUT_MS: u32 = 100;
pub(super) const CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS: u64 = 5;
pub(super) const CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS: u64 = 5_000;
pub(super) const CACHE_RESPONSE_BUF_SIZE: usize = 65536;
pub const LOOKUP_LOGICAL_ITEMS_DEFAULT: u32 = 65_536;
pub const LOOKUP_LOGICAL_SUBCALLS_DEFAULT: u32 = 4_096;
pub const LOOKUP_LOGICAL_RESPONSE_BYTES_DEFAULT: u32 = 64 * 1024 * 1024;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct LookupLogicalConfig {
    pub max_items: u32,
    pub max_subcalls: u32,
    pub max_response_bytes: u32,
}

impl Default for LookupLogicalConfig {
    fn default() -> Self {
        Self {
            max_items: LOOKUP_LOGICAL_ITEMS_DEFAULT,
            max_subcalls: LOOKUP_LOGICAL_SUBCALLS_DEFAULT,
            max_response_bytes: LOOKUP_LOGICAL_RESPONSE_BYTES_DEFAULT,
        }
    }
}

impl LookupLogicalConfig {
    pub(super) fn normalize(self) -> Self {
        let defaults = Self::default();
        Self {
            max_items: if self.max_items == 0 {
                defaults.max_items
            } else {
                self.max_items
            },
            max_subcalls: if self.max_subcalls == 0 {
                defaults.max_subcalls
            } else {
                self.max_subcalls
            },
            max_response_bytes: if self.max_response_bytes == 0 {
                defaults.max_response_bytes
            } else {
                self.max_response_bytes
            },
        }
    }
}

pub(super) fn next_power_of_2_u32(n: u32) -> u32 {
    if n < 16 {
        return 16;
    }

    if n > (1u32 << 31) {
        return 1u32 << 31;
    }

    let mut value = n - 1;
    value |= value >> 1;
    value |= value >> 2;
    value |= value >> 4;
    value |= value >> 8;
    value |= value >> 16;
    value + 1
}

pub(super) fn ensure_client_scratch(buf: &mut Vec<u8>, needed: usize) -> &mut [u8] {
    if buf.len() < needed {
        buf.resize(needed, 0);
    }
    &mut buf[..needed]
}

pub(super) fn lookup_raw_response_size(
    hdr_size: usize,
    item_count: usize,
    items: &[Vec<u8>],
) -> Result<usize, crate::protocol::NipcError> {
    let dir_size = item_count
        .checked_mul(crate::protocol::LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(crate::protocol::NipcError::Overflow)?;
    let mut data = hdr_size
        .checked_add(dir_size)
        .ok_or(crate::protocol::NipcError::Overflow)?;
    for item in items {
        data = crate::protocol::align8(data);
        data = data
            .checked_add(item.len())
            .ok_or(crate::protocol::NipcError::Overflow)?;
    }
    Ok(data)
}
