// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef HTTP_SERVER_H
#define HTTP_SERVER_H

#include <stddef.h>

void *h2o_main(void * ptr);

int h2o_stream_write(void *ctx, const char *data, size_t data_len);
size_t h2o_stream_read(void *ctx, char *buf, size_t read_bytes);

bool httpd_is_enabled();

#endif /* HTTP_SERVER_H */
