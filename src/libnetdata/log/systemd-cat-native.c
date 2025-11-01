// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-cat-native.h"
#include "../required_dummies.h"

#ifdef __FreeBSD__
#include <sys/endian.h>
#endif

#ifdef __APPLE__
#include <machine/endian.h>
#endif

bool verbose = false;

static inline void log_message_to_stderr(BUFFER *msg, const char *scope) {
    CLEAN_BUFFER *tmp = buffer_create(0, NULL);

    for(size_t i = 0; i < msg->len ;i++) {
        if(isprint(msg->buffer[i]))
            buffer_putc(tmp, msg->buffer[i]);
        else {
            buffer_putc(tmp, '[');
            buffer_print_uint64_hex(tmp, msg->buffer[i]);
            buffer_putc(tmp, ']');
        }
    }

    fprintf(stderr, "SENDING %s: %s\n", scope, buffer_tostring(tmp));
}

static inline buffered_reader_ret_t get_next_line(struct buffered_reader *reader, BUFFER *line, int timeout_ms) {
    while(true) {
        if(unlikely(!buffered_reader_next_line(reader, line))) {
            buffered_reader_ret_t ret = buffered_reader_read_timeout(reader, STDIN_FILENO, timeout_ms, verbose);
            if(unlikely(ret != BUFFERED_READER_READ_OK))
                return ret;

            continue;
        }
        else {
            // make sure the buffer is NULL terminated
            line->buffer[line->len] = '\0';

            // remove the trailing newlines
            while(line->len && line->buffer[line->len - 1] == '\n')
                line->buffer[--line->len] = '\0';

            return BUFFERED_READER_READ_OK;
        }
    }
}

static inline size_t copy_replacing_newlines(char *dst, size_t dst_len, const char *src, size_t src_len, const char *newline) {
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

static inline void buffer_memcat_replacing_newlines(BUFFER *wb, const char *src, size_t src_len, const char *newline) {
    if(!src) return;

    const char *equal;
    if(!newline || !*newline || !strstr(src, newline) || !(equal = strchr(src, '='))) {
        buffer_memcat(wb, src, src_len);
        buffer_putc(wb, '\n');
        return;
    }

    size_t key_len = equal - src;
    buffer_memcat(wb, src, key_len);
    buffer_putc(wb, '\n');

    size_t length_offset = wb->len;
    uint64_t le_size = 0;
    buffer_memcat(wb, &le_size, sizeof(le_size));

    const char *value = ++equal;
    size_t value_len = src_len - key_len - 1;
    buffer_need_bytes(wb, value_len + 1);
    size_t size = copy_replacing_newlines(&wb->buffer[wb->len], value_len + 1, value, value_len, newline);
    wb->len += size;
    buffer_putc(wb, '\n');

    le_size = htole64(size);
    memcpy(&wb->buffer[length_offset], &le_size, sizeof(le_size));
}

// ----------------------------------------------------------------------------
// log to a systemd-journal-remote

#ifdef HAVE_LIBCURL
#include <curl/curl.h>

#ifndef HOST_NAME_MAX
#define HOST_NAME_MAX 256
#endif

char global_hostname[HOST_NAME_MAX] = "";
char global_boot_id[UUID_COMPACT_STR_LEN] = "";
char global_machine_id[UUID_COMPACT_STR_LEN] = "";
char global_stream_id[UUID_COMPACT_STR_LEN] = "";
char global_namespace[1024] = "";
char global_systemd_invocation_id[1024] = "";
#define BOOT_ID_PATH "/proc/sys/kernel/random/boot_id"
#define MACHINE_ID_PATH "/etc/machine-id"

#define DEFAULT_PRIVATE_KEY "/etc/ssl/private/journal-upload.pem"
#define DEFAULT_PUBLIC_KEY "/etc/ssl/certs/journal-upload.pem"
#define DEFAULT_CA_CERT "/etc/ssl/ca/trusted.pem"

struct upload_data {
    char *data;
    size_t length;
};

static size_t systemd_journal_remote_read_callback(void *ptr, size_t size, size_t nmemb, void *userp) {
    struct upload_data *upload = (struct upload_data *)userp;
    size_t buffer_size = size * nmemb;

    if (upload->length) {
        size_t copy_size = upload->length < buffer_size ? upload->length : buffer_size;
        memcpy(ptr, upload->data, copy_size);
        upload->data += copy_size;
        upload->length -= copy_size;
        return copy_size;
    }

    return 0;
}

CURL* initialize_connection_to_systemd_journal_remote(const char* url, const char* private_key, const char* public_key, const char* ca_cert, struct curl_slist **headers) {
    CURL *curl = curl_easy_init();
    if (!curl) {
        fprintf(stderr, "Failed to initialize curl\n");
        return NULL;
    }

    *headers = curl_slist_append(*headers, "Content-Type: application/vnd.fdo.journal");
    *headers = curl_slist_append(*headers, "Transfer-Encoding: chunked");
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, *headers);
    curl_easy_setopt(curl, CURLOPT_URL, url);
    curl_easy_setopt(curl, CURLOPT_POST, 1L);
    curl_easy_setopt(curl, CURLOPT_READFUNCTION, systemd_journal_remote_read_callback);

    if (strncmp(url, "https://", 8) == 0) {
        if (private_key) curl_easy_setopt(curl, CURLOPT_SSLKEY, private_key);
        if (public_key) curl_easy_setopt(curl, CURLOPT_SSLCERT, public_key);

        if (strcmp(ca_cert, "all") != 0) {
            curl_easy_setopt(curl, CURLOPT_CAINFO, ca_cert);
        } else {
            curl_easy_setopt(curl, CURLOPT_SSL_VERIFYPEER, 0L);
        }
    }
    // curl_easy_setopt(curl, CURLOPT_VERBOSE, 1L); // Remove for less verbose output

    return curl;
}

