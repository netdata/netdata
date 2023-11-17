// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-cat-native.h"
#include "../required_dummies.h"

#ifdef __FreeBSD__
#include <sys/endian.h>
#endif

#ifdef __APPLE__
#include <machine/endian.h>
#endif

static int help(void) {
    fprintf(stderr,
            "Netdata systemd-cat-native\n"
            "\n"
            "This program reads from its standard input, lines in the format:\n"
            "\n"
            "KEY1=VALUE1\\n\n"
            "KEY2=VALUE2\\n\n"
            "KEYN=VALUEN\\n\n"
            "\\n\n"
            "\n"
            "and sends them to systemd-journal as-is.\n"
            "\n"
            "   - Binary data are not accepted\n"
            "   - Messages have to be separated by an empty line\n"
            "   - Keys starting with underscore are not accepted (by journald)\n"
            "\n"
            "Usage:\n"
            "\n"
            "   %s [--namespace=NAMESPACE] [--socket=PATH] [--log-as-netdata|-N] [--newline=STRING]\n"
            "\n"
            "The default namespace and socket depends on whether the program is started by Netdata.\n"
            "When it is started by Netdata, it inherits whatever settings Netdata has.\n"
            "When it is started by other programs, it uses the default namespace and the default\n"
            "systemd-journald socket.\n"
            "\n"
            "--log-as-netdata, means to log the received messages the same way Netdata does\n"
            "(using the same log output and format as the Netdata daemon in its process tree).\n"
            "\n"
            "--newline, sets a string which will be replaced with a newline, allowing sending\n"
            "multiline logs to systemd-journal.\n"
            "\n",
            program_name);

    return 1;
}

static void lgs_reset(struct log_stack_entry *lgs) {
    for(size_t i = 1; i < _NDF_MAX ;i++) {
        if(lgs[i].type == NDFT_TXT && lgs[i].set && lgs[i].txt)
            freez((void *)lgs[i].txt);

        lgs[i] = ND_LOG_FIELD_TXT(i, NULL);
    }

    lgs[0] = ND_LOG_FIELD_TXT(NDF_MESSAGE, NULL);
    lgs[_NDF_MAX] = ND_LOG_FIELD_END();
}

static size_t copy_replacing_newlines(char *dst, size_t dst_len, const char *src, size_t src_len, const char *newline) {
    if (!dst || !src) return 0;

    const char *current_src = src;
    const char *src_end = src + src_len; // Pointer to the end of src
    char *current_dst = dst;
    size_t remaining_dst_len = dst_len;
    size_t newline_len = newline && *newline ? strlen(newline) : 0;

    size_t bytes_copied = 0; // To track the number of bytes copied

    while (remaining_dst_len > 1 && current_src < src_end) {
        if (newline_len > 0) {
            const char *found = strstr(current_src, newline);
            if (found && found < src_end) {
                size_t copy_len = found - current_src;
                if (copy_len >= remaining_dst_len) copy_len = remaining_dst_len - 1;

                memcpy(current_dst, current_src, copy_len);
                current_dst += copy_len;
                *current_dst++ = '\n';
                remaining_dst_len -= (copy_len + 1);
                bytes_copied += copy_len + 1; // +1 for the newline character
                current_src = found + newline_len;
                continue;
            }
        }

        // Copy the remaining part of src to dst
        size_t copy_len = src_end - current_src;
        if (copy_len >= remaining_dst_len) copy_len = remaining_dst_len - 1;

        memcpy(current_dst, current_src, copy_len);
        current_dst += copy_len;
        remaining_dst_len -= copy_len;
        bytes_copied += copy_len;
        break;
    }

    // Ensure the string is null-terminated
    *current_dst = '\0';

    return bytes_copied;
}

static const char *strdupz_replacing_newlines(const char *src, const char *newline) {
    if(!src) src = "";

    size_t src_len = strlen(src);
    char *buffer = mallocz(src_len + 1);
    copy_replacing_newlines(buffer, src_len + 1, src, src_len, newline);
    return buffer;
}

static void buffer_memcat_replacing_newlines(BUFFER *wb, const char *src, size_t src_len, const char *newline) {
    if(!src) return;

    const char *equal;
    if(!newline || !*newline || !strstr(src, newline) || !(equal = strchr(src, '='))) {
        buffer_memcat(wb, src, src_len);
        return;
    }

    size_t key_len = equal - src;
    buffer_memcat(wb, src, key_len);
    buffer_putc(wb, '\n');

    char *length_ptr = &wb->buffer[wb->len];
    uint64_t le_size = 0;
    buffer_memcat(wb, &le_size, sizeof(le_size));

    const char *value = ++equal;
    size_t value_len = src_len - key_len - 1;
    buffer_need_bytes(wb, value_len + 1);
    size_t size = copy_replacing_newlines(&wb->buffer[wb->len], value_len + 1, value, value_len, newline);
    wb->len += size;
    buffer_putc(wb, '\n');

    le_size = htole64(size);
    memcpy(length_ptr, &le_size, sizeof(le_size));
}

