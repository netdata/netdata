// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_METALOGPLUGINSD_H
#define NETDATA_METALOGPLUGINSD_H

#include "../../../collectors/plugins.d/pluginsd_parser.h"
#include "../../../collectors/plugins.d/plugins_d.h"
#include "../../../parser/parser.h"

struct metalog_pluginsd_state {
    struct metalog_instance *ctx;
    uuid_t uuid;
    uint8_t skip_record; /* skip this record due to errors in parsing */
    struct metadata_logfile *metalogfile; /* current metadata log file being replayed */
};

extern void metalog_pluginsd_state_init(struct metalog_pluginsd_state *state, struct metalog_instance *ctx);

extern PARSER_RC metalog_pluginsd_chart_action(void *user, char *type, char *id, char *name, char *family,
                                               char *context, char *title, char *units, char *plugin, char *module,
                                               int priority, int update_every, RRDSET_TYPE chart_type, char *options);
extern PARSER_RC metalog_pluginsd_dimension_action(void *user, RRDSET *st, char *id, char *name, char *algorithm,
                                                   long multiplier, long divisor, char *options,
                                                   RRD_ALGORITHM algorithm_type);
extern PARSER_RC metalog_pluginsd_guid_action(void *user, uuid_t *uuid);
extern PARSER_RC metalog_pluginsd_context_action(void *user, uuid_t *uuid);
extern PARSER_RC metalog_pluginsd_tombstone_action(void *user, uuid_t *uuid);
extern PARSER_RC metalog_pluginsd_host(char **words, void *user, PLUGINSD_ACTION  *plugins_action);
extern PARSER_RC metalog_pluginsd_host_action(void *user, char *machine_guid, char *hostname, char *registry_hostname, int update_every, char *os, char *timezone, char *tags);

#endif /* NETDATA_METALOGPLUGINSD_H */
