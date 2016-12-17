#include "common.h"

#define RRD_DEFAULT_GAP_INTERPOLATIONS 1

// ----------------------------------------------------------------------------
// globals

/*
// if not zero it gives the time (in seconds) to remove un-updated dimensions
// DO NOT ENABLE
// if dimensions are removed, the chart generation will have to run again
int rrd_delete_unupdated_dimensions = 0;
*/

int rrd_update_every = UPDATE_EVERY;
int rrd_default_history_entries = RRD_DEFAULT_HISTORY_ENTRIES;
int rrd_memory_mode = RRD_MEMORY_MODE_SAVE;

static int rrdset_compare(void* a, void* b);
static int rrdset_compare_name(void* a, void* b);
static int rrdfamily_compare(void *a, void *b);

// ----------------------------------------------------------------------------
// RRDHOST

RRDHOST localhost = {
        .hostname = "localhost",
        .rrdset_root = NULL,
        .rrdset_root_rwlock = PTHREAD_RWLOCK_INITIALIZER,
        .rrdset_root_index = {
            { NULL, rrdset_compare },
            AVL_LOCK_INITIALIZER
        },
        .rrdset_root_index_name = {
            { NULL, rrdset_compare_name },
            AVL_LOCK_INITIALIZER
        },
        .rrdfamily_root_index = {
            { NULL, rrdfamily_compare },
            AVL_LOCK_INITIALIZER
        },
        .variables_root_index = {
            { NULL, rrdvar_compare },
            AVL_LOCK_INITIALIZER
        },
        .health_log = {
            .next_log_id = 1,
            .next_alarm_id = 1,
            .count = 0,
            .max = 1000,
            .alarms = NULL,
            .alarm_log_rwlock = PTHREAD_RWLOCK_INITIALIZER
        }
};

void rrdhost_init(char *hostname) {
    localhost.hostname = hostname;
    localhost.health_log.next_log_id =
        localhost.health_log.next_alarm_id = now_realtime_sec();
}

void rrdhost_rwlock(RRDHOST *host) {
    pthread_rwlock_wrlock(&host->rrdset_root_rwlock);
}

void rrdhost_rdlock(RRDHOST *host) {
    pthread_rwlock_rdlock(&host->rrdset_root_rwlock);
}

void rrdhost_unlock(RRDHOST *host) {
    pthread_rwlock_unlock(&host->rrdset_root_rwlock);
}

void rrdhost_check_rdlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    int ret = pthread_rwlock_trywrlock(&host->rrdset_root_rwlock);

    if(ret == 0)
        fatal("RRDHOST '%s' should be read-locked, but it is not, at function %s() at line %lu of file '%s'", host->hostname, function, line, file);
}

void rrdhost_check_wrlock_int(RRDHOST *host, const char *file, const char *function, const unsigned long line) {
    int ret = pthread_rwlock_tryrdlock(&host->rrdset_root_rwlock);

    if(ret == 0)
        fatal("RRDHOST '%s' should be write-locked, but it is not, at function %s() at line %lu of file '%s'", host->hostname, function, line, file);
}

// ----------------------------------------------------------------------------
// RRDFAMILY index

static int rrdfamily_compare(void *a, void *b) {
    if(((RRDFAMILY *)a)->hash_family < ((RRDFAMILY *)b)->hash_family) return -1;
    else if(((RRDFAMILY *)a)->hash_family > ((RRDFAMILY *)b)->hash_family) return 1;
    else return strcmp(((RRDFAMILY *)a)->family, ((RRDFAMILY *)b)->family);
}

#define rrdfamily_index_add(host, rc) (RRDFAMILY *)avl_insert_lock(&((host)->rrdfamily_root_index), (avl *)(rc))
#define rrdfamily_index_del(host, rc) (RRDFAMILY *)avl_remove_lock(&((host)->rrdfamily_root_index), (avl *)(rc))

static RRDFAMILY *rrdfamily_index_find(RRDHOST *host, const char *id, uint32_t hash) {
    RRDFAMILY tmp;
    tmp.family = id;
    tmp.hash_family = (hash)?hash:simple_hash(tmp.family);

    return (RRDFAMILY *)avl_search_lock(&(host->rrdfamily_root_index), (avl *) &tmp);
}

RRDFAMILY *rrdfamily_create(const char *id) {
    RRDFAMILY *rc = rrdfamily_index_find(&localhost, id, 0);
    if(!rc) {
        rc = callocz(1, sizeof(RRDFAMILY));

        rc->family = strdupz(id);
        rc->hash_family = simple_hash(rc->family);

        // initialize the variables index
        avl_init_lock(&rc->variables_root_index, rrdvar_compare);

        RRDFAMILY *ret = rrdfamily_index_add(&localhost, rc);
        if(ret != rc)
            fatal("INTERNAL ERROR: Expected to INSERT RRDFAMILY '%s' into index, but inserted '%s'.", rc->family, (ret)?ret->family:"NONE");
    }

    rc->use_count++;
    return rc;
}

void rrdfamily_free(RRDFAMILY *rc) {
    rc->use_count--;
    if(!rc->use_count) {
        RRDFAMILY *ret = rrdfamily_index_del(&localhost, rc);
        if(ret != rc)
            fatal("INTERNAL ERROR: Expected to DELETE RRDFAMILY '%s' from index, but deleted '%s'.", rc->family, (ret)?ret->family:"NONE");

        if(rc->variables_root_index.avl_tree.root != NULL)
            fatal("INTERNAL ERROR: Variables index of RRDFAMILY '%s' that is freed, is not empty.", rc->family);

        freez((void *)rc->family);
        freez(rc);
    }
}

// ----------------------------------------------------------------------------
// RRDSET index

static int rrdset_compare(void* a, void* b) {
    if(((RRDSET *)a)->hash < ((RRDSET *)b)->hash) return -1;
    else if(((RRDSET *)a)->hash > ((RRDSET *)b)->hash) return 1;
    else return strcmp(((RRDSET *)a)->id, ((RRDSET *)b)->id);
}

#define rrdset_index_add(host, st) (RRDSET *)avl_insert_lock(&((host)->rrdset_root_index), (avl *)(st))
#define rrdset_index_del(host, st) (RRDSET *)avl_remove_lock(&((host)->rrdset_root_index), (avl *)(st))

