/*
 * netipc_protocol.c - Wire envelope and codec implementation.
 *
 * Localhost-only IPC — struct layouts match wire format exactly.
 * Encode/decode uses direct memcpy (single copy per struct).
 * No endianness conversion — both peers share host byte order.
 */

#include "netipc/netipc_protocol.h"
#include <stddef.h>
#include <string.h>

/*
 * Safe multiplication check: returns true if count * entry_size would
 * overflow size_t. Portable across 32-bit and 64-bit without triggering
 * -Wtype-limits.
 */
static inline bool mul_would_overflow(size_t count, size_t entry_size) {
    return entry_size != 0 && count > SIZE_MAX / entry_size;
}

/* ------------------------------------------------------------------ */
/*  Compile-time layout assertions                                    */
/*                                                                    */
/*  These guarantee struct layouts match wire format exactly, so       */
/*  encode/decode can be a single memcpy.                             */
/* ------------------------------------------------------------------ */

/* Outer message header (32 bytes) */
_Static_assert(sizeof(nipc_header_t) == 32,
               "nipc_header_t must be 32 bytes");
_Static_assert(offsetof(nipc_header_t, magic) == 0, "");
_Static_assert(offsetof(nipc_header_t, version) == 4, "");
_Static_assert(offsetof(nipc_header_t, header_len) == 6, "");
_Static_assert(offsetof(nipc_header_t, kind) == 8, "");
_Static_assert(offsetof(nipc_header_t, flags) == 10, "");
_Static_assert(offsetof(nipc_header_t, code) == 12, "");
_Static_assert(offsetof(nipc_header_t, transport_status) == 14, "");
_Static_assert(offsetof(nipc_header_t, payload_len) == 16, "");
_Static_assert(offsetof(nipc_header_t, item_count) == 20, "");
_Static_assert(offsetof(nipc_header_t, message_id) == 24, "");

/* Chunk continuation header (32 bytes) */
_Static_assert(sizeof(nipc_chunk_header_t) == 32,
               "nipc_chunk_header_t must be 32 bytes");
_Static_assert(offsetof(nipc_chunk_header_t, magic) == 0, "");
_Static_assert(offsetof(nipc_chunk_header_t, version) == 4, "");
_Static_assert(offsetof(nipc_chunk_header_t, flags) == 6, "");
_Static_assert(offsetof(nipc_chunk_header_t, message_id) == 8, "");
_Static_assert(offsetof(nipc_chunk_header_t, total_message_len) == 16, "");
_Static_assert(offsetof(nipc_chunk_header_t, chunk_index) == 20, "");
_Static_assert(offsetof(nipc_chunk_header_t, chunk_count) == 24, "");
_Static_assert(offsetof(nipc_chunk_header_t, chunk_payload_len) == 28, "");

/* Batch entry (8 bytes) */
_Static_assert(sizeof(nipc_batch_entry_t) == 8,
               "nipc_batch_entry_t must be 8 bytes");
_Static_assert(offsetof(nipc_batch_entry_t, offset) == 0, "");
_Static_assert(offsetof(nipc_batch_entry_t, length) == 4, "");

/* Hello (wire = 44, sizeof may be 48 due to trailing alignment) */
_Static_assert(offsetof(nipc_hello_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_hello_t, flags) == 2, "");
_Static_assert(offsetof(nipc_hello_t, supported_profiles) == 4, "");
_Static_assert(offsetof(nipc_hello_t, preferred_profiles) == 8, "");
_Static_assert(offsetof(nipc_hello_t, max_request_payload_bytes) == 12, "");
_Static_assert(offsetof(nipc_hello_t, max_request_batch_items) == 16, "");
_Static_assert(offsetof(nipc_hello_t, max_response_payload_bytes) == 20, "");
_Static_assert(offsetof(nipc_hello_t, max_response_batch_items) == 24, "");
_Static_assert(offsetof(nipc_hello_t, _reserved) == 28, "");
_Static_assert(offsetof(nipc_hello_t, auth_token) == 32, "");
_Static_assert(offsetof(nipc_hello_t, packet_size) == 40, "");

/* Hello-ack (48 bytes) */
_Static_assert(sizeof(nipc_hello_ack_t) == 48,
               "nipc_hello_ack_t must be 48 bytes");
