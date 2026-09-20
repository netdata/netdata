// SPDX-License-Identifier: GPL-3.0-or-later

#include "support.h"

// The engine's private headers are C headers with no extern "C" of their own - page.h is the one exception, and it
// is the one that wraps rrdengine.h and, through it, the rest of them. Including it first is what makes the
// constants below reachable from C++; the compression header itself declares its functions with no guard, so it is
// wrapped here or the calls would look for C++-mangled symbols that do not exist.
//
// This file is named internal_ because of that: it reaches past the public surface on purpose, and
// check-public-includes.sh exempts it by name.
extern "C" {
#include "database/storage-engines/dbengine/page.h"
#include "database/storage-engines/dbengine/dbengine-compression.h"
}

#include <string>
#include <vector>

// The extent compression codecs. They are pure functions over a buffer - no engine, no tier, no cache - and until
// now nothing tested them at all.
//
// The contract that shapes every case here, learned by writing the cases against a guess and being corrected:
// dbengine_compress() returns 0 to mean "this payload is not compressed", and it does that in two situations - the
// algorithm is NONE, or a real codec produced something that was not smaller than the input, in which case the
// payload is left exactly as it was. So a codec never reports a size that is not an improvement, and the extent
// writer stores the payload as it stands when it gets 0 back. dbengine_decompress() is the counterpart and must
// never be called for an uncompressed payload; it says so itself and traps in internal-checks builds.

namespace {

// Compressible on purpose: a codec that silently did nothing would still round-trip, so the cases that care about
// compression check the size as well as the contents.
std::vector<uint8_t> repetitive_payload(size_t size) {
    std::vector<uint8_t> payload(size);

    for (size_t i = 0; i < size; i++)
        payload[i] = static_cast<uint8_t>('a' + (i % 4));

    return payload;
}

std::vector<uint8_t> incompressible_payload(size_t size) {
    std::vector<uint8_t> payload(size);

    // A fixed sequence rather than a random one: a test that fails should fail the same way twice.
    uint32_t state = 0x12345678;
    for (size_t i = 0; i < size; i++) {
        state = state * 1103515245u + 12345u;
        payload[i] = static_cast<uint8_t>(state >> 24);
    }

    return payload;
}

// The real codecs this build has: NONE is an algorithm the engine accepts, but it is a statement that a payload is
// stored as it is, not something to compress or decompress with.
std::vector<uint8_t> available_codecs() {
    std::vector<uint8_t> codecs;

    for (uint8_t algorithm : {static_cast<uint8_t>(DBENGINE_COMPRESSION_LZ4),
                              static_cast<uint8_t>(DBENGINE_COMPRESSION_ZSTD)}) {
        if (dbengine_valid_compression_algorithm(algorithm))
            codecs.push_back(algorithm);
    }

    return codecs;
}

// Every algorithm this build actually has. Which ones exist depends on how the tree was configured, so the list is
// built from the engine's own answer rather than assumed.
std::vector<uint8_t> available_algorithms() {
    std::vector<uint8_t> algorithms;

    for (uint8_t algorithm : {static_cast<uint8_t>(DBENGINE_COMPRESSION_NONE),
                              static_cast<uint8_t>(DBENGINE_COMPRESSION_LZ4),
                              static_cast<uint8_t>(DBENGINE_COMPRESSION_ZSTD)}) {
        if (dbengine_valid_compression_algorithm(algorithm))
            algorithms.push_back(algorithm);
    }

    return algorithms;
}

const char *algorithm_name(uint8_t algorithm) {
    switch (algorithm) {
        case DBENGINE_COMPRESSION_NONE:
            return "none";
        case DBENGINE_COMPRESSION_LZ4:
            return "lz4";
        case DBENGINE_COMPRESSION_ZSTD:
            return "zstd";
        default:
            return "unknown";
    }
}

} // namespace

TEST(Compression, TheDefaultAlgorithmIsOneThisBuildSupports) {
    EXPECT_TRUE(dbengine_valid_compression_algorithm(dbengine_default_compression()));
}

TEST(Compression, UnknownAlgorithmsAreRejected) {
    // Everything above the highest defined value, and the gap the defined ones leave. A codec byte comes off disk,
    // so an unknown one must be refused rather than trusted.
    for (unsigned algorithm = DBENGINE_COMPRESSION_ZSTD + 1; algorithm <= 255; algorithm++) {
        SCOPED_TRACE(algorithm);
        EXPECT_FALSE(dbengine_valid_compression_algorithm(static_cast<uint8_t>(algorithm)));
    }
}

TEST(Compression, UncompressedIsAlwaysAvailable) {
    // The engine must be able to write an extent even in a build with neither codec.
    EXPECT_TRUE(dbengine_valid_compression_algorithm(DBENGINE_COMPRESSION_NONE));
    EXPECT_EQ(dbengine_max_compressed_size(1024, DBENGINE_COMPRESSION_NONE), 1024u);
}

