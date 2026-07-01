#include "netipc_protocol_cgroups_snapshot_internal.h"

/* Cgroups request (4 bytes) */
_Static_assert(sizeof(nipc_cgroups_req_t) == 4,
               "nipc_cgroups_req_t must be 4 bytes");

/* Cgroups snapshot response header (24 bytes) */
_Static_assert(sizeof(nipc_cgroups_resp_header_t) == 24,
               "nipc_cgroups_resp_header_t must be 24 bytes");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, layout_version) == 0, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, flags) == 2, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, item_count) == 4, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, systemd_enabled) == 8, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, reserved) == 12, "");
_Static_assert(offsetof(nipc_cgroups_resp_header_t, generation) == 16, "");

_Static_assert(sizeof(nipc_cgroups_item_wire_t) == 32,
               "nipc_cgroups_item_wire_t must be 32 bytes");

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
    if (!view || !out || !view->_payload)
        return NIPC_ERR_BAD_LAYOUT;
    if (index >= view->item_count)
        return NIPC_ERR_OUT_OF_BOUNDS;

    /* Overflow already checked in nipc_cgroups_resp_decode, but
     * guard defensively since this is a public API. */
    if (mul_would_overflow((size_t)view->item_count, NIPC_CGROUPS_DIR_ENTRY_SIZE))
        return NIPC_ERR_BAD_ITEM_COUNT;

    size_t dir_start = NIPC_CGROUPS_RESP_HDR_SIZE;
    size_t dir_size  = (size_t)view->item_count * NIPC_CGROUPS_DIR_ENTRY_SIZE;
#if SIZE_MAX <= UINT32_MAX
    if (dir_start > SIZE_MAX - dir_size)
        return NIPC_ERR_BAD_ITEM_COUNT;
#endif
    size_t packed_area_start = dir_start + dir_size;
    if (packed_area_start > view->_payload_len)
        return NIPC_ERR_TRUNCATED;
    size_t packed_area_len = view->_payload_len - packed_area_start;

    /* Read directory entry */
    nipc_batch_entry_t dir_entry;
    memcpy(&dir_entry,
           view->_payload + dir_start + index * sizeof(dir_entry),
           sizeof(dir_entry));
    if (dir_entry.offset % NIPC_ALIGNMENT != 0)
        return NIPC_ERR_BAD_ALIGNMENT;
    if ((uint64_t)dir_entry.offset + dir_entry.length > packed_area_len)
        return NIPC_ERR_OUT_OF_BOUNDS;
    if (dir_entry.length < NIPC_CGROUPS_ITEM_HDR_SIZE)
        return NIPC_ERR_TRUNCATED;

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
