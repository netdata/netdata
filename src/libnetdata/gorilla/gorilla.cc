// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/common.h"
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

static uint32_t gorilla_buffer_nbytes(uint32_t nbits) {
    uint32_t slots = (nbits + RRDENG_GORILLA_32BIT_SLOT_BITS - 1) / RRDENG_GORILLA_32BIT_SLOT_BITS;
    assert(slots > 0 && slots <= RRDENG_GORILLA_32BIT_BUFFER_SLOTS);

    // this is needed to avoid heap buffer overflow in bit_buffer_read()
    if(slots < RRDENG_GORILLA_32BIT_BUFFER_SLOTS)
        slots++;

    return slots * RRDENG_GORILLA_32BIT_SLOT_BYTES;
}

static void bit_buffer_write(uint32_t *buf, size_t pos, uint32_t v, size_t nbits)
{
    assert(nbits > 0 && nbits <= bit_size<uint32_t>());

    const size_t index = pos / bit_size<uint32_t>();
    const size_t offset = pos % bit_size<uint32_t>();

    pos += nbits;

    if (offset == 0) {
        buf[index] = v;
    } else {
        const size_t remaining_bits = bit_size<uint32_t>() - offset;

        // write the lower part of the value
        const uint32_t low_bits_mask = ((uint32_t) 1 << remaining_bits) - 1;
        const uint32_t lowest_bits_in_value = v & low_bits_mask;
        buf[index] |= (lowest_bits_in_value << offset);

        if (nbits > remaining_bits) {
            // write the upper part of the value
            const uint32_t high_bits_mask = ~low_bits_mask;
            const uint32_t highest_bits_in_value = (v & high_bits_mask) >> (remaining_bits);
            buf[index + 1] = highest_bits_in_value;
        }
    }
}

static void bit_buffer_read(const uint32_t *buf, size_t pos, uint32_t *v, size_t nbits)
{
    assert(nbits > 0 && nbits <= bit_size<uint32_t>());

    const size_t index = pos / bit_size<uint32_t>();
    const size_t offset = pos % bit_size<uint32_t>();

    pos += nbits;

    if (offset == 0) {
        *v = (nbits == bit_size<uint32_t>()) ?
                    buf[index] :
                    buf[index] & (((uint32_t) 1 << nbits) - 1);
    } else {
        const size_t remaining_bits = bit_size<uint32_t>() - offset;

        // extract the lower part of the value
        if (nbits < remaining_bits) {
            *v = (buf[index] >> offset) & (((uint32_t) 1 << nbits) - 1);
        } else {
            *v = (buf[index] >> offset) & (((uint32_t) 1 << remaining_bits) - 1);
            nbits -= remaining_bits;
            *v |= (buf[index + 1] & (((uint32_t) 1 << nbits) - 1)) << remaining_bits;
        }
    }
}

gorilla_writer_t gorilla_writer_init(gorilla_buffer_t *gbuf, size_t n)
{
    gorilla_writer_t gw = gorilla_writer_t {
        .head_buffer = gbuf,
        .last_buffer = NULL,
        .prev_number = 0,
        .prev_xor_lzc = 0,
        .capacity = 0
    };

    gorilla_writer_add_buffer(&gw, gbuf, n);
    return gw;
}

void gorilla_writer_add_buffer(gorilla_writer_t *gw, gorilla_buffer_t *gbuf, size_t n)
{
    gbuf->header.next = NULL;
    gbuf->header.entries = 0;
    gbuf->header.nbits = 0;

    uint32_t capacity = (n * bit_size<uint32_t>()) - (sizeof(gorilla_header_t) * CHAR_BIT);

    gw->prev_number = 0;
    gw->prev_xor_lzc = 0;
    gw->capacity = capacity;

    if (gw->last_buffer)
        gw->last_buffer->header.next = gbuf;

    __atomic_store_n(&gw->last_buffer, gbuf, __ATOMIC_RELEASE);
}

