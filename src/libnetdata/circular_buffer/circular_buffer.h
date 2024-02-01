#ifndef CIRCULAR_BUFFER_H
#define CIRCULAR_BUFFER_H 1

#include <string.h>

struct circular_buffer {
    size_t size, write, read, max_size;
    size_t *statistics;
    char *data;
};

struct circular_buffer *cbuffer_new(size_t initial, size_t max, size_t *statistics);
void cbuffer_free(struct circular_buffer *buf);
int cbuffer_add_unsafe(struct circular_buffer *buf, const char *d, size_t d_len);
void cbuffer_remove_unsafe(struct circular_buffer *buf, size_t num);
size_t cbuffer_next_unsafe(struct circular_buffer *buf, char **start);
size_t cbuffer_available_size_unsafe(struct circular_buffer *buf);
void cbuffer_flush(struct circular_buffer*buf);

#endif
