// SPDX-License-Identifier: GPL-3.0-or-later

#include "gorilla.h"

#include <cassert>
#include <climits>
#include <cstdio>
#include <cstring>

using std::size_t;

template <typename T>
static constexpr size_t bit_size() noexcept
{
    static_assert((sizeof(T) * CHAR_BIT) == 32 || (sizeof(T) * CHAR_BIT) == 64,
                  "Word size should be 32 or 64 bits.");
    return (sizeof(T) * CHAR_BIT);
}

/*
 * Low-level bitstream operations, allowing us to read/write individual bits.
*/

template<typename Word>
struct bit_stream_t {
    Word *buffer;
    size_t capacity;
    size_t position;
};

template<typename Word>
static bit_stream_t<Word> bit_stream_new(Word *buffer, Word capacity) {
    bit_stream_t<Word> bs;

    bs.buffer = buffer;
    bs.capacity = capacity * bit_size<Word>();
    bs.position = 0;

    return bs;
}

template<typename Word>
static bool bit_stream_write(bit_stream_t<Word> *bs, Word value, size_t nbits) {
    assert(nbits > 0 && nbits <= bit_size<Word>());
    assert(bs->capacity >= (bs->position + nbits));

    if (bs->position + nbits > bs->capacity) {
        return false;
    }

    const size_t index = bs->position / bit_size<Word>();
    const size_t offset = bs->position % bit_size<Word>();
    bs->position += nbits;

    if (offset == 0) {
        bs->buffer[index] = value;
    } else {
        const size_t remaining_bits = bit_size<Word>() - offset;

        // write the lower part of the value
        const Word low_bits_mask = ((Word) 1 << remaining_bits) - 1;
        const Word lowest_bits_in_value = value & low_bits_mask;
        bs->buffer[index] |= (lowest_bits_in_value << offset);

        if (nbits > remaining_bits) {
            // write the upper part of the value
            const Word high_bits_mask = ~low_bits_mask;
            const Word highest_bits_in_value = (value & high_bits_mask) >> (remaining_bits);
            bs->buffer[index + 1] = highest_bits_in_value;
        }
    }

    return true;
}

template<typename Word>
static bool bit_stream_read(bit_stream_t<Word> *bs, Word *value, size_t nbits) {
    assert(nbits > 0 && nbits <= bit_size<Word>());
    assert(bs->capacity >= (bs->position + nbits));

    if (bs->position + nbits > bs->capacity) {
        return false;
    }

    const size_t index = bs->position / bit_size<Word>();
    const size_t offset = bs->position % bit_size<Word>();
    bs->position += nbits;

    if (offset == 0) {
        *value = (nbits == bit_size<Word>()) ?
                    bs->buffer[index] :
                    bs->buffer[index] & (((Word) 1 << nbits) - 1);
    } else {
        const size_t remaining_bits = bit_size<Word>() - offset;

        // extract the lower part of the value
        if (nbits < remaining_bits) {
            *value = (bs->buffer[index] >> offset) & (((Word) 1 << nbits) - 1);
        } else {
            *value = (bs->buffer[index] >> offset) & (((Word) 1 << remaining_bits) - 1);
            nbits -= remaining_bits;
            *value |= (bs->buffer[index + 1] & (((Word) 1 << nbits) - 1)) << remaining_bits;
        }
    }

    return true;
}

/*
 * High-level Gorilla codec implementation
*/

template<typename Word>
struct bit_code_t {
    bit_stream_t<Word> bs;
    Word entries;
    Word prev_number;
    Word prev_xor;
    Word prev_xor_lzc;
};

template<typename Word>
static void bit_code_init(bit_code_t<Word> *bc, Word *buffer, Word capacity) {
    bc->bs = bit_stream_new(buffer, capacity);

    bc->entries = 0;
    bc->prev_number = 0;
    bc->prev_xor = 0;
    bc->prev_xor_lzc = 0;

    // reserved two words:
    //     Buffer[0] -> number of entries written
    //     Buffer[1] -> number of bits written

    bc->bs.position += 2 * bit_size<Word>();
}

