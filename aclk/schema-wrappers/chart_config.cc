#include "chart_config.h"

#include "proto/chart/v1/config.pb.h"

#include "libnetdata/libnetdata.h"

void destroy_update_chart_config(struct update_chart_config *cfg)
{
    freez(cfg->claim_id);
    freez(cfg->node_id);
    freez(cfg->hashes);
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

    char* dest = ((char*)res.hashes) + (hash_count + 1) * sizeof(char *) /* NULL ptr */;
    // now copy them strings
    // null bytes handled by callocz
    for (int i = 0; i < hash_count; i++) {
        strcpy(dest, cfgs.config_hashes(i).c_str());
        res.hashes[i] = dest;
        dest += strlen(dest) + 1 /* end string null */;
    }

    return res;
}
