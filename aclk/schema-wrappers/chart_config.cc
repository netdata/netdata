#include "chart_config.h"

#include "proto/chart/v1/config.pb.h"

#include "libnetdata/libnetdata.h"

#include "schema_wrapper_utils.h"

void destroy_update_chart_config(struct update_chart_config *cfg)
{
    freez(cfg->claim_id);
    freez(cfg->node_id);
    freez(cfg->hashes);
}

void destroy_chart_config_updated(struct chart_config_updated *cfg)
{
    freez(cfg->type);
    freez(cfg->family);
    freez(cfg->context);
    freez(cfg->title);
    freez(cfg->plugin);
    freez(cfg->module);
    freez(cfg->units);
    freez(cfg->config_hash);
}

struct update_chart_config parse_update_chart_config(const char *data, size_t len)
{
    chart::v1::UpdateChartConfigs cfgs;
    update_chart_config res;
    memset(&res, 0, sizeof(res));

    if (!cfgs.ParseFromArray(data, len))
        return res;

    res.claim_id = strdupz(cfgs.claim_id().c_str());
    res.node_id = strdupz(cfgs.node_id().c_str());

    // to not do bazillion tiny allocations for individual strings
    // we calculate how much memory we will need for all of them
    // and allocate at once
    int hash_count = cfgs.config_hashes_size();
    size_t total_strlen = 0;
    for (int i = 0; i < hash_count; i++)
        total_strlen += cfgs.config_hashes(i).length();
    total_strlen += hash_count; //null bytes

    res.hashes = (char**)callocz( 1,
        (hash_count+1) * sizeof(char*) + //char * array incl. terminating NULL at the end
        total_strlen //strings themselves incl. 1 null byte each
    );

    char* dest = ((char*)res.hashes) + (hash_count + 1 /* NULL ptr */) * sizeof(char *);
    // now copy them strings
    // null bytes handled by callocz
    for (int i = 0; i < hash_count; i++) {
        strcpy(dest, cfgs.config_hashes(i).c_str());
        res.hashes[i] = dest;
        dest += strlen(dest) + 1 /* end string null */;
    }

    return res;
}

char *generate_chart_configs_updated(size_t *len, const struct chart_config_updated *config_list, int list_size)
{
    chart::v1::ChartConfigsUpdated configs;
    for (int i = 0; i < list_size; i++) {
        chart::v1::ChartConfigUpdated *config = configs.add_configs();
        config->set_type(config_list[i].type);
        if (config_list[i].family)
            config->set_family(config_list[i].family);
        config->set_context(config_list[i].context);
        config->set_title(config_list[i].title);
        config->set_priority(config_list[i].priority);
        config->set_plugin(config_list[i].plugin);

        if (config_list[i].module)
            config->set_module(config_list[i].module);

        switch (config_list[i].chart_type) {
        case RRDSET_TYPE_LINE:
            config->set_chart_type(chart::v1::LINE);
            break;
        case RRDSET_TYPE_AREA:
            config->set_chart_type(chart::v1::AREA);
            break;
        case RRDSET_TYPE_STACKED:
            config->set_chart_type(chart::v1::STACKED);
            break;
        default:
            return NULL;
        }

        config->set_units(config_list[i].units);
        config->set_config_hash(config_list[i].config_hash);
    }

    *len = PROTO_COMPAT_MSG_SIZE(configs);
    char *bin = (char*)mallocz(*len);
    configs.SerializeToArray(bin, *len);

    return bin;
}
