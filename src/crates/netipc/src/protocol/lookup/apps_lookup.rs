//! APPS_LOOKUP codec.

use super::common::*;
use crate::protocol::{align8, NipcError, StrView};

pub const NIPC_UID_UNSET: u32 = u32::MAX;

pub const PID_LOOKUP_KNOWN: u16 = 0;
pub const PID_LOOKUP_UNKNOWN: u16 = 1;

pub const APPS_CGROUP_KNOWN: u16 = 0;
pub const APPS_CGROUP_UNKNOWN_RETRY_LATER: u16 = 1;
pub const APPS_CGROUP_UNKNOWN_PERMANENT: u16 = 2;
pub const APPS_CGROUP_HOST_ROOT: u16 = 3;

pub const APPS_LOOKUP_REQ_HDR_SIZE: usize = 16;
pub const APPS_LOOKUP_RESP_HDR_SIZE: usize = 16;
pub const APPS_LOOKUP_ITEM_HDR_SIZE: usize = 60;
pub const APPS_LOOKUP_KEY_SIZE: usize = 8;

#[derive(Debug)]
pub struct AppsLookupRequestView<'a> {
    pub item_count: u32,
    payload: &'a [u8],
}

#[derive(Debug)]
pub struct AppsLookupResponseView<'a> {
    pub layout_version: u16,
    pub flags: u16,
    pub item_count: u32,
    pub generation: u64,
    payload: &'a [u8],
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct AppsLookupItemView<'a> {
    pub status: u16,
    pub orchestrator: u16,
    pub cgroup_status: u16,
    pub pid: u32,
    pub ppid: u32,
    pub uid: u32,
    pub starttime: u64,
    pub comm: StrView<'a>,
    pub cgroup_path: StrView<'a>,
    pub cgroup_name: StrView<'a>,
    pub label_count: u16,
    item: &'a [u8],
    label_table_offset: usize,
}

fn validate_apps_lookup_semantics(
    status: u16,
    cgroup_status: u16,
    orchestrator: u16,
    ppid: u32,
    uid: u32,
    starttime: u64,
    comm_len: u64,
    path_len: u64,
    name_len: u64,
    label_count: u64,
) -> Result<(), NipcError> {
    validate_apps_lookup_domains(status, cgroup_status, comm_len)?;
    if status == PID_LOOKUP_UNKNOWN {
        return validate_apps_lookup_unknown(
            orchestrator,
            cgroup_status,
            ppid,
            uid,
            starttime,
            comm_len,
            path_len,
            name_len,
            label_count,
        );
    }
    validate_apps_lookup_known(
        cgroup_status,
        orchestrator,
        comm_len,
        path_len,
        name_len,
        label_count,
    )
}

fn validate_apps_lookup_domains(
    status: u16,
    cgroup_status: u16,
    comm_len: u64,
) -> Result<(), NipcError> {
    if status != PID_LOOKUP_KNOWN && status != PID_LOOKUP_UNKNOWN {
        return Err(NipcError::BadLayout);
    }
    if cgroup_status != APPS_CGROUP_KNOWN
        && cgroup_status != APPS_CGROUP_UNKNOWN_RETRY_LATER
        && cgroup_status != APPS_CGROUP_UNKNOWN_PERMANENT
        && cgroup_status != APPS_CGROUP_HOST_ROOT
    {
        return Err(NipcError::BadLayout);
    }
    if comm_len > 15 {
        return Err(NipcError::BadLayout);
    }
    Ok(())
}

fn validate_apps_lookup_unknown(
    orchestrator: u16,
    cgroup_status: u16,
    ppid: u32,
    uid: u32,
    starttime: u64,
    comm_len: u64,
    path_len: u64,
    name_len: u64,
    label_count: u64,
) -> Result<(), NipcError> {
    if orchestrator != 0
        || cgroup_status != 0
        || ppid != 0
        || uid != NIPC_UID_UNSET
        || starttime != 0
        || comm_len != 0
        || path_len != 0
        || name_len != 0
        || label_count != 0
    {
        return Err(NipcError::BadLayout);
    }
    Ok(())
}

