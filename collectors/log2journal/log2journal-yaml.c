// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

#define YAML_MAX_LINE (1024 * 64)
#define YAML_MAX_NESTING 1024

// ----------------------------------------------------------------------------
// hashtable for CFG_KEY

// cleanup hashtable defines
#undef SIMPLE_HASHTABLE_SORT_FUNCTION
#undef SIMPLE_HASHTABLE_VALUE_TYPE
#undef SIMPLE_HASHTABLE_NAME
#undef NETDATA_SIMPLE_HASHTABLE_H

struct cfg_node;
static inline int compare_cfg_nodes(struct cfg_node *n1, struct cfg_node *n2);
#define SIMPLE_HASHTABLE_SORT_FUNCTION compare_cfg_nodes
#define SIMPLE_HASHTABLE_VALUE_TYPE struct cfg_node
#define SIMPLE_HASHTABLE_NAME _CFG_NODE
#define SIMPLE_HASHTABLE_SAMPLE_IMPLEMENTATION 1
#include "../../libnetdata/simple_hashtable.h"

// ----------------------------------------------------------------------------

typedef struct cfg_node CFG_NODE;
typedef struct cfg_value CFG_VALUE;

static inline void cfg_value_cleanup(CFG_VALUE *v);
static inline XXH64_hash_t cfg_node_hash(CFG_NODE *n);

// ----------------------------------------------------------------------------

typedef struct cfg_key {
    char *name;
    XXH64_hash_t hash;
    size_t len;
} CFG_KEY;

static inline void cfg_key_cleanup(CFG_KEY *k) {
    assert(k);
    freez(k->name);

    memset(k, 0, sizeof(*k));
}

static inline void cfg_key_init(CFG_KEY *k, const char *key, size_t key_len) {
    assert(key);

    cfg_key_cleanup(k);

    if(key && *key && key_len) {
        k->name = strndupz(key, key_len);
        k->len = key_len;
        k->hash = XXH3_64bits(k->name, k->len);
    }
}

static inline XXH64_hash_t cfg_key_hash(CFG_KEY *k) {
    return k->hash;
}

// ----------------------------------------------------------------------------

typedef enum {
    CFG_NODE_ID_TYPE_NONE = 0,
    CFG_NODE_ID_TYPE_NAMED,
    CFG_NODE_ID_TYPE_NUMBERED,
} CFG_NODE_ID_TYPE;

typedef struct cfg_node_id {
    CFG_NODE_ID_TYPE type;
    union {
        CFG_KEY key;
        uint32_t number;
    };
} CFG_NODE_ID;

static inline void cfg_node_id_cleanup(CFG_NODE_ID *id) {
    assert(id);

    if(id->type == CFG_NODE_ID_TYPE_NAMED)
        cfg_key_cleanup(&id->key);

    memset(id, 0, sizeof(*id));
}

static inline bool cfg_node_id_set_named(CFG_NODE_ID *id, const char *key, size_t key_len) {
    assert(id);

    if(id->type != CFG_NODE_ID_TYPE_NONE)
        return false;

    cfg_node_id_cleanup(id);

    id->type = CFG_NODE_ID_TYPE_NAMED;
    cfg_key_init(&id->key, key, key_len);
    return true;
}

static inline bool cfg_node_id_set_numbered(CFG_NODE_ID *id, size_t number) {
    assert(id);

    if(id->type != CFG_NODE_ID_TYPE_NONE)
        return false;

    cfg_node_id_cleanup(id);

    id->type = CFG_NODE_ID_TYPE_NUMBERED;
    id->number = number;
    return true;
}

static inline XXH64_hash_t cfg_node_id_hash(CFG_NODE_ID *id) {
    return cfg_key_hash(&id->key);
}

// ----------------------------------------------------------------------------

typedef struct cfg_value_map {
    SIMPLE_HASHTABLE_CFG_NODE hashtable;
} CFG_VALUE_MAP;

static inline void cfg_value_map_cleanup(CFG_VALUE_MAP *map);
static inline void cfg_value_map_init(CFG_VALUE_MAP *map) {
    assert(map);
    cfg_value_map_cleanup(map);
    simple_hashtable_init_CFG_NODE(&map->hashtable, 1);
}

static inline void cfg_value_map_add_child(CFG_VALUE_MAP *map, CFG_NODE *child) {
    SIMPLE_HASHTABLE_SLOT_CFG_NODE *sl = simple_hashtable_get_slot_CFG_NODE(&map->hashtable, cfg_node_hash(child), true);
    simple_hashtable_set_slot_CFG_NODE(&map->hashtable, sl, cfg_node_hash(child), child);
}

// ----------------------------------------------------------------------------

typedef struct cfg_value_array {
    uint32_t size;
    uint32_t used;
    CFG_NODE **array;
} CFG_VALUE_ARRAY;

static inline void cfg_value_array_cleanup(CFG_VALUE_ARRAY *arr);
static inline void cfg_value_array_init(CFG_VALUE_ARRAY *arr) {
    assert(arr);
    cfg_value_array_cleanup(arr);
    // no need to initialize anything
}

static inline void cfg_value_array_add_child(CFG_VALUE_ARRAY *arr, CFG_NODE *child);

// ----------------------------------------------------------------------------

typedef enum {
    CFG_NON = 0,
    CFG_TXT,
    CFG_U64,
    CFG_I64,
    CFG_DBL,
    CFG_BLN,
    CFG_MAP,
    CFG_ARR,
    CFG_LNK,
} CFG_VALUE_TYPE;

struct cfg_value {
    CFG_VALUE_TYPE type;
    union {
        char           *txt;  // for strings
        uint64_t        u64;
        int64_t         i64;
        double          dbl;  // for doubles
        bool            bln;  // for booleans
        CFG_VALUE_MAP   map;
        CFG_VALUE_ARRAY arr;
    } value;
};

