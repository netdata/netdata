// SPDX-License-Identifier: GPL-3.0-or-later

#include "alarm_config.h"

#include "proto/alarm/v1/config.pb.h"

#include "libnetdata/libnetdata.h"

#include "schema_wrapper_utils.h"

using namespace alarms::v1;

void destroy_aclk_alarm_configuration(struct aclk_alarm_configuration *cfg)
{
    freez(cfg->alarm);
    freez(cfg->tmpl);
    freez(cfg->on_chart);
    freez(cfg->classification);
    freez(cfg->type);
    freez(cfg->component);
    freez(cfg->os);
    freez(cfg->hosts);
    freez(cfg->plugin);
    freez(cfg->module);
    freez(cfg->charts);
    freez(cfg->lookup);
    freez(cfg->every);
    freez(cfg->units);
    freez(cfg->green);
    freez(cfg->red);
    freez(cfg->calculation_expr);
    freez(cfg->warning_expr);
    freez(cfg->critical_expr);
    freez(cfg->recipient);
    freez(cfg->exec);
    freez(cfg->delay);
    freez(cfg->repeat);
    freez(cfg->info);
    freez(cfg->options);
    freez(cfg->host_labels);
    freez(cfg->p_db_lookup_dimensions);
    freez(cfg->p_db_lookup_method);
    freez(cfg->p_db_lookup_options);
    freez(cfg->chart_labels);
    freez(cfg->summary);
}

char *generate_provide_alarm_configuration(size_t *len, struct provide_alarm_configuration *data)
{
    ProvideAlarmConfiguration msg;
    AlarmConfiguration *cfg = msg.mutable_config();

    msg.set_config_hash(data->cfg_hash);

    if (data->cfg.alarm)
        cfg->set_alarm(data->cfg.alarm);
    if (data->cfg.tmpl)
        cfg->set_template_(data->cfg.tmpl);
    if(data->cfg.on_chart)
        cfg->set_on_chart(data->cfg.on_chart);
    if (data->cfg.classification)
        cfg->set_classification(data->cfg.classification);
    if (data->cfg.type)
        cfg->set_type(data->cfg.type);
    if (data->cfg.component)
        cfg->set_component(data->cfg.component);
    if (data->cfg.os)
        cfg->set_os(data->cfg.os);
    if (data->cfg.hosts)
        cfg->set_hosts(data->cfg.hosts);
    if (data->cfg.plugin)
        cfg->set_plugin(data->cfg.plugin);
    if(data->cfg.module)
        cfg->set_module(data->cfg.module);
    if(data->cfg.charts)
        cfg->set_charts(data->cfg.charts);
    if(data->cfg.lookup)
        cfg->set_lookup(data->cfg.lookup);
    if(data->cfg.every)
        cfg->set_every(data->cfg.every);
    if(data->cfg.units)
        cfg->set_units(data->cfg.units);
    if (data->cfg.green)
        cfg->set_green(data->cfg.green);
    if (data->cfg.red)
        cfg->set_red(data->cfg.red);
    if (data->cfg.calculation_expr)
        cfg->set_calculation_expr(data->cfg.calculation_expr);
    if (data->cfg.warning_expr)
        cfg->set_warning_expr(data->cfg.warning_expr);
    if (data->cfg.critical_expr)
        cfg->set_critical_expr(data->cfg.critical_expr);
    if (data->cfg.recipient)
        cfg->set_recipient(data->cfg.recipient);
    if (data->cfg.exec)
        cfg->set_exec(data->cfg.exec);
    if (data->cfg.delay)
        cfg->set_delay(data->cfg.delay);
    if (data->cfg.repeat)
        cfg->set_repeat(data->cfg.repeat);
    if (data->cfg.info)
        cfg->set_info(data->cfg.info);
    if (data->cfg.options)
        cfg->set_options(data->cfg.options);
    if (data->cfg.host_labels)
        cfg->set_host_labels(data->cfg.host_labels);

    cfg->set_p_db_lookup_after(data->cfg.p_db_lookup_after);
    cfg->set_p_db_lookup_before(data->cfg.p_db_lookup_before);
    if (data->cfg.p_db_lookup_dimensions)
        cfg->set_p_db_lookup_dimensions(data->cfg.p_db_lookup_dimensions);
    if (data->cfg.p_db_lookup_method)
        cfg->set_p_db_lookup_method(data->cfg.p_db_lookup_method);
    if (data->cfg.p_db_lookup_options)
        cfg->set_p_db_lookup_options(data->cfg.p_db_lookup_options);
    cfg->set_p_update_every(data->cfg.p_update_every);

    if (data->cfg.chart_labels)
        cfg->set_chart_labels(data->cfg.chart_labels);
    if (data->cfg.summary)
        cfg->set_summary(data->cfg.summary);

    *len = PROTO_COMPAT_MSG_SIZE(msg);
    char *bin = (char*)mallocz(*len);
    if (!msg.SerializeToArray(bin, *len))
        return NULL;

    return bin;
}

char *parse_send_alarm_configuration(const char *data, size_t len)
{
    SendAlarmConfiguration msg;
    if (!msg.ParseFromArray(data, len))
        return NULL;
    if (!msg.config_hash().c_str())
        return NULL;
    return strdupz(msg.config_hash().c_str());
}