template<typename Word>
static bool bit_code_read(bit_code_t<Word> *bc, Word *number) {
    bit_stream_t<Word> *bs = &bc->bs;

    bc->entries++;

    // read the first number
    if (bc->entries == 1) {
        bool ok = bit_stream_read(bs, number, bit_size<Word>());
        bc->prev_number = *number;
        return ok;
    }

    // process same-number bit
    Word is_same_number;
    if (!bit_stream_read(bs, &is_same_number, 1)) {
        return false;        
    }

    if (is_same_number) {
        *number = bc->prev_number;
        return true;
    }

    // proceess same-xor-lzc bit
    Word xor_lzc = bc->prev_xor_lzc;

    Word same_xor_lzc;
    if (!bit_stream_read(bs, &same_xor_lzc, 1)) {
        return false;
    }

    if (!same_xor_lzc) {
        if (!bit_stream_read(bs, &xor_lzc, (bit_size<Word>() == 32) ? 5 : 6)) {
            return false;        
        }
    }

    // process the non-lzc suffix
    Word xor_value = 0;
    if (!bit_stream_read(bs, &xor_value, bit_size<Word>() - xor_lzc)) {
        return false;        
    }

    *number = (bc->prev_number ^ xor_value);

    bc->prev_number = *number;
    bc->prev_xor_lzc = xor_lzc;
    bc->prev_xor = xor_value;

    return true;
}

template<typename Word>
static bool bit_code_write(bit_code_t<Word> *bc, const Word number) {
    bit_stream_t<Word> *bs = &bc->bs;
    Word position = bs->position;

    bc->entries++;

    // this is the first number we are writing
    if (bc->entries == 1) {
        bc->prev_number = number;
        return bit_stream_write(bs, number, bit_size<Word>());
    }
    
    // write true/false based on whether we got the same number or not.
    if (number == bc->prev_number) {
        return bit_stream_write(bs, static_cast<Word>(1), 1);
    } else {
        if (bit_stream_write(bs, static_cast<Word>(0), 1) == false) {
            return false;
        }
    }

    // otherwise:
    //     - compute the non-zero xor
    //     - find its leading-zero count

    Word xor_value = bc->prev_number ^ number;
    // FIXME: Use SFINAE
    Word xor_lzc = (bit_size<Word>() == 32) ? __builtin_clz(xor_value) : __builtin_clzll(xor_value);
    Word is_xor_lzc_same = (xor_lzc == bc->prev_xor_lzc) ? 1 : 0;

    if (is_xor_lzc_same) {
        // xor-lzc is same
        if (bit_stream_write(bs, static_cast<Word>(1), 1) == false) {
            goto RET_FALSE;
        }
    } else {
        // xor-lzc is different
        if (bit_stream_write(bs, static_cast<Word>(0), 1) == false) {
            goto RET_FALSE;
        }
        
        if (bit_stream_write(bs, xor_lzc, (bit_size<Word>() == 32) ? 5 : 6) == false) {
            goto RET_FALSE;
        }
    }

    // write the bits of the XOR value without the LZC prefix
    if (bit_stream_write(bs, xor_value, bit_size<Word>() - xor_lzc) == false) {
        goto RET_FALSE; 
    }

    bc->prev_number = number;
    bc->prev_xor_lzc = xor_lzc;
    return true;

RET_FALSE:
    bc->bs.position = position;
    return false;
}

// only valid for writers
template<typename Word>
static bool bit_code_flush(bit_code_t<Word> *bc) {
    bit_stream_t<Word> *bs = &bc->bs;

    Word num_entries_written = bc->entries;
    Word num_bits_written = bs->position;

    // we want to write these at the beginning
    bs->position = 0;

    if (!bit_stream_write(bs, num_entries_written, bit_size<Word>())) {
        return false;
    }

    if (!bit_stream_write(bs, num_bits_written, bit_size<Word>())) {
        return false;
    }

    bs->position = num_bits_written;
    return true;
}

