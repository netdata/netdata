// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-cat-native.h"
#include "../required_dummies.h"

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
            "   %s [--namespace=NAMESPACE] [--socket=PATH] [--log-as-netdata|-N]\n"
            "\n"
            "The default namespace and socket depends on whether the program is started by Netdata.\n"
            "When it is started by Netdata, it inherits whatever settings Netdata has.\n"
            "When it is started by other programs, it uses the default namespace and the default\n"
            "systemd-journald socket.\n"
            "\n"
            "--log-as-netdata, means to log the received messages the same way Netdata does\n"
            "(using the same log output and format as the Netdata daemon in its process tree).\n"
            "\n"
            "When --log-as-netdata is not specified, entries are sent directly to systemd-journald\n"
            "without any kind of processing.\n"
            "\n",
            program_name);

    return 1;
}

static void lgs_reset(struct log_stack_entry *lgs) {
    for(size_t i = 0; i < _NDF_MAX ;i++) {
        if(lgs[i].type == NDFT_TXT && lgs[i].txt)
            freez((void *)lgs[i].txt);

        lgs[i] = ND_LOG_FIELD_TXT(i, NULL);
    }

    lgs[_NDF_MAX] = ND_LOG_FIELD_END();
}

int main(int argc, char *argv[]) {
    clocks_init();

    // if we don't run under Netdata, log to stderr,
    // otherwise, use the logging method Netdata wants us to use.
    setenv("NETDATA_LOG_METHOD", "stderr", 0);

    nd_log_initialize_for_external_plugins(argv[0]);

    int timeout_ms = 0;
    bool log_as_netdata = false;
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

    ND_LOG_STACK lgs[_NDF_MAX + 1] = { 0 };
    lgs_reset(lgs);

    while(true) {
        if(unlikely(!buffered_reader_next_line(&reader, wb))) {
            if(!buffered_reader_read_timeout(&reader, STDIN_FILENO, timeout_ms))
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
                nd_log(NDLS_COLLECTORS, NDLP_INFO, NULL);
                lgs_reset(lgs);
            }
            else {
                char *equal = strchr(wb->buffer, '=');
                if(equal) {
                    const char *field = wb->buffer;
                    size_t field_len = equal - wb->buffer;
                    int id = nd_log_field_id_by_name(field, field_len);
                    if(id != NDF_STOP) {
                        const char *value = ++equal;

                        if(lgs[id].txt)
                            freez((void *)lgs[id].txt);

                        lgs[id].txt = strdupz(value);
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
                if (msg->len) {
                    bool ret = journal_direct_send(fd, msg->buffer, msg->len);
                    if (!ret)
                        fatal("Cannot send message to systemd journal.");
                }
                buffer_flush(msg);
            }
            else {
                buffer_memcat(msg, wb->buffer, wb->len);
            }
        }

        wb->len = 0;
        wb->buffer[0] = '\0';
    }

    return 0;
}
