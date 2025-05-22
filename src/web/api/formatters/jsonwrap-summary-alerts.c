// SPDX-License-Identifier: GPL-3.0-or-later

#include "jsonwrap.h"
#include "jsonwrap-internal.h"

void query_target_summary_alerts_v2(BUFFER *wb, QUERY_TARGET *qt, const char *key) {
    buffer_json_member_add_array(wb, key);
    QUERY_ALERTS_COUNTS *z;

    DICTIONARY *dict = dictionary_create(DICT_OPTION_SINGLE_THREADED | DICT_OPTION_DONT_OVERWRITE_VALUE);
    for (long c = 0; c < (long) qt->instances.used; c++) {
        QUERY_INSTANCE *qi = query_instance(qt, c);
        RRDSET *st = rrdinstance_acquired_rrdset(qi->ria);
        if (st) {
            rw_spinlock_read_lock(&st->alerts.spinlock);
            if (st->alerts.base) {
                for (RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
                    z = dictionary_set(dict, string2str(rc->config.name), NULL, sizeof(*z));

                    switch(rc->status) {
                        case RRDCALC_STATUS_CLEAR:
                            z->clear++;
                            break;

                        case RRDCALC_STATUS_WARNING:
                            z->warning++;
                            break;

                        case RRDCALC_STATUS_CRITICAL:
                            z->critical++;
                            break;

                        default:
                        case RRDCALC_STATUS_UNINITIALIZED:
                        case RRDCALC_STATUS_UNDEFINED:
                        case RRDCALC_STATUS_REMOVED:
                            z->other++;
                            break;
                    }
                }
            }
            rw_spinlock_read_unlock(&st->alerts.spinlock);
        }
    }
    dfe_start_read(dict, z)
        query_target_alerts_counts(wb, z, z_dfe.name, true);
    dfe_done(z);
    dictionary_destroy(dict);
    buffer_json_array_close(wb); // alerts
}
