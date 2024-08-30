// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef RINGBUFFER_H
#define RINGBUFFER_H
#include "../libnetdata.h"

typedef struct rbuf *rbuf_t;

rbuf_t rbuf_create(size_t size);
void rbuf_free(rbuf_t buffer);
void rbuf_flush(rbuf_t buffer);

/* /param bytes how much bytes can be copied into pointer returned
 * /return pointer where data can be copied to or NULL if buffer full
 */
char *rbuf_get_linear_insert_range(rbuf_t buffer, size_t *bytes);
char *rbuf_get_linear_read_range(rbuf_t buffer, size_t *bytes);

int rbuf_bump_head(rbuf_t buffer, size_t bytes);
int rbuf_bump_tail(rbuf_t buffer, size_t bytes);

/* @param buffer related buffer instance
 * @returns total capacity of buffer in bytes (not free/used)
 */
size_t rbuf_get_capacity(rbuf_t buffer);

/* @param buffer related buffer instance
 * @returns count of bytes stored in the buffer
 */
size_t rbuf_bytes_available(rbuf_t buffer);

/* @param buffer related buffer instance
 * @returns count of bytes available/free in the buffer (how many more bytes you can store in this buffer)
 */
size_t rbuf_bytes_free(rbuf_t buffer);

/* writes as many bytes from `data` into the `buffer` as possible
 * but maximum `len` bytes
 */
size_t rbuf_push(rbuf_t buffer, const char *data, size_t len);
size_t rbuf_pop(rbuf_t buffer, char *data, size_t len);

char *rbuf_find_bytes(rbuf_t buffer, const char *needle, size_t needle_bytes, int *found_idx);
int rbuf_memcmp_n(rbuf_t buffer, const char *to_cmp, size_t to_cmp_bytes);

#endif
