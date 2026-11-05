// SPDX-License-Identifier: GPL-3.0-or-later

// Re-matching alerts (label recheck, health reload, DynCfg save) must keep the alerts whose rule did not change:
// no transition, same status. Only alerts whose attaching rule changed are replaced, removed or added.

#include "health_internals.h"
#include "health-alert-entry.h"
#include "health-rematch-unittest.h"

#define REMATCH_CHART_TYPE "rematch_test"
#define REMATCH_CHART_ID   "data"
#define REMATCH_CONTEXT    "rematch_test.data"

// alert A has two rules, picked by the chart label rematch_mode; alert B matches every chart of the context.
// Chart labels keep the test off localhost's labels; the re-match treats host and chart label changes alike.
#define REMATCH_RULE(chart_labels, warn)                                                                             \
    "{\"enabled\":true,\"type\":\"template\",\"config\":{"                                                        \
    "\"match\":{\"on\":\"" REMATCH_CONTEXT "\",\"host_labels\":\"*\",\"instance_labels\":\"" chart_labels "\"},"   \
    "\"value\":{\"database_lookup\":{\"after\":-60,\"before\":0,\"time_group\":\"average\","                      \
    "\"dims_group\":\"sum\",\"data_source\":\"samples\",\"options\":[],\"dimensions\":\"\"},\"update_every\":10},"\
    "\"conditions\":{\"warning_condition\":\"$this > " warn "\",\"critical_condition\":\"\"}}}"

#define REMATCH_PAYLOAD_A                                                                                           \
    "{\"format_version\":1,\"rules\":[" REMATCH_RULE("rematch_mode=one", "1") "," REMATCH_RULE("rematch_mode=two", "2") "]}"
#define REMATCH_PAYLOAD_B   "{\"format_version\":1,\"rules\":[" REMATCH_RULE("*", "5") "]}"
#define REMATCH_PAYLOAD_B2  "{\"format_version\":1,\"rules\":[" REMATCH_RULE("*", "6") "]}"

static int rematch_failed = 0;
static int rematch_passed = 0;

static void rematch_check(bool ok, const char *what) {
    if(ok)
        rematch_passed++;
    else {
        rematch_failed++;
        fprintf(stderr, "FAILED [health rematch]: %s\n", what);
    }
}

static RRDCALC *rematch_alert(RRDSET *st, const char *name) {
    RRDCALC *found = NULL;
    rw_spinlock_read_lock(&st->alerts.spinlock);
    for(RRDCALC *rc = st->alerts.base; rc; rc = rc->next) {
        if(!strcmp(rrdcalc_name(rc), name)) {
            found = rc;
            break;
        }
    }
    rw_spinlock_read_unlock(&st->alerts.spinlock);
    return found;
}

static uint64_t rematch_transitions(void) {
    return __atomic_load_n(&localhost->health_transitions, __ATOMIC_RELAXED);
}

// a raised alert: a reset would record WARNING -> REMOVED and start over from UNINITIALIZED
static void rematch_raise(RRDCALC *rc) {
    if(rc)
        rc->status = RRDCALC_STATUS_WARNING;
}

static bool rematch_kept(RRDSET *st, const char *name) {
    RRDCALC *rc = rematch_alert(st, name);
    return rc && rc->status == RRDCALC_STATUS_WARNING;
}

static int rematch_dyncfg(DYNCFG_CMDS cmd, const char *id, const char *add_name, const char *payload) {
    CLEAN_BUFFER *wb = payload ? buffer_create(0, NULL) : NULL;
    CLEAN_BUFFER *result = buffer_create(0, NULL);
    if(wb)
        buffer_strcat(wb, payload);

    return dyncfg_health_cb(NULL, id, cmd, add_name, wb, NULL, NULL, result, HTTP_ACCESS_NONE, "unittest", NULL);
}

static void rematch_set_mode(RRDSET *st, const char *mode) {
    rrdlabels_add(st->rrdlabels, "rematch_mode", mode, RRDLABEL_SRC_CONFIG);
}

// the latest log entry of the alert, which a pending notification would come from
static ALARM_ENTRY *rematch_latest_entry(RRDCALC *rc) {
    ALARM_ENTRY *found = NULL;
    rw_spinlock_read_lock(&localhost->health_log.spinlock);
    for(ALARM_ENTRY *ae = localhost->health_log.alarms; ae; ae = ae->next) {
        if(ae->alarm_id == rc->id) {
            found = ae;
            break;
        }
    }
    rw_spinlock_read_unlock(&localhost->health_log.spinlock);
    return found;
}