// only valid for readers
template<typename Word>
static bool bit_code_info(bit_code_t<Word> *bc, Word *num_entries_written,
                                                Word *num_bits_written) {
    bit_stream_t<Word> *bs = &bc->bs;

    assert(bs->position == 2 * bit_size<Word>());
    if (bs->capacity < (2 * bit_size<Word>())) {
        return false;
    }

    if (num_entries_written) {
        *num_entries_written = bs->buffer[0];
    }
    if (num_bits_written) {
        *num_bits_written = bs->buffer[1];
    }

    return true;
}

template<typename Word>
static size_t gorilla_encode(Word *dst, Word dst_len, const Word *src, Word src_len) {
    bit_code_t<Word> bcw;

    bit_code_init(&bcw, dst, dst_len);

    for (size_t i = 0; i != src_len; i++) {
        if (!bit_code_write(&bcw, src[i]))
            return 0;
    }

    if (!bit_code_flush(&bcw))
        return 0;

    return src_len;
}

template<typename Word>
static size_t gorilla_decode(Word *dst, Word dst_len, const Word *src, Word src_len) {
    bit_code_t<Word> bcr;

    bit_code_init(&bcr, (Word *) src, src_len);

    Word num_entries;
    if (!bit_code_info(&bcr, &num_entries, (Word *) NULL)) {
        return 0;
    }
    if (num_entries > dst_len) {
        return 0;
    }
    
    for (size_t i = 0; i != num_entries; i++) {
        if (!bit_code_read(&bcr, &dst[i]))
            return 0;
    }

    return num_entries;
}

/*
 * Low-level public API
*/

// 32-bit API

void bit_code_writer_u32_init(bit_code_writer_u32_t *bcw, uint32_t *buffer, uint32_t capacity) {
    bit_code_t<uint32_t> *bc = (bit_code_t<uint32_t> *) bcw;
    bit_code_init(bc, buffer, capacity);
}

bool bit_code_writer_u32_write(bit_code_writer_u32_t *bcw, const uint32_t number) {
    bit_code_t<uint32_t> *bc = (bit_code_t<uint32_t> *) bcw;
    return bit_code_write(bc, number);
}

bool bit_code_writer_u32_flush(bit_code_writer_u32_t *bcw) {
    bit_code_t<uint32_t> *bc = (bit_code_t<uint32_t> *) bcw;
    return bit_code_flush(bc);
}

void bit_code_reader_u32_init(bit_code_reader_u32_t *bcr, uint32_t *buffer, uint32_t capacity) {
    bit_code_t<uint32_t> *bc = (bit_code_t<uint32_t> *) bcr;
    bit_code_init(bc, buffer, capacity);
}

bool bit_code_reader_u32_read(bit_code_reader_u32_t *bcr, uint32_t *number) {
    bit_code_t<uint32_t> *bc = (bit_code_t<uint32_t> *) bcr;
    return bit_code_read(bc, number);
}

bool bit_code_reader_u32_info(bit_code_reader_u32_t *bcr, uint32_t *num_entries_written,
                                                          uint32_t *num_bits_written) {
    bit_code_t<uint32_t> *bc = (bit_code_t<uint32_t> *) bcr;
    return bit_code_info(bc, num_entries_written, num_bits_written);
}

// 64-bit API

void bit_code_writer_u64_init(bit_code_writer_u64_t *bcw, uint64_t *buffer, uint64_t capacity) {
    bit_code_t<uint64_t> *bc = (bit_code_t<uint64_t> *) bcw;
    bit_code_init(bc, buffer, capacity);
}

bool bit_code_writer_u64_write(bit_code_writer_u64_t *bcw, const uint64_t number) {
    bit_code_t<uint64_t> *bc = (bit_code_t<uint64_t> *) bcw;
    return bit_code_write(bc, number);
}

bool bit_code_writer_u64_flush(bit_code_writer_u64_t *bcw) {
    bit_code_t<uint64_t> *bc = (bit_code_t<uint64_t> *) bcw;
    return bit_code_flush(bc);
}

