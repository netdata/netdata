/*
 * netipc_protocol.c - Wire envelope and codec implementation.
 *
 * Localhost-only IPC — struct layouts match wire format exactly.
 * Encode/decode uses direct memcpy (single copy per struct).
 * No endianness conversion — both peers share host byte order.
 */

#include "netipc_protocol_internal.h"

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

    memset(out, 0, sizeof(*out));
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