static void journal_remote_complete_event(BUFFER *msg, usec_t *monotonic_ut) {
    usec_t ut = now_monotonic_usec();

    if(monotonic_ut)
        *monotonic_ut = ut;

    buffer_sprintf(msg,
                   ""
                   "__REALTIME_TIMESTAMP=%"PRIu64"\n"
                   "__MONOTONIC_TIMESTAMP=%"PRIu64"\n"
                   "_MACHINE_ID=%s\n"
                   "_BOOT_ID=%s\n"
                   "_HOSTNAME=%s\n"
                   "_TRANSPORT=stdout\n"
                   "_LINE_BREAK=nul\n"
                   "_STREAM_ID=%s\n"
                   "_RUNTIME_SCOPE=system\n"
                   "%s%s\n"
                   , now_realtime_usec()
                   , ut
                   , global_machine_id
                   , global_boot_id
                   , global_hostname
                   , global_stream_id
                   , global_namespace
                   , global_systemd_invocation_id
                  );
}

static CURLcode journal_remote_send_buffer(CURL* curl, BUFFER *msg) {

    if(verbose)
        log_message_to_stderr(msg, "REMOTE");

    if (!curl || !buffer_strlen(msg))
        return CURLE_FAILED_INIT;

    struct upload_data upload = {
        .data = (char *) buffer_tostring(msg),
        .length = buffer_strlen(msg),
    };

    curl_easy_setopt(curl, CURLOPT_READDATA, &upload);
    curl_easy_setopt(curl, CURLOPT_INFILESIZE_LARGE, (curl_off_t)upload.length);

    return curl_easy_perform(curl);
}

typedef enum {
    LOG_TO_JOURNAL_REMOTE_BAD_PARAMS = -1,
    LOG_TO_JOURNAL_REMOTE_CANNOT_INITIALIZE = -2,
    LOG_TO_JOURNAL_REMOTE_CANNOT_SEND = -3,
    LOG_TO_JOURNAL_REMOTE_CANNOT_READ = -4,
} log_to_journal_remote_ret_t;

