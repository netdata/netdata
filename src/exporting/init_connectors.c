// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"
#include "graphite/graphite.h"
#include "json/json.h"
#include "opentsdb/opentsdb.h"

#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
#include "prometheus/remote_write/remote_write.h"
#endif

#if HAVE_KINESIS
#include "aws_kinesis/aws_kinesis.h"
#endif

#ifdef ENABLE_EXPORTING_PUBSUB
#include "pubsub/pubsub.h"
#endif

#ifdef HAVE_MONGOC
#include "mongodb/mongodb.h"
#endif

/**
 * Initialize connectors
 *
 * @param engine an engine data structure.
 * @return Returns 0 on success, 1 on failure.
 */
int init_connectors(struct engine *engine)
{
    engine->now = now_realtime_sec();

    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        instance->index = engine->instance_num++;
        instance->after = engine->now;
        
        // Set thread tag and initialize connector based on type
        switch (instance->config.type) {
            case EXPORTING_CONNECTOR_TYPE_GRAPHITE:
                instance->config.thread_tag = "EXPGRPH";
                if (init_graphite_instance(instance) != 0)
                    return 1;
                break;
            case EXPORTING_CONNECTOR_TYPE_GRAPHITE_HTTP:
                instance->config.thread_tag = "EXPGRPH";
                if (init_graphite_instance(instance) != 0)
                    return 1;
                break;
            case EXPORTING_CONNECTOR_TYPE_JSON:
                instance->config.thread_tag = "EXPJSON";
                if (init_json_instance(instance) != 0)
                    return 1;
                break;
            case EXPORTING_CONNECTOR_TYPE_JSON_HTTP:
                instance->config.thread_tag = "EXPJSON";
                if (init_json_http_instance(instance) != 0)
                    return 1;
                break;
            case EXPORTING_CONNECTOR_TYPE_OPENTSDB:
                instance->config.thread_tag = "EXPTSDB";
                if (init_opentsdb_telnet_instance(instance) != 0)
                    return 1;
                break;
            case EXPORTING_CONNECTOR_TYPE_OPENTSDB_HTTP:
                instance->config.thread_tag = "EXPTSDB";
                if (init_opentsdb_http_instance(instance) != 0)
                    return 1;
                break;
            case EXPORTING_CONNECTOR_TYPE_PROMETHEUS_REMOTE_WRITE:
                instance->config.thread_tag = "EXPPRW";
#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
                if (init_prometheus_remote_write_instance(instance) != 0)
                    return 1;
#endif
                break;
            case EXPORTING_CONNECTOR_TYPE_KINESIS:
                instance->config.thread_tag = "EXPKINS";
#if HAVE_KINESIS
                if (init_aws_kinesis_instance(instance) != 0)
                    return 1;
#endif
                break;
            case EXPORTING_CONNECTOR_TYPE_PUBSUB:
                instance->config.thread_tag = "EXPPUBS";
#if ENABLE_EXPORTING_PUBSUB
                if (init_pubsub_instance(instance) != 0)
                    return 1;
#endif
                break;
            case EXPORTING_CONNECTOR_TYPE_MONGODB:
                instance->config.thread_tag = "EXPMNG";
#ifdef HAVE_MONGOC
                if (init_mongodb_instance(instance) != 0)
                    return 1;
#endif
                break;
            default:
                instance->config.thread_tag = "EXPCON";
                netdata_log_error("EXPORTING: unknown exporting connector type");
                return 1;
        }

        // dispatch the instance worker thread
        char threadname[ND_THREAD_TAG_MAX + 1];
        snprintfz(threadname, ND_THREAD_TAG_MAX, "%s[%zu]", instance->config.thread_tag, instance->index);
        
        instance->thread = nd_thread_create(threadname, NETDATA_THREAD_OPTION_DEFAULT, instance->worker, instance);
        if (!instance->thread) {
            netdata_log_error("EXPORTING: cannot create thread worker for instance %s", instance->config.name);
            instance->exited = 1;
            return 1;
        }
    }

    return 0;
}

