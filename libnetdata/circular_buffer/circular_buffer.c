#include "../libnetdata.h"

struct circular_buffer *cbuffer_new(size_t initial, size_t max, size_t *statistics) {
    struct circular_buffer *buf = mallocz(sizeof(struct circular_buffer));
    buf->size = initial;
    buf->data = mallocz(initial);
    buf->write = 0;
    buf->read = 0;
    buf->max_size = max;
    buf->statistics = statistics;

    if(buf->statistics)
        __atomic_add_fetch(buf->statistics, sizeof(struct circular_buffer) + buf->size, __ATOMIC_RELAXED);

    return buf;
}

void cbuffer_free(struct circular_buffer *buf) {
    if (unlikely(!buf))
        return;

    if(buf->statistics)
        __atomic_sub_fetch(buf->statistics, sizeof(struct circular_buffer) + buf->size, __ATOMIC_RELAXED);

    freez(buf->data);
    freez(buf);
}

static int cbuffer_realloc_unsafe(struct circular_buffer *buf) {
    // Check that we can grow
    if (buf->size >= buf->max_size)
        return 1;

    size_t old_size = buf->size;
    size_t new_size = buf->size * 2;
    if (new_size > buf->max_size)
        new_size = buf->max_size;

    // We know that: size < new_size <= max_size
    // For simplicity align the current data at the bottom of the new buffer
    char *new_data = mallocz(new_size);
    if (buf->read == buf->write)
        buf->write = 0; // buffer is empty
    else if (buf->read < buf->write) {
        memcpy(new_data, buf->data + buf->read, buf->write - buf->read);
        buf->write -= buf->read;
    } else {
        size_t top_part = buf->size - buf->read;
        memcpy(new_data, buf->data + buf->read, top_part);
        memcpy(new_data + top_part, buf->data, buf->write);
        buf->write = top_part + buf->write;
    }
    buf->read = 0;

    // Switch buffers
    freez(buf->data);
    buf->data = new_data;
    buf->size = new_size;

    if(buf->statistics)
        __atomic_add_fetch(buf->statistics, new_size - old_size, __ATOMIC_RELAXED);

    return 0;
}

size_t cbuffer_available_size_unsafe(struct circular_buffer *buf) {
    size_t len = (buf->write >= buf->read) ? (buf->write - buf->read) : (buf->size - buf->read + buf->write);
    return buf->max_size - len;
}

int cbuffer_add_unsafe(struct circular_buffer *buf, const char *d, size_t d_len) {
    size_t len = (buf->write >= buf->read) ? (buf->write - buf->read) : (buf->size - buf->read + buf->write);
    while (d_len + len >= buf->size) {
        if (cbuffer_realloc_unsafe(buf)) {
            return 1;
        }
    }
    // Guarantee: write + d_len cannot hit read
    if (buf->write + d_len < buf->size) {
        memcpy(buf->data + buf->write, d, d_len);
        buf->write += d_len;
    }
    else {
        size_t top_part = buf->size - buf->write;
        memcpy(buf->data + buf->write, d, top_part);
        memcpy(buf->data, d + top_part, d_len - top_part); 
        buf->write = d_len - top_part;
    }
    return 0;
}

// Assume caller does not remove too many bytes (i.e. read will jump over write)
void cbuffer_remove_unsafe(struct circular_buffer *buf, size_t num) {
    buf->read += num;
    // Assume num < size (i.e. caller cannot remove more bytes than are in the buffer)
    if (buf->read >= buf->size)
        buf->read -= buf->size;
}

size_t cbuffer_next_unsafe(struct circular_buffer *buf, char **start) {
    if (start != NULL)
      *start = buf->data + buf->read;

    if (buf->read <= buf->write) {
        return buf->write - buf->read;      // Includes empty case
    }
    return buf->size - buf->read;
}

void cbuffer_flush(struct circular_buffer*buf) {
    buf->write = 0;
    buf->read = 0;
}