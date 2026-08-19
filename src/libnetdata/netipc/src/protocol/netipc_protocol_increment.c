#include "netipc_protocol_internal.h"

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