fn validate_apps_lookup_known(
    cgroup_status: u16,
    orchestrator: u16,
    comm_len: u64,
    path_len: u64,
    name_len: u64,
    label_count: u64,
) -> Result<(), NipcError> {
    if comm_len == 0 {
        return Err(NipcError::BadLayout);
    }
    match cgroup_status {
        APPS_CGROUP_KNOWN => {
            if path_len == 0 {
                return Err(NipcError::BadLayout);
            }
        }
        APPS_CGROUP_UNKNOWN_RETRY_LATER => {
            if orchestrator != 0 || name_len != 0 || label_count != 0 {
                return Err(NipcError::BadLayout);
            }
        }
        APPS_CGROUP_UNKNOWN_PERMANENT => {
            if path_len == 0 || orchestrator != 0 || name_len != 0 || label_count != 0 {
                return Err(NipcError::BadLayout);
            }
        }
        APPS_CGROUP_HOST_ROOT => {
            if orchestrator != 0 || path_len != 0 || name_len != 0 || label_count != 0 {
                return Err(NipcError::BadLayout);
            }
        }
        _ => return Err(NipcError::BadLayout),
    }
    Ok(())
}

pub fn encode_apps_lookup_request(pids: &[u32], buf: &mut [u8]) -> Result<usize, NipcError> {
    let count = pids.len();
    let dir_size = count
        .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
        .ok_or(NipcError::Overflow)?;
    let key_size = count
        .checked_mul(APPS_LOOKUP_KEY_SIZE)
        .ok_or(NipcError::Overflow)?;
    let packed_start = APPS_LOOKUP_REQ_HDR_SIZE
        .checked_add(dir_size)
        .ok_or(NipcError::Overflow)?;
    let total = packed_start
        .checked_add(key_size)
        .ok_or(NipcError::Overflow)?;
    if total > buf.len() {
        return Err(NipcError::Overflow);
    }

    for (i, pid) in pids.iter().enumerate() {
        let dir = APPS_LOOKUP_REQ_HDR_SIZE + i * LOOKUP_DIR_ENTRY_SIZE;
        let key_offset = i
            .checked_mul(APPS_LOOKUP_KEY_SIZE)
            .ok_or(NipcError::Overflow)?;
        put_u32(buf, dir, checked_u32(key_offset)?);
        put_u32(buf, dir + 4, APPS_LOOKUP_KEY_SIZE as u32);
        let key = packed_start + i * APPS_LOOKUP_KEY_SIZE;
        put_u32(buf, key, *pid);
        put_u32(buf, key + 4, 0);
    }

    put_u16(buf, 0, 1);
    put_u16(buf, 2, 0);
    put_u32(buf, 4, checked_u32(count)?);
    put_u32(buf, 8, 0);
    put_u32(buf, 12, 0);
    Ok(total)
}

impl<'a> AppsLookupRequestView<'a> {
    pub fn decode(buf: &'a [u8]) -> Result<Self, NipcError> {
        if buf.len() < APPS_LOOKUP_REQ_HDR_SIZE {
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
        let dir_end = APPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::BadItemCount)?;
        if dir_end > buf.len() {
            return Err(NipcError::Truncated);
        }
        validate_lookup_dir(
            buf,
            APPS_LOOKUP_REQ_HDR_SIZE,
            item_count,
            buf.len() - dir_end,
            0,
            Some(APPS_LOOKUP_KEY_SIZE),
        )?;
        for i in 0..item_count as usize {
            let base = APPS_LOOKUP_REQ_HDR_SIZE + i * LOOKUP_DIR_ENTRY_SIZE;
            let off = u32_at(buf, base) as usize;
            let key = checked_subslice(buf, dir_end, off, APPS_LOOKUP_KEY_SIZE)?;
            if u32_at(key, 4) != 0 {
                return Err(NipcError::BadLayout);
            }
        }
        Ok(Self {
            item_count,
            payload: buf,
        })
    }

    pub fn item(&self, index: u32) -> Result<u32, NipcError> {
        if index >= self.item_count {
            return Err(NipcError::OutOfBounds);
        }
        let dir_size = (self.item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::BadItemCount)?;
        let packed_start = APPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::BadItemCount)?;
        let base = APPS_LOOKUP_REQ_HDR_SIZE
            .checked_add(
                (index as usize)
                    .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
                    .ok_or(NipcError::BadItemCount)?,
            )
            .ok_or(NipcError::BadItemCount)?;
        let off = u32_at(self.payload, base) as usize;
        Ok(u32_at(
            checked_subslice(self.payload, packed_start, off, APPS_LOOKUP_KEY_SIZE)?,
            0,
        ))
    }
}