static inline const char *cfg_value_type(CFG_VALUE_TYPE type) {
    switch(type) {
        default:
        case CFG_NON:
            return "empty";

        case CFG_TXT:
            return "text";

        case CFG_U64:
            return "unsigned integer";

        case CFG_I64:
            return "signed integer";

        case CFG_DBL:
            return "double";

        case CFG_BLN:
            return "boolean";

        case CFG_MAP:
            return "map";

        case CFG_ARR:
            return "array";

        case CFG_LNK:
            return "link";
    }
}

static inline void cfg_value_cleanup(CFG_VALUE *v) {
    assert(v);

    switch(v->type) {
        case CFG_TXT:
            freez(v->value.txt);
            v->value.txt = NULL;
            break;

        case CFG_NON:
        case CFG_U64:
        case CFG_I64:
        case CFG_DBL:
        case CFG_BLN:
            break;

        case CFG_MAP:
            cfg_value_map_cleanup(&v->value.map);
            break;

        case CFG_ARR:
            cfg_value_array_cleanup(&v->value.arr);
            break;

        case CFG_LNK:
            // FIXME implement link cleanup
            break;
    }

    memset(v, 0, sizeof(*v));
}

static inline bool cfg_value_make_array(CFG_VALUE *v) {
    if(v->type != CFG_NON && v->type != CFG_ARR)
        return false;

    if(v->type == CFG_ARR)
        return true;

    v->type = CFG_ARR;
    cfg_value_array_init(&v->value.arr);
    return true;
}

static inline bool cfg_value_make_map(CFG_VALUE *v) {
    if(v->type != CFG_NON && v->type != CFG_MAP)
        return false;

    if(v->type == CFG_MAP)
        return true;

    v->type = CFG_MAP;
    cfg_value_map_init(&v->value.map);
    return true;
}

static inline bool cfg_value_done(CFG_VALUE *v) {
    return v->type != CFG_NON;
}

static inline bool cfg_value_add_child(CFG_VALUE *v, CFG_NODE *child);

static inline bool cfg_value_set_literal(CFG_VALUE *v, const char *s, size_t len) {
    assert(v || s || len);

    if(v->type != CFG_NON)
        return false;

    cfg_value_cleanup(v);

    v->type = CFG_TXT;
    v->value.txt = strndupz(s, len);

    // FIXME the text at this point includes everything including Block Scalars
    // so, this needs to be parsed and the text needs to be extracted to support
    // the full feature set of YAML.
    // Also, we should automatically convert the literal to the right type,
    // based on the value we find.

    return true;
}

// ----------------------------------------------------------------------------

struct cfg_node {
    CFG_NODE_ID id;
    CFG_VALUE value;       // Node value
};

static inline void cfg_node_cleanup(CFG_NODE *n) {
    assert(n);

    cfg_node_id_cleanup(&n->id);
    cfg_value_cleanup(&n->value);
    memset(n, 0, sizeof(*n));
}

static inline void cfg_node_init(CFG_NODE *n) {
    assert(n);
    cfg_node_cleanup(n);
}

static inline CFG_NODE *cfg_node_create(void) {
    return callocz(1, sizeof(CFG_NODE));
}

static inline void cfg_node_free(CFG_NODE *n) {
    if(!n) return;

    cfg_node_cleanup(n);
    freez(n);
}

static inline bool cfg_node_make_array(CFG_NODE *n) {
    return cfg_value_make_array(&n->value);
}

static inline bool cfg_node_make_map(CFG_NODE *n) {
    return cfg_value_make_map(&n->value);
}

static inline bool cfg_node_done(CFG_NODE *n) {
    return cfg_value_done(&n->value);
}

static inline bool cfg_node_add_child(CFG_NODE *n, CFG_NODE *child) {
    return cfg_value_add_child(&n->value, child);
}

static inline XXH64_hash_t cfg_node_hash(CFG_NODE *n) {
    return cfg_node_id_hash(&n->id);
}

static inline bool cfg_node_set_name(CFG_NODE *n, const char *key, size_t key_len) {
    return cfg_node_id_set_named(&n->id, key, key_len);
}

static inline bool cfg_node_set_literal(CFG_NODE *n, const char *s, size_t len) {
    return cfg_value_set_literal(&n->value, s, len);
}

static inline int compare_cfg_nodes(struct cfg_node *n1, struct cfg_node *n2) {
    assert(n1->id.type == n2->id.type);
    assert(n1->id.type == CFG_NODE_ID_TYPE_NAMED || n1->id.type == CFG_NODE_ID_TYPE_NUMBERED);

    if(n1->id.type == CFG_NODE_ID_TYPE_NAMED)
        return strcmp(n1->id.key.name, n2->id.key.name);

    int a = (int)n1->id.number;
    int b = (int)n2->id.number;

    return a - b;
}

// ----------------------------------------------------------------------------
// functions that need to have visibility on all structure definitions

static inline void cfg_value_array_cleanup(CFG_VALUE_ARRAY *arr) {
    assert(arr);
    for(size_t i = 0; i < arr->used ;i++) {
        cfg_node_cleanup(arr->array[i]);
        freez(arr->array[i]);
    }

    freez(arr->array);
    memset(arr, 0, sizeof(*arr));
}

static inline void cfg_value_map_cleanup(CFG_VALUE_MAP *map) {
    assert(map);

    SIMPLE_HASHTABLE_FOREACH_READ_ONLY(&map->hashtable, sl, _CFG_NODE) {
        CFG_NODE *n = SIMPLE_HASHTABLE_FOREACH_READ_ONLY_VALUE(sl);
        // the order of these statements is important!
        simple_hashtable_del_slot_CFG_NODE(&map->hashtable, sl); // remove any references to n
        cfg_node_cleanup(n); // cleanup all the internals of n
        freez(n); // free n
    }

    simple_hashtable_destroy_CFG_NODE(&map->hashtable);
    memset(map, 0, sizeof(*map));
}