static RRDSET *rrdset_index_find(RRDHOST *host, const char *id, uint32_t hash) {
    RRDSET tmp;
    strncpyz(tmp.id, id, RRD_ID_LENGTH_MAX);
    tmp.hash = (hash)?hash:simple_hash(tmp.id);

    return (RRDSET *)avl_search_lock(&(host->rrdset_root_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------
// RRDSET name index

#define rrdset_from_avlname(avlname_ptr) ((RRDSET *)((avlname_ptr) - offsetof(RRDSET, avlname)))

static int rrdset_compare_name(void* a, void* b) {
    RRDSET *A = rrdset_from_avlname(a);
    RRDSET *B = rrdset_from_avlname(b);

    // fprintf(stderr, "COMPARING: %s with %s\n", A->name, B->name);

    if(A->hash_name < B->hash_name) return -1;
    else if(A->hash_name > B->hash_name) return 1;
    else return strcmp(A->name, B->name);
}

RRDSET *rrdset_index_add_name(RRDHOST *host, RRDSET *st) {
    void *result;
    // fprintf(stderr, "ADDING: %s (name: %s)\n", st->id, st->name);
    result = avl_insert_lock(&host->rrdset_root_index_name, (avl *) (&st->avlname));
    if(result) return rrdset_from_avlname(result);
    return NULL;
}

RRDSET *rrdset_index_del_name(RRDHOST *host, RRDSET *st) {
    void *result;
    // fprintf(stderr, "DELETING: %s (name: %s)\n", st->id, st->name);
    result = (RRDSET *)avl_remove_lock(&((host)->rrdset_root_index_name), (avl *)(&st->avlname));
    if(result) return rrdset_from_avlname(result);
    return NULL;
}

static RRDSET *rrdset_index_find_name(RRDHOST *host, const char *name, uint32_t hash) {
    void *result = NULL;
    RRDSET tmp;
    tmp.name = name;
    tmp.hash_name = (hash)?hash:simple_hash(tmp.name);

    // fprintf(stderr, "SEARCHING: %s\n", name);
    result = avl_search_lock(&host->rrdset_root_index_name, (avl *) (&(tmp.avlname)));
    if(result) {
        RRDSET *st = rrdset_from_avlname(result);
        if(strcmp(st->magic, RRDSET_MAGIC))
            error("Search for RRDSET %s returned an invalid RRDSET %s (name %s)", name, st->id, st->name);

        // fprintf(stderr, "FOUND: %s\n", name);
        return rrdset_from_avlname(result);
    }
    // fprintf(stderr, "NOT FOUND: %s\n", name);
    return NULL;
}


// ----------------------------------------------------------------------------
// RRDDIM index

static int rrddim_compare(void* a, void* b) {
    if(((RRDDIM *)a)->hash < ((RRDDIM *)b)->hash) return -1;
    else if(((RRDDIM *)a)->hash > ((RRDDIM *)b)->hash) return 1;
    else return strcmp(((RRDDIM *)a)->id, ((RRDDIM *)b)->id);
}

#define rrddim_index_add(st, rd) avl_insert_lock(&((st)->dimensions_index), (avl *)(rd))
#define rrddim_index_del(st,rd ) avl_remove_lock(&((st)->dimensions_index), (avl *)(rd))

static RRDDIM *rrddim_index_find(RRDSET *st, const char *id, uint32_t hash) {
    RRDDIM tmp;
    strncpyz(tmp.id, id, RRD_ID_LENGTH_MAX);
    tmp.hash = (hash)?hash:simple_hash(tmp.id);

    return (RRDDIM *)avl_search_lock(&(st->dimensions_index), (avl *) &tmp);
}

// ----------------------------------------------------------------------------
// chart types

int rrdset_type_id(const char *name)
{
    if(unlikely(strcmp(name, RRDSET_TYPE_AREA_NAME) == 0)) return RRDSET_TYPE_AREA;
    else if(unlikely(strcmp(name, RRDSET_TYPE_STACKED_NAME) == 0)) return RRDSET_TYPE_STACKED;
    else if(unlikely(strcmp(name, RRDSET_TYPE_LINE_NAME) == 0)) return RRDSET_TYPE_LINE;
    return RRDSET_TYPE_LINE;
}

const char *rrdset_type_name(int chart_type)
{
    static char line[] = RRDSET_TYPE_LINE_NAME;
    static char area[] = RRDSET_TYPE_AREA_NAME;
    static char stacked[] = RRDSET_TYPE_STACKED_NAME;

    switch(chart_type) {
        case RRDSET_TYPE_LINE:
            return line;

        case RRDSET_TYPE_AREA:
            return area;

        case RRDSET_TYPE_STACKED:
            return stacked;
    }
    return line;
}

// ----------------------------------------------------------------------------
// load / save

const char *rrd_memory_mode_name(int id)
{
    static const char ram[] = RRD_MEMORY_MODE_RAM_NAME;
    static const char map[] = RRD_MEMORY_MODE_MAP_NAME;
    static const char save[] = RRD_MEMORY_MODE_SAVE_NAME;

    switch(id) {
        case RRD_MEMORY_MODE_RAM:
            return ram;

        case RRD_MEMORY_MODE_MAP:
            return map;

        case RRD_MEMORY_MODE_SAVE:
        default:
            return save;
    }

    return save;
}

int rrd_memory_mode_id(const char *name)
{
    if(unlikely(!strcmp(name, RRD_MEMORY_MODE_RAM_NAME)))
        return RRD_MEMORY_MODE_RAM;
    else if(unlikely(!strcmp(name, RRD_MEMORY_MODE_MAP_NAME)))
        return RRD_MEMORY_MODE_MAP;

    return RRD_MEMORY_MODE_SAVE;
}

// ----------------------------------------------------------------------------
// algorithms types

int rrddim_algorithm_id(const char *name)
{
    if(strcmp(name, RRDDIM_INCREMENTAL_NAME) == 0)          return RRDDIM_INCREMENTAL;
    if(strcmp(name, RRDDIM_ABSOLUTE_NAME) == 0)             return RRDDIM_ABSOLUTE;
    if(strcmp(name, RRDDIM_PCENT_OVER_ROW_TOTAL_NAME) == 0)         return RRDDIM_PCENT_OVER_ROW_TOTAL;
    if(strcmp(name, RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME) == 0)    return RRDDIM_PCENT_OVER_DIFF_TOTAL;
    return RRDDIM_ABSOLUTE;
}

const char *rrddim_algorithm_name(int chart_type)
{
    static char absolute[] = RRDDIM_ABSOLUTE_NAME;
    static char incremental[] = RRDDIM_INCREMENTAL_NAME;
    static char percentage_of_absolute_row[] = RRDDIM_PCENT_OVER_ROW_TOTAL_NAME;
    static char percentage_of_incremental_row[] = RRDDIM_PCENT_OVER_DIFF_TOTAL_NAME;

    switch(chart_type) {
        case RRDDIM_ABSOLUTE:
            return absolute;

        case RRDDIM_INCREMENTAL:
            return incremental;

        case RRDDIM_PCENT_OVER_ROW_TOTAL:
            return percentage_of_absolute_row;

        case RRDDIM_PCENT_OVER_DIFF_TOTAL:
            return percentage_of_incremental_row;
    }
    return absolute;
}

// ----------------------------------------------------------------------------
// chart names

char *rrdset_strncpyz_name(char *to, const char *from, size_t length)
{
    char c, *p = to;

    while (length-- && (c = *from++)) {
        if(c != '.' && !isalnum(c))
            c = '_';

        *p++ = c;
    }

    *p = '\0';

    return to;
}

void rrdset_set_name(RRDSET *st, const char *name)
{
    if(unlikely(st->name && !strcmp(st->name, name)))
        return;

    debug(D_RRD_CALLS, "rrdset_set_name() old: %s, new: %s", st->name, name);

    char b[CONFIG_MAX_VALUE + 1];
    char n[RRD_ID_LENGTH_MAX + 1];

    snprintfz(n, RRD_ID_LENGTH_MAX, "%s.%s", st->type, name);
    rrdset_strncpyz_name(b, n, CONFIG_MAX_VALUE);

    if(st->name) {
        rrdset_index_del_name(&localhost, st);
        st->name = config_set_default(st->id, "name", b);
        st->hash_name = simple_hash(st->name);
        rrdsetvar_rename_all(st);
    }
    else {
        st->name = config_get(st->id, "name", b);
        st->hash_name = simple_hash(st->name);
    }

    pthread_rwlock_wrlock(&st->rwlock);
    RRDDIM *rd;
    for(rd = st->dimensions; rd ;rd = rd->next)
        rrddimvar_rename_all(rd);
    pthread_rwlock_unlock(&st->rwlock);

    rrdset_index_add_name(&localhost, st);
}

// ----------------------------------------------------------------------------
// cache directory

char *rrdset_cache_dir(const char *id)
{
    char *ret = NULL;

    static char *cache_dir = NULL;
    if(!cache_dir) {
        cache_dir = config_get("global", "cache directory", CACHE_DIR);
        int r = mkdir(cache_dir, 0755);
        if(r != 0 && errno != EEXIST)
            error("Cannot create directory '%s'", cache_dir);
    }

    char b[FILENAME_MAX + 1];
    char n[FILENAME_MAX + 1];
    rrdset_strncpyz_name(b, id, FILENAME_MAX);

    snprintfz(n, FILENAME_MAX, "%s/%s", cache_dir, b);
    ret = config_get(id, "cache directory", n);

    if(rrd_memory_mode == RRD_MEMORY_MODE_MAP || rrd_memory_mode == RRD_MEMORY_MODE_SAVE) {
        int r = mkdir(ret, 0775);
        if(r != 0 && errno != EEXIST)
            error("Cannot create directory '%s'", ret);
    }

    return ret;
}

// ----------------------------------------------------------------------------
// core functions

void rrdset_reset(RRDSET *st)
{
    debug(D_RRD_CALLS, "rrdset_reset() %s", st->name);

    st->last_collected_time.tv_sec = 0;
    st->last_collected_time.tv_usec = 0;
    st->last_updated.tv_sec = 0;
    st->last_updated.tv_usec = 0;
    st->current_entry = 0;
    st->counter = 0;
    st->counter_done = 0;

    RRDDIM *rd;
    for(rd = st->dimensions; rd ; rd = rd->next) {
        rd->last_collected_time.tv_sec = 0;
        rd->last_collected_time.tv_usec = 0;
        rd->counter = 0;
        memset(rd->values, 0, rd->entries * sizeof(storage_number));
    }
}
static inline long align_entries_to_pagesize(long entries) {
    if(entries < 5) entries = 5;
    if(entries > RRD_HISTORY_ENTRIES_MAX) entries = RRD_HISTORY_ENTRIES_MAX;

#ifdef NETDATA_LOG_ALLOCATIONS
    long page = (size_t)sysconf(_SC_PAGESIZE);

    long size = sizeof(RRDDIM) + entries * sizeof(storage_number);
    if(size % page) {
        size -= (size % page);
        size += page;

        long n = (size - sizeof(RRDDIM)) / sizeof(storage_number);
        return n;
    }

    return entries;
#else
    return entries;
#endif
}

static inline void timeval_align(struct timeval *tv, int update_every) {
    tv->tv_sec -= tv->tv_sec % update_every;
    tv->tv_usec = 500000;
}

RRDSET *rrdset_create(const char *type, const char *id, const char *name, const char *family, const char *context, const char *title, const char *units, long priority, int update_every, int chart_type)
{
    if(!type || !type[0]) {
        fatal("Cannot create rrd stats without a type.");
        return NULL;
    }

    if(!id || !id[0]) {
        fatal("Cannot create rrd stats without an id.");
        return NULL;
    }

    char fullid[RRD_ID_LENGTH_MAX + 1];
    char fullfilename[FILENAME_MAX + 1];

    snprintfz(fullid, RRD_ID_LENGTH_MAX, "%s.%s", type, id);

    RRDSET *st = rrdset_find(fullid);
    if(st) {
        error("Cannot create rrd stats for '%s', it already exists.", fullid);
        return st;
    }

    long rentries = config_get_number(fullid, "history", rrd_default_history_entries);
    long entries = align_entries_to_pagesize(rentries);
    if(entries != rentries) entries = config_set_number(fullid, "history", entries);

    int enabled = config_get_boolean(fullid, "enabled", 1);
    if(!enabled) entries = 5;

    unsigned long size = sizeof(RRDSET);
    char *cache_dir = rrdset_cache_dir(fullid);

    debug(D_RRD_CALLS, "Creating RRD_STATS for '%s.%s'.", type, id);

    snprintfz(fullfilename, FILENAME_MAX, "%s/main.db", cache_dir);
    if(rrd_memory_mode != RRD_MEMORY_MODE_RAM) st = (RRDSET *)mymmap(fullfilename, size, ((rrd_memory_mode == RRD_MEMORY_MODE_MAP)?MAP_SHARED:MAP_PRIVATE), 0);
    if(st) {
        if(strcmp(st->magic, RRDSET_MAGIC) != 0) {
            errno = 0;
            info("Initializing file %s.", fullfilename);
            memset(st, 0, size);
        }
        else if(strcmp(st->id, fullid) != 0) {
            errno = 0;
            error("File %s contents are not for chart %s. Clearing it.", fullfilename, fullid);
            // munmap(st, size);
            // st = NULL;
            memset(st, 0, size);
        }
        else if(st->memsize != size || st->entries != entries) {
            errno = 0;
            error("File %s does not have the desired size. Clearing it.", fullfilename);
            memset(st, 0, size);
        }
        else if(st->update_every != update_every) {
            errno = 0;
            error("File %s does not have the desired update frequency. Clearing it.", fullfilename);
            memset(st, 0, size);
        }
        else if((now_realtime_sec() - st->last_updated.tv_sec) > update_every * entries) {
            errno = 0;
            error("File %s is too old. Clearing it.", fullfilename);
            memset(st, 0, size);
        }

        // make sure the database is aligned
        if(st->last_updated.tv_sec)
            timeval_align(&st->last_updated, update_every);
    }

    if(st) {
        st->name = NULL;
        st->type = NULL;
        st->family = NULL;
        st->context = NULL;
        st->title = NULL;
        st->units = NULL;
        st->dimensions = NULL;
        st->next = NULL;
        st->mapped = rrd_memory_mode;
        st->variables = NULL;
        st->alarms = NULL;
    }
    else {
        st = callocz(1, size);
        st->mapped = RRD_MEMORY_MODE_RAM;
    }

    st->memsize = size;
    st->entries = entries;
    st->update_every = update_every;

    if(st->current_entry >= st->entries) st->current_entry = 0;

    strcpy(st->cache_filename, fullfilename);
    strcpy(st->magic, RRDSET_MAGIC);

    strcpy(st->id, fullid);
    st->hash = simple_hash(st->id);

    st->cache_dir = cache_dir;

    st->chart_type = rrdset_type_id(config_get(st->id, "chart type", rrdset_type_name(chart_type)));
    st->type       = config_get(st->id, "type", type);
    st->family     = config_get(st->id, "family", family?family:st->type);
    st->units      = config_get(st->id, "units", units?units:"");

    st->context    = config_get(st->id, "context", context?context:st->id);
    st->hash_context = simple_hash(st->context);

    st->priority = config_get_number(st->id, "priority", priority);
    st->enabled = enabled;

    st->isdetail = 0;
    st->debug = 0;

    // if(!strcmp(st->id, "disk_util.dm-0")) {
    //     st->debug = 1;
    //     error("enabled debugging for '%s'", st->id);
    // }
    // else error("not enabled debugging for '%s'", st->id);

    st->green = NAN;
    st->red = NAN;

    st->last_collected_time.tv_sec = 0;
    st->last_collected_time.tv_usec = 0;
    st->counter_done = 0;

    st->gap_when_lost_iterations_above = (int) (
            config_get_number(st->id, "gap when lost iterations above", RRD_DEFAULT_GAP_INTERPOLATIONS) + 2);

    avl_init_lock(&st->dimensions_index, rrddim_compare);
    avl_init_lock(&st->variables_root_index, rrdvar_compare);

    pthread_rwlock_init(&st->rwlock, NULL);
    rrdhost_rwlock(&localhost);

    if(name && *name) rrdset_set_name(st, name);
    else rrdset_set_name(st, id);

    {
        char varvalue[CONFIG_MAX_VALUE + 1];
        char varvalue2[CONFIG_MAX_VALUE + 1];
        snprintfz(varvalue, CONFIG_MAX_VALUE, "%s (%s)", title?title:"", st->name);
        json_escape_string(varvalue2, varvalue, sizeof(varvalue2));
        st->title = config_get(st->id, "title", varvalue2);
    }

    st->rrdfamily = rrdfamily_create(st->family);
    st->rrdhost = &localhost;

    st->next = localhost.rrdset_root;
    localhost.rrdset_root = st;

    if(health_enabled) {
        rrdsetvar_create(st, "last_collected_t", RRDVAR_TYPE_TIME_T, &st->last_collected_time.tv_sec, 0);
        rrdsetvar_create(st, "collected_total_raw", RRDVAR_TYPE_TOTAL, &st->last_collected_total, 0);
        rrdsetvar_create(st, "green", RRDVAR_TYPE_CALCULATED, &st->green, 0);
        rrdsetvar_create(st, "red", RRDVAR_TYPE_CALCULATED, &st->red, 0);
        rrdsetvar_create(st, "update_every", RRDVAR_TYPE_INT, &st->update_every, 0);
    }

    rrdset_index_add(&localhost, st);

    rrdsetcalc_link_matching(st);
    rrdcalctemplate_link_matching(st);

    rrdhost_unlock(&localhost);

    return(st);
}

RRDDIM *rrddim_add(RRDSET *st, const char *id, const char *name, long multiplier, long divisor, int algorithm)
{
    char filename[FILENAME_MAX + 1];
    char fullfilename[FILENAME_MAX + 1];

    char varname[CONFIG_MAX_NAME + 1];
    RRDDIM *rd = NULL;
    unsigned long size = sizeof(RRDDIM) + (st->entries * sizeof(storage_number));

    debug(D_RRD_CALLS, "Adding dimension '%s/%s'.", st->id, id);

    rrdset_strncpyz_name(filename, id, FILENAME_MAX);
    snprintfz(fullfilename, FILENAME_MAX, "%s/%s.db", st->cache_dir, filename);
    if(rrd_memory_mode != RRD_MEMORY_MODE_RAM) rd = (RRDDIM *)mymmap(fullfilename, size, ((rrd_memory_mode == RRD_MEMORY_MODE_MAP)?MAP_SHARED:MAP_PRIVATE), 1);
    if(rd) {
        struct timeval now;
        now_realtime_timeval(&now);

        if(strcmp(rd->magic, RRDDIMENSION_MAGIC) != 0) {
            errno = 0;
            info("Initializing file %s.", fullfilename);
            memset(rd, 0, size);
        }
        else if(rd->memsize != size) {
            errno = 0;
            error("File %s does not have the desired size. Clearing it.", fullfilename);
            memset(rd, 0, size);
        }
        else if(rd->multiplier != multiplier) {
            errno = 0;
            error("File %s does not have the same multiplier. Clearing it.", fullfilename);
            memset(rd, 0, size);
        }
        else if(rd->divisor != divisor) {
            errno = 0;
            error("File %s does not have the same divisor. Clearing it.", fullfilename);
            memset(rd, 0, size);
        }
        else if(rd->algorithm != algorithm) {
            errno = 0;
            error("File %s does not have the same algorithm. Clearing it.", fullfilename);
            memset(rd, 0, size);
        }
        else if(rd->update_every != st->update_every) {
            errno = 0;
            error("File %s does not have the same refresh frequency. Clearing it.", fullfilename);
            memset(rd, 0, size);
        }
        else if(dt_usec(&now, &rd->last_collected_time) > (rd->entries * rd->update_every * USEC_PER_SEC)) {
            errno = 0;
            error("File %s is too old. Clearing it.", fullfilename);
            memset(rd, 0, size);
        }
        else if(strcmp(rd->id, id) != 0) {
            errno = 0;
            error("File %s contents are not for dimension %s. Clearing it.", fullfilename, id);
            // munmap(rd, size);
            // rd = NULL;
            memset(rd, 0, size);
        }
    }

    if(rd) {
        // we have a file mapped for rd
        rd->mapped = rrd_memory_mode;
        rd->flags = 0x00000000;
        rd->variables = NULL;
        rd->next = NULL;
        rd->name = NULL;
    }
    else {
        // if we didn't manage to get a mmap'd dimension, just create one

        rd = callocz(1, size);
        rd->mapped = RRD_MEMORY_MODE_RAM;
    }
    rd->memsize = size;

    strcpy(rd->magic, RRDDIMENSION_MAGIC);
    strcpy(rd->cache_filename, fullfilename);
    strncpyz(rd->id, id, RRD_ID_LENGTH_MAX);
    rd->hash = simple_hash(rd->id);

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
    rd->name = config_get(st->id, varname, (name && *name)?name:rd->id);

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s algorithm", rd->id);
    rd->algorithm = rrddim_algorithm_id(config_get(st->id, varname, rrddim_algorithm_name(algorithm)));

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s multiplier", rd->id);
    rd->multiplier = config_get_number(st->id, varname, multiplier);

    snprintfz(varname, CONFIG_MAX_NAME, "dim %s divisor", rd->id);
    rd->divisor = config_get_number(st->id, varname, divisor);
    if(!rd->divisor) rd->divisor = 1;

    rd->entries = st->entries;
    rd->update_every = st->update_every;

    // prevent incremental calculation spikes
    rd->counter = 0;
    rd->updated = 0;
    rd->calculated_value = 0;
    rd->last_calculated_value = 0;
    rd->collected_value = 0;
    rd->last_collected_value = 0;
    rd->collected_volume = 0;
    rd->stored_volume = 0;
    rd->last_stored_value = 0;
    rd->values[st->current_entry] = pack_storage_number(0, SN_NOT_EXISTS);
    rd->last_collected_time.tv_sec = 0;
    rd->last_collected_time.tv_usec = 0;
    rd->rrdset = st;

    // append this dimension
    pthread_rwlock_wrlock(&st->rwlock);
    if(!st->dimensions)
        st->dimensions = rd;
    else {
        RRDDIM *td = st->dimensions;
        for(; td->next; td = td->next) ;
        td->next = rd;
    }

    if(health_enabled) {
        rrddimvar_create(rd, RRDVAR_TYPE_CALCULATED, NULL, NULL, &rd->last_stored_value, 0);
        rrddimvar_create(rd, RRDVAR_TYPE_COLLECTED, NULL, "_raw", &rd->last_collected_value, 0);
        rrddimvar_create(rd, RRDVAR_TYPE_TIME_T, NULL, "_last_collected_t", &rd->last_collected_time.tv_sec, 0);
    }

    pthread_rwlock_unlock(&st->rwlock);

    rrddim_index_add(st, rd);

    return(rd);
}

void rrddim_set_name(RRDSET *st, RRDDIM *rd, const char *name)
{
    if(unlikely(rd->name && !strcmp(rd->name, name)))
        return;

    debug(D_RRD_CALLS, "rrddim_set_name() from %s.%s to %s.%s", st->name, rd->name, st->name, name);

    char varname[CONFIG_MAX_NAME + 1];
    snprintfz(varname, CONFIG_MAX_NAME, "dim %s name", rd->id);
    rd->name = config_set_default(st->id, varname, name);

    rrddimvar_rename_all(rd);
}

void rrddim_free(RRDSET *st, RRDDIM *rd)
{
    debug(D_RRD_CALLS, "rrddim_free() %s.%s", st->name, rd->name);

    if(rd == st->dimensions)
        st->dimensions = rd->next;
    else {
        RRDDIM *i;
        for (i = st->dimensions; i && i->next != rd; i = i->next) ;

        if (i && i->next == rd)
            i->next = rd->next;
        else
            error("Request to free dimension '%s.%s' but it is not linked.", st->id, rd->name);
    }
    rd->next = NULL;

    while(rd->variables)
        rrddimvar_free(rd->variables);

    rrddim_index_del(st, rd);

    // free(rd->annotations);
    if(rd->mapped == RRD_MEMORY_MODE_SAVE) {
        debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_filename);
        savememory(rd->cache_filename, rd, rd->memsize);

        debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
        munmap(rd, rd->memsize);
    }
    else if(rd->mapped == RRD_MEMORY_MODE_MAP) {
        debug(D_RRD_CALLS, "Unmapping dimension '%s'.", rd->name);
        munmap(rd, rd->memsize);
    }
    else {
        debug(D_RRD_CALLS, "Removing dimension '%s'.", rd->name);
        freez(rd);
    }
}

void rrdset_free_all(void)
{
    info("Freeing all memory...");

    rrdhost_rwlock(&localhost);

    RRDSET *st;
    for(st = localhost.rrdset_root; st ;) {
        RRDSET *next = st->next;

        pthread_rwlock_wrlock(&st->rwlock);

        while(st->variables)
            rrdsetvar_free(st->variables);

        while(st->alarms)
            rrdsetcalc_unlink(st->alarms);

        while(st->dimensions)
            rrddim_free(st, st->dimensions);

        rrdset_index_del(&localhost, st);

        st->rrdfamily->use_count--;
        if(!st->rrdfamily->use_count)
            rrdfamily_free(st->rrdfamily);

        pthread_rwlock_unlock(&st->rwlock);

        if(st->mapped == RRD_MEMORY_MODE_SAVE) {
            debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_filename);
            savememory(st->cache_filename, st, st->memsize);

            debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
            munmap(st, st->memsize);
        }
        else if(st->mapped == RRD_MEMORY_MODE_MAP) {
            debug(D_RRD_CALLS, "Unmapping stats '%s'.", st->name);
            munmap(st, st->memsize);
        }
        else
            freez(st);

        st = next;
    }
    localhost.rrdset_root = NULL;

    rrdhost_unlock(&localhost);

    info("Memory cleanup completed...");
}

void rrdset_save_all(void) {
    info("Saving database...");

    RRDSET *st;
    RRDDIM *rd;

    rrdhost_rwlock(&localhost);
    for(st = localhost.rrdset_root; st ; st = st->next) {
        pthread_rwlock_wrlock(&st->rwlock);

        if(st->mapped == RRD_MEMORY_MODE_SAVE) {
            debug(D_RRD_CALLS, "Saving stats '%s' to '%s'.", st->name, st->cache_filename);
            savememory(st->cache_filename, st, st->memsize);
        }

        for(rd = st->dimensions; rd ; rd = rd->next) {
            if(likely(rd->mapped == RRD_MEMORY_MODE_SAVE)) {
                debug(D_RRD_CALLS, "Saving dimension '%s' to '%s'.", rd->name, rd->cache_filename);
                savememory(rd->cache_filename, rd, rd->memsize);
            }
        }

        pthread_rwlock_unlock(&st->rwlock);
    }
    rrdhost_unlock(&localhost);
}


RRDSET *rrdset_find(const char *id)
{
    debug(D_RRD_CALLS, "rrdset_find() for chart %s", id);

    RRDSET *st = rrdset_index_find(&localhost, id, 0);
    return(st);
}

RRDSET *rrdset_find_bytype(const char *type, const char *id)
{
    debug(D_RRD_CALLS, "rrdset_find_bytype() for chart %s.%s", type, id);

    char buf[RRD_ID_LENGTH_MAX + 1];

    strncpyz(buf, type, RRD_ID_LENGTH_MAX - 1);
    strcat(buf, ".");
    int len = (int) strlen(buf);
    strncpyz(&buf[len], id, (size_t) (RRD_ID_LENGTH_MAX - len));

    return(rrdset_find(buf));
}

RRDSET *rrdset_find_byname(const char *name)
{
    debug(D_RRD_CALLS, "rrdset_find_byname() for chart %s", name);

    RRDSET *st = rrdset_index_find_name(&localhost, name, 0);
    return(st);
}

RRDDIM *rrddim_find(RRDSET *st, const char *id)
{
    debug(D_RRD_CALLS, "rrddim_find() for chart %s, dimension %s", st->name, id);

    return rrddim_index_find(st, id, 0);
}

int rrddim_hide(RRDSET *st, const char *id)
{
    debug(D_RRD_CALLS, "rrddim_hide() for chart %s, dimension %s", st->name, id);

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
        return 1;
    }

    rd->flags |= RRDDIM_FLAG_HIDDEN;
    return 0;
}

