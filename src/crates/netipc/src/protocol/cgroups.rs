//! Cgroups snapshot codec -- request, response view, builder, dispatch.

use super::{align8, NipcError, ALIGNMENT};

const CGROUPS_REQ_SIZE: usize = 4;
const CGROUPS_RESP_HDR_SIZE: usize = 24;
const CGROUPS_DIR_ENTRY_SIZE: usize = 8;
const CGROUPS_ITEM_HDR_SIZE: usize = 32;

// ---------------------------------------------------------------------------
//  Cgroups snapshot request (4 bytes)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct CgroupsRequest {
    pub layout_version: u16,
    pub flags: u16,
}

impl CgroupsRequest {
    /// Encode into `buf`. Returns 4 on success, 0 if buf is too small.
    pub fn encode(&self, buf: &mut [u8]) -> usize {
        if buf.len() < CGROUPS_REQ_SIZE {
            return 0;
        }
        buf[0..2].copy_from_slice(&self.layout_version.to_ne_bytes());
        buf[2..4].copy_from_slice(&self.flags.to_ne_bytes());
        CGROUPS_REQ_SIZE
    }

    /// Decode from `buf`. Validates layout_version.
    pub fn decode(buf: &[u8]) -> Result<Self, NipcError> {
        if buf.len() < CGROUPS_REQ_SIZE {
            return Err(NipcError::Truncated);
        }
        let r = CgroupsRequest {
            layout_version: u16::from_ne_bytes(buf[0..2].try_into().unwrap()),
            flags: u16::from_ne_bytes(buf[2..4].try_into().unwrap()),
        };
        if r.layout_version != 1 {
            return Err(NipcError::BadLayout);
        }
        // flags must be zero (reserved for future use)
        if r.flags != 0 {
            return Err(NipcError::BadLayout);
        }
        Ok(r)
    }
}

// ---------------------------------------------------------------------------
//  Cgroups snapshot response
// ---------------------------------------------------------------------------

/// Borrowed string view into the payload buffer.
/// Valid only while the underlying payload buffer is alive.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct StrView<'a> {
    /// Slice into payload, NUL-terminated.
    pub bytes: &'a [u8],
    /// Length excluding the NUL.
    pub len: u32,
}

impl<'a> StrView<'a> {
    /// Return the string content as a `&str`, or a UTF-8 error.
    pub fn as_str(&self) -> Result<&'a str, core::str::Utf8Error> {
        core::str::from_utf8(&self.bytes[..self.len as usize])
    }

    /// Return the string content as a byte slice (without the NUL).
    pub fn as_bytes(&self) -> &'a [u8] {
        &self.bytes[..self.len as usize]
    }
}

/// Per-item view -- ephemeral, borrows the payload buffer.
/// Valid only while the payload buffer is alive.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct CgroupsItemView<'a> {
    pub layout_version: u16,
    pub flags: u16,
    pub hash: u32,
    pub options: u32,
    pub enabled: u32,
    pub name: StrView<'a>,
    pub path: StrView<'a>,
}

/// Full snapshot view -- ephemeral, borrows the payload buffer.
/// Valid only during the current library call or callback.
/// Copy immediately if the data is needed later.
#[derive(Debug)]
pub struct CgroupsResponseView<'a> {
    pub layout_version: u16,
    pub flags: u16,
    pub item_count: u32,
    pub systemd_enabled: u32,
    pub generation: u64,
    payload: &'a [u8],
}

