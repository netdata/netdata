#include "netipc_protocol_apps_lookup_internal.h"

#include <stdlib.h>

_Static_assert(sizeof(nipc_apps_lookup_key_wire_t) == NIPC_APPS_LOOKUP_KEY_SIZE,
               "apps lookup key must be 8 bytes");
_Static_assert(offsetof(nipc_apps_lookup_key_wire_t, pid) == 0, "");
_Static_assert(offsetof(nipc_apps_lookup_key_wire_t, reserved) == 4, "");

_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, status) == 2, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, orchestrator) == 4, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, cgroup_status) == 6, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, pid) == 8, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, ppid) == 12, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, uid) == 16, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, reserved0) == 20, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, starttime) == 24, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, comm_offset) == 32, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, comm_length) == 36, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, cgroup_path_offset) == 40,
               "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, cgroup_path_length) == 44,
               "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, cgroup_name_offset) == 48,
               "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, cgroup_name_length) == 52,
               "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, label_count) == 56, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, reserved1) == 58, "");
_Static_assert(offsetof(nipc_apps_lookup_item_wire_t, reserved1) +
                       sizeof(((nipc_apps_lookup_item_wire_t *)0)->reserved1) ==
                   NIPC_APPS_LOOKUP_ITEM_HDR_SIZE,
               "apps lookup fixed wire header must end at byte 60");
_Static_assert(sizeof(nipc_apps_lookup_item_wire_t) >=
                   NIPC_APPS_LOOKUP_ITEM_HDR_SIZE,
               "apps lookup C struct must cover the fixed wire header");

static nipc_error_t apps_lookup_validate_domains(uint16_t status,
                                                 uint16_t cgroup_status,
                                                 uint64_t comm_len) {
  if (status != NIPC_PID_LOOKUP_KNOWN && status != NIPC_PID_LOOKUP_UNKNOWN &&
      status != NIPC_PID_LOOKUP_PAYLOAD_EXCEEDED &&
      status != NIPC_PID_LOOKUP_OVERSIZED_ITEM)
    return NIPC_ERR_BAD_LAYOUT;
  if (cgroup_status != NIPC_APPS_CGROUP_KNOWN &&
      cgroup_status != NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER &&
      cgroup_status != NIPC_APPS_CGROUP_UNKNOWN_PERMANENT &&
      cgroup_status != NIPC_APPS_CGROUP_HOST_ROOT)
    return NIPC_ERR_BAD_LAYOUT;
  if (comm_len > 15)
    return NIPC_ERR_BAD_LAYOUT;
  return NIPC_OK;
}

static nipc_error_t
apps_lookup_validate_unknown(uint16_t orchestrator, uint16_t cgroup_status,
                             uint32_t ppid, uint32_t uid, uint64_t starttime,
                             uint64_t comm_len, uint64_t cgroup_path_len,
                             uint64_t cgroup_name_len, uint64_t label_count) {
  if (orchestrator != 0 || cgroup_status != 0 || ppid != 0 ||
      uid != NIPC_UID_UNSET || starttime != 0 || comm_len != 0 ||
      cgroup_path_len != 0 || cgroup_name_len != 0 || label_count != 0)
    return NIPC_ERR_BAD_LAYOUT;
  return NIPC_OK;
}

static nipc_error_t
apps_lookup_validate_known(uint16_t cgroup_status, uint16_t orchestrator,
                           uint64_t comm_len, uint64_t cgroup_path_len,
                           uint64_t cgroup_name_len, uint64_t label_count) {
  if (comm_len == 0)
    return NIPC_ERR_BAD_LAYOUT;

  switch (cgroup_status) {
  case NIPC_APPS_CGROUP_KNOWN:
    if (cgroup_path_len == 0)
      return NIPC_ERR_BAD_LAYOUT;
    break;
  case NIPC_APPS_CGROUP_UNKNOWN_RETRY_LATER:
    if (orchestrator != 0 || cgroup_name_len != 0 || label_count != 0)
      return NIPC_ERR_BAD_LAYOUT;
    break;
  case NIPC_APPS_CGROUP_UNKNOWN_PERMANENT:
    if (cgroup_path_len == 0 || orchestrator != 0 || cgroup_name_len != 0 ||
        label_count != 0)
      return NIPC_ERR_BAD_LAYOUT;
    break;
  case NIPC_APPS_CGROUP_HOST_ROOT:
    if (orchestrator != 0 || cgroup_path_len != 0 || cgroup_name_len != 0 ||
        label_count != 0)
      return NIPC_ERR_BAD_LAYOUT;
    break;
  default:
    return NIPC_ERR_BAD_LAYOUT;
  }
  return NIPC_OK;
}

