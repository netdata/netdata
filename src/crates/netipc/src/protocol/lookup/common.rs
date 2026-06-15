//! Shared helpers for lookup codecs.

use crate::protocol::{align8, NipcError, StrView, ALIGNMENT};

pub const LOOKUP_DIR_ENTRY_SIZE: usize = 8;
pub const LOOKUP_LABEL_ENTRY_SIZE: usize = 16;

pub const ORCHESTRATOR_UNKNOWN: u16 = 0;
pub const ORCHESTRATOR_SYSTEMD: u16 = 1;
pub const ORCHESTRATOR_DOCKER: u16 = 2;
pub const ORCHESTRATOR_K8S: u16 = 3;
pub const ORCHESTRATOR_KVM: u16 = 4;
pub const ORCHESTRATOR_LXC: u16 = 5;
pub const ORCHESTRATOR_PODMAN: u16 = 6;
pub const ORCHESTRATOR_NSPAWN: u16 = 7;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct LookupLabelView<'a> {
    pub key: StrView<'a>,
    pub value: StrView<'a>,
}

#[inline]
pub(super) fn u16_at(buf: &[u8], off: usize) -> u16 {
    u16::from_ne_bytes(buf[off..off + 2].try_into().unwrap())
}

#[inline]
pub(super) fn u32_at(buf: &[u8], off: usize) -> u32 {
    u32::from_ne_bytes(buf[off..off + 4].try_into().unwrap())
}

#[inline]
pub(super) fn u64_at(buf: &[u8], off: usize) -> u64 {
    u64::from_ne_bytes(buf[off..off + 8].try_into().unwrap())
}

#[inline]
pub(super) fn put_u16(buf: &mut [u8], off: usize, value: u16) {
    buf[off..off + 2].copy_from_slice(&value.to_ne_bytes());
}

#[inline]
pub(super) fn put_u32(buf: &mut [u8], off: usize, value: u32) {
    buf[off..off + 4].copy_from_slice(&value.to_ne_bytes());
}

#[inline]
pub(super) fn put_u64(buf: &mut [u8], off: usize, value: u64) {
    buf[off..off + 8].copy_from_slice(&value.to_ne_bytes());
}

pub(super) fn checked_u32(value: usize) -> Result<u32, NipcError> {
    u32::try_from(value).map_err(|_| NipcError::Overflow)
}

pub(super) fn checked_u16(value: usize) -> Result<u16, NipcError> {
    u16::try_from(value).map_err(|_| NipcError::Overflow)
}

pub(super) fn source_string_invalid(bytes: &[u8], require_non_empty: bool) -> bool {
    (require_non_empty && bytes.is_empty()) || bytes.contains(&0)
}

pub(super) fn validate_lookup_dir(
    buf: &[u8],
    dir_start: usize,
    item_count: u32,
    packed_area_len: usize,
    min_len: usize,
    exact_len: Option<usize>,
) -> Result<(), NipcError> {
    let dir_size = (item_count as usize)
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(NipcError::BadItemCount)?;
    if dir_start
        .checked_add(dir_size)
        .ok_or(NipcError::BadItemCount)?
        > buf.len()
    {
        return Err(NipcError::Truncated);
    }

    let mut prev_end = 0usize;
    for i in 0..item_count as usize {
        let base = dir_start + i * LOOKUP_DIR_ENTRY_SIZE;
        let off = u32_at(buf, base) as usize;
        let len = u32_at(buf, base + 4) as usize;
        if off % ALIGNMENT != 0 {
            return Err(NipcError::BadAlignment);
        }
        if let Some(exact) = exact_len {
            if len != exact {
                return Err(NipcError::BadLayout);
            }
        } else if len < min_len {
            return Err(NipcError::BadLayout);
        }
        let end = off.checked_add(len).ok_or(NipcError::OutOfBounds)?;
        if end > packed_area_len {
            return Err(NipcError::OutOfBounds);
        }
        if i > 0 && off < prev_end {
            return Err(NipcError::BadLayout);
        }
        prev_end = end;
    }
    Ok(())
}

pub(super) fn lookup_string<'a>(
    item: &'a [u8],
    hdr_size: usize,
    offset: usize,
    length: usize,
) -> Result<(StrView<'a>, usize), NipcError> {
    if offset < hdr_size {
        return Err(NipcError::OutOfBounds);
    }
    let nul = offset.checked_add(length).ok_or(NipcError::OutOfBounds)?;
    if nul >= item.len() {
        return Err(NipcError::OutOfBounds);
    }
    if item[nul] != 0 {
        return Err(NipcError::MissingNul);
    }
    if item[offset..nul].contains(&0) {
        return Err(NipcError::BadLayout);
    }
    Ok((
        StrView {
            bytes: &item[offset..nul + 1],
            len: length as u32,
        },
        nul + 1,
    ))
}

