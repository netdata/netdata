//! CGROUPS_LOOKUP codec.

use super::common::*;
use crate::protocol::{align8, NipcError, StrView};

pub const CGROUP_LOOKUP_KNOWN: u16 = 0;
pub const CGROUP_LOOKUP_UNKNOWN_RETRY_LATER: u16 = 1;
pub const CGROUP_LOOKUP_UNKNOWN_PERMANENT: u16 = 2;
pub const CGROUP_LOOKUP_PAYLOAD_EXCEEDED: u16 = 3;
pub const CGROUP_LOOKUP_OVERSIZED_ITEM: u16 = 4;

pub const CGROUPS_LOOKUP_REQ_HDR_SIZE: usize = 16;
pub const CGROUPS_LOOKUP_RESP_HDR_SIZE: usize = 16;
pub const CGROUPS_LOOKUP_ITEM_HDR_SIZE: usize = 28;

#[derive(Debug)]
pub struct CgroupsLookupRequestView<'a> {
    pub item_count: u32,
    packed_start: usize,
    payload: &'a [u8],
}

#[derive(Debug)]
pub struct CgroupsLookupResponseView<'a> {
    pub layout_version: u16,
    pub flags: u16,
    pub item_count: u32,
    pub generation: u64,
    payload: &'a [u8],
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct CgroupsLookupItemView<'a> {
    pub status: u16,
    pub orchestrator: u16,
    pub path: StrView<'a>,
    pub name: StrView<'a>,
    pub label_count: u16,
    item: &'a [u8],
    label_table_offset: usize,
}

fn validate_cgroups_lookup_semantics(
    status: u16,
    orchestrator: u16,
    path_len: u64,
    name_len: u64,
    label_count: u64,
) -> Result<(), NipcError> {
    if status != CGROUP_LOOKUP_KNOWN
        && status != CGROUP_LOOKUP_UNKNOWN_RETRY_LATER
        && status != CGROUP_LOOKUP_UNKNOWN_PERMANENT
        && status != CGROUP_LOOKUP_PAYLOAD_EXCEEDED
        && status != CGROUP_LOOKUP_OVERSIZED_ITEM
    {
        return Err(NipcError::BadLayout);
    }
    if path_len == 0 {
        return Err(NipcError::BadLayout);
    }
    if status != CGROUP_LOOKUP_KNOWN && (orchestrator != 0 || name_len != 0 || label_count != 0) {
        return Err(NipcError::BadLayout);
    }
    Ok(())
}

pub fn encode_cgroups_lookup_request(paths: &[&[u8]], buf: &mut [u8]) -> Result<usize, NipcError> {
    let count = paths.len();
    let dir_size = count
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(NipcError::Overflow)?;
    let packed_start = CGROUPS_LOOKUP_REQ_HDR_SIZE
        .checked_add(dir_size)
        .ok_or(NipcError::Overflow)?;
    if buf.len() < packed_start {
        return Err(NipcError::Overflow);
    }

    let mut data = packed_start;
    for (i, path) in paths.iter().enumerate() {
        if source_string_invalid(path, true) {
            return Err(NipcError::BadLayout);
        }
        let aligned = align8(data);
        let key_len = path.len().checked_add(1).ok_or(NipcError::Overflow)?;
        let end = aligned.checked_add(key_len).ok_or(NipcError::Overflow)?;
        if end > buf.len() {
            return Err(NipcError::Overflow);
        }
        if aligned > data {
            buf[data..aligned].fill(0);
        }

        let dir = CGROUPS_LOOKUP_REQ_HDR_SIZE + i * LOOKUP_DIR_ENTRY_SIZE;
        put_u32(buf, dir, checked_u32(aligned - packed_start)?);
        put_u32(buf, dir + 4, checked_u32(key_len)?);
        buf[aligned..aligned + path.len()].copy_from_slice(path);
        buf[aligned + path.len()] = 0;
        data = end;
    }

    put_u16(buf, 0, 1);
    put_u16(buf, 2, 0);
    put_u32(buf, 4, checked_u32(count)?);
    put_u32(buf, 8, 0);
    put_u32(buf, 12, 0);
    Ok(data)
}

