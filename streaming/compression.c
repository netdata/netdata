// SPDX-License-Identifier: GPL-3.0-or-later

#include "compression.h"

#include "compression_gzip.h"

#ifdef ENABLE_LZ4
#include "compression_lz4.h"
#endif

#ifdef ENABLE_ZSTD
#include "compression_zstd.h"
#endif

#ifdef ENABLE_BROTLI
#include "compression_brotli.h"
#endif

int rrdpush_compression_levels[COMPRESSION_ALGORITHM_MAX] = {
        [COMPRESSION_ALGORITHM_NONE]    = 0,
        [COMPRESSION_ALGORITHM_BROTLI]  = 3,    // 0 (faster)  - 11 (smaller)
        [COMPRESSION_ALGORITHM_ZSTD]    = 3,    // 1 (faster)  - 22 (smaller)
        [COMPRESSION_ALGORITHM_LZ4]     = 1,    // 1 (smaller) -  9 (faster)
        [COMPRESSION_ALGORITHM_GZIP]    = 1,    // 1 (faster)  -  9 (smaller)
};

void rrdpush_parse_compression_order(struct receiver_state *rpt, const char *order) {
    // empty all slots
    for(size_t i = 0; i < COMPRESSION_ALGORITHM_MAX ;i++)
        rpt->config.compression_priorities[i] = STREAM_CAP_NONE;

    char *s = strdupz(order);

    char *words[COMPRESSION_ALGORITHM_MAX + 100] = { NULL };
    size_t num_words = quoted_strings_splitter_pluginsd(s, words, COMPRESSION_ALGORITHM_MAX + 100);
    size_t slot = 0;
    STREAM_CAPABILITIES added = STREAM_CAP_NONE;
    for(size_t i = 0; i < num_words && slot < COMPRESSION_ALGORITHM_MAX ;i++) {
        if((STREAM_CAP_BROTLI_AVAILABLE) && strcasecmp(words[i], "brotli") == 0 && !(added & STREAM_CAP_BROTLI)) {
            rpt->config.compression_priorities[slot++] = STREAM_CAP_BROTLI;
            added |= STREAM_CAP_BROTLI;
        }
        else if((STREAM_CAP_ZSTD_AVAILABLE) && strcasecmp(words[i], "zstd") == 0 && !(added & STREAM_CAP_ZSTD)) {
            rpt->config.compression_priorities[slot++] = STREAM_CAP_ZSTD;
            added |= STREAM_CAP_ZSTD;
        }
        else if((STREAM_CAP_LZ4_AVAILABLE) && strcasecmp(words[i], "lz4") == 0 && !(added & STREAM_CAP_LZ4)) {
            rpt->config.compression_priorities[slot++] = STREAM_CAP_LZ4;
            added |= STREAM_CAP_LZ4;
        }
        else if(strcasecmp(words[i], "gzip") == 0 && !(added & STREAM_CAP_GZIP)) {
            rpt->config.compression_priorities[slot++] = STREAM_CAP_GZIP;
            added |= STREAM_CAP_GZIP;
        }
    }

    freez(s);

    // make sure all participate
    if((STREAM_CAP_BROTLI_AVAILABLE) && slot < COMPRESSION_ALGORITHM_MAX && !(added & STREAM_CAP_BROTLI))
        rpt->config.compression_priorities[slot++] = STREAM_CAP_BROTLI;
    if((STREAM_CAP_ZSTD_AVAILABLE) && slot < COMPRESSION_ALGORITHM_MAX && !(added & STREAM_CAP_ZSTD))
        rpt->config.compression_priorities[slot++] = STREAM_CAP_ZSTD;
    if((STREAM_CAP_LZ4_AVAILABLE) && slot < COMPRESSION_ALGORITHM_MAX && !(added & STREAM_CAP_LZ4))
        rpt->config.compression_priorities[slot++] = STREAM_CAP_LZ4;
    if(slot < COMPRESSION_ALGORITHM_MAX && !(added & STREAM_CAP_GZIP))
        rpt->config.compression_priorities[slot++] = STREAM_CAP_GZIP;
}