void bit_code_reader_u64_init(bit_code_reader_u64_t *bcr, uint64_t *buffer, uint64_t capacity) {
    bit_code_t<uint64_t> *bc = (bit_code_t<uint64_t> *) bcr;
    bit_code_init(bc, buffer, capacity);
}

bool bit_code_reader_u64_read(bit_code_reader_u64_t *bcr, uint64_t *number) {
    bit_code_t<uint64_t> *bc = (bit_code_t<uint64_t> *) bcr;
    return bit_code_read(bc, number);
}

bool bit_code_reader_u64_info(bit_code_reader_u64_t *bcr, uint64_t *num_entries_written,
                                                          uint64_t *num_bits_written) {
    bit_code_t<uint64_t> *bc = (bit_code_t<uint64_t> *) bcr;
    return bit_code_info(bc, num_entries_written, num_bits_written);
}

/*
 * High-level public API
*/

// 32-bit API

size_t gorilla_encode_u32(uint32_t *dst, size_t dst_len, const uint32_t *src, size_t src_len) {
    return gorilla_encode(dst, (uint32_t) dst_len, src, (uint32_t) src_len);
}

size_t gorilla_decode_u32(uint32_t *dst, size_t dst_len, const uint32_t *src, size_t src_len) {
    return gorilla_decode(dst, (uint32_t) dst_len, src, (uint32_t) src_len);
}

// 64-bit API

size_t gorilla_encode_u64(uint64_t *dst, size_t dst_len, const uint64_t *src, size_t src_len) {
    return gorilla_encode(dst, (uint64_t) dst_len, src, (uint64_t) src_len);
}

size_t gorilla_decode_u64(uint64_t *dst, size_t dst_len, const uint64_t *src, size_t src_len) {
    return gorilla_decode(dst, (uint64_t) dst_len, src, (uint64_t) src_len);
}

/*
 * Internal code used for fuzzing the library
*/

#ifdef ENABLE_FUZZER

#include <vector>

template<typename Word>
static std::vector<Word> random_vector(const uint8_t *data, size_t size) {
    std::vector<Word> V;

    V.reserve(1024);

    while (size >= sizeof(Word)) {
        size -= sizeof(Word);

        Word w;
        memcpy(&w, &data[size], sizeof(Word));
        V.push_back(w);
    }

    return V;
}

template<typename Word>
static void check_equal_buffers(Word *lhs, Word lhs_size, Word *rhs, Word rhs_size) {
    assert((lhs_size == rhs_size) && "Buffers have different size.");

    for (size_t i = 0; i != lhs_size; i++) {
        assert((lhs[i] == rhs[i]) && "Buffers differ");
    }
}

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    // 32-bit tests
    {
        if (Size < 4)
            return 0;

        std::vector<uint32_t> RandomData = random_vector<uint32_t>(Data, Size);
        std::vector<uint32_t> EncodedData(10 * RandomData.capacity(), 0);
        std::vector<uint32_t> DecodedData(10 * RandomData.capacity(), 0);

        size_t num_entries_written = gorilla_encode_u32(EncodedData.data(), EncodedData.size(),
                                                        RandomData.data(), RandomData.size());
        size_t num_entries_read = gorilla_decode_u32(DecodedData.data(), DecodedData.size(),
                                                     EncodedData.data(), EncodedData.size());

        assert(num_entries_written == num_entries_read);
        check_equal_buffers(RandomData.data(), (uint32_t) RandomData.size(),
                            DecodedData.data(), (uint32_t) RandomData.size());
    }

    // 64-bit tests
    {
        if (Size < 8)
            return 0;

        std::vector<uint64_t> RandomData = random_vector<uint64_t>(Data, Size);
        std::vector<uint64_t> EncodedData(10 * RandomData.capacity(), 0);
        std::vector<uint64_t> DecodedData(10 * RandomData.capacity(), 0);

        size_t num_entries_written = gorilla_encode_u64(EncodedData.data(), EncodedData.size(),
                                                        RandomData.data(), RandomData.size());
        size_t num_entries_read = gorilla_decode_u64(DecodedData.data(), DecodedData.size(),
                                                     EncodedData.data(), EncodedData.size());

        assert(num_entries_written == num_entries_read);
        check_equal_buffers(RandomData.data(), (uint64_t) RandomData.size(),
                            DecodedData.data(), (uint64_t) RandomData.size());
    }

    return 0;
}

