// SPDX-License-Identifier: GPL-3.0-or-later

#include "gorilla.h"

#include <cassert>
#include <climits>
#include <cstdio>
#include <cstring>

#include <forward_list>

using std::size_t;

template <typename T>
static constexpr size_t bit_size() noexcept
{
    static_assert((sizeof(T) * CHAR_BIT) == 32 || (sizeof(T) * CHAR_BIT) == 64,
                  "Word size should be 32 or 64 bits.");
    return (sizeof(T) * CHAR_BIT);
}

template<typename Word>
class BitBuffer {
public:
    void init(size_t capacity) {
        cap = capacity;
    }

    bool write(Word *buf, size_t pos, Word v, size_t nbits)
    {
        assert(nbits > 0 && nbits <= bit_size<Word>());

        if ((pos + nbits) > cap)
            return false;

        const size_t index = pos / bit_size<Word>();
        const size_t offset = pos % bit_size<Word>();

        pos += nbits;

        if (offset == 0) {
            buf[index] = v;
        } else {
            const size_t remaining_bits = bit_size<Word>() - offset;

            // write the lower part of the value
            const Word low_bits_mask = ((Word) 1 << remaining_bits) - 1;
            const Word lowest_bits_in_value = v & low_bits_mask;
            buf[index] |= (lowest_bits_in_value << offset);

            if (nbits > remaining_bits) {
                // write the upper part of the value
                const Word high_bits_mask = ~low_bits_mask;
                const Word highest_bits_in_value = (v & high_bits_mask) >> (remaining_bits);
                buf[index + 1] = highest_bits_in_value;
            }
        }

        return true;
    }

    bool read(const Word *buf, size_t pos, Word *v, size_t nbits) const
    {
        assert(nbits > 0 && nbits <= bit_size<Word>());

        if (pos + nbits > cap)
            return false;

        const size_t index = pos / bit_size<Word>();
        const size_t offset = pos % bit_size<Word>();

        pos += nbits;

        if (offset == 0) {
            *v = (nbits == bit_size<Word>()) ?
                        buf[index] :
                        buf[index] & (((Word) 1 << nbits) - 1);
        } else {
            const size_t remaining_bits = bit_size<Word>() - offset;

            // extract the lower part of the value
            if (nbits < remaining_bits) {
                *v = (buf[index] >> offset) & (((Word) 1 << nbits) - 1);
            } else {
                *v = (buf[index] >> offset) & (((Word) 1 << remaining_bits) - 1);
                nbits -= remaining_bits;
                *v |= (buf[index + 1] & (((Word) 1 << nbits) - 1)) << remaining_bits;
            }
        }

        return true;
    }

    size_t capacity() const {
        return cap;
    }

private:
    size_t cap;
};

template<typename Word>
class BitStreamWriter {
public:
    void init(size_t capacity) {
        bb.init(capacity);
        pos = bit_size<Word>();
        assert(pos <= capacity);
    }

    inline bool write(Word *buf, Word value, size_t nbits)
    {
        bool ok = bb.write(buf, pos, value, nbits);

        if (ok)
            pos += nbits;

        return ok;
    }

    inline void flush(Word *buf)
    {
        __atomic_store_n(&buf[0], pos, __ATOMIC_RELAXED);
    }

    inline size_t capacity() const
    {
        return bb.capacity();
    }

    inline size_t position() const
    {
        return pos;
    }

private:
    BitBuffer<Word> bb;
    size_t pos;
};

template<typename Word>
class BitStreamReader {
public:
    void init(const Word *buffer) {
        size_t capacity = __atomic_load_n(&buffer[0], __ATOMIC_SEQ_CST);

        bb.init(capacity);
        pos = bit_size<Word>();
    }

    inline bool read(const Word *buf, Word *value, size_t nbits)
    {
        bool ok = bb.read(buf, pos, value, nbits);

        if (ok)
            pos += nbits;

        return ok;
    }

    inline size_t capacity() const
    {
        return bb.capacity();
    }

    inline size_t position() const
    {
        return pos;
    }

private:
    BitBuffer<Word> bb;
    size_t pos;
};