static inline void cfg_value_array_add_child(CFG_VALUE_ARRAY *arr, CFG_NODE *child) {
    if(arr->size <= arr->used) {
        size_t new_size = arr->size ? arr->size * 2 : 1;
        arr->array = reallocz(arr->array, new_size * sizeof(CFG_NODE *));
        arr->size = new_size;
    }

    arr->array[arr->used++] = child;
}

static inline bool cfg_value_add_child(CFG_VALUE *v, CFG_NODE *child) {
    switch(v->type) {
        case CFG_ARR:
            if(child->id.type != CFG_NODE_ID_TYPE_NUMBERED) return false;
            cfg_value_array_add_child(&v->value.arr, child);
            return true;

        case CFG_MAP:
            if(child->id.type != CFG_NODE_ID_TYPE_NAMED) return false;
            cfg_value_map_add_child(&v->value.map, child);
            return true;

        default:
            return false;
    }

    return false;
}

// ----------------------------------------------------------------------------

typedef struct cfg {
    SIMPLE_HASHTABLE_CFG_NODE hashtable;
} CFG;

static inline void cfg_cleanup(CFG *cfg) {
    assert(cfg);

    SIMPLE_HASHTABLE_FOREACH_READ_ONLY(&cfg->hashtable, sl, _CFG_NODE) {
        CFG_NODE *n = SIMPLE_HASHTABLE_FOREACH_READ_ONLY_VALUE(sl);
        // the order of these statements is important!
        simple_hashtable_del_slot_CFG_NODE(&cfg->hashtable, sl); // remove any references to n
        cfg_node_cleanup(n); // cleanup all the internals of n
        freez(n); // free n
    }

    simple_hashtable_destroy_CFG_NODE(&cfg->hashtable);

    memset(cfg, 0, sizeof(*cfg));
}

static inline void cfg_init(CFG *cfg) {
    assert(cfg);

    cfg_cleanup(cfg);
    simple_hashtable_init_CFG_NODE(&cfg->hashtable, 1);
}

// ----------------------------------------------------------------------------

// returning true means "append another line to the buffer and run me again"
// remaining data are returned in endptr
static inline bool parse_literal_after_keyword(const char *s, size_t len, const char **endptr) {
    const char *e = &s[len - 1];

    // trim trailing spaces and newlines
    while(e > s && (isspace(*e) || *e == '\n' || *e == '\r')) e--;

    // trim leading spaces
    while(isspace(*s)) s++;

    switch(*s) {
        case '\'':
            // single quote, possibly multiline string without processing escapes
            // FIXME find a matching quote, otherwise we need more input

        case '"':
            // FIXME find a matching quote on the line
            // if found, ignore everything after that.
            // if not found, the last character should be a backslash
            // otherwise through an error
            if(*e == '\\')
                return true;

        case '|':
            // FIXME block scalar - we need to compare indentation of the lines to find where the scalar ends

        case '>':
            // FIXME block scalar - we need to compare indentation of the lines to find where the scalar ends

        case '-':
            // FIXME special single-item list

        case '[':
            // FIXME start of array

        case '{':
            // FIXME start of object

        case '&':
        case '*':

    }
}


// ----------------------------------------------------------------------------

struct yaml_parser_stack_entry {
    CFG_NODE *node;
    size_t indent;
};

typedef struct yaml_parser_state {
    const char *txt;

    struct {
        size_t line;
        const char *line_start;

        CFG_NODE *node;

        size_t pos;
        size_t indent;
    } current;

    struct {
        struct yaml_parser_stack_entry *array;
        size_t depth;
        size_t size;
    } stack;

    CFG *cfg;

    char error[1024];
} YAML_PARSER;

static inline void yaml_push(YAML_PARSER *yp) {
    assert(yp->current.node);
    assert(yp->stack.array[yp->stack.depth - 1].node != yp->current.node);

    if(yp->stack.depth >= yp->stack.size) {
        size_t size = yp->stack.size ? yp->stack.size * 2 : 2;
        yp->stack.array = reallocz(yp->stack.array, size * sizeof(*yp->stack.array));
        yp->stack.size = size;
    }

    yp->stack.array[yp->stack.depth++] = (struct yaml_parser_stack_entry) {
        .node = yp->current.node,
        .indent = yp->current.indent,
    };

    yp->current.node = NULL;
}

static inline void yaml_pop(YAML_PARSER *yp) {
    assert(yp->current.node != NULL);
    assert(yp->stack.depth);

    yp->stack.depth--;
    yp->current.node = yp->stack.array[yp->stack.depth].node;
    yp->current.indent = yp->stack.array[yp->stack.depth].indent;
}

static inline CFG_NODE *yaml_parent_node(YAML_PARSER *yp) {
    assert(yp->stack.depth);
    return yp->stack.array[yp->stack.depth - 1].node;
}

const char *yaml_current_pos(YAML_PARSER *yp) {
    return &yp->txt[yp->current.pos];
}

static inline bool yaml_is_line_done(YAML_PARSER *yp) {
    const char *s = yaml_current_pos(yp);
    return !*s || *s == '\n' || strncmp(s, "\r\n", 2) == 0;
}

static inline bool yaml_next_token_start(YAML_PARSER *yp) {
    const char *s = yaml_current_pos(yp);
    bool at_line_start = (s == yp->current.line_start);

    while(*s) {
        if(*s == '\n') {
            // line changed
            yp->current.line++;
            yp->current.line_start = ++s;
            yp->current.indent = 0;
            at_line_start = true;
        }

        else if(*s == ' ') {
            // a space
            if(at_line_start)
                yp->current.indent++;

            s++;
        }

        else if(*s == '#') {
            // skip the comment
            while(*s && *s != '\n')
                s++;
        }

        else
            break;
    }

    yp->current.pos += s - yaml_current_pos(yp);
    return *s != '\0';
}