static void journal_send_buffer(int fd, BUFFER *msg) {
// DEBUGGING:
//
//    CLEAN_BUFFER *tmp = buffer_create(0, NULL);
//
//    for(size_t i = 0; i < msg->len ;i++) {
//        if(isprint(msg->buffer[i]))
//            buffer_putc(tmp, msg->buffer[i]);
//        else {
//            buffer_putc(tmp, '[');
//            buffer_print_uint64_hex(tmp, msg->buffer[i]);
//            buffer_putc(tmp, ']');
//        }
//    }
//
//    fprintf(stderr, "SENDING: %s\n", buffer_tostring(tmp));

    bool ret = journal_direct_send(fd, msg->buffer, msg->len);
    if (!ret)
        fatal("Cannot send message to systemd journal.");
}

int main(int argc, char *argv[]) {
    clocks_init();
    nd_log_initialize_for_external_plugins(argv[0]);

    int timeout_ms = 0;
    bool log_as_netdata = false;
    const char *newline = NULL;
    const char *namespace = NULL;
    const char *socket = getenv("NETDATA_SYSTEMD_JOURNAL_PATH");

    for(int i = 1; i < argc ;i++) {
        const char *k = argv[i];

        if(strcmp(k, "--help") == 0 || strcmp(k, "-h") == 0)
            return help();

        else if(strcmp(k, "--log-as-netdata") == 0 || strcmp(k, "-N") == 0)
            log_as_netdata = true;

        else if(strncmp(k, "--namespace=", 12) == 0)
            namespace = &k[12];

        else if(strncmp(k, "--socket=", 9) == 0)
            socket = &k[9];

        else if(strncmp(k, "--newline=", 10) == 0)
            newline = &k[10];

        else {
            fprintf(stderr, "Unknown parameter '%s'\n", k);
            return 1;
        }
    }

    char path[FILENAME_MAX + 1];
    int fd = -1;

    if(!log_as_netdata) {
        journal_construct_path(path, sizeof(path), socket, namespace);
        fd = journal_direct_fd(path);
        if (fd == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open '%s' as a UNIX socket.", socket);
            return 1;
        }
    }

    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *wb = buffer_create(sizeof(reader.read_buffer), NULL);
    CLEAN_BUFFER *msg = buffer_create(sizeof(reader.read_buffer), NULL);

    ND_LOG_STACK lgs[_NDF_MAX + 1] = { 0 };
    ND_LOG_STACK_PUSH(lgs);
    lgs_reset(lgs);
    size_t fields_added = 0;
    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;

    while(true) {
        if(unlikely(!buffered_reader_next_line(&reader, wb))) {
            if(!buffered_reader_read_timeout(&reader, STDIN_FILENO, timeout_ms, false))
                break;

            continue;
        }

        // make sure the buffer is NULL terminated
        wb->buffer[wb->len] = '\0';

        // remove the newline from the end
        if(likely(wb->len && wb->buffer[wb->len - 1] == '\n'))
            wb->buffer[--wb->len] = '\0';

        if(unlikely(log_as_netdata)) { // unlikely to optimize batch processing
            if (!wb->len) {
                // an empty line - we are done for this message
                nd_log(NDLS_HEALTH, priority, "added %d fields", fields_added);
                lgs_reset(lgs);
                fields_added = 0;
            }
            else {
                char *equal = strchr(wb->buffer, '=');
                if(equal) {
                    const char *field = wb->buffer;
                    size_t field_len = equal - wb->buffer;
                    ND_LOG_FIELD_ID id = nd_log_field_id_by_name(field, field_len);
                    if(id != NDF_STOP) {
                        const char *value = ++equal;

                        if(lgs[id].txt)
                            freez((void *)lgs[id].txt);

                        lgs[id].txt = strdupz_replacing_newlines(value, newline);
                        lgs[id].set = true;

                        fields_added++;

                        if(id == NDF_PRIORITY)
                            priority = nd_log_priority2id(value);
                    }
                    else
                        nd_log(NDLS_COLLECTORS, NDLP_ERR,
                               "Field '%.*s' is not a Netdata field. Ignoring it.",
                               field_len, field);
                }
                else
                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "Line does not contain an = sign; ignoring it: %s", wb->buffer);
            }
        }
        else {
            if (!wb->len) {
                // an empty line - we are done for this message
                if (msg->len)
                    journal_send_buffer(fd, msg);

                buffer_flush(msg);
            }
            else {
                buffer_memcat_replacing_newlines(msg, wb->buffer, wb->len, newline);
            }
        }

        wb->len = 0;
        wb->buffer[0] = '\0';
    }

    // if the last message did not have an empty line, log it

    if(log_as_netdata && fields_added)
        nd_log(NDLS_HEALTH, priority, "added %d fields", fields_added);

    else if (msg && msg->len)
        journal_send_buffer(fd, msg);

    return 0;
}