uint32_t gorilla_writer_entries(const gorilla_writer_t *gw) {
    uint32_t entries = 0;

    const gorilla_buffer_t *curr_gbuf = __atomic_load_n(&gw->head_buffer, __ATOMIC_ACQUIRE);
    do {
        const gorilla_buffer_t *next_gbuf = __atomic_load_n(&curr_gbuf->header.next, __ATOMIC_ACQUIRE);

        entries += __atomic_load_n(&curr_gbuf->header.entries, __ATOMIC_ACQUIRE);

        curr_gbuf = next_gbuf;
    } while (curr_gbuf);

    return entries;
}

extern "C" {
    ALWAYS_INLINE_ONLY bool gorilla_writer_write(gorilla_writer_t *gw, uint32_t number)
    {
        gorilla_header_t *hdr = &gw->last_buffer->header;
        uint32_t *data = gw->last_buffer->data;

        // this is the first number we are writing
        if (hdr->entries == 0) {
            if (hdr->nbits + bit_size<uint32_t>() >= gw->capacity)
                return false;
            bit_buffer_write(data, hdr->nbits, number, bit_size<uint32_t>());

            __atomic_fetch_add(&hdr->nbits, bit_size<uint32_t>(), __ATOMIC_RELEASE);
            __atomic_fetch_add(&hdr->entries, 1, __ATOMIC_RELEASE);
            gw->prev_number = number;
            return true;
        }

        // write true/false based on whether we got the same number or not.
        if (number == gw->prev_number) {
            if (hdr->nbits + 1 >= gw->capacity)
                return false;

            bit_buffer_write(data, hdr->nbits, static_cast<uint32_t>(1), 1);
            __atomic_fetch_add(&hdr->nbits, 1, __ATOMIC_RELEASE);
            __atomic_fetch_add(&hdr->entries, 1, __ATOMIC_RELEASE);
            return true;
        }

        if (hdr->nbits + 1 >= gw->capacity)
            return false;
        bit_buffer_write(data, hdr->nbits, static_cast<uint32_t>(0), 1);
        __atomic_fetch_add(&hdr->nbits, 1, __ATOMIC_RELEASE);

        uint32_t xor_value = gw->prev_number ^ number;
        uint32_t xor_lzc = (bit_size<uint32_t>() == 32) ? __builtin_clz(xor_value) : __builtin_clzll(xor_value);
        uint32_t is_xor_lzc_same = (xor_lzc == gw->prev_xor_lzc) ? 1 : 0;

        if (hdr->nbits + 1 >= gw->capacity)
            return false;
        bit_buffer_write(data, hdr->nbits, is_xor_lzc_same, 1);
        __atomic_fetch_add(&hdr->nbits, 1, __ATOMIC_RELEASE);

        if (!is_xor_lzc_same) {
            size_t bits_needed = (bit_size<uint32_t>() == 32) ? 5 : 6;
            if ((hdr->nbits + bits_needed) >= gw->capacity)
                return false;
            bit_buffer_write(data, hdr->nbits, xor_lzc, bits_needed);
            __atomic_fetch_add(&hdr->nbits, bits_needed, __ATOMIC_RELEASE);
        }

        // write the bits of the XOR'd value without the LZC prefix
        if (hdr->nbits + (bit_size<uint32_t>() - xor_lzc) >= gw->capacity)
            return false;
        bit_buffer_write(data, hdr->nbits, xor_value, bit_size<uint32_t>() - xor_lzc);
        __atomic_fetch_add(&hdr->nbits, bit_size<uint32_t>() - xor_lzc, __ATOMIC_RELEASE);
        __atomic_fetch_add(&hdr->entries, 1, __ATOMIC_RELEASE);

        gw->prev_number = number;
        gw->prev_xor_lzc = xor_lzc;
        return true;
    }
}

gorilla_buffer_t *gorilla_writer_drop_head_buffer(gorilla_writer_t *gw) {
    if (!gw->head_buffer)
        return NULL;

    gorilla_buffer_t *curr_head = gw->head_buffer;
    gorilla_buffer_t *next_head = gw->head_buffer->header.next;
    __atomic_store_n(&gw->head_buffer, next_head, __ATOMIC_RELEASE);
    return curr_head;
}