impl<'a> CgroupsResponseView<'a> {
    /// Decode the snapshot response header and validate the item directory.
    /// On success, use `item()` to access individual items.
    pub fn decode(buf: &'a [u8]) -> Result<Self, NipcError> {
        if buf.len() < CGROUPS_RESP_HDR_SIZE {
            return Err(NipcError::Truncated);
        }

        let layout_version = u16::from_ne_bytes(buf[0..2].try_into().unwrap());
        let flags = u16::from_ne_bytes(buf[2..4].try_into().unwrap());
        let item_count = u32::from_ne_bytes(buf[4..8].try_into().unwrap());
        let systemd_enabled = u32::from_ne_bytes(buf[8..12].try_into().unwrap());
        // buf[12..16] reserved, must be zero
        let reserved = u32::from_ne_bytes(buf[12..16].try_into().unwrap());
        let generation = u64::from_ne_bytes(buf[16..24].try_into().unwrap());

        if layout_version != 1 {
            return Err(NipcError::BadLayout);
        }

        // flags must be zero
        if flags != 0 {
            return Err(NipcError::BadLayout);
        }

        // reserved field must be zero
        if reserved != 0 {
            return Err(NipcError::BadLayout);
        }

        // Validate directory fits (checked arithmetic for 32-bit safety)
        let dir_size = (item_count as usize)
            .checked_mul(CGROUPS_DIR_ENTRY_SIZE)
            .ok_or(NipcError::BadLayout)?;
        let dir_end = CGROUPS_RESP_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::BadLayout)?;
        if dir_end > buf.len() {
            return Err(NipcError::Truncated);
        }

        let packed_area_len = buf.len() - dir_end;

        // Validate each directory entry
        for i in 0..item_count as usize {
            let base = CGROUPS_RESP_HDR_SIZE + i * 8;
            let off = u32::from_ne_bytes(buf[base..base + 4].try_into().unwrap());
            let len = u32::from_ne_bytes(buf[base + 4..base + 8].try_into().unwrap());

            if (off as usize) % ALIGNMENT != 0 {
                return Err(NipcError::BadAlignment);
            }
            if (off as u64) + (len as u64) > packed_area_len as u64 {
                return Err(NipcError::OutOfBounds);
            }
            if (len as usize) < CGROUPS_ITEM_HDR_SIZE {
                return Err(NipcError::Truncated);
            }
        }

        Ok(CgroupsResponseView {
            layout_version,
            flags,
            item_count,
            systemd_enabled,
            generation,
            payload: buf,
        })
    }

    /// Access item at `index`. Returns an ephemeral item view.
    pub fn item(&self, index: u32) -> Result<CgroupsItemView<'a>, NipcError> {
        if index >= self.item_count {
            return Err(NipcError::OutOfBounds);
        }

        let dir_start = CGROUPS_RESP_HDR_SIZE;
        let dir_size = self.item_count as usize * CGROUPS_DIR_ENTRY_SIZE;
        let packed_area_start = dir_start + dir_size;

        let dir_base = dir_start + index as usize * 8;
        let item_off = u32::from_ne_bytes(self.payload[dir_base..dir_base + 4].try_into().unwrap());
        let item_len =
            u32::from_ne_bytes(self.payload[dir_base + 4..dir_base + 8].try_into().unwrap());

        let item_start = packed_area_start + item_off as usize;
        let item = &self.payload[item_start..item_start + item_len as usize];

        let layout_version = u16::from_ne_bytes(item[0..2].try_into().unwrap());
        let flags = u16::from_ne_bytes(item[2..4].try_into().unwrap());
        let hash = u32::from_ne_bytes(item[4..8].try_into().unwrap());
        let options = u32::from_ne_bytes(item[8..12].try_into().unwrap());
        let enabled = u32::from_ne_bytes(item[12..16].try_into().unwrap());

        let name_off = u32::from_ne_bytes(item[16..20].try_into().unwrap()) as usize;
        let name_len = u32::from_ne_bytes(item[20..24].try_into().unwrap());
        let path_off = u32::from_ne_bytes(item[24..28].try_into().unwrap()) as usize;
        let path_len = u32::from_ne_bytes(item[28..32].try_into().unwrap());

        if layout_version != 1 {
            return Err(NipcError::BadLayout);
        }

        // item flags must be zero
        if flags != 0 {
            return Err(NipcError::BadLayout);
        }

        // Validate name string
        if name_off < CGROUPS_ITEM_HDR_SIZE {
            return Err(NipcError::OutOfBounds);
        }
        if (name_off as u64) + (name_len as u64) + 1 > item_len as u64 {
            return Err(NipcError::OutOfBounds);
        }
        if item[name_off + name_len as usize] != 0 {
            return Err(NipcError::MissingNul);
        }

        // Validate path string
        if path_off < CGROUPS_ITEM_HDR_SIZE {
            return Err(NipcError::OutOfBounds);
        }
        if (path_off as u64) + (path_len as u64) + 1 > item_len as u64 {
            return Err(NipcError::OutOfBounds);
        }
        if item[path_off + path_len as usize] != 0 {
            return Err(NipcError::MissingNul);
        }

        // Reject overlapping name and path regions (including NUL)
        {
            let name_start = name_off as u64;
            let name_end = name_start + name_len as u64 + 1;
            let path_start = path_off as u64;
            let path_end = path_start + path_len as u64 + 1;
            if name_start < path_end && path_start < name_end {
                return Err(NipcError::BadLayout);
            }
        }

        let name = StrView {
            bytes: &item[name_off..name_off + name_len as usize + 1],
            len: name_len,
        };
        let path = StrView {
            bytes: &item[path_off..path_off + path_len as usize + 1],
            len: path_len,
        };

        Ok(CgroupsItemView {
            layout_version,
            flags,
            hash,
            options,
            enabled,
            name,
            path,
        })
    }
}