_Static_assert(offsetof(nipc_hello_ack_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_hello_ack_t, flags) == 2, "");
_Static_assert(offsetof(nipc_hello_ack_t, server_supported_profiles) == 4, "");
_Static_assert(offsetof(nipc_hello_ack_t, intersection_profiles) == 8, "");
_Static_assert(offsetof(nipc_hello_ack_t, selected_profile) == 12, "");
_Static_assert(offsetof(nipc_hello_ack_t, agreed_max_request_payload_bytes) == 16, "");
_Static_assert(offsetof(nipc_hello_ack_t, agreed_max_request_batch_items) == 20, "");
_Static_assert(offsetof(nipc_hello_ack_t, agreed_max_response_payload_bytes) == 24, "");
_Static_assert(offsetof(nipc_hello_ack_t, agreed_max_response_batch_items) == 28, "");
_Static_assert(offsetof(nipc_hello_ack_t, agreed_packet_size) == 32, "");
_Static_assert(offsetof(nipc_hello_ack_t, _reserved) == 36, "");
_Static_assert(offsetof(nipc_hello_ack_t, session_id) == 40, "");

/* Cgroups snapshot response header (24 bytes) */
_Static_assert(sizeof(nipc_cgroups_resp_header_t) == 24,
               "nipc_cgroups_resp_header_t must be 24 bytes");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, flags) == 2, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, item_count) == 4, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, systemd_enabled) == 8, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, reserved) == 12, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, generation) == 16, "");

/* Cgroups item wire header (internal, 32 bytes) */
typedef struct {
    uint16_t layout_version;
    uint16_t flags;
    uint32_t hash;
    uint32_t options;
    uint32_t enabled;
    uint32_t name_offset;
    uint32_t name_length;
    uint32_t path_offset;
    uint32_t path_length;
} nipc_cgroups_item_wire_t;

_Static_assert(sizeof(nipc_cgroups_item_wire_t) == 32,
               "nipc_cgroups_item_wire_t must be 32 bytes");

/* Cgroups request (4 bytes) */
_Static_assert(sizeof(nipc_cgroups_req_t) == 4,
               "nipc_cgroups_req_t must be 4 bytes");

/* ------------------------------------------------------------------ */
/*  Outer message header (32 bytes)                                   */
/* ------------------------------------------------------------------ */

size_t nipc_header_encode(const nipc_header_t *hdr, void *buf, size_t buf_len) {
    if (buf_len < NIPC_HEADER_LEN)
        return 0;

    memcpy(buf, hdr, NIPC_HEADER_LEN);
    return NIPC_HEADER_LEN;
}

