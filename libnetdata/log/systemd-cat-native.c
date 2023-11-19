// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-cat-native.h"
#include "../required_dummies.h"

#ifdef __FreeBSD__
#include <sys/endian.h>
#endif

#ifdef __APPLE__
#include <machine/endian.h>
#endif

static void log_message_to_stderr(BUFFER *msg) {
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

    fprintf(stderr, "SENDING: %s\n", buffer_tostring(tmp));
}

static inline bool get_next_line(struct buffered_reader *reader, BUFFER *line, int timeout_ms) {
    buffer_flush(line);

    while(true) {
        if(unlikely(!buffered_reader_next_line(reader, line))) {
            if(!buffered_reader_read_timeout(reader, STDIN_FILENO, timeout_ms, false))
                return false;

            continue;
        }
        else {
            // make sure the buffer is NULL terminated
            line->buffer[line->len] = '\0';

            // remove the trailing newlines
            while(line->len && line->buffer[line->len - 1] == '\n')
                line->buffer[--line->len] = '\0';

            return true;
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

// ----------------------------------------------------------------------------
// log to a systemd-journal-remote

#ifdef HAVE_CURL
#include <curl/curl.h>

#ifndef HOST_NAME_MAX
#define HOST_NAME_MAX 256
#endif

char global_hostname[HOST_NAME_MAX] = "";
char global_boot_id[UUID_COMPACT_STR_LEN] = "";
#define BOOT_ID_PATH "/proc/sys/kernel/random/boot_id"

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

CURLcode journal_remote_send_buffer(CURL* curl, BUFFER *msg) {
    buffer_sprintf(msg,
                   ""
                   "__REALTIME_TIMESTAMP=%llu\n"
                   "__MONOTONIC_TIMESTAMP=%llu\n"
                   "_BOOT_ID=%s\n"
                   "_HOSTNAME=%s\n"
                   "\n"
                   , now_realtime_usec()
                   , now_monotonic_usec()
                   , global_boot_id
                   , global_hostname
                   );

    // log_message_to_stderr(msg);

    struct upload_data upload = {0};

    if (!curl || !buffer_strlen(msg))
        return CURLE_FAILED_INIT;

    upload.data = (char *) buffer_tostring(msg);
    upload.length = buffer_strlen(msg);

    curl_easy_setopt(curl, CURLOPT_READDATA, &upload);
    curl_easy_setopt(curl, CURLOPT_INFILESIZE_LARGE, (curl_off_t)upload.length);

    return curl_easy_perform(curl);
}

static int log_input_to_journal_remote(const char *url, const char *key, const char *cert, const char *trust, const char *newline, int timeout_ms) {
    if(!url || !*url) {
        fprintf(stderr, "No URL is given.\n");
        return -1;
    }

    global_boot_id[0] = '\0';
    char boot_id[1024];
    if(read_file(BOOT_ID_PATH, boot_id, sizeof(boot_id)) == 0) {
        uuid_t uuid;
        if(uuid_parse_flexi(boot_id, uuid) == 0)
            uuid_unparse_lower_compact(uuid, global_boot_id);
        else
            fprintf(stderr, "WARNING: cannot parse the UUID found in '%s'.\n", BOOT_ID_PATH);
    }

    if(global_boot_id[0] == '\0') {
        fprintf(stderr, "WARNING: cannot read '%s'. Will generate a random _BOOT_ID.\n", BOOT_ID_PATH);
        uuid_t uuid;
        uuid_generate_random(uuid);
        uuid_unparse_lower_compact(uuid, global_boot_id);
    }

    if(global_hostname[0] == '\0') {
        if(gethostname(global_hostname, sizeof(global_hostname)) != 0) {
            fprintf(stderr, "WARNING: cannot get system's hostname. Will use internal default.\n");
            snprintfz(global_hostname, sizeof(global_hostname), "systemd-cat-native-unknown-hostname");
        }
    }

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
        return 1;

    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *line = buffer_create(sizeof(reader.read_buffer), NULL);
    CLEAN_BUFFER *msg = buffer_create(sizeof(reader.read_buffer), NULL);

    size_t failures = 0;
    size_t messages_logged = 0;
    while(get_next_line(&reader, line, timeout_ms)) {
        if (!line->len) {
            // an empty line - we are done for this message
            if (msg->len) {
                res = journal_remote_send_buffer(curl, msg);
                if (res != CURLE_OK) {
                    fprintf(stderr, "journal_remote_send_buffer() failed: %s\n", curl_easy_strerror(res));
                    failures++;
                    goto cleanup;
                }
                else
                    messages_logged++;
            }

            buffer_flush(msg);
        }
        else
            buffer_memcat_replacing_newlines(msg, line->buffer, line->len, newline);
    }

    if (msg->len) {
        res = journal_remote_send_buffer(curl, msg);
        if (res != CURLE_OK) {
            fprintf(stderr, "journal_remote_send_buffer() failed: %s\n", curl_easy_strerror(res));
            failures++;
        }
        else
            messages_logged++;
    }

cleanup:
    curl_easy_cleanup(curl);
    curl_slist_free_all(headers);
    curl_global_cleanup();

    return !failures && messages_logged ? 0 : 1;
}

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
            "and sends them to systemd-journal.\n"
            "\n"
            "   - Binary fields are not accepted, but are generated after newline processing\n"
            "   - Messages have to be separated by an empty line\n"
            "   - Keys starting with underscore are not accepted (by journald)\n"
            "\n"
            "Usage:\n"
            "\n"
            "   %s [--newline=STRING]\n"
            "          [--log-as-netdata|-N]\n"
            "          [--namespace=NAMESPACE] [--socket=PATH]\n"
#ifdef HAVE_CURL
            "          [--url=URL [--key=FILENAME] [--cert=FILENAME] [--trust=FILENAME|all]]\n"
#endif
            "\n"
            "The program has the following modes of logging:\n"
            "\n"
            "  * log as Netdata (it uses environment variables set by Netdata for the log destination)\n"
            "  * log to local systemd-journald (use --socket and --namespace to configure destination)\n"
#ifdef HAVE_CURL
            "  * log to a remote systemd-journal-remote (use --url to enable)\n"
#endif
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
            "multiline logs to systemd-journal. So, by passing --newline=\"{NEWLINE}\", it will\n"
            "replace all occurrences of {NEWLINE} with \\n and use the binary form of the journal\n"
            "export format for the field.\n"
#ifdef HAVE_CURL
            "\n"
            "When logging to systemd-journal-remote, the defaults are:\n"
            "\n"
            "  --key=" DEFAULT_PRIVATE_KEY "\n"
            "  --cert=" DEFAULT_PUBLIC_KEY "\n"
            "  --trust=" DEFAULT_CA_CERT "\n"
#endif
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

    size_t fields_added = 0;
    size_t messages_logged = 0;
    ND_LOG_FIELD_PRIORITY priority = NDLP_INFO;

    while(get_next_line(&reader, line, timeout_ms)) {
        if(!line->len) {
            // an empty line - we are done for this message

            nd_log(NDLS_HEALTH, priority,
                   "added %d fields", // if the user supplied a MESSAGE, this will be ignored
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
                ND_LOG_FIELD_ID id = nd_log_field_id_by_name(field, field_len);
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

                    nd_log(NDLS_COLLECTORS, NDLP_ERR,
                           "Field '%.*s' is not a Netdata field. Ignoring it.",
                           field_len, field);

                    lgs[NDF_MESSAGE] = backup;
                }
            }
            else {
                struct log_stack_entry backup = lgs[NDF_MESSAGE];
                lgs[NDF_MESSAGE] = ND_LOG_FIELD_TXT(NDF_MESSAGE, NULL);

                nd_log(NDLS_COLLECTORS, NDLP_ERR,
                       "Line does not contain an = sign; ignoring it: %s",
                       line->buffer);

                lgs[NDF_MESSAGE] = backup;
            }
        }
    }

    if(fields_added) {
        nd_log(NDLS_HEALTH, priority, "added %d fields", fields_added);
        messages_logged++;
    }

    return messages_logged ? 0 : 1;
}

// ----------------------------------------------------------------------------
// log to a local systemd-journald

static bool journal_local_send_buffer(int fd, BUFFER *msg) {
    // log_message_to_stderr(msg);

    bool ret = journal_direct_send(fd, msg->buffer, msg->len);
    if (!ret)
        fprintf(stderr, "Cannot send message to systemd journal.\n");

    return ret;
}

static int log_input_to_journal(const char *socket, const char *namespace, const char *newline, int timeout_ms) {
    char path[FILENAME_MAX + 1];
    int fd = -1;

    if(socket)
        snprintfz(path, sizeof(path), socket);
    else
        journal_construct_path(path, sizeof(path), NULL, namespace);

    fd = journal_direct_fd(path);
    if (fd == -1) {
        fprintf(stderr, "Cannot open '%s' as a UNIX socket.\n", path);
        return 1;
    }

    struct buffered_reader reader;
    buffered_reader_init(&reader);
    CLEAN_BUFFER *line = buffer_create(sizeof(reader.read_buffer), NULL);
    CLEAN_BUFFER *msg = buffer_create(sizeof(reader.read_buffer), NULL);

    size_t messages_logged = 0;
    size_t failed_messages = 0;

    while(get_next_line(&reader, line, timeout_ms)) {
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
    }

    if (msg && msg->len) {
        if(journal_local_send_buffer(fd, msg))
            messages_logged++;
        else
            failed_messages++;
    }

cleanup:
    return !failed_messages && messages_logged ? 0 : 1;
}

int main(int argc, char *argv[]) {
    clocks_init();
    nd_log_initialize_for_external_plugins(argv[0]);

    int timeout_ms = 0;
    bool log_as_netdata = false;
    const char *newline = NULL;
    const char *namespace = NULL;
    const char *socket = getenv("NETDATA_SYSTEMD_JOURNAL_PATH");
#ifdef HAVE_CURL
    const char *url = NULL;
    const char *key = NULL;
    const char *cert = NULL;
    const char *trust = NULL;
#endif

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

#ifdef HAVE_CURL
        else if (strncmp(k, "--url=", 6) == 0)
            url = &k[6];

        else if (strncmp(k, "--key=", 6) == 0)
            key = &k[6];

        else if (strncmp(k, "--cert=", 7) == 0)
            cert = &k[7];

        else if (strncmp(k, "--trust=", 8) == 0)
            trust = &k[8];
#endif
        else {
            fprintf(stderr, "Unknown parameter '%s'\n", k);
            return 1;
        }
    }

#ifdef HAVE_CURL
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

    if(url && namespace) {
        fprintf(stderr, "Cannot log to a systemd-journal-remote URL using a namespace. "
                        "Please either give --url or --namespace, not both.\n");
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

#ifdef HAVE_CURL
    if(url)
        return log_input_to_journal_remote(url, key, cert, trust, newline, timeout_ms);
#endif

    return log_input_to_journal(socket, namespace, newline, timeout_ms);
}
