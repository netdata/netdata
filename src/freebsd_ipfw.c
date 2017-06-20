#include "common.h"

#include <netinet/ip_fw.h>

#define FREE_MEM_THRESHOLD 10000 // number of unused chunks that trigger memory freeing

#define COMMON_IPFW_ERROR() error("DISABLED: ipfw.packets chart"); \
                            error("DISABLED: ipfw.bytes chart"); \
                            error("DISABLED: ipfw.dyn_active chart"); \
                            error("DISABLED: ipfw.dyn_expired chart"); \
                            error("DISABLED: ipfw.mem chart");

// --------------------------------------------------------------------------------------------------------------------
// ipfw

int do_ipfw(int update_every, usec_t dt) {
    (void)dt;
#if __FreeBSD__ >= 11

    static int do_static = -1, do_dynamic = -1, do_mem = -1;

    if (unlikely(do_static == -1)) {
        do_static  = config_get_boolean("plugin:freebsd:ipfw", "counters for static rules", 1);
        do_dynamic = config_get_boolean("plugin:freebsd:ipfw", "number of dynamic rules", 1);
        do_mem     = config_get_boolean("plugin:freebsd:ipfw", "allocated memory", 1);
    }

    // variables for getting ipfw configuration

    int error;
    static int ipfw_socket = -1;
    static ipfw_cfg_lheader *cfg = NULL;
    ip_fw3_opheader *op3 = NULL;
    static socklen_t *optlen = NULL, cfg_size = 0;

    // variables for static rules handling

    ipfw_obj_ctlv *ctlv = NULL;
    ipfw_obj_tlv *rbase = NULL;
    int rcnt = 0;

    int n, seen;
    struct ip_fw_rule *rule;
    struct ip_fw_bcounter *cntr;
    int c = 0;

    char rule_num_str[12];

    // variables for dynamic rules handling

    caddr_t dynbase = NULL;
    size_t dynsz = 0;
    size_t readsz = sizeof(*cfg);;
    int ttype = 0;
    ipfw_obj_tlv *tlv;
    ipfw_dyn_rule *dyn_rule;
    uint16_t rulenum, prev_rulenum = IPFW_DEFAULT_RULE;
    unsigned srn, static_rules_num = 0;
    static size_t dyn_rules_num_size = 0;

    static struct dyn_rule_num {
        uint16_t rule_num;
        uint32_t active_rules;
        uint32_t expired_rules;
    } *dyn_rules_num = NULL;

    uint32_t *dyn_rules_counter;

    if (likely(do_static | do_dynamic | do_mem)) {

        // initialize the smallest ipfw_cfg_lheader possible

        if (unlikely((optlen == NULL) || (cfg == NULL))) {
            optlen = reallocz(optlen, sizeof(socklen_t));
            *optlen = cfg_size = 32;
            cfg = reallocz(cfg, *optlen);
        }

        // get socket descriptor and initialize ipfw_cfg_lheader structure

        if (unlikely(ipfw_socket == -1))
            ipfw_socket = socket(AF_INET, SOCK_RAW, IPPROTO_RAW);
        if (unlikely(ipfw_socket == -1)) {
            error("FREEBSD: can't get socket for ipfw configuration");
            error("FREEBSD: run netdata as root to get access to ipfw data");
            COMMON_IPFW_ERROR();
            return 1;
        }

        bzero(cfg, 32);
        cfg->flags = IPFW_CFG_GET_STATIC | IPFW_CFG_GET_COUNTERS | IPFW_CFG_GET_STATES;
        op3 = &cfg->opheader;
        op3->opcode = IP_FW_XGET;

        // get ifpw configuration size than get configuration

        *optlen = cfg_size;
        error = getsockopt(ipfw_socket, IPPROTO_IP, IP_FW3, op3, optlen);
        if (error)
            if (errno != ENOMEM) {
                error("FREEBSD: ipfw socket reading error");
                COMMON_IPFW_ERROR();
                return 1;
            }
        if ((cfg->size > cfg_size) || ((cfg_size - cfg->size) > sizeof(struct dyn_rule_num) * FREE_MEM_THRESHOLD)) {
            *optlen = cfg_size = cfg->size;
            cfg = reallocz(cfg, *optlen);
            bzero(cfg, 32);
            cfg->flags = IPFW_CFG_GET_STATIC | IPFW_CFG_GET_COUNTERS | IPFW_CFG_GET_STATES;
            op3 = &cfg->opheader;
            op3->opcode = IP_FW_XGET;
            error = getsockopt(ipfw_socket, IPPROTO_IP, IP_FW3, op3, optlen);
            if (error) {
                error("FREEBSD: ipfw socket reading error");
                COMMON_IPFW_ERROR();
                return 1;
            }
        }

        // go through static rules configuration structures

        ctlv = (ipfw_obj_ctlv *) (cfg + 1);

        if (cfg->flags & IPFW_CFG_GET_STATIC) {
            /* We've requested static rules */
            if (ctlv->head.type == IPFW_TLV_TBLNAME_LIST) {
                readsz += ctlv->head.length;
                ctlv = (ipfw_obj_ctlv *) ((caddr_t) ctlv +
                                          ctlv->head.length);
            }

            if (ctlv->head.type == IPFW_TLV_RULE_LIST) {
                rbase = (ipfw_obj_tlv *) (ctlv + 1);
                rcnt = ctlv->count;
                readsz += ctlv->head.length;
                ctlv = (ipfw_obj_ctlv *) ((caddr_t) ctlv + ctlv->head.length);
            }
        }

        if ((cfg->flags & IPFW_CFG_GET_STATES) && (readsz != *optlen)) {
            /* We may have some dynamic states */
            dynsz = *optlen - readsz;
            /* Skip empty header */
            if (dynsz != sizeof(ipfw_obj_ctlv))
                dynbase = (caddr_t) ctlv;
            else
                dynsz = 0;
        }

        // --------------------------------------------------------------------

        if (likely(do_mem)) {
            static RRDSET *st_mem = NULL;
            static RRDDIM *rd_dyn_mem = NULL;
            static RRDDIM *rd_stat_mem = NULL;

            if (unlikely(!st_mem)) {
                st_mem = rrdset_create_localhost("ipfw",
                                                 "mem",
                                                 NULL,
                                                 "memory allocated",
                                                 NULL,
                                                 "Memory allocated by rules",
                                                 "bytes",
                                                 3005,
                                                 update_every,
                                                 RRDSET_TYPE_STACKED
                );
                rrdset_flag_set(st_mem, RRDSET_FLAG_DETAIL);

                rd_dyn_mem = rrddim_add(st_mem, "dynamic", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rd_stat_mem = rrddim_add(st_mem, "static", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
            } else
                rrdset_next(st_mem);

            rrddim_set_by_pointer(st_mem, rd_dyn_mem, dynsz);
            rrddim_set_by_pointer(st_mem, rd_stat_mem, *optlen - dynsz);
            rrdset_done(st_mem);
        }

        // --------------------------------------------------------------------

        static RRDSET *st_packets = NULL, *st_bytes = NULL;
        RRDDIM *rd_packets = NULL, *rd_bytes = NULL;

        if (likely(do_static || do_dynamic)) {
            if (likely(do_static)) {
                if (unlikely(!st_packets))
                    st_packets = rrdset_create_localhost("ipfw",
                                                         "packets",
                                                         NULL,
                                                         "static rules",
                                                         NULL,
                                                         "Packets",
                                                         "packets/s",
                                                         3001,
                                                         update_every,
                                                         RRDSET_TYPE_STACKED
                    );
                else
                    rrdset_next(st_packets);

                if (unlikely(!st_bytes))
                    st_bytes = rrdset_create_localhost("ipfw",
                                                       "bytes",
                                                       NULL,
                                                       "static rules",
                                                       NULL,
                                                       "Bytes",
                                                       "bytes/s",
                                                       3002,
                                                       update_every,
                                                       RRDSET_TYPE_STACKED
                    );
                else
                    rrdset_next(st_bytes);
            }

            for (n = seen = 0; n < rcnt; n++, rbase = (ipfw_obj_tlv *) ((caddr_t) rbase + rbase->length)) {
                cntr = (struct ip_fw_bcounter *) (rbase + 1);
                rule = (struct ip_fw_rule *) ((caddr_t) cntr + cntr->size);
                if (rule->rulenum != prev_rulenum)
                    static_rules_num++;
                if (rule->rulenum > IPFW_DEFAULT_RULE)
                    break;

                if (likely(do_static)) {
                    sprintf(rule_num_str, "%d_%d", rule->rulenum, rule->id);

                    rd_packets = rrddim_find(st_packets, rule_num_str);
                    if (unlikely(!rd_packets))
                        rd_packets = rrddim_add(st_packets, rule_num_str, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_set_by_pointer(st_packets, rd_packets, cntr->pcnt);

                    rd_bytes = rrddim_find(st_bytes, rule_num_str);
                    if (unlikely(!rd_bytes))
                        rd_bytes = rrddim_add(st_bytes, rule_num_str, NULL, 1, 1, RRD_ALGORITHM_INCREMENTAL);
                    rrddim_set_by_pointer(st_bytes, rd_bytes, cntr->bcnt);
                }

                c += rbase->length;
                seen++;
            }

            if (likely(do_static)) {
                rrdset_done(st_packets);
                rrdset_done(st_bytes);
            }
        }

        // --------------------------------------------------------------------

        // go through dynamic rules configuration structures

        if (likely(do_dynamic && (dynsz > 0))) {
            if ((dyn_rules_num_size < sizeof(struct dyn_rule_num) * static_rules_num) ||
                ((dyn_rules_num_size - sizeof(struct dyn_rule_num) * static_rules_num) >
                 sizeof(struct dyn_rule_num) * FREE_MEM_THRESHOLD)) {
                dyn_rules_num_size = sizeof(struct dyn_rule_num) * static_rules_num;
                dyn_rules_num = reallocz(dyn_rules_num, dyn_rules_num_size);
            }
            bzero(dyn_rules_num, sizeof(struct dyn_rule_num) * static_rules_num);
            dyn_rules_num->rule_num = IPFW_DEFAULT_RULE;

            if (dynsz > 0 && ctlv->head.type == IPFW_TLV_DYNSTATE_LIST) {
                dynbase += sizeof(*ctlv);
                dynsz -= sizeof(*ctlv);
                ttype = IPFW_TLV_DYN_ENT;
            }

            while (dynsz > 0) {
                tlv = (ipfw_obj_tlv *) dynbase;
                if (tlv->type != ttype)
                    break;

                dyn_rule = (ipfw_dyn_rule *) (tlv + 1);
                bcopy(&dyn_rule->rule, &rulenum, sizeof(rulenum));

                for (srn = 0; srn < (static_rules_num - 1); srn++) {
                    if (dyn_rule->expire > 0)
                        dyn_rules_counter = &dyn_rules_num[srn].active_rules;
                    else
                        dyn_rules_counter = &dyn_rules_num[srn].expired_rules;
                    if (dyn_rules_num[srn].rule_num == rulenum) {
                        (*dyn_rules_counter)++;
                        break;
                    }
                    if (dyn_rules_num[srn].rule_num == IPFW_DEFAULT_RULE) {
                        dyn_rules_num[srn].rule_num = rulenum;
                        dyn_rules_num[srn + 1].rule_num = IPFW_DEFAULT_RULE;
                        (*dyn_rules_counter)++;
                        break;
                    }
                }

                dynsz -= tlv->length;
                dynbase += tlv->length;
            }

            // --------------------------------------------------------------------

            static RRDSET *st_active = NULL, *st_expired = NULL;
            RRDDIM *rd_active = NULL, *rd_expired = NULL;

            if (unlikely(!st_active))
                st_active = rrdset_create_localhost("ipfw",
                                                    "active",
                                                    NULL,
                                                    "dynamic_rules",
                                                    NULL,
                                                    "Active rules",
                                                    "rules",
                                                    3003,
                                                    update_every,
                                                    RRDSET_TYPE_STACKED
                );
            else
                rrdset_next(st_active);

            if (unlikely(!st_expired))
                st_expired = rrdset_create_localhost("ipfw",
                                                     "expired",
                                                     NULL,
                                                     "dynamic_rules",
                                                     NULL,
                                                     "Expired rules",
                                                     "rules",
                                                     3004,
                                                     update_every,
                                                     RRDSET_TYPE_STACKED
                );
            else
                rrdset_next(st_expired);

            for (srn = 0; (srn < (static_rules_num - 1)) && (dyn_rules_num[srn].rule_num != IPFW_DEFAULT_RULE); srn++) {
                sprintf(rule_num_str, "%d", dyn_rules_num[srn].rule_num);

                rd_active = rrddim_find(st_active, rule_num_str);
                if (unlikely(!rd_active))
                    rd_active = rrddim_add(st_active, rule_num_str, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rrddim_set_by_pointer(st_active, rd_active, dyn_rules_num[srn].active_rules);

                rd_expired = rrddim_find(st_expired, rule_num_str);
                if (unlikely(!rd_expired))
                    rd_expired = rrddim_add(st_expired, rule_num_str, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                rrddim_set_by_pointer(st_expired, rd_expired, dyn_rules_num[srn].expired_rules);
            }

            rrdset_done(st_active);
            rrdset_done(st_expired);
        }
    }

    return 0;
#else
    error("FREEBSD: ipfw charts supported for FreeBSD 11.0 and newer releases only");
    COMMON_IPFW_ERROR();
    return 1;
#endif
}
