#include "netipc_protocol_internal.h"

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

    if (str_offset != NIPC_STRING_REVERSE_HDR_SIZE)
        return NIPC_ERR_BAD_LAYOUT;
    if ((uint64_t)str_offset + str_length + 1 > buf_len)
        return NIPC_ERR_OUT_OF_BOUNDS;

    if (p[str_offset + str_length] != '\0')
        return NIPC_ERR_MISSING_NUL;

    view_out->str     = (const char *)(p + str_offset);
    view_out->str_len = str_length;
    return NIPC_OK;
}

bool nipc_dispatch_string_reverse(
    const uint8_t *req, size_t req_len,
    uint8_t *resp, size_t resp_size, size_t *resp_len,
    nipc_string_reverse_handler_fn handler, void *user)
{
    nipc_string_reverse_view_t view;
    if (nipc_string_reverse_decode(req, req_len, &view) != NIPC_OK)
        return false;

    /* The handler writes the response string into scratch space that is
     * already positioned at the codec payload offset. */
    uint32_t capacity = (resp_size > NIPC_STRING_REVERSE_HDR_SIZE + 1)
                            ? (uint32_t)(resp_size - NIPC_STRING_REVERSE_HDR_SIZE - 1)
                            : 0;
    char *scratch = (char *)(resp + NIPC_STRING_REVERSE_HDR_SIZE);

    uint32_t response_str_len = 0;
    if (!handler(user, view.str, view.str_len,
                 scratch, capacity, &response_str_len))
        return false;

    *resp_len = nipc_string_reverse_encode(scratch, response_str_len,
                                           resp, resp_size);
    return *resp_len > 0;
}
