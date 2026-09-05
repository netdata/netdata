// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "streaming/stream-receiver-internals.h"

// The receiver loop feeds buffered_reader_next_line() from a 15KB-ish read
// buffer; without a newline everything accumulates into the line buffer.
// This test drives the real primitive the same way and checks the threshold
// the receiver acts on.

static size_t feed_no_newline(struct buffered_reader *reader, BUFFER *line_buffer, size_t chunks) {
    char chunk[4096];
    memset(chunk, 'A', sizeof(chunk));

    size_t fed = 0;
    for(size_t i = 0; i < chunks; i++) {
        memcpy(reader->read_buffer, chunk, sizeof(chunk));
        reader->read_len = sizeof(chunk);
        reader->pos = 0;

        while(buffered_reader_next_line(reader, line_buffer))
            ; // complete lines are consumed by the parser in the real receiver

        fed += sizeof(chunk);
    }
    return fed;
}

int main(void) {
    int failed = 0;

    // 1. no newline: the line buffer accumulates without bound and the
    //    overflow condition fires as soon as it passes PLUGINSD_LINE_MAX
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        size_t fed = feed_no_newline(&reader, lb, 2); // 8KB, still under the cap
        if(lb->len != fed || stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "unexpected state after %zu bytes (len %zu), cap is %d\n",
                    fed, lb->len, PLUGINSD_LINE_MAX);
            failed++;
        }

        feed_no_newline(&reader, lb, 5); // +20KB, past the cap
        if(lb->len != 7 * 4096 || !stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "overflow not reported after %zu bytes without a newline (cap %d)\n",
                    lb->len, PLUGINSD_LINE_MAX);
            failed++;
        }

        // the accumulation is unbounded: doubling the input doubles the buffer
        size_t len_before = lb->len;
        feed_no_newline(&reader, lb, 14);
        if(lb->len <= len_before) {
            fprintf(stderr, "line buffer did not keep accumulating (%zu -> %zu)\n", len_before, lb->len);
            failed++;
        }

        buffer_free(lb);
    }

    // 2. boundary: exactly PLUGINSD_LINE_MAX is accepted, one byte more is not
    {
        BUFFER *lb = buffer_create(16, NULL);

        lb->len = PLUGINSD_LINE_MAX;
        if(stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "overflow reported at exactly the cap (%d)\n", PLUGINSD_LINE_MAX);
            failed++;
        }

        lb->len = PLUGINSD_LINE_MAX + 1;
        if(!stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "overflow not reported at cap + 1\n");
            failed++;
        }

        buffer_free(lb);
    }

    // 3. complete lines keep the line buffer empty, so normal senders never
    //    reach the cap
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        char data[4096];
        memset(data, 'B', sizeof(data) - 1);
        data[sizeof(data) - 1] = '\n';

        for(size_t i = 0; i < 64; i++) { // 256KB of complete lines in 4KB chunks
            memcpy(reader.read_buffer, data, sizeof(data));
            reader.read_len = sizeof(data);
            reader.pos = 0;

            size_t lines = 0;
            while(buffered_reader_next_line(&reader, lb)) {
                // the parser consumes the line and resets the buffer, like
                // stream_receive_and_process() does
                lb->len = 0;
                lb->buffer[0] = '\0';
                lines++;
            }

            if(lines != 1 || lb->len != 0) {
                fprintf(stderr, "expected 1 complete line at round %zu, got %zu (len %zu)\n",
                        i, lines, lb->len);
                failed++;
                break;
            }
            if(stream_receiver_line_buffer_overflow(lb)) {
                fprintf(stderr, "false overflow with complete lines\n");
                failed++;
                break;
            }
        }

        buffer_free(lb);
    }

    if(failed) {
        fprintf(stderr, "stream-receiver-line-cap-test: %d failures\n", failed);
        return 1;
    }

    fprintf(stderr, "stream-receiver-line-cap-test: OK\n");
    return 0;
}