impl<'a> CgroupsLookupRequestView<'a> {
    pub fn decode(buf: &'a [u8]) -> Result<Self, NipcError> {
        if buf.len() < CGROUPS_LOOKUP_REQ_HDR_SIZE {
            return Err(NipcError::Truncated);
        }
        if u16_at(buf, 0) != 1 || u16_at(buf, 2) != 0 || u32_at(buf, 8) != 0 || u32_at(buf, 12) != 0
        {
            return Err(NipcError::BadLayout);
        }
        let item_count = u32_at(buf, 4);
        let dir_size = (item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::BadItemCount)?;
        let dir_end = CGROUPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::BadItemCount)?;
        if dir_end > buf.len() {
            return Err(NipcError::Truncated);
        }
        let packed_len = buf.len() - dir_end;
        validate_lookup_dir(
            buf,
            CGROUPS_LOOKUP_REQ_HDR_SIZE,
            item_count,
            packed_len,
            2,
            None,
        )?;
        for i in 0..item_count as usize {
            let base = CGROUPS_LOOKUP_REQ_HDR_SIZE + i * LOOKUP_DIR_ENTRY_SIZE;
            let off = u32_at(buf, base) as usize;
            let len = u32_at(buf, base + 4) as usize;
            let key = checked_subslice(buf, dir_end, off, len)?;
            if key[len - 1] != 0 {
                return Err(NipcError::MissingNul);
            }
            if key[..len - 1].contains(&0) {
                return Err(NipcError::BadLayout);
            }
        }
        Ok(Self {
            item_count,
            packed_start: dir_end,
            payload: buf,
        })
    }

    pub fn item(&self, index: u32) -> Result<StrView<'a>, NipcError> {
        if index >= self.item_count {
            return Err(NipcError::OutOfBounds);
        }
        let base = CGROUPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(
                (index as usize)
                    .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
                    .ok_or(NipcError::BadItemCount)?,
            )
            .ok_or(NipcError::BadItemCount)?;
        let off = u32_at(self.payload, base) as usize;
        let len = u32_at(self.payload, base + 4) as usize;
        Ok(StrView {
            bytes: checked_subslice(self.payload, self.packed_start, off, len)?,
            len: (len - 1) as u32,
        })
    }

    fn payload_exceeded_item_len(&self, index: u32) -> Result<usize, NipcError> {
        if index >= self.item_count {
            return Err(NipcError::OutOfBounds);
        }
        let base = CGROUPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(
                (index as usize)
                    .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
                    .ok_or(NipcError::BadItemCount)?,
            )
            .ok_or(NipcError::BadItemCount)?;
        let path_wire_len = u32_at(self.payload, base + 4) as usize;
        CGROUPS_LOOKUP_ITEM_HDR_SIZE
            .checked_add(path_wire_len)
            .and_then(|v| v.checked_add(1))
            .ok_or(NipcError::Overflow)
    }
}

impl<'a> CgroupsLookupResponseView<'a> {
    pub fn decode(buf: &'a [u8]) -> Result<Self, NipcError> {
        if buf.len() < CGROUPS_LOOKUP_RESP_HDR_SIZE {
            return Err(NipcError::Truncated);
        }
        if u16_at(buf, 0) != 1 || u16_at(buf, 2) != 0 {
            return Err(NipcError::BadLayout);
        }
        let item_count = u32_at(buf, 4);
        let dir_size = (item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::BadItemCount)?;
        let dir_end = CGROUPS_LOOKUP_RESP_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::BadItemCount)?;
        if dir_end > buf.len() {
            return Err(NipcError::Truncated);
        }
        validate_lookup_dir(
            buf,
            CGROUPS_LOOKUP_RESP_HDR_SIZE,
            item_count,
            buf.len() - dir_end,
            CGROUPS_LOOKUP_ITEM_HDR_SIZE,
            None,
        )?;
        for i in 0..item_count as usize {
            let base = CGROUPS_LOOKUP_RESP_HDR_SIZE + i * LOOKUP_DIR_ENTRY_SIZE;
            let off = u32_at(buf, base) as usize;
            let len = u32_at(buf, base + 4) as usize;
            decode_cgroups_item(checked_subslice(buf, dir_end, off, len)?)?;
        }
        Ok(Self {
            layout_version: 1,
            flags: 0,
            item_count,
            generation: u64_at(buf, 8),
            payload: buf,
        })
    }

    pub fn item(&self, index: u32) -> Result<CgroupsLookupItemView<'a>, NipcError> {
        if index >= self.item_count {
            return Err(NipcError::OutOfBounds);
        }
        let packed_start = lookup_data_offset(CGROUPS_LOOKUP_RESP_HDR_SIZE, self.item_count)?;
        let base = lookup_dir_entry_offset(CGROUPS_LOOKUP_RESP_HDR_SIZE, index)?;
        let off = u32_at(self.payload, base) as usize;
        let len = u32_at(self.payload, base + 4) as usize;
        decode_cgroups_item(checked_subslice(self.payload, packed_start, off, len)?)
    }

    pub fn raw_item(&self, index: u32) -> Result<&'a [u8], NipcError> {
        response_raw_item(
            self.payload,
            CGROUPS_LOOKUP_RESP_HDR_SIZE,
            self.item_count,
            index,
        )
    }
}

