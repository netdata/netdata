// SPDX-License-Identifier: GPL-3.0-or-later

#include "pluginsd_dyncfg.h"

struct mutex_cond {
    pthread_mutex_t lock;
    pthread_cond_t cond;
    int rc;
};

static void virt_fnc_got_data_cb(BUFFER *wb __maybe_unused, int code, void *callback_data)
{
    struct mutex_cond *ctx = callback_data;
    pthread_mutex_lock(&ctx->lock);
    ctx->rc = code;
    pthread_cond_broadcast(&ctx->cond);
    pthread_mutex_unlock(&ctx->lock);
}

#define VIRT_FNC_TIMEOUT_S 10
#define VIRT_FNC_BUF_SIZE (4096)
void call_virtual_function_async(BUFFER *wb, RRDHOST *host, const char *name, const char *payload, rrd_function_result_callback_t callback, void *callback_data) {
    PARSER *parser = NULL;

    //TODO simplify (as we really need only first parameter to get plugin name maybe we can avoid parsing all)
    char *words[PLUGINSD_MAX_WORDS];
    char *function_with_params = strdupz(name);
    size_t num_words = quoted_strings_splitter(function_with_params, words, PLUGINSD_MAX_WORDS, isspace_map_pluginsd);

    if (num_words < 2) {
        netdata_log_error("PLUGINSD: virtual function name is empty.");
        freez(function_with_params);
        return;
    }

    const DICTIONARY_ITEM *cpi = dictionary_get_and_acquire_item(host->configurable_plugins, get_word(words, num_words, 1));
    if (unlikely(cpi == NULL)) {
        netdata_log_error("PLUGINSD: virtual function plugin '%s' not found.", name);
        freez(function_with_params);
        return;
    }
    struct configurable_plugin *cp = dictionary_acquired_item_value(cpi);
    parser = (PARSER *)cp->cb_usr_ctx;

    BUFFER *function_out = buffer_create(VIRT_FNC_BUF_SIZE, NULL);
    // if we are forwarding this to a plugin (as opposed to streaming/child) we have to remove the first parameter (plugin_name)
    buffer_strcat(function_out, get_word(words, num_words, 0));
    for (size_t i = 1; i < num_words; i++) {
        if (i == 1 && SERVING_PLUGINSD(parser))
            continue;
        buffer_sprintf(function_out, " %s", get_word(words, num_words, i));
    }
    freez(function_with_params);

    usec_t now_ut = now_monotonic_usec();

    struct inflight_function tmp = {
        .started_monotonic_ut = now_ut,
        .result_body_wb = wb,
        .timeout_s = VIRT_FNC_TIMEOUT_S,
        .function = string_strdupz(buffer_tostring(function_out)),
        .payload = payload != NULL ? strdupz(payload) : NULL,
        .virtual = true,

        .result = {
                .cb = callback,
                .data = callback_data,
        },
        .dyncfg = {
            .stop_monotonic_ut = now_ut + VIRT_FNC_TIMEOUT_S * USEC_PER_SEC,
        }
    };
    tmp.stop_monotonic_ut = &tmp.dyncfg.stop_monotonic_ut;
    buffer_free(function_out);

    uuid_generate_time(tmp.transaction);
    char key[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(tmp.transaction, key);

    dictionary_write_lock(parser->inflight.functions);

    // if there is any error, our dictionary callbacks will call the caller callback to notify
    // the caller about the error - no need for error handling here.
    dictionary_set(parser->inflight.functions, key, &tmp, sizeof(struct inflight_function));

    if(!parser->inflight.smaller_monotonic_timeout_ut || *tmp.stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT < parser->inflight.smaller_monotonic_timeout_ut)
        parser->inflight.smaller_monotonic_timeout_ut = *tmp.stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;

    // garbage collect stale inflight functions
    if(parser->inflight.smaller_monotonic_timeout_ut < now_ut)
        pluginsd_inflight_functions_garbage_collect(parser, now_ut);

    dictionary_write_unlock(parser->inflight.functions);
}


dyncfg_config_t call_virtual_function_blocking(PARSER *parser, const char *name, int *rc, const char *payload) {
    usec_t now_ut = now_monotonic_usec();
    BUFFER *wb = buffer_create(VIRT_FNC_BUF_SIZE, NULL);

    struct mutex_cond cond = {
            .lock = PTHREAD_MUTEX_INITIALIZER,
            .cond = PTHREAD_COND_INITIALIZER
    };

    struct inflight_function tmp = {
            .started_monotonic_ut = now_ut,
            .result_body_wb = wb,
            .timeout_s = VIRT_FNC_TIMEOUT_S,
            .function = string_strdupz(name),
            .payload = payload != NULL ? strdupz(payload) : NULL,
            .virtual = true,

            .result = {
                    .cb = virt_fnc_got_data_cb,
                    .data = &cond,
            },
        .dyncfg = {
            .stop_monotonic_ut = now_ut + VIRT_FNC_TIMEOUT_S * USEC_PER_SEC,
        }
    };
    tmp.stop_monotonic_ut = &tmp.dyncfg.stop_monotonic_ut;

    uuid_generate_time(tmp.transaction);

    char key[UUID_COMPACT_STR_LEN];
    uuid_unparse_lower_compact(tmp.transaction, key);

    dictionary_write_lock(parser->inflight.functions);

    // if there is any error, our dictionary callbacks will call the caller callback to notify
    // the caller about the error - no need for error handling here.
    dictionary_set(parser->inflight.functions, key, &tmp, sizeof(struct inflight_function));

    if(!parser->inflight.smaller_monotonic_timeout_ut || *tmp.stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT < parser->inflight.smaller_monotonic_timeout_ut)
        parser->inflight.smaller_monotonic_timeout_ut = *tmp.stop_monotonic_ut + RRDFUNCTIONS_TIMEOUT_EXTENSION_UT;

    // garbage collect stale inflight functions
    if(parser->inflight.smaller_monotonic_timeout_ut < now_ut)
        pluginsd_inflight_functions_garbage_collect(parser, now_ut);

    dictionary_write_unlock(parser->inflight.functions);

    struct timespec tp;
    clock_gettime(CLOCK_REALTIME, &tp);
    tp.tv_sec += (time_t)VIRT_FNC_TIMEOUT_S;

    pthread_mutex_lock(&cond.lock);

    int ret = pthread_cond_timedwait(&cond.cond, &cond.lock, &tp);
    if (ret == ETIMEDOUT)
        netdata_log_error("PLUGINSD: DYNCFG virtual function %s timed out", name);

    pthread_mutex_unlock(&cond.lock);

    dyncfg_config_t cfg;
    cfg.data = strdupz(buffer_tostring(wb));
    cfg.data_size = buffer_strlen(wb);

    if (rc != NULL)
        *rc = cond.rc;

    buffer_free(wb);
    return cfg;
}

#define CVF_MAX_LEN (1024)
static dyncfg_config_t get_plugin_config_cb(void *usr_ctx, const char *plugin_name)
{
    PARSER *parser = usr_ctx;

    if (SERVING_STREAMING(parser)) {
        char buf[CVF_MAX_LEN + 1];
        snprintfz(buf, CVF_MAX_LEN, FUNCTION_NAME_GET_PLUGIN_CONFIG " %s", plugin_name);
        return call_virtual_function_blocking(parser, buf, NULL, NULL);
    }

    return call_virtual_function_blocking(parser, FUNCTION_NAME_GET_PLUGIN_CONFIG, NULL, NULL);
}

static dyncfg_config_t get_plugin_config_schema_cb(void *usr_ctx, const char *plugin_name)
{
    PARSER *parser = usr_ctx;

    if (SERVING_STREAMING(parser)) {
        char buf[CVF_MAX_LEN + 1];
        snprintfz(buf, CVF_MAX_LEN, FUNCTION_NAME_GET_PLUGIN_CONFIG_SCHEMA " %s", plugin_name);
        return call_virtual_function_blocking(parser, buf, NULL, NULL);
    }

    return call_virtual_function_blocking(parser, "get_plugin_config_schema", NULL, NULL);
}

static dyncfg_config_t get_module_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_MODULE_CONFIG);
    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