template<typename Word>
class GorillaWriter
{
public:
    void init(Word *buf, size_t n) {
        buffer = buf;
        entries = 0;
        bs.init((n - 1) * bit_size<Word>());
        prev_number = 0;
        prev_xor_lzc = 0;
    }

    bool write(Word number)
    {
        // this is the first number we are writing
        if (entries == 0) {
            bool ok = bs.write(bit_buffer(), number, bit_size<Word>());

            if (ok) {
                entries++;
                prev_number = number;
            }

            return ok;
        }
    
        // write true/false based on whether we got the same number or not.
        if (number == prev_number) {
            bool ok = bs.write(bit_buffer(), static_cast<Word>(1), 1);
            if (ok)
                entries++;
            return ok;
        } else {
            bool ok = bs.write(bit_buffer(),static_cast<Word>(0), 1);
            if (!ok)
                return false;
        }

        Word xor_value = prev_number ^ number;
        Word xor_lzc = (bit_size<Word>() == 32) ? __builtin_clz(xor_value) : __builtin_clzll(xor_value);
        Word is_xor_lzc_same = (xor_lzc == prev_xor_lzc) ? 1 : 0;

        if (!bs.write(bit_buffer(), is_xor_lzc_same, 1))
            return false;
        
        if (!is_xor_lzc_same) {
            if (!bs.write(bit_buffer(), xor_lzc, (bit_size<Word>() == 32) ? 5 : 6))
                return false;
        }

        // write the bits of the XOR'd value without the LZC prefix
        if (!bs.write(bit_buffer(), xor_value, bit_size<Word>() - xor_lzc))
            return false;

        entries++;
        prev_number = number;
        prev_xor_lzc = xor_lzc;
        return true;
    }

    inline void flush()
    {
        bs.flush(bit_buffer());
        __atomic_store_n(&buffer[0], entries, __ATOMIC_RELAXED);
    }

    Word *data() const
    {
        return buffer;
    }

    inline void capacity() const
    {
        return bs.capacity() / bit_size<Word>();
    }

    inline void size() const
    {
        return (bs.position() + (bit_size<Word>() - 1)) / bit_size<Word>();
    }

private:
    inline Word *bit_buffer() const
    {
        return &buffer[1];
    }

private:
    Word *buffer;
    Word entries;

    BitStreamWriter<Word> bs;

    Word prev_number;
    Word prev_xor_lzc;
};

template<typename Word>
class GorillaReader {
public: 
    void init(const Word *buf) {
        buffer = buf;
        bs.init(bit_buffer());
        position = 0;

        prev_number = 0;
        prev_xor_lzc = 0;
        prev_xor = 0;
    }

    bool read(Word *number) {
        // read the first number
        if (position == 0) {
            bool ok = bs.read(bit_buffer(), number, bit_size<Word>());

            if (ok) {
                position++;
                prev_number = *number;
            }

            return ok;
        }

        // process same-number bit
        Word is_same_number;
        if (!bs.read(bit_buffer(), &is_same_number, 1)) {
            return false;
        }

        if (is_same_number) {
            *number = prev_number;
            return true;
        }

        // proceess same-xor-lzc bit
        Word xor_lzc = prev_xor_lzc;

        Word same_xor_lzc;
        if (!bs.read(bit_buffer(), &same_xor_lzc, 1)) {
            return false;
        }

        if (!same_xor_lzc) {
            if (!bs.read(bit_buffer(), &xor_lzc, (bit_size<Word>() == 32) ? 5 : 6)) {
                return false;        
            }
        }

        // process the non-lzc suffix
        Word xor_value = 0;
        if (!bs.read(bit_buffer(), &xor_value, bit_size<Word>() - xor_lzc)) {
            return false;        
        }

        *number = (prev_number ^ xor_value);

        position++;
        prev_number = *number;
        prev_xor_lzc = xor_lzc;
        prev_xor = xor_value;

        return true;
    }

    size_t entries() const {
        return __atomic_load_n(&buffer[0], __ATOMIC_SEQ_CST);
    }

    Word *data() {
        return buffer;
    }

private:
    inline const Word *bit_buffer() const
    {
        return &buffer[1];
    }

private:
    const Word *buffer;
    BitStreamReader<Word> bs;

