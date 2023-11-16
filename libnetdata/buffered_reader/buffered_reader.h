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

static inline bool buffered_reader_read(struct buffered_reader *reader, int fd) {
#ifdef NETDATA_INTERNAL_CHECKS
    if(reader->read_buffer[reader->read_len] != '\0')
        fatal("read_buffer does not start with zero");
#endif

    ssize_t bytes_read = read(fd, reader->read_buffer + reader->read_len, sizeof(reader->read_buffer) - reader->read_len - 1);
    if(unlikely(bytes_read <= 0))
        return false;

    reader->read_len += bytes_read;
    reader->read_buffer[reader->read_len] = '\0';

    return true;
}

static inline bool buffered_reader_read_timeout(struct buffered_reader *reader, int fd, int timeout_ms, bool log_error) {
    errno = 0;
    struct pollfd fds[1];

    fds[0].fd = fd;
    fds[0].events = POLLIN;

    int ret = poll(fds, 1, timeout_ms);

    if (ret > 0) {
        /* There is data to read */
        if (fds[0].revents & POLLIN)
            return buffered_reader_read(reader, fd);

        else if(fds[0].revents & POLLERR) {
            if(log_error)
                netdata_log_error("PARSER: read failed: POLLERR.");
            return false;
        }
        else if(fds[0].revents & POLLHUP) {
            if(log_error)
                netdata_log_error("PARSER: read failed: POLLHUP.");
            return false;
        }
        else if(fds[0].revents & POLLNVAL) {
            if(log_error)
                netdata_log_error("PARSER: read failed: POLLNVAL.");
            return false;
        }

        if(log_error)
            netdata_log_error("PARSER: poll() returned positive number, but POLLIN|POLLERR|POLLHUP|POLLNVAL are not set.");
        return false;
    }
    else if (ret == 0) {
        if(log_error)
            netdata_log_error("PARSER: timeout while waiting for data.");
        return false;
    }

    if(log_error)
        netdata_log_error("PARSER: poll() failed with code %d.", ret);
    return false;
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

    // copy all bytes to buffer
    while(ss < se && ds < de && *ss != '\n') {
        *ds++ = *ss++;
        dst->len++;
    }

    // if we have a newline, return the buffer
    if(ss < se && ds < de && *ss == '\n') {
        // newline found in the r->read_buffer

        *ds++ = *ss++; // copy the newline too
        dst->len++;

        *ds = '\0';

        reader->pos = ss - reader->read_buffer;
        return true;
    }

    reader->pos = 0;
    reader->read_len = 0;
    reader->read_buffer[reader->read_len] = '\0';
    return false;
}

#endif //NETDATA_BUFFERED_READER_H