void rrdpush_select_receiver_compression_algorithm(struct receiver_state *rpt) {
    if (!rpt->config.rrdpush_compression)
        rpt->capabilities &= ~STREAM_CAP_COMPRESSIONS_AVAILABLE;

    // select the right compression before sending our capabilities to the child
    if(stream_has_more_than_one_capability_of(rpt->capabilities, STREAM_CAP_COMPRESSIONS_AVAILABLE)) {
        STREAM_CAPABILITIES compressions = rpt->capabilities & STREAM_CAP_COMPRESSIONS_AVAILABLE;
        for(int i = 0; i < COMPRESSION_ALGORITHM_MAX; i++) {
            STREAM_CAPABILITIES c = rpt->config.compression_priorities[i];

            if(!(c & STREAM_CAP_COMPRESSIONS_AVAILABLE))
                continue;

            if(compressions & c) {
                STREAM_CAPABILITIES exclude = compressions;
                exclude &= ~c;

                rpt->capabilities &= ~exclude;
                break;
            }
        }
    }
}

bool rrdpush_compression_initialize(struct sender_state *s) {
    rrdpush_compressor_destroy(&s->compressor);

    // IMPORTANT
    // KEEP THE SAME ORDER IN DECOMPRESSION

    if(stream_has_capability(s, STREAM_CAP_BROTLI))
        s->compressor.algorithm = COMPRESSION_ALGORITHM_BROTLI;
    else if(stream_has_capability(s, STREAM_CAP_ZSTD))
        s->compressor.algorithm = COMPRESSION_ALGORITHM_ZSTD;
    else if(stream_has_capability(s, STREAM_CAP_LZ4))
        s->compressor.algorithm = COMPRESSION_ALGORITHM_LZ4;
    else if(stream_has_capability(s, STREAM_CAP_GZIP))
        s->compressor.algorithm = COMPRESSION_ALGORITHM_GZIP;
    else
        s->compressor.algorithm = COMPRESSION_ALGORITHM_NONE;

    if(s->compressor.algorithm != COMPRESSION_ALGORITHM_NONE) {
        s->compressor.level = rrdpush_compression_levels[s->compressor.algorithm];
        rrdpush_compressor_init(&s->compressor);
        return true;
    }

    return false;
}

bool rrdpush_decompression_initialize(struct receiver_state *rpt) {
    rrdpush_decompressor_destroy(&rpt->decompressor);

    // IMPORTANT
    // KEEP THE SAME ORDER IN COMPRESSION

    if(stream_has_capability(rpt, STREAM_CAP_BROTLI))
        rpt->decompressor.algorithm = COMPRESSION_ALGORITHM_BROTLI;
    else if(stream_has_capability(rpt, STREAM_CAP_ZSTD))
        rpt->decompressor.algorithm = COMPRESSION_ALGORITHM_ZSTD;
    else if(stream_has_capability(rpt, STREAM_CAP_LZ4))
        rpt->decompressor.algorithm = COMPRESSION_ALGORITHM_LZ4;
    else if(stream_has_capability(rpt, STREAM_CAP_GZIP))
        rpt->decompressor.algorithm = COMPRESSION_ALGORITHM_GZIP;
    else
        rpt->decompressor.algorithm = COMPRESSION_ALGORITHM_NONE;

    if(rpt->decompressor.algorithm != COMPRESSION_ALGORITHM_NONE) {
        rrdpush_decompressor_init(&rpt->decompressor);
        return true;
    }

    return false;
}



// ----------------------------------------------------------------------------
// compressor public API

void rrdpush_compressor_init(struct compressor_state *state) {
    switch(state->algorithm) {
#ifdef ENABLE_BROTLI
        case COMPRESSION_ALGORITHM_BROTLI:
            rrdpush_compressor_init_brotli(state);
            break;
#endif

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_compressor_init_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_compressor_init_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_compressor_init_gzip(state);
            break;
    }

    simple_ring_buffer_reset(&state->input);
    simple_ring_buffer_reset(&state->output);
}