pub fn encode_cgroups_lookup_raw_response(
    items: &[&[u8]],
    generation: u64,
    buf: &mut [u8],
) -> Result<usize, NipcError> {
    encode_raw_response(
        buf,
        CGROUPS_LOOKUP_RESP_HDR_SIZE,
        generation,
        items,
        CGROUPS_LOOKUP_ITEM_HDR_SIZE,
        validate_cgroups_lookup_raw_item,
    )
}

fn validate_cgroups_lookup_raw_item(item: &[u8]) -> Result<(), NipcError> {
    decode_cgroups_item(item).map(|_| ())
}

fn decode_cgroups_item(item: &[u8]) -> Result<CgroupsLookupItemView<'_>, NipcError> {
    if item.len() < CGROUPS_LOOKUP_ITEM_HDR_SIZE {
        return Err(NipcError::Truncated);
    }
    let status = u16_at(item, 2);
    let orchestrator = u16_at(item, 4);
    let path_off = u32_at(item, 8) as usize;
    let path_len = u32_at(item, 12) as usize;
    let name_off = u32_at(item, 16) as usize;
    let name_len = u32_at(item, 20) as usize;
    let label_count = u16_at(item, 24);

    if u16_at(item, 0) != 1 || u16_at(item, 6) != 0 || u16_at(item, 26) != 0 {
        return Err(NipcError::BadLayout);
    }
    validate_cgroups_lookup_semantics(
        status,
        orchestrator,
        path_len as u64,
        name_len as u64,
        label_count as u64,
    )?;

    let (path, path_end) = lookup_string(item, CGROUPS_LOOKUP_ITEM_HDR_SIZE, path_off, path_len)?;
    let (name, name_end) = lookup_string(item, CGROUPS_LOOKUP_ITEM_HDR_SIZE, name_off, name_len)?;
    if overlap(path_off, path_end, name_off, name_end) {
        return Err(NipcError::BadLayout);
    }
    let label_table_offset = validate_labels(
        item,
        CGROUPS_LOOKUP_ITEM_HDR_SIZE,
        label_count,
        path_end.max(name_end),
    )?;
    Ok(CgroupsLookupItemView {
        status,
        orchestrator,
        path,
        name,
        label_count,
        item,
        label_table_offset,
    })
}

impl<'a> CgroupsLookupItemView<'a> {
    pub fn label(&self, index: u32) -> Result<LookupLabelView<'a>, NipcError> {
        label_at(
            self.item,
            CGROUPS_LOOKUP_ITEM_HDR_SIZE,
            self.label_count,
            self.label_table_offset,
            index,
        )
    }
}

pub struct CgroupsLookupBuilder<'a> {
    buf: &'a mut [u8],
    generation: u64,
    item_count: u32,
    max_items: u32,
    data_offset: usize,
    error: Option<NipcError>,
    payload_exceeded_suffix: bool,
    payload_exceeded_suffix_bytes: Vec<u32>,
}