impl<'a> AppsLookupResponseView<'a> {
    pub fn decode(buf: &'a [u8]) -> Result<Self, NipcError> {
        if buf.len() < APPS_LOOKUP_RESP_HDR_SIZE {
            return Err(NipcError::Truncated);
        }
        if u16_at(buf, 0) != 1 || u16_at(buf, 2) != 0 {
            return Err(NipcError::BadLayout);
        }
        let item_count = u32_at(buf, 4);
        let dir_size = (item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::BadItemCount)?;
        let dir_end = APPS_LOOKUP_RESP_HDR_SIZE
            .checked_add(dir_size)
            .ok_or(NipcError::BadItemCount)?;
        if dir_end > buf.len() {
            return Err(NipcError::Truncated);
        }
        validate_lookup_dir(
            buf,
            APPS_LOOKUP_RESP_HDR_SIZE,
            item_count,
            buf.len() - dir_end,
            APPS_LOOKUP_ITEM_HDR_SIZE,
            None,
        )?;
        for i in 0..item_count as usize {
            let base = APPS_LOOKUP_RESP_HDR_SIZE + i * LOOKUP_DIR_ENTRY_SIZE;
            let off = u32_at(buf, base) as usize;
            let len = u32_at(buf, base + 4) as usize;
            decode_apps_item(checked_subslice(buf, dir_end, off, len)?)?;
        }
        Ok(Self {
            layout_version: 1,
            flags: 0,
            item_count,
            generation: u64_at(buf, 8),
            payload: buf,
        })
    }

    pub fn item(&self, index: u32) -> Result<AppsLookupItemView<'a>, NipcError> {
        if index >= self.item_count {
            return Err(NipcError::OutOfBounds);
        }
        let packed_start = lookup_data_offset(APPS_LOOKUP_RESP_HDR_SIZE, self.item_count)?;
        let base = lookup_dir_entry_offset(APPS_LOOKUP_RESP_HDR_SIZE, index)?;
        let off = u32_at(self.payload, base) as usize;
        let len = u32_at(self.payload, base + 4) as usize;
        decode_apps_item(checked_subslice(self.payload, packed_start, off, len)?)
    }
}

fn decode_apps_item(item: &[u8]) -> Result<AppsLookupItemView<'_>, NipcError> {
    if item.len() < APPS_LOOKUP_ITEM_HDR_SIZE {
        return Err(NipcError::Truncated);
    }
    let status = u16_at(item, 2);
    let orchestrator = u16_at(item, 4);
    let cgroup_status = u16_at(item, 6);
    let pid = u32_at(item, 8);
    let ppid = u32_at(item, 12);
    let uid = u32_at(item, 16);
    let starttime = u64_at(item, 24);
    let comm_off = u32_at(item, 32) as usize;
    let comm_len = u32_at(item, 36) as usize;
    let path_off = u32_at(item, 40) as usize;
    let path_len = u32_at(item, 44) as usize;
    let name_off = u32_at(item, 48) as usize;
    let name_len = u32_at(item, 52) as usize;
    let label_count = u16_at(item, 56);

    if u16_at(item, 0) != 1 || u32_at(item, 20) != 0 || u16_at(item, 58) != 0 {
        return Err(NipcError::BadLayout);
    }
    validate_apps_lookup_semantics(
        status,
        cgroup_status,
        orchestrator,
        ppid,
        uid,
        starttime,
        comm_len as u64,
        path_len as u64,
        name_len as u64,
        label_count as u64,
    )?;

    let (comm, comm_end) = lookup_string(item, APPS_LOOKUP_ITEM_HDR_SIZE, comm_off, comm_len)?;
    let (cgroup_path, path_end) =
        lookup_string(item, APPS_LOOKUP_ITEM_HDR_SIZE, path_off, path_len)?;
    let (cgroup_name, name_end) =
        lookup_string(item, APPS_LOOKUP_ITEM_HDR_SIZE, name_off, name_len)?;
    if overlap(comm_off, comm_end, path_off, path_end)
        || overlap(comm_off, comm_end, name_off, name_end)
        || overlap(path_off, path_end, name_off, name_end)
    {
        return Err(NipcError::BadLayout);
    }
    let label_table_offset = validate_labels(
        item,
        APPS_LOOKUP_ITEM_HDR_SIZE,
        label_count,
        comm_end.max(path_end).max(name_end),
    )?;
    Ok(AppsLookupItemView {
        status,
        orchestrator,
        cgroup_status,
        pid,
        ppid,
        uid,
        starttime,
        comm,
        cgroup_path,
        cgroup_name,
        label_count,
        item,
        label_table_offset,
    })
}