#endif /* ENABLE_FUZZER */

#ifdef ENABLE_BENCHMARK

#include <benchmark/benchmark.h>
#include <random>

static size_t NumItems = 1024;

static void BM_EncodeU32Numbers(benchmark::State& state) {
    std::random_device rd;
    std::mt19937 mt(rd());
    std::uniform_int_distribution<uint32_t> dist(0x0, 0x0000FFFF);

    std::vector<uint32_t> RandomData;
    for (size_t idx = 0; idx != NumItems; idx++) {
        RandomData.push_back(dist(mt));
    }
    std::vector<uint32_t> EncodedData(10 * RandomData.capacity(), 0);

    for (auto _ : state) {
        benchmark::DoNotOptimize(
        gorilla_encode_u32(EncodedData.data(), EncodedData.size(),
                                  RandomData.data(), RandomData.size())
        );
        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint32_t));
}
BENCHMARK(BM_EncodeU32Numbers);

static void BM_DecodeU32Numbers(benchmark::State& state) {
    std::random_device rd;
    std::mt19937 mt(rd());
    std::uniform_int_distribution<uint32_t> dist(0x0, 0xFFFFFFFF);

    std::vector<uint32_t> RandomData;
    for (size_t idx = 0; idx != NumItems; idx++) {
        RandomData.push_back(dist(mt));
    }
    std::vector<uint32_t> EncodedData(10 * RandomData.capacity(), 0);
    std::vector<uint32_t> DecodedData(10 * RandomData.capacity(), 0);

    gorilla_encode_u32(EncodedData.data(), EncodedData.size(),
                       RandomData.data(), RandomData.size());

    for (auto _ : state) {
        benchmark::DoNotOptimize(
            gorilla_decode_u32(DecodedData.data(), DecodedData.size(),
                               EncodedData.data(), EncodedData.size())
        );
        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint32_t));
}
// Register the function as a benchmark
BENCHMARK(BM_DecodeU32Numbers);

static void BM_EncodeU64Numbers(benchmark::State& state) {
    std::random_device rd;
    std::mt19937 mt(rd());
    std::uniform_int_distribution<uint64_t> dist(0x0, 0x0000FFFF);

    std::vector<uint64_t> RandomData;
    for (size_t idx = 0; idx != 1024; idx++) {
        RandomData.push_back(dist(mt));
    }
    std::vector<uint64_t> EncodedData(10 * RandomData.capacity(), 0);

    for (auto _ : state) {
        benchmark::DoNotOptimize(
        gorilla_encode_u64(EncodedData.data(), EncodedData.size(),
                                  RandomData.data(), RandomData.size())
        );
        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint64_t));
}
BENCHMARK(BM_EncodeU64Numbers);

static void BM_DecodeU64Numbers(benchmark::State& state) {
    std::random_device rd;
    std::mt19937 mt(rd());
    std::uniform_int_distribution<uint64_t> dist(0x0, 0xFFFFFFFF);

    std::vector<uint64_t> RandomData;
    for (size_t idx = 0; idx != 1024; idx++) {
        RandomData.push_back(dist(mt));
    }
    std::vector<uint64_t> EncodedData(10 * RandomData.capacity(), 0);
    std::vector<uint64_t> DecodedData(10 * RandomData.capacity(), 0);

    gorilla_encode_u64(EncodedData.data(), EncodedData.size(),
                       RandomData.data(), RandomData.size());

    for (auto _ : state) {
        benchmark::DoNotOptimize(
            gorilla_decode_u64(DecodedData.data(), DecodedData.size(),
                               EncodedData.data(), EncodedData.size())
        );
        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint64_t));
}
// Register the function as a benchmark
BENCHMARK(BM_DecodeU64Numbers);

#endif /* ENABLE_BENCHMARK */