static nipc_error_t apps_lookup_validate_semantics(
    uint16_t status, uint16_t cgroup_status, uint16_t orchestrator,
    uint32_t ppid, uint32_t uid, uint64_t starttime, uint64_t comm_len,
    uint64_t cgroup_path_len, uint64_t cgroup_name_len, uint64_t label_count) {
  nipc_error_t err =
      apps_lookup_validate_domains(status, cgroup_status, comm_len);
  if (err != NIPC_OK)
    return err;
  if (status != NIPC_PID_LOOKUP_KNOWN)
    return apps_lookup_validate_unknown(orchestrator, cgroup_status, ppid, uid,
                                        starttime, comm_len, cgroup_path_len,
                                        cgroup_name_len, label_count);
  return apps_lookup_validate_known(cgroup_status, orchestrator, comm_len,
                                    cgroup_path_len, cgroup_name_len,
                                    label_count);
}

/* ------------------------------------------------------------------ */
/*  Apps lookup request                                                */
/* ------------------------------------------------------------------ */

size_t nipc_apps_lookup_req_encode(const uint32_t *pids, uint32_t item_count,
                                   void *buf, size_t buf_len) {
  if (mul_would_overflow((size_t)item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return 0;
  if (mul_would_overflow((size_t)item_count, NIPC_APPS_LOOKUP_KEY_SIZE))
    return 0;

  size_t dir_size = (size_t)item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t key_size = (size_t)item_count * NIPC_APPS_LOOKUP_KEY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (dir_size > SIZE_MAX - NIPC_APPS_LOOKUP_REQ_HDR_SIZE)
    return 0;
#endif
  size_t packed_start = NIPC_APPS_LOOKUP_REQ_HDR_SIZE + dir_size;
#if SIZE_MAX <= UINT32_MAX
  if (key_size > SIZE_MAX - packed_start)
    return 0;
#endif
  if (buf_len < packed_start + key_size)
    return 0;
  if (item_count > 0 && !pids)
    return 0;

  uint8_t *p = (uint8_t *)buf;
  for (uint32_t i = 0; i < item_count; i++) {
    uint64_t key_offset = (uint64_t)i * NIPC_APPS_LOOKUP_KEY_SIZE;
    if (key_offset > UINT32_MAX)
      return 0;
    nipc_lookup_dir_entry_t entry = {
        .offset = (uint32_t)key_offset,
        .length = NIPC_APPS_LOOKUP_KEY_SIZE,
    };
    nipc_apps_lookup_key_wire_t key = {
        .pid = pids[i],
        .reserved = 0,
    };
    memcpy(p + NIPC_APPS_LOOKUP_REQ_HDR_SIZE +
               (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE,
           &entry, sizeof(entry));
    memcpy(p + packed_start + (size_t)i * NIPC_APPS_LOOKUP_KEY_SIZE, &key,
           sizeof(key));
  }

  nipc_lookup_req_header_wire_t hdr = {
      .layout_version = 1,
      .flags = 0,
      .item_count = item_count,
      .reserved0 = 0,
      .reserved1 = 0,
  };
  memcpy(p, &hdr, sizeof(hdr));
  return packed_start + key_size;
}

nipc_error_t nipc_apps_lookup_req_decode(const void *buf, size_t buf_len,
                                         nipc_apps_lookup_req_view_t *out) {
  if (buf_len < NIPC_APPS_LOOKUP_REQ_HDR_SIZE)
    return NIPC_ERR_TRUNCATED;

  nipc_lookup_req_header_wire_t hdr;
  memcpy(&hdr, buf, sizeof(hdr));
  if (hdr.layout_version != 1 || hdr.flags != 0 || hdr.reserved0 != 0 ||
      hdr.reserved1 != 0)
    return NIPC_ERR_BAD_LAYOUT;

  if (mul_would_overflow((size_t)hdr.item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE) ||
      mul_would_overflow((size_t)hdr.item_count, NIPC_APPS_LOOKUP_KEY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;

  size_t dir_size = (size_t)hdr.item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_end = NIPC_APPS_LOOKUP_REQ_HDR_SIZE + dir_size;
  if (dir_end > buf_len)
    return NIPC_ERR_TRUNCATED;
  size_t packed_area_len = buf_len - dir_end;
  if (packed_area_len > UINT32_MAX)
    return NIPC_ERR_BAD_ITEM_COUNT;

  const uint8_t *p = (const uint8_t *)buf;
  const uint8_t *dir = p + NIPC_APPS_LOOKUP_REQ_HDR_SIZE;
  nipc_error_t err = nipc_lookup_validate_ordered_dir(
      dir, hdr.item_count, (uint32_t)packed_area_len, 0, true,
      NIPC_APPS_LOOKUP_KEY_SIZE);
  if (err != NIPC_OK)
    return err;

  const uint8_t *packed = p + dir_end;
  for (uint32_t i = 0; i < hdr.item_count; i++) {
    nipc_lookup_dir_entry_t entry;
    memcpy(&entry, dir + (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE, sizeof(entry));
    nipc_apps_lookup_key_wire_t key;
    memcpy(&key, packed + entry.offset, sizeof(key));
    if (key.reserved != 0)
      return NIPC_ERR_BAD_LAYOUT;
  }

  out->item_count = hdr.item_count;
  out->_payload = p;
  out->_payload_len = buf_len;
  return NIPC_OK;
}

nipc_error_t nipc_apps_lookup_req_item(const nipc_apps_lookup_req_view_t *view,
                                       uint32_t index,
                                       nipc_apps_lookup_req_item_t *out) {
  if (!view || !out || !view->_payload)
    return NIPC_ERR_BAD_LAYOUT;
  if (index >= view->item_count)
    return NIPC_ERR_OUT_OF_BOUNDS;

  /* Decode validates this, but item accessors are public and may be
   * called with manually constructed views. */
  if (mul_would_overflow((size_t)view->item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE) ||
      mul_would_overflow((size_t)view->item_count, NIPC_APPS_LOOKUP_KEY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;

  size_t dir_size = (size_t)view->item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (NIPC_APPS_LOOKUP_REQ_HDR_SIZE > SIZE_MAX - dir_size)
    return NIPC_ERR_BAD_ITEM_COUNT;
#endif
  size_t dir_end = NIPC_APPS_LOOKUP_REQ_HDR_SIZE + dir_size;
  if (dir_end > view->_payload_len)
    return NIPC_ERR_TRUNCATED;
  size_t packed_area_len = view->_payload_len - dir_end;
  const uint8_t *dir = view->_payload + NIPC_APPS_LOOKUP_REQ_HDR_SIZE;
  const uint8_t *packed = view->_payload + dir_end;

  nipc_lookup_dir_entry_t entry;
  memcpy(&entry, dir + (size_t)index * NIPC_LOOKUP_DIR_ENTRY_SIZE,
         sizeof(entry));
  if (entry.offset % NIPC_ALIGNMENT != 0)
    return NIPC_ERR_BAD_ALIGNMENT;
  if (entry.length != NIPC_APPS_LOOKUP_KEY_SIZE)
    return NIPC_ERR_BAD_LAYOUT;
  if ((uint64_t)entry.offset + entry.length > packed_area_len)
    return NIPC_ERR_OUT_OF_BOUNDS;
  nipc_apps_lookup_key_wire_t key;
  memcpy(&key, packed + entry.offset, sizeof(key));
  if (key.reserved != 0)
    return NIPC_ERR_BAD_LAYOUT;
  out->pid = key.pid;
  return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Apps lookup response                                               */
/* ------------------------------------------------------------------ */

static nipc_error_t
apps_lookup_decode_item_bytes(const uint8_t *item, uint32_t item_len,
                              nipc_apps_lookup_item_view_t *out) {
  if (item_len < NIPC_APPS_LOOKUP_ITEM_HDR_SIZE)
    return NIPC_ERR_TRUNCATED;

  nipc_apps_lookup_item_wire_t wire;
  memcpy(&wire, item, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE);

  if (wire.layout_version != 1 || wire.reserved0 != 0 || wire.reserved1 != 0)
    return NIPC_ERR_BAD_LAYOUT;
  nipc_error_t err = apps_lookup_validate_semantics(
      wire.status, wire.cgroup_status, wire.orchestrator, wire.ppid, wire.uid,
      wire.starttime, wire.comm_length, wire.cgroup_path_length,
      wire.cgroup_name_length, wire.label_count);
  if (err != NIPC_OK)
    return err;

  nipc_str_view_t comm, cgroup_path, cgroup_name;
  uint64_t comm_end, path_end, name_end;
  err = nipc_lookup_string_view(item, item_len, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE,
                                wire.comm_offset, wire.comm_length, &comm,
                                &comm_end);
  if (err != NIPC_OK)
    return err;
  err = nipc_lookup_string_view(
      item, item_len, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE, wire.cgroup_path_offset,
      wire.cgroup_path_length, &cgroup_path, &path_end);
  if (err != NIPC_OK)
    return err;
  err = nipc_lookup_string_view(
      item, item_len, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE, wire.cgroup_name_offset,
      wire.cgroup_name_length, &cgroup_name, &name_end);
  if (err != NIPC_OK)
    return err;

  if (nipc_lookup_ranges_overlap_u64(wire.comm_offset, comm_end,
                                     wire.cgroup_path_offset, path_end) ||
      nipc_lookup_ranges_overlap_u64(wire.comm_offset, comm_end,
                                     wire.cgroup_name_offset, name_end) ||
      nipc_lookup_ranges_overlap_u64(wire.cgroup_path_offset, path_end,
                                     wire.cgroup_name_offset, name_end))
    return NIPC_ERR_BAD_LAYOUT;

  uint64_t fixed_end = comm_end;
  if (path_end > fixed_end)
    fixed_end = path_end;
  if (name_end > fixed_end)
    fixed_end = name_end;
  uint32_t label_table_offset = 0;
  err = nipc_lookup_validate_labels(
      item, item_len, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE, wire.label_count,
      fixed_end, &label_table_offset);
  if (err != NIPC_OK)
    return err;

  if (out) {
    out->status = wire.status;
    out->orchestrator = wire.orchestrator;
    out->cgroup_status = wire.cgroup_status;
    out->pid = wire.pid;
    out->ppid = wire.ppid;
    out->uid = wire.uid;
    out->starttime = wire.starttime;
    out->comm = comm;
    out->cgroup_path = cgroup_path;
    out->cgroup_name = cgroup_name;
    out->label_count = wire.label_count;
    out->_item = item;
    out->_item_len = item_len;
    out->_label_table_offset = label_table_offset;
  }
  return NIPC_OK;
}

nipc_error_t nipc_apps_lookup_resp_decode(const void *buf, size_t buf_len,
                                          nipc_apps_lookup_resp_view_t *out) {
  if (buf_len < NIPC_APPS_LOOKUP_RESP_HDR_SIZE)
    return NIPC_ERR_TRUNCATED;

  nipc_lookup_resp_header_wire_t hdr;
  memcpy(&hdr, buf, sizeof(hdr));
  if (hdr.layout_version != 1 || hdr.flags != 0)
    return NIPC_ERR_BAD_LAYOUT;

  if (mul_would_overflow((size_t)hdr.item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;
  size_t dir_size = (size_t)hdr.item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_end = NIPC_APPS_LOOKUP_RESP_HDR_SIZE + dir_size;
  if (dir_end > buf_len)
    return NIPC_ERR_TRUNCATED;
  size_t packed_area_len = buf_len - dir_end;
  if (packed_area_len > UINT32_MAX)
    return NIPC_ERR_BAD_ITEM_COUNT;

  const uint8_t *p = (const uint8_t *)buf;
  const uint8_t *dir = p + NIPC_APPS_LOOKUP_RESP_HDR_SIZE;
  nipc_error_t err = nipc_lookup_validate_ordered_dir(
      dir, hdr.item_count, (uint32_t)packed_area_len,
      NIPC_APPS_LOOKUP_ITEM_HDR_SIZE, false, 0);
  if (err != NIPC_OK)
    return err;

  const uint8_t *packed = p + dir_end;
  for (uint32_t i = 0; i < hdr.item_count; i++) {
    nipc_lookup_dir_entry_t entry;
    memcpy(&entry, dir + (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE, sizeof(entry));
    err = apps_lookup_decode_item_bytes(packed + entry.offset, entry.length,
                                        NULL);
    if (err != NIPC_OK)
      return err;
  }

  out->layout_version = hdr.layout_version;
  out->flags = hdr.flags;
  out->item_count = hdr.item_count;
  out->generation = hdr.generation;
  out->_payload = p;
  out->_payload_len = buf_len;
  return NIPC_OK;
}

nipc_error_t
nipc_apps_lookup_resp_item(const nipc_apps_lookup_resp_view_t *view,
                           uint32_t index, nipc_apps_lookup_item_view_t *out) {
  if (!view || !out || !view->_payload)
    return NIPC_ERR_BAD_LAYOUT;
  if (index >= view->item_count)
    return NIPC_ERR_OUT_OF_BOUNDS;

  /* Decode validates this, but item accessors are public and may be
   * called with manually constructed views. */
  if (mul_would_overflow((size_t)view->item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;

  size_t dir_size = (size_t)view->item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (NIPC_APPS_LOOKUP_RESP_HDR_SIZE > SIZE_MAX - dir_size)
    return NIPC_ERR_BAD_ITEM_COUNT;
#endif
  size_t dir_end = NIPC_APPS_LOOKUP_RESP_HDR_SIZE + dir_size;
  if (dir_end > view->_payload_len)
    return NIPC_ERR_TRUNCATED;
  size_t packed_area_len = view->_payload_len - dir_end;
  const uint8_t *dir = view->_payload + NIPC_APPS_LOOKUP_RESP_HDR_SIZE;
  const uint8_t *packed = view->_payload + dir_end;
  nipc_lookup_dir_entry_t entry;
  memcpy(&entry, dir + (size_t)index * NIPC_LOOKUP_DIR_ENTRY_SIZE,
         sizeof(entry));
  if (entry.offset % NIPC_ALIGNMENT != 0)
    return NIPC_ERR_BAD_ALIGNMENT;
  if ((uint64_t)entry.offset + entry.length > packed_area_len)
    return NIPC_ERR_OUT_OF_BOUNDS;
  return apps_lookup_decode_item_bytes(packed + entry.offset, entry.length,
                                       out);
}

nipc_error_t
nipc_apps_lookup_resp_raw_item(const nipc_apps_lookup_resp_view_t *view,
                               uint32_t index, const uint8_t **item_out,
                               uint32_t *item_len_out) {
  if (!view || !item_out || !item_len_out || !view->_payload)
    return NIPC_ERR_BAD_LAYOUT;
  if (index >= view->item_count)
    return NIPC_ERR_OUT_OF_BOUNDS;
  if (mul_would_overflow((size_t)view->item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;

  size_t dir_size = (size_t)view->item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (NIPC_APPS_LOOKUP_RESP_HDR_SIZE > SIZE_MAX - dir_size)
    return NIPC_ERR_BAD_ITEM_COUNT;
#endif
  size_t dir_end = NIPC_APPS_LOOKUP_RESP_HDR_SIZE + dir_size;
  if (dir_end > view->_payload_len)
    return NIPC_ERR_TRUNCATED;

  const uint8_t *dir = view->_payload + NIPC_APPS_LOOKUP_RESP_HDR_SIZE;
  const uint8_t *packed = view->_payload + dir_end;
  nipc_lookup_dir_entry_t entry;
  memcpy(&entry, dir + (size_t)index * NIPC_LOOKUP_DIR_ENTRY_SIZE,
         sizeof(entry));
  if (entry.offset % NIPC_ALIGNMENT != 0)
    return NIPC_ERR_BAD_ALIGNMENT;
  uint64_t end;
  if (nipc_lookup_add_u64_over_limit(entry.offset, entry.length,
                                     view->_payload_len - dir_end, &end))
    return NIPC_ERR_OUT_OF_BOUNDS;
  *item_out = packed + entry.offset;
  *item_len_out = entry.length;
  return NIPC_OK;
}

nipc_error_t nipc_apps_lookup_raw_resp_encode(
    const uint8_t *const *items, const uint32_t *item_lens, uint32_t item_count,
    uint64_t generation, void *buf, size_t buf_len, size_t *encoded_len_out) {
  if (item_count > 0 && (!items || !item_lens))
    return NIPC_ERR_BAD_LAYOUT;
  if (mul_would_overflow((size_t)item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_OVERFLOW;

  uint8_t *out = (uint8_t *)buf;
  size_t dir_size = (size_t)item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (NIPC_APPS_LOOKUP_RESP_HDR_SIZE > SIZE_MAX - dir_size)
    return NIPC_ERR_OVERFLOW;
#endif
  size_t data_offset = NIPC_APPS_LOOKUP_RESP_HDR_SIZE + dir_size;
  if (data_offset > buf_len)
    return NIPC_ERR_OVERFLOW;

  for (uint32_t i = 0; i < item_count; i++) {
    if (!items[i] || item_lens[i] < NIPC_APPS_LOOKUP_ITEM_HDR_SIZE)
      return NIPC_ERR_BAD_LAYOUT;
    nipc_error_t err =
        apps_lookup_decode_item_bytes(items[i], item_lens[i], NULL);
    if (err != NIPC_OK)
      return err;

    size_t item_start = nipc_align8(data_offset);
    if (item_start < data_offset || item_start > buf_len ||
        item_lens[i] > buf_len - item_start)
      return NIPC_ERR_OVERFLOW;
    if (item_start > data_offset)
      memset(out + data_offset, 0, item_start - data_offset);
    memcpy(out + item_start, items[i], item_lens[i]);
    nipc_lookup_builder_write_dir_entry(out, NIPC_APPS_LOOKUP_RESP_HDR_SIZE, i,
                                        item_start, item_lens[i]);
    data_offset = item_start + item_lens[i];
  }

  size_t encoded =
      nipc_lookup_finish_common(out, buf_len, item_count, data_offset,
                                NIPC_APPS_LOOKUP_RESP_HDR_SIZE, generation);
  if (encoded == 0)
    return NIPC_ERR_OVERFLOW;
  *encoded_len_out = encoded;
  return NIPC_OK;
}

nipc_error_t
nipc_apps_lookup_item_label(const nipc_apps_lookup_item_view_t *item,
                            uint32_t index, nipc_lookup_label_view_t *out) {
  return nipc_lookup_label_at(item->_item, item->_item_len,
                              NIPC_APPS_LOOKUP_ITEM_HDR_SIZE, item->label_count,
                              item->_label_table_offset, index, out);
}

void nipc_apps_lookup_builder_init(nipc_apps_lookup_builder_t *b, void *buf,
                                   size_t buf_len, uint32_t max_items,
                                   uint64_t generation) {
  b->buf = (uint8_t *)buf;
  b->buf_len = buf_len;
  b->generation = generation;
  b->item_count = 0;
  b->max_items = max_items;
  b->error = NIPC_OK;
  b->payload_exceeded_suffix = false;
  b->payload_exceeded_item_lens = NULL;
  b->payload_exceeded_suffix_bytes = NULL;
  b->payload_exceeded_item_lens_count = 0;
  if (mul_would_overflow((size_t)max_items, NIPC_LOOKUP_DIR_ENTRY_SIZE)) {
    b->data_offset = SIZE_MAX;
  } else {
    size_t dir_size = (size_t)max_items * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
    if (dir_size > SIZE_MAX - NIPC_APPS_LOOKUP_RESP_HDR_SIZE) {
      b->data_offset = SIZE_MAX;
    } else
#endif
    {
      b->data_offset = NIPC_APPS_LOOKUP_RESP_HDR_SIZE + dir_size;
    }
  }
}

void nipc_apps_lookup_builder_set_generation(nipc_apps_lookup_builder_t *b,
                                             uint64_t generation) {
  b->generation = generation;
}

void nipc_apps_lookup_builder_set_payload_exceeded_item_lens(
    nipc_apps_lookup_builder_t *b, const uint32_t *item_lens,
    uint32_t item_count) {
  b->payload_exceeded_item_lens = item_lens;
  b->payload_exceeded_suffix_bytes = NULL;
  b->payload_exceeded_item_lens_count = item_count;
}

static bool apps_lookup_builder_suffix_fits(const nipc_apps_lookup_builder_t *b,
                                            size_t data_offset,
                                            uint32_t first_index) {
  if (b->payload_exceeded_suffix_bytes &&
      b->payload_exceeded_item_lens_count == b->max_items) {
    if (first_index > b->max_items)
      return true;
    size_t item_start = nipc_align8(data_offset);
    if (item_start < data_offset || item_start > b->buf_len)
      return false;
    return (size_t)b->payload_exceeded_suffix_bytes[first_index] <=
           b->buf_len - item_start;
  }

  if (!b->payload_exceeded_item_lens ||
      b->payload_exceeded_item_lens_count != b->max_items)
    return true;

  for (uint32_t i = first_index; i < b->max_items; i++) {
    size_t item_start = nipc_align8(data_offset);
    if (item_start < data_offset || item_start > b->buf_len)
      return false;
    uint32_t item_len = b->payload_exceeded_item_lens[i];
    if ((size_t)item_len > b->buf_len - item_start)
      return false;
    data_offset = item_start + (size_t)item_len;
  }

  return true;
}

uint32_t nipc_apps_lookup_builder_estimate_max_items(size_t buf_len) {
  if (buf_len <= NIPC_APPS_LOOKUP_RESP_HDR_SIZE)
    return 0;
  size_t min_item = nipc_align8(NIPC_APPS_LOOKUP_ITEM_HDR_SIZE + 3u);
  return (uint32_t)((buf_len - NIPC_APPS_LOOKUP_RESP_HDR_SIZE) /
                    (NIPC_LOOKUP_DIR_ENTRY_SIZE + min_item));
}

static nipc_error_t apps_lookup_builder_add_checked(
    nipc_apps_lookup_builder_t *b, uint16_t status, uint16_t cgroup_status,
    uint16_t orchestrator, uint32_t pid, uint32_t ppid, uint32_t uid,
    uint64_t starttime, const char *comm, uint32_t comm_len,
    const char *cgroup_path, uint32_t cgroup_path_len, const char *cgroup_name,
    uint32_t cgroup_name_len, const nipc_lookup_label_view_t *labels,
    uint16_t label_count, bool allow_overflow_status);

static nipc_error_t
apps_lookup_builder_add_overflow_item(nipc_apps_lookup_builder_t *b,
                                      uint16_t status, uint32_t pid) {
  return apps_lookup_builder_add_checked(b, status, 0, 0, pid, 0,
                                         NIPC_UID_UNSET, 0, "", 0, "", 0, "", 0,
                                         NULL, 0, false);
}

static nipc_error_t
apps_lookup_builder_note_item_overflow(nipc_apps_lookup_builder_t *b,
                                       uint32_t pid) {
  if (b->item_count == 0)
    return apps_lookup_builder_add_overflow_item(
        b, NIPC_PID_LOOKUP_OVERSIZED_ITEM, pid);

  b->payload_exceeded_suffix = true;
  return apps_lookup_builder_add_overflow_item(
      b, NIPC_PID_LOOKUP_PAYLOAD_EXCEEDED, pid);
}

static nipc_error_t apps_lookup_builder_add_checked(
    nipc_apps_lookup_builder_t *b, uint16_t status, uint16_t cgroup_status,
    uint16_t orchestrator, uint32_t pid, uint32_t ppid, uint32_t uid,
    uint64_t starttime, const char *comm, uint32_t comm_len,
    const char *cgroup_path, uint32_t cgroup_path_len, const char *cgroup_name,
    uint32_t cgroup_name_len, const nipc_lookup_label_view_t *labels,
    uint16_t label_count, bool allow_overflow_status) {
  if (b->payload_exceeded_suffix && allow_overflow_status)
    return apps_lookup_builder_add_overflow_item(
        b, NIPC_PID_LOOKUP_PAYLOAD_EXCEEDED, pid);

  if (b->item_count >= b->max_items) {
    b->error = NIPC_ERR_OVERFLOW;
    return b->error;
  }
  b->error = apps_lookup_validate_semantics(
      status, cgroup_status, orchestrator, ppid, uid, starttime, comm_len,
      cgroup_path_len, cgroup_name_len, label_count);
  if (b->error != NIPC_OK)
    return b->error;

  nipc_lookup_builder_string_t strings[] = {
      {
          .ptr = comm,
          .len = comm_len,
          .require_non_empty = status == NIPC_PID_LOOKUP_KNOWN,
      },
      {.ptr = cgroup_path, .len = cgroup_path_len, .require_non_empty = false},
      {.ptr = cgroup_name, .len = cgroup_name_len, .require_non_empty = false},
  };
  b->error = nipc_lookup_builder_validate_strings(strings, 3);
  if (b->error != NIPC_OK)
    return b->error;

  nipc_lookup_builder_item_layout_t layout = {0};
  b->error = nipc_lookup_builder_layout_item(
      b->data_offset, b->buf_len, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE, strings, 3,
      labels, label_count, &layout);
  if (b->error != NIPC_OK) {
    if (b->error == NIPC_ERR_OVERFLOW && allow_overflow_status)
      return apps_lookup_builder_note_item_overflow(b, pid);
    return b->error;
  }

  size_t item_start = (size_t)layout.item_start;
  size_t item_size = (size_t)layout.item_size;
  if (allow_overflow_status &&
      !apps_lookup_builder_suffix_fits(b, item_start + item_size,
                                       b->item_count + 1u))
    return apps_lookup_builder_note_item_overflow(b, pid);

  if (item_start > b->data_offset)
    memset(b->buf + b->data_offset, 0, item_start - b->data_offset);

  uint8_t *item = b->buf + item_start;
  nipc_apps_lookup_item_wire_t wire = {
      .layout_version = 1,
      .status = status,
      .orchestrator = orchestrator,
      .cgroup_status = cgroup_status,
      .pid = pid,
      .ppid = ppid,
      .uid = uid,
      .reserved0 = 0,
      .starttime = starttime,
      .comm_offset = (uint32_t)strings[0].offset,
      .comm_length = comm_len,
      .cgroup_path_offset = (uint32_t)strings[1].offset,
      .cgroup_path_length = cgroup_path_len,
      .cgroup_name_offset = (uint32_t)strings[2].offset,
      .cgroup_name_length = cgroup_name_len,
      .label_count = label_count,
      .reserved1 = 0,
  };
  memcpy(item, &wire, NIPC_APPS_LOOKUP_ITEM_HDR_SIZE);
  nipc_lookup_builder_write_strings(item, strings, 3);

  if (label_count > 0) {
    size_t fixed_end = (size_t)layout.fixed_end;
    size_t table_start = (size_t)layout.table_start;
    if (table_start > fixed_end)
      memset(item + fixed_end, 0, table_start - fixed_end);
    nipc_lookup_write_labels(item, table_start, (size_t)layout.table_bytes,
                             labels, label_count);
  }

  nipc_lookup_builder_write_dir_entry(b->buf, NIPC_APPS_LOOKUP_RESP_HDR_SIZE,
                                      b->item_count, item_start, item_size);

  b->data_offset = item_start + item_size;
  b->item_count++;
  return NIPC_OK;
}

nipc_error_t nipc_apps_lookup_builder_add(
    nipc_apps_lookup_builder_t *b, uint16_t status, uint16_t cgroup_status,
    uint16_t orchestrator, uint32_t pid, uint32_t ppid, uint32_t uid,
    uint64_t starttime, const char *comm, uint32_t comm_len,
    const char *cgroup_path, uint32_t cgroup_path_len, const char *cgroup_name,
    uint32_t cgroup_name_len, const nipc_lookup_label_view_t *labels,
    uint16_t label_count) {
  return apps_lookup_builder_add_checked(
      b, status, cgroup_status, orchestrator, pid, ppid, uid, starttime, comm,
      comm_len, cgroup_path, cgroup_path_len, cgroup_name, cgroup_name_len,
      labels, label_count, true);
}

size_t nipc_apps_lookup_builder_finish(nipc_apps_lookup_builder_t *b) {
  return nipc_lookup_finish_common(
      b->buf, b->buf_len, b->item_count, b->data_offset,
      NIPC_APPS_LOOKUP_RESP_HDR_SIZE, b->generation);
}

nipc_error_t nipc_dispatch_apps_lookup(const uint8_t *req, size_t req_len,
                                       uint8_t *resp, size_t resp_size,
                                       size_t *resp_len,
                                       nipc_apps_lookup_handler_fn handler,
                                       void *user) {
  nipc_apps_lookup_req_view_t request;
  nipc_error_t err = nipc_apps_lookup_req_decode(req, req_len, &request);
  if (err != NIPC_OK)
    return err;

  uint32_t *payload_exceeded_suffix_bytes = NULL;
  if (request.item_count > 0) {
#if SIZE_MAX <= UINT32_MAX
    if ((size_t)request.item_count >=
        ((size_t)-1) / sizeof(*payload_exceeded_suffix_bytes))
      return NIPC_ERR_OVERFLOW;
#endif
    payload_exceeded_suffix_bytes =
        (uint32_t *)malloc(((size_t)request.item_count + 1u) *
                           sizeof(*payload_exceeded_suffix_bytes));
    if (!payload_exceeded_suffix_bytes)
      return NIPC_ERR_OVERFLOW;
    payload_exceeded_suffix_bytes[request.item_count] = 0;
    for (uint32_t i = request.item_count; i > 0; i--) {
      uint32_t idx = i - 1u;
      size_t item_cost = NIPC_APPS_LOOKUP_ITEM_HDR_SIZE + 3u;
      if (payload_exceeded_suffix_bytes[idx + 1u] > 0u)
        item_cost = nipc_align8(item_cost);
      if (item_cost > UINT32_MAX ||
          item_cost > UINT32_MAX - payload_exceeded_suffix_bytes[idx + 1u]) {
        err = NIPC_ERR_OVERFLOW;
        goto cleanup;
      }
      payload_exceeded_suffix_bytes[idx] =
          (uint32_t)item_cost + payload_exceeded_suffix_bytes[idx + 1u];
    }
  }

  nipc_apps_lookup_builder_t builder;
  nipc_apps_lookup_builder_init(&builder, resp, resp_size, request.item_count,
                                0);
  builder.payload_exceeded_suffix_bytes = payload_exceeded_suffix_bytes;
  builder.payload_exceeded_item_lens_count = request.item_count;

  if (!handler(user, &request, &builder)) {
    err = builder.error != NIPC_OK ? builder.error : NIPC_ERR_HANDLER_FAILED;
    goto cleanup;
  }

  if (builder.error != NIPC_OK) {
    err = builder.error;
    goto cleanup;
  }
  if (builder.item_count != request.item_count) {
    err = NIPC_ERR_BAD_ITEM_COUNT;
    goto cleanup;
  }

  *resp_len = nipc_apps_lookup_builder_finish(&builder);
  err = (*resp_len > 0) ? NIPC_OK : NIPC_ERR_OVERFLOW;

cleanup:
  free(payload_exceeded_suffix_bytes);
  return err;
}
