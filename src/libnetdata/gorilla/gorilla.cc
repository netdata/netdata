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

static constexpr size_t gorilla_buffer_data_bits(size_t n) {
    return (n * bit_size<uint32_t>()) - (sizeof(gorilla_header_t) * CHAR_BIT);
}

static bool gorilla_disk_buffer_has_valid_nbits(const gorilla_buffer_t *gbuf) {
    return gbuf->header.nbits < gorilla_buffer_data_bits(RRDENG_GORILLA_32BIT_BUFFER_SLOTS);
}

static uint32_t gorilla_data_word_load(const uint32_t *word)
{
    return __atomic_load_n(word, __ATOMIC_RELAXED);
}

static void gorilla_data_word_store(uint32_t *word, uint32_t value)
{
    __atomic_store_n(word, value, __ATOMIC_RELAXED);
}

// Writes the low `nbits` bits of `v` (up to 39 bits, no bits set above them) at bit
// position `pos`, LSB-first. Bits at and above `pos` are zero in the target words:
// every word is first entered by a plain store of a value with zeros above it.
// Only the writer thread stores data words, so no read-modify-write is needed.
static void bit_buffer_write(uint32_t *buf, uint32_t pos, uint64_t v, uint32_t nbits)
{
    assert(nbits > 0 && nbits <= 39);

    const uint32_t index = pos / bit_size<uint32_t>();
    const uint32_t offset = pos % bit_size<uint32_t>();

    uint32_t word = (uint32_t) (v << offset);
    if (offset)
        word |= gorilla_data_word_load(&buf[index]);
    gorilla_data_word_store(&buf[index], word);

    const uint32_t written = bit_size<uint32_t>() - offset;
    if (nbits > written) {
        v >>= written;
        gorilla_data_word_store(&buf[index + 1], (uint32_t) v);

        if (nbits > written + bit_size<uint32_t>())
            gorilla_data_word_store(&buf[index + 2], (uint32_t) (v >> bit_size<uint32_t>()));
    }
}

