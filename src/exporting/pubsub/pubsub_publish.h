// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EXPORTING_PUBSUB_PUBLISH_H
#define NETDATA_EXPORTING_PUBSUB_PUBLISH_H

#define ERROR_LINE_MAX 1023

#ifdef __cplusplus
extern "C" {
#endif

struct pubsub_specific_data {
    void *stub;
    void *request;
    void *completion_queue;

    void *responses;
    size_t last_tag;
};

int pubsub_init(
    void *pubsub_specific_data_p, char *error_message, const char *destination, const char *credentials_file,
    const char *project_id, const char *topic_id);
void pubsub_cleanup(void *pubsub_specific_data_p);

int pubsub_add_message(void *pubsub_specific_data_p, char *data);

int pubsub_publish(void *pubsub_specific_data_p, char *error_message, size_t buffered_metrics, size_t buffered_bytes);
int pubsub_get_result(
    void *pubsub_specific_data_p, char *error_message,
    size_t *sent_metrics, size_t *sent_bytes, size_t *lost_metrics, size_t *lost_bytes);

#ifdef __cplusplus
}
#endif

#endif //NETDATA_EXPORTING_PUBSUB_PUBLISH_H
