#ifndef CIRCULAR_BUFFER_H
#define CIRCULAR_BUFFER_H 1

#include <string.h>

struct circular_buffer {
    size_t size, write, read, max_size;
    size_t *statistics;
    char *data;
};

// Allocation/deallocation functions
struct circular_buffer *cbuffer_new(size_t initial, size_t max, size_t *statistics);
void cbuffer_free(struct circular_buffer *buf);

// Static allocation support
void cbuffer_init(struct circular_buffer *buf, size_t initial, size_t max, size_t *statistics);
void cbuffer_cleanup(struct circular_buffer *buf);

// Buffer operations
int cbuffer_add_unsafe(struct circular_buffer *buf, const char *d, size_t d_len);
void cbuffer_remove_unsafe(struct circular_buffer *buf, size_t num);
size_t cbuffer_next_unsafe(struct circular_buffer *buf, char **start);
size_t cbuffer_available_size_unsafe(struct circular_buffer *buf);
void cbuffer_flush(struct circular_buffer *buf);

// Reserve/commit operations for direct buffer access
char *cbuffer_reserve_unsafe(struct circular_buffer *buf, size_t size);
void cbuffer_commit_reserved_unsafe(struct circular_buffer *buf, size_t size);

// Check if a size is wrapped in buffer and unwrap if necessary
bool cbuffer_ensure_unwrapped_size(struct circular_buffer *buf, size_t size);

size_t cbuffer_used_size_unsafe(struct circular_buffer *buf);

#endif