int rrddim_unhide(RRDSET *st, const char *id)
{
    debug(D_RRD_CALLS, "rrddim_unhide() for chart %s, dimension %s", st->name, id);

    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
        return 1;
    }

    if(rd->flags & RRDDIM_FLAG_HIDDEN) rd->flags ^= RRDDIM_FLAG_HIDDEN;
    return 0;
}

collected_number rrddim_set_by_pointer(RRDSET *st, RRDDIM *rd, collected_number value)
{
    debug(D_RRD_CALLS, "rrddim_set_by_pointer() for chart %s, dimension %s, value " COLLECTED_NUMBER_FORMAT, st->name, rd->name, value);

    now_realtime_timeval(&rd->last_collected_time);
    rd->collected_value = value;
    rd->updated = 1;
    rd->counter++;

    // fprintf(stderr, "%s.%s %llu " COLLECTED_NUMBER_FORMAT " dt %0.6f" " rate " CALCULATED_NUMBER_FORMAT "\n", st->name, rd->name, st->usec_since_last_update, value, (float)((double)st->usec_since_last_update / (double)1000000), (calculated_number)((value - rd->last_collected_value) * (calculated_number)rd->multiplier / (calculated_number)rd->divisor * 1000000.0 / (calculated_number)st->usec_since_last_update));

    return rd->last_collected_value;
}