uint32_t gorilla_writer_actual_nbytes(const gorilla_writer_t *gw)
{
    uint32_t nbytes = 0;

    const gorilla_buffer_t *curr_gbuf = __atomic_load_n(&gw->head_buffer, __ATOMIC_ACQUIRE);
    do {
        const gorilla_buffer_t *next_gbuf = __atomic_load_n(&curr_gbuf->header.next, __ATOMIC_ACQUIRE);

        nbytes += RRDENG_GORILLA_32BIT_BUFFER_SIZE;

        curr_gbuf = next_gbuf;
    } while (curr_gbuf);

    return nbytes;
}

uint32_t gorilla_writer_optimal_nbytes(const gorilla_writer_t *gw)
{
    uint32_t nbytes = 0;

    const gorilla_buffer_t *curr_gbuf = __atomic_load_n(&gw->head_buffer, __ATOMIC_ACQUIRE);
    do {
        const gorilla_buffer_t *next_gbuf = __atomic_load_n(&curr_gbuf->header.next, __ATOMIC_ACQUIRE);

        if(next_gbuf)
            nbytes += RRDENG_GORILLA_32BIT_BUFFER_SIZE;
        else
            nbytes += gorilla_buffer_nbytes(__atomic_load_n(&curr_gbuf->header.nbits, __ATOMIC_ACQUIRE));

        curr_gbuf = next_gbuf;
    } while (curr_gbuf);

    return nbytes;
}

bool gorilla_writer_serialize(const gorilla_writer_t *gw, uint8_t *dst, uint32_t dst_size) {
    const gorilla_buffer_t *curr_gbuf = gw->head_buffer;

    do {
        const gorilla_buffer_t *next_gbuf = curr_gbuf->header.next;

        size_t bytes = RRDENG_GORILLA_32BIT_BUFFER_SIZE;
        if (bytes > dst_size)
            return false;   

        memcpy(dst, curr_gbuf, bytes);
        dst += bytes;
        dst_size -= bytes;

        curr_gbuf = next_gbuf;
    } while (curr_gbuf);

    return true;
}

uint32_t gorilla_buffer_patch(gorilla_buffer_t *gbuf) {
    gorilla_buffer_t *curr_gbuf = gbuf;
    uint32_t n = curr_gbuf->header.entries;

    while (curr_gbuf->header.next) {
        uint32_t *buf = reinterpret_cast<uint32_t *>(gbuf);
        gbuf = reinterpret_cast<gorilla_buffer_t *>(&buf[RRDENG_GORILLA_32BIT_BUFFER_SLOTS]);

        assert(((uintptr_t) (gbuf) % sizeof(uintptr_t)) == 0 &&
               "Gorilla buffer not aligned to uintptr_t");

        curr_gbuf->header.next = gbuf;
        curr_gbuf = curr_gbuf->header.next;

        n += curr_gbuf->header.entries;
    }

    return n;
}

size_t gorilla_buffer_unpatched_nbuffers(const gorilla_buffer_t *gbuf) {
    size_t nbuffers = 0;
    while(gbuf) {
        nbuffers++;

        if(gbuf->header.next) {
            const auto *buf = reinterpret_cast<const uint32_t *>(gbuf);
            gbuf = reinterpret_cast<const gorilla_buffer_t *>(&buf[RRDENG_GORILLA_32BIT_BUFFER_SLOTS]);
        }
        else
            break;
    }

    return nbuffers;
}

size_t gorilla_buffer_unpatched_nbytes(const gorilla_buffer_t *gbuf) {
    size_t nbytes = sizeof(gorilla_buffer_t);
    while(gbuf) {
        if(gbuf->header.next) {
            nbytes += RRDENG_GORILLA_32BIT_BUFFER_SIZE;
            const auto *buf = reinterpret_cast<const uint32_t *>(gbuf);
            gbuf = reinterpret_cast<const gorilla_buffer_t *>(&buf[RRDENG_GORILLA_32BIT_BUFFER_SLOTS]);
        }
        else {
            nbytes += gorilla_buffer_nbytes(gbuf->header.nbits);
            break;
        }
    }

    return nbytes;
}

