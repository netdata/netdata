#include "../libnetdata.h"

// Initialize a pre-allocated circular buffer
void cbuffer_init(struct circular_buffer *buf, size_t initial, size_t max, size_t *statistics) {
    if (unlikely(!buf))
        return;

    buf->size = initial;
    buf->data = mallocz(initial);
    buf->write = 0;
    buf->read = 0;
    buf->max_size = max;
    buf->statistics = statistics;

    if(buf->statistics)
        __atomic_add_fetch(buf->statistics, buf->size, __ATOMIC_RELAXED);
}

// Cleanup resources for a pre-allocated circular buffer
void cbuffer_cleanup(struct circular_buffer *buf) {
    if (unlikely(!buf))
        return;

    if(buf->statistics)
        __atomic_sub_fetch(buf->statistics, buf->size, __ATOMIC_RELAXED);

    freez(buf->data);
    buf->data = NULL;
    buf->size = 0;
    buf->write = 0;
    buf->read = 0;
}

// Allocate and initialize a new circular buffer
struct circular_buffer *cbuffer_new(size_t initial, size_t max, size_t *statistics) {
    struct circular_buffer *buf = mallocz(sizeof(struct circular_buffer));
    cbuffer_init(buf, initial, max, statistics);

    if(buf->statistics)
        __atomic_add_fetch(buf->statistics, sizeof(struct circular_buffer), __ATOMIC_RELAXED);

    return buf;
}

// Free a circular buffer allocated with cbuffer_new
void cbuffer_free(struct circular_buffer *buf) {
    if (unlikely(!buf))
        return;

    if(buf->statistics)
        __atomic_sub_fetch(buf->statistics, sizeof(struct circular_buffer), __ATOMIC_RELAXED);

    cbuffer_cleanup(buf);
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

ALWAYS_INLINE
size_t cbuffer_used_size_unsafe(struct circular_buffer *buf) {
    return (buf->write >= buf->read) ? (buf->write - buf->read) : (buf->size - buf->read + buf->write);
}

ALWAYS_INLINE
size_t cbuffer_available_size_unsafe(struct circular_buffer *buf) {
    return buf->max_size - cbuffer_used_size_unsafe(buf);
}

int cbuffer_add_unsafe(struct circular_buffer *buf, const char *d, size_t d_len) {
    size_t len = cbuffer_used_size_unsafe(buf);
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
ALWAYS_INLINE
void cbuffer_remove_unsafe(struct circular_buffer *buf, size_t num) {
    buf->read += num;
    // Assume num < size (i.e. caller cannot remove more bytes than are in the buffer)
    if (buf->read >= buf->size)
        buf->read -= buf->size;
}

ALWAYS_INLINE
size_t cbuffer_next_unsafe(struct circular_buffer *buf, char **start) {
    if (start != NULL)
      *start = buf->data + buf->read;

    if (buf->read <= buf->write) {
        return buf->write - buf->read;      // Includes empty case
    }
    return buf->size - buf->read;
}

ALWAYS_INLINE
void cbuffer_flush(struct circular_buffer*buf) {
    buf->write = 0;
    buf->read = 0;
}

// Ensures that the requested size is available as a contiguous block in the buffer
// Returns true if there's enough data and it's now contiguous, false otherwise
bool cbuffer_ensure_unwrapped_size(struct circular_buffer *buf, size_t size) {
    if (unlikely(!buf || !buf->data))
        return false;

    size_t used = cbuffer_used_size_unsafe(buf);
    if(used < size)
        return false;

    char *unwrapped;
    size_t unwrapped_size = cbuffer_next_unsafe(buf, &unwrapped);
    if(unwrapped_size >= size)
        return true;

    size_t wrapped_size = used - unwrapped_size;

    char *tmp = mallocz(unwrapped_size);
    memcpy(tmp, unwrapped, unwrapped_size);
    cbuffer_remove_unsafe(buf, unwrapped_size);

    memmove(buf->data + unwrapped_size, buf->data, wrapped_size);
    memcpy(buf->data, tmp, unwrapped_size);
    freez(tmp);

    buf->read = 0;
    buf->write = unwrapped_size + wrapped_size;

    return true;
}

// Reserve space in the circular buffer for direct writing
// Returns a pointer to the reserved space, or NULL if reservation fails
char *cbuffer_reserve_unsafe(struct circular_buffer *buf, size_t size) {
    if (unlikely(!buf || !buf->data || size == 0))
        return NULL;

    // First, make sure we have enough space in the buffer
    size_t len = cbuffer_used_size_unsafe(buf);
    while (size + len >= buf->size) {
        if (cbuffer_realloc_unsafe(buf)) {
            // Can't grow buffer anymore
            return NULL;
        }
    }

    if(buf->write + size > buf->size) {
        if (!cbuffer_ensure_unwrapped_size(buf, len))
            return NULL;

        if(buf->read != 0 && buf->write + size > buf->size) {
            // It is a contiguous buffer, but we need to move the data
            // Move the data to the beginning of the buffer
            memmove(buf->data, buf->data + buf->read, buf->write - buf->read);
            buf->write -= buf->read;
            buf->read = 0;
        }
    }

    // Check if we can write contiguously from the current write position
    if (buf->write + size <= buf->size) {
        // Simple case - we have enough space at the current write position
        return buf->data + buf->write;
    }
    else {
        // impossible case since cbuffer_ensure_unwrapped_size() returned true
        return NULL;
    }
}

// Commit the reserved space after writing to it
// Size should be less than or equal to the size passed to cbuffer_reserve_unsafe
void cbuffer_commit_reserved_unsafe(struct circular_buffer *buf, size_t size) {
    if (unlikely(!buf || !buf->data || size == 0))
        return;

    // Update the write pointer
    buf->write += size;

    // Handle wrap-around if we've gone past the buffer boundary
    if (buf->write >= buf->size)
        buf->write -= buf->size;
}