static log_to_journal_remote_ret_t log_input_to_journal_remote(const char *url, const char *key, const char *cert, const char *trust, const char *newline, int timeout_ms) {
    if(!url || !*url) {
        fprintf(stderr, "No URL is given.\n");
        return LOG_TO_JOURNAL_REMOTE_BAD_PARAMS;
    }

    if(timeout_ms < 10)
        timeout_ms = 10;

    global_boot_id[0] = '\0';
    char buffer[1024];
    if(read_txt_file(BOOT_ID_PATH, buffer, sizeof(buffer)) == 0) {
        nd_uuid_t uuid;
        if(uuid_parse_flexi(buffer, uuid) == 0)
            uuid_unparse_lower_compact(uuid, global_boot_id);
        else
            fprintf(stderr, "WARNING: cannot parse the UUID found in '%s'.\n", BOOT_ID_PATH);
    }

    if(global_boot_id[0] == '\0') {
        fprintf(stderr, "WARNING: cannot read '%s'. Will generate a random _BOOT_ID.\n", BOOT_ID_PATH);
        nd_uuid_t uuid;
        uuid_generate_random(uuid);
        uuid_unparse_lower_compact(uuid, global_boot_id);
    }

    if(read_txt_file(MACHINE_ID_PATH, buffer, sizeof(buffer)) == 0) {
        nd_uuid_t uuid;
        if(uuid_parse_flexi(buffer, uuid) == 0)
            uuid_unparse_lower_compact(uuid, global_machine_id);
        else
            fprintf(stderr, "WARNING: cannot parse the UUID found in '%s'.\n", MACHINE_ID_PATH);
    }

    if(global_machine_id[0] == '\0') {
        fprintf(stderr, "WARNING: cannot read '%s'. Will generate a random _MACHINE_ID.\n", MACHINE_ID_PATH);
        nd_uuid_t uuid;
        uuid_generate_random(uuid);
        uuid_unparse_lower_compact(uuid, global_machine_id);
    }

    if(global_stream_id[0] == '\0') {
        nd_uuid_t uuid;
        uuid_generate_random(uuid);
        uuid_unparse_lower_compact(uuid, global_stream_id);
    }

    if(global_hostname[0] == '\0') {
        if(gethostname(global_hostname, sizeof(global_hostname)) != 0) {
            fprintf(stderr, "WARNING: cannot get system's hostname. Will use internal default.\n");
            snprintfz(global_hostname, sizeof(global_hostname), "systemd-cat-native-unknown-hostname");
        }
    }

    if(global_systemd_invocation_id[0] == '\0' && getenv("INVOCATION_ID"))
        snprintfz(global_systemd_invocation_id, sizeof(global_systemd_invocation_id), "_SYSTEMD_INVOCATION_ID=%s\n", getenv("INVOCATION_ID"));

    if(!key)
        key = DEFAULT_PRIVATE_KEY;

    if(!cert)
        cert = DEFAULT_PUBLIC_KEY;

    if(!trust)
        trust = DEFAULT_CA_CERT;

    char full_url[4096];
    snprintfz(full_url, sizeof(full_url), "%s/upload", url);

    CURL *curl;
    CURLcode res = CURLE_OK;
    struct curl_slist *headers = NULL;

    curl_global_init(CURL_GLOBAL_ALL);
    curl = initialize_connection_to_systemd_journal_remote(full_url, key, cert, trust, &headers);

    if(!curl)
        return LOG_TO_JOURNAL_REMOTE_CANNOT_INITIALIZE;

    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *line = buffer_create(sizeof(reader.read_buffer), NULL);
    CLEAN_BUFFER *msg = buffer_create(sizeof(reader.read_buffer), NULL);

    size_t msg_full_events = 0;
    size_t msg_partial_fields = 0;
    usec_t msg_started_ut = 0;
    size_t failures = 0;
    size_t messages_logged = 0;

    log_to_journal_remote_ret_t ret = 0;

    while(true) {
        buffered_reader_ret_t rc = get_next_line(&reader, line, timeout_ms);
        if(rc == BUFFERED_READER_READ_POLL_TIMEOUT) {
            if(msg_full_events && !msg_partial_fields) {
                res = journal_remote_send_buffer(curl, msg);
                if(res != CURLE_OK) {
                    fprintf(stderr, "journal_remote_send_buffer() failed: %s\n", curl_easy_strerror(res));
                    failures++;
                    ret = LOG_TO_JOURNAL_REMOTE_CANNOT_SEND;
                    goto cleanup;
                }
                else
                    messages_logged++;

                msg_full_events = 0;
                buffer_flush(msg);
            }
        }
        else if(rc == BUFFERED_READER_READ_OK) {
            if(!line->len) {
                // an empty line - we are done for this message
                if(msg_partial_fields) {
                    msg_partial_fields = 0;

                    usec_t ut;
                    journal_remote_complete_event(msg, &ut);
                    if(!msg_full_events)
                        msg_started_ut = ut;

                    msg_full_events++;

                    if(ut - msg_started_ut >= USEC_PER_SEC / 2) {
                        res = journal_remote_send_buffer(curl, msg);
                        if(res != CURLE_OK) {
                            fprintf(stderr, "journal_remote_send_buffer() failed: %s\n", curl_easy_strerror(res));
                            failures++;
                            ret = LOG_TO_JOURNAL_REMOTE_CANNOT_SEND;
                            goto cleanup;
                        }
                        else
                            messages_logged++;

                        msg_full_events = 0;
                        buffer_flush(msg);
                    }
                }
            }
            else {
                buffer_memcat_replacing_newlines(msg, line->buffer, line->len, newline);
                msg_partial_fields++;
            }

            buffer_flush(line);
        }
        else {
            fprintf(stderr, "cannot read input data, failed with code %d\n", rc);
            ret = LOG_TO_JOURNAL_REMOTE_CANNOT_READ;
            break;
        }
    }

    if (msg_full_events || msg_partial_fields) {
        if(msg_partial_fields) {
            msg_partial_fields = 0;
            msg_full_events++;
            journal_remote_complete_event(msg, NULL);
        }

        if(msg_full_events) {
            res = journal_remote_send_buffer(curl, msg);
            if(res != CURLE_OK) {
                fprintf(stderr, "journal_remote_send_buffer() failed: %s\n", curl_easy_strerror(res));
                failures++;
            }
            else
                messages_logged++;

            msg_full_events = 0;
            buffer_flush(msg);
        }
    }

cleanup:
    curl_easy_cleanup(curl);
    curl_slist_free_all(headers);
    curl_global_cleanup();

    return ret;
}