impl<'a> AppsLookupItemView<'a> {
    pub fn label(&self, index: u32) -> Result<LookupLabelView<'a>, NipcError> {
        label_at(
            self.item,
            APPS_LOOKUP_ITEM_HDR_SIZE,
            self.label_count,
            self.label_table_offset,
            index,
        )
    }
}

pub struct AppsLookupBuilder<'a> {
    buf: &'a mut [u8],
    generation: u64,
    item_count: u32,
    max_items: u32,
    data_offset: usize,
    error: Option<NipcError>,
}

impl<'a> AppsLookupBuilder<'a> {
    pub fn new(buf: &'a mut [u8], max_items: u32, generation: u64) -> Self {
        let data_offset = (max_items as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .and_then(|v| APPS_LOOKUP_RESP_HDR_SIZE.checked_add(v))
            .expect("AppsLookupBuilder buffer too small");
        assert!(
            buf.len() >= data_offset,
            "AppsLookupBuilder buffer too small"
        );
        Self {
            buf,
            generation,
            item_count: 0,
            max_items,
            data_offset,
            error: None,
        }
    }

    pub fn set_generation(&mut self, generation: u64) {
        self.generation = generation;
    }

    #[allow(clippy::too_many_arguments)]
    pub fn add(
        &mut self,
        status: u16,
        cgroup_status: u16,
        orchestrator: u16,
        pid: u32,
        ppid: u32,
        uid: u32,
        starttime: u64,
        comm: &[u8],
        cgroup_path: &[u8],
        cgroup_name: &[u8],
        labels: &[(&[u8], &[u8])],
    ) -> Result<(), NipcError> {
        if self.item_count >= self.max_items {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        }
        if let Err(err) = validate_apps_lookup_semantics(
            status,
            cgroup_status,
            orchestrator,
            ppid,
            uid,
            starttime,
            comm.len() as u64,
            cgroup_path.len() as u64,
            cgroup_name.len() as u64,
            labels.len() as u64,
        ) {
            self.error = Some(err);
            return Err(err);
        }
        if source_string_invalid(comm, status == PID_LOOKUP_KNOWN)
            || source_string_invalid(cgroup_path, false)
            || source_string_invalid(cgroup_name, false)
        {
            self.error = Some(NipcError::BadLayout);
            return Err(NipcError::BadLayout);
        }
        let label_count = match checked_u16(labels.len()) {
            Ok(v) => v,
            Err(err) => {
                self.error = Some(err);
                return Err(err);
            }
        };

        let item_start = align8(self.data_offset);
        let comm_offset = APPS_LOOKUP_ITEM_HDR_SIZE;
        let Some(path_offset) = comm_offset
            .checked_add(comm.len())
            .and_then(|v| v.checked_add(1))
        else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let Some(name_offset) = path_offset
            .checked_add(cgroup_path.len())
            .and_then(|v| v.checked_add(1))
        else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let Some(fixed_end) = name_offset
            .checked_add(cgroup_name.len())
            .and_then(|v| v.checked_add(1))
        else {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        };
        let (table_start, table_bytes, mut item_size) = label_layout(fixed_end, labels)?;
        let item_end = item_start
            .checked_add(item_size)
            .ok_or(NipcError::Overflow)?;
        if item_end > self.buf.len() {
            self.error = Some(NipcError::Overflow);
            return Err(NipcError::Overflow);
        }
        if item_start > self.data_offset {
            self.buf[self.data_offset..item_start].fill(0);
        }
        let item = &mut self.buf[item_start..item_end];
        if let Err(err) = write_apps_item_header(
            item,
            status,
            orchestrator,
            cgroup_status,
            pid,
            ppid,
            uid,
            starttime,
            comm_offset,
            comm.len(),
            path_offset,
            cgroup_path.len(),
            name_offset,
            cgroup_name.len(),
            label_count,
        ) {
            self.error = Some(err);
            return Err(err);
        }
        item[comm_offset..comm_offset + comm.len()].copy_from_slice(comm);
        item[comm_offset + comm.len()] = 0;
        item[path_offset..path_offset + cgroup_path.len()].copy_from_slice(cgroup_path);
        item[path_offset + cgroup_path.len()] = 0;
        item[name_offset..name_offset + cgroup_name.len()].copy_from_slice(cgroup_name);
        item[name_offset + cgroup_name.len()] = 0;
        if !labels.is_empty() {
            item[fixed_end..table_start].fill(0);
            item_size = write_lookup_labels(item, table_start, table_bytes, labels)?;
        }
        let dir_offset = (self.item_count as usize)
            .checked_mul(LOOKUP_DIR_ENTRY_SIZE)
            .ok_or(NipcError::Overflow)?;
        let dir = APPS_LOOKUP_RESP_HDR_SIZE
            .checked_add(dir_offset)
            .ok_or(NipcError::Overflow)?;
        put_u32(self.buf, dir, checked_u32(item_start)?);
        put_u32(self.buf, dir + 4, checked_u32(item_size)?);
        self.data_offset = item_end;
        self.item_count += 1;
        Ok(())
    }

    pub fn finish(self) -> Result<usize, NipcError> {
        finish_lookup_response(
            self.buf,
            APPS_LOOKUP_RESP_HDR_SIZE,
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

#[allow(clippy::too_many_arguments)]
fn write_apps_item_header(
    item: &mut [u8],
    status: u16,
    orchestrator: u16,
    cgroup_status: u16,
    pid: u32,
    ppid: u32,
    uid: u32,
    starttime: u64,
    comm_off: usize,
    comm_len: usize,
    path_off: usize,
    path_len: usize,
    name_off: usize,
    name_len: usize,
    label_count: u16,
) -> Result<(), NipcError> {
    put_u16(item, 0, 1);
    put_u16(item, 2, status);
    put_u16(item, 4, orchestrator);
    put_u16(item, 6, cgroup_status);
    put_u32(item, 8, pid);
    put_u32(item, 12, ppid);
    put_u32(item, 16, uid);
    put_u32(item, 20, 0);
    put_u64(item, 24, starttime);
    put_u32(item, 32, checked_u32(comm_off)?);
    put_u32(item, 36, checked_u32(comm_len)?);
    put_u32(item, 40, checked_u32(path_off)?);
    put_u32(item, 44, checked_u32(path_len)?);
    put_u32(item, 48, checked_u32(name_off)?);
    put_u32(item, 52, checked_u32(name_len)?);
    put_u16(item, 56, label_count);
    put_u16(item, 58, 0);
    Ok(())
}

pub fn dispatch_apps_lookup<F>(req: &[u8], resp: &mut [u8], handler: F) -> Result<usize, NipcError>
where
    F: FnOnce(&AppsLookupRequestView, &mut AppsLookupBuilder) -> bool,
{
    let request = AppsLookupRequestView::decode(req)?;
    let min_required = lookup_data_offset(APPS_LOOKUP_RESP_HDR_SIZE, request.item_count)
        .map_err(|_| NipcError::Overflow)?;
    if resp.len() < min_required {
        return Err(NipcError::Overflow);
    }
    let mut builder = AppsLookupBuilder::new(resp, request.item_count, 0);
    if !handler(&request, &mut builder) {
        return Err(builder.error().unwrap_or(NipcError::BadLayout));
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
    use super::super::common::{put_u16, put_u32, response_item_bounds, LOOKUP_DIR_ENTRY_SIZE};
    use super::*;
    use crate::protocol::ORCHESTRATOR_DOCKER;

    #[test]
    fn apps_lookup_response_variants_roundtrip() {
        let mut buf = [0u8; 1024];
        let mut b = AppsLookupBuilder::new(&mut buf, 4, 7);
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_KNOWN,
            ORCHESTRATOR_DOCKER,
            123,
            1,
            1000,
            42,
            b"nginx",
            b"/docker/abc",
            b"container-a",
            &[(b"image".as_slice(), b"nginx:latest".as_slice())],
        )
        .unwrap();
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_UNKNOWN_RETRY_LATER,
            0,
            125,
            1,
            0,
            44,
            b"worker",
            b"",
            b"",
            &[],
        )
        .unwrap();
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_HOST_ROOT,
            0,
            124,
            1,
            0,
            43,
            b"sshd",
            b"",
            b"",
            &[],
        )
        .unwrap();
        b.add(
            PID_LOOKUP_UNKNOWN,
            APPS_CGROUP_KNOWN,
            0,
            0,
            0,
            NIPC_UID_UNSET,
            0,
            b"",
            b"",
            b"",
            &[],
        )
        .unwrap();
        let n = b.finish().unwrap();
        let view = AppsLookupResponseView::decode(&buf[..n]).unwrap();
        assert_eq!(view.item_count, 4);
        assert_eq!(view.item(0).unwrap().comm.as_bytes(), b"nginx");
        assert_eq!(
            view.item(1).unwrap().cgroup_status,
            APPS_CGROUP_UNKNOWN_RETRY_LATER
        );
        assert_eq!(view.item(1).unwrap().cgroup_path.as_bytes(), b"");
        assert_eq!(view.item(2).unwrap().cgroup_status, APPS_CGROUP_HOST_ROOT);
        assert_eq!(view.item(3).unwrap().status, PID_LOOKUP_UNKNOWN);
    }

    #[test]
    fn apps_lookup_comm_boundary() {
        let mut buf = [0u8; 256];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 0);
        assert!(b
            .add(
                PID_LOOKUP_KNOWN,
                APPS_CGROUP_HOST_ROOT,
                0,
                1,
                0,
                0,
                1,
                b"123456789012345",
                b"",
                b"",
                &[],
            )
            .is_ok());
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 0);
        assert_eq!(
            b.add(
                PID_LOOKUP_KNOWN,
                APPS_CGROUP_HOST_ROOT,
                0,
                1,
                0,
                0,
                1,
                b"1234567890123456",
                b"",
                b"",
                &[],
            )
            .unwrap_err(),
            NipcError::BadLayout
        );
    }

