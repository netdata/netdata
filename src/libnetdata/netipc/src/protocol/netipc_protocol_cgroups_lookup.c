#include "netipc_protocol_cgroups_lookup_internal.h"

_Static_assert(sizeof(nipc_cgroups_lookup_item_wire_t) == NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE,
               "cgroups lookup item header must be 28 bytes");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, status) == 2, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, orchestrator) == 4, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, reserved0) == 6, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, path_offset) == 8, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, path_length) == 12, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, name_offset) == 16, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, name_length) == 20, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, label_count) == 24, "");
_Static_assert(offsetof(nipc_cgroups_lookup_item_wire_t, reserved1) == 26, "");

static nipc_error_t cgroups_lookup_validate_semantics(uint16_t status,
                                                      uint16_t orchestrator,
                                                      uint64_t path_len,
                                                      uint64_t name_len,
                                                      uint64_t label_count) {
  if (status != NIPC_CGROUP_LOOKUP_KNOWN &&
      status != NIPC_CGROUP_LOOKUP_UNKNOWN_RETRY_LATER &&
      status != NIPC_CGROUP_LOOKUP_UNKNOWN_PERMANENT)
    return NIPC_ERR_BAD_LAYOUT;
  if (path_len == 0)
    return NIPC_ERR_BAD_LAYOUT;
  if (status != NIPC_CGROUP_LOOKUP_KNOWN &&
      (orchestrator != 0 || name_len != 0 || label_count != 0))
    return NIPC_ERR_BAD_LAYOUT;
  return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Cgroups lookup request                                             */
/* ------------------------------------------------------------------ */

size_t nipc_cgroups_lookup_req_encode(const nipc_str_view_t *paths,
                                      uint32_t item_count, void *buf,
                                      size_t buf_len) {
  if (mul_would_overflow((size_t)item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return 0;

  size_t dir_size = (size_t)item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
  if (dir_size > SIZE_MAX - NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE)
    return 0;
#endif
  size_t packed_start = NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE + dir_size;
  if (buf_len < packed_start)
    return 0;

  uint8_t *p = (uint8_t *)buf;
  size_t data = packed_start;

  for (uint32_t i = 0; i < item_count; i++) {
    if (!paths ||
        nipc_lookup_source_string_invalid(paths[i].ptr, paths[i].len, true))
      return 0;

    size_t aligned = nipc_align8(data);
    uint64_t key_len_u64;
    if (nipc_lookup_add_u64_over_limit(paths[i].len, 1u, UINT32_MAX,
                                       &key_len_u64))
      return 0;
    size_t key_len = (size_t)key_len_u64;
    if (aligned < data || key_len > SIZE_MAX - aligned ||
        aligned + key_len > buf_len)
      return 0;
    size_t key_offset = aligned - packed_start;
    if (key_offset > UINT32_MAX || key_len > UINT32_MAX)
      return 0;
    if (aligned > data)
      memset(p + data, 0, aligned - data);

    nipc_lookup_dir_entry_t entry = {
        .offset = (uint32_t)key_offset,
        .length = (uint32_t)key_len,
    };
    memcpy(p + NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE +
               (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE,
           &entry, sizeof(entry));
    memcpy(p + aligned, paths[i].ptr, paths[i].len);
    p[aligned + paths[i].len] = '\0';
    data = aligned + key_len;
  }

  nipc_lookup_req_header_wire_t hdr = {
      .layout_version = 1,
      .flags = 0,
      .item_count = item_count,
      .reserved0 = 0,
      .reserved1 = 0,
  };
  memcpy(p, &hdr, sizeof(hdr));
  return data;
}

nipc_error_t
nipc_cgroups_lookup_req_decode(const void *buf, size_t buf_len,
                               nipc_cgroups_lookup_req_view_t *out) {
  if (buf_len < NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE)
    return NIPC_ERR_TRUNCATED;

  nipc_lookup_req_header_wire_t hdr;
  memcpy(&hdr, buf, sizeof(hdr));
  if (hdr.layout_version != 1 || hdr.flags != 0 || hdr.reserved0 != 0 ||
      hdr.reserved1 != 0)
    return NIPC_ERR_BAD_LAYOUT;

  if (mul_would_overflow((size_t)hdr.item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;
  size_t dir_size = (size_t)hdr.item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_end = NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE + dir_size;
  if (dir_end > buf_len)
    return NIPC_ERR_TRUNCATED;
  size_t packed_area_len = buf_len - dir_end;
  if (packed_area_len > UINT32_MAX)
    return NIPC_ERR_BAD_ITEM_COUNT;

  const uint8_t *p = (const uint8_t *)buf;
  const uint8_t *dir = p + NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE;
  nipc_error_t err = nipc_lookup_validate_ordered_dir(
      dir, hdr.item_count, (uint32_t)packed_area_len, 2, false, 0);
  if (err != NIPC_OK)
    return err;

  const uint8_t *packed = p + dir_end;
  for (uint32_t i = 0; i < hdr.item_count; i++) {
    nipc_lookup_dir_entry_t entry;
    memcpy(&entry, dir + (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE, sizeof(entry));
    const uint8_t *key = packed + entry.offset;
    if (key[entry.length - 1] != '\0')
      return NIPC_ERR_MISSING_NUL;
    if (nipc_lookup_bytes_have_nul(key, entry.length - 1))
      return NIPC_ERR_BAD_LAYOUT;
  }

  out->item_count = hdr.item_count;
  out->_payload = p;
  out->_payload_len = buf_len;
  return NIPC_OK;
}

nipc_error_t
nipc_cgroups_lookup_req_item(const nipc_cgroups_lookup_req_view_t *view,
                             uint32_t index,
                             nipc_cgroups_lookup_req_item_t *out) {
  if (index >= view->item_count)
    return NIPC_ERR_OUT_OF_BOUNDS;

  /* Decode validates this, but item accessors are public and may be
   * called with manually constructed views. */
  if (mul_would_overflow((size_t)view->item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;

  size_t dir_size = (size_t)view->item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_end = NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE + dir_size;
  const uint8_t *dir = view->_payload + NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE;
  const uint8_t *packed = view->_payload + dir_end;

  nipc_lookup_dir_entry_t entry;
  memcpy(&entry, dir + (size_t)index * NIPC_LOOKUP_DIR_ENTRY_SIZE,
         sizeof(entry));
  out->path.ptr = (const char *)(packed + entry.offset);
  out->path.len = entry.length - 1;
  return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Cgroups lookup response                                            */
/* ------------------------------------------------------------------ */

static nipc_error_t
cgroups_lookup_decode_item_bytes(const uint8_t *item, uint32_t item_len,
                                 nipc_cgroups_lookup_item_view_t *out) {
  if (item_len < NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE)
    return NIPC_ERR_TRUNCATED;

  nipc_cgroups_lookup_item_wire_t wire;
  memcpy(&wire, item, NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE);

  if (wire.layout_version != 1 || wire.reserved0 != 0 || wire.reserved1 != 0)
    return NIPC_ERR_BAD_LAYOUT;

  nipc_error_t err = cgroups_lookup_validate_semantics(
      wire.status, wire.orchestrator, wire.path_length, wire.name_length,
      wire.label_count);
  if (err != NIPC_OK)
    return err;

  nipc_str_view_t path, name;
  uint64_t path_end, name_end;
  err = nipc_lookup_string_view(
      item, item_len, NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE, wire.path_offset,
      wire.path_length, &path, &path_end);
  if (err != NIPC_OK)
    return err;
  err = nipc_lookup_string_view(
      item, item_len, NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE, wire.name_offset,
      wire.name_length, &name, &name_end);
  if (err != NIPC_OK)
    return err;
  if (nipc_lookup_ranges_overlap_u64(wire.path_offset, path_end,
                                     wire.name_offset, name_end))
    return NIPC_ERR_BAD_LAYOUT;

  uint64_t fixed_end = path_end > name_end ? path_end : name_end;
  uint32_t label_table_offset = 0;
  err = nipc_lookup_validate_labels(
      item, item_len, NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE, wire.label_count,
      fixed_end, &label_table_offset);
  if (err != NIPC_OK)
    return err;

  if (out) {
    out->status = wire.status;
    out->orchestrator = wire.orchestrator;
    out->path = path;
    out->name = name;
    out->label_count = wire.label_count;
    out->_item = item;
    out->_item_len = item_len;
    out->_label_table_offset = label_table_offset;
  }
  return NIPC_OK;
}

nipc_error_t
nipc_cgroups_lookup_resp_decode(const void *buf, size_t buf_len,
                                nipc_cgroups_lookup_resp_view_t *out) {
  if (buf_len < NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE)
    return NIPC_ERR_TRUNCATED;

  nipc_lookup_resp_header_wire_t hdr;
  memcpy(&hdr, buf, sizeof(hdr));
  if (hdr.layout_version != 1 || hdr.flags != 0)
    return NIPC_ERR_BAD_LAYOUT;

  if (mul_would_overflow((size_t)hdr.item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;
  size_t dir_size = (size_t)hdr.item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_end = NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE + dir_size;
  if (dir_end > buf_len)
    return NIPC_ERR_TRUNCATED;
  size_t packed_area_len = buf_len - dir_end;
  if (packed_area_len > UINT32_MAX)
    return NIPC_ERR_BAD_ITEM_COUNT;

  const uint8_t *p = (const uint8_t *)buf;
  const uint8_t *dir = p + NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE;
  nipc_error_t err = nipc_lookup_validate_ordered_dir(
      dir, hdr.item_count, (uint32_t)packed_area_len,
      NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE, false, 0);
  if (err != NIPC_OK)
    return err;

  const uint8_t *packed = p + dir_end;
  for (uint32_t i = 0; i < hdr.item_count; i++) {
    nipc_lookup_dir_entry_t entry;
    memcpy(&entry, dir + (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE, sizeof(entry));
    err = cgroups_lookup_decode_item_bytes(packed + entry.offset, entry.length,
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
nipc_cgroups_lookup_resp_item(const nipc_cgroups_lookup_resp_view_t *view,
                              uint32_t index,
                              nipc_cgroups_lookup_item_view_t *out) {
  if (index >= view->item_count)
    return NIPC_ERR_OUT_OF_BOUNDS;

  /* Decode validates this, but item accessors are public and may be
   * called with manually constructed views. */
  if (mul_would_overflow((size_t)view->item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return NIPC_ERR_BAD_ITEM_COUNT;

  size_t dir_size = (size_t)view->item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  size_t dir_end = NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE + dir_size;
  const uint8_t *dir = view->_payload + NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE;
  const uint8_t *packed = view->_payload + dir_end;
  nipc_lookup_dir_entry_t entry;
  memcpy(&entry, dir + (size_t)index * NIPC_LOOKUP_DIR_ENTRY_SIZE,
         sizeof(entry));
  return cgroups_lookup_decode_item_bytes(packed + entry.offset, entry.length,
                                          out);
}

nipc_error_t
nipc_cgroups_lookup_item_label(const nipc_cgroups_lookup_item_view_t *item,
                               uint32_t index, nipc_lookup_label_view_t *out) {
  return nipc_lookup_label_at(
      item->_item, item->_item_len, NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE,
      item->label_count, item->_label_table_offset, index, out);
}

void nipc_cgroups_lookup_builder_init(nipc_cgroups_lookup_builder_t *b,
                                      void *buf, size_t buf_len,
                                      uint32_t max_items, uint64_t generation) {
  b->buf = (uint8_t *)buf;
  b->buf_len = buf_len;
  b->generation = generation;
  b->item_count = 0;
  b->max_items = max_items;
  b->error = NIPC_OK;
  if (mul_would_overflow((size_t)max_items, NIPC_LOOKUP_DIR_ENTRY_SIZE)) {
    b->data_offset = SIZE_MAX;
  } else {
    size_t dir_size = (size_t)max_items * NIPC_LOOKUP_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
    if (dir_size > SIZE_MAX - NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE) {
      b->data_offset = SIZE_MAX;
    } else
#endif
    {
      b->data_offset = NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE + dir_size;
    }
  }
}

void nipc_cgroups_lookup_builder_set_generation(
    nipc_cgroups_lookup_builder_t *b, uint64_t generation) {
  b->generation = generation;
}

uint32_t nipc_cgroups_lookup_builder_estimate_max_items(size_t buf_len) {
  if (buf_len <= NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE)
    return 0;
  size_t min_item = nipc_align8(NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE + 2u + 1u);
  return (uint32_t)((buf_len - NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE) /
                    (NIPC_LOOKUP_DIR_ENTRY_SIZE + min_item));
}

nipc_error_t nipc_cgroups_lookup_builder_add(
    nipc_cgroups_lookup_builder_t *b, uint16_t status, uint16_t orchestrator,
    const char *path, uint32_t path_len, const char *name, uint32_t name_len,
    const nipc_lookup_label_view_t *labels, uint16_t label_count) {
  if (b->item_count >= b->max_items) {
    b->error = NIPC_ERR_OVERFLOW;
    return b->error;
  }
  nipc_error_t err = cgroups_lookup_validate_semantics(
      status, orchestrator, path_len, name_len, label_count);
  if (err != NIPC_OK) {
    b->error = err;
    return b->error;
  }

  nipc_lookup_builder_string_t strings[] = {
      {.ptr = path, .len = path_len, .require_non_empty = true},
      {.ptr = name, .len = name_len, .require_non_empty = false},
  };
  err = nipc_lookup_builder_validate_strings(strings, 2);
  if (err != NIPC_OK) {
    b->error = err;
    return b->error;
  }

  nipc_lookup_builder_item_layout_t layout = {0};
  err = nipc_lookup_builder_layout_item(
      b->data_offset, b->buf_len, NIPC_CGROUPS_LOOKUP_ITEM_HDR_SIZE, strings, 2,
      labels, label_count, &layout);
  if (err != NIPC_OK) {
    b->error = err;
    return b->error;
  }

  size_t item_start = (size_t)layout.item_start;
  size_t item_size = (size_t)layout.item_size;

  if (item_start > b->data_offset)
    memset(b->buf + b->data_offset, 0, item_start - b->data_offset);

  uint8_t *item = b->buf + item_start;
  nipc_cgroups_lookup_item_wire_t wire = {
      .layout_version = 1,
      .status = status,
      .orchestrator = orchestrator,
      .reserved0 = 0,
      .path_offset = (uint32_t)strings[0].offset,
      .path_length = path_len,
      .name_offset = (uint32_t)strings[1].offset,
      .name_length = name_len,
      .label_count = label_count,
      .reserved1 = 0,
  };
  memcpy(item, &wire, sizeof(wire));
  nipc_lookup_builder_write_strings(item, strings, 2);

  if (label_count > 0) {
    size_t fixed_end = (size_t)layout.fixed_end;
    size_t table_start = (size_t)layout.table_start;
    if (table_start > fixed_end)
      memset(item + fixed_end, 0, table_start - fixed_end);
    nipc_lookup_write_labels(item, table_start, (size_t)layout.table_bytes,
                             labels, label_count);
  }

  nipc_lookup_builder_write_dir_entry(b->buf, NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE,
                                      b->item_count, item_start, item_size);

  b->data_offset = item_start + item_size;
  b->item_count++;
  return NIPC_OK;
}

size_t nipc_cgroups_lookup_builder_finish(nipc_cgroups_lookup_builder_t *b) {
  return nipc_lookup_finish_common(
      b->buf, b->buf_len, b->item_count, b->data_offset,
      NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE, b->generation);
}

nipc_error_t nipc_dispatch_cgroups_lookup(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    nipc_cgroups_lookup_handler_fn handler, void *user)
{
  nipc_cgroups_lookup_req_view_t request;
  nipc_error_t err = nipc_cgroups_lookup_req_decode(req, req_len, &request);
  if (err != NIPC_OK)
    return err;

  nipc_cgroups_lookup_builder_t builder;
  nipc_cgroups_lookup_builder_init(&builder, resp, resp_size,
                                   request.item_count, 0);

  if (!handler(user, &request, &builder)) {
    if (builder.error != NIPC_OK)
      return builder.error;
    return NIPC_ERR_HANDLER_FAILED;
  }

  if (builder.error != NIPC_OK)
    return builder.error;
  if (builder.item_count != request.item_count)
    return NIPC_ERR_BAD_ITEM_COUNT;

  *resp_len = nipc_cgroups_lookup_builder_finish(&builder);
  return (*resp_len > 0) ? NIPC_OK : NIPC_ERR_OVERFLOW;
}