    size_t position;

    Word prev_number;
    Word prev_xor_lzc;
    Word prev_xor;
};

template<typename Word>
class GorillaPageWriter {
public:
    void init() {}

    void add_buffer() {
        int length = std::distance(buffers.begin(), buffers.end());
        if (length) {
            gw.flush();
            buffers.insert_after(buffers.cend(), gw.data());
        }

        size_t n = 256;
        Word *buffer = new Word[n];
        gw.init(buffer, n);
    }

    bool write(Word value) {
        if (gw.write(value))
            return true;

        add_buffer();
        return gw.write(value);
    }

private:
    GorillaWriter<Word> gw;
    std::forward_list<Word *> buffers;
};

/*
 * C API
*/

gpw_t *gpw_new() {
    GorillaPageWriter<uint32_t> *gpw = new GorillaPageWriter<uint32_t>();
    gpw->init();
    return reinterpret_cast<gpw_t *>(gpw);
}

void gpw_free(gpw_t *ptr) {
    GorillaPageWriter<uint32_t> *gpw = reinterpret_cast<GorillaPageWriter<uint32_t> *>(ptr);
    delete gpw;
}

void gpw_add_buffer(gpw_t *ptr) {
    GorillaPageWriter<uint32_t> *gpw = reinterpret_cast<GorillaPageWriter<uint32_t> *>(ptr);
    fprintf(stderr, "\nGVD: adding new gorilla buffer\n");
    gpw->add_buffer();
}

bool gpw_add(gpw_t *ptr, uint32_t value) {
    GorillaPageWriter<uint32_t> *gpw = reinterpret_cast<GorillaPageWriter<uint32_t> *>(ptr);
    return gpw->write(value);
}

// gorilla_writer_t *gorilla_writer_new(uint32_t *buffer, size_t n)
// {
//     GorillaWriter<uint32_t> *GW = new GorillaWriter<uint32_t>();
//     GW->init(buffer, n);
//     return reinterpret_cast<gorilla_writer_t *>(GW);
// }

// void gorilla_writer_free(gorilla_writer_t *writer) {
//     GorillaWriter<uint32_t> *GW = reinterpret_cast<GorillaWriter<uint32_t> *>(writer);
//     delete GW;
// }

// bool gorilla_writer_add(gorilla_writer *writer, uint32_t number) {
//     GorillaWriter<uint32_t> *GW = reinterpret_cast<GorillaWriter<uint32_t> *>(writer);
//     return GW->write(number);
// }

// void gorilla_writer_flush(gorilla_writer_t *writer) {
//     GorillaWriter<uint32_t> *GW = reinterpret_cast<GorillaWriter<uint32_t> *>(writer);
//     GW->flush();
// }

// gorilla_reader_t *gorilla_reader_alloc(const uint32_t *buffer)
// {
//     GorillaReader<uint32_t> *GW = new GorillaReader<uint32_t>();
//     GW->init(buffer);
//     return reinterpret_cast<gorilla_reader_t *>(GW);
// }

// void gorilla_reader_free(gorilla_reader_t *reader) {
//     GorillaReader<uint32_t> *GR = reinterpret_cast<GorillaReader<uint32_t> *>(reader);
//     delete GR;
// }

// size_t gorilla_reader_entries(gorilla_reader_t *reader) {
//     GorillaReader<uint32_t> *GR = reinterpret_cast<GorillaReader<uint32_t> *>(reader);
//     return GR->entries();
// }

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

