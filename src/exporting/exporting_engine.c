// SPDX-License-Identifier: GPL-3.0-or-later

#include "exporting_engine.h"

static struct engine *engine = NULL;

void analytics_exporting_connectors_ssl(BUFFER *b)
{
    if (netdata_ssl_exporting_ctx) {
        for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
            struct simple_connector_data *connector_specific_data = instance->connector_specific_data;
            if (SSL_connection(&connector_specific_data->ssl)) {
                buffer_strcat(b, "exporting");
                break;
            }
        }
    }
    buffer_strcat(b, "|");
}

void analytics_exporting_connectors(BUFFER *b)
{
    if (!engine)
        return;

    uint8_t count = 0;

    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        if (count)
            buffer_strcat(b, "|");

        switch (instance->config.type) {
            case EXPORTING_CONNECTOR_TYPE_GRAPHITE:
                buffer_strcat(b, "Graphite");
                break;
            case EXPORTING_CONNECTOR_TYPE_GRAPHITE_HTTP:
                buffer_strcat(b, "GraphiteHTTP");
                break;
            case EXPORTING_CONNECTOR_TYPE_JSON:
                buffer_strcat(b, "JSON");
                break;
            case EXPORTING_CONNECTOR_TYPE_JSON_HTTP:
                buffer_strcat(b, "JSONHTTP");
                break;
            case EXPORTING_CONNECTOR_TYPE_OPENTSDB:
                buffer_strcat(b, "OpenTSDB");
                break;
            case EXPORTING_CONNECTOR_TYPE_OPENTSDB_HTTP:
                buffer_strcat(b, "OpenTSDBHTTP");
                break;
            case EXPORTING_CONNECTOR_TYPE_PROMETHEUS_REMOTE_WRITE:
#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
                buffer_strcat(b, "PrometheusRemoteWrite");
#endif
                break;
            case EXPORTING_CONNECTOR_TYPE_KINESIS:
#if HAVE_KINESIS
                buffer_strcat(b, "Kinesis");
#endif
                break;
            case EXPORTING_CONNECTOR_TYPE_PUBSUB:
#if ENABLE_EXPORTING_PUBSUB
                buffer_strcat(b, "Pubsub");
#endif
                break;
            case EXPORTING_CONNECTOR_TYPE_MONGODB:
#ifdef HAVE_MONGOC
                buffer_strcat(b, "MongoDB");
#endif
                break;
            default:
                buffer_strcat(b, "Unknown");
        }

        count++;
    }
}

/**
 * Exporting Clean Engine
 *
 * Clean all variables allocated inside engine structure
 *
 * @param en a pointer to the structure that will be cleaned.
 */
static void exporting_clean_engine()
{
    if (!engine)
        return;

#if HAVE_KINESIS
    if (engine->aws_sdk_initialized)
        aws_sdk_shutdown();
#endif

#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
    if (engine->protocol_buffers_initialized)
        protocol_buffers_shutdown();
#endif

    //Cleanup web api
    prometheus_clean_server_root();

    for (struct instance *instance = engine->instance_root; instance;) {
        struct instance *current_instance = instance;
        instance = instance->next;

        clean_instance(current_instance);
        freez(current_instance);
    }

    freez((void *)engine->config.hostname);
    freez(engine);
}

/**
 * Clean up the main exporting thread and all connector workers on Netdata exit
 *
 * @param ptr thread data.
 */
static void exporting_main_cleanup(void *pptr)
{
    struct netdata_static_thread *static_thread = CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    if (!engine) {
        static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
        return;
    }

    engine->exit = 1;

    size_t all = 0, exited = 0;

    for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
        all++;

        if (!instance->exited) {
            netdata_log_info("EXPORTING: signaling worker '%s' to stop...", instance->config.name);
            // Lock the mutex before signaling the condition variable
            uv_mutex_lock(&instance->mutex);
            instance->data_is_ready = 1;
            uv_cond_signal(&instance->cond_var);
            uv_mutex_unlock(&instance->mutex);
        }
        else
            netdata_log_info("EXPORTING: found worker '%s' already stopped", instance->config.name);
    }

    size_t iterations = 0;
    while (exited < all) {
        iterations++;
        microsleep(10 * USEC_PER_MS);

        exited = 0;
        for (struct instance *instance = engine->instance_root; instance; instance = instance->next) {
            if (instance->exited) {
                exited++;

                if(instance->thread) {
                    nd_thread_join(instance->thread);
                    instance->thread = NULL;
                }
            }
            else if(iterations % 100 == 0)
                netdata_log_info("EXPORTING: still waiting for worker '%s' to exit...", instance->config.name);
        }
    }

    // this must be called once all the worker thread have exited
    exporting_clean_engine();

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

/**
 * Exporting engine main
 *
 * The main thread used to control the exporting engine.
 *
 * @param ptr a pointer to netdata_static_structure.
 *
 * @return It always returns NULL.
 */
void exporting_main(void *ptr)
{
    CLEANUP_FUNCTION_REGISTER(exporting_main_cleanup) cleanup_ptr = ptr;

    engine = read_exporting_config();
    if (!engine) {
        netdata_log_info("EXPORTING: no exporting connectors configured");
        return;
    }

    if (init_connectors(engine) != 0) {
        netdata_log_error("EXPORTING: cannot initialize exporting connectors");
        return;
    }

    RRDSET *st_main_rusage = NULL;
    RRDDIM *rd_main_user = NULL;
    RRDDIM *rd_main_system = NULL;
    create_main_rusage_chart(&st_main_rusage, &rd_main_user, &rd_main_system);

    heartbeat_t hb;
    heartbeat_init(&hb, localhost->rrd_update_every * USEC_PER_SEC);

    while (service_running(SERVICE_EXPORTERS)) {
        heartbeat_next(&hb);
        engine->now = now_realtime_sec();

        if (mark_scheduled_instances(engine))
            prepare_buffers(engine);

        send_main_rusage(st_main_rusage, rd_main_user, rd_main_system);

#ifdef UNIT_TESTING
        return NULL;
#endif
    }
    service_exits();
}
