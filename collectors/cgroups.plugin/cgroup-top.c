// SPDX-License-Identifier: GPL-3.0-or-later

#include "cgroup-internals.h"

struct cgroup_netdev_link {
    size_t read_slot;
    NETDATA_DOUBLE received[2];
    NETDATA_DOUBLE sent[2];
};

static DICTIONARY *cgroup_netdev_link_dict = NULL;

void cgroup_netdev_link_init(void) {
    cgroup_netdev_link_dict = dictionary_create_advanced(DICT_OPTION_FIXED_SIZE|DICT_OPTION_DONT_OVERWRITE_VALUE, NULL, sizeof(struct cgroup_netdev_link));
}

const DICTIONARY_ITEM *cgroup_netdev_get(struct cgroup *cg) {
    if(cg->cgroup_netdev_link)
        return cg->cgroup_netdev_link;


    struct cgroup_netdev_link t = {
            .read_slot = 0,
            .received = { NAN, NAN },
            .sent = { NAN, NAN },
    };

    cg->cgroup_netdev_link = dictionary_set_and_acquire_item(cgroup_netdev_link_dict, cg->id, &t, sizeof(struct cgroup_netdev_link));
    return dictionary_acquired_item_dup(cgroup_netdev_link_dict, cg->cgroup_netdev_link);
}

void cgroup_netdev_delete(struct cgroup *cg) {
    if(cg->cgroup_netdev_link) {
        dictionary_acquired_item_release(cgroup_netdev_link_dict, cg->cgroup_netdev_link);
        dictionary_del(cgroup_netdev_link_dict, cg->id);
        dictionary_garbage_collect(cgroup_netdev_link_dict);
    }
}

void cgroup_netdev_release(const DICTIONARY_ITEM *link) {
    if(link)
        dictionary_acquired_item_release(cgroup_netdev_link_dict, link);
}

const void *cgroup_netdev_dup(const DICTIONARY_ITEM *link) {
    return dictionary_acquired_item_dup(cgroup_netdev_link_dict, link);
}

void cgroup_netdev_reset_all(void) {
    struct cgroup_netdev_link *t;
    dfe_start_read(cgroup_netdev_link_dict, t) {
        if(t->read_slot >= 1) {
            t->read_slot = 0;
            t->received[1] = NAN;
            t->sent[1] = NAN;
        }
        else {
            t->read_slot = 1;
            t->received[0] = NAN;
            t->sent[0] = NAN;
        }
    }
    dfe_done(t);
}

void cgroup_netdev_add_bandwidth(const DICTIONARY_ITEM *link, NETDATA_DOUBLE received, NETDATA_DOUBLE sent) {
    if(!link)
        return;

    struct cgroup_netdev_link *t = dictionary_acquired_item_value(link);

    size_t slot = (t->read_slot) ? 0 : 1;

    if(isnan(t->received[slot]))
        t->received[slot] = received;
    else
        t->received[slot] += received;

    if(isnan(t->sent[slot]))
        t->sent[slot] = sent;
    else
        t->sent[slot] += sent;
}

void cgroup_netdev_get_bandwidth(struct cgroup *cg, NETDATA_DOUBLE *received, NETDATA_DOUBLE *sent) {
    if(!cg->cgroup_netdev_link) {
        *received = NAN;
        *sent = NAN;
        return;
    }

    struct cgroup_netdev_link *t = dictionary_acquired_item_value(cg->cgroup_netdev_link);

    size_t slot = (t->read_slot) ? 1 : 0;

    *received = t->received[slot];
    *sent = t->sent[slot];
}