static inline bool yaml_parse_string_literal(YAML_PARSER *yp, const char *stop_chars) {
    return false;
}

static inline bool yaml_parse_value(YAML_PARSER  *yp) {
    return yaml_parse_string_literal(yp, NULL);
}

static inline bool yaml_parse_keyword(YAML_PARSER *yp, const char *stop_chars) {
    return yaml_parse_string_literal(yp, stop_chars);
}

static inline CFG *cfg_parse_yaml(const char *txt) {
    YAML_PARSER yaml_parser = {
            .cfg = callocz(1, sizeof(CFG *)),
    };
    YAML_PARSER *yp = &yaml_parser;

    cfg_init(yp->cfg);

    // push the parent node into the stack
    yp->current.node = cfg_node_create();
    yaml_push(yp);

    yp->current.line_start = txt;
    yp->current.line = 1;
    while(yaml_next_token_start(yp)) {
        // expect a keyword or -

        const char *s = yaml_current_pos(yp);
        if(*s == '-') {
            // the parent node must be an array
            CFG_NODE *parent = yaml_parent_node(yp);
            if(!cfg_node_make_array(parent)) {
                if(!yp->error[0])
                    snprintf(yp->error, sizeof(yp->error), "parent object is a %s; cannot switch it to %s",
                             cfg_value_type(parent->value.type), cfg_value_type(CFG_ARR));
                goto cleanup;
            }

            yp->current.pos++;

            if(!yaml_next_token_start(yp)) {
                if(!yp->error[0])
                    snprintf(yp->error, sizeof(yp->error), "a - is handing around");
                goto cleanup;
            }
        }
        else {
            // the parent node must be a map
            CFG_NODE *parent = yaml_parent_node(yp);
            if(!cfg_node_make_map(parent)) {
                if(!yp->error[0])
                    snprintf(yp->error, sizeof(yp->error), "parent object is a %s; cannot switch it to %s",
                            cfg_value_type(parent->value.type), cfg_value_type(CFG_MAP));
                goto cleanup;
            }
        }

        if(!yaml_parse_keyword(yp, ":\n")) {
            if(!yp->error[0])
                snprintf(yp->error, sizeof(yp->error), "no keyword");
            goto cleanup;
        }

        if(!yaml_next_token_start(yp)) {
            if(!yp->error[0])
                snprintf(yp->error, sizeof(yp->error), "document ended without assigning value");
            goto cleanup;
        }

        if(!yaml_parse_value(yp)) {
            if(!yp->error[0])
                snprintf(yp->error, sizeof(yp->error), "cannot parse value");
            goto cleanup;
        }

        CFG_NODE *parent = yaml_parent_node(yp);
        if(!cfg_node_add_child(parent, yp->current.node)) {
            if(!yp->error[0])
                snprintf(yp->error, sizeof(yp->error), "parent node is %s and cannot accept children nodes.",
                         cfg_value_type(parent->value.type));

            goto cleanup;
        }

        yp->current.node = NULL;
    }

    if(yp->current.node) {
        if(!yp->error[0])
            snprintf(yp->error, sizeof(yp->error), "last node was not completed");

        goto cleanup;
    }

    return yp->cfg;

cleanup:
    log2stderr("YAML PARSER: at line %zu: %s", yp->current.line, yp->error);
    cfg_node_free(yp->current.node);
    cfg_cleanup(yp->cfg);
    freez(yp->cfg);
    return NULL;
}

static char *cfg_load_file(const char *filename) {
    FILE *file = fopen(filename, "rb");
    char *buffer;
    long length;

    if (file == NULL) {
        log2stderr("YAML: cannot open file '%s'", filename);
        return NULL;
    }

    // Seek to the end of the file to determine its size
    fseek(file, 0, SEEK_END);
    length = ftell(file);
    fseek(file, 0, SEEK_SET);

    // Allocate memory for the entire content
    buffer = mallocz(length + 1);

    // Read the file into the buffer
    if (fread(buffer, 1, length, file) != (size_t)length) {
        log2stderr("YAML: cannot read file '%s'", filename);
        free(buffer);
        fclose(file);
        return NULL;
    }

    // Null-terminate the buffer
    buffer[length] = '\0';

    fclose(file);
    return buffer;
}

static inline CFG *cfg_parse_yaml_file(const char *filename) {
    const char *s = cfg_load_file(filename);
    CFG *cfg = cfg_parse_yaml(s);
    freez((void *)s);
    return cfg;
}

// ----------------------------------------------------------------------------
// yaml configuration file

#ifdef HAVE_LIBYAML

static const char *yaml_event_name(yaml_event_type_t type) {
    switch (type) {
        case YAML_NO_EVENT:
            return "YAML_NO_EVENT";

        case YAML_SCALAR_EVENT:
            return "YAML_SCALAR_EVENT";

        case YAML_ALIAS_EVENT:
            return "YAML_ALIAS_EVENT";

        case YAML_MAPPING_START_EVENT:
            return "YAML_MAPPING_START_EVENT";

        case YAML_MAPPING_END_EVENT:
            return "YAML_MAPPING_END_EVENT";

        case YAML_SEQUENCE_START_EVENT:
            return "YAML_SEQUENCE_START_EVENT";

        case YAML_SEQUENCE_END_EVENT:
            return "YAML_SEQUENCE_END_EVENT";

        case YAML_STREAM_START_EVENT:
            return "YAML_STREAM_START_EVENT";

        case YAML_STREAM_END_EVENT:
            return "YAML_STREAM_END_EVENT";

        case YAML_DOCUMENT_START_EVENT:
            return "YAML_DOCUMENT_START_EVENT";

        case YAML_DOCUMENT_END_EVENT:
            return "YAML_DOCUMENT_END_EVENT";

        default:
            return "UNKNOWN";
    }
}