extern "C" int LLVMFuzzerTestOneInput(const uint8_t *Data, size_t Size) {
    // 32-bit tests
    {
        if (Size < 4)
            return 0;

        std::vector<uint32_t> RandomData = random_vector<uint32_t>(Data, Size);
        std::vector<uint32_t> EncodedData(10 * RandomData.capacity(), 0);

        // write data
        {
            GorillaWriter<uint32_t> GW;
            GW.init(EncodedData.data(), EncodedData.capacity());
            for (size_t i = 0; i != RandomData.size(); i++)
                GW.write(RandomData[i]);

            GW.flush();
        }

        // read data
        {
            GorillaReader<uint32_t> GR;
            GR.init(EncodedData.data());

            assert((GR.entries() == RandomData.size()) &&
                   "Bad number of entries in gorilla buffer");

            for (size_t i = 0; i != RandomData.size(); i++) {
                uint32_t number = 0;
                bool ok = GR.read(&number);
                assert(ok && "Failed to read number from gorilla buffer");

                assert((number == RandomData[i])
                        && "Read wrong number from gorilla buffer");
            }
        }
    }

    // 64-bit tests
    {
        if (Size < 8)
            return 0;

        std::vector<uint64_t> RandomData = random_vector<uint64_t>(Data, Size);
        std::vector<uint64_t> EncodedData(10 * RandomData.capacity(), 0);

        // write data
        {
            GorillaWriter<uint64_t> GW;
            GW.init(EncodedData.data(), EncodedData.capacity());
            for (size_t i = 0; i != RandomData.size(); i++)
                GW.write(RandomData[i]);

            GW.flush();
        }

        // read data
        {
            GorillaReader<uint64_t> GR;
            GR.init(EncodedData.data());

            assert((GR.entries() == RandomData.size()) &&
                   "Bad number of entries in gorilla buffer");

            for (size_t i = 0; i != RandomData.size(); i++) {
                uint64_t number = 0;
                bool ok = GR.read(&number);
                assert(ok && "Failed to read number from gorilla buffer");

                assert((number == RandomData[i])
                        && "Read wrong number from gorilla buffer");
            }
        }
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
        GorillaWriter<uint32_t> GW;
        GW.init(EncodedData.data(), EncodedData.size());

        for (size_t i = 0; i != RandomData.size(); i++)
            benchmark::DoNotOptimize(GW.write(RandomData[i]));

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

    GorillaWriter<uint32_t> GW;
    GW.init(EncodedData.data(), EncodedData.size());
    for (size_t i = 0; i != RandomData.size(); i++)
        GW.write(RandomData[i]);
    GW.flush();

    for (auto _ : state) {
        GorillaReader<uint32_t> GR;
        GR.init(EncodedData.data());

        for (size_t i = 0; i != RandomData.size(); i++) {
            uint32_t number = 0;
            benchmark::DoNotOptimize(GR.read(&number));
        }

        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint32_t));
}
BENCHMARK(BM_DecodeU32Numbers);

static void BM_EncodeU64Numbers(benchmark::State& state) {
    std::random_device rd;
    std::mt19937 mt(rd());
    std::uniform_int_distribution<uint64_t> dist(0x0, 0x0000FFFF);

    std::vector<uint64_t> RandomData;
    for (size_t idx = 0; idx != NumItems; idx++) {
        RandomData.push_back(dist(mt));
    }
    std::vector<uint64_t> EncodedData(10 * RandomData.capacity(), 0);

    for (auto _ : state) {
        GorillaWriter<uint64_t> GW;
        GW.init(EncodedData.data(), EncodedData.size());

        for (size_t i = 0; i != RandomData.size(); i++)
            benchmark::DoNotOptimize(GW.write(RandomData[i]));

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
    for (size_t idx = 0; idx != NumItems; idx++) {
        RandomData.push_back(dist(mt));
    }
    std::vector<uint64_t> EncodedData(10 * RandomData.capacity(), 0);
    std::vector<uint64_t> DecodedData(10 * RandomData.capacity(), 0);

    GorillaWriter<uint64_t> GW;
    GW.init(EncodedData.data(), EncodedData.size());
    for (size_t i = 0; i != RandomData.size(); i++)
        GW.write(RandomData[i]);
    GW.flush();

    for (auto _ : state) {
        GorillaReader<uint64_t> GR;
        GR.init(EncodedData.data());

        for (size_t i = 0; i != RandomData.size(); i++) {
            uint64_t number = 0;
            benchmark::DoNotOptimize(GR.read(&number));
        }

        benchmark::ClobberMemory();
    }

    state.SetItemsProcessed(NumItems * state.iterations());
    state.SetBytesProcessed(NumItems * state.iterations() * sizeof(uint64_t));
}
BENCHMARK(BM_DecodeU64Numbers);

#endif /* ENABLE_BENCHMARK */