int cgroup_function_cgroup_top(BUFFER *wb, int timeout __maybe_unused, const char *function __maybe_unused,
        void *collector_data __maybe_unused,
        rrd_function_result_callback_t result_cb, void *result_cb_data,
        rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
        rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
        void *register_canceller_cb_data __maybe_unused) {

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_CGTOP_HELP);
    buffer_json_member_add_array(wb, "data");

    double max_cpu = 0.0;
    double max_ram = 0.0;
    double max_disk_io_read = 0.0;
    double max_disk_io_written = 0.0;
    double max_net_received = 0.0;
    double max_net_sent = 0.0;

    RRDDIM *rd = NULL;

    uv_mutex_lock(&cgroup_root_mutex);

    for(struct cgroup *cg = cgroup_root; cg ; cg = cg->next) {
        if(unlikely(!cg->enabled || cg->pending_renames || !cg->function_ready || is_cgroup_systemd_service(cg)))
            continue;

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, cg->name); // Name

        if(k8s_is_kubepod(cg))
            buffer_json_add_array_item_string(wb, "k8s"); // Kind
        else
            buffer_json_add_array_item_string(wb, "cgroup"); // Kind

        double cpu = NAN;
        if (cg->st_cpu_rd_user && cg->st_cpu_rd_system) {
            cpu = cg->st_cpu_rd_user->collector.last_stored_value + cg->st_cpu_rd_system->collector.last_stored_value;
            max_cpu = MAX(max_cpu, cpu);
        }

        double ram = rrddim_get_last_stored_value(cg->st_mem_rd_ram, &max_ram, 1.0);

        rd = cg->st_throttle_io_rd_read ? cg->st_throttle_io_rd_read : cg->st_io_rd_read;
        double disk_io_read = rrddim_get_last_stored_value(rd, &max_disk_io_read, 1024.0);
        rd = cg->st_throttle_io_rd_written ? cg->st_throttle_io_rd_written : cg->st_io_rd_written;
        double disk_io_written = rrddim_get_last_stored_value(rd, &max_disk_io_written, 1024.0);


        NETDATA_DOUBLE received, sent;
        cgroup_netdev_get_bandwidth(cg, &received, &sent);
        if (!isnan(received) && !isnan(sent)) {
            received /= 1000.0;
            sent /= 1000.0;
            max_net_received = MAX(max_net_received, received);
            max_net_sent = MAX(max_net_sent, sent);
        }

        buffer_json_add_array_item_double(wb, cpu);
        buffer_json_add_array_item_double(wb, ram);
        buffer_json_add_array_item_double(wb, disk_io_read);
        buffer_json_add_array_item_double(wb, disk_io_written);
        buffer_json_add_array_item_double(wb, received);
        buffer_json_add_array_item_double(wb, sent);

        buffer_json_array_close(wb);
    }

    uv_mutex_unlock(&cgroup_root_mutex);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Node
        buffer_rrdf_table_add_field(wb, field_id++, "Name", "CGROUP Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        // Kind
        buffer_rrdf_table_add_field(wb, field_id++, "Kind", "CGROUP Kind",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // CPU
        buffer_rrdf_table_add_field(wb, field_id++, "CPU", "CPU Usage",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "%", max_cpu, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // RAM
        buffer_rrdf_table_add_field(wb, field_id++, "RAM", "RAM Usage",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_ram, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
        
        // Disk IO Reads
        buffer_rrdf_table_add_field(wb, field_id++, "Reads", "Disk Read Data",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_disk_io_read, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // Disk IO Writes
        buffer_rrdf_table_add_field(wb, field_id++, "Writes", "Disk Written Data",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_disk_io_written, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // Network Received
        buffer_rrdf_table_add_field(wb, field_id++, "Received", "Network Traffic Received",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbps", max_net_received, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // Network Sent
        buffer_rrdf_table_add_field(wb, field_id++, "Sent", "Network Traffic Sent ",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "Mbps", max_net_sent, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
    }
    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "CPU");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "CPU");
        {
            buffer_json_member_add_string(wb, "name", "CPU");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "CPU");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Memory");
        {
            buffer_json_member_add_string(wb, "name", "Memory");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "RAM");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Traffic");
        {
            buffer_json_member_add_string(wb, "name", "Traffic");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Received");
                buffer_json_add_array_item_string(wb, "Sent");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "CPU");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Memory");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_object(wb, "group_by");
    {
        buffer_json_member_add_object(wb, "Kind");
        {
            buffer_json_member_add_string(wb, "name", "Kind");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "Kind");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // group_by

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    int response = HTTP_RESP_OK;
    if(is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data)) {
        buffer_flush(wb);
        response = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, response, result_cb_data);

    return response;
}

int cgroup_function_systemd_top(BUFFER *wb, int timeout __maybe_unused, const char *function __maybe_unused,
        void *collector_data __maybe_unused,
        rrd_function_result_callback_t result_cb, void *result_cb_data,
        rrd_function_is_cancelled_cb_t is_cancelled_cb, void *is_cancelled_cb_data,
        rrd_function_register_canceller_cb_t register_canceller_cb __maybe_unused,
        void *register_canceller_cb_data __maybe_unused) {

    buffer_flush(wb);
    wb->content_type = CT_APPLICATION_JSON;
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    buffer_json_member_add_string(wb, "hostname", rrdhost_hostname(localhost));
    buffer_json_member_add_uint64(wb, "status", HTTP_RESP_OK);
    buffer_json_member_add_string(wb, "type", "table");
    buffer_json_member_add_time_t(wb, "update_every", 1);
    buffer_json_member_add_string(wb, "help", RRDFUNCTIONS_CGTOP_HELP);
    buffer_json_member_add_array(wb, "data");

    double max_cpu = 0.0;
    double max_ram = 0.0;
    double max_disk_io_read = 0.0;
    double max_disk_io_written = 0.0;

    RRDDIM *rd = NULL;

    uv_mutex_lock(&cgroup_root_mutex);

    for(struct cgroup *cg = cgroup_root; cg ; cg = cg->next) {
        if(unlikely(!cg->enabled || cg->pending_renames || !cg->function_ready || !is_cgroup_systemd_service(cg)))
            continue;

        buffer_json_add_array_item_array(wb);

        buffer_json_add_array_item_string(wb, cg->name);

        double cpu = NAN;
        if (cg->st_cpu_rd_user && cg->st_cpu_rd_system) {
            cpu = cg->st_cpu_rd_user->collector.last_stored_value + cg->st_cpu_rd_system->collector.last_stored_value;
            max_cpu = MAX(max_cpu, cpu);
        }

        double ram = rrddim_get_last_stored_value(cg->st_mem_rd_ram, &max_ram, 1.0);

        rd = cg->st_throttle_io_rd_read ? cg->st_throttle_io_rd_read : cg->st_io_rd_read;
        double disk_io_read = rrddim_get_last_stored_value(rd, &max_disk_io_read, 1024.0);
        rd = cg->st_throttle_io_rd_written ? cg->st_throttle_io_rd_written : cg->st_io_rd_written;
        double disk_io_written = rrddim_get_last_stored_value(rd, &max_disk_io_written, 1024.0);

        buffer_json_add_array_item_double(wb, cpu);
        buffer_json_add_array_item_double(wb, ram);
        buffer_json_add_array_item_double(wb, disk_io_read);
        buffer_json_add_array_item_double(wb, disk_io_written);

        buffer_json_array_close(wb);
    }

    uv_mutex_unlock(&cgroup_root_mutex);

    buffer_json_array_close(wb); // data
    buffer_json_member_add_object(wb, "columns");
    {
        size_t field_id = 0;

        // Node
        buffer_rrdf_table_add_field(wb, field_id++, "Name", "Systemd Service Name",
                RRDF_FIELD_TYPE_STRING, RRDF_FIELD_VISUAL_VALUE, RRDF_FIELD_TRANSFORM_NONE,
                0, NULL, NAN, RRDF_FIELD_SORT_ASCENDING, NULL,
                RRDF_FIELD_SUMMARY_COUNT, RRDF_FIELD_FILTER_MULTISELECT,
                RRDF_FIELD_OPTS_VISIBLE | RRDF_FIELD_OPTS_UNIQUE_KEY | RRDF_FIELD_OPTS_STICKY | RRDF_FIELD_OPTS_FULL_WIDTH,
                NULL);

        // CPU
        buffer_rrdf_table_add_field(wb, field_id++, "CPU", "CPU Usage",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "%", max_cpu, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // RAM
        buffer_rrdf_table_add_field(wb, field_id++, "RAM", "RAM Usage",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_ram, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // Disk IO Reads
        buffer_rrdf_table_add_field(wb, field_id++, "Reads", "Disk Read Data",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_disk_io_read, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);

        // Disk IO Writes
        buffer_rrdf_table_add_field(wb, field_id++, "Writes", "Disk Written Data",
                RRDF_FIELD_TYPE_BAR_WITH_INTEGER, RRDF_FIELD_VISUAL_BAR, RRDF_FIELD_TRANSFORM_NUMBER,
                2, "MiB", max_disk_io_written, RRDF_FIELD_SORT_DESCENDING, NULL,
                RRDF_FIELD_SUMMARY_SUM, RRDF_FIELD_FILTER_NONE,
                RRDF_FIELD_OPTS_VISIBLE,
                NULL);
    }

    buffer_json_object_close(wb); // columns
    buffer_json_member_add_string(wb, "default_sort_column", "CPU");

    buffer_json_member_add_object(wb, "charts");
    {
        buffer_json_member_add_object(wb, "CPU");
        {
            buffer_json_member_add_string(wb, "name", "CPU");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "CPU");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);

        buffer_json_member_add_object(wb, "Memory");
        {
            buffer_json_member_add_string(wb, "name", "Memory");
            buffer_json_member_add_string(wb, "type", "stacked-bar");
            buffer_json_member_add_array(wb, "columns");
            {
                buffer_json_add_array_item_string(wb, "RAM");
            }
            buffer_json_array_close(wb);
        }
        buffer_json_object_close(wb);
    }
    buffer_json_object_close(wb); // charts

    buffer_json_member_add_array(wb, "default_charts");
    {
        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "CPU");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);

        buffer_json_add_array_item_array(wb);
        buffer_json_add_array_item_string(wb, "Memory");
        buffer_json_add_array_item_string(wb, "Name");
        buffer_json_array_close(wb);
    }
    buffer_json_array_close(wb);

    buffer_json_member_add_time_t(wb, "expires", now_realtime_sec() + 1);
    buffer_json_finalize(wb);

    int response = HTTP_RESP_OK;
    if(is_cancelled_cb && is_cancelled_cb(is_cancelled_cb_data)) {
        buffer_flush(wb);
        response = HTTP_RESP_CLIENT_CLOSED_REQUEST;
    }

    if(result_cb)
        result_cb(wb, response, result_cb_data);

    return response;
}
