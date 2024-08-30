// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef LIBNETDATA_DYNCFG_H
#define LIBNETDATA_DYNCFG_H

#define DYNCFG_VERSION (size_t)1

#define DYNCFG_RESP_SUCCESS(code) (code >= 200 && code <= 299)
#define DYNCFG_RESP_RUNNING                   200 // accepted and running
#define DYNCFG_RESP_ACCEPTED                  202 // accepted, but not running yet
#define DYNCFG_RESP_ACCEPTED_DISABLED         298 // accepted, but is disabled
#define DYNCFG_RESP_ACCEPTED_RESTART_REQUIRED 299 // accepted, but restart is required to apply it

typedef enum __attribute__((packed)) {
    DYNCFG_TYPE_SINGLE = 0,
    DYNCFG_TYPE_TEMPLATE,
    DYNCFG_TYPE_JOB,
} DYNCFG_TYPE;
DYNCFG_TYPE dyncfg_type2id(const char *type);
const char *dyncfg_id2type(DYNCFG_TYPE type);

typedef enum __attribute__((packed)) {
    DYNCFG_SOURCE_TYPE_INTERNAL = 0,
    DYNCFG_SOURCE_TYPE_STOCK,
    DYNCFG_SOURCE_TYPE_USER,
    DYNCFG_SOURCE_TYPE_DYNCFG,
    DYNCFG_SOURCE_TYPE_DISCOVERED,
} DYNCFG_SOURCE_TYPE;
DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type);
const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type);

typedef enum __attribute__((packed)) {
    DYNCFG_STATUS_NONE = 0,
    DYNCFG_STATUS_ACCEPTED,     // the plugin has accepted the configuration
    DYNCFG_STATUS_RUNNING,      // the plugin runs the accepted configuration
    DYNCFG_STATUS_FAILED,       // the plugin fails to run the accepted configuration
    DYNCFG_STATUS_DISABLED,     // the configuration is disabled by a user
    DYNCFG_STATUS_ORPHAN,       // no plugin has claimed this configurations
    DYNCFG_STATUS_INCOMPLETE,   // a special kind of failed configuration
} DYNCFG_STATUS;
DYNCFG_STATUS dyncfg_status2id(const char *status);
const char *dyncfg_id2status(DYNCFG_STATUS status);

typedef enum __attribute__((packed)) {
    DYNCFG_CMD_NONE         = 0,
    DYNCFG_CMD_GET          = (1 << 0),
    DYNCFG_CMD_SCHEMA       = (1 << 1),
    DYNCFG_CMD_UPDATE       = (1 << 2),
    DYNCFG_CMD_ADD          = (1 << 3),
    DYNCFG_CMD_TEST         = (1 << 4),
    DYNCFG_CMD_REMOVE       = (1 << 5),
    DYNCFG_CMD_ENABLE       = (1 << 6),
    DYNCFG_CMD_DISABLE      = (1 << 7),
    DYNCFG_CMD_RESTART      = (1 << 8),
    DYNCFG_CMD_USERCONFIG   = (1 << 9),
} DYNCFG_CMDS;

DYNCFG_CMDS dyncfg_cmds2id(const char *cmds);
void dyncfg_cmds2buffer(DYNCFG_CMDS cmds, struct web_buffer *wb);
void dyncfg_cmds2json_array(DYNCFG_CMDS cmds, const char *key, struct web_buffer *wb);
void dyncfg_cmds2fp(DYNCFG_CMDS cmds, FILE *fp);
const char *dyncfg_id2cmd_one(DYNCFG_CMDS cmd);

bool dyncfg_is_valid_id(const char *id);
char *dyncfg_escape_id_for_filename(const char *id);

#include "../clocks/clocks.h"
#include "../buffer/buffer.h"
#include "../dictionary/dictionary.h"

typedef int (*dyncfg_cb_t)(const char *transaction, const char *id, DYNCFG_CMDS cmd, const char *add_name,
                           BUFFER *payload, usec_t *stop_monotonic_ut, bool *cancelled, BUFFER *result,
                           HTTP_ACCESS access, const char *source, void *data);

struct dyncfg_node {
    DYNCFG_TYPE type;
    DYNCFG_CMDS cmds;
    dyncfg_cb_t cb;
    void *data;
};

#define dyncfg_nodes_dictionary_create() dictionary_create_advanced(DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct dyncfg_node))

int dyncfg_default_response(BUFFER *wb, int code, const char *msg);

int dyncfg_node_find_and_call(DICTIONARY *dyncfg_nodes, const char *transaction, const char *function,
                              usec_t *stop_monotonic_ut, bool *cancelled,
                              BUFFER *payload, HTTP_ACCESS access, const char *source, BUFFER *result);

#endif //LIBNETDATA_DYNCFG_H