// ---------------------------------------------------------------------------
//  Cgroups snapshot response builder
// ---------------------------------------------------------------------------

/// Builds a cgroups snapshot response payload.
///
/// Layout during building (max_items directory slots reserved):
///   [24-byte header space] [max_items*8 directory] [packed items]
///
/// Layout after finish (compacted to actual item_count):
///   [24-byte header] [item_count*8 directory] [packed items]
pub struct CgroupsBuilder<'a> {
    buf: &'a mut [u8],
    systemd_enabled: u32,
    generation: u64,
    item_count: u32,
    max_items: u32,
    data_offset: usize, // current write position (absolute in buf)
}

impl<'a> CgroupsBuilder<'a> {
    /// Initialize the builder. `buf` must be caller-owned and large enough
    /// for the expected snapshot.
    /// Initialize the builder. `buf` must be at least `CGROUPS_RESP_HDR_SIZE`
    /// (24) bytes plus `max_items * 8` directory bytes.
    ///
    /// # Panics
    /// Panics if `buf` is too small for the response header and item directory.
    pub fn new(buf: &'a mut [u8], max_items: u32, systemd_enabled: u32, generation: u64) -> Self {
        let dir_size = (max_items as usize).saturating_mul(CGROUPS_DIR_ENTRY_SIZE);
        let min_required = CGROUPS_RESP_HDR_SIZE.saturating_add(dir_size);
        assert!(
            buf.len() >= min_required,
            "CgroupsBuilder buffer too small: need at least {} bytes (hdr {} + dir {}), got {}",
            min_required,
            CGROUPS_RESP_HDR_SIZE,
            dir_size,
            buf.len()
        );
        let data_offset = min_required;
        CgroupsBuilder {
            buf,
            systemd_enabled,
            generation,
            item_count: 0,
            max_items,
            data_offset,
        }
    }

    /// Update the response header fields written by `finish()`.
    pub fn set_header(&mut self, systemd_enabled: u32, generation: u64) {
        self.systemd_enabled = systemd_enabled;
        self.generation = generation;
    }