gorilla_reader_t gorilla_writer_get_reader(const gorilla_writer_t *gw)
{
    const gorilla_buffer_t *buffer = __atomic_load_n(&gw->head_buffer, __ATOMIC_ACQUIRE);

    uint32_t entries = __atomic_load_n(&buffer->header.entries, __ATOMIC_ACQUIRE);
    uint32_t capacity = __atomic_load_n(&buffer->header.nbits, __ATOMIC_ACQUIRE);

    return gorilla_reader_t {
        .buffer = buffer,
        .entries = entries,
        .index = 0,
        .capacity = capacity,
        .position = 0,
        .prev_number = 0,
        .prev_xor_lzc = 0,
        .prev_xor = 0,
    };
}

gorilla_reader_t gorilla_reader_init(gorilla_buffer_t *gbuf)
{
    uint32_t entries = __atomic_load_n(&gbuf->header.entries, __ATOMIC_ACQUIRE);
    uint32_t capacity = __atomic_load_n(&gbuf->header.nbits, __ATOMIC_ACQUIRE);

    return gorilla_reader_t {
        .buffer = gbuf,
        .entries = entries,
        .index = 0,
        .capacity = capacity,
        .position = 0,
        .prev_number = 0,
        .prev_xor_lzc = 0,
        .prev_xor = 0,
    };
}

extern "C" {
    ALWAYS_INLINE_ONLY bool gorilla_reader_read(gorilla_reader_t *gr, uint32_t *number)
    {
        const uint32_t *data = gr->buffer->data;

        while (gr->index + 1 > gr->entries) {
            // We don't have any more entries to return. However, the writer
            // might have updated the buffer's entries. We need to check once
            // more in case more elements were added.
            gr->entries = __atomic_load_n(&gr->buffer->header.entries, __ATOMIC_ACQUIRE);
            gr->capacity = __atomic_load_n(&gr->buffer->header.nbits, __ATOMIC_ACQUIRE);

            // if the reader's current buffer has not been updated, we need to
            // check if it has a pointer to a next buffer.
            if (gr->index + 1 > gr->entries) {
                gorilla_buffer_t *next_buffer = __atomic_load_n(&gr->buffer->header.next, __ATOMIC_ACQUIRE);

                if (!next_buffer) {
                    // fprintf(stderr, "Consumed reader with %zu entries from buffer %p\n (No more buffers to read from)", gr->length, gr->buffer);
                    return false;
                }

                // fprintf(stderr, "Consumed reader with %zu entries from buffer %p\n", gr->length, gr->buffer);
                *gr = gorilla_reader_init(next_buffer);
                data = gr->buffer->data;
            }
            else
                break;
        }

        // read the first number
        if (gr->index == 0) {
            bit_buffer_read(data, gr->position, number, bit_size<uint32_t>());

            gr->index++;
            gr->position += bit_size<uint32_t>();
            gr->prev_number = *number;
            return true;
        }

        // process same-number bit
        uint32_t is_same_number;
        bit_buffer_read(data, gr->position, &is_same_number, 1);
        gr->position++;

        if (is_same_number) {
            *number = gr->prev_number;
            gr->index++;
            return true;
        }

        // proceess same-xor-lzc bit
        uint32_t xor_lzc = gr->prev_xor_lzc;

        uint32_t same_xor_lzc;
        bit_buffer_read(data, gr->position, &same_xor_lzc, 1);
        gr->position++;

        if (!same_xor_lzc) {
            bit_buffer_read(data, gr->position, &xor_lzc, (bit_size<uint32_t>() == 32) ? 5 : 6);
            gr->position += (bit_size<uint32_t>() == 32) ? 5 : 6;
        }

        // process the non-lzc suffix
        uint32_t xor_value = 0;
        bit_buffer_read(data, gr->position, &xor_value, bit_size<uint32_t>() - xor_lzc);
        gr->position += bit_size<uint32_t>() - xor_lzc;

        *number = (gr->prev_number ^ xor_value);

        gr->index++;
        gr->prev_number = *number;
        gr->prev_xor_lzc = xor_lzc;
        gr->prev_xor = xor_value;

        return true;
    }
}

