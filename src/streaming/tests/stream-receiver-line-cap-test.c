// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "streaming/stream-receiver-internals.h"

// The receiver loop feeds buffered_reader_next_line() from a 15KB-ish read
// buffer; without a newline everything accumulates into the line buffer.
// This test drives the real primitive the same way and checks the threshold
// the receiver acts on, including the order in which the receiver checks it.

// One round of the receiver loop over a filled read buffer: every complete
// line is checked against the cap right after the append and before the
// parser consumes and resets the buffer.
typedef struct {
    size_t lines_parsed;
    bool overflow;
} receiver_step_result;

static receiver_step_result receiver_process_buffer(struct buffered_reader *reader, BUFFER *line_buffer) {
    receiver_step_result res = { 0, false };

    while(buffered_reader_next_line(reader, line_buffer)) {
        if(stream_receiver_line_buffer_overflow(line_buffer)) {
            res.overflow = true;
            return res; // the receiver disconnects without parsing this line
        }

        res.lines_parsed++;
        line_buffer->len = 0;
        line_buffer->buffer[0] = '\0';
    }

    // partial line left over from this buffer
    if(stream_receiver_line_buffer_overflow(line_buffer))
        res.overflow = true;

    return res;
}

static size_t feed_no_newline(struct buffered_reader *reader, BUFFER *line_buffer, size_t chunks) {
    char chunk[4096];
    memset(chunk, 'A', sizeof(chunk));

    size_t fed = 0;
    for(size_t i = 0; i < chunks; i++) {
        memcpy(reader->read_buffer, chunk, sizeof(chunk));
        reader->read_len = sizeof(chunk);
        reader->pos = 0;

        receiver_step_result res = receiver_process_buffer(reader, line_buffer);
        if(res.overflow) {
            // the receiver would have disconnected here
            fed += sizeof(chunk);
            break;
        }

        fed += sizeof(chunk);
    }
    return fed;
}