static dyncfg_config_t get_module_config_schema_cb(void *usr_ctx, const char *plugin_name, const char *module_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_MODULE_CONFIG_SCHEMA);
    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

static dyncfg_config_t get_job_config_schema_cb(void *usr_ctx, const char *plugin_name, const char *module_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_JOB_CONFIG_SCHEMA);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

static dyncfg_config_t get_job_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name, const char* job_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_GET_JOB_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s %s", module_name, job_name);

    dyncfg_config_t ret = call_virtual_function_blocking(parser, buffer_tostring(wb), NULL, NULL);

    buffer_free(wb);

    return ret;
}

enum set_config_result set_plugin_config_cb(void *usr_ctx, const char *plugin_name, dyncfg_config_t *cfg)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_SET_PLUGIN_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, cfg->data);

    buffer_free(wb);
    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}

enum set_config_result set_module_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name, dyncfg_config_t *cfg)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_SET_MODULE_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s", module_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, cfg->data);

    buffer_free(wb);

    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}

enum set_config_result set_job_config_cb(void *usr_ctx, const char *plugin_name, const char *module_name, const char *job_name, dyncfg_config_t *cfg)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_SET_JOB_CONFIG);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s %s", module_name, job_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, cfg->data);

    buffer_free(wb);

    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}