#endif

static int help(void) {
    fprintf(stderr,
            "\n"
            "Netdata systemd-cat-native " NETDATA_VERSION "\n"
            "\n"
            "This program reads from its standard input, lines in the format:\n"
            "\n"
            "KEY1=VALUE1\\n\n"
            "KEY2=VALUE2\\n\n"
            "KEYN=VALUEN\\n\n"
            "\\n\n"
            "\n"
            "and sends them to systemd-journal.\n"
            "\n"
            "   - Binary journal fields are not accepted at its input\n"
            "   - Binary journal fields can be generated after newline processing\n"
            "   - Messages have to be separated by an empty line\n"
            "   - Keys starting with underscore are not accepted (by journald)\n"
            "   - Other rules imposed by systemd-journald are imposed (by journald)\n"
            "\n"
            "Usage:\n"
            "\n"
            "   %s\n"
            "          [--verbose|-v]\n"
            "          [--newline=STRING]\n"
            "          [--log-as-netdata|-N]\n"
            "          [--namespace=NAMESPACE] [--socket=PATH]\n"
#ifdef HAVE_LIBCURL
            "          [--url=URL [--key=FILENAME] [--cert=FILENAME] [--trust=FILENAME|all]]\n"
#endif
            "\n"
            "The program has the following modes of logging:\n"
            "\n"
            "  * Log to a local systemd-journald or stderr\n"
            "\n"
            "    This is the default mode. If systemd-journald is available, logs will be\n"
            "    sent to systemd, otherwise logs will be printed on stderr, using logfmt\n"
            "    formatting. Options --socket and --namespace are available to configure\n"
            "    the journal destination:\n"
            "\n"
            "      --socket=PATH\n"
            "        The path of a systemd-journald UNIX socket.\n"
            "        The program will use the default systemd-journald socket when this\n"
            "        option is not used.\n"
            "\n"
            "      --namespace=NAMESPACE\n"
            "        The name of a configured and running systemd-journald namespace.\n"
            "        The program will produce the socket path based on its internal\n"
            "        defaults, to send the messages to the systemd journal namespace.\n"
            "\n"
            "  * Log as Netdata, enabled with --log-as-netdata or -N\n"
            "\n"
            "    In this mode the program uses environment variables set by Netdata for\n"
            "    the log destination. Only log fields defined by Netdata are accepted.\n"
            "    If the environment variables expected by Netdata are not found, it\n"
            "    falls back to stderr logging in logfmt format.\n"
#ifdef HAVE_LIBCURL
            "\n"
            "  * Log to a systemd-journal-remote TCP socket, enabled with --url=URL\n"
            "\n"
            "    In this mode, the program will directly sent logs to a remote systemd\n"
            "    journal (systemd-journal-remote expected at the destination)\n"
            "    This mode is available even when the local system does not support\n"
            "    systemd, or even it is not Linux, allowing a remote Linux systemd\n"
            "    journald to become the logs database of the local system.\n"
            "\n"
            "    Unfortunately systemd-journal-remote does not accept compressed\n"
            "    data over the network, so the stream will be uncompressed.\n"
            "\n"
            "      --url=URL\n"
            "        The destination systemd-journal-remote address and port, similarly\n"
            "        to what /etc/systemd/journal-upload.conf accepts.\n"
            "        Usually it is in the form: https://ip.address:19532\n"
            "        Both http and https URLs are accepted. When using https, the\n"
            "        following additional options are accepted:\n"
            "\n"
            "      --key=FILENAME\n"
            "        The filename of the private key of the server.\n"
            "        The default is: " DEFAULT_PRIVATE_KEY "\n"
            "\n"
            "      --cert=FILENAME\n"
            "        The filename of the public key of the server.\n"
            "        The default is: " DEFAULT_PUBLIC_KEY "\n"
            "\n"
            "      --trust=FILENAME | all\n"
            "        The filename of the trusted CA public key.\n"
            "        The default is: " DEFAULT_CA_CERT "\n"
            "        The keyword 'all' can be used to trust all CAs.\n"
            "\n"
            "      --namespace=NAMESPACE\n"
            "        Set the namespace of the messages sent.\n"
            "\n"
            "      --keep-trying\n"
            "        Keep trying to send the message, if the remote journal is not there.\n"
#endif
            "\n"
            "    NEWLINES PROCESSING\n"
            "    systemd-journal logs entries may have newlines in them. However the\n"
            "    Journal Export Format uses binary formatted data to achieve this,\n"
            "    making it hard for text processing.\n"
            "\n"
            "    To overcome this limitation, this program allows single-line text\n"
            "    formatted values at its input, to be binary formatted multi-line Journal\n"
            "    Export Format at its output.\n"
            "\n"
            "    To achieve that it allows replacing a given string to a newline.\n"
            "    The parameter --newline=STRING allows setting the string to be replaced\n"
            "    with newlines.\n"
            "\n"
            "    With the default setting of --newline='\\n', the program will replace\n"
            "    all occurrences of \\n with the newline character, within each\n"
            "    VALUE of the KEY=VALUE lines. Once this this done, the program will\n"
            "    switch the field to the binary Journal Export Format before sending the\n"
            "    log event to systemd-journal.\n"
            "\n",
            program_name);

    return 1;
}

