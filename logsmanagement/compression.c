// SPDX-License-Identifier: GPL-3.0-or-later

/** @file compression.c
 *  @brief This is the file containing the implementation of 
 *         the LZ4 decompression API.
 */

#include "compression.h"
#include "helper.h"

/**
 * @brief Decompress compressed text
 * @details This function will decompress the compressed text stored in msg. If 
 * out_buf is NULL, the results of the decompression will be stored inside the 
 * Message_t msg struct. Otherwise, the results will be stored in out_buf 
 * (which must be large enough to store them).
 * @param[in,out] msg Message_t struct that stores the compressed text, its 
 * size and the decompressed results as well, in case out_buf is NULL.
 * @param[out] out_buf Buffer to store the results of the decompression. 
 * In case it is NULL, the results will be stored back in the msg struct. The 
 * out_buf must be pre-allocated and large enough (equal to msg->text_size at 
 * least).
 */
int decompress_text(Circ_buff_item_t *const msg, char *const out_buf) {
    int rc = 0;

    if(!out_buf) msg->data = mallocz(msg->text_size);

    char *dstBuffer = out_buf ? out_buf : msg->data;

    rc = LZ4_decompress_safe(msg->text_compressed, dstBuffer, msg->text_compressed_size, msg->text_size);
    if (unlikely(rc < 0)) {
        collector_error("Decompression error: %d", rc);
        m_assert(0, "Decompression error");
    }

    return rc;
}
