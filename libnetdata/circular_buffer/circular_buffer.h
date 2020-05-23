#ifndef CIRCULAR_BUFFER_H
#define CIRCULAR_BUFFER_H 1

#include <string.h>

struct circular_buffer {
    size_t size, write, read, max_size;
    char *data;
    netdata_mutex_t mutex;
};

struct circular_buffer *cbuffer_new(size_t initial, size_t max);
int cbuffer_add(struct circular_buffer *buf, const char *d, size_t d_len);
void cbuffer_remove(struct circular_buffer *buf, size_t num);
size_t cbuffer_next_unsafe(struct circular_buffer *buf, char **start);
#endif
