// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_TYPE type;
    const char *name;
} dyncfg_types[] = {
    { .type = DYNCFG_TYPE_SINGLE, .name = "single" },
    { .type = DYNCFG_TYPE_TEMPLATE, .name = "template" },
    { .type = DYNCFG_TYPE_JOB, .name = "job" },
};

DYNCFG_TYPE dyncfg_type2id(const char *type) {
    if(!type || !*type)
        return DYNCFG_TYPE_SINGLE;

    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_types[i].name, type) == 0)
            return dyncfg_types[i].type;
    }

    return DYNCFG_TYPE_SINGLE;
}

const char *dyncfg_id2type(DYNCFG_TYPE type) {
    size_t entries = sizeof(dyncfg_types) / sizeof(dyncfg_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(type == dyncfg_types[i].type)
            return dyncfg_types[i].name;
    }

    return "single";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_SOURCE_TYPE source_type;
    const char *name;
} dyncfg_source_types[] = {
    { .source_type = DYNCFG_SOURCE_TYPE_INTERNAL, .name = "internal" },
    { .source_type = DYNCFG_SOURCE_TYPE_STOCK, .name = "stock" },
    { .source_type = DYNCFG_SOURCE_TYPE_USER, .name = "user" },
    { .source_type = DYNCFG_SOURCE_TYPE_DYNCFG, .name = "dyncfg" },
    { .source_type = DYNCFG_SOURCE_TYPE_DISCOVERED, .name = "discovered" },
};

DYNCFG_SOURCE_TYPE dyncfg_source_type2id(const char *source_type) {
    if(!source_type || !*source_type)
        return DYNCFG_SOURCE_TYPE_STOCK;

    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_source_types[i].name, source_type) == 0)
            return dyncfg_source_types[i].source_type;
    }

    return DYNCFG_SOURCE_TYPE_STOCK;
}

const char *dyncfg_id2source_type(DYNCFG_SOURCE_TYPE source_type) {
    size_t entries = sizeof(dyncfg_source_types) / sizeof(dyncfg_source_types[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(source_type == dyncfg_source_types[i].source_type)
            return dyncfg_source_types[i].name;
    }

    return "stock";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_STATUS status;
    const char *name;
} dyncfg_statuses[] = {
    { .status = DYNCFG_STATUS_OK, .name = "ok" },
    { .status = DYNCFG_STATUS_DISABLED, .name = "disabled" },
    { .status = DYNCFG_STATUS_ORPHAN, .name = "orphan" },
    { .status = DYNCFG_STATUS_REJECTED, .name = "rejected" },
};

DYNCFG_STATUS dyncfg_status2id(const char *status) {
    if(!status || !*status)
        return DYNCFG_STATUS_OK;

    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(strcmp(dyncfg_statuses[i].name, status) == 0)
            return dyncfg_statuses[i].status;
    }

    return DYNCFG_STATUS_OK;
}

const char *dyncfg_id2status(DYNCFG_STATUS status) {
    size_t entries = sizeof(dyncfg_statuses) / sizeof(dyncfg_statuses[0]);
    for(size_t i = 0; i < entries ;i++) {
        if(status == dyncfg_statuses[i].status)
            return dyncfg_statuses[i].name;
    }

    return "ok";
}

// ----------------------------------------------------------------------------

static struct {
    DYNCFG_CMDS cmd;
    const char *name;
} cmd_map[] = {
    { .cmd = DYNCFG_CMD_GET, .name = "get" },
    { .cmd = DYNCFG_CMD_SCHEMA, .name = "schema" },
    { .cmd = DYNCFG_CMD_UPDATE, .name = "update" },
    { .cmd = DYNCFG_CMD_ADD, .name = "add" },
    { .cmd = DYNCFG_CMD_TEST, .name = "test" },
    { .cmd = DYNCFG_CMD_REMOVE, .name = "remove" },
    { .cmd = DYNCFG_CMD_ENABLE, .name = "enable" },
    { .cmd = DYNCFG_CMD_DISABLE, .name = "disable" },
    { .cmd = DYNCFG_CMD_RESTART, .name = "restart" }
};

DYNCFG_CMDS dyncfg_cmds2id(const char *cmds) {
    DYNCFG_CMDS result = DYNCFG_CMD_NONE;
    const char *p = cmds;
    size_t len, i;

    while (*p) {
        // Skip any leading spaces
        while (*p == ' ') p++;

        // Find the end of the current word
        const char *end = p;
        while (*end && *end != ' ') end++;
        len = end - p;

        // Compare with known commands
        for (i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
            if (strncmp(p, cmd_map[i].name, len) == 0 && cmd_map[i].name[len] == '\0') {
                result |= cmd_map[i].cmd;
                break;
            }
        }

        // Move to the next word
        p = end;
    }

    return result;
}

void dyncfg_cmds2fp(DYNCFG_CMDS cmds, FILE *fp) {
    fprintf(fp, "cmds=");
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd)
            fprintf(fp, "%s ", cmd_map[i].name);
    }
    fprintf(fp, "\n");
}

void dyncfg_cmds2json_array(DYNCFG_CMDS cmds, const char *key, BUFFER *wb) {
    buffer_json_member_add_array(wb, key);
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd)
            buffer_json_add_array_item_string(wb, cmd_map[i].name);
    }
    buffer_json_array_close(wb);
}

void dyncfg_cmds2buffer(DYNCFG_CMDS cmds, BUFFER *wb) {
    size_t added = 0;
    for (size_t i = 0; i < sizeof(cmd_map) / sizeof(cmd_map[0]); i++) {
        if(cmds & cmd_map[i].cmd) {
            if(added)
                buffer_fast_strcat(wb, " ", 1);

            buffer_strcat(wb, cmd_map[i].name);
            added++;
        }
    }
}

// ----------------------------------------------------------------------------

bool dyncfg_is_valid_id(const char *id) {
    const char *s = id;

    while(*s) {
        if(isspace(*s)) return false;
        s++;
    }

    return true;
}

char *dyncfg_escape_id(const char *id) {
    if (id == NULL) return NULL;

    // Allocate memory for the worst case, where every character is escaped.
    char *escaped = mallocz(strlen(id) * 3 + 1); // Each char can become '%XX', plus '\0'
    if (!escaped) return NULL;

    const char *src = id;
    char *dest = escaped;

    while (*src) {
        if (*src == '/' || isspace(*src) || !isprint(*src)) {
            sprintf(dest, "%%%02X", (unsigned char)*src);
            dest += 3;
        } else {
            *dest++ = *src;
        }
        src++;
    }

    *dest = '\0';
    return escaped;
}

// ----------------------------------------------------------------------------