    fn apps_lookup_host_root_response() -> Vec<u8> {
        let mut buf = vec![0u8; 256];
        let mut b = AppsLookupBuilder::new(&mut buf, 1, 1);
        b.add(
            PID_LOOKUP_KNOWN,
            APPS_CGROUP_HOST_ROOT,
            0,
            123,
            1,
            1000,
            42,
            b"a",
            b"",
            b"",
            &[],
        )
        .unwrap();
        let n = b.finish().unwrap();
        buf.truncate(n);
        buf
    }

    #[test]
    fn apps_lookup_empty_request_response() {
        let mut buf = [0u8; 64];
        let n = encode_apps_lookup_request(&[], &mut buf).unwrap();
        let view = AppsLookupRequestView::decode(&buf[..n]).unwrap();
        assert_eq!(view.item_count, 0);

        let mut abuf = [0u8; 64];
        let b = AppsLookupBuilder::new(&mut abuf, 0, 10);
        let n = b.finish().unwrap();
        let view = AppsLookupResponseView::decode(&abuf[..n]).unwrap();
        assert_eq!(view.item_count, 0);
        assert_eq!(view.generation, 10);
    }

    #[test]
    fn apps_lookup_dispatch_rejects_short_response_buffer() {
        let mut req = [0u8; 64];
        let n = encode_apps_lookup_request(&[1234], &mut req).unwrap();
        let mut resp = vec![0u8; APPS_LOOKUP_RESP_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE - 1];
        assert_eq!(
            dispatch_apps_lookup(&req[..n], &mut resp, |_, _| {
                panic!("handler must not run with undersized response buffer")
            })
            .unwrap_err(),
            NipcError::Overflow
        );
    }

