// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#ifndef NETDATA_BUFFERED_READER_H
#define NETDATA_BUFFERED_READER_H

struct buffered_reader {
    ssize_t read_len;
    ssize_t pos;
    char read_buffer[PLUGINSD_LINE_MAX + 1];
};

static inline void buffered_reader_init(struct buffered_reader *reader) {
    reader->read_buffer[0] = '\0';
    reader->read_len = 0;
    reader->pos = 0;
}

typedef enum {
    BUFFERED_READER_READ_OK = 0,
    BUFFERED_READER_READ_FAILED = -1,
    BUFFERED_READER_READ_BUFFER_FULL = -2,
    BUFFERED_READER_READ_POLLERR = -3,
    BUFFERED_READER_READ_POLLHUP = -4,
    BUFFERED_READER_READ_POLLNVAL = -5,
    BUFFERED_READER_READ_POLL_UNKNOWN = -6,
    BUFFERED_READER_READ_POLL_TIMEOUT = -7,
    BUFFERED_READER_READ_POLL_CANCELLED = -8,
} buffered_reader_ret_t;


static inline buffered_reader_ret_t buffered_reader_read(struct buffered_reader *reader, int fd) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(reader->read_buffer[reader->read_len] != '\0')
        fatal("read_buffer does not start with zero");
#endif

    char *read_at = reader->read_buffer + reader->read_len;
    ssize_t remaining = sizeof(reader->read_buffer) - reader->read_len - 1;

    if(unlikely(remaining <= 0))
        return BUFFERED_READER_READ_BUFFER_FULL;

    ssize_t bytes_read = read(fd, read_at, remaining);
    if(unlikely(bytes_read <= 0))
        return BUFFERED_READER_READ_FAILED;

    reader->read_len += bytes_read;
    reader->read_buffer[reader->read_len] = '\0';

    return BUFFERED_READER_READ_OK;
}

static inline buffered_reader_ret_t buffered_reader_read_timeout(struct buffered_reader *reader, int fd, int timeout_ms, bool log_error) {
    short int revents = 0;
    switch(wait_on_socket_or_cancel_with_timeout(
        NULL,
        fd, timeout_ms, POLLIN, &revents)) {

        case 0: // data are waiting
            return buffered_reader_read(reader, fd);

        case 1: // timeout reached
            if(log_error)
                netdata_log_error("PARSER: timeout while waiting for data.");
            return BUFFERED_READER_READ_POLL_TIMEOUT;

        case -1: // thread cancelled
            netdata_log_error("PARSER: thread cancelled while waiting for data.");
            return BUFFERED_READER_READ_POLL_CANCELLED;

        default:
        case 2: // error on socket
            if(revents & POLLERR) {
                if(log_error)
                    netdata_log_error("PARSER: read failed: POLLERR.");
                return BUFFERED_READER_READ_POLLERR;
            }
            if(revents & POLLHUP) {
                if(log_error)
                    netdata_log_error("PARSER: read failed: POLLHUP.");
                return BUFFERED_READER_READ_POLLHUP;
            }
            if(revents & POLLNVAL) {
                if(log_error)
                    netdata_log_error("PARSER: read failed: POLLNVAL.");
                return BUFFERED_READER_READ_POLLNVAL;
            }
    }

    if(log_error)
        netdata_log_error("PARSER: poll() returned positive number, but POLLIN|POLLERR|POLLHUP|POLLNVAL are not set.");
    return BUFFERED_READER_READ_POLL_UNKNOWN;
}

/* Produce a full line if one exists, statefully return where we start next time.
 * When we hit the end of the buffer with a partial line move it to the beginning for the next fill.
 */
static inline bool buffered_reader_next_line(struct buffered_reader *reader, BUFFER *dst) {
    buffer_need_bytes(dst, reader->read_len - reader->pos + 2);

    size_t start = reader->pos;

    char *ss = &reader->read_buffer[start];
    char *se = &reader->read_buffer[reader->read_len];
    char *ds = &dst->buffer[dst->len];
    char *de = &ds[dst->size - dst->len - 2];

    if(ss >= se) {
        *ds = '\0';
        reader->pos = 0;
        reader->read_len = 0;
        reader->read_buffer[reader->read_len] = '\0';
        return false;
    }

    // Find out how many bytes we want to copy and whether we found a newline
    size_t bytes_to_copy;
    bool found_newline = false;
    {
        char *next_newline = (char *) memchr(ss, '\n', se - ss);
        if (!next_newline) {
            bytes_to_copy = se - ss;
        } else {
            bytes_to_copy = (next_newline - ss) + 1;
            found_newline = true;
        }
    }

    // Check we don't overflow the destination buffer
    if (bytes_to_copy > (size_t)(de - ds)) {
        bytes_to_copy = de - ds;
        found_newline = false;
    }

    memcpy(ds, ss, bytes_to_copy);
    ds[bytes_to_copy] = '\0';
    dst->len += bytes_to_copy;

    if (found_newline) {
        reader->pos = start + bytes_to_copy;
        return true;
    }

    reader->pos = 0;
    reader->read_len = 0;
    reader->read_buffer[reader->read_len] = '\0';
    return false;
}

#endif //NETDATA_BUFFERED_READER_H