// ----------------------------------------------------------------------------
// log as Netdata

static void lgs_reset(struct log_stack_entry *lgs) {
    for(size_t i = 1; i < _NDF_MAX ;i++) {
        if(lgs[i].type == NDFT_TXT && lgs[i].set && lgs[i].txt)
            freez((void *)lgs[i].txt);

        lgs[i] = ND_LOG_FIELD_TXT(i, NULL);
    }

    lgs[0] = ND_LOG_FIELD_TXT(NDF_MESSAGE, NULL);
    lgs[_NDF_MAX] = ND_LOG_FIELD_END();
}

static const char *strdupz_replacing_newlines(const char *src, const char *newline) {
    if(!src) src = "";

    size_t src_len = strlen(src);
    char *buffer = mallocz(src_len + 1);
    copy_replacing_newlines(buffer, src_len + 1, src, src_len, newline);
    return buffer;
}

static int log_input_as_netdata(const char *newline, int timeout_ms) {
    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *line = buffer_create(sizeof(reader.read_buffer), NULL);

    ND_LOG_STACK lgs[_NDF_MAX + 1] = { 0 };
    ND_LOG_STACK_PUSH(lgs);
    lgs_reset(lgs);

    ND_LOG_SOURCES source = NDLS_HEALTH;
    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;
    size_t fields_added = 0;
    size_t messages_logged = 0;

    while(get_next_line(&reader, line, timeout_ms) == BUFFERED_READER_READ_OK) {
        if(!line->len) {
            // an empty line - we are done for this message

            nd_log(source, priority,
                   "added %zu fields", // if the user supplied a MESSAGE, this will be ignored
                   fields_added);

            lgs_reset(lgs);
            fields_added = 0;
            messages_logged++;
        }
        else {
            char *equal = strchr(line->buffer, '=');
            if(equal) {
                const char *field = line->buffer;
                size_t field_len = equal - line->buffer;
                ND_LOG_FIELD_ID id = nd_log_field_id_by_journal_name(field, field_len);
                if(id != NDF_STOP) {
                    const char *value = ++equal;

                    if(lgs[id].txt)
                        freez((void *) lgs[id].txt);

                    lgs[id].txt = strdupz_replacing_newlines(value, newline);
                    lgs[id].set = true;

                    fields_added++;

                    if(id == NDF_PRIORITY)
                        priority = nd_log_priority2id(value);
                }
                else {
                    struct log_stack_entry backup = lgs[NDF_MESSAGE];
                    lgs[NDF_MESSAGE] = ND_LOG_FIELD_TXT(NDF_MESSAGE, NULL);

                    nd_log(source, NDLP_ERR,
                           "Field '%.*s' is not a Netdata field. Ignoring it.",
                           (int)field_len, field);

                    lgs[NDF_MESSAGE] = backup;
                }
            }
            else {
                struct log_stack_entry backup = lgs[NDF_MESSAGE];
                lgs[NDF_MESSAGE] = ND_LOG_FIELD_TXT(NDF_MESSAGE, NULL);

                nd_log(source, NDLP_ERR,
                       "Line does not contain an = sign; ignoring it: %s",
                       line->buffer);

                lgs[NDF_MESSAGE] = backup;
            }
        }

        buffer_flush(line);
    }

    if(fields_added) {
        nd_log(source, priority, "added %zu fields", fields_added);
        messages_logged++;
    }

    return messages_logged ? 0 : 1;
}