enum set_config_result delete_job_cb(void *usr_ctx, const char *plugin_name ,const char *module_name, const char *job_name)
{
    PARSER *parser = usr_ctx;
    BUFFER *wb = buffer_create(CVF_MAX_LEN, NULL);

    buffer_strcat(wb, FUNCTION_NAME_DELETE_JOB);

    if (SERVING_STREAMING(parser))
        buffer_sprintf(wb, " %s", plugin_name);

    buffer_sprintf(wb, " %s %s", module_name, job_name);

    int rc;
    call_virtual_function_blocking(parser, buffer_tostring(wb), &rc, NULL);

    buffer_free(wb);

    if(rc != DYNCFG_VFNC_RET_CFG_ACCEPTED)
        return SET_CONFIG_REJECTED;
    return SET_CONFIG_ACCEPTED;
}


PARSER_RC pluginsd_register_plugin(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    netdata_log_info("PLUGINSD: DYNCFG_ENABLE");

    if (unlikely (num_words != 2))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_ENABLE, "missing name parameter");

    struct configurable_plugin *cfg = callocz(1, sizeof(struct configurable_plugin));

    cfg->name = strdupz(words[1]);
    cfg->set_config_cb = set_plugin_config_cb;
    cfg->get_config_cb = get_plugin_config_cb;
    cfg->get_config_schema_cb = get_plugin_config_schema_cb;
    cfg->cb_usr_ctx = parser;

    const DICTIONARY_ITEM *di = register_plugin(parser->user.host->configurable_plugins, cfg, SERVING_PLUGINSD(parser));
    if (unlikely(di == NULL)) {
        freez(cfg->name);
        freez(cfg);
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_ENABLE, "error registering plugin");
    }

    if (SERVING_PLUGINSD(parser)) {
        // this is optimization for pluginsd to avoid extra dictionary lookup
        // as we know which plugin is comunicating with us
        parser->user.cd->cfg_dict_item = di;
        parser->user.cd->configuration = cfg;
    } else {
        // register_plugin keeps the item acquired, so we need to release it
        dictionary_acquired_item_release(parser->user.host->configurable_plugins, di);
    }

    rrdpush_send_dyncfg_enable(parser->user.host, cfg->name);

    return PARSER_RC_OK;
}

#define LOG_MSG_SIZE (1024)
#define MODULE_NAME_IDX (SERVING_PLUGINSD(parser) ? 1 : 2)
#define MODULE_TYPE_IDX (SERVING_PLUGINSD(parser) ? 2 : 3)