collected_number rrddim_set(RRDSET *st, const char *id, collected_number value)
{
    RRDDIM *rd = rrddim_find(st, id);
    if(unlikely(!rd)) {
        error("Cannot find dimension with id '%s' on stats '%s' (%s).", id, st->name, st->id);
        return 0;
    }

    return rrddim_set_by_pointer(st, rd, value);
}

void rrdset_next_usec_unfiltered(RRDSET *st, usec_t microseconds)
{
    if(unlikely(!st->last_collected_time.tv_sec || !microseconds)) {
        // the first entry
        microseconds = st->update_every * USEC_PER_SEC;
    }
    st->usec_since_last_update = microseconds;
}

void rrdset_next_usec(RRDSET *st, usec_t microseconds)
{
    struct timeval now;
    now_realtime_timeval(&now);

    if(unlikely(!st->last_collected_time.tv_sec)) {
        // the first entry
        microseconds = st->update_every * USEC_PER_SEC;
    }
    else if(unlikely(!microseconds)) {
        // no dt given by the plugin
        microseconds = dt_usec(&now, &st->last_collected_time);
    }
    else {
        // microseconds has the time since the last collection
        usec_t now_usec = timeval_usec(&now);
        usec_t last_usec = timeval_usec(&st->last_collected_time);
        usec_t since_last_usec = dt_usec(&now, &st->last_collected_time);

        // verify the microseconds given is good
        if(unlikely(microseconds > since_last_usec)) {
            debug(D_RRD_CALLS, "dt %llu usec given is too big - it leads %llu usec to the future, for chart '%s' (%s).", microseconds, microseconds - since_last_usec, st->name, st->id);

#ifdef NETDATA_INTERNAL_CHECKS
            if(unlikely(last_usec + microseconds > now_usec + 1000))
                error("dt %llu usec given is too big - it leads %llu usec to the future, for chart '%s' (%s).", microseconds, microseconds - since_last_usec, st->name, st->id);
#endif

            microseconds = since_last_usec;
        }
        else if(unlikely(microseconds < since_last_usec * 0.8)) {
            debug(D_RRD_CALLS, "dt %llu usec given is too small - expected %llu usec up to -20%%, for chart '%s' (%s).", microseconds, since_last_usec, st->name, st->id);

#ifdef NETDATA_INTERNAL_CHECKS
            error("dt %llu usec given is too small - expected %llu usec up to -20%%, for chart '%s' (%s).", microseconds, since_last_usec, st->name, st->id);
#endif
            microseconds = since_last_usec;
        }
    }
    debug(D_RRD_CALLS, "rrdset_next_usec() for chart %s with microseconds %llu", st->name, microseconds);

    if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: NEXT: %llu microseconds", st->name, microseconds);
    st->usec_since_last_update = microseconds;
}