extern "C" {
struct aral;
void aral_unmark_allocation(struct aral *ar, void *ptr);
}

void gorilla_writer_aral_unmark(const gorilla_writer_t *gw, struct aral *ar)
{
    const gorilla_buffer_t *curr_gbuf = __atomic_load_n(&gw->head_buffer, __ATOMIC_ACQUIRE);
    do {
        const gorilla_buffer_t *next_gbuf = __atomic_load_n(&curr_gbuf->header.next, __ATOMIC_ACQUIRE);

        // Call the C function here
        aral_unmark_allocation(ar, const_cast<void*>(static_cast<const void*>(curr_gbuf)));

        curr_gbuf = next_gbuf;
    } while (curr_gbuf);
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

class Storage {
public:
    gorilla_buffer_t *alloc_buffer(size_t words) {
        uint32_t *new_buffer = new uint32_t[words]();
        assert(((((uintptr_t) new_buffer) % 8u) == 0) && "Unaligned buffer...");
        Buffers.push_back(new_buffer);
        return reinterpret_cast<gorilla_buffer_t *>(new_buffer);
    }

    void free_buffers() {
        for (uint32_t *buffer : Buffers) {
            delete[] buffer;
        }
    }
    
private:
    std::vector<uint32_t *> Buffers;
};

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    if (Size < 4)
        return 0;

    std::vector<uint32_t> RandomData = random_vector<uint32_t>(Data, Size);

    Storage S;
    size_t words_per_buffer = 8;

    /*
     * write data
    */
    gorilla_buffer_t *first_buffer = S.alloc_buffer(words_per_buffer);
    gorilla_writer_t gw = gorilla_writer_init(first_buffer, words_per_buffer);

    for (size_t i = 0; i != RandomData.size(); i++) {
        bool ok = gorilla_writer_write(&gw, RandomData[i]);
        if (ok)
            continue;

        // add new buffer
        gorilla_buffer_t *buffer = S.alloc_buffer(words_per_buffer);
        gorilla_writer_add_buffer(&gw, buffer, words_per_buffer);

        ok = gorilla_writer_write(&gw, RandomData[i]);
        assert(ok && "Could not write data to new buffer!!!");
    }


    /*
     * read data
    */
    gorilla_reader_t gr = gorilla_writer_get_reader(&gw);

    for (size_t i = 0; i != RandomData.size(); i++) {
        uint32_t number = 0;
        bool ok = gorilla_reader_read(&gr, &number);
        assert(ok && "Failed to read number from gorilla buffer");

        assert((number == RandomData[i])
                && "Read wrong number from gorilla buffer");
    }

    S.free_buffers();
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
        gorilla_writer_t gw = gorilla_writer_init(
            reinterpret_cast<gorilla_buffer_t *>(EncodedData.data()),
            EncodedData.size());

        for (size_t i = 0; i != RandomData.size(); i++)
            benchmark::DoNotOptimize(gorilla_writer_write(&gw, RandomData[i]));

        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint32_t));
}
BENCHMARK(BM_EncodeU32Numbers)->ThreadRange(1, 16)->UseRealTime();

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

    gorilla_writer_t gw = gorilla_writer_init(
        reinterpret_cast<gorilla_buffer_t *>(EncodedData.data()),
        EncodedData.size());

    for (size_t i = 0; i != RandomData.size(); i++)
        gorilla_writer_write(&gw, RandomData[i]);

    for (auto _ : state) {
        gorilla_reader_t gr = gorilla_reader_init(reinterpret_cast<gorilla_buffer_t *>(EncodedData.data()));

        for (size_t i = 0; i != RandomData.size(); i++) {
            uint32_t number = 0;
            benchmark::DoNotOptimize(gorilla_reader_read(&gr, &number));
        }

        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint32_t));
}
BENCHMARK(BM_DecodeU32Numbers)->ThreadRange(1, 16)->UseRealTime();

#endif /* ENABLE_BENCHMARK */