#[inline]
pub(super) fn overlap(a_start: usize, a_end: usize, b_start: usize, b_end: usize) -> bool {
    a_start < b_end && b_start < a_end
}

pub(super) fn checked_subslice<'a>(
    buf: &'a [u8],
    base: usize,
    offset: usize,
    len: usize,
) -> Result<&'a [u8], NipcError> {
    let start = base.checked_add(offset).ok_or(NipcError::OutOfBounds)?;
    let end = start.checked_add(len).ok_or(NipcError::OutOfBounds)?;
    buf.get(start..end).ok_or(NipcError::OutOfBounds)
}

pub(super) fn lookup_data_offset(hdr_size: usize, item_count: u32) -> Result<usize, NipcError> {
    (item_count as usize)
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .and_then(|v| hdr_size.checked_add(v))
        .ok_or(NipcError::BadItemCount)
}

pub(super) fn lookup_dir_entry_offset(hdr_size: usize, index: u32) -> Result<usize, NipcError> {
    (index as usize)
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .and_then(|v| hdr_size.checked_add(v))
        .ok_or(NipcError::BadItemCount)
}

pub(super) fn validate_labels(
    item: &[u8],
    hdr_size: usize,
    label_count: u16,
    fixed_end: usize,
) -> Result<usize, NipcError> {
    if label_count == 0 {
        if fixed_end != item.len() {
            return Err(NipcError::BadLayout);
        }
        return Ok(fixed_end);
    }

    let table_start = align8(fixed_end);
    if table_start > item.len() {
        return Err(NipcError::OutOfBounds);
    }
    if item[fixed_end..table_start].iter().any(|&b| b != 0) {
        return Err(NipcError::BadLayout);
    }

    let table_bytes = (label_count as usize)
        .checked_mul(LOOKUP_LABEL_ENTRY_SIZE)
        .ok_or(NipcError::OutOfBounds)?;
    let mut expected = table_start
        .checked_add(table_bytes)
        .ok_or(NipcError::OutOfBounds)?;
    if expected > item.len() {
        return Err(NipcError::OutOfBounds);
    }

    for i in 0..label_count as usize {
        let entry_rel = i
            .checked_mul(LOOKUP_LABEL_ENTRY_SIZE)
            .ok_or(NipcError::OutOfBounds)?;
        let base = table_start
            .checked_add(entry_rel)
            .ok_or(NipcError::OutOfBounds)?;
        let key_off = u32_at(item, base) as usize;
        let key_len = u32_at(item, base + 4) as usize;
        let value_off = u32_at(item, base + 8) as usize;
        let value_len = u32_at(item, base + 12) as usize;
        if key_len == 0 || key_off != expected {
            return Err(NipcError::BadLayout);
        }
        let (_, key_end) = lookup_string(item, hdr_size, key_off, key_len)?;
        expected = key_end;
        if value_off != expected {
            return Err(NipcError::BadLayout);
        }
        let (_, value_end) = lookup_string(item, hdr_size, value_off, value_len)?;
        expected = value_end;
    }

    if expected != item.len() {
        return Err(NipcError::BadLayout);
    }
    Ok(table_start)
}

pub(super) fn label_at<'a>(
    item: &'a [u8],
    hdr_size: usize,
    label_count: u16,
    label_table_offset: usize,
    index: u32,
) -> Result<LookupLabelView<'a>, NipcError> {
    if index >= label_count as u32 {
        return Err(NipcError::OutOfBounds);
    }
    let entry_offset = (index as usize)
        .checked_mul(LOOKUP_LABEL_ENTRY_SIZE)
        .ok_or(NipcError::OutOfBounds)?;
    let base = label_table_offset
        .checked_add(entry_offset)
        .ok_or(NipcError::OutOfBounds)?;
    let entry_end = base
        .checked_add(LOOKUP_LABEL_ENTRY_SIZE)
        .ok_or(NipcError::OutOfBounds)?;
    if entry_end > item.len() {
        return Err(NipcError::OutOfBounds);
    }
    let key_off = u32_at(item, base) as usize;
    let key_len = u32_at(item, base + 4) as usize;
    let value_off = u32_at(item, base + 8) as usize;
    let value_len = u32_at(item, base + 12) as usize;
    let (key, _) = lookup_string(item, hdr_size, key_off, key_len)?;
    let (value, _) = lookup_string(item, hdr_size, value_off, value_len)?;
    Ok(LookupLabelView { key, value })
}

