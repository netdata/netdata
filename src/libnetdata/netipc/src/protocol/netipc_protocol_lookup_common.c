#include "netipc_protocol_lookup_common.h"

_Static_assert(sizeof(nipc_lookup_req_header_wire_t) == NIPC_CGROUPS_LOOKUP_REQ_HDR_SIZE,
               "lookup request header must be 16 bytes");
_Static_assert(sizeof(nipc_lookup_resp_header_wire_t) == NIPC_CGROUPS_LOOKUP_RESP_HDR_SIZE,
               "lookup response header must be 16 bytes");
_Static_assert(sizeof(nipc_lookup_dir_entry_t) == NIPC_LOOKUP_DIR_ENTRY_SIZE,
               "lookup directory entry must be 8 bytes");
_Static_assert(sizeof(nipc_lookup_label_entry_t) == NIPC_LOOKUP_LABEL_ENTRY_SIZE,
               "lookup label entry must be 16 bytes");

_Static_assert(offsetof(nipc_lookup_req_header_wire_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_lookup_req_header_wire_t, flags) == 2, "");
_Static_assert(offsetof(nipc_lookup_req_header_wire_t, item_count) == 4, "");
_Static_assert(offsetof(nipc_lookup_req_header_wire_t, reserved0) == 8, "");
_Static_assert(offsetof(nipc_lookup_req_header_wire_t, reserved1) == 12, "");

_Static_assert(offsetof(nipc_lookup_resp_header_wire_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_lookup_resp_header_wire_t, flags) == 2, "");
_Static_assert(offsetof(nipc_lookup_resp_header_wire_t, item_count) == 4, "");
_Static_assert(offsetof(nipc_lookup_resp_header_wire_t, generation) == 8, "");

_Static_assert(offsetof(nipc_lookup_dir_entry_t, offset) == 0, "");
_Static_assert(offsetof(nipc_lookup_dir_entry_t, length) == 4, "");
_Static_assert(offsetof(nipc_lookup_label_entry_t, key_offset) == 0, "");
_Static_assert(offsetof(nipc_lookup_label_entry_t, key_length) == 4, "");
_Static_assert(offsetof(nipc_lookup_label_entry_t, value_offset) == 8, "");
_Static_assert(offsetof(nipc_lookup_label_entry_t, value_length) == 12, "");

bool nipc_lookup_bytes_have_nul(const void *ptr, uint32_t len) {
  return len > 0 && memchr(ptr, '\0', len) != NULL;
}

bool nipc_lookup_source_string_invalid(const char *ptr, uint32_t len,
                                       bool require_non_empty) {
  if (require_non_empty && len == 0)
    return true;
  if (len > 0 && !ptr)
    return true;
  return ptr && nipc_lookup_bytes_have_nul(ptr, len);
}

static bool
lookup_label_storage_add_u64(uint64_t *item_size,
                             const nipc_lookup_label_view_t *label) {
  if (nipc_lookup_add_u64_over_limit(*item_size, label->key.len, UINT32_MAX,
                                     item_size))
    return false;

  if (nipc_lookup_add_u64_over_limit(*item_size, 1u, UINT32_MAX, item_size))
    return false;

  if (nipc_lookup_add_u64_over_limit(*item_size, label->value.len, UINT32_MAX,
                                     item_size))
    return false;

  if (nipc_lookup_add_u64_over_limit(*item_size, 1u, UINT32_MAX, item_size))
    return false;

  return true;
}

bool nipc_lookup_ranges_overlap_u64(uint64_t a_start, uint64_t a_end,
                                    uint64_t b_start, uint64_t b_end) {
  return a_start < b_end && b_start < a_end;
}

nipc_error_t nipc_lookup_string_view(const uint8_t *item, uint32_t item_len,
                                     uint32_t hdr_size, uint32_t offset,
                                     uint32_t length, nipc_str_view_t *out,
                                     uint64_t *end_out) {
  if (offset < hdr_size)
    return NIPC_ERR_OUT_OF_BOUNDS;

  uint64_t end;
  if (nipc_lookup_add_u64_over_limit(offset, length, item_len, &end) ||
      nipc_lookup_add_u64_over_limit(end, 1, item_len, &end))
    return NIPC_ERR_OUT_OF_BOUNDS;

  if (item[offset + length] != '\0')
    return NIPC_ERR_MISSING_NUL;
  if (nipc_lookup_bytes_have_nul(item + offset, length))
    return NIPC_ERR_BAD_LAYOUT;

  if (out) {
    out->ptr = (const char *)(item + offset);
    out->len = length;
  }
  if (end_out)
    *end_out = end;
  return NIPC_OK;
}