impl<'a> CgroupsLookupBuilder<'a> {
    pub fn new(buf: &'a mut [u8], max_items: u32, generation: u64) -> Self {
        let data_offset = (max_items as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .and_then(|v| CGROUPS_LOOKUP_RESP_HDR_SIZE.checked_add(v))
            .expect("CgroupsLookupBuilder buffer too small");
        assert!(
            buf.len() >= data_offset,
            "CgroupsLookupBuilder buffer too small"
        );
        Self {
            buf,
            generation,
            item_count: 0,
            max_items,
            data_offset,
            error: None,
            payload_exceeded_suffix: false,
            payload_exceeded_suffix_bytes: Vec::new(),
        }
    }

    pub fn set_generation(&mut self, generation: u64) {
        self.generation = generation;
    }

    pub fn set_payload_exceeded_item_lens(&mut self, item_lens: Vec<usize>) {
        match payload_exceeded_suffix_bytes_from_lens(item_lens) {
            Ok(suffix_bytes) => self.payload_exceeded_suffix_bytes = suffix_bytes,
            Err(err) => self.error = Some(err),
        }
    }

    pub fn add(
        &mut self,
        status: u16,
        orchestrator: u16,
        path: &[u8],
        name: &[u8],
        labels: &[(&[u8], &[u8])],
    ) -> Result<(), NipcError> {
        self.add_with_path(status, orchestrator, path, name, labels, false)
    }

    pub fn add_request_item(
        &mut self,
        request: &CgroupsLookupRequestView,
        index: u32,
        status: u16,
        orchestrator: u16,
        name: &[u8],
        labels: &[(&[u8], &[u8])],
    ) -> Result<(), NipcError> {
        let path = request.item(index)?;
        self.add_with_path(status, orchestrator, path.as_bytes(), name, labels, true)
    }

    fn add_with_path(
        &mut self,
        status: u16,
        orchestrator: u16,
        path: &[u8],
        name: &[u8],
        labels: &[(&[u8], &[u8])],
        path_validated: bool,
    ) -> Result<(), NipcError> {
        if self.payload_exceeded_suffix {
            return self.add_non_known_item(CGROUP_LOOKUP_PAYLOAD_EXCEEDED, path);
        }
        if self.item_count >= self.max_items {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        }
        if let Err(err) = validate_cgroups_lookup_semantics(
            status,
            orchestrator,
            path.len() as u64,
            name.len() as u64,
            labels.len() as u64,
        ) {
            self.error = Some(err);
            return Err(err);
        }
        if (!path_validated && source_string_invalid(path, true))
            || source_string_invalid(name, false)
        {
            self.error = Some(NipcError::BadLayout);
            return Err(NipcError::BadLayout);
        }
        if status != CGROUP_LOOKUP_KNOWN {
            return match self.add_non_known_item(status, path) {
                Err(NipcError::Overflow) => self.note_item_overflow(path),
                other => other,
            };
        }
        let label_count = match checked_u16(labels.len()) {
            Ok(v) => v,
            Err(err) => {
                if err == NipcError::Overflow {
                    return self.note_item_overflow(path);
                }
                self.error = Some(err);
                return Err(err);
            }
        };

        let item_start = align8(self.data_offset);
        let path_offset = CGROUPS_LOOKUP_ITEM_HDR_SIZE;
        let Some(name_offset) = path_offset
            .checked_add(path.len())
            .and_then(|v| v.checked_add(1))
        else {
            return self.note_item_overflow(path);
        };
        let Some(fixed_end) = name_offset
            .checked_add(name.len())
            .and_then(|v| v.checked_add(1))
        else {
            return self.note_item_overflow(path);
        };
        let (table_start, table_bytes, mut item_size) = match label_layout(fixed_end, labels) {
            Ok(v) => v,
            Err(NipcError::Overflow) => return self.note_item_overflow(path),
            Err(err) => {
                self.error = Some(err);
                return Err(err);
            }
        };
        let item_end = match item_start.checked_add(item_size) {
            Some(v) if v <= self.buf.len() => v,
            _ => return self.note_item_overflow(path),
        };
        if !payload_exceeded_suffix_fits(
            self.buf.len(),
            item_end,
            &self.payload_exceeded_suffix_bytes,
            self.item_count + 1,
            self.max_items,
        ) {
            return self.note_item_overflow(path);
        }
        if item_start > self.data_offset {
            self.buf[self.data_offset..item_start].fill(0);
        }
        let item = &mut self.buf[item_start..item_end];
        if let Err(err) = write_cgroups_item_header(
            item,
            status,
            orchestrator,
            path_offset,
            path.len(),
            name_offset,
            name.len(),
            label_count,
        ) {
            self.error = Some(err);
            return Err(err);
        }
        item[path_offset..path_offset + path.len()].copy_from_slice(path);
        item[path_offset + path.len()] = 0;
        item[name_offset..name_offset + name.len()].copy_from_slice(name);
        item[name_offset + name.len()] = 0;
        if !labels.is_empty() {
            item[fixed_end..table_start].fill(0);
            item_size = write_lookup_labels(item, table_start, table_bytes, labels)?;
        }
        let dir_offset = (self.item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let dir = CGROUPS_LOOKUP_RESP_HDR_SIZE
            .checked_add(dir_offset)
            .ok_or(NipcError::Overflow)?;
        put_u32(self.buf, dir, checked_u32(item_start)?);
        put_u32(self.buf, dir + 4, checked_u32(item_size)?);
        self.data_offset = item_end;
        self.item_count += 1;
        self.error = None;
        Ok(())
    }

    fn add_non_known_item(&mut self, status: u16, path: &[u8]) -> Result<(), NipcError> {
        if self.item_count >= self.max_items {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        }
        let item_start = align8(self.data_offset);
        let Some(name_offset) = CGROUPS_LOOKUP_ITEM_HDR_SIZE
            .checked_add(path.len())
            .and_then(|v| v.checked_add(1))
        else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let Some(item_size) = name_offset.checked_add(1) else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let Some(item_end) = item_start.checked_add(item_size) else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        if item_end > self.buf.len() {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        }
        if item_start > self.data_offset {
            self.buf[self.data_offset..item_start].fill(0);
        }
        let item = &mut self.buf[item_start..item_end];
        if let Err(err) = write_cgroups_item_header(
            item,
            status,
            0,
            CGROUPS_LOOKUP_ITEM_HDR_SIZE,
            path.len(),
            name_offset,
            0,
            0,
        ) {
            self.error = Some(err);
            return Err(err);
        }
        item[CGROUPS_LOOKUP_ITEM_HDR_SIZE..CGROUPS_LOOKUP_ITEM_HDR_SIZE + path.len()]
            .copy_from_slice(path);
        item[CGROUPS_LOOKUP_ITEM_HDR_SIZE + path.len()] = 0;
        item[name_offset] = 0;
        let Some(dir_offset) = (self.item_count as usize).checked_mul(LOOKUP_DIR_ENTRY_SIZE) else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let Some(dir) = CGROUPS_LOOKUP_RESP_HDR_SIZE.checked_add(dir_offset) else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let item_start_u32 = match checked_u32(item_start) {
            Ok(v) => v,
            Err(err) => {
                self.error = Some(err);
                return Err(err);
            }
        };
        let item_size_u32 = match checked_u32(item_size) {
            Ok(v) => v,
            Err(err) => {
                self.error = Some(err);
                return Err(err);
            }
        };
        put_u32(self.buf, dir, item_start_u32);
        put_u32(self.buf, dir + 4, item_size_u32);
        self.data_offset = item_end;
        self.item_count += 1;
        self.error = None;
        Ok(())
    }

    fn note_item_overflow(&mut self, path: &[u8]) -> Result<(), NipcError> {
        if self.item_count == 0 {
            return self.add_non_known_item(CGROUP_LOOKUP_OVERSIZED_ITEM, path);
        }
        self.payload_exceeded_suffix = true;
        self.add_non_known_item(CGROUP_LOOKUP_PAYLOAD_EXCEEDED, path)
    }

    pub fn finish(self) -> Result<usize, NipcError> {
        finish_lookup_response(
            self.buf,
            CGROUPS_LOOKUP_RESP_HDR_SIZE,
            self.item_count,
            self.data_offset,
            self.generation,
        )
    }

    pub fn error(&self) -> Option<NipcError> {
        self.error
    }

    pub fn item_count(&self) -> u32 {
        self.item_count
    }
}

fn write_cgroups_item_header(
    item: &mut [u8],
    status: u16,
    orchestrator: u16,
    path_off: usize,
    path_len: usize,
    name_off: usize,
    name_len: usize,
    label_count: u16,
) -> Result<(), NipcError> {
    put_u16(item, 0, 1);
    put_u16(item, 2, status);
    put_u16(item, 4, orchestrator);
    put_u16(item, 6, 0);
    put_u32(item, 8, checked_u32(path_off)?);
    put_u32(item, 12, checked_u32(path_len)?);
    put_u32(item, 16, checked_u32(name_off)?);
    put_u32(item, 20, checked_u32(name_len)?);
    put_u16(item, 24, label_count);
    put_u16(item, 26, 0);
    Ok(())
}

pub fn dispatch_cgroups_lookup<F>(
    req: &[u8],
    resp: &mut [u8],
    handler: F,
) -> Result<usize, NipcError>
where
    F: FnOnce(&CgroupsLookupRequestView, &mut CgroupsLookupBuilder) -> bool,
{
    let request = CgroupsLookupRequestView::decode(req)?;
    let min_required = lookup_data_offset(CGROUPS_LOOKUP_RESP_HDR_SIZE, request.item_count)
        .map_err(|_| NipcError::Overflow)?;
    if resp.len() < min_required {
        return Err(NipcError::Overflow);
    }
    let mut builder = CgroupsLookupBuilder::new(resp, request.item_count, 0);
    if request.item_count > 0 {
        let item_count = request.item_count as usize;
        let capacity = item_count.checked_add(1).ok_or(NipcError::Overflow)?;
        let mut payload_exceeded_item_lens = Vec::with_capacity(capacity);
        for i in 0..request.item_count {
            payload_exceeded_item_lens.push(request.payload_exceeded_item_len(i)?);
        }
        builder.set_payload_exceeded_item_lens(payload_exceeded_item_lens);
        if let Some(err) = builder.error {
            return Err(err);
        }
    }
    if !handler(&request, &mut builder) {
        return Err(builder.error().unwrap_or(NipcError::HandlerFailed));
    }
    if let Some(err) = builder.error() {
        return Err(err);
    }
    if builder.item_count != request.item_count {
        return Err(NipcError::BadItemCount);
    }
    builder.finish()
}

#[cfg(test)]
mod tests {
    use super::super::common::{
        put_u16, put_u32, response_item_bounds, u32_at, LOOKUP_DIR_ENTRY_SIZE,
    };
    use super::*;
    use crate::protocol::{align8, ORCHESTRATOR_K8S};

    #[test]
    fn cgroups_lookup_request_roundtrip() {
        let mut buf = [0u8; 128];
        let n = encode_cgroups_lookup_request(&[b"/a/b", b"/c"], &mut buf).unwrap();
        let view = CgroupsLookupRequestView::decode(&buf[..n]).unwrap();
        assert_eq!(view.item_count, 2);
        assert_eq!(view.item(0).unwrap().as_bytes(), b"/a/b");
        assert_eq!(view.item(1).unwrap().as_bytes(), b"/c");
    }

    #[test]
    fn cgroups_lookup_builder_add_request_item() {
        let mut req = [0u8; 128];
        let req_len = encode_cgroups_lookup_request(&[b"/request-path"], &mut req).unwrap();
        let request = CgroupsLookupRequestView::decode(&req[..req_len]).unwrap();

        let mut resp = [0u8; 256];
        let mut builder = CgroupsLookupBuilder::new(&mut resp, 1, 42);
        builder
            .add_request_item(
                &request,
                0,
                CGROUP_LOOKUP_KNOWN,
                ORCHESTRATOR_K8S,
                b"pod-a",
                &[],
            )
            .unwrap();
        let n = builder.finish().unwrap();
        let view = CgroupsLookupResponseView::decode(&resp[..n]).unwrap();
        let item = view.item(0).unwrap();
        assert_eq!(item.path.as_bytes(), b"/request-path");
        assert_eq!(item.name.as_bytes(), b"pod-a");

        let mut resp = [0u8; 256];
        let mut builder = CgroupsLookupBuilder::new(&mut resp, 1, 42);
        assert_eq!(
            builder.add_request_item(
                &request,
                request.item_count,
                CGROUP_LOOKUP_KNOWN,
                ORCHESTRATOR_K8S,
                b"pod-a",
                &[],
            ),
            Err(NipcError::OutOfBounds)
        );
    }

    #[test]
    fn cgroups_lookup_response_labels_roundtrip() {
        let mut buf = [0u8; 512];
        let mut b = CgroupsLookupBuilder::new(&mut buf, 1, 99);
        b.add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_K8S,
            b"/kubepod",
            b"pod-a",
            &[(b"namespace".as_slice(), b"default".as_slice())],
        )
        .unwrap();
        let n = b.finish().unwrap();
        let view = CgroupsLookupResponseView::decode(&buf[..n]).unwrap();
        assert_eq!(view.generation, 99);
        let item = view.item(0).unwrap();
        assert_eq!(item.path.as_bytes(), b"/kubepod");
        assert_eq!(item.name.as_bytes(), b"pod-a");
        let label = item.label(0).unwrap();
        assert_eq!(label.key.as_bytes(), b"namespace");
        assert_eq!(label.value.as_bytes(), b"default");
    }

    fn build_cgroups_lookup_boundary_response(buf: &mut [u8]) -> usize {
        let mut b = CgroupsLookupBuilder::new(buf, 2, 333);
        b.add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_K8S,
            b"/kubepods/pod-a",
            b"pod-a",
            &[],
        )
        .unwrap();
        b.add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_K8S,
            b"/kubepods/long-pod-path",
            b"long-pod-name",
            &[(b"namespace".as_slice(), b"default".as_slice())],
        )
        .unwrap();
        b.finish().unwrap()
    }

    #[test]
    fn cgroups_lookup_response_exact_and_short_boundary() {
        let mut large = vec![0u8; 1024];
        let exact_len = build_cgroups_lookup_boundary_response(&mut large);
        assert!(exact_len > CGROUPS_LOOKUP_RESP_HDR_SIZE + 2 * LOOKUP_DIR_ENTRY_SIZE);

        let mut exact = vec![0u8; exact_len];
        assert_eq!(
            build_cgroups_lookup_boundary_response(&mut exact),
            exact_len
        );
        let view = CgroupsLookupResponseView::decode(&exact).unwrap();
        let item = view.item(1).unwrap();
        assert_eq!(item.status, CGROUP_LOOKUP_KNOWN);
        assert_eq!(item.path.as_bytes(), b"/kubepods/long-pod-path");

        let mut plus_one = vec![0u8; exact_len + 1];
        assert_eq!(
            build_cgroups_lookup_boundary_response(&mut plus_one),
            exact_len
        );
        let view = CgroupsLookupResponseView::decode(&plus_one[..exact_len]).unwrap();
        let item = view.item(1).unwrap();
        assert_eq!(item.status, CGROUP_LOOKUP_KNOWN);
        assert_eq!(item.path.as_bytes(), b"/kubepods/long-pod-path");

        let mut short = vec![0u8; exact_len - 1];
        let short_len = build_cgroups_lookup_boundary_response(&mut short);
        let view = CgroupsLookupResponseView::decode(&short[..short_len]).unwrap();
        let item = view.item(1).unwrap();
        assert_eq!(item.status, CGROUP_LOOKUP_PAYLOAD_EXCEEDED);
        assert_eq!(item.path.as_bytes(), b"/kubepods/long-pod-path");
    }

    fn cgroups_lookup_labeled_response() -> Vec<u8> {
        let mut buf = vec![0u8; 512];
        let mut b = CgroupsLookupBuilder::new(&mut buf, 1, 1);
        b.add(
            CGROUP_LOOKUP_KNOWN,
            ORCHESTRATOR_K8S,
            b"/x",
            b"n",
            &[(b"k".as_slice(), b"v".as_slice())],
        )
        .unwrap();
        let n = b.finish().unwrap();
        buf.truncate(n);
        buf
    }

    #[test]
    fn cgroups_lookup_empty_request_response() {
        let mut buf = [0u8; 64];
        let n = encode_cgroups_lookup_request(&[], &mut buf).unwrap();
        let view = CgroupsLookupRequestView::decode(&buf[..n]).unwrap();
        assert_eq!(view.item_count, 0);

        let mut cbuf = [0u8; 64];
        let b = CgroupsLookupBuilder::new(&mut cbuf, 0, 9);
        let n = b.finish().unwrap();
        let view = CgroupsLookupResponseView::decode(&cbuf[..n]).unwrap();
        assert_eq!(view.item_count, 0);
        assert_eq!(view.generation, 9);
    }

    #[test]
    fn cgroups_lookup_dispatch_rejects_short_response_buffer() {
        let mut req = [0u8; 64];
        let n = encode_cgroups_lookup_request(&[b"/x"], &mut req).unwrap();
        let mut resp = vec![0u8; CGROUPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE - 1];
        assert_eq!(
            dispatch_cgroups_lookup(&req[..n], &mut resp, |_, _| {
                panic!("handler must not run with undersized response buffer")
            })
            .unwrap_err(),
            NipcError::Overflow
        );
    }

    #[test]
    fn cgroups_lookup_dispatch_reports_handler_failed() {
        let mut req = [0u8; 64];
        let n = encode_cgroups_lookup_request(&[b"/x"], &mut req).unwrap();
        let mut resp = vec![0u8; 256];
        assert_eq!(
            dispatch_cgroups_lookup(&req[..n], &mut resp, |_, _| false).unwrap_err(),
            NipcError::HandlerFailed
        );
    }

    #[test]
    fn cgroups_lookup_request_rejects_bad_layouts() {
        let mut buf = [0u8; 128];
        let n = encode_cgroups_lookup_request(&[b"/x"], &mut buf).unwrap();
        assert_eq!(
            CgroupsLookupRequestView::decode(&buf[..CGROUPS_LOOKUP_REQ_HDR_SIZE - 1]).unwrap_err(),
            NipcError::Truncated
        );

        let mut bad = buf[..n].to_vec();
        put_u16(&mut bad, 0, 99);
        assert_eq!(
            CgroupsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(&mut bad, 8, 1);
        assert_eq!(
            CgroupsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(&mut bad, CGROUPS_LOOKUP_REQ_HDR_SIZE, 1);
        assert_eq!(
            CgroupsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadAlignment
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(&mut bad, CGROUPS_LOOKUP_REQ_HDR_SIZE + 4, 4096);
        assert_eq!(
            CgroupsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::OutOfBounds
        );

        bad.copy_from_slice(&buf[..n]);
        let last = bad.len() - 1;
        bad[last] = b'x';
        assert_eq!(
            CgroupsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::MissingNul
        );

        bad.copy_from_slice(&buf[..n]);
        bad[CGROUPS_LOOKUP_REQ_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE] = 0;
        assert_eq!(
            CgroupsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );
    }

    #[test]
    fn cgroups_lookup_response_rejects_bad_layouts() {
        let buf = cgroups_lookup_labeled_response();
        assert_eq!(
            CgroupsLookupResponseView::decode(&buf[..CGROUPS_LOOKUP_RESP_HDR_SIZE - 1])
                .unwrap_err(),
            NipcError::Truncated
        );

        let mut bad = buf.clone();
        put_u16(&mut bad, 0, 99);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        put_u16(&mut bad, 2, 1);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        put_u32(&mut bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadAlignment
        );

        bad = buf.clone();
        put_u32(&mut bad, CGROUPS_LOOKUP_RESP_HDR_SIZE + 4, 4096);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::OutOfBounds
        );

        bad = buf.clone();
        put_u32(
            &mut bad,
            CGROUPS_LOOKUP_RESP_HDR_SIZE + 4,
            (CGROUPS_LOOKUP_ITEM_HDR_SIZE - 1) as u32,
        );
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        put_u16(&mut bad, item_start, 99);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        let path_off = u32_at(&bad, item_start + 8) as usize;
        let path_len = u32_at(&bad, item_start + 12) as usize;
        bad[item_start + path_off + path_len] = b'x';
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::MissingNul
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        put_u32(&mut bad, item_start + 8, 4);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::OutOfBounds
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        let path_off = u32_at(&bad, item_start + 8);
        let path_len = u32_at(&bad, item_start + 12);
        put_u32(&mut bad, item_start + 16, path_off);
        put_u32(&mut bad, item_start + 20, path_len);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        let path_off = u32_at(&bad, item_start + 8) as usize;
        let path_len = u32_at(&bad, item_start + 12) as usize;
        let name_off = u32_at(&bad, item_start + 16) as usize;
        let name_len = u32_at(&bad, item_start + 20) as usize;
        let fixed_end = (path_off + path_len + 1).max(name_off + name_len + 1);
        bad[item_start + fixed_end] = 1;
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, CGROUPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        let path_off = u32_at(&bad, item_start + 8) as usize;
        let path_len = u32_at(&bad, item_start + 12) as usize;
        let name_off = u32_at(&bad, item_start + 16) as usize;
        let name_len = u32_at(&bad, item_start + 20) as usize;
        let fixed_end = (path_off + path_len + 1).max(name_off + name_len + 1);
        let table_start = align8(fixed_end);
        put_u32(&mut bad, item_start + table_start + 4, 0);
        assert_eq!(
            CgroupsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        let mut two = vec![0u8; 512];
        let mut b = CgroupsLookupBuilder::new(&mut two, 2, 1);
        b.add(CGROUP_LOOKUP_UNKNOWN_PERMANENT, 0, b"/a", b"", &[])
            .unwrap();
        b.add(CGROUP_LOOKUP_UNKNOWN_PERMANENT, 0, b"/b", b"", &[])
            .unwrap();
        let n = b.finish().unwrap();
        two.truncate(n);
        let first_off = u32_at(&two, CGROUPS_LOOKUP_RESP_HDR_SIZE);
        put_u32(
            &mut two,
            CGROUPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE,
            first_off,
        );
        assert_eq!(
            CgroupsLookupResponseView::decode(&two).unwrap_err(),
            NipcError::BadLayout
        );
    }
}
