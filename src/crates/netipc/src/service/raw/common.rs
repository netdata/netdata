pub(super) const SERVER_POLL_TIMEOUT_MS: u32 = 100;
pub(super) const CLIENT_SHM_ATTACH_RETRY_INTERVAL_MS: u64 = 5;
pub(super) const CLIENT_SHM_ATTACH_RETRY_TIMEOUT_MS: u64 = 5_000;
pub(super) const CACHE_RESPONSE_BUF_SIZE: usize = 65536;

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
