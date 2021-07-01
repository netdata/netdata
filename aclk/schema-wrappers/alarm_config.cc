// SPDX-License-Identifier: GPL-3.0-or-later

#include "alarm_config.h"

#include "proto/alarm/v1/config.pb.h"

#include "libnetdata/libnetdata.h"

using namespace alarmconfig::v1;

char *generate_provide_alarm_configuration(size_t *len, struct provide_alarm_configuration *data)
{
    ProvideAlarmConfiguration msg;
    AlarmConfiguration *cfg = msg.mutable_config();

    msg.set_config_hash(data->cfg_hash);

    cfg->set_alarm(data->cfg.alarm);
    cfg->set_template_(data->cfg.tmpl);
    cfg->set_on_chart(data->cfg.on_chart);
    
    cfg->set_classification(data->cfg.classification);
    cfg->set_type(data->cfg.type);
    cfg->set_component(data->cfg.component);
        
    cfg->set_os(data->cfg.os);
    cfg->set_hosts(data->cfg.hosts);
    cfg->set_plugin(data->cfg.plugin);
    cfg->set_module(data->cfg.module);
    cfg->set_charts(data->cfg.charts);
    cfg->set_families(data->cfg.families);
    cfg->set_lookup(data->cfg.lookup);
    cfg->set_every(data->cfg.every);
    cfg->set_units(data->cfg.units);

    cfg->set_green(data->cfg.green);
    cfg->set_red(data->cfg.red);

    cfg->set_calculation_expr(data->cfg.calculation_expr);
    cfg->set_warning_expr(data->cfg.warning_expr);
    cfg->set_critical_expr(data->cfg.critical_expr);
    
    cfg->set_recipient(data->cfg.recipient);
    cfg->set_exec(data->cfg.exec);
    cfg->set_delay(data->cfg.delay);
    cfg->set_repeat(data->cfg.repeat);
    cfg->set_info(data->cfg.info);
    cfg->set_options(data->cfg.options);
    cfg->set_host_labels(data->cfg.host_labels);

    *len = msg.ByteSizeLong();
    char *bin = (char*)mallocz(*len);
    if (!msg.SerializeToArray(bin, *len))
        return NULL;

    return bin;
}

char *parse_send_alarm_configuration(const char *data, size_t len)
{
    SendAlarmConfiguration msg;
    try {
        if (!msg.ParseFromArray(data, len))
            return NULL;
        if (!msg.config_hash().c_str())
            return NULL;
        return strdupz(msg.config_hash().c_str());
    } catch (...) {
        return NULL;
    }
}