nipc_error_t nipc_lookup_validate_ordered_dir(const uint8_t *dir,
                                              uint32_t item_count,
                                              uint32_t packed_area_len,
                                              uint32_t min_len, bool exact_len,
                                              uint32_t exact_value) {
  uint64_t prev_end = 0;

  for (uint32_t i = 0; i < item_count; i++) {
    nipc_lookup_dir_entry_t entry;
    memcpy(&entry, dir + (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE, sizeof(entry));

    if (entry.offset % NIPC_ALIGNMENT != 0)
      return NIPC_ERR_BAD_ALIGNMENT;
    if (exact_len && entry.length != exact_value)
      return NIPC_ERR_BAD_LAYOUT;
    if (!exact_len && entry.length < min_len)
      return NIPC_ERR_BAD_LAYOUT;

    uint64_t end;
    if (nipc_lookup_add_u64_over_limit(entry.offset, entry.length,
                                       packed_area_len, &end))
      return NIPC_ERR_OUT_OF_BOUNDS;
    if (i > 0 && entry.offset < prev_end)
      return NIPC_ERR_BAD_LAYOUT;
    prev_end = end;
  }

  return NIPC_OK;
}

nipc_error_t nipc_lookup_validate_labels(const uint8_t *item, uint32_t item_len,
                                         uint32_t hdr_size,
                                         uint16_t label_count,
                                         uint64_t fixed_end,
                                         uint32_t *label_table_offset_out) {
  if (label_count == 0) {
    if (fixed_end != item_len)
      return NIPC_ERR_BAD_LAYOUT;
    if (label_table_offset_out)
      *label_table_offset_out = (uint32_t)fixed_end;
    return NIPC_OK;
  }

  uint64_t table_start;
  if (nipc_lookup_align8_u64_over_limit(fixed_end, UINT32_MAX, &table_start) ||
      table_start > item_len)
    return NIPC_ERR_OUT_OF_BOUNDS;

  for (uint64_t i = fixed_end; i < table_start; i++) {
    if (item[i] != 0)
      return NIPC_ERR_BAD_LAYOUT;
  }

  uint64_t table_bytes = (uint64_t)label_count * NIPC_LOOKUP_LABEL_ENTRY_SIZE;
  uint64_t after_table;
  if (nipc_lookup_add_u64_over_limit(table_start, table_bytes, item_len,
                                     &after_table))
    return NIPC_ERR_OUT_OF_BOUNDS;

  uint64_t expected = after_table;
  for (uint32_t i = 0; i < label_count; i++) {
    nipc_lookup_label_entry_t entry;
    memcpy(&entry,
           item + table_start + (uint64_t)i * NIPC_LOOKUP_LABEL_ENTRY_SIZE,
           sizeof(entry));

    if (entry.key_length == 0)
      return NIPC_ERR_BAD_LAYOUT;
    if (entry.key_offset != expected)
      return NIPC_ERR_BAD_LAYOUT;

    uint64_t key_end;
    nipc_error_t err =
        nipc_lookup_string_view(item, item_len, hdr_size, entry.key_offset,
                                entry.key_length, NULL, &key_end);
    if (err != NIPC_OK)
      return err;
    expected = key_end;

    if (entry.value_offset != expected)
      return NIPC_ERR_BAD_LAYOUT;
    uint64_t value_end;
    err = nipc_lookup_string_view(item, item_len, hdr_size, entry.value_offset,
                                  entry.value_length, NULL, &value_end);
    if (err != NIPC_OK)
      return err;
    expected = value_end;
  }

  if (expected != item_len)
    return NIPC_ERR_BAD_LAYOUT;
  if (label_table_offset_out)
    *label_table_offset_out = (uint32_t)table_start;
  return NIPC_OK;
}

nipc_error_t nipc_lookup_label_at(const uint8_t *item, uint32_t item_len,
                                  uint32_t hdr_size, uint16_t label_count,
                                  uint32_t label_table_offset, uint32_t index,
                                  nipc_lookup_label_view_t *out) {
  if (index >= label_count)
    return NIPC_ERR_OUT_OF_BOUNDS;

  uint64_t entry_pos = (uint64_t)label_table_offset +
                       (uint64_t)index * NIPC_LOOKUP_LABEL_ENTRY_SIZE;
  if (entry_pos + NIPC_LOOKUP_LABEL_ENTRY_SIZE > item_len)
    return NIPC_ERR_OUT_OF_BOUNDS;

  nipc_lookup_label_entry_t entry;
  memcpy(&entry, item + entry_pos, sizeof(entry));

  uint64_t ignored;
  nipc_error_t err =
      nipc_lookup_string_view(item, item_len, hdr_size, entry.key_offset,
                              entry.key_length, &out->key, &ignored);
  if (err != NIPC_OK)
    return err;
  return nipc_lookup_string_view(item, item_len, hdr_size, entry.value_offset,
                                 entry.value_length, &out->value, &ignored);
}

void nipc_lookup_write_labels(uint8_t *item, size_t table_start,
                              size_t table_bytes,
                              const nipc_lookup_label_view_t *labels,
                              uint16_t label_count) {
  size_t next = table_start + table_bytes;
  for (uint32_t i = 0; i < label_count; i++) {
    nipc_lookup_label_entry_t entry = {
        .key_offset = (uint32_t)next,
        .key_length = labels[i].key.len,
        .value_offset = (uint32_t)(next + labels[i].key.len + 1u),
        .value_length = labels[i].value.len,
    };
    memcpy(item + table_start + (size_t)i * NIPC_LOOKUP_LABEL_ENTRY_SIZE,
           &entry, sizeof(entry));
    memcpy(item + entry.key_offset, labels[i].key.ptr, labels[i].key.len);
    item[entry.key_offset + labels[i].key.len] = '\0';
    if (labels[i].value.len > 0)
      memcpy(item + entry.value_offset, labels[i].value.ptr,
             labels[i].value.len);
    item[entry.value_offset + labels[i].value.len] = '\0';
    next = entry.value_offset + labels[i].value.len + 1u;
  }
}

nipc_error_t nipc_lookup_builder_validate_strings(
    const nipc_lookup_builder_string_t *strings, uint32_t string_count) {
  for (uint32_t i = 0; i < string_count; i++) {
    if (nipc_lookup_source_string_invalid(strings[i].ptr, strings[i].len,
                                          strings[i].require_non_empty))
      return NIPC_ERR_BAD_LAYOUT;
  }
  return NIPC_OK;
}

static nipc_error_t
lookup_builder_layout_strings(uint32_t fixed_header_size,
                              nipc_lookup_builder_string_t *strings,
                              uint32_t string_count, uint64_t *fixed_end_out) {
  uint64_t cursor = fixed_header_size;

  for (uint32_t i = 0; i < string_count; i++) {
    strings[i].offset = cursor;
    if (nipc_lookup_add_u64_over_limit(cursor, strings[i].len, UINT32_MAX,
                                       &cursor) ||
        nipc_lookup_add_u64_over_limit(cursor, 1u, UINT32_MAX, &cursor))
      return NIPC_ERR_OVERFLOW;
  }

  *fixed_end_out = cursor;
  return NIPC_OK;
}

static nipc_error_t lookup_builder_layout_labels(
    uint64_t fixed_end, const nipc_lookup_label_view_t *labels,
    uint16_t label_count, nipc_lookup_builder_item_layout_t *layout) {
  layout->table_start = fixed_end;
  layout->table_bytes = 0;
  layout->item_size = fixed_end;

  if (label_count == 0)
    return NIPC_OK;

  if (!labels)
    return NIPC_ERR_BAD_LAYOUT;

  if (nipc_lookup_align8_u64_over_limit(fixed_end, UINT32_MAX,
                                        &layout->table_start))
    return NIPC_ERR_OVERFLOW;

  layout->table_bytes = (uint64_t)label_count * NIPC_LOOKUP_LABEL_ENTRY_SIZE;
  if (nipc_lookup_add_u64_over_limit(layout->table_start, layout->table_bytes,
                                     UINT32_MAX, &layout->item_size))
    return NIPC_ERR_OVERFLOW;

  for (uint32_t i = 0; i < label_count; i++) {
    if (nipc_lookup_source_string_invalid(labels[i].key.ptr, labels[i].key.len,
                                          true) ||
        nipc_lookup_source_string_invalid(labels[i].value.ptr,
                                          labels[i].value.len, false))
      return NIPC_ERR_BAD_LAYOUT;

    if (!lookup_label_storage_add_u64(&layout->item_size, &labels[i]))
      return NIPC_ERR_OVERFLOW;
  }

  return NIPC_OK;
}

nipc_error_t nipc_lookup_builder_layout_item(
    size_t data_offset, size_t buf_len, uint32_t fixed_header_size,
    nipc_lookup_builder_string_t *strings, uint32_t string_count,
    const nipc_lookup_label_view_t *labels, uint16_t label_count,
    nipc_lookup_builder_item_layout_t *layout) {
  if (nipc_lookup_align8_u64_over_limit((uint64_t)data_offset, UINT32_MAX,
                                        &layout->item_start))
    return NIPC_ERR_OVERFLOW;

  nipc_error_t err = lookup_builder_layout_strings(
      fixed_header_size, strings, string_count, &layout->fixed_end);
  if (err != NIPC_OK)
    return err;

  err = lookup_builder_layout_labels(layout->fixed_end, labels, label_count,
                                     layout);
  if (err != NIPC_OK)
    return err;

  uint64_t buf_len_u64 = (uint64_t)buf_len;
  if (layout->item_size > buf_len_u64 ||
      layout->item_start > buf_len_u64 - layout->item_size)
    return NIPC_ERR_OVERFLOW;

  return NIPC_OK;
}

void nipc_lookup_builder_write_strings(
    uint8_t *item, const nipc_lookup_builder_string_t *strings,
    uint32_t string_count) {
  for (uint32_t i = 0; i < string_count; i++) {
    size_t offset = (size_t)strings[i].offset;
    if (strings[i].len > 0)
      memcpy(item + offset, strings[i].ptr, strings[i].len);
    item[offset + strings[i].len] = '\0';
  }
}

void nipc_lookup_builder_write_dir_entry(uint8_t *buf,
                                         size_t response_header_size,
                                         uint32_t item_count, size_t item_start,
                                         size_t item_size) {
  nipc_lookup_dir_entry_t dir_entry = {
      .offset = (uint32_t)item_start,
      .length = (uint32_t)item_size,
  };
  size_t dir_pos =
      response_header_size + (size_t)item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  memcpy(buf + dir_pos, &dir_entry, sizeof(dir_entry));
}

size_t nipc_lookup_finish_common(uint8_t *p, size_t buf_len,
                                 uint32_t item_count, size_t data_offset,
                                 size_t header_size, uint64_t generation) {
  nipc_lookup_resp_header_wire_t hdr = {
      .layout_version = 1,
      .flags = 0,
      .item_count = item_count,
      .generation = generation,
  };

  if (buf_len < header_size)
    return 0;

  if (item_count == 0) {
    memcpy(p, &hdr, sizeof(hdr));
    return header_size;
  }

  if (mul_would_overflow((size_t)item_count, NIPC_LOOKUP_DIR_ENTRY_SIZE))
    return 0;
  size_t dir_size = (size_t)item_count * NIPC_LOOKUP_DIR_ENTRY_SIZE;
  if (header_size > SIZE_MAX - dir_size)
    return 0;
  size_t final_packed_start = header_size + dir_size;
  nipc_lookup_dir_entry_t first_entry;
  memcpy(&first_entry, p + header_size, sizeof(first_entry));
  uint32_t first_item_abs = first_entry.offset;

  if (data_offset < first_item_abs) {
    hdr.item_count = 0;
    memcpy(p, &hdr, sizeof(hdr));
    return header_size;
  }

  size_t packed_data_len = data_offset - first_item_abs;
  if (final_packed_start < first_item_abs)
    memmove(p + final_packed_start, p + first_item_abs, packed_data_len);

  for (uint32_t i = 0; i < item_count; i++) {
    size_t entry_pos = header_size + (size_t)i * NIPC_LOOKUP_DIR_ENTRY_SIZE;
    nipc_lookup_dir_entry_t entry;
    memcpy(&entry, p + entry_pos, sizeof(entry));
    if (entry.offset < first_item_abs)
      return 0;
    entry.offset -= first_item_abs;
    memcpy(p + entry_pos, &entry, sizeof(entry));
  }

  memcpy(p, &hdr, sizeof(hdr));
  return final_packed_start + packed_data_len;
}
