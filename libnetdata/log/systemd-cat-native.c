// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-cat-native.h"
#include "../required_dummies.h"

static int help(void) {
    fprintf(stderr,
            "Netdata systemd-cat-native\n"
            "\n"
            " Usage:\n"
            "\n"
            "   %s [--namespace=NAMESPACE] [--socket=PATH]\n"
            "\n",
            program_name);

    return 1;
}

int main(int argc, char *argv[]) {
    clocks_init();
    setenv("NETDATA_LOG_METHOD", "stderr", 0);

    nd_log_initialize_for_external_plugins(argv[0]);

    int timeout_ms = 0;
    const char *namespace = NULL;
    const char *socket = getenv("NETDATA_SYSTEMD_JOURNAL_PATH");

    for(int i = 1; i < argc ;i++) {
        const char *k = argv[i];

        if(strcmp(k, "--help") == 0 || strcmp(k, "-h") == 0)
            return help();

        else if(strncmp(k, "--namespace=", 12) == 0)
            namespace = &k[12];

        else if(strncmp(k, "--socket=", 9) == 0)
            socket = &k[9];

        else {
            fprintf(stderr, "Unknown parameter '%s'\n", k);
            return 1;
        }
    }

    char path[FILENAME_MAX + 1];
    journal_construct_path(path, sizeof(path), socket, namespace);
    int fd = journal_direct_fd(path);
    if(fd == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open '%s' as a UNIX socket.", socket);
        return 1;
    }

    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *wb = buffer_create(sizeof(reader.read_buffer), NULL);
    CLEAN_BUFFER *msg = buffer_create(sizeof(reader.read_buffer), NULL);

    while(true) {
        if(unlikely(!buffered_reader_next_line(&reader, wb))) {
            if(!buffered_reader_read_timeout(&reader, STDIN_FILENO, timeout_ms))
                break;

            continue;
        }

        if(wb->buffer[0] == '\n' && wb->len == 1) {
            // an empty line - we are done for this message
            if(msg->len) {
                bool ret = journal_direct_send(fd, msg->buffer, msg->len);
                if(!ret)
                    fatal("Cannot send message to systemd journal.");
            }
            buffer_flush(msg);
        }
        else {
            buffer_memcat(msg, wb->buffer, wb->len);
        }

        wb->len = 0;
        wb->buffer[0] = '\0';
    }

    return 0;
}