// TODO: use a base64 encoder from a library
static size_t base64_encode(unsigned char *input, size_t input_size, char *output, size_t output_size)
{
    uint32_t value;
    static char lookup[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
                           "abcdefghijklmnopqrstuvwxyz"
                           "0123456789+/";
    if ((input_size / 3 + 1) * 4 >= output_size) {
        netdata_log_error("Output buffer for encoding size=%zu is not large enough for %zu-bytes input", output_size, input_size);
        return 0;
    }
    size_t count = 0;
    while (input_size >= 3) {
        value = ((input[0] << 16) + (input[1] << 8) + input[2]) & 0xffffff;
        output[0] = lookup[value >> 18];
        output[1] = lookup[(value >> 12) & 0x3f];
        output[2] = lookup[(value >> 6) & 0x3f];
        output[3] = lookup[value & 0x3f];
        //netdata_log_error("Base-64 encode (%04x) -> %c %c %c %c\n", value, output[0], output[1], output[2], output[3]);
        output += 4;
        input += 3;
        input_size -= 3;
        count += 4;
    }
    switch (input_size) {
        case 2:
            value = (input[0] << 10) + (input[1] << 2);
            output[0] = lookup[(value >> 12) & 0x3f];
            output[1] = lookup[(value >> 6) & 0x3f];
            output[2] = lookup[value & 0x3f];
            output[3] = '=';
            //netdata_log_error("Base-64 encode (%06x) -> %c %c %c %c\n", (value>>2)&0xffff, output[0], output[1], output[2], output[3]);
            count += 4;
            output[4] = '\0';
            break;
        case 1:
            value = input[0] << 4;
            output[0] = lookup[(value >> 6) & 0x3f];
            output[1] = lookup[value & 0x3f];
            output[2] = '=';
            output[3] = '=';
            //netdata_log_error("Base-64 encode (%06x) -> %c %c %c %c\n", value, output[0], output[1], output[2], output[3]);
            count += 4;
            output[4] = '\0';
            break;
        case 0:
            output[0] = '\0';
            break;
    }

    return count;
}

/**
 * Initialize a ring buffer and credentials for a simple connector
 *
 * @param instance an instance data structure.
 */
void simple_connector_init(struct instance *instance)
{
    struct simple_connector_data *connector_specific_data =
        (struct simple_connector_data *)instance->connector_specific_data;

    if (connector_specific_data->first_buffer)
        return;

    // Initialize the active buffers that will be used for sending data
    connector_specific_data->header = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
    connector_specific_data->buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

    // create a ring buffer with all buffers initialized
    struct simple_connector_buffer *first_buffer = NULL;

    if (instance->config.buffer_on_failures < 1)
        instance->config.buffer_on_failures = 1;

    for (int i = 0; i < instance->config.buffer_on_failures; i++) {
        struct simple_connector_buffer *current_buffer = callocz(1, sizeof(struct simple_connector_buffer));
        
        // Initialize both header and buffer for each ring buffer entry
        // This ensures we never have NULL pointers during buffer swapping
        current_buffer->header = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);
        current_buffer->buffer = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

        if (!connector_specific_data->first_buffer)
            first_buffer = current_buffer;
        else
            current_buffer->next = connector_specific_data->first_buffer;

        connector_specific_data->first_buffer = current_buffer;
    }

    first_buffer->next = connector_specific_data->first_buffer;
    connector_specific_data->last_buffer = connector_specific_data->first_buffer;

    if (*instance->config.username || *instance->config.password) {
        BUFFER *auth_string = buffer_create(0, &netdata_buffers_statistics.buffers_exporters);

        buffer_sprintf(auth_string, "%s:%s", instance->config.username, instance->config.password);

        size_t encoded_size = (buffer_strlen(auth_string) / 3 + 1) * 4 + 1;
        char *encoded_credentials = callocz(1, encoded_size);

        base64_encode((unsigned char*)buffer_tostring(auth_string), buffer_strlen(auth_string), encoded_credentials, encoded_size);

        buffer_flush(auth_string);
        buffer_sprintf(auth_string, "Authorization: Basic %s\n", encoded_credentials);

        freez(encoded_credentials);

        connector_specific_data->auth_string = strdupz(buffer_tostring(auth_string));

        buffer_free(auth_string);
    }
}