PARSER_RC pluginsd_register_module(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    netdata_log_info("PLUGINSD: DYNCFG_REG_MODULE");

    size_t expected_num_words = SERVING_PLUGINSD(parser) ? 3 : 4;

    if (unlikely(num_words != expected_num_words)) {
        char log[LOG_MSG_SIZE + 1];
        snprintfz(log, LOG_MSG_SIZE, "expected %zu (got %zu) parameters: %smodule_name module_type", expected_num_words - 1, num_words - 1, SERVING_PLUGINSD(parser) ? "" : "plugin_name ");
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, log);
    }

    struct configurable_plugin *plug_cfg;
    const DICTIONARY_ITEM *di = NULL;
    if (SERVING_PLUGINSD(parser)) {
        plug_cfg = parser->user.cd->configuration;
        if (unlikely(plug_cfg == NULL))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, "you have to enable dynamic configuration first using " PLUGINSD_KEYWORD_DYNCFG_ENABLE);
    } else {
        di = dictionary_get_and_acquire_item(parser->user.host->configurable_plugins, words[1]);
        if (unlikely(di == NULL))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, "plugin not found");

        plug_cfg = (struct configurable_plugin *)dictionary_acquired_item_value(di);
    }

    struct module *mod = callocz(1, sizeof(struct module));

    mod->type = str2_module_type(words[MODULE_TYPE_IDX]);
    if (unlikely(mod->type == MOD_TYPE_UNKNOWN)) {
        freez(mod);
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_MODULE, "unknown module type (allowed: job_array, single)");
    }

    mod->name = strdupz(words[MODULE_NAME_IDX]);

    mod->set_config_cb = set_module_config_cb;
    mod->get_config_cb = get_module_config_cb;
    mod->get_config_schema_cb = get_module_config_schema_cb;
    mod->config_cb_usr_ctx = parser;

    mod->get_job_config_cb = get_job_config_cb;
    mod->get_job_config_schema_cb = get_job_config_schema_cb;
    mod->set_job_config_cb = set_job_config_cb;
    mod->delete_job_cb = delete_job_cb;
    mod->job_config_cb_usr_ctx = parser;

    register_module(parser->user.host->configurable_plugins, plug_cfg, mod, SERVING_PLUGINSD(parser));

    if (di != NULL)
        dictionary_acquired_item_release(parser->user.host->configurable_plugins, di);

    rrdpush_send_dyncfg_reg_module(parser->user.host, plug_cfg->name, mod->name, mod->type);

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_register_job_common(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused, const char *plugin_name) {
    const char *module_name = words[0];
    const char *job_name = words[1];
    const char *job_type_str = words[2];
    const char *flags_str = words[3];

    long f = str2l(flags_str);

    if (f < 0)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "invalid flags received");

    dyncfg_job_flg_t flags = f;

    if (SERVING_PLUGINSD(parser))
        flags |= JOB_FLG_PLUGIN_PUSHED;
    else
        flags |= JOB_FLG_STREAMING_PUSHED;

    enum job_type job_type = dyncfg_str2job_type(job_type_str);
    if (job_type == JOB_TYPE_UNKNOWN)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "unknown job type");

    if (SERVING_PLUGINSD(parser) && job_type == JOB_TYPE_USER)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "plugins cannot push jobs of type \"user\" (this is allowed only in streaming)");

    if (register_job(parser->user.host->configurable_plugins, plugin_name, module_name, job_name, job_type, flags, 0)) // ignore existing is off as this is explicitly called register job
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, "error registering job");

    rrdpush_send_dyncfg_reg_job(parser->user.host, plugin_name, module_name, job_name, job_type, flags);

    return PARSER_RC_OK;
}

PARSER_RC pluginsd_register_job(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    size_t expected_num_words = SERVING_PLUGINSD(parser) ? 5 : 6;

    if (unlikely(num_words != expected_num_words)) {
        char log[LOG_MSG_SIZE + 1];
        snprintfz(log, LOG_MSG_SIZE, "expected %zu (got %zu) parameters: %smodule_name job_name job_type", expected_num_words - 1, num_words - 1, SERVING_PLUGINSD(parser) ? "" : "plugin_name ");
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_REGISTER_JOB, log);
    }

    if (SERVING_PLUGINSD(parser)) {
        return pluginsd_register_job_common(&words[1], num_words - 1, parser,  parser->user.cd->configuration->name);
    }
    return pluginsd_register_job_common(&words[2], num_words - 2, parser, words[1]);
}