// ----------------------------------------------------------------------------
// log to a local systemd-journald

static bool journal_local_send_buffer(int fd, BUFFER *msg) {
    if(verbose)
        log_message_to_stderr(msg, "LOCAL");

    bool ret = journal_direct_send(fd, msg->buffer, msg->len);
    if (!ret)
        fprintf(stderr, "Cannot send message to systemd journal.\n");

    return ret;
}

static int log_input_to_journal(const char *socket, const char *namespace, const char *newline, int timeout_ms) {
    char path[FILENAME_MAX + 1];
    int fd = -1;

    if(socket)
        snprintfz(path, sizeof(path), "%s", socket);
    else
        journal_construct_path(path, sizeof(path), NULL, namespace);

    fd = journal_direct_fd(path);
    if (fd == -1) {
        fprintf(stderr, "Cannot open '%s' as a UNIX socket (errno = %d)\n",
                path, errno);
        return 1;
    }

    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *line = buffer_create(sizeof(reader.read_buffer), NULL);
    CLEAN_BUFFER *msg = buffer_create(sizeof(reader.read_buffer), NULL);

    size_t messages_logged = 0;
    size_t failed_messages = 0;

    while(get_next_line(&reader, line, timeout_ms) == BUFFERED_READER_READ_OK) {
        if (!line->len) {
            // an empty line - we are done for this message
            if (msg->len) {
                if(journal_local_send_buffer(fd, msg))
                    messages_logged++;
                else {
                    failed_messages++;
                    goto cleanup;
                }
            }

            buffer_flush(msg);
        }
        else
            buffer_memcat_replacing_newlines(msg, line->buffer, line->len, newline);

        buffer_flush(line);
    }

    if (msg && msg->len) {
        if(journal_local_send_buffer(fd, msg))
            messages_logged++;
        else
            failed_messages++;
    }

cleanup:
    if(verbose) {
        if(failed_messages)
            fprintf(stderr, "%zu messages failed to be logged\n", failed_messages);
        if(!messages_logged)
            fprintf(stderr, "No messages were logged!\n");
    }

    return !failed_messages && messages_logged ? 0 : 1;
}