static void bit_buffer_read(const uint32_t *buf, size_t pos, uint32_t *v, size_t nbits)
{
    assert(nbits > 0 && nbits <= bit_size<uint32_t>());

    const size_t index = pos / bit_size<uint32_t>();
    const size_t offset = pos % bit_size<uint32_t>();

    if (offset == 0) {
        uint32_t word = gorilla_data_word_load(&buf[index]);
        *v = (nbits == bit_size<uint32_t>()) ?
                    word :
                    word & (((uint32_t) 1 << nbits) - 1);
    } else {
        const size_t remaining_bits = bit_size<uint32_t>() - offset;

        // extract the lower part of the value
        uint32_t word = gorilla_data_word_load(&buf[index]);
        if (nbits < remaining_bits) {
            *v = (word >> offset) & (((uint32_t) 1 << nbits) - 1);
        } else {
            *v = (word >> offset) & (((uint32_t) 1 << remaining_bits) - 1);
            nbits -= remaining_bits;
            word = gorilla_data_word_load(&buf[index + 1]);
            *v |= (word & (((uint32_t) 1 << nbits) - 1)) << remaining_bits;
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
    // readers load these atomically; the release store of the previous
    // buffer's next pointer below publishes them
    __atomic_store_n(&gbuf->header.next, (gorilla_buffer_t *) NULL, __ATOMIC_RELAXED);
    __atomic_store_n(&gbuf->header.entries, 0, __ATOMIC_RELAXED);
    __atomic_store_n(&gbuf->header.nbits, 0, __ATOMIC_RELAXED);

    uint32_t capacity = gorilla_buffer_data_bits(n);

    gw->prev_number = 0;
    gw->prev_xor_lzc = 0;
    gw->capacity = capacity;

    if (gw->last_buffer)
        __atomic_store_n(&gw->last_buffer->header.next, gbuf, __ATOMIC_RELEASE);

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

        // Single writer: only this thread modifies nbits, entries and the data words.
        // Readers acquire-load entries, then nbits, so publishing nbits before
        // entries with plain release stores is enough; no locked RMW is needed.
        const uint32_t nbits = __atomic_load_n(&hdr->nbits, __ATOMIC_RELAXED);
        const uint32_t entries = __atomic_load_n(&hdr->entries, __ATOMIC_RELAXED);

        uint32_t xor_lzc = gw->prev_xor_lzc;
        uint64_t bits;
        uint32_t bits_count;

        if (entries == 0) {
            // the first number is stored as is
            bits = number;
            bits_count = bit_size<uint32_t>();
        }
        else if (number == gw->prev_number) {
            // a single 1 bit: same number
            bits = 1;
            bits_count = 1;
        }
        else {
            // a 0 bit (different number), then a 1 bit if the XOR's leading zero
            // count is the previous one, or a 0 bit and the new count in 5 bits,
            // then the XOR'd value without its leading zeros
            const uint32_t xor_value = gw->prev_number ^ number;
            xor_lzc = __builtin_clz(xor_value);

            if (xor_lzc == gw->prev_xor_lzc) {
                bits = 0x2 | ((uint64_t) xor_value << 2);
                bits_count = 2 + (bit_size<uint32_t>() - xor_lzc);
            }
            else {
                bits = ((uint64_t) xor_lzc << 2) | ((uint64_t) xor_value << 7);
                bits_count = 7 + (bit_size<uint32_t>() - xor_lzc);
            }
        }

        if (nbits + bits_count >= gw->capacity)
            return false;

        bit_buffer_write(data, nbits, bits, bits_count);

        __atomic_store_n(&hdr->nbits, nbits + bits_count, __ATOMIC_RELEASE);
        __atomic_store_n(&hdr->entries, entries + 1, __ATOMIC_RELEASE);

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

bool gorilla_buffer_patch(gorilla_buffer_t *gbuf, size_t nbuffers, uint32_t *entries) {
    gorilla_buffer_t *curr_gbuf = gbuf;
    uint32_t n = curr_gbuf->header.entries;
    size_t buffers = 1;

    if(unlikely(!gorilla_disk_buffer_has_valid_nbits(curr_gbuf)))
        return false;

    while (curr_gbuf->header.next) {
        if(unlikely(buffers == nbuffers))
            return false;

        auto *buf = static_cast<unsigned char *>(static_cast<void *>(curr_gbuf));
        auto *next_gbuf = static_cast<gorilla_buffer_t *>(static_cast<void *>(buf + RRDENG_GORILLA_32BIT_BUFFER_SIZE));

        assert(((uintptr_t) (next_gbuf) % sizeof(uintptr_t)) == 0 &&
               "Gorilla buffer not aligned to uintptr_t");

        curr_gbuf->header.next = next_gbuf;
        curr_gbuf = curr_gbuf->header.next;
        buffers++;

        if(unlikely(!gorilla_disk_buffer_has_valid_nbits(curr_gbuf)))
            return false;

        n += curr_gbuf->header.entries;
    }

    *entries = n;
    return true;
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

static bool gorilla_reader_read_bits(gorilla_reader_t *gr, const uint32_t *data, uint32_t *number, size_t nbits)
{
    if (unlikely(gr->position > gr->capacity || nbits > gr->capacity - gr->position))
        return false;

    bit_buffer_read(data, gr->position, number, nbits);
    gr->position += nbits;
    return true;
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

                if (!next_buffer)
                    return false;

                // The writer appends its last values to this buffer before it
                // links the next one, and they may have landed after the loads
                // above: read this buffer's entries again before leaving it.
                gr->entries = __atomic_load_n(&gr->buffer->header.entries, __ATOMIC_ACQUIRE);
                gr->capacity = __atomic_load_n(&gr->buffer->header.nbits, __ATOMIC_ACQUIRE);
                if (gr->index + 1 <= gr->entries)
                    break;

                *gr = gorilla_reader_init(next_buffer);
                data = gr->buffer->data;
            }
            else
                break;
        }

        // read the first number
        if (gr->index == 0) {
            if (unlikely(!gorilla_reader_read_bits(gr, data, number, bit_size<uint32_t>())))
                return false;
            gr->index++;
            gr->prev_number = *number;
            return true;
        }

        // process same-number bit
        uint32_t is_same_number;
        if (unlikely(!gorilla_reader_read_bits(gr, data, &is_same_number, 1)))
            return false;

        if (is_same_number) {
            *number = gr->prev_number;
            gr->index++;
            return true;
        }

        // proceess same-xor-lzc bit
        uint32_t xor_lzc = gr->prev_xor_lzc;

        uint32_t same_xor_lzc;
        if (unlikely(!gorilla_reader_read_bits(gr, data, &same_xor_lzc, 1)))
            return false;

        if (!same_xor_lzc) {
            size_t xor_lzc_bits = (bit_size<uint32_t>() == 32) ? 5 : 6;
            if (unlikely(!gorilla_reader_read_bits(gr, data, &xor_lzc, xor_lzc_bits)))
                return false;
        }

        // process the non-lzc suffix
        uint32_t xor_value = 0;
        if (unlikely(!gorilla_reader_read_bits(gr, data, &xor_value, bit_size<uint32_t>() - xor_lzc)))
            return false;

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
    while (curr_gbuf) {
        const gorilla_buffer_t *next_gbuf = __atomic_load_n(&curr_gbuf->header.next, __ATOMIC_ACQUIRE);

        aral_unmark_allocation(ar, const_cast<void*>(static_cast<const void*>(curr_gbuf)));

        curr_gbuf = next_gbuf;
    }
}

/*
 * Unit test (netdata -W unittest)
 */

#include <atomic>
#include <thread>
#include <vector>

// one gorilla buffer, 8-byte aligned for the header's next pointer
struct gorilla_test_buffer {
    uint64_t words[RRDENG_GORILLA_32BIT_BUFFER_SIZE / sizeof(uint64_t)];
};

static uint32_t gorilla_test_value(uint32_t i, uint32_t prev, uint32_t *state)
{
    *state = *state * 1103515245u + 12345u;
    uint32_t r = *state;

    switch (i % 5) {
        case 0:  return prev;                                   // same number
        case 1:  return prev ^ (1u << (r >> 27));               // single bit, any leading zero count
        case 2:  return prev ^ (r | 0x80000000u);               // no leading zeros: 39-bit encodings
        case 3:  return prev ^ ((r >> (r & 31)) | 1);           // random width
        default: return r;
    }
}

// The bit stream of gorilla_test_value(0..99) starting from 0x12345678, as written
// by the original writer. It pins the encoding: pages on disk depend on it.
static const uint32_t gorilla_test_golden_words[] = {
    0x12345678, 0x02000034, 0x4f03c928, 0xb76427e9, 0x004333e9, 0x90100000,
    0x89cfbae3, 0x6a4d709f, 0x02064e24, 0x46f84514, 0x218015d9, 0x019d16bb,
    0x01000000, 0xd2bfa1d7, 0x118cd690, 0x8cc33c27, 0x0000d34a, 0x7cf98008,
    0x8f2d6680, 0x8c1e0650, 0x0805cfe9, 0x9ac82b28, 0xecc24f65, 0x023eed92,
    0x04000000, 0xbba92018, 0x595aba83, 0xddd74401, 0xc040c9cb, 0xab615c18,
    0x99a13461, 0xf3bf16be, 0x18020306, 0xc1faaa6d, 0x133f0081, 0x0010c7dd,
    0x44040000, 0x62d16fd9, 0x3e0c071f, 0x01e719ba, 0x41080400, 0x3b32a330,
    0xb5cf0138, 0x00079ada, 0xf06af010, 0x901f8fa5, 0x9e2dd697, 0xa0040008,
    0x1324fd9a, 0x8f3615c2, 0x074dddaa, 0x00000083, 0x4d4cfd01, 0x07a1d09e,
    0x97677a6c, 0x3e010327, 0xfcadda17, 0xd310e800, 0x00001ccd, 0xc85d8080,
    0x13786c94, 0xddc316d6, 0x08000061, 0x5f0d5c20, 0x03ae0e05, 0x7d0ecd28,
    0x00000013, 0xcab48080, 0x58dc4258, 0xc39384d3, 0x000868cb, 0xf4020000,
    0x71f01371, 0xfc23bb83, 0x0cd0b9a5, 0x08000000, 0xb9dfd438, 0x701596a5,
    0x0abf1ab3,
};

static int gorilla_unittest_golden(void)
{
    gorilla_test_buffer mem = {};
    auto *gbuf = reinterpret_cast<gorilla_buffer_t *>(&mem);
    gorilla_writer_t gw = gorilla_writer_init(gbuf, RRDENG_GORILLA_32BIT_BUFFER_SLOTS);

    std::vector<uint32_t> values;
    uint32_t state = 1, v = 0x12345678;
    for (uint32_t i = 0; i < 100; i++) {
        v = gorilla_test_value(i, v, &state);
        values.push_back(v);
        if (!gorilla_writer_write(&gw, v)) {
            fprintf(stderr, "gorilla golden: write %u failed\n", i);
            return 1;
        }
    }

    const size_t nwords = sizeof(gorilla_test_golden_words) / sizeof(gorilla_test_golden_words[0]);
    if (gbuf->header.entries != 100 || gbuf->header.nbits != 2524) {
        fprintf(stderr, "gorilla golden: entries %u nbits %u, expected 100 and 2524\n",
                gbuf->header.entries, gbuf->header.nbits);
        return 1;
    }
    for (size_t i = 0; i < nwords; i++) {
        if (gbuf->data[i] != gorilla_test_golden_words[i]) {
            fprintf(stderr, "gorilla golden: word %zu is 0x%08x, expected 0x%08x\n",
                    i, gbuf->data[i], gorilla_test_golden_words[i]);
            return 1;
        }
    }

    gorilla_reader_t gr = gorilla_reader_init(gbuf);
    for (size_t i = 0; i < values.size(); i++) {
        uint32_t n = 0;
        if (!gorilla_reader_read(&gr, &n) || n != values[i]) {
            fprintf(stderr, "gorilla golden: read %zu failed\n", i);
            return 1;
        }
    }

    return 0;
}

// Small and full-size buffers, so values land at every bit offset and writes
// fail at every distance from the end of a buffer.
static int gorilla_unittest_roundtrip(void)
{
    uint32_t seed = 0x5eed;

    for (uint32_t iteration = 0; iteration < 2000; iteration++) {
        seed = seed * 1103515245u + 12345u;
        const size_t nvalues = 1 + (seed >> 8) % 2500;
        const size_t words = (iteration % 2) ? RRDENG_GORILLA_32BIT_BUFFER_SLOTS : 6 + (seed >> 20) % 26;

        std::vector<std::vector<uint64_t>> storage;
        storage.reserve(nvalues + 1);
        auto new_buffer = [&]() {
            storage.emplace_back((words + 1) / 2, 0);
            return reinterpret_cast<gorilla_buffer_t *>(storage.back().data());
        };

        std::vector<uint32_t> values(nvalues);
        uint32_t state = seed, v = seed;
        for (size_t i = 0; i < nvalues; i++)
            values[i] = v = gorilla_test_value((uint32_t)(i + iteration), v, &state);

        gorilla_writer_t gw = gorilla_writer_init(new_buffer(), words);
        for (size_t i = 0; i < nvalues; i++) {
            const gorilla_header_t before = gw.last_buffer->header;

            if (gorilla_writer_write(&gw, values[i]))
                continue;

            // a failed write leaves the buffer as it was
            if (gw.last_buffer->header.nbits != before.nbits || gw.last_buffer->header.entries != before.entries) {
                fprintf(stderr, "gorilla roundtrip: failed write changed the buffer header\n");
                return 1;
            }

            gorilla_writer_add_buffer(&gw, new_buffer(), words);
            if (!gorilla_writer_write(&gw, values[i])) {
                fprintf(stderr, "gorilla roundtrip: write to a new buffer failed\n");
                return 1;
            }
        }

        gorilla_reader_t gr = gorilla_writer_get_reader(&gw);
        for (size_t i = 0; i < nvalues; i++) {
            uint32_t n = 0;
            if (!gorilla_reader_read(&gr, &n) || n != values[i]) {
                fprintf(stderr, "gorilla roundtrip: iteration %u, value %zu of %zu read wrong\n", iteration, i, nvalues);
                return 1;
            }
        }

        uint32_t n = 0;
        if (gorilla_reader_read(&gr, &n)) {
            fprintf(stderr, "gorilla roundtrip: read past the last value\n");
            return 1;
        }
    }

    return 0;
}

// Readers decode a page while the writer appends to it, as queries do on hot pages.
static int gorilla_unittest_concurrent_readers(void)
{
    const size_t nvalues = 200000;

    std::vector<uint32_t> values(nvalues);
    uint32_t state = 7, v = 7;
    for (size_t i = 0; i < nvalues; i++)
        values[i] = v = gorilla_test_value((uint32_t)i, v, &state);

    // worst case is 39 bits per value
    std::vector<gorilla_test_buffer> storage(nvalues * 39 / (RRDENG_GORILLA_32BIT_BUFFER_SIZE * CHAR_BIT / 2) + 2);
    size_t used = 0;

    gorilla_writer_t gw = gorilla_writer_init(reinterpret_cast<gorilla_buffer_t *>(&storage[used++]), RRDENG_GORILLA_32BIT_BUFFER_SLOTS);

    std::atomic<bool> done(false);
    std::atomic<int> errors(0);
    std::atomic<int> started(0);

    auto reader = [&]() {
        started++;
        bool last = false;
        while (!last) {
            last = done.load(std::memory_order_acquire);

            gorilla_reader_t gr = gorilla_writer_get_reader(&gw);
            size_t i = 0;
            uint32_t n;
            while (i < nvalues && gorilla_reader_read(&gr, &n)) {
                if (n != values[i]) {
                    errors++;
                    return;
                }
                i++;
            }

            if (last && i != nvalues)
                errors++;
        }
    };

    std::thread r1(reader), r2(reader);

    // make the readers overlap the writer
    while (started.load() != 2)
        std::this_thread::yield();

    for (size_t i = 0; i < nvalues; i++) {
        if (gorilla_writer_write(&gw, values[i]))
            continue;

        if (used == storage.size()) {
            errors++;
            break;
        }
        gorilla_writer_add_buffer(&gw, reinterpret_cast<gorilla_buffer_t *>(&storage[used++]), RRDENG_GORILLA_32BIT_BUFFER_SLOTS);
        if (!gorilla_writer_write(&gw, values[i])) {
            errors++;
            break;
        }
    }
    done.store(true, std::memory_order_release);

    r1.join();
    r2.join();

    if (errors)
        fprintf(stderr, "gorilla concurrent readers: %d errors\n", errors.load());

    return errors ? 1 : 0;
}

extern "C" int gorilla_unittest(void)
{
    fprintf(stderr, "\nrunning gorilla unittest\n");

    int errors = 0;
    errors += gorilla_unittest_golden();
    errors += gorilla_unittest_roundtrip();
    errors += gorilla_unittest_concurrent_readers();

    if (errors)
        fprintf(stderr, "gorilla unittest: %d ERROR(S)\n", errors);
    else
        fprintf(stderr, "gorilla unittest: OK\n");

    return errors;
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

static void fuzz_disk_buffer_patch(const uint8_t *data, size_t size) {
    if (size == 0)
        return;

    const size_t buffer_size = RRDENG_GORILLA_32BIT_BUFFER_SIZE;
    if (size > SIZE_MAX - (buffer_size - 1))
        return;

    const size_t disk_size = ((size + buffer_size - 1) / buffer_size) * buffer_size;
    const size_t nbuffers = disk_size / buffer_size;
    const size_t words = disk_size / sizeof(uint32_t);

    Storage S;
    gorilla_buffer_t *disk_buffer = S.alloc_buffer(words);
    memcpy(disk_buffer, data, size);

    uint32_t entries = 0;
    if (gorilla_buffer_patch(disk_buffer, nbuffers, &entries)) {
        gorilla_reader_t gr = gorilla_reader_init(disk_buffer);

        // entries is fuzzer-controlled, so keep this path bounded.
        constexpr uint32_t max_reads = 16384;
        if (entries > max_reads)
            entries = max_reads;

        for (uint32_t i = 0; i != entries; i++) {
            uint32_t number = 0;
            if (!gorilla_reader_read(&gr, &number))
                break;
        }
    }

    S.free_buffers();
}

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    fuzz_disk_buffer_patch(Data, Size);

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