PARSER_RC pluginsd_dyncfg_reset(char **words __maybe_unused, size_t num_words __maybe_unused, PARSER *parser __maybe_unused) {
    if (unlikely(num_words != (SERVING_PLUGINSD(parser) ? 1 : 2)))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_RESET, SERVING_PLUGINSD(parser) ? "expected 0 parameters" : "expected 1 parameter: plugin_name");

    if (SERVING_PLUGINSD(parser)) {
        unregister_plugin(parser->user.host->configurable_plugins, parser->user.cd->cfg_dict_item);
        rrdpush_send_dyncfg_reset(parser->user.host, parser->user.cd->configuration->name);
        parser->user.cd->configuration = NULL;
    } else {
        const DICTIONARY_ITEM *di = dictionary_get_and_acquire_item(parser->user.host->configurable_plugins, words[1]);
        if (unlikely(di == NULL))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DYNCFG_RESET, "plugin not found");
        unregister_plugin(parser->user.host->configurable_plugins, di);
        rrdpush_send_dyncfg_reset(parser->user.host, words[1]);
    }

    return PARSER_RC_OK;
}

static inline PARSER_RC pluginsd_job_status_common(char **words, size_t num_words, PARSER *parser, const char *plugin_name) {
    int state = str2i(words[3]);

    enum job_status status = str2job_state(words[2]);
    if (unlikely(SERVING_PLUGINSD(parser) && status == JOB_STATUS_UNKNOWN))
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_REPORT_JOB_STATUS, "unknown job status");

    char *message = NULL;
    if (num_words == 5 && strlen(words[4]) > 0)
        message = words[4];

    const DICTIONARY_ITEM *plugin_item;
    DICTIONARY *job_dict;
    const DICTIONARY_ITEM *job_item = report_job_status_acq_lock(parser->user.host->configurable_plugins, &plugin_item, &job_dict, plugin_name, words[0], words[1], status, state, message);

    if (job_item != NULL) {
        struct job *job = dictionary_acquired_item_value(job_item);
        rrdpush_send_job_status_update(parser->user.host, plugin_name, words[0], job);

        pthread_mutex_unlock(&job->lock);
        dictionary_acquired_item_release(job_dict, job_item);
        dictionary_acquired_item_release(parser->user.host->configurable_plugins, plugin_item);
    }

    return PARSER_RC_OK;
}

// job_status [plugin_name if streaming] <module_name> <job_name> <status_code> <state> [message]
PARSER_RC pluginsd_job_status(char **words, size_t num_words, PARSER *parser) {
    if (SERVING_PLUGINSD(parser)) {
        if (unlikely(num_words != 5 && num_words != 6))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_REPORT_JOB_STATUS, "expected 4 or 5 parameters: module_name, job_name, status_code, state, [optional: message]");
    } else {
        if (unlikely(num_words != 6 && num_words != 7))
            return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_REPORT_JOB_STATUS, "expected 5 or 6 parameters: plugin_name, module_name, job_name, status_code, state, [optional: message]");
    }

    if (SERVING_PLUGINSD(parser)) {
        return pluginsd_job_status_common(&words[1], num_words - 1, parser, parser->user.cd->configuration->name);
    }
    return pluginsd_job_status_common(&words[2], num_words - 2, parser, words[1]);
}

PARSER_RC pluginsd_delete_job(char **words, size_t num_words, PARSER *parser) {
    // this can confuse a bit but there is a diference between KEYWORD_DELETE_JOB and actual delete_job function
    // they are of opossite direction
    if (num_words != 4)
        return PLUGINSD_DISABLE_PLUGIN(parser, PLUGINSD_KEYWORD_DELETE_JOB, "expected 2 parameters: plugin_name, module_name, job_name");

    const char *plugin_name = get_word(words, num_words, 1);
    const char *module_name = get_word(words, num_words, 2);
    const char *job_name = get_word(words, num_words, 3);

    if (SERVING_STREAMING(parser))
        delete_job_pname(parser->user.host->configurable_plugins, plugin_name, module_name, job_name);

    // forward to parent if any
    rrdpush_send_job_deleted(parser->user.host, plugin_name, module_name, job_name);
    return PARSER_RC_OK;
}

void pluginsd_dyncfg_cleanup(PARSER *parser) {
    if (parser->user.cd != NULL && parser->user.cd->configuration != NULL) {
        unregister_plugin(parser->user.host->configurable_plugins, parser->user.cd->cfg_dict_item);
        parser->user.cd->configuration = NULL;
    } else if (parser->user.host != NULL && SERVING_STREAMING(parser) && parser->user.host != localhost){
        dictionary_flush(parser->user.host->configurable_plugins);
    }
}