#define yaml_error(parser, event, fmt, args...) yaml_error_with_trace(parser, event, __LINE__, __FUNCTION__, __FILE__, fmt, ##args)
static void yaml_error_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file, const char *format, ...) __attribute__ ((format(__printf__, 6, 7)));
static void yaml_error_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line, const char *function, const char *file, const char *format, ...) {
    char buf[1024] = ""; // Initialize buf to an empty string
    const char *type = "";

    if(event) {
        type = yaml_event_name(event->type);

        switch (event->type) {
            case YAML_SCALAR_EVENT:
                copy_to_buffer(buf, sizeof(buf), (char *)event->data.scalar.value, event->data.scalar.length);
                break;

            case YAML_ALIAS_EVENT:
                snprintf(buf, sizeof(buf), "%s", event->data.alias.anchor);
                break;

            default:
                break;
        }
    }

    fprintf(stderr, "YAML %zu@%s, %s(): (line %d, column %d, %s%s%s): ",
            line, file, function,
            (int)(parser->mark.line + 1), (int)(parser->mark.column + 1),
            type, buf[0]? ", near ": "", buf);

    va_list args;
    va_start(args, format);
    vfprintf(stderr, format, args);
    va_end(args);
    fprintf(stderr, "\n");
}

#define yaml_parse(parser, event) yaml_parse_with_trace(parser, event, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_parse_with_trace(yaml_parser_t *parser, yaml_event_t *event, size_t line __maybe_unused, const char *function __maybe_unused, const char *file __maybe_unused) {
    if (!yaml_parser_parse(parser, event)) {
        yaml_error(parser, NULL, "YAML parser error %d", parser->error);
        return false;
    }

//    fprintf(stderr, ">>> %s >>> %.*s\n",
//            yaml_event_name(event->type),
//            event->type == YAML_SCALAR_EVENT ? event->data.scalar.length : 0,
//            event->type == YAML_SCALAR_EVENT ? (char *)event->data.scalar.value : "");

    return true;
}

#define yaml_parse_expect_event(parser, type) yaml_parse_expect_event_with_trace(parser, type, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_parse_expect_event_with_trace(yaml_parser_t *parser, yaml_event_type_t type, size_t line, const char *function, const char *file) {
    yaml_event_t event;
    if (!yaml_parse(parser, &event))
        return false;

    bool ret = true;
    if(event.type != type) {
        yaml_error_with_trace(parser, &event, line, function, file, "unexpected event - expecting: %s", yaml_event_name(type));
        ret = false;
    }
//    else
//        fprintf(stderr, "OK (%zu@%s, %s()\n", line, file, function);

    yaml_event_delete(&event);
    return ret;
}

#define yaml_scalar_matches(event, s, len) yaml_scalar_matches_with_trace(event, s, len, __LINE__, __FUNCTION__, __FILE__)
static bool yaml_scalar_matches_with_trace(yaml_event_t *event, const char *s, size_t len, size_t line __maybe_unused, const char *function __maybe_unused, const char *file __maybe_unused) {
    if(event->type != YAML_SCALAR_EVENT)
        return false;

    if(len != event->data.scalar.length)
        return false;
//    else
//        fprintf(stderr, "OK (%zu@%s, %s()\n", line, file, function);

    return strcmp((char *)event->data.scalar.value, s) == 0;
}

// ----------------------------------------------------------------------------