int main(int argc, char *argv[]) {
    nd_log_initialize_for_external_plugins(argv[0]);

    int timeout_ms = 0; // wait forever
    bool log_as_netdata = false;
    const char *newline = "\\n";
    const char *namespace = NULL;
    const char *socket = getenv("NETDATA_SYSTEMD_JOURNAL_PATH");
#ifdef HAVE_LIBCURL
    const char *url = NULL;
    const char *key = NULL;
    const char *cert = NULL;
    const char *trust = NULL;
    bool keep_trying = false;
#endif

    for(int i = 1; i < argc ;i++) {
        const char *k = argv[i];

        if(strcmp(k, "--help") == 0 || strcmp(k, "-h") == 0)
            return help();

        else if(strcmp(k, "--verbose") == 0 || strcmp(k, "-v") == 0)
            verbose = true;

        else if(strcmp(k, "--log-as-netdata") == 0 || strcmp(k, "-N") == 0)
            log_as_netdata = true;

        else if(strncmp(k, "--namespace=", 12) == 0)
            namespace = &k[12];

        else if(strncmp(k, "--socket=", 9) == 0)
            socket = &k[9];

        else if(strncmp(k, "--newline=", 10) == 0)
            newline = &k[10];

#ifdef HAVE_LIBCURL
        else if (strncmp(k, "--url=", 6) == 0)
            url = &k[6];

        else if (strncmp(k, "--key=", 6) == 0)
            key = &k[6];

        else if (strncmp(k, "--cert=", 7) == 0)
            cert = &k[7];

        else if (strncmp(k, "--trust=", 8) == 0)
            trust = &k[8];

        else if (strcmp(k, "--keep-trying") == 0)
            keep_trying = true;
#endif
        else {
            fprintf(stderr, "Unknown parameter '%s'\n", k);
            return 1;
        }
    }

#ifdef HAVE_LIBCURL
    if(log_as_netdata && url) {
        fprintf(stderr, "Cannot log to a systemd-journal-remote URL as Netdata. "
                        "Please either give --url or --log-as-netdata, not both.\n");
        return 1;
    }

    if(socket && url) {
        fprintf(stderr, "Cannot log to a systemd-journal-remote URL using a UNIX socket. "
                        "Please either give --url or --socket, not both.\n");
        return 1;
    }

#endif

    if(log_as_netdata && namespace) {
        fprintf(stderr, "Cannot log as netdata using a namespace. "
                        "Please either give --log-as-netdata or --namespace, not both.\n");
        return 1;
    }

    if(log_as_netdata)
        return log_input_as_netdata(newline, timeout_ms);

#ifdef HAVE_LIBCURL
    if(url) {
        if(url && namespace && *namespace)
            snprintfz(global_namespace, sizeof(global_namespace), "_NAMESPACE=%s\n", namespace);

        log_to_journal_remote_ret_t rc;
        do {
            rc = log_input_to_journal_remote(url, key, cert, trust, newline, timeout_ms);
        } while(keep_trying && rc == LOG_TO_JOURNAL_REMOTE_CANNOT_SEND);
    }
#endif

    return log_input_to_journal(socket, namespace, newline, timeout_ms);
}
