// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef GORILLA_H
#define GORILLA_H

#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

struct gorilla_buffer;

typedef struct {
    struct gorilla_buffer *next;
    uint32_t entries;
    uint32_t nbits;
} gorilla_header_t;

typedef struct gorilla_buffer {
    gorilla_header_t header;
    uint32_t data[];
} gorilla_buffer_t;

typedef struct {
    gorilla_buffer_t *head_buffer;
    gorilla_buffer_t *last_buffer;

    uint32_t prev_number;
    uint32_t prev_xor_lzc;

    // in bits
    uint32_t capacity;
} gorilla_writer_t;

typedef struct {
    const gorilla_buffer_t *buffer;

    // number of values
    size_t entries;
    size_t index;

    // in bits
    size_t capacity; // FIXME: this not needed on the reader's side
    size_t position;

    uint32_t prev_number;
    uint32_t prev_xor_lzc;
    uint32_t prev_xor;
} gorilla_reader_t;

gorilla_writer_t gorilla_writer_init(gorilla_buffer_t *gbuf, size_t n);
void gorilla_writer_add_buffer(gorilla_writer_t *gw, gorilla_buffer_t *gbuf, size_t n);
bool gorilla_writer_write(gorilla_writer_t *gw, uint32_t number);
uint32_t gorilla_writer_entries(const gorilla_writer_t *gw);

struct aral;
void gorilla_writer_aral_unmark(const gorilla_writer_t *gw, struct aral *ar);

gorilla_reader_t gorilla_writer_get_reader(const gorilla_writer_t *gw);

gorilla_buffer_t *gorilla_writer_drop_head_buffer(gorilla_writer_t *gw);

uint32_t gorilla_writer_actual_nbytes(const gorilla_writer_t *gw);
uint32_t gorilla_writer_optimal_nbytes(const gorilla_writer_t *gw);
bool gorilla_writer_serialize(const gorilla_writer_t *gw, uint8_t *dst, uint32_t dst_size);

uint32_t gorilla_buffer_patch(gorilla_buffer_t *buf);
size_t gorilla_buffer_unpatched_nbuffers(const gorilla_buffer_t *gbuf);
size_t gorilla_buffer_unpatched_nbytes(const gorilla_buffer_t *gbuf);
gorilla_reader_t gorilla_reader_init(gorilla_buffer_t *buf);
bool gorilla_reader_read(gorilla_reader_t *gr, uint32_t *number);

#define RRDENG_GORILLA_32BIT_SLOT_BYTES sizeof(uint32_t)
#define RRDENG_GORILLA_32BIT_SLOT_BITS (RRDENG_GORILLA_32BIT_SLOT_BYTES * CHAR_BIT)
#define RRDENG_GORILLA_32BIT_BUFFER_SLOTS 128
#define RRDENG_GORILLA_32BIT_BUFFER_SIZE (RRDENG_GORILLA_32BIT_BUFFER_SLOTS * RRDENG_GORILLA_32BIT_SLOT_BYTES)

#ifdef __cplusplus
}
#endif

#endif /* GORILLA_H */
