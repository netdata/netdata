#ifndef NETIPC_PROTOCOL_LOOKUP_COMMON_H
#define NETIPC_PROTOCOL_LOOKUP_COMMON_H

#include "netipc_protocol_internal.h"

typedef struct {
  uint16_t layout_version;
  uint16_t flags;
  uint32_t item_count;
  uint32_t reserved0;
  uint32_t reserved1;
} nipc_lookup_req_header_wire_t;

typedef struct {
  uint16_t layout_version;
  uint16_t flags;
  uint32_t item_count;
  uint64_t generation;
} nipc_lookup_resp_header_wire_t;

typedef struct {
  const char *ptr;
  uint32_t len;
  bool require_non_empty;
  uint64_t offset;
} nipc_lookup_builder_string_t;

typedef struct {
  uint64_t item_start;
  uint64_t fixed_end;
  uint64_t table_start;
  uint64_t table_bytes;
  uint64_t item_size;
} nipc_lookup_builder_item_layout_t;

static inline bool nipc_lookup_add_u64_over_limit(uint64_t a, uint64_t b,
                                                  uint64_t limit,
                                                  uint64_t *out) {
  if (UINT64_MAX - a < b)
    return true;
  uint64_t value = a + b;
  if (value > limit)
    return true;
  if (out)
    *out = value;
  return false;
}

static inline bool nipc_lookup_align8_u64_over_limit(uint64_t value,
                                                     uint64_t limit,
                                                     uint64_t *out) {
  if (nipc_lookup_add_u64_over_limit(value, NIPC_ALIGNMENT - 1u, limit, &value))
    return true;
  value &= ~(uint64_t)(NIPC_ALIGNMENT - 1u);
  if (out)
    *out = value;
  return false;
}

bool nipc_lookup_bytes_have_nul(const void *ptr, uint32_t len);
bool nipc_lookup_source_string_invalid(const char *ptr, uint32_t len,
                                       bool require_non_empty);
bool nipc_lookup_ranges_overlap_u64(uint64_t a_start, uint64_t a_end,
                                    uint64_t b_start, uint64_t b_end);
nipc_error_t nipc_lookup_string_view(const uint8_t *item, uint32_t item_len,
                                     uint32_t hdr_size, uint32_t offset,
                                     uint32_t length, nipc_str_view_t *out,
                                     uint64_t *end_out);
nipc_error_t nipc_lookup_validate_ordered_dir(const uint8_t *dir,
                                              uint32_t item_count,
                                              uint32_t packed_area_len,
                                              uint32_t min_len, bool exact_len,
                                              uint32_t exact_value);
nipc_error_t nipc_lookup_validate_labels(const uint8_t *item, uint32_t item_len,
                                         uint32_t hdr_size,
                                         uint16_t label_count,
                                         uint64_t fixed_end,
                                         uint32_t *label_table_offset_out);
nipc_error_t nipc_lookup_label_at(const uint8_t *item, uint32_t item_len,
                                  uint32_t hdr_size, uint16_t label_count,
                                  uint32_t label_table_offset, uint32_t index,
                                  nipc_lookup_label_view_t *out);
void nipc_lookup_write_labels(uint8_t *item, size_t table_start,
                              size_t table_bytes,
                              const nipc_lookup_label_view_t *labels,
                              uint16_t label_count);
nipc_error_t nipc_lookup_builder_validate_strings(
    const nipc_lookup_builder_string_t *strings, uint32_t string_count);
nipc_error_t nipc_lookup_builder_layout_item(
    size_t data_offset, size_t buf_len, uint32_t fixed_header_size,
    nipc_lookup_builder_string_t *strings, uint32_t string_count,
    const nipc_lookup_label_view_t *labels, uint16_t label_count,
    nipc_lookup_builder_item_layout_t *layout);
void nipc_lookup_builder_write_strings(
    uint8_t *item, const nipc_lookup_builder_string_t *strings,
    uint32_t string_count);
void nipc_lookup_builder_write_dir_entry(uint8_t *buf,
                                         size_t response_header_size,
                                         uint32_t item_count, size_t item_start,
                                         size_t item_size);
size_t nipc_lookup_finish_common(uint8_t *p, size_t buf_len,
                                 uint32_t item_count, size_t data_offset,
                                 size_t header_size, uint64_t generation);

#endif /* NETIPC_PROTOCOL_LOOKUP_COMMON_H */