    /// Add one cgroup item. Handles offset bookkeeping, NUL termination,
    /// and alignment.
    pub fn add(
        &mut self,
        hash: u32,
        options: u32,
        enabled: u32,
        name: &[u8],
        path: &[u8],
    ) -> Result<(), NipcError> {
        if self.item_count >= self.max_items {
            return Err(NipcError::Overflow);
        }

        // Align item start to 8 bytes
        let item_start = align8(self.data_offset);

        // Item payload: 32-byte header + name + NUL + path + NUL
        let item_size = CGROUPS_ITEM_HDR_SIZE
            .checked_add(name.len())
            .and_then(|v| v.checked_add(1))
            .and_then(|v| v.checked_add(path.len()))
            .and_then(|v| v.checked_add(1))
            .ok_or(NipcError::Overflow)?;

        let item_end = item_start
            .checked_add(item_size)
            .ok_or(NipcError::Overflow)?;
        if item_end > self.buf.len() {
            return Err(NipcError::Overflow);
        }

        // Zero alignment padding
        if item_start > self.data_offset {
            self.buf[self.data_offset..item_start].fill(0);
        }

        let name_offset = CGROUPS_ITEM_HDR_SIZE as u32;
        let path_offset = CGROUPS_ITEM_HDR_SIZE as u32 + name.len() as u32 + 1;

        // Write item header
        let p = item_start;
        self.buf[p..p + 2].copy_from_slice(&1u16.to_ne_bytes()); // layout_version
        self.buf[p + 2..p + 4].copy_from_slice(&0u16.to_ne_bytes()); // flags
        self.buf[p + 4..p + 8].copy_from_slice(&hash.to_ne_bytes());
        self.buf[p + 8..p + 12].copy_from_slice(&options.to_ne_bytes());
        self.buf[p + 12..p + 16].copy_from_slice(&enabled.to_ne_bytes());
        self.buf[p + 16..p + 20].copy_from_slice(&name_offset.to_ne_bytes());
        self.buf[p + 20..p + 24].copy_from_slice(&(name.len() as u32).to_ne_bytes());
        self.buf[p + 24..p + 28].copy_from_slice(&path_offset.to_ne_bytes());
        self.buf[p + 28..p + 32].copy_from_slice(&(path.len() as u32).to_ne_bytes());

        // Write strings with NUL terminators
        let name_start = p + name_offset as usize;
        self.buf[name_start..name_start + name.len()].copy_from_slice(name);
        self.buf[name_start + name.len()] = 0;

        let path_start = p + path_offset as usize;
        self.buf[path_start..path_start + path.len()].copy_from_slice(path);
        self.buf[path_start + path.len()] = 0;

        // Write directory entry (absolute offset stored temporarily)
        let dir_entry = CGROUPS_RESP_HDR_SIZE + self.item_count as usize * CGROUPS_DIR_ENTRY_SIZE;
        self.buf[dir_entry..dir_entry + 4].copy_from_slice(&(item_start as u32).to_ne_bytes());
        self.buf[dir_entry + 4..dir_entry + 8].copy_from_slice(&(item_size as u32).to_ne_bytes());

        self.data_offset = item_start + item_size;
        self.item_count += 1;
        Ok(())
    }