int main(void) {
    int failed = 0;

    // 1. no newline: the line buffer accumulates while under the cap, and the
    //    receiver disconnects on the first buffer that crosses it
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        size_t fed = feed_no_newline(&reader, lb, 2); // 8KB, still under the cap
        if(fed != 2 * 4096 || lb->len != fed || stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "unexpected state after %zu bytes (len %zu), cap is %d\n",
                    fed, lb->len, PLUGINSD_LINE_MAX);
            failed++;
        }

        // more chunks without a newline: the receiver stops at the crossing
        fed = feed_no_newline(&reader, lb, 5);
        if(!stream_receiver_line_buffer_overflow(lb) || lb->len > 5 * 4096) {
            fprintf(stderr, "overflow not reported after %zu bytes without a newline (len %zu, cap %d)\n",
                    fed, lb->len, PLUGINSD_LINE_MAX);
            failed++;
        }

        buffer_free(lb);
    }

    // 2. multi-chunk accumulation without a newline must be detected on the
    //    first chunk that crosses the cap, not after unbounded growth
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        size_t chunks = 0;
        size_t fed = 0;
        bool overflow = false;
        for(size_t i = 0; i < 16 && !overflow; i++) {
            char chunk[4096];
            memset(chunk, 'B', sizeof(chunk));

            memcpy(reader.read_buffer, chunk, sizeof(chunk));
            reader.read_len = sizeof(chunk);
            reader.pos = 0;

            chunks++;
            fed += sizeof(chunk);

            receiver_step_result res = receiver_process_buffer(&reader, lb);
            overflow = res.overflow;
        }

        size_t chunks_before_crossing = (PLUGINSD_LINE_MAX / 4096) + 1; // 15487 -> 4 chunks
        if(!overflow) {
            fprintf(stderr, "no overflow after %zu chunks without a newline\n", chunks);
            failed++;
        }
        else if(chunks != chunks_before_crossing || fed != chunks_before_crossing * 4096) {
            fprintf(stderr, "overflow fired after %zu chunks / %zu bytes, expected %zu chunks / %zu bytes\n",
                    chunks, fed, chunks_before_crossing, chunks_before_crossing * 4096);
            failed++;
        }

        buffer_free(lb);
    }

    // 3. boundary: exactly PLUGINSD_LINE_MAX is accepted, one byte more is not
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

    // 4. regression: a complete line longer than the cap, terminated with \n,
    //    must be detected before the parser consumes and resets the buffer
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        // first read: exactly PLUGINSD_LINE_MAX bytes, no newline yet
        memset(reader.read_buffer, 'C', PLUGINSD_LINE_MAX);
        reader.read_len = PLUGINSD_LINE_MAX;
        reader.pos = 0;

        bool got_line = buffered_reader_next_line(&reader, lb);
        if(got_line || lb->len != PLUGINSD_LINE_MAX || stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "unexpected state after a cap-sized partial line (len %zu)\n", lb->len);
            failed++;
        }

        // second read: completing the line, still without a newline in between
        size_t tail = 400;
        memset(reader.read_buffer, 'D', tail - 1);
        reader.read_buffer[tail - 1] = '\n';
        reader.read_len = tail;
        reader.pos = 0;

        got_line = buffered_reader_next_line(&reader, lb);
        if(!got_line) {
            fprintf(stderr, "complete line was not returned by the reader\n");
            failed++;
        }
        else if(lb->len != PLUGINSD_LINE_MAX + tail) {
            fprintf(stderr, "unexpected line length %zu, expected %zu\n",
                    lb->len, (size_t)PLUGINSD_LINE_MAX + tail);
            failed++;
        }
        else if(!stream_receiver_line_buffer_overflow(lb)) {
            // the receiver checks here, before parser_action() and the reset
            fprintf(stderr, "overlong complete line was not detected before the parser reset\n");
            failed++;
        }

        buffer_free(lb);
    }

    // 5. the reset-then-check order the receiver used before must NOT see the
    //    overlong complete line: this is the bypass the check order fixes
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        memset(reader.read_buffer, 'C', PLUGINSD_LINE_MAX);
        reader.read_len = PLUGINSD_LINE_MAX;
        reader.pos = 0;
        (void)buffered_reader_next_line(&reader, lb);

        size_t tail = 400;
        memset(reader.read_buffer, 'D', tail - 1);
        reader.read_buffer[tail - 1] = '\n';
        reader.read_len = tail;
        reader.pos = 0;

        size_t lines = 0;
        while(buffered_reader_next_line(&reader, lb)) {
            // old order: consume the line first, then reset
            lines++;
            lb->len = 0;
            lb->buffer[0] = '\0';
        }

        if(lines != 1 || stream_receiver_line_buffer_overflow(lb)) {
            fprintf(stderr, "old ordering unexpectedly detected the overlong line "
                            "(lines %zu, len %zu)\n", lines, lb->len);
            failed++;
        }

        buffer_free(lb);
    }

    // 6. complete lines keep the line buffer empty, so normal senders never
    //    reach the cap
    {
        struct buffered_reader reader;
        buffered_reader_init(&reader);
        BUFFER *lb = buffer_create(16, NULL);

        char data[4096];
        memset(data, 'E', sizeof(data) - 1);
        data[sizeof(data) - 1] = '\n';

        for(size_t i = 0; i < 64; i++) { // 256KB of complete lines in 4KB chunks
            memcpy(reader.read_buffer, data, sizeof(data));
            reader.read_len = sizeof(data);
            reader.pos = 0;

            receiver_step_result res = receiver_process_buffer(&reader, lb);

            if(res.lines_parsed != 1 || res.overflow || lb->len != 0) {
                fprintf(stderr, "unexpected state at round %zu (lines %zu, overflow %d, len %zu)\n",
                        i, res.lines_parsed, res.overflow, lb->len);
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