int health_rematch_unittest(void) {
    fprintf(stderr, "\nStarting health alert re-match unit tests\n");
    fprintf(stderr, "=========================================\n\n");

    rematch_failed = rematch_passed = 0;

    // the alert entry allocator and the prototypes; with health disabled (unit tests) no configuration is loaded
    bool initialized_prototypes = !health_globals.prototypes.dict;
    health_plugin_init();

    bool saved_enabled = localhost->health.enabled;
    bool saved_initialized = rrdhost_flag_check(localhost, RRDHOST_FLAG_INITIALIZED_HEALTH);
    localhost->health.enabled = true;
    rrdhost_flag_set(localhost, RRDHOST_FLAG_INITIALIZED_HEALTH);
    rrdcalc_rrdhost_index_init(localhost);

    RRDSET *st = rrdset_create_localhost(REMATCH_CHART_TYPE, REMATCH_CHART_ID, NULL, "rematch", REMATCH_CONTEXT,
                                         "Re-match test", "value", "unittest", NULL, 1, 1, RRDSET_TYPE_LINE);
    rematch_set_mode(st, "one");

    rematch_check(rematch_dyncfg(DYNCFG_CMD_ADD, "health:alert:prototype", "rematch_a", REMATCH_PAYLOAD_A) == DYNCFG_RESP_ACCEPTED,
                  "DynCfg add of rematch_a");
    rematch_check(rematch_dyncfg(DYNCFG_CMD_ADD, "health:alert:prototype", "rematch_b", REMATCH_PAYLOAD_B) == DYNCFG_RESP_ACCEPTED,
                  "DynCfg add of rematch_b");
    rematch_check(rematch_alert(st, "rematch_a") && rematch_alert(st, "rematch_b"), "both alerts attached");

    uint64_t before;
    rematch_raise(rematch_alert(st, "rematch_a"));
    rematch_raise(rematch_alert(st, "rematch_b"));

    // AC1: a label change that changes no alert's rule writes nothing and keeps the alerts
    rrdlabels_add(st->rrdlabels, "rematch_other", "x", RRDLABEL_SRC_CONFIG);
    before = rematch_transitions();
    health_prototype_rematch_rrdset(st);
    rematch_check(rematch_transitions() == before, "AC1 unchanged labels: no transitions");
    rematch_check(rematch_kept(st, "rematch_a") && rematch_kept(st, "rematch_b"), "AC1 unchanged labels: alerts kept raised");

    // AC2: another rule of A wins - A is replaced, B is kept
    nd_uuid_t hash_one;
    uuid_copy(hash_one, rematch_alert(st, "rematch_a")->config.hash_id);
    rematch_set_mode(st, "two");
    before = rematch_transitions();
    health_prototype_rematch_rrdset(st);
    RRDCALC *a = rematch_alert(st, "rematch_a");
    rematch_check(a && !uuid_eq(a->config.hash_id, hash_one) && a->status != RRDCALC_STATUS_WARNING,
                  "AC2 other rule wins: alert replaced");
    rematch_check(rematch_kept(st, "rematch_b"), "AC2 other rule wins: unrelated alert kept");
    rematch_check(rematch_transitions() > before, "AC2 other rule wins: transitions recorded");

    // AC2: no rule of A matches - A is removed; then a rule matches again - A is added
    rematch_set_mode(st, "none");
    health_prototype_rematch_rrdset(st);
    rematch_check(!rematch_alert(st, "rematch_a") && rematch_kept(st, "rematch_b"), "AC2 no rule: alert removed");
    rematch_set_mode(st, "one");
    health_prototype_rematch_rrdset(st);
    rematch_check(rematch_alert(st, "rematch_a") && rematch_kept(st, "rematch_b"), "AC2 matching again: alert added");
    rematch_raise(rematch_alert(st, "rematch_a"));

    // AC3: reload with unchanged rules writes nothing, keeps the alerts and their pending notifications
    ALARM_ENTRY *pending = rematch_latest_entry(rematch_alert(st, "rematch_a"));
    rematch_check(pending && !(pending->flags & HEALTH_ENTRY_FLAG_UPDATED), "AC3 setup: latest entry not superseded");
    before = rematch_transitions();
    health_apply_prototypes_to_host(localhost);
    rematch_check(rematch_transitions() == before, "AC3 reload: no transitions");
    rematch_check(rematch_kept(st, "rematch_a") && rematch_kept(st, "rematch_b"), "AC3 reload: alerts kept raised");
    rematch_check(pending && !(pending->flags & HEALTH_ENTRY_FLAG_UPDATED), "AC3 reload: pending entry not superseded");

    // AC4: DynCfg save of an unchanged rule writes nothing on the host
    before = rematch_transitions();
    rematch_check(rematch_dyncfg(DYNCFG_CMD_UPDATE, "health:alert:prototype:rematch_b", NULL, REMATCH_PAYLOAD_B) == DYNCFG_RESP_ACCEPTED,
                  "AC4 DynCfg identical update accepted");
    rematch_check(rematch_transitions() == before, "AC4 DynCfg identical update: no transitions");
    rematch_check(rematch_kept(st, "rematch_a") && rematch_kept(st, "rematch_b"), "AC4 DynCfg identical update: alerts kept");

    // AC4: a changed rule replaces only its own alert
    rematch_check(rematch_dyncfg(DYNCFG_CMD_UPDATE, "health:alert:prototype:rematch_b", NULL, REMATCH_PAYLOAD_B2) == DYNCFG_RESP_ACCEPTED,
                  "AC4 DynCfg changed update accepted");
    RRDCALC *b = rematch_alert(st, "rematch_b");
    rematch_check(b && b->status != RRDCALC_STATUS_WARNING, "AC4 DynCfg changed update: alert replaced");
    rematch_check(rematch_kept(st, "rematch_a"), "AC4 DynCfg changed update: other alert kept");

    // AC4: disable removes, enable adds back
    rematch_dyncfg(DYNCFG_CMD_DISABLE, "health:alert:prototype:rematch_b", NULL, NULL);
    rematch_check(!rematch_alert(st, "rematch_b") && rematch_kept(st, "rematch_a"), "AC4 DynCfg disable: alert removed");
    rematch_dyncfg(DYNCFG_CMD_ENABLE, "health:alert:prototype:rematch_b", NULL, NULL);
    rematch_check(rematch_alert(st, "rematch_b") && rematch_kept(st, "rematch_a"), "AC4 DynCfg enable: alert added");

    // AC7: a kept alert takes the current rule's provenance
    {
        const DICTIONARY_ITEM *item = dictionary_get_and_acquire_item(health_globals.prototypes.dict, "rematch_a");
        RRD_ALERT_PROTOTYPE *ap = item ? dictionary_acquired_item_value(item) : NULL;
        if(ap) {
            rw_spinlock_write_lock(&ap->_internal.rw_spinlock);
            for(RRD_ALERT_PROTOTYPE *t = ap; t; t = t->_internal.next) {
                string_freez(t->config.source);
                t->config.source = string_strdupz("rematch-unittest:42");
            }
            rw_spinlock_write_unlock(&ap->_internal.rw_spinlock);
            dictionary_acquired_item_release(health_globals.prototypes.dict, item);
        }
        health_prototype_rematch_rrdset(st);
        a = rematch_alert(st, "rematch_a");
        rematch_check(ap && a && rematch_kept(st, "rematch_a") && !strcmp(string2str(a->config.source), "rematch-unittest:42"),
                      "AC7 kept alert takes the rule's source");
    }

    // a chart id longer than the alert key: two ids that differ only past the key length share a key, and
    // re-matching one chart must not delete the alert linked to the other
    {
        char id_a[1101], id_b[1101];
        memset(id_a, 'x', sizeof(id_a) - 2); id_a[sizeof(id_a) - 2] = 'a'; id_a[sizeof(id_a) - 1] = '\0';
        memset(id_b, 'x', sizeof(id_b) - 2); id_b[sizeof(id_b) - 2] = 'b'; id_b[sizeof(id_b) - 1] = '\0';

        RRDSET *sa = rrdset_create_localhost(REMATCH_CHART_TYPE, id_a, NULL, "rematch", REMATCH_CONTEXT,
                                             "Re-match test", "value", "unittest", NULL, 1, 1, RRDSET_TYPE_LINE);
        RRDSET *sb = rrdset_create_localhost(REMATCH_CHART_TYPE, id_b, NULL, "rematch", REMATCH_CONTEXT,
                                             "Re-match test", "value", "unittest", NULL, 1, 1, RRDSET_TYPE_LINE);
        rematch_set_mode(sa, "one");
        rematch_set_mode(sb, "none");
        health_prototype_rematch_rrdset(sa);
        rematch_raise(rematch_alert(sa, "rematch_a"));
        health_prototype_rematch_rrdset(sb);
        rematch_check(rematch_kept(sa, "rematch_a"), "long chart ids: re-matching a chart keeps the other chart's alert");
        rrdcalc_unlink_and_delete_all_rrdset_alerts(sa);
        rrdcalc_unlink_and_delete_all_rrdset_alerts(sb);
    }

    // AC3: a rule gone from the prototypes (as after a reload without it) loses its alert, the others are kept
    dictionary_del(health_globals.prototypes.dict, "rematch_b");
    health_apply_prototypes_to_host(localhost);
    rematch_check(!rematch_alert(st, "rematch_b") && rematch_kept(st, "rematch_a"), "AC3 reload without a rule: its alert removed");

    // cleanup
    rematch_dyncfg(DYNCFG_CMD_REMOVE, "health:alert:prototype:rematch_a", NULL, NULL);
    rrdcalc_unlink_and_delete_all_rrdset_alerts(st);
    localhost->health.enabled = saved_enabled;
    if(!saved_initialized)
        rrdhost_flag_clear(localhost, RRDHOST_FLAG_INITIALIZED_HEALTH);

    if(initialized_prototypes) {
        health_dyncfg_unregister_all_prototypes();
        dictionary_destroy(health_globals.prototypes.dict);
        health_globals.prototypes.dict = NULL;
    }

    fprintf(stderr, "\nhealth re-match tests: %d passed, %d failed\n", rematch_passed, rematch_failed);
    return rematch_failed ? 1 : 0;
}