static size_t yaml_parse_filename_injection(yaml_parser_t *parser, LOG_JOB *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    if (!yaml_parse(parser, &event))
        return 1;

    if (yaml_scalar_matches(&event, "key", strlen("key"))) {
        yaml_event_t sub_event;
        if (!yaml_parse(parser, &sub_event))
            errors++;

        else {
            if (sub_event.type == YAML_SCALAR_EVENT) {
                if(!log_job_filename_key_set(jb, (char *) sub_event.data.scalar.value,
                                             sub_event.data.scalar.length))
                    errors++;
            }

            else {
                yaml_error(parser, &sub_event, "expected the filename as %s", yaml_event_name(YAML_SCALAR_EVENT));
                errors++;
            }

            yaml_event_delete(&sub_event);
        }
    }

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_END_EVENT))
        errors++;

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_filters(yaml_parser_t *parser, LOG_JOB *jb) {
    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    size_t errors = 0;
    bool finished = false;

    while(!errors && !finished) {
        yaml_event_t event;

        if(!yaml_parse(parser, &event))
            return 1;

        if(event.type == YAML_SCALAR_EVENT) {
            if(yaml_scalar_matches(&event, "include", strlen("include"))) {
                yaml_event_t sub_event;
                if(!yaml_parse(parser, &sub_event))
                    errors++;

                else {
                    if(sub_event.type == YAML_SCALAR_EVENT) {
                        if(!log_job_include_pattern_set(jb, (char *) sub_event.data.scalar.value,
                                                        sub_event.data.scalar.length))
                            errors++;
                    }

                    else {
                        yaml_error(parser, &sub_event, "expected the include as %s",
                                   yaml_event_name(YAML_SCALAR_EVENT));
                        errors++;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
            else if(yaml_scalar_matches(&event, "exclude", strlen("exclude"))) {
                yaml_event_t sub_event;
                if(!yaml_parse(parser, &sub_event))
                    errors++;

                else {
                    if(sub_event.type == YAML_SCALAR_EVENT) {
                        if(!log_job_exclude_pattern_set(jb,(char *) sub_event.data.scalar.value,
                                                        sub_event.data.scalar.length))
                            errors++;
                    }

                    else {
                        yaml_error(parser, &sub_event, "expected the exclude as %s",
                                   yaml_event_name(YAML_SCALAR_EVENT));
                        errors++;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
        }
        else if(event.type == YAML_MAPPING_END_EVENT)
            finished = true;
        else {
            yaml_error(parser, &event, "expected %s or %s",
                       yaml_event_name(YAML_SCALAR_EVENT),
                       yaml_event_name(YAML_MAPPING_END_EVENT));
            errors++;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_prefix(yaml_parser_t *parser, LOG_JOB *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if (!yaml_parse(parser, &event))
        return 1;

    if (event.type == YAML_SCALAR_EVENT) {
        if(!log_job_key_prefix_set(jb, (char *) event.data.scalar.value, event.data.scalar.length))
            errors++;
    }

    yaml_event_delete(&event);
    return errors;
}

static bool yaml_parse_constant_field_injection(yaml_parser_t *parser, LOG_JOB *jb, bool unmatched) {
    yaml_event_t event;
    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection key");
        yaml_event_delete(&event);
        return false;
    }

    char *key = strndupz((char *)event.data.scalar.value, event.data.scalar.length);
    char *value = NULL;
    bool ret = false;

    yaml_event_delete(&event);

    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection value");
        goto cleanup;
    }

    if(!yaml_scalar_matches(&event, "value", strlen("value"))) {
        yaml_error(parser, &event, "Expected scalar 'value'");
        goto cleanup;
    }

    if (!yaml_parse(parser, &event) || event.type != YAML_SCALAR_EVENT) {
        yaml_error(parser, &event, "Expected scalar for constant field injection value");
        goto cleanup;
    }

    value = strndupz((char *)event.data.scalar.value, event.data.scalar.length);

    if(!log_job_injection_add(jb, key, strlen(key), value, strlen(value), unmatched))
        ret = false;
    else
        ret = true;

    ret = true;

cleanup:
    yaml_event_delete(&event);
    freez(key);
    freez(value);
    return !ret ? 1 : 0;
}

static bool yaml_parse_injection_mapping(yaml_parser_t *parser, LOG_JOB *jb, bool unmatched) {
    yaml_event_t event;
    size_t errors = 0;
    bool finished = false;

    while (!errors && !finished) {
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                    errors += yaml_parse_constant_field_injection(parser, jb, unmatched);
                } else {
                    yaml_error(parser, &event, "Unexpected scalar in injection mapping");
                    errors++;
                }
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in injection mapping");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors == 0;
}

static size_t yaml_parse_injections(yaml_parser_t *parser, LOG_JOB *jb, bool unmatched) {
    yaml_event_t event;
    size_t errors = 0;
    bool finished = false;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    while (!errors && !finished) {
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
                if (!yaml_parse_injection_mapping(parser, jb, unmatched))
                    errors++;
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in injections sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_unmatched(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;
    bool finished = false;

    if (!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT))
        return 1;

    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "key", strlen("key"))) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                    } else {
                        if (sub_event.type == YAML_SCALAR_EVENT) {
                            hashed_key_len_set(&jb->unmatched.key, (char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                        } else {
                            yaml_error(parser, &sub_event, "expected a scalar value for 'key'");
                            errors++;
                        }
                        yaml_event_delete(&sub_event);
                    }
                } else if (yaml_scalar_matches(&event, "inject", strlen("inject"))) {
                    errors += yaml_parse_injections(parser, jb, true);
                } else {
                    yaml_error(parser, &event, "Unexpected scalar in unmatched section");
                    errors++;
                }
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in unmatched section");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_rewrites(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
            {
                RW_FLAGS flags = RW_NONE;
                char *key = NULL;
                char *search_pattern = NULL;
                char *replace_pattern = NULL;

                bool mapping_finished = false;
                while (!errors && !mapping_finished) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                        continue;
                    }

                    switch (sub_event.type) {
                        case YAML_SCALAR_EVENT:
                            if (yaml_scalar_matches(&sub_event, "key", strlen("key"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite key");
                                    errors++;
                                } else {
                                    key = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "match", strlen("match"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite match PCRE2 pattern");
                                    errors++;
                                }
                                else {
                                    if(search_pattern)
                                        freez(search_pattern);
                                    flags |= RW_MATCH_PCRE2;
                                    flags &= ~RW_MATCH_NON_EMPTY;
                                    search_pattern = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "not_empty", strlen("not_empty"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite not empty condition");
                                    errors++;
                                }
                                else {
                                    if(search_pattern)
                                        freez(search_pattern);
                                    flags |= RW_MATCH_NON_EMPTY;
                                    flags &= ~RW_MATCH_PCRE2;
                                    search_pattern = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "value", strlen("value"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite value");
                                    errors++;
                                } else {
                                    replace_pattern = strndupz((char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "stop", strlen("stop"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite stop boolean");
                                    errors++;
                                } else {
                                    if(strncmp((char*)sub_event.data.scalar.value, "no", 2) == 0 ||
                                        strncmp((char*)sub_event.data.scalar.value, "false", 5) == 0)
                                        flags |= RW_DONT_STOP;
                                    else
                                        flags &= ~RW_DONT_STOP;

                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "inject", strlen("inject"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rewrite inject boolean");
                                    errors++;
                                } else {
                                    if(strncmp((char*)sub_event.data.scalar.value, "yes", 3) == 0 ||
                                       strncmp((char*)sub_event.data.scalar.value, "true", 4) == 0)
                                        flags |= RW_INJECT;
                                    else
                                        flags &= ~RW_INJECT;

                                    yaml_event_delete(&sub_event);
                                }
                            } else {
                                yaml_error(parser, &sub_event, "Unexpected scalar in rewrite mapping");
                                errors++;
                            }
                            break;

                        case YAML_MAPPING_END_EVENT:
                            if(key) {
                                if (!log_job_rewrite_add(jb, key, flags, search_pattern, replace_pattern))
                                    errors++;
                            }

                            freez(key);
                            key = NULL;

                            freez(search_pattern);
                            search_pattern = NULL;

                            freez(replace_pattern);
                            replace_pattern = NULL;

                            flags = RW_NONE;

                            mapping_finished = true;
                            break;

                        default:
                            yaml_error(parser, &sub_event, "Unexpected event in rewrite mapping");
                            errors++;
                            break;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in rewrites sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_renames(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;

    if (!yaml_parse_expect_event(parser, YAML_SEQUENCE_START_EVENT))
        return 1;

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if (!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch (event.type) {
            case YAML_MAPPING_START_EVENT:
            {
                struct key_rename rn = { 0 };

                bool mapping_finished = false;
                while (!errors && !mapping_finished) {
                    yaml_event_t sub_event;
                    if (!yaml_parse(parser, &sub_event)) {
                        errors++;
                        continue;
                    }

                    switch (sub_event.type) {
                        case YAML_SCALAR_EVENT:
                            if (yaml_scalar_matches(&sub_event, "new_key", strlen("new_key"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rename new_key");
                                    errors++;
                                } else {
                                    hashed_key_len_set(&rn.new_key, (char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else if (yaml_scalar_matches(&sub_event, "old_key", strlen("old_key"))) {
                                if (!yaml_parse(parser, &sub_event) || sub_event.type != YAML_SCALAR_EVENT) {
                                    yaml_error(parser, &sub_event, "Expected scalar for rename old_key");
                                    errors++;
                                } else {
                                    hashed_key_len_set(&rn.old_key, (char *)sub_event.data.scalar.value, sub_event.data.scalar.length);
                                    yaml_event_delete(&sub_event);
                                }
                            } else {
                                yaml_error(parser, &sub_event, "Unexpected scalar in rewrite mapping");
                                errors++;
                            }
                            break;

                        case YAML_MAPPING_END_EVENT:
                            if(rn.old_key.key && rn.new_key.key) {
                                if (!log_job_rename_add(jb, rn.new_key.key, rn.new_key.len,
                                                        rn.old_key.key, rn.old_key.len))
                                    errors++;
                            }
                            rename_cleanup(&rn);

                            mapping_finished = true;
                            break;

                        default:
                            yaml_error(parser, &sub_event, "Unexpected event in rewrite mapping");
                            errors++;
                            break;
                    }

                    yaml_event_delete(&sub_event);
                }
            }
                break;

            case YAML_SEQUENCE_END_EVENT:
                finished = true;
                break;

            default:
                yaml_error(parser, &event, "Unexpected event in rewrites sequence");
                errors++;
                break;
        }

        yaml_event_delete(&event);
    }

    return errors;
}

static size_t yaml_parse_pattern(yaml_parser_t *parser, LOG_JOB *jb) {
    yaml_event_t event;
    size_t errors = 0;

    if (!yaml_parse(parser, &event))
        return 1;

    if(event.type == YAML_SCALAR_EVENT)
        log_job_pattern_set(jb, (char *) event.data.scalar.value, event.data.scalar.length);
    else {
        yaml_error(parser, &event, "unexpected event type");
        errors++;
    }

    yaml_event_delete(&event);
    return errors;
}

static size_t yaml_parse_initialized(yaml_parser_t *parser, LOG_JOB *jb) {
    size_t errors = 0;

    if(!yaml_parse_expect_event(parser, YAML_STREAM_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_DOCUMENT_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!yaml_parse_expect_event(parser, YAML_MAPPING_START_EVENT)) {
        errors++;
        goto cleanup;
    }

    bool finished = false;
    while (!errors && !finished) {
        yaml_event_t event;
        if(!yaml_parse(parser, &event)) {
            errors++;
            continue;
        }

        switch(event.type) {
            default:
                yaml_error(parser, &event, "unexpected type");
                errors++;
                break;

            case YAML_MAPPING_END_EVENT:
                finished = true;
                break;

            case YAML_SCALAR_EVENT:
                if (yaml_scalar_matches(&event, "pattern", strlen("pattern")))
                    errors += yaml_parse_pattern(parser, jb);

                else if (yaml_scalar_matches(&event, "prefix", strlen("prefix")))
                    errors += yaml_parse_prefix(parser, jb);

                else if (yaml_scalar_matches(&event, "filename", strlen("filename")))
                    errors += yaml_parse_filename_injection(parser, jb);

                else if (yaml_scalar_matches(&event, "filter", strlen("filter")))
                    errors += yaml_parse_filters(parser, jb);

                else if (yaml_scalar_matches(&event, "inject", strlen("inject")))
                    errors += yaml_parse_injections(parser, jb, false);

                else if (yaml_scalar_matches(&event, "unmatched", strlen("unmatched")))
                    errors += yaml_parse_unmatched(parser, jb);

                else if (yaml_scalar_matches(&event, "rewrite", strlen("rewrite")))
                    errors += yaml_parse_rewrites(parser, jb);

                else if (yaml_scalar_matches(&event, "rename", strlen("rename")))
                    errors += yaml_parse_renames(parser, jb);

                else {
                    yaml_error(parser, &event, "unexpected scalar");
                    errors++;
                }
                break;
        }

        yaml_event_delete(&event);
    }

    if(!errors && !yaml_parse_expect_event(parser, YAML_DOCUMENT_END_EVENT)) {
        errors++;
        goto cleanup;
    }

    if(!errors && !yaml_parse_expect_event(parser, YAML_STREAM_END_EVENT)) {
        errors++;
        goto cleanup;
    }

cleanup:
    return errors;
}

bool yaml_parse_file(const char *config_file_path, LOG_JOB *jb) {
    // cfg_parse_yaml_file(config_file_path);

    if(!config_file_path || !*config_file_path) {
        log2stderr("yaml configuration filename cannot be empty.");
        return false;
    }

    FILE *fp = fopen(config_file_path, "r");
    if (!fp) {
        log2stderr("Error opening config file: %s", config_file_path);
        return false;
    }

    yaml_parser_t parser;
    yaml_parser_initialize(&parser);
    yaml_parser_set_input_file(&parser, fp);

    size_t errors = yaml_parse_initialized(&parser, jb);

    yaml_parser_delete(&parser);
    fclose(fp);
    return errors == 0;
}

bool yaml_parse_config(const char *config_name, LOG_JOB *jb) {
    char filename[FILENAME_MAX + 1];

    snprintf(filename, sizeof(filename), "%s/%s.yaml", LOG2JOURNAL_CONFIG_PATH, config_name);
    return yaml_parse_file(filename, jb);
}

#endif // HAVE_LIBYAML

// ----------------------------------------------------------------------------
// printing yaml

static void yaml_print_multiline_value(const char *s, size_t depth) {
    if (!s)
        s = "";

    do {
        const char* next = strchr(s, '\n');
        if(next) next++;

        size_t len = next ? (size_t)(next - s) : strlen(s);
        char buf[len + 1];
        copy_to_buffer(buf, sizeof(buf), s, len);

        fprintf(stderr, "%.*s%s%s",
                (int)(depth * 2), "                    ",
                buf, next ? "" : "\n");

        s = next;
    } while(s && *s);
}

static bool needs_quotes_in_yaml(const char *str) {
    // Lookup table for special YAML characters
    static bool special_chars[256] = { false };
    static bool table_initialized = false;

    if (!table_initialized) {
        // Initialize the lookup table
        const char *special_chars_str = ":{}[],&*!|>'\"%@`^";
        for (const char *c = special_chars_str; *c; ++c) {
            special_chars[(unsigned char)*c] = true;
        }
        table_initialized = true;
    }

    while (*str) {
        if (special_chars[(unsigned char)*str]) {
            return true;
        }
        str++;
    }
    return false;
}

static void yaml_print_node(const char *key, const char *value, size_t depth, bool dash) {
    if(depth > 10) depth = 10;
    const char *quote = "'";

    const char *second_line = NULL;
    if(value && strchr(value, '\n')) {
        second_line = value;
        value = "|";
        quote = "";
    }
    else if(!value || !needs_quotes_in_yaml(value))
        quote = "";

    fprintf(stderr, "%.*s%s%s%s%s%s%s\n",
            (int)(depth * 2), "                    ", dash ? "- ": "",
            key ? key : "", key ? ": " : "",
            quote, value ? value : "", quote);

    if(second_line) {
        yaml_print_multiline_value(second_line, depth + 1);
    }
}

void log_job_configuration_to_yaml(LOG_JOB *jb) {
    if(jb->pattern)
        yaml_print_node("pattern", jb->pattern, 0, false);

    if(jb->prefix) {
        fprintf(stderr, "\n");
        yaml_print_node("prefix", jb->prefix, 0, false);
    }

    if(jb->filename.key.key) {
        fprintf(stderr, "\n");
        yaml_print_node("filename", NULL, 0, false);
        yaml_print_node("key", jb->filename.key.key, 1, false);
    }

    if(jb->filter.include.pattern || jb->filter.exclude.pattern) {
        fprintf(stderr, "\n");
        yaml_print_node("filter", NULL, 0, false);

        if(jb->filter.include.pattern)
            yaml_print_node("include", jb->filter.include.pattern, 1, false);

        if(jb->filter.exclude.pattern)
            yaml_print_node("exclude", jb->filter.exclude.pattern, 1, false);
    }

    if(jb->renames.used) {
        fprintf(stderr, "\n");
        yaml_print_node("rename", NULL, 0, false);

        for(size_t i = 0; i < jb->renames.used ;i++) {
            yaml_print_node("new_key", jb->renames.array[i].new_key.key, 1, true);
            yaml_print_node("old_key", jb->renames.array[i].old_key.key, 2, false);
        }
    }

    if(jb->injections.used) {
        fprintf(stderr, "\n");
        yaml_print_node("inject", NULL, 0, false);

        for (size_t i = 0; i < jb->injections.used; i++) {
            yaml_print_node("key", jb->injections.keys[i].key.key, 1, true);
            yaml_print_node("value", jb->injections.keys[i].value.pattern, 2, false);
        }
    }

    if(jb->rewrites.used) {
        fprintf(stderr, "\n");
        yaml_print_node("rewrite", NULL, 0, false);

        for(size_t i = 0; i < jb->rewrites.used ;i++) {
            REWRITE *rw = &jb->rewrites.array[i];

            yaml_print_node("key", rw->key.key, 1, true);

            if(rw->flags & RW_MATCH_PCRE2)
                yaml_print_node("match", rw->match_pcre2.pattern, 2, false);

            else if(rw->flags & RW_MATCH_NON_EMPTY)
                yaml_print_node("not_empty", rw->match_non_empty.pattern, 2, false);

            yaml_print_node("value", rw->value.pattern, 2, false);

            if(rw->flags & RW_INJECT)
                yaml_print_node("inject", "yes", 2, false);

            if(rw->flags & RW_DONT_STOP)
                yaml_print_node("stop", "no", 2, false);
        }
    }

    if(jb->unmatched.key.key || jb->unmatched.injections.used) {
        fprintf(stderr, "\n");
        yaml_print_node("unmatched", NULL, 0, false);

        if(jb->unmatched.key.key)
            yaml_print_node("key", jb->unmatched.key.key, 1, false);

        if(jb->unmatched.injections.used) {
            fprintf(stderr, "\n");
            yaml_print_node("inject", NULL, 1, false);

            for (size_t i = 0; i < jb->unmatched.injections.used; i++) {
                yaml_print_node("key", jb->unmatched.injections.keys[i].key.key, 2, true);
                yaml_print_node("value", jb->unmatched.injections.keys[i].value.pattern, 3, false);
            }
        }
    }
}