    /// Finalize the builder. Returns the total payload size. The buffer now
    /// contains a complete, decodable cgroups snapshot response payload.
    pub fn finish(self) -> usize {
        let p = &mut *{ self.buf };

        if self.item_count == 0 {
            p[0..2].copy_from_slice(&1u16.to_ne_bytes());
            p[2..4].copy_from_slice(&0u16.to_ne_bytes());
            p[4..8].copy_from_slice(&0u32.to_ne_bytes());
            p[8..12].copy_from_slice(&self.systemd_enabled.to_ne_bytes());
            p[12..16].copy_from_slice(&0u32.to_ne_bytes());
            p[16..24].copy_from_slice(&self.generation.to_ne_bytes());
            return CGROUPS_RESP_HDR_SIZE;
        }

        // Where the decoder expects packed data to start
        let final_packed_start =
            CGROUPS_RESP_HDR_SIZE + self.item_count as usize * CGROUPS_DIR_ENTRY_SIZE;

        // Read the first directory entry to find where packed data begins
        let first_item_abs = u32::from_ne_bytes(
            p[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
                .try_into()
                .unwrap(),
        ) as usize;

        let packed_data_len = self.data_offset - first_item_abs;

        if final_packed_start < first_item_abs {
            // Shift packed data left
            p.copy_within(
                first_item_abs..first_item_abs + packed_data_len,
                final_packed_start,
            );
        }

        // Convert directory entries from absolute to relative offsets
        let dir_base = CGROUPS_RESP_HDR_SIZE;
        for i in 0..self.item_count as usize {
            let entry = dir_base + i * CGROUPS_DIR_ENTRY_SIZE;
            let abs_off = u32::from_ne_bytes(p[entry..entry + 4].try_into().unwrap());
            let rel_off = abs_off - first_item_abs as u32;
            p[entry..entry + 4].copy_from_slice(&rel_off.to_ne_bytes());
            // length stays the same
        }

        // Write snapshot header
        p[0..2].copy_from_slice(&1u16.to_ne_bytes());
        p[2..4].copy_from_slice(&0u16.to_ne_bytes());
        p[4..8].copy_from_slice(&self.item_count.to_ne_bytes());
        p[8..12].copy_from_slice(&self.systemd_enabled.to_ne_bytes());
        p[12..16].copy_from_slice(&0u32.to_ne_bytes());
        p[16..24].copy_from_slice(&self.generation.to_ne_bytes());

        final_packed_start + packed_data_len
    }
}

/// Estimate a safe upper bound for the number of cgroup items that can fit in
/// `buf_size`. This is a reservation hint for the builder, not a guarantee for
/// arbitrary string lengths.
pub fn estimate_cgroups_max_items(buf_size: usize) -> u32 {
    if buf_size <= CGROUPS_RESP_HDR_SIZE {
        return 0;
    }

    let min_aligned_item = align8(CGROUPS_ITEM_HDR_SIZE + 2);
    ((buf_size - CGROUPS_RESP_HDR_SIZE) / (CGROUPS_DIR_ENTRY_SIZE + min_aligned_item)) as u32
}

/// CGROUPS_SNAPSHOT dispatch: decode request, build response via handler.
pub fn dispatch_cgroups_snapshot<F>(
    req: &[u8],
    resp: &mut [u8],
    max_items: u32,
    handler: F,
) -> Option<usize>
where
    F: FnOnce(&CgroupsRequest, &mut CgroupsBuilder) -> bool,
{
    let request = CgroupsRequest::decode(req).ok()?;
    let min_required = (max_items as usize)
        .checked_mul(CGROUPS_DIR_ENTRY_SIZE)
        .and_then(|d| d.checked_add(CGROUPS_RESP_HDR_SIZE));
    let min_required = match min_required {
        Some(v) => v,
        None => return None,
    };
    if resp.len() < min_required {
        return None;
    }
    let mut builder = CgroupsBuilder::new(resp, max_items, 0, 0);
    if !handler(&request, &mut builder) {
        return None;
    }
    Some(builder.finish())
}

// ---------------------------------------------------------------------------
//  Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cgroups_item_overlapping_regions() {
        let mut buf = [0u8; 4096];
        let mut b = CgroupsBuilder::new(&mut buf, 1, 0, 1);
        b.add(1, 0, 1, b"test", b"/test").unwrap();
        let total = b.finish();

        let dir_end = CGROUPS_RESP_HDR_SIZE + CGROUPS_DIR_ENTRY_SIZE;
        let item_off = u32::from_ne_bytes(
            buf[CGROUPS_RESP_HDR_SIZE..CGROUPS_RESP_HDR_SIZE + 4]
                .try_into()
                .unwrap(),
        ) as usize;
        let item_start = dir_end + item_off;

        let name_off =
            u32::from_ne_bytes(buf[item_start + 16..item_start + 20].try_into().unwrap());
        buf[item_start + 24..item_start + 28].copy_from_slice(&name_off.to_ne_bytes());
        let name_len =
            u32::from_ne_bytes(buf[item_start + 20..item_start + 24].try_into().unwrap());
        buf[item_start + 28..item_start + 32].copy_from_slice(&name_len.to_ne_bytes());

        let view = CgroupsResponseView::decode(&buf[..total]).unwrap();
        assert_eq!(view.item(0).unwrap_err(), NipcError::BadLayout);
    }

    #[test]
    fn dispatch_cgroups_empty_finish_returns_some_header_only() {
        let req = CgroupsRequest {
            layout_version: 1,
            flags: 0,
        };
        let mut req_buf = [0u8; 4];
        req.encode(&mut req_buf);
        let mut resp = [0u8; 4096];
        let result = dispatch_cgroups_snapshot(&req_buf, &mut resp, 1, |_req, _builder| true);
        assert!(result.is_some());
        assert_eq!(result.unwrap(), CGROUPS_RESP_HDR_SIZE);
    }
}