    #[test]
    fn apps_lookup_request_rejects_bad_layouts() {
        let mut buf = [0u8; 128];
        let n = encode_apps_lookup_request(&[1234], &mut buf).unwrap();
        let mut bad = buf[..n].to_vec();
        put_u32(&mut bad, 8, 1);
        assert_eq!(
            AppsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(&mut bad, APPS_LOOKUP_REQ_HDR_SIZE, 1);
        assert_eq!(
            AppsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadAlignment
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(&mut bad, APPS_LOOKUP_REQ_HDR_SIZE + 4, 7);
        assert_eq!(
            AppsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(&mut bad, APPS_LOOKUP_REQ_HDR_SIZE, 8);
        assert_eq!(
            AppsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::OutOfBounds
        );

        bad.copy_from_slice(&buf[..n]);
        put_u32(
            &mut bad,
            APPS_LOOKUP_REQ_HDR_SIZE + LOOKUP_DIR_ENTRY_SIZE + 4,
            1,
        );
        assert_eq!(
            AppsLookupRequestView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );
    }

    #[test]
    fn apps_lookup_response_rejects_bad_layouts() {
        let buf = apps_lookup_host_root_response();

        let mut bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, APPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        put_u16(&mut bad, item_start + 2, 99);
        assert_eq!(
            AppsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, APPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        put_u16(&mut bad, item_start + 6, 99);
        assert_eq!(
            AppsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );

        bad = buf.clone();
        let (item_start, _) = response_item_bounds(&bad, APPS_LOOKUP_RESP_HDR_SIZE, 1, 0);
        put_u32(&mut bad, item_start + 36, 0);
        assert_eq!(
            AppsLookupResponseView::decode(&bad).unwrap_err(),
            NipcError::BadLayout
        );
    }
}
