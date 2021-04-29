#ifndef CIRCULAR_BUFFER_H
#define CIRCULAR_BUFFER_H 1

#include <string.h>

struct circular_buffer {
    size_t size, write, read, max_size;
    char *data;
};

extern struct circular_buffer *cbuffer_new(size_t initial, size_t max);
extern void cbuffer_free(struct circular_buffer *buf);
extern int cbuffer_add_unsafe(struct circular_buffer *buf, const char *d, size_t d_len);
extern void cbuffer_remove_unsafe(struct circular_buffer *buf, size_t num);
extern size_t cbuffer_next_unsafe(struct circular_buffer *buf, char **start);
#endif
