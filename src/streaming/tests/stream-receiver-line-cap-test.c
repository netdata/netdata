// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "streaming/stream-receiver-internals.h"

typedef struct {
    struct buffered_reader reader;
    BUFFER *line_buffer;
} line_test_context;

static void line_test_init(line_test_context *ctx) {
    buffered_reader_init(&ctx->reader);
    ctx->line_buffer = buffer_create(16, NULL);
}

static void line_test_cleanup(line_test_context *ctx) {
    buffer_free(ctx->line_buffer);
}

static void line_test_set_input(
    line_test_context *ctx,
    char value,
    size_t len,
    bool newline) {

    memset(ctx->reader.read_buffer, value, len);

    if(newline)
        ctx->reader.read_buffer[len - 1] = '\n';

    ctx->reader.read_buffer[len] = '\0';
    ctx->reader.read_len = (ssize_t)len;
    ctx->reader.pos = 0;
}

static int test_partial_line_crosses_limit(void) {
    line_test_context ctx;
    line_test_init(&ctx);

    const size_t chunk_size = 4096;
    const size_t expected_chunks =
        (PLUGINSD_LINE_MAX / chunk_size) + 1;

    bool overflow = false;
    size_t chunks = 0;

    while(chunks < expected_chunks && !overflow) {
        line_test_set_input(&ctx, 'A', chunk_size, false);

        bool complete = stream_receiver_next_line_checked(
            &ctx.reader,
            ctx.line_buffer,
            &overflow);

        if(complete) {
            fprintf(
                stderr,
                "partial-line test unexpectedly produced a complete line\n");
            line_test_cleanup(&ctx);
            return 1;
        }

        chunks++;
    }

    int failed = 0;

    if(!overflow) {
        fprintf(
            stderr,
            "partial-line overflow was not detected after %zu chunks\n",
            chunks);
        failed = 1;
    }
    else if(chunks != expected_chunks) {
        fprintf(
            stderr,
            "partial-line overflow fired after %zu chunks, expected %zu\n",
            chunks,
            expected_chunks);
        failed = 1;
    }

    line_test_cleanup(&ctx);
    return failed;
}

static int test_line_limit_boundary(void) {
    BUFFER *line_buffer = buffer_create(16, NULL);
    buffer_need_bytes(line_buffer, PLUGINSD_LINE_MAX + 2);

    int failed = 0;

    line_buffer->len = PLUGINSD_LINE_MAX;
    line_buffer->buffer[line_buffer->len] = '\0';

    if(stream_receiver_line_buffer_overflow(line_buffer)) {
        fprintf(
            stderr,
            "overflow reported at exactly PLUGINSD_LINE_MAX (%d)\n",
            PLUGINSD_LINE_MAX);
        failed = 1;
    }

    line_buffer->len = PLUGINSD_LINE_MAX + 1;
    line_buffer->buffer[line_buffer->len] = '\0';

    if(!stream_receiver_line_buffer_overflow(line_buffer)) {
        fprintf(
            stderr,
            "overflow not reported at PLUGINSD_LINE_MAX + 1\n");
        failed = 1;
    }

    buffer_free(line_buffer);
    return failed;
}

static int test_overlong_newline_terminated_line(void) {
    line_test_context ctx;
    line_test_init(&ctx);

    bool overflow = false;

    line_test_set_input(
        &ctx,
        'B',
        PLUGINSD_LINE_MAX,
        false);

    bool complete = stream_receiver_next_line_checked(
        &ctx.reader,
        ctx.line_buffer,
        &overflow);

    if(complete ||
       overflow ||
       ctx.line_buffer->len != PLUGINSD_LINE_MAX) {

        fprintf(
            stderr,
            "unexpected state at line limit: "
            "complete=%d overflow=%d len=%zu\n",
            complete,
            overflow,
            ctx.line_buffer->len);

        line_test_cleanup(&ctx);
        return 1;
    }

    line_test_set_input(&ctx, 'C', 2, true);

    complete = stream_receiver_next_line_checked(
        &ctx.reader,
        ctx.line_buffer,
        &overflow);

    int failed = 0;

    if(!complete || !overflow) {
        fprintf(
            stderr,
            "overlong newline-terminated line was not rejected "
            "(complete=%d overflow=%d len=%zu)\n",
            complete,
            overflow,
            ctx.line_buffer->len);
        failed = 1;
    }

    line_test_cleanup(&ctx);
    return failed;
}

static int test_normal_complete_line(void) {
    line_test_context ctx;
    line_test_init(&ctx);

    line_test_set_input(&ctx, 'D', 4096, true);

    bool overflow = false;
    bool complete = stream_receiver_next_line_checked(
        &ctx.reader,
        ctx.line_buffer,
        &overflow);

    int failed = 0;

    if(!complete || overflow) {
        fprintf(
            stderr,
            "normal complete line failed "
            "(complete=%d overflow=%d len=%zu)\n",
            complete,
            overflow,
            ctx.line_buffer->len);
        failed = 1;
    }

    line_test_cleanup(&ctx);
    return failed;
}

int main(void) {
    int failed = 0;

    failed += test_partial_line_crosses_limit();
    failed += test_line_limit_boundary();
    failed += test_overlong_newline_terminated_line();
    failed += test_normal_complete_line();

    if(failed) {
        fprintf(
            stderr,
            "stream-receiver-line-cap-test: %d failures\n",
            failed);
        return 1;
    }

    fprintf(stderr, "stream-receiver-line-cap-test: OK\n");
    return 0;
}