nipc_error_t nipc_header_decode(const void *buf, size_t buf_len,
                                nipc_header_t *out) {
    if (buf_len < NIPC_HEADER_LEN)
        return NIPC_ERR_TRUNCATED;

    memcpy(out, buf, NIPC_HEADER_LEN);

    if (out->magic != NIPC_MAGIC_MSG)
        return NIPC_ERR_BAD_MAGIC;
    if (out->version != NIPC_VERSION)
        return NIPC_ERR_BAD_VERSION;
    if (out->header_len != NIPC_HEADER_LEN)
        return NIPC_ERR_BAD_HEADER_LEN;
    if (out->kind < NIPC_KIND_REQUEST || out->kind > NIPC_KIND_CONTROL)
        return NIPC_ERR_BAD_KIND;

    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Chunk continuation header (32 bytes)                              */
/* ------------------------------------------------------------------ */

size_t nipc_chunk_header_encode(const nipc_chunk_header_t *chk,
                                void *buf, size_t buf_len) {
    if (buf_len < NIPC_HEADER_LEN)
        return 0;

    memcpy(buf, chk, NIPC_HEADER_LEN);
    return NIPC_HEADER_LEN;
}

nipc_error_t nipc_chunk_header_decode(const void *buf, size_t buf_len,
                                      nipc_chunk_header_t *out) {
    if (buf_len < NIPC_HEADER_LEN)
        return NIPC_ERR_TRUNCATED;

    memcpy(out, buf, NIPC_HEADER_LEN);

    if (out->magic != NIPC_MAGIC_CHUNK)
        return NIPC_ERR_BAD_MAGIC;
    if (out->version != NIPC_VERSION)
        return NIPC_ERR_BAD_VERSION;
    if (out->flags != 0)
        return NIPC_ERR_BAD_LAYOUT;
    if (out->chunk_payload_len == 0)
        return NIPC_ERR_BAD_LAYOUT;

    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Batch item directory                                              */
/* ------------------------------------------------------------------ */

size_t nipc_batch_dir_encode(const nipc_batch_entry_t *entries,
                             uint32_t item_count,
                             void *buf, size_t buf_len) {
    size_t need = (size_t)item_count * sizeof(nipc_batch_entry_t);
    if (buf_len < need)
        return 0;

    memcpy(buf, entries, need);
    return need;
}

nipc_error_t nipc_batch_dir_decode(const void *buf, size_t buf_len,
                                   uint32_t item_count,
                                   uint32_t packed_area_len,
                                   nipc_batch_entry_t *out) {
    if (mul_would_overflow((size_t)item_count, sizeof(nipc_batch_entry_t)))
        return NIPC_ERR_BAD_ITEM_COUNT;
    size_t dir_size = (size_t)item_count * sizeof(nipc_batch_entry_t);
    if (buf_len < dir_size)
        return NIPC_ERR_TRUNCATED;

    memcpy(out, buf, dir_size);

    for (uint32_t i = 0; i < item_count; i++) {
        if (out[i].offset % NIPC_ALIGNMENT != 0)
            return NIPC_ERR_BAD_ALIGNMENT;
        if ((uint64_t)out[i].offset + out[i].length > packed_area_len)
            return NIPC_ERR_OUT_OF_BOUNDS;
    }
    return NIPC_OK;
}

nipc_error_t nipc_batch_dir_validate(const void *buf, size_t buf_len,
                                      uint32_t item_count,
                                      uint32_t packed_area_len) {
    if (mul_would_overflow((size_t)item_count, sizeof(nipc_batch_entry_t)))
        return NIPC_ERR_BAD_ITEM_COUNT;
    size_t dir_size = (size_t)item_count * sizeof(nipc_batch_entry_t);
    if (buf_len < dir_size)
        return NIPC_ERR_TRUNCATED;

    const uint8_t *p = (const uint8_t *)buf;
    for (uint32_t i = 0; i < item_count; i++) {
        uint32_t off, len;
        memcpy(&off, p + i * 8, 4);
        memcpy(&len, p + i * 8 + 4, 4);
        if (off % NIPC_ALIGNMENT != 0)
            return NIPC_ERR_BAD_ALIGNMENT;
        if ((uint64_t)off + len > packed_area_len)
            return NIPC_ERR_OUT_OF_BOUNDS;
    }
    return NIPC_OK;
}

nipc_error_t nipc_batch_item_get(const void *payload, size_t payload_len,
                                 uint32_t item_count, uint32_t index,
                                 const void **item_ptr, uint32_t *item_len) {
    if (index >= item_count)
        return NIPC_ERR_OUT_OF_BOUNDS;

    if (mul_would_overflow((size_t)item_count, sizeof(nipc_batch_entry_t)))
        return NIPC_ERR_BAD_ITEM_COUNT;
    size_t dir_size = (size_t)item_count * sizeof(nipc_batch_entry_t);
    size_t dir_aligned = nipc_align8(dir_size);

    if (payload_len < dir_aligned)
        return NIPC_ERR_TRUNCATED;

    nipc_batch_entry_t entry;
    memcpy(&entry, (const uint8_t *)payload + index * sizeof(entry), sizeof(entry));

    size_t packed_area_start = dir_aligned;
    size_t packed_area_len   = payload_len - packed_area_start;

    if (entry.offset % NIPC_ALIGNMENT != 0)
        return NIPC_ERR_BAD_ALIGNMENT;
    if ((uint64_t)entry.offset + entry.length > packed_area_len)
        return NIPC_ERR_OUT_OF_BOUNDS;

    *item_ptr = (const uint8_t *)payload + packed_area_start + entry.offset;
    *item_len = entry.length;
    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Batch builder                                                     */
/* ------------------------------------------------------------------ */

void nipc_batch_builder_init(nipc_batch_builder_t *b,
                             void *buf, size_t buf_len,
                             uint32_t max_items) {
    b->buf        = (uint8_t *)buf;
    b->buf_len    = buf_len;
    b->item_count = 0;
    b->max_items  = max_items;
    b->dir_end    = nipc_align8((size_t)max_items * sizeof(nipc_batch_entry_t));
    b->data_offset = 0;
}

nipc_error_t nipc_batch_builder_add(nipc_batch_builder_t *b,
                                    const void *item, size_t item_len) {
    if (b->item_count >= b->max_items)
        return NIPC_ERR_OVERFLOW;

    size_t aligned_off = nipc_align8(b->data_offset);
    size_t abs_pos = b->dir_end + aligned_off;

    if (abs_pos + item_len > b->buf_len)
        return NIPC_ERR_OVERFLOW;

    /* Zero alignment padding */
    if (aligned_off > b->data_offset)
        memset(b->buf + b->dir_end + b->data_offset, 0,
               aligned_off - b->data_offset);

    memcpy(b->buf + abs_pos, item, item_len);

    /* Write directory entry */
    nipc_batch_entry_t entry = {
        .offset = (uint32_t)aligned_off,
        .length = (uint32_t)item_len,
    };
    memcpy(b->buf + b->item_count * sizeof(entry), &entry, sizeof(entry));

    b->data_offset = aligned_off + item_len;
    b->item_count++;
    return NIPC_OK;
}

size_t nipc_batch_builder_finish(nipc_batch_builder_t *b,
                                 uint32_t *item_count_out) {
    if (item_count_out)
        *item_count_out = b->item_count;

    /* The decoder expects: [dir: item_count*8] [align pad] [packed items].
     * During building we placed packed data after dir_end = align8(max_items*8).
     * If item_count < max_items, compact by shifting packed data left. */
    size_t final_dir_aligned = nipc_align8(
        (size_t)b->item_count * sizeof(nipc_batch_entry_t));

    if (final_dir_aligned < b->dir_end && b->data_offset > 0) {
        memmove(b->buf + final_dir_aligned,
                b->buf + b->dir_end,
                b->data_offset);
    }

    /* Zero trailing alignment padding for deterministic output,
     * but only within the caller's buffer. */
    size_t data_aligned = nipc_align8(b->data_offset);
    if (data_aligned > b->data_offset) {
        size_t total_with_pad = final_dir_aligned + data_aligned;
        if (total_with_pad <= b->buf_len) {
            memset(b->buf + final_dir_aligned + b->data_offset,
                   0, data_aligned - b->data_offset);
        } else if (final_dir_aligned + b->data_offset < b->buf_len) {
            /* Partial padding: zero only within bounds */
            memset(b->buf + final_dir_aligned + b->data_offset,
                   0, b->buf_len - (final_dir_aligned + b->data_offset));
        }
    }

    size_t total = final_dir_aligned + data_aligned;
    return total <= b->buf_len ? total : final_dir_aligned + b->data_offset;
}

/* ------------------------------------------------------------------ */
/*  Hello payload (44 bytes on wire)                                  */
/* ------------------------------------------------------------------ */

size_t nipc_hello_encode(const nipc_hello_t *h, void *buf, size_t buf_len) {
    if (buf_len < NIPC_HELLO_WIRE_SIZE)
        return 0;

    memcpy(buf, h, NIPC_HELLO_WIRE_SIZE);
    return NIPC_HELLO_WIRE_SIZE;
}

nipc_error_t nipc_hello_decode(const void *buf, size_t buf_len,
                               nipc_hello_t *out) {
    if (buf_len < NIPC_HELLO_WIRE_SIZE)
        return NIPC_ERR_TRUNCATED;

    memcpy(out, buf, NIPC_HELLO_WIRE_SIZE);

    if (out->layout_version != 1)
        return NIPC_ERR_BAD_LAYOUT;
    if (out->_reserved != 0)
        return NIPC_ERR_BAD_LAYOUT;

    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Hello-ack payload (48 bytes)                                      */
/* ------------------------------------------------------------------ */

size_t nipc_hello_ack_encode(const nipc_hello_ack_t *h,
                             void *buf, size_t buf_len) {
    if (buf_len < sizeof(nipc_hello_ack_t))
        return 0;

    memcpy(buf, h, sizeof(nipc_hello_ack_t));
    return sizeof(nipc_hello_ack_t);
}

nipc_error_t nipc_hello_ack_decode(const void *buf, size_t buf_len,
                                   nipc_hello_ack_t *out) {
    if (buf_len < sizeof(nipc_hello_ack_t))
        return NIPC_ERR_TRUNCATED;

    memcpy(out, buf, sizeof(nipc_hello_ack_t));

    if (out->layout_version != 1)
        return NIPC_ERR_BAD_LAYOUT;
    if (out->flags != 0)
        return NIPC_ERR_BAD_LAYOUT;

    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Cgroups snapshot request (4 bytes)                                */
/* ------------------------------------------------------------------ */

size_t nipc_cgroups_req_encode(const nipc_cgroups_req_t *r,
                               void *buf, size_t buf_len) {
    if (buf_len < sizeof(nipc_cgroups_req_t))
        return 0;

    memcpy(buf, r, sizeof(nipc_cgroups_req_t));
    return sizeof(nipc_cgroups_req_t);
}

nipc_error_t nipc_cgroups_req_decode(const void *buf, size_t buf_len,
                                     nipc_cgroups_req_t *out) {
    if (buf_len < sizeof(nipc_cgroups_req_t))
        return NIPC_ERR_TRUNCATED;

    memcpy(out, buf, sizeof(nipc_cgroups_req_t));

    if (out->layout_version != 1)
        return NIPC_ERR_BAD_LAYOUT;
    if (out->flags != 0)
        return NIPC_ERR_BAD_LAYOUT;

    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Cgroups snapshot response decode                                  */
/* ------------------------------------------------------------------ */

nipc_error_t nipc_cgroups_resp_decode(const void *buf, size_t buf_len,
                                      nipc_cgroups_resp_view_t *out) {
    if (buf_len < NIPC_CGROUPS_RESP_HDR_SIZE)
        return NIPC_ERR_TRUNCATED;

    nipc_cgroups_resp_header_t hdr;
    memcpy(&hdr, buf, sizeof(hdr));

    if (hdr.layout_version != 1)
        return NIPC_ERR_BAD_LAYOUT;
    if (hdr.flags != 0)
        return NIPC_ERR_BAD_LAYOUT;
    if (hdr.reserved != 0)
        return NIPC_ERR_BAD_LAYOUT;

    out->layout_version  = hdr.layout_version;
    out->flags           = hdr.flags;
    out->item_count      = hdr.item_count;
    out->systemd_enabled = hdr.systemd_enabled;
    out->generation      = hdr.generation;

    /* Validate directory fits (with overflow check) */
    if (mul_would_overflow((size_t)out->item_count, NIPC_CGROUPS_DIR_ENTRY_SIZE))
        return NIPC_ERR_BAD_ITEM_COUNT;
    size_t dir_size = (size_t)out->item_count * NIPC_CGROUPS_DIR_ENTRY_SIZE;
    size_t dir_end  = NIPC_CGROUPS_RESP_HDR_SIZE + dir_size;
    if (dir_end > buf_len)
        return NIPC_ERR_TRUNCATED;

    size_t packed_area_len = buf_len - dir_end;

    /* Validate each directory entry */
    const uint8_t *dir = (const uint8_t *)buf + NIPC_CGROUPS_RESP_HDR_SIZE;
    for (uint32_t i = 0; i < out->item_count; i++) {
        nipc_batch_entry_t entry;
        memcpy(&entry, dir + i * sizeof(entry), sizeof(entry));

        if (entry.offset % NIPC_ALIGNMENT != 0)
            return NIPC_ERR_BAD_ALIGNMENT;
        if ((uint64_t)entry.offset + entry.length > packed_area_len)
            return NIPC_ERR_OUT_OF_BOUNDS;
        if (entry.length < NIPC_CGROUPS_ITEM_HDR_SIZE)
            return NIPC_ERR_TRUNCATED;
    }

    out->_payload     = (const uint8_t *)buf;
    out->_payload_len = buf_len;
    return NIPC_OK;
}

nipc_error_t nipc_cgroups_resp_item(const nipc_cgroups_resp_view_t *view,
                                    uint32_t index,
                                    nipc_cgroups_item_view_t *out) {
    if (index >= view->item_count)
        return NIPC_ERR_OUT_OF_BOUNDS;

    /* Overflow already checked in nipc_cgroups_resp_decode, but
     * guard defensively since this is a public API. */
    if (mul_would_overflow((size_t)view->item_count, NIPC_CGROUPS_DIR_ENTRY_SIZE))
        return NIPC_ERR_BAD_ITEM_COUNT;

    size_t dir_start = NIPC_CGROUPS_RESP_HDR_SIZE;
    size_t dir_size  = (size_t)view->item_count * NIPC_CGROUPS_DIR_ENTRY_SIZE;
    size_t packed_area_start = dir_start + dir_size;

    /* Read directory entry */
    nipc_batch_entry_t dir_entry;
    memcpy(&dir_entry,
           view->_payload + dir_start + index * sizeof(dir_entry),
           sizeof(dir_entry));

    const uint8_t *item = view->_payload + packed_area_start + dir_entry.offset;
    uint32_t item_len = dir_entry.length;

    /* Read the 32-byte item wire header in one copy */
    nipc_cgroups_item_wire_t wire;
    memcpy(&wire, item, sizeof(wire));

    if (wire.layout_version != 1)
        return NIPC_ERR_BAD_LAYOUT;
    if (wire.flags != 0)
        return NIPC_ERR_BAD_LAYOUT;

    /* Validate name string */
    if (wire.name_offset < NIPC_CGROUPS_ITEM_HDR_SIZE)
        return NIPC_ERR_OUT_OF_BOUNDS;
    if ((uint64_t)wire.name_offset + wire.name_length + 1 > item_len)
        return NIPC_ERR_OUT_OF_BOUNDS;
    if (item[wire.name_offset + wire.name_length] != '\0')
        return NIPC_ERR_MISSING_NUL;

    /* Validate path string */
    if (wire.path_offset < NIPC_CGROUPS_ITEM_HDR_SIZE)
        return NIPC_ERR_OUT_OF_BOUNDS;
    if ((uint64_t)wire.path_offset + wire.path_length + 1 > item_len)
        return NIPC_ERR_OUT_OF_BOUNDS;
    if (item[wire.path_offset + wire.path_length] != '\0')
        return NIPC_ERR_MISSING_NUL;

    /* Reject overlapping name and path regions (including NUL) */
    {
        uint64_t name_start = wire.name_offset;
        uint64_t name_end   = name_start + wire.name_length + 1;
        uint64_t path_start = wire.path_offset;
        uint64_t path_end   = path_start + wire.path_length + 1;
        if (name_start < path_end && path_start < name_end)
            return NIPC_ERR_BAD_LAYOUT;
    }

    out->layout_version = wire.layout_version;
    out->flags          = wire.flags;
    out->hash           = wire.hash;
    out->options        = wire.options;
    out->enabled        = wire.enabled;
    out->name.ptr       = (const char *)(item + wire.name_offset);
    out->name.len       = wire.name_length;
    out->path.ptr       = (const char *)(item + wire.path_offset);
    out->path.len       = wire.path_length;

    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Cgroups snapshot response builder                                 */
/*                                                                    */
/*  Layout during building (max_items directory slots reserved):      */
/*    [24-byte header space] [max_items*8 directory] [packed items]   */
/*                                                                    */
/*  Layout after finish (compacted to actual item_count):             */
/*    [24-byte header] [item_count*8 directory] [packed items]        */
/*                                                                    */
/*  If item_count < max_items, finish() shifts packed data left and   */
/*  adjusts directory offsets accordingly.                             */
/* ------------------------------------------------------------------ */

void nipc_cgroups_builder_init(nipc_cgroups_builder_t *b,
                               void *buf, size_t buf_len,
                               uint32_t max_items,
                               uint32_t systemd_enabled,
                               uint64_t generation) {
    b->buf             = (uint8_t *)buf;
    b->buf_len         = buf_len;
    b->systemd_enabled = systemd_enabled;
    b->generation      = generation;
    b->item_count      = 0;
    b->max_items       = max_items;
    b->error           = NIPC_OK;

    /* Packed item data starts after reserved directory */
    b->data_offset = NIPC_CGROUPS_RESP_HDR_SIZE +
                     (size_t)max_items * NIPC_CGROUPS_DIR_ENTRY_SIZE;
}

void nipc_cgroups_builder_set_header(nipc_cgroups_builder_t *b,
                                     uint32_t systemd_enabled,
                                     uint64_t generation) {
    b->systemd_enabled = systemd_enabled;
    b->generation = generation;
}

uint32_t nipc_cgroups_builder_estimate_max_items(size_t buf_len) {
    if (buf_len <= NIPC_CGROUPS_RESP_HDR_SIZE)
        return 0;

    size_t min_aligned_item = nipc_align8(NIPC_CGROUPS_ITEM_HDR_SIZE + 2u);
    return (uint32_t)((buf_len - NIPC_CGROUPS_RESP_HDR_SIZE) /
                      (NIPC_CGROUPS_DIR_ENTRY_SIZE + min_aligned_item));
}

nipc_error_t nipc_cgroups_builder_add(nipc_cgroups_builder_t *b,
                                      uint32_t hash,
                                      uint32_t options,
                                      uint32_t enabled,
                                      const char *name, uint32_t name_len,
                                      const char *path, uint32_t path_len) {
    if (b->item_count >= b->max_items) {
        b->error = NIPC_ERR_OVERFLOW;
        return NIPC_ERR_OVERFLOW;
    }

    /* Align item start to 8 bytes */
    size_t item_start = nipc_align8(b->data_offset);

    /* Item payload: 32-byte header + name + NUL + path + NUL */
    size_t item_size = NIPC_CGROUPS_ITEM_HDR_SIZE +
                       (size_t)name_len + 1 +
                       (size_t)path_len + 1;

    if (item_start + item_size > b->buf_len) {
        b->error = NIPC_ERR_OVERFLOW;
        return NIPC_ERR_OVERFLOW;
    }

    /* Zero alignment padding */
    if (item_start > b->data_offset)
        memset(b->buf + b->data_offset, 0, item_start - b->data_offset);

    uint8_t *item = b->buf + item_start;

    /* Write item header as a single struct copy */
    nipc_cgroups_item_wire_t wire = {
        .layout_version = 1,
        .flags          = 0,
        .hash           = hash,
        .options        = options,
        .enabled        = enabled,
        .name_offset    = NIPC_CGROUPS_ITEM_HDR_SIZE,
        .name_length    = name_len,
        .path_offset    = NIPC_CGROUPS_ITEM_HDR_SIZE + name_len + 1,
        .path_length    = path_len,
    };
    memcpy(item, &wire, sizeof(wire));

    /* Write strings with NUL terminators */
    memcpy(item + wire.name_offset, name, name_len);
    item[wire.name_offset + name_len] = '\0';
    memcpy(item + wire.path_offset, path, path_len);
    item[wire.path_offset + path_len] = '\0';

    /* Write directory entry (absolute offset stored temporarily) */
    nipc_batch_entry_t dir_entry = {
        .offset = (uint32_t)item_start,
        .length = (uint32_t)item_size,
    };
    size_t dir_pos = NIPC_CGROUPS_RESP_HDR_SIZE +
                     (size_t)b->item_count * NIPC_CGROUPS_DIR_ENTRY_SIZE;
    memcpy(b->buf + dir_pos, &dir_entry, sizeof(dir_entry));

    b->data_offset = item_start + item_size;
    b->item_count++;
    return NIPC_OK;
}

size_t nipc_cgroups_builder_finish(nipc_cgroups_builder_t *b) {
    uint8_t *p = b->buf;

    nipc_cgroups_resp_header_t hdr = {
        .layout_version  = 1,
        .flags           = 0,
        .item_count      = b->item_count,
        .systemd_enabled = b->systemd_enabled,
        .reserved        = 0,
        .generation      = b->generation,
    };

    if (b->item_count == 0) {
        memcpy(p, &hdr, sizeof(hdr));
        return NIPC_CGROUPS_RESP_HDR_SIZE;
    }

    /* Where the decoder expects packed data to start */
    size_t final_packed_start = NIPC_CGROUPS_RESP_HDR_SIZE +
                                (size_t)b->item_count * NIPC_CGROUPS_DIR_ENTRY_SIZE;

    /* Read the first directory entry to find where packed data actually begins */
    nipc_batch_entry_t first_entry;
    memcpy(&first_entry, p + NIPC_CGROUPS_RESP_HDR_SIZE, sizeof(first_entry));
    uint32_t first_item_abs = first_entry.offset;

    /* Guard against underflow if builder state is inconsistent */
    if (b->data_offset < first_item_abs) {
        hdr.item_count = 0;
        memcpy(p, &hdr, sizeof(hdr));
        return NIPC_CGROUPS_RESP_HDR_SIZE;
    }

    size_t packed_data_len = b->data_offset - first_item_abs;

    if (final_packed_start < first_item_abs) {
        memmove(p + final_packed_start, p + first_item_abs, packed_data_len);
    }

    /* Convert directory entries from absolute offsets to relative offsets */
    size_t dir_base = NIPC_CGROUPS_RESP_HDR_SIZE;
    for (uint32_t i = 0; i < b->item_count; i++) {
        size_t entry_pos = dir_base + (size_t)i * NIPC_CGROUPS_DIR_ENTRY_SIZE;
        nipc_batch_entry_t entry;
        memcpy(&entry, p + entry_pos, sizeof(entry));
        if (entry.offset < first_item_abs)
            continue; /* skip corrupted entry */
        entry.offset -= first_item_abs;
        memcpy(p + entry_pos, &entry, sizeof(entry));
    }

    /* Write snapshot header */
    memcpy(p, &hdr, sizeof(hdr));

    return final_packed_start + packed_data_len;
}

/* ------------------------------------------------------------------ */
/*  INCREMENT codec                                                    */
/* ------------------------------------------------------------------ */

size_t nipc_increment_encode(uint64_t value, void *buf, size_t buf_len) {
    if (buf_len < NIPC_INCREMENT_PAYLOAD_SIZE)
        return 0;
    memcpy(buf, &value, 8);
    return NIPC_INCREMENT_PAYLOAD_SIZE;
}

nipc_error_t nipc_increment_decode(const void *buf, size_t buf_len,
                                    uint64_t *value_out) {
    if (buf_len < NIPC_INCREMENT_PAYLOAD_SIZE)
        return NIPC_ERR_TRUNCATED;
    memcpy(value_out, buf, 8);
    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  STRING_REVERSE codec                                               */
/* ------------------------------------------------------------------ */

size_t nipc_string_reverse_encode(const char *str, uint32_t str_len,
                                   void *buf, size_t buf_len) {
    /* Guard against size_t overflow only where uint32_t can exceed size_t. */
#if SIZE_MAX <= UINT32_MAX
    if ((size_t)str_len > SIZE_MAX - (size_t)NIPC_STRING_REVERSE_HDR_SIZE - 1u)
        return 0;
#endif

    size_t total = NIPC_STRING_REVERSE_HDR_SIZE + str_len + 1;
    if (buf_len < total)
        return 0;

    uint8_t *p = (uint8_t *)buf;
    uint32_t offset = NIPC_STRING_REVERSE_HDR_SIZE;
    memcpy(p + 0, &offset, 4);
    memcpy(p + 4, &str_len, 4);
    if (str_len > 0)
        memcpy(p + offset, str, str_len);
    p[offset + str_len] = '\0';
    return total;
}

nipc_error_t nipc_string_reverse_decode(const void *buf, size_t buf_len,
                                         nipc_string_reverse_view_t *view_out) {
    if (buf_len < NIPC_STRING_REVERSE_HDR_SIZE)
        return NIPC_ERR_TRUNCATED;

    const uint8_t *p = (const uint8_t *)buf;
    uint32_t str_offset, str_length;
    memcpy(&str_offset, p + 0, 4);
    memcpy(&str_length, p + 4, 4);

    if ((uint64_t)str_offset + str_length + 1 > buf_len)
        return NIPC_ERR_OUT_OF_BOUNDS;

    if (p[str_offset + str_length] != '\0')
        return NIPC_ERR_MISSING_NUL;

    view_out->str     = (const char *)(p + str_offset);
    view_out->str_len = str_length;
    return NIPC_OK;
}

/* ------------------------------------------------------------------ */
/*  Server-side typed dispatch helpers                                 */
/* ------------------------------------------------------------------ */

bool nipc_dispatch_increment(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    nipc_increment_handler_fn handler, void *user)
{
    uint64_t value;
    if (nipc_increment_decode(req, req_len, &value) != NIPC_OK)
        return false;

    uint64_t result;
    if (!handler(user, value, &result))
        return false;

    *resp_len = nipc_increment_encode(result, resp, resp_size);
    return *resp_len > 0;
}

bool nipc_dispatch_string_reverse(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    nipc_string_reverse_handler_fn handler, void *user)
{
    nipc_string_reverse_view_t view;
    if (nipc_string_reverse_decode(req, req_len, &view) != NIPC_OK)
        return false;

    /* Provide a scratch buffer for the handler's response string.
     * The handler writes the response string into it; we encode after. */
    uint32_t capacity = (resp_size > NIPC_STRING_REVERSE_HDR_SIZE + 1)
                            ? (uint32_t)(resp_size - NIPC_STRING_REVERSE_HDR_SIZE - 1)
                            : 0;
    char *scratch = (char *)(resp + NIPC_STRING_REVERSE_HDR_SIZE);

    uint32_t response_str_len = 0;
    if (!handler(user, view.str, view.str_len,
                 scratch, capacity, &response_str_len))
        return false;

    /* Encode from the scratch area (already at the right offset) */
    *resp_len = nipc_string_reverse_encode(scratch, response_str_len,
                                            resp, resp_size);
    return *resp_len > 0;
}

nipc_error_t nipc_dispatch_cgroups_snapshot(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    uint32_t max_items,
    nipc_cgroups_handler_fn handler, void *user)
{
    nipc_cgroups_req_t request;
    nipc_error_t err = nipc_cgroups_req_decode(req, req_len, &request);
    if (err != NIPC_OK)
        return err;

    nipc_cgroups_builder_t builder;
    nipc_cgroups_builder_init(&builder, resp, resp_size, max_items, 0, 0);

    if (!handler(user, &request, &builder)) {
        if (builder.error != NIPC_OK)
            return builder.error;
        return NIPC_ERR_HANDLER_FAILED;
    }

    if (builder.error != NIPC_OK)
        return builder.error;

    *resp_len = nipc_cgroups_builder_finish(&builder);
    return (*resp_len > 0) ? NIPC_OK : NIPC_ERR_OVERFLOW;
}