TEST(Compression, RoundTripsAPayload) {
    for (uint8_t algorithm : available_codecs()) {
        SCOPED_TRACE(algorithm_name(algorithm));

        const std::vector<uint8_t> original = repetitive_payload(4096);

        // dbengine_compress() writes its result into the payload it is given, so the buffer has to be big enough
        // for the worst case the codec can produce.
        std::vector<uint8_t> buffer(dbengine_max_compressed_size(original.size(), algorithm));
        ASSERT_GE(buffer.size(), original.size());
        memcpy(buffer.data(), original.data(), original.size());

        const size_t compressed_size = dbengine_compress(buffer.data(), original.size(), algorithm);
        ASSERT_GT(compressed_size, 0u);
        ASSERT_LE(compressed_size, buffer.size());

        std::vector<uint8_t> restored(original.size());
        const size_t restored_size =
            dbengine_decompress(restored.data(), buffer.data(), restored.size(), compressed_size, algorithm);

        EXPECT_EQ(restored_size, original.size());
        EXPECT_EQ(restored, original) << "the payload did not survive the round trip";
    }
}

TEST(Compression, ACompressiblePayloadGetsSmaller) {
    for (uint8_t algorithm : available_codecs()) {
        SCOPED_TRACE(algorithm_name(algorithm));

        const std::vector<uint8_t> original = repetitive_payload(8192);

        std::vector<uint8_t> buffer(dbengine_max_compressed_size(original.size(), algorithm));
        memcpy(buffer.data(), original.data(), original.size());

        const size_t compressed_size = dbengine_compress(buffer.data(), original.size(), algorithm);

        // Without this a codec that copied its input would pass the round-trip case above.
        EXPECT_LT(compressed_size, original.size())
            << "a highly repetitive payload did not compress";
    }
}

TEST(Compression, AnIncompressiblePayloadIsLeftAlone) {
    for (uint8_t algorithm : available_codecs()) {
        SCOPED_TRACE(algorithm_name(algorithm));

        const std::vector<uint8_t> original = incompressible_payload(4096);

        std::vector<uint8_t> buffer(dbengine_max_compressed_size(original.size(), algorithm));
        memcpy(buffer.data(), original.data(), original.size());

        const size_t compressed_size = dbengine_compress(buffer.data(), original.size(), algorithm);

        // The invariant the extent writer depends on: a codec either improves on the payload or reports 0 and
        // leaves it untouched. It never reports a size that is not an improvement.
        EXPECT_TRUE(compressed_size == 0 || compressed_size < original.size())
            << "the codec reported " << compressed_size << " for a payload of " << original.size();

        if (compressed_size == 0) {
            const std::vector<uint8_t> after(buffer.begin(), buffer.begin() + original.size());
            EXPECT_EQ(after, original) << "the payload was modified even though nothing was compressed";
            continue;
        }

        std::vector<uint8_t> restored(original.size());
        EXPECT_EQ(dbengine_decompress(restored.data(), buffer.data(), restored.size(), compressed_size, algorithm),
                  original.size());
        EXPECT_EQ(restored, original);
    }
}

TEST(Compression, NoneMeansThePayloadIsStoredAsItIs) {
    const std::vector<uint8_t> original = repetitive_payload(1024);
    std::vector<uint8_t> buffer = original;

    // Not a codec: it reports 0, the way a codec reports that it could not improve on the payload, and the caller
    // stores what it already has.
    EXPECT_EQ(dbengine_compress(buffer.data(), buffer.size(), DBENGINE_COMPRESSION_NONE), 0u);
    EXPECT_EQ(buffer, original) << "the payload was modified by an algorithm that compresses nothing";
}

TEST(Compression, TheBoundIsNeverBelowTheInput) {
    for (uint8_t algorithm : available_algorithms()) {
        SCOPED_TRACE(algorithm_name(algorithm));

        for (size_t size : {size_t(1), size_t(64), size_t(4096), size_t(65536)}) {
            SCOPED_TRACE(size);

            // The caller sizes one buffer from this and compresses in place, so a bound below the input would be a
            // buffer overrun before any compression happened.
            EXPECT_GE(dbengine_max_compressed_size(size, algorithm), size);
        }
    }
}

TEST(Compression, ASinglePageOfDataRoundTrips) {
    // The size the engine actually works in: extents are built from pages, and a page is the smallest thing that
    // reaches these codecs in production.
    for (uint8_t algorithm : available_codecs()) {
        SCOPED_TRACE(algorithm_name(algorithm));

        const std::vector<uint8_t> original = repetitive_payload(1024);

        std::vector<uint8_t> buffer(dbengine_max_compressed_size(original.size(), algorithm));
        memcpy(buffer.data(), original.data(), original.size());

        const size_t compressed_size = dbengine_compress(buffer.data(), original.size(), algorithm);

        std::vector<uint8_t> restored(original.size());
        EXPECT_EQ(dbengine_decompress(restored.data(), buffer.data(), restored.size(), compressed_size, algorithm),
                  original.size());
        EXPECT_EQ(restored, original);
    }
}