pub(super) fn write_lookup_labels(
    item: &mut [u8],
    table_start: usize,
    table_bytes: usize,
    labels: &[(&[u8], &[u8])],
) -> Result<usize, NipcError> {
    let mut next = table_start
        .checked_add(table_bytes)
        .ok_or(NipcError::Overflow)?;
    for (i, (key, value)) in labels.iter().enumerate() {
        let entry_offset = i
            .checked_mul(LOOKUP_LABEL_ENTRY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let entry = table_start
            .checked_add(entry_offset)
            .ok_or(NipcError::Overflow)?;
        let value_offset = next
            .checked_add(key.len())
            .and_then(|v| v.checked_add(1))
            .ok_or(NipcError::Overflow)?;
        put_u32(item, entry, checked_u32(next)?);
        put_u32(item, entry + 4, checked_u32(key.len())?);
        put_u32(item, entry + 8, checked_u32(value_offset)?);
        put_u32(item, entry + 12, checked_u32(value.len())?);
        item[next..next + key.len()].copy_from_slice(key);
        item[next + key.len()] = 0;
        next = value_offset;
        item[next..next + value.len()].copy_from_slice(value);
        item[next + value.len()] = 0;
        next = next
            .checked_add(value.len())
            .and_then(|v| v.checked_add(1))
            .ok_or(NipcError::Overflow)?;
    }
    Ok(next)
}

pub(super) fn label_layout(
    fixed_end: usize,
    labels: &[(&[u8], &[u8])],
) -> Result<(usize, usize, usize), NipcError> {
    if labels.is_empty() {
        return Ok((fixed_end, 0, fixed_end));
    }
    let table_start = align8(fixed_end);
    let table_bytes = labels
        .len()
        .checked_mul(LOOKUP_LABEL_ENTRY_SIZE)
        .ok_or(NipcError::Overflow)?;
    let mut item_size = table_start
        .checked_add(table_bytes)
        .ok_or(NipcError::Overflow)?;
    for (key, value) in labels {
        if source_string_invalid(key, true) || source_string_invalid(value, false) {
            return Err(NipcError::BadLayout);
        }
        let key_size = key.len().checked_add(1).ok_or(NipcError::Overflow)?;
        let value_size = value.len().checked_add(1).ok_or(NipcError::Overflow)?;
        item_size = item_size
            .checked_add(key_size)
            .and_then(|v| v.checked_add(value_size))
            .ok_or(NipcError::Overflow)?;
    }
    Ok((table_start, table_bytes, item_size))
}

pub(super) fn finish_lookup_response(
    buf: &mut [u8],
    hdr_size: usize,
    item_count: u32,
    data_offset: usize,
    generation: u64,
) -> Result<usize, NipcError> {
    put_u16(buf, 0, 1);
    put_u16(buf, 2, 0);
    put_u32(buf, 4, item_count);
    put_u64(buf, 8, generation);
    if item_count == 0 {
        return Ok(hdr_size);
    }
    let final_packed_start = (item_count as usize)
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .and_then(|v| hdr_size.checked_add(v))
        .ok_or(NipcError::Overflow)?;
    let first_item_abs = u32_at(buf, hdr_size) as usize;
    let packed_data_len = data_offset
        .checked_sub(first_item_abs)
        .ok_or(NipcError::Overflow)?;
    if final_packed_start < first_item_abs {
        let copy_end = first_item_abs
            .checked_add(packed_data_len)
            .ok_or(NipcError::Overflow)?;
        if copy_end > buf.len() {
            return Err(NipcError::Overflow);
        }
        let dest_end = final_packed_start
            .checked_add(packed_data_len)
            .ok_or(NipcError::Overflow)?;
        if dest_end > buf.len() {
            return Err(NipcError::Overflow);
        }
        buf.copy_within(first_item_abs..copy_end, final_packed_start);
    }
    for i in 0..item_count as usize {
        let entry_offset = i
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let entry = hdr_size
            .checked_add(entry_offset)
            .ok_or(NipcError::Overflow)?;
        let abs = u32_at(buf, entry) as usize;
        let rel = abs.checked_sub(first_item_abs).ok_or(NipcError::Overflow)?;
        put_u32(buf, entry, checked_u32(rel)?);
    }
    final_packed_start
        .checked_add(packed_data_len)
        .ok_or(NipcError::Overflow)
}

#[cfg(test)]
pub(super) fn response_item_bounds(
    buf: &[u8],
    hdr_size: usize,
    item_count: usize,
    index: usize,
) -> (usize, usize) {
    let dir = hdr_size + index * LOOKUP_DIR_ENTRY_SIZE;
    let off = u32_at(buf, dir) as usize;
    let len = u32_at(buf, dir + 4) as usize;
    (hdr_size + item_count * LOOKUP_DIR_ENTRY_SIZE + off, len)
}
