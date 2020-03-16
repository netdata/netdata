// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_PROMETHEUS_REMOTE_WRITE_REQUEST_H
#define NETDATA_EXPORTING_PROMETHEUS_REMOTE_WRITE_REQUEST_H

#ifdef __cplusplus
extern "C" {
#endif

struct prometheus_remote_write_specific_data {
    void *write_request;
};

void *init_write_request();

void add_host_info(
    void *write_request_p,
    const char *name, const char *instance, const char *application, const char *version, const int64_t timestamp);

void add_label(void *write_request_p, char *key, char *value);

void add_metric(
    void *write_request_p,
    const char *name, const char *chart, const char *family, const char *dimension,
    const char *instance, const double value, const int64_t timestamp);

size_t get_write_request_size(void *write_request_p);

int pack_and_clear_write_request(void *write_request_p, char *buffer, size_t *size);

void protocol_buffers_shutdown();

#ifdef __cplusplus
}
#endif

#endif //NETDATA_EXPORTING_PROMETHEUS_REMOTE_WRITE_REQUEST_H
