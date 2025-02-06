// SPDX-License-Identifier: GPL-3.0-or-later

#include "rrdhost-labels.h"
#include "rrdhost.h"
#include "streaming/stream.h"

void rrdhost_set_is_parent_label(void) {
    uint32_t count = stream_receivers_currently_connected();

    if (count == 0 || count == 1) {
        RRDLABELS *labels = localhost->rrdlabels;
        rrdlabels_add(labels, "_is_parent", (count) ? "true" : "false", RRDLABEL_SRC_AUTO);

        // queue a node info
        aclk_queue_node_info(localhost, false);
    }
}

static bool config_label_cb(void *data __maybe_unused, const char *name, const char *value) {
    rrdlabels_add(localhost->rrdlabels, name, value, RRDLABEL_SRC_CONFIG);
    return true;
}

static void rrdhost_load_config_labels(void) {
    int status = inicfg_load(&netdata_config, NULL, 1, CONFIG_SECTION_HOST_LABEL);
    if(!status) {
        char *filename = CONFIG_DIR "/" CONFIG_FILENAME;
        nd_log(NDLS_DAEMON, NDLP_WARNING,
               "RRDLABEL: Cannot reload the configuration file '%s', using labels in memory",
               filename);
    }

    inicfg_foreach_value_in_section(&netdata_config, CONFIG_SECTION_HOST_LABEL, config_label_cb, NULL);
}

static void rrdhost_load_kubernetes_labels(void) {
    char label_script[sizeof(char) * (strlen(netdata_configured_primary_plugins_dir) + strlen("get-kubernetes-labels.sh") + 2)];
    sprintf(label_script, "%s/%s", netdata_configured_primary_plugins_dir, "get-kubernetes-labels.sh");

    if (unlikely(access(label_script, R_OK) != 0)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "Kubernetes pod label fetching script %s not found.",
               label_script);

        return;
    }

    POPEN_INSTANCE *instance = spawn_popen_run(label_script);
    if(!instance) return;

    char buffer[1000 + 1];
    while (fgets(buffer, 1000, spawn_popen_stdout(instance)) != NULL)
        rrdlabels_add_pair(localhost->rrdlabels, buffer, RRDLABEL_SRC_AUTO|RRDLABEL_SRC_K8S);

    // Non-zero exit code means that all the script output is error messages. We've shown already any message that didn't include a ':'
    // Here we'll inform with an ERROR that the script failed, show whatever (if anything) was added to the list of labels, free the memory and set the return to null
    int rc = spawn_popen_wait(instance);
    if(rc)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "%s exited abnormally. Failed to get kubernetes labels.",
               label_script);
}

static void rrdhost_load_auto_labels(void) {
    RRDLABELS *labels = localhost->rrdlabels;

    rrdhost_system_info_to_rrdlabels(localhost->system_info, labels);
    add_aclk_host_labels();

    // The source should be CONF, but when it is set, these labels are exported by default ('send configured labels' in exporting.conf).
    // Their export seems to break exporting to Graphite, see https://github.com/netdata/netdata/issues/14084.

    int is_ephemeral = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_GLOBAL, "is ephemeral node", CONFIG_BOOLEAN_NO);
    rrdlabels_add(labels, HOST_LABEL_IS_EPHEMERAL, is_ephemeral ? "true" : "false", RRDLABEL_SRC_CONFIG);

    int has_unstable_connection = inicfg_get_boolean(&netdata_config, CONFIG_SECTION_GLOBAL, "has unstable connection", CONFIG_BOOLEAN_NO);
    rrdlabels_add(labels, "_has_unstable_connection", has_unstable_connection ? "true" : "false", RRDLABEL_SRC_AUTO);

    rrdlabels_add(labels, "_is_parent", (stream_receivers_currently_connected() > 0) ? "true" : "false", RRDLABEL_SRC_AUTO);

    rrdlabels_add(labels, "_hostname", string2str(localhost->hostname), RRDLABEL_SRC_AUTO);
    rrdlabels_add(labels, "_os", string2str(localhost->os), RRDLABEL_SRC_AUTO);

    if (localhost->stream.snd.destination)
        rrdlabels_add(labels, "_streams_to", string2str(localhost->stream.snd.destination), RRDLABEL_SRC_AUTO);
}

void reload_host_labels(void) {
    if(!localhost->rrdlabels)
        localhost->rrdlabels = rrdlabels_create();

    rrdlabels_unmark_all(localhost->rrdlabels);

    // priority is important here
    rrdhost_load_config_labels();
    rrdhost_load_kubernetes_labels();
    rrdhost_load_auto_labels();

    rrdhost_flag_set(localhost,RRDHOST_FLAG_METADATA_LABELS | RRDHOST_FLAG_METADATA_UPDATE);

    stream_send_host_labels(localhost);
}