void rrdpush_compressor_destroy(struct compressor_state *state) {
    switch(state->algorithm) {
#ifdef ENABLE_BROTLI
        case COMPRESSION_ALGORITHM_BROTLI:
            rrdpush_compressor_destroy_brotli(state);
            break;
#endif

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_compressor_destroy_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_compressor_destroy_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_compressor_destroy_gzip(state);
            break;
    }

    state->initialized = false;

    simple_ring_buffer_destroy(&state->input);
    simple_ring_buffer_destroy(&state->output);
}

size_t rrdpush_compress(struct compressor_state *state, const char *data, size_t size, const char **out) {
    size_t ret = 0;

    switch(state->algorithm) {
#ifdef ENABLE_BROTLI
        case COMPRESSION_ALGORITHM_BROTLI:
            ret = rrdpush_compress_brotli(state, data, size, out);
            break;
#endif

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            ret = rrdpush_compress_zstd(state, data, size, out);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            ret = rrdpush_compress_lz4(state, data, size, out);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            ret = rrdpush_compress_gzip(state, data, size, out);
            break;
    }

    if(unlikely(ret >= COMPRESSION_MAX_CHUNK)) {
        netdata_log_error("RRDPUSH_COMPRESS: compressed data is %zu bytes, which is >= than the max chunk size %zu",
                ret, COMPRESSION_MAX_CHUNK);
        return 0;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// decompressor public API

void rrdpush_decompressor_destroy(struct decompressor_state *state) {
    if(unlikely(!state->initialized))
        return;

    switch(state->algorithm) {
#ifdef ENABLE_BROTLI
        case COMPRESSION_ALGORITHM_BROTLI:
            rrdpush_decompressor_destroy_brotli(state);
            break;
#endif

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_decompressor_destroy_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_decompressor_destroy_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_decompressor_destroy_gzip(state);
            break;
    }

    simple_ring_buffer_destroy(&state->output);

    state->initialized = false;
}

void rrdpush_decompressor_init(struct decompressor_state *state) {
    switch(state->algorithm) {
#ifdef ENABLE_BROTLI
        case COMPRESSION_ALGORITHM_BROTLI:
            rrdpush_decompressor_init_brotli(state);
            break;
#endif

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            rrdpush_decompressor_init_zstd(state);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            rrdpush_decompressor_init_lz4(state);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            rrdpush_decompressor_init_gzip(state);
            break;
    }

    state->signature_size = RRDPUSH_COMPRESSION_SIGNATURE_SIZE;
    simple_ring_buffer_reset(&state->output);
}

size_t rrdpush_decompress(struct decompressor_state *state, const char *compressed_data, size_t compressed_size) {
    if (unlikely(state->output.read_pos != state->output.write_pos))
        fatal("RRDPUSH_DECOMPRESS: asked to decompress new data, while there are unread data in the decompression buffer!");

    size_t ret = 0;

    switch(state->algorithm) {
#ifdef ENABLE_BROTLI
        case COMPRESSION_ALGORITHM_BROTLI:
            ret = rrdpush_decompress_brotli(state, compressed_data, compressed_size);
            break;
#endif

#ifdef ENABLE_ZSTD
        case COMPRESSION_ALGORITHM_ZSTD:
            ret = rrdpush_decompress_zstd(state, compressed_data, compressed_size);
            break;
#endif

#ifdef ENABLE_LZ4
        case COMPRESSION_ALGORITHM_LZ4:
            ret = rrdpush_decompress_lz4(state, compressed_data, compressed_size);
            break;
#endif

        default:
        case COMPRESSION_ALGORITHM_GZIP:
            ret = rrdpush_decompress_gzip(state, compressed_data, compressed_size);
            break;
    }

    // for backwards compatibility we cannot check for COMPRESSION_MAX_MSG_SIZE,
    // because old children may send this big payloads.
    if(unlikely(ret > COMPRESSION_MAX_CHUNK)) {
        netdata_log_error("RRDPUSH_DECOMPRESS: decompressed data is %zu bytes, which is bigger than the max msg size %zu",
                          ret, COMPRESSION_MAX_CHUNK);
        return 0;
    }

    return ret;
}

// ----------------------------------------------------------------------------
// unit test

int unittest_rrdpush_compression(compression_algorithm_t algorithm, const char *name) {
    fprintf(stderr, "\nTesting streaming compression with %s\n", name);

    struct compressor_state cctx =  {
            .initialized = false,
            .algorithm = algorithm,
    };
    struct decompressor_state dctx = {
            .initialized = false,
            .algorithm = algorithm,
    };

    char txt[COMPRESSION_MAX_MSG_SIZE];

    rrdpush_compressor_init(&cctx);
    rrdpush_decompressor_init(&dctx);

    int errors = 0;

    memset(txt, '=', COMPRESSION_MAX_MSG_SIZE);

    for(int i = 0; i < COMPRESSION_MAX_MSG_SIZE ;i++) {
        txt[i] = 'A' + (i % 26);
        size_t txt_len = i + 1;

        const char *out;
        size_t size = rrdpush_compress(&cctx, txt, txt_len, &out);

        if(size == 0) {
            fprintf(stderr, "iteration %d: compressed size %zu is zero\n",
                    i, size);
            errors++;
            goto cleanup;
        }
        else if(size >= COMPRESSION_MAX_CHUNK) {
            fprintf(stderr, "iteration %d: compressed size %zu exceeds max allowed size\n",
                    i, size);
            errors++;
            goto cleanup;
        }
        else {
            size_t dtxt_len = rrdpush_decompress(&dctx, out, size);
            char *dtxt = (char *) &dctx.output.data[dctx.output.read_pos];

            if(rrdpush_decompressed_bytes_in_buffer(&dctx) != dtxt_len) {
                fprintf(stderr, "iteration %d: decompressed size %zu does not rrdpush_decompressed_bytes_in_buffer() %zu\n",
                        i, dtxt_len, rrdpush_decompressed_bytes_in_buffer(&dctx)
                       );
                errors++;
                goto cleanup;
            }

            if(!dtxt_len) {
                fprintf(stderr, "iteration %d: decompressed size is zero\n", i);
                errors++;
                goto cleanup;
            }
            else if(dtxt_len != txt_len) {
                fprintf(stderr, "iteration %d: decompressed size %zu does not match original size %zu\n",
                        i, dtxt_len, txt_len
                       );
                errors++;
                goto cleanup;
            }
            else {
                if(memcmp(txt, dtxt, txt_len) != 0) {
                    txt[txt_len] = '\0';
                    dtxt[txt_len + 5] = '\0';

                    fprintf(stderr, "iteration %d: decompressed data '%s' do not match original data '%s' of length %zu\n",
                            i, dtxt, txt, txt_len);
                    errors++;
                    goto cleanup;
                }
            }
        }

        // fill the compressed buffer with garbage
        memset((void *)out, 'x', size);

        // here we are supposed to copy the data and advance the position
        dctx.output.read_pos += rrdpush_decompressed_bytes_in_buffer(&dctx);
    }

cleanup:
    rrdpush_compressor_destroy(&cctx);
    rrdpush_decompressor_destroy(&dctx);

    if(errors)
        fprintf(stderr, "Compression with %s: FAILED (%d errors)\n", name, errors);
    else
        fprintf(stderr, "Compression with %s: OK\n", name);

    return errors;
}

int unittest_rrdpush_compressions(void) {
    int ret = 0;

    ret += unittest_rrdpush_compression(COMPRESSION_ALGORITHM_BROTLI, "BROTLI");
    ret += unittest_rrdpush_compression(COMPRESSION_ALGORITHM_GZIP, "GZIP");
    ret += unittest_rrdpush_compression(COMPRESSION_ALGORITHM_LZ4, "LZ4");
    ret += unittest_rrdpush_compression(COMPRESSION_ALGORITHM_ZSTD, "ZSTD");

    return ret;
}