usec_t rrdset_done(RRDSET *st)
{
    if(unlikely(netdata_exit)) return 0;

    debug(D_RRD_CALLS, "rrdset_done() for chart %s", st->name);

    RRDDIM *rd;

    int
        pthreadoldcancelstate;  // store the old cancelable pthread state, to restore it at the end

    char
        store_this_entry = 1,   // boolean: 1 = store this entry, 0 = don't store this entry
        first_entry = 0;        // boolean: 1 = this is the first entry seen for this chart, 0 = all other entries

    unsigned int
        stored_entries = 0;     // the number of entries we have stored in the db, during this call to rrdset_done()

    usec_t
        last_collect_ut,        // the timestamp in microseconds, of the last collected value
        now_collect_ut,         // the timestamp in microseconds, of this collected value (this is NOW)
        last_stored_ut,         // the timestamp in microseconds, of the last stored entry in the db
        next_store_ut,          // the timestamp in microseconds, of the next entry to store in the db
        update_every_ut = st->update_every * USEC_PER_SEC; // st->update_every in microseconds

    if(unlikely(pthread_setcancelstate(PTHREAD_CANCEL_DISABLE, &pthreadoldcancelstate) != 0))
        error("Cannot set pthread cancel state to DISABLE.");

    // a read lock is OK here
    pthread_rwlock_rdlock(&st->rwlock);

/*
    // enable the chart, if it was disabled
    if(unlikely(rrd_delete_unupdated_dimensions) && !st->enabled)
        st->enabled = 1;
*/

    // check if the chart has a long time to be updated
    if(unlikely(st->usec_since_last_update > st->entries * update_every_ut)) {
        info("%s: took too long to be updated (%0.3Lf secs). Resetting it.", st->name, (long double)(st->usec_since_last_update / 1000000.0));
        rrdset_reset(st);
        st->usec_since_last_update = update_every_ut;
        first_entry = 1;
    }
    if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: microseconds since last update: %llu", st->name, st->usec_since_last_update);

    // set last_collected_time
    if(unlikely(!st->last_collected_time.tv_sec)) {
        // it is the first entry
        // set the last_collected_time to now
        now_realtime_timeval(&st->last_collected_time);
        timeval_align(&st->last_collected_time, st->update_every);

        last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec - update_every_ut;

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;

        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: has not set last_collected_time. Setting it now. Will not store the next entry.", st->name);
    }
    else {
        // it is not the first entry
        // calculate the proper last_collected_time, using usec_since_last_update
        last_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
        usec_t ut = last_collect_ut + st->usec_since_last_update;
        st->last_collected_time.tv_sec = (time_t) (ut / USEC_PER_SEC);
        st->last_collected_time.tv_usec = (suseconds_t) (ut % USEC_PER_SEC);
    }

    // if this set has not been updated in the past
    // we fake the last_update time to be = now - usec_since_last_update
    if(unlikely(!st->last_updated.tv_sec)) {
        // it has never been updated before
        // set a fake last_updated, in the past using usec_since_last_update
        usec_t ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec - st->usec_since_last_update;
        st->last_updated.tv_sec = (time_t) (ut / USEC_PER_SEC);
        st->last_updated.tv_usec = (suseconds_t) (ut % USEC_PER_SEC);

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;

        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: initializing last_updated to now - %llu microseconds (%0.3Lf). Will not store the next entry.", st->name, st->usec_since_last_update, (long double)ut/1000000.0);
    }

    // check if we will re-write the entire data set
    if(unlikely(dt_usec(&st->last_collected_time, &st->last_updated) > st->entries * update_every_ut)) {
        info("%s: too old data (last updated at %ld.%ld, last collected at %ld.%ld). Resetting it. Will not store the next entry.", st->name, st->last_updated.tv_sec, st->last_updated.tv_usec, st->last_collected_time.tv_sec, st->last_collected_time.tv_usec);
        rrdset_reset(st);

        st->usec_since_last_update = update_every_ut;

        now_realtime_timeval(&st->last_collected_time);
        timeval_align(&st->last_collected_time, st->update_every);

        usec_t ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec - st->usec_since_last_update;
        st->last_updated.tv_sec = (time_t) (ut / USEC_PER_SEC);
        st->last_updated.tv_usec = (suseconds_t) (ut % USEC_PER_SEC);

        // the first entry should not be stored
        store_this_entry = 0;
        first_entry = 1;
    }

    // these are the 3 variables that will help us in interpolation
    // last_stored_ut = the last time we added a value to the storage
    // now_collect_ut = the time the current value has been collected
    // next_store_ut  = the time of the next interpolation point
    last_stored_ut = st->last_updated.tv_sec * USEC_PER_SEC + st->last_updated.tv_usec;
    now_collect_ut = st->last_collected_time.tv_sec * USEC_PER_SEC + st->last_collected_time.tv_usec;
    next_store_ut  = (st->last_updated.tv_sec + st->update_every) * USEC_PER_SEC;

    if(unlikely(st->debug)) {
        debug(D_RRD_STATS, "%s: last_collect_ut = %0.3Lf (last collection time)", st->name, (long double)last_collect_ut/1000000.0);
        debug(D_RRD_STATS, "%s: now_collect_ut  = %0.3Lf (current collection time)", st->name, (long double)now_collect_ut/1000000.0);
        debug(D_RRD_STATS, "%s: last_stored_ut  = %0.3Lf (last updated time)", st->name, (long double)last_stored_ut/1000000.0);
        debug(D_RRD_STATS, "%s: next_store_ut   = %0.3Lf (next interpolation point)", st->name, (long double)next_store_ut/1000000.0);
    }

    if(unlikely(!st->counter_done)) {
        store_this_entry = 0;
        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: Will not store the next entry.", st->name);
    }
    st->counter_done++;

    // calculate totals and count the dimensions
    int dimensions;
    st->collected_total = 0;
    for( rd = st->dimensions, dimensions = 0 ; rd ; rd = rd->next, dimensions++ )
        if(likely(rd->updated)) st->collected_total += rd->collected_value;

    uint32_t storage_flags = SN_EXISTS;

    // process all dimensions to calculate their values
    // based on the collected figures only
    // at this stage we do not interpolate anything
    for( rd = st->dimensions ; rd ; rd = rd->next ) {

        if(unlikely(!rd->updated)) {
            rd->calculated_value = 0;
            continue;
        }

        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: START "
            " last_collected_value = " COLLECTED_NUMBER_FORMAT
            " collected_value = " COLLECTED_NUMBER_FORMAT
            " last_calculated_value = " CALCULATED_NUMBER_FORMAT
            " calculated_value = " CALCULATED_NUMBER_FORMAT
            , st->id, rd->name
            , rd->last_collected_value
            , rd->collected_value
            , rd->last_calculated_value
            , rd->calculated_value
            );

        switch(rd->algorithm) {
            case RRDDIM_ABSOLUTE:
                rd->calculated_value = (calculated_number)rd->collected_value
                    * (calculated_number)rd->multiplier
                    / (calculated_number)rd->divisor;

                if(unlikely(st->debug))
                    debug(D_RRD_STATS, "%s/%s: CALC ABS/ABS-NO-IN "
                        CALCULATED_NUMBER_FORMAT " = "
                        COLLECTED_NUMBER_FORMAT
                        " * " CALCULATED_NUMBER_FORMAT
                        " / " CALCULATED_NUMBER_FORMAT
                        , st->id, rd->name
                        , rd->calculated_value
                        , rd->collected_value
                        , (calculated_number)rd->multiplier
                        , (calculated_number)rd->divisor
                        );
                break;

            case RRDDIM_PCENT_OVER_ROW_TOTAL:
                if(unlikely(!st->collected_total))
                    rd->calculated_value = 0;
                else
                    // the percentage of the current value
                    // over the total of all dimensions
                    rd->calculated_value =
                          (calculated_number)100
                        * (calculated_number)rd->collected_value
                        / (calculated_number)st->collected_total;

                if(unlikely(st->debug))
                    debug(D_RRD_STATS, "%s/%s: CALC PCENT-ROW "
                        CALCULATED_NUMBER_FORMAT " = 100"
                        " * " COLLECTED_NUMBER_FORMAT
                        " / " COLLECTED_NUMBER_FORMAT
                        , st->id, rd->name
                        , rd->calculated_value
                        , rd->collected_value
                        , st->collected_total
                        );
                break;

            case RRDDIM_INCREMENTAL:
                if(unlikely(rd->counter <= 1)) {
                    rd->calculated_value = 0;
                    continue;
                }

                // if the new is smaller than the old (an overflow, or reset), set the old equal to the new
                // to reset the calculation (it will give zero as the calculation for this second)
                if(unlikely(rd->last_collected_value > rd->collected_value)) {
                    debug(D_RRD_STATS, "%s.%s: RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                            , st->name, rd->name
                            , rd->last_collected_value
                            , rd->collected_value);
                    if(!(rd->flags & RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)) storage_flags = SN_EXISTS_RESET;
                    rd->last_collected_value = rd->collected_value;
                }

                rd->calculated_value +=
                      (calculated_number)(rd->collected_value - rd->last_collected_value)
                    * (calculated_number)rd->multiplier
                    / (calculated_number)rd->divisor;

                if(unlikely(st->debug))
                    debug(D_RRD_STATS, "%s/%s: CALC INC PRE "
                        CALCULATED_NUMBER_FORMAT " = ("
                        COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT
                        ")"
                        " * " CALCULATED_NUMBER_FORMAT
                        " / " CALCULATED_NUMBER_FORMAT
                        , st->id, rd->name
                        , rd->calculated_value
                        , rd->collected_value, rd->last_collected_value
                        , (calculated_number)rd->multiplier
                        , (calculated_number)rd->divisor
                        );
                break;

            case RRDDIM_PCENT_OVER_DIFF_TOTAL:
                if(unlikely(rd->counter <= 1)) {
                    rd->calculated_value = 0;
                    continue;
                }

                // if the new is smaller than the old (an overflow, or reset), set the old equal to the new
                // to reset the calculation (it will give zero as the calculation for this second)
                if(unlikely(rd->last_collected_value > rd->collected_value)) {
                    debug(D_RRD_STATS, "%s.%s: RESET or OVERFLOW. Last collected value = " COLLECTED_NUMBER_FORMAT ", current = " COLLECTED_NUMBER_FORMAT
                    , st->name, rd->name
                    , rd->last_collected_value
                    , rd->collected_value);
                    if(!(rd->flags & RRDDIM_FLAG_DONT_DETECT_RESETS_OR_OVERFLOWS)) storage_flags = SN_EXISTS_RESET;
                    rd->last_collected_value = rd->collected_value;
                }

                // the percentage of the current increment
                // over the increment of all dimensions together
                if(unlikely(st->collected_total == st->last_collected_total))
                    rd->calculated_value = 0;
                else
                    rd->calculated_value =
                          (calculated_number)100
                        * (calculated_number)(rd->collected_value - rd->last_collected_value)
                        / (calculated_number)(st->collected_total - st->last_collected_total);

                if(unlikely(st->debug))
                    debug(D_RRD_STATS, "%s/%s: CALC PCENT-DIFF "
                        CALCULATED_NUMBER_FORMAT " = 100"
                        " * (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                        " / (" COLLECTED_NUMBER_FORMAT " - " COLLECTED_NUMBER_FORMAT ")"
                        , st->id, rd->name
                        , rd->calculated_value
                        , rd->collected_value, rd->last_collected_value
                        , st->collected_total, st->last_collected_total
                        );
                break;

            default:
                // make the default zero, to make sure
                // it gets noticed when we add new types
                rd->calculated_value = 0;

                if(unlikely(st->debug))
                    debug(D_RRD_STATS, "%s/%s: CALC "
                        CALCULATED_NUMBER_FORMAT " = 0"
                        , st->id, rd->name
                        , rd->calculated_value
                        );
                break;
        }

        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: PHASE2 "
            " last_collected_value = " COLLECTED_NUMBER_FORMAT
            " collected_value = " COLLECTED_NUMBER_FORMAT
            " last_calculated_value = " CALCULATED_NUMBER_FORMAT
            " calculated_value = " CALCULATED_NUMBER_FORMAT
            , st->id, rd->name
            , rd->last_collected_value
            , rd->collected_value
            , rd->last_calculated_value
            , rd->calculated_value
            );

    }

    // at this point we have all the calculated values ready
    // it is now time to interpolate values on a second boundary

    if(unlikely(now_collect_ut < next_store_ut)) {
        // this is collected in the same interpolation point
        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: THIS IS IN THE SAME INTERPOLATION POINT", st->name);
#ifdef NETDATA_INTERNAL_CHECKS
        info("%s is collected in the same interpolation point: short by %llu microseconds", st->name, next_store_ut - now_collect_ut);
#endif
    }

    usec_t first_ut = last_stored_ut;
    long long iterations = (now_collect_ut - last_stored_ut) / (update_every_ut);
    if((now_collect_ut % (update_every_ut)) == 0) iterations++;

    for( ; next_store_ut <= now_collect_ut ; last_collect_ut = next_store_ut, next_store_ut += update_every_ut, iterations-- ) {
#ifdef NETDATA_INTERNAL_CHECKS
        if(iterations < 0) { error("%s: iterations calculation wrapped! first_ut = %llu, last_stored_ut = %llu, next_store_ut = %llu, now_collect_ut = %llu", st->name, first_ut, last_stored_ut, next_store_ut, now_collect_ut); }
#endif

        if(unlikely(st->debug)) {
            debug(D_RRD_STATS, "%s: last_stored_ut = %0.3Lf (last updated time)", st->name, (long double)last_stored_ut/1000000.0);
            debug(D_RRD_STATS, "%s: next_store_ut  = %0.3Lf (next interpolation point)", st->name, (long double)next_store_ut/1000000.0);
        }

        st->last_updated.tv_sec = (time_t) (next_store_ut / USEC_PER_SEC);
        st->last_updated.tv_usec = 0;

        for( rd = st->dimensions ; likely(rd) ; rd = rd->next ) {
            calculated_number new_value;

            switch(rd->algorithm) {
                case RRDDIM_INCREMENTAL:
                    new_value = (calculated_number)
                        (      rd->calculated_value
                            * (calculated_number)(next_store_ut - last_collect_ut)
                            / (calculated_number)(now_collect_ut - last_collect_ut)
                        );

                    if(unlikely(st->debug))
                        debug(D_RRD_STATS, "%s/%s: CALC2 INC "
                            CALCULATED_NUMBER_FORMAT " = "
                            CALCULATED_NUMBER_FORMAT
                            " * %llu"
                            " / %llu"
                            , st->id, rd->name
                            , new_value
                            , rd->calculated_value
                            , (next_store_ut - last_stored_ut)
                            , (now_collect_ut - last_stored_ut)
                            );

                    rd->calculated_value -= new_value;
                    new_value += rd->last_calculated_value;
                    rd->last_calculated_value = 0;
                    new_value /= (calculated_number)st->update_every;

                    if(unlikely(next_store_ut - last_stored_ut < update_every_ut)) {
                        if(unlikely(st->debug))
                            debug(D_RRD_STATS, "%s/%s: COLLECTION POINT IS SHORT " CALCULATED_NUMBER_FORMAT " - EXTRAPOLATING",
                                st->id, rd->name
                                , (calculated_number)(next_store_ut - last_stored_ut)
                                );
                        new_value = new_value * (calculated_number)(st->update_every * 1000000) / (calculated_number)(next_store_ut - last_stored_ut);
                    }
                    break;

                case RRDDIM_ABSOLUTE:
                case RRDDIM_PCENT_OVER_ROW_TOTAL:
                case RRDDIM_PCENT_OVER_DIFF_TOTAL:
                default:
                    if(iterations == 1) {
                        // this is the last iteration
                        // do not interpolate
                        // just show the calculated value

                        new_value = rd->calculated_value;
                    }
                    else {
                        // we have missed an update
                        // interpolate in the middle values

                        new_value = (calculated_number)
                            (   (     (rd->calculated_value - rd->last_calculated_value)
                                    * (calculated_number)(next_store_ut - last_collect_ut)
                                    / (calculated_number)(now_collect_ut - last_collect_ut)
                                )
                                +  rd->last_calculated_value
                            );

                        if(unlikely(st->debug))
                            debug(D_RRD_STATS, "%s/%s: CALC2 DEF "
                                CALCULATED_NUMBER_FORMAT " = ((("
                                "(" CALCULATED_NUMBER_FORMAT " - " CALCULATED_NUMBER_FORMAT ")"
                                " * %llu"
                                " / %llu) + " CALCULATED_NUMBER_FORMAT
                                , st->id, rd->name
                                , new_value
                                , rd->calculated_value, rd->last_calculated_value
                                , (next_store_ut - first_ut)
                                , (now_collect_ut - first_ut), rd->last_calculated_value
                                );
                    }
                    break;
            }

            if(unlikely(!store_this_entry)) {
                rd->values[st->current_entry] = pack_storage_number(0, SN_NOT_EXISTS);
                continue;
            }

            if(likely(rd->updated && rd->counter > 1 && iterations < st->gap_when_lost_iterations_above)) {
                rd->values[st->current_entry] = pack_storage_number(new_value, storage_flags );
                rd->last_stored_value = new_value;

                if(unlikely(st->debug))
                    debug(D_RRD_STATS, "%s/%s: STORE[%ld] "
                        CALCULATED_NUMBER_FORMAT " = " CALCULATED_NUMBER_FORMAT
                        , st->id, rd->name
                        , st->current_entry
                        , unpack_storage_number(rd->values[st->current_entry]), new_value
                        );
            }
            else {
                if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: STORE[%ld] = NON EXISTING "
                        , st->id, rd->name
                        , st->current_entry
                        );
                rd->values[st->current_entry] = pack_storage_number(0, SN_NOT_EXISTS);
                rd->last_stored_value = NAN;
            }

            stored_entries++;

            if(unlikely(st->debug)) {
                calculated_number t1 = new_value * (calculated_number)rd->multiplier / (calculated_number)rd->divisor;
                calculated_number t2 = unpack_storage_number(rd->values[st->current_entry]);
                calculated_number accuracy = accuracy_loss(t1, t2);
                debug(D_RRD_STATS, "%s/%s: UNPACK[%ld] = " CALCULATED_NUMBER_FORMAT " FLAGS=0x%08x (original = " CALCULATED_NUMBER_FORMAT ", accuracy loss = " CALCULATED_NUMBER_FORMAT "%%%s)"
                        , st->id, rd->name
                        , st->current_entry
                        , t2
                        , get_storage_number_flags(rd->values[st->current_entry])
                        , t1
                        , accuracy
                        , (accuracy > ACCURACY_LOSS) ? " **TOO BIG** " : ""
                        );

                rd->collected_volume += t1;
                rd->stored_volume += t2;
                accuracy = accuracy_loss(rd->collected_volume, rd->stored_volume);
                debug(D_RRD_STATS, "%s/%s: VOLUME[%ld] = " CALCULATED_NUMBER_FORMAT ", calculated  = " CALCULATED_NUMBER_FORMAT ", accuracy loss = " CALCULATED_NUMBER_FORMAT "%%%s"
                        , st->id, rd->name
                        , st->current_entry
                        , rd->stored_volume
                        , rd->collected_volume
                        , accuracy
                        , (accuracy > ACCURACY_LOSS) ? " **TOO BIG** " : ""
                        );

            }
        }
        // reset the storage flags for the next point, if any;
        storage_flags = SN_EXISTS;

        st->counter++;
        st->current_entry = ((st->current_entry + 1) >= st->entries) ? 0 : st->current_entry + 1;
        last_stored_ut = next_store_ut;
    }

    st->last_collected_total  = st->collected_total;

    for( rd = st->dimensions; rd ; rd = rd->next ) {
        if(unlikely(!rd->updated)) continue;

        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: setting last_collected_value (old: " COLLECTED_NUMBER_FORMAT ") to last_collected_value (new: " COLLECTED_NUMBER_FORMAT ")", st->id, rd->name, rd->last_collected_value, rd->collected_value);
        rd->last_collected_value = rd->collected_value;

        switch(rd->algorithm) {
            case RRDDIM_INCREMENTAL:
                if(unlikely(!first_entry)) {
                    if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: setting last_calculated_value (old: " CALCULATED_NUMBER_FORMAT ") to last_calculated_value (new: " CALCULATED_NUMBER_FORMAT ")", st->id, rd->name, rd->last_calculated_value + rd->calculated_value, rd->calculated_value);
                    rd->last_calculated_value += rd->calculated_value;
                }
                else {
                    if(unlikely(st->debug)) debug(D_RRD_STATS, "%s: THIS IS THE FIRST POINT", st->name);
                }
                break;

            case RRDDIM_ABSOLUTE:
            case RRDDIM_PCENT_OVER_ROW_TOTAL:
            case RRDDIM_PCENT_OVER_DIFF_TOTAL:
                if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: setting last_calculated_value (old: " CALCULATED_NUMBER_FORMAT ") to last_calculated_value (new: " CALCULATED_NUMBER_FORMAT ")", st->id, rd->name, rd->last_calculated_value, rd->calculated_value);
                rd->last_calculated_value = rd->calculated_value;
                break;
        }

        rd->calculated_value = 0;
        rd->collected_value = 0;
        rd->updated = 0;

        if(unlikely(st->debug)) debug(D_RRD_STATS, "%s/%s: END "
            " last_collected_value = " COLLECTED_NUMBER_FORMAT
            " collected_value = " COLLECTED_NUMBER_FORMAT
            " last_calculated_value = " CALCULATED_NUMBER_FORMAT
            " calculated_value = " CALCULATED_NUMBER_FORMAT
            , st->id, rd->name
            , rd->last_collected_value
            , rd->collected_value
            , rd->last_calculated_value
            , rd->calculated_value
            );
    }

    // ALL DONE ABOUT THE DATA UPDATE
    // --------------------------------------------------------------------

