// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef GORILLA_H
#define GORILLA_H

#include <stdbool.h>
#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Low-level public API
*/

// 32-bit API

typedef struct bit_code_writer_u32 bit_code_writer_u32_t;
typedef struct bit_code_reader_u32 bit_code_reader_u32_t;

void bit_code_writer_u32_init(bit_code_writer_u32_t *bcw, uint32_t *buffer, uint32_t capacity);
bool bit_code_writer_u32_write(bit_code_writer_u32_t *bcw, const uint32_t number);
bool bit_code_writer_u32_flush(bit_code_writer_u32_t *bcw);

void bit_code_reader_u32_init(bit_code_reader_u32_t *bcr, uint32_t *buffer, uint32_t capacity);
bool bit_code_reader_u32_read(bit_code_reader_u32_t *bcr, uint32_t *number);
bool bit_code_reader_u32_info(bit_code_reader_u32_t *bcr, uint32_t *num_entries_written,
                                                          uint64_t *num_bits_written);

// 64-bit API

typedef struct bit_code_writer_u64 bit_code_writer_u64_t;
typedef struct bit_code_reader_u64 bit_code_reader_u64_t;

void bit_code_writer_u64_init(bit_code_writer_u64_t *bcw, uint64_t *buffer, uint64_t capacity);
bool bit_code_writer_u64_write(bit_code_writer_u64_t *bcw, const uint64_t number);
bool bit_code_writer_u64_flush(bit_code_writer_u64_t *bcw);

void bit_code_reader_u64_init(bit_code_reader_u64_t *bcr, uint64_t *buffer, uint64_t capacity);
bool bit_code_reader_u64_read(bit_code_reader_u64_t *bcr, uint64_t *number);
bool bit_code_reader_u64_info(bit_code_reader_u64_t *bcr, uint64_t *num_entries_written,
                                                          uint64_t *num_bits_written);

/*
 * High-level public API
*/

size_t gorilla_encode_u32(uint32_t *dst, size_t dst_len, const uint32_t *src, size_t src_len);
size_t gorilla_decode_u32(uint32_t *dst, size_t dst_len, const uint32_t *src, size_t src_len);

size_t gorilla_encode_u64(uint64_t *dst, size_t dst_len, const uint64_t *src, size_t src_len);
size_t gorilla_decode_u64(uint64_t *dst, size_t dst_len, const uint64_t *src, size_t src_len);

#ifdef __cplusplus
}
#endif

#endif /* GORILLA_H */
