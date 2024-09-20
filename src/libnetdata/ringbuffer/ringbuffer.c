// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "ringbuffer_internal.h"

rbuf_t rbuf_create(size_t size)
{
    rbuf_t buffer = mallocz(sizeof(struct rbuf) + size);
    memset(buffer, 0, sizeof(struct rbuf));

    buffer->data = ((char*)buffer) + sizeof(struct rbuf);

    buffer->head = buffer->data;
    buffer->tail = buffer->data;
    buffer->size = size;
    buffer->end = buffer->data + size;

    return buffer;
}

void rbuf_free(rbuf_t buffer)
{
    freez(buffer);
}

void rbuf_flush(rbuf_t buffer)
{
    buffer->head = buffer->data;
    buffer->tail = buffer->data;
    buffer->size_data = 0;
}

char *rbuf_get_linear_insert_range(rbuf_t buffer, size_t *bytes)
{
    *bytes = 0;
    if (buffer->head == buffer->tail && buffer->size_data)
        return NULL;

    *bytes = ((buffer->head >= buffer->tail) ? buffer->end : buffer->tail) - buffer->head;
    return buffer->head;
}

char *rbuf_get_linear_read_range(rbuf_t buffer, size_t *bytes)
{
    *bytes = 0;
    if(buffer->head == buffer->tail && !buffer->size_data)
        return NULL;

    *bytes = ((buffer->tail >= buffer->head) ? buffer->end : buffer->head) - buffer->tail;

    return buffer->tail;
}

int rbuf_bump_head(rbuf_t buffer, size_t bytes)
{
    size_t free_bytes = rbuf_bytes_free(buffer);
    if (bytes > free_bytes)
        return 0;
    int i = buffer->head - buffer->data;
    buffer->head = &buffer->data[(i + bytes) % buffer->size];
    buffer->size_data += bytes;
    return 1;
}

int rbuf_bump_tail_noopt(rbuf_t buffer, size_t bytes)
{
    if (bytes > buffer->size_data)
        return 0;
    int i = buffer->tail - buffer->data;
    buffer->tail = &buffer->data[(i + bytes) % buffer->size];
    buffer->size_data -= bytes;

    return 1;
}

int rbuf_bump_tail(rbuf_t buffer, size_t bytes)
{
    if(!rbuf_bump_tail_noopt(buffer, bytes))
        return 0;

    // if tail catched up with head
    // start writing buffer from beggining
    // this is not necessary (rbuf must work well without it)
    // but helps to optimize big writes as rbuf_get_linear_insert_range
    // will return bigger continuous region
    if(buffer->tail == buffer->head) {
        assert(buffer->size_data == 0);
        rbuf_flush(buffer);
    }

    return 1;
}

size_t rbuf_get_capacity(rbuf_t buffer)
{
    return buffer->size;
}

size_t rbuf_bytes_available(rbuf_t buffer)
{
    return buffer->size_data;
}

size_t rbuf_bytes_free(rbuf_t buffer)
{
    return buffer->size - buffer->size_data;
}

size_t rbuf_push(rbuf_t buffer, const char *data, size_t len)
{
    size_t to_cpy;
    char *w_ptr = rbuf_get_linear_insert_range(buffer, &to_cpy);
    if(!to_cpy)
        return to_cpy;

    to_cpy = MIN(to_cpy, len);
    memcpy(w_ptr, data, to_cpy);
    rbuf_bump_head(buffer, to_cpy);
    if(to_cpy < len)
        to_cpy += rbuf_push(buffer, &data[to_cpy], len - to_cpy);
    return to_cpy;
}

size_t rbuf_pop(rbuf_t buffer, char *data, size_t len)
{
    size_t to_cpy;
    const char *r_ptr = rbuf_get_linear_read_range(buffer, &to_cpy);
    if(!to_cpy)
        return to_cpy;

    to_cpy = MIN(to_cpy, len);
    memcpy(data, r_ptr, to_cpy);
    rbuf_bump_tail(buffer, to_cpy);
    if(to_cpy < len)
        to_cpy += rbuf_pop(buffer, &data[to_cpy], len - to_cpy);
    return to_cpy;
}

static inline void rbuf_ptr_inc(rbuf_t buffer, const char **ptr)
{
    (*ptr)++;
    if(*ptr >= buffer->end)
        *ptr = buffer->data;
}

int rbuf_memcmp(rbuf_t buffer, const char *haystack, const char *needle, size_t needle_bytes)
{
    const char *end = needle + needle_bytes;

    // as head==tail can mean 2 things here
    if (haystack == buffer->head && buffer->size_data) {
        if (*haystack != *needle)
            return (*haystack - *needle);
        rbuf_ptr_inc(buffer, &haystack);
        needle++;
    }

    while (haystack != buffer->head && needle != end) {
        if (*haystack != *needle)
            return (*haystack - *needle);
        rbuf_ptr_inc(buffer, &haystack);
        needle++;
    }
    return 0;
}

int rbuf_memcmp_n(rbuf_t buffer, const char *to_cmp, size_t to_cmp_bytes)
{
    return rbuf_memcmp(buffer, buffer->tail, to_cmp, to_cmp_bytes);
}

char *rbuf_find_bytes(rbuf_t buffer, const char *needle, size_t needle_bytes, int *found_idx)
{
    const char *ptr = buffer->tail;
    *found_idx = 0;

    if (!rbuf_bytes_available(buffer))
        return NULL;

    if (buffer->head == buffer->tail && buffer->size_data) {
        if(!rbuf_memcmp(buffer, ptr, needle, needle_bytes))
            return (char *)ptr;
        rbuf_ptr_inc(buffer, &ptr);
        (*found_idx)++;
    }

    while (ptr != buffer->head)
    {
        if(!rbuf_memcmp(buffer, ptr, needle, needle_bytes))
            return (char *)ptr;
        rbuf_ptr_inc(buffer, &ptr);
        (*found_idx)++;
    }
    return NULL;
}