/*
    // find if there are any obsolete dimensions (not updated recently)
    if(unlikely(rrd_delete_unupdated_dimensions)) {

        for( rd = st->dimensions; likely(rd) ; rd = rd->next )
            if((rd->last_collected_time.tv_sec + (rrd_delete_unupdated_dimensions * st->update_every)) < st->last_collected_time.tv_sec)
                break;

        if(unlikely(rd)) {
            RRDDIM *last;
            // there is dimension to free
            // upgrade our read lock to a write lock
            pthread_rwlock_unlock(&st->rwlock);
            pthread_rwlock_wrlock(&st->rwlock);

            for( rd = st->dimensions, last = NULL ; likely(rd) ; ) {
                // remove it only it is not updated in rrd_delete_unupdated_dimensions seconds

                if(unlikely((rd->last_collected_time.tv_sec + (rrd_delete_unupdated_dimensions * st->update_every)) < st->last_collected_time.tv_sec)) {
                    info("Removing obsolete dimension '%s' (%s) of '%s' (%s).", rd->name, rd->id, st->name, st->id);

                    if(unlikely(!last)) {
                        st->dimensions = rd->next;
                        rd->next = NULL;
                        rrddim_free(st, rd);
                        rd = st->dimensions;
                        continue;
                    }
                    else {
                        last->next = rd->next;
                        rd->next = NULL;
                        rrddim_free(st, rd);
                        rd = last->next;
                        continue;
                    }
                }

                last = rd;
                rd = rd->next;
            }

            if(unlikely(!st->dimensions)) {
                info("Disabling chart %s (%s) since it does not have any dimensions", st->name, st->id);
                st->enabled = 0;
            }
        }
    }
*/

    pthread_rwlock_unlock(&st->rwlock);

    if(unlikely(pthread_setcancelstate(pthreadoldcancelstate, NULL) != 0))
        error("Cannot set pthread cancel state to RESTORE (%d).", pthreadoldcancelstate);

    return(st->usec_since_last_update);
}
