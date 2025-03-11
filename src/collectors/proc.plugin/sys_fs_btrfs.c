// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_BTRFS_NAME "/sys/fs/btrfs"

typedef struct btrfs_disk {
    char *name;
    uint32_t hash;
    int exists;

    char *size_filename;
    unsigned long long size;

    struct btrfs_disk *next;
} BTRFS_DISK;

typedef struct btrfs_device {
    int id;
    int exists;

    char *error_stats_filename;
    RRDSET *st_error_stats;
    RRDDIM *rd_write_errs;
    RRDDIM *rd_read_errs;
    RRDDIM *rd_flush_errs;
    RRDDIM *rd_corruption_errs;
    RRDDIM *rd_generation_errs;
    collected_number write_errs;
    collected_number read_errs;
    collected_number flush_errs;
    collected_number corruption_errs;
    collected_number generation_errs;

    struct btrfs_device *next;
} BTRFS_DEVICE;

typedef struct btrfs_node {
    int exists;
    int logged_error;

    char *id;
    uint32_t hash;

    char *label;

    #define declare_btrfs_allocation_section_field(SECTION, FIELD) \
        char *allocation_ ## SECTION ## _ ## FIELD ## _filename; \
        unsigned long long int allocation_ ## SECTION ## _ ## FIELD;

    #define declare_btrfs_allocation_field(FIELD) \
        char *allocation_ ## FIELD ## _filename; \
        unsigned long long int allocation_ ## FIELD;

    RRDSET *st_allocation_disks;
    RRDDIM *rd_allocation_disks_unallocated;
    RRDDIM *rd_allocation_disks_data_used;
    RRDDIM *rd_allocation_disks_data_free;
    RRDDIM *rd_allocation_disks_metadata_used;
    RRDDIM *rd_allocation_disks_metadata_free;
    RRDDIM *rd_allocation_disks_system_used;
    RRDDIM *rd_allocation_disks_system_free;
    unsigned long long all_disks_total;

    RRDSET *st_allocation_data;
    RRDDIM *rd_allocation_data_free;
    RRDDIM *rd_allocation_data_used;
    declare_btrfs_allocation_section_field(data, total_bytes)
    declare_btrfs_allocation_section_field(data, bytes_used)
    declare_btrfs_allocation_section_field(data, disk_total)
    declare_btrfs_allocation_section_field(data, disk_used)

    RRDSET *st_allocation_metadata;
    RRDDIM *rd_allocation_metadata_free;
    RRDDIM *rd_allocation_metadata_used;
    RRDDIM *rd_allocation_metadata_reserved;
    declare_btrfs_allocation_section_field(metadata, total_bytes)
    declare_btrfs_allocation_section_field(metadata, bytes_used)
    declare_btrfs_allocation_section_field(metadata, disk_total)
    declare_btrfs_allocation_section_field(metadata, disk_used)
    //declare_btrfs_allocation_field(global_rsv_reserved)
    declare_btrfs_allocation_field(global_rsv_size)

    RRDSET *st_allocation_system;
    RRDDIM *rd_allocation_system_free;
    RRDDIM *rd_allocation_system_used;
    declare_btrfs_allocation_section_field(system, total_bytes)
    declare_btrfs_allocation_section_field(system, bytes_used)
    declare_btrfs_allocation_section_field(system, disk_total)
    declare_btrfs_allocation_section_field(system, disk_used)

    // --------------------------------------------------------------------
    // commit stats

    char *commit_stats_filename;

    RRDSET *st_commits;
    RRDDIM *rd_commits;
    long long commits_total;
    collected_number commits_new;

    RRDSET *st_commits_percentage_time;
    RRDDIM *rd_commits_percentage_time;
    long long commit_timings_total;
    long long commits_percentage_time;

    RRDSET *st_commit_timings;
    RRDDIM *rd_commit_timings_last;
    RRDDIM *rd_commit_timings_max;
    collected_number commit_timings_last;
    collected_number commit_timings_max;

    BTRFS_DISK *disks;

    BTRFS_DEVICE *devices;

    struct btrfs_node *next;
} BTRFS_NODE;

static BTRFS_NODE *nodes = NULL;

static inline int collect_btrfs_error_stats(BTRFS_DEVICE *device){
    char buffer[120 + 1];
    
    int ret = read_txt_file(device->error_stats_filename, buffer, sizeof(buffer));
    if(unlikely(ret)) {
        collector_error("BTRFS: failed to read '%s'", device->error_stats_filename);
        device->write_errs = 0;
        device->read_errs = 0;
        device->flush_errs = 0;
        device->corruption_errs = 0;
        device->generation_errs = 0;
        return ret;
    } 
    
    char *p = buffer;
    while(p){
        char *val = strsep_skip_consecutive_separators(&p, "\n");
        if(unlikely(!val || !*val)) break;
        char *key = strsep_skip_consecutive_separators(&val, " ");

        if(!strcmp(key, "write_errs")) device->write_errs = str2ull(val, NULL);
        else if(!strcmp(key, "read_errs")) device->read_errs = str2ull(val, NULL);
        else if(!strcmp(key, "flush_errs")) device->flush_errs = str2ull(val, NULL);
        else if(!strcmp(key, "corruption_errs")) device->corruption_errs = str2ull(val, NULL);
        else if(!strcmp(key, "generation_errs")) device->generation_errs = str2ull(val, NULL);
    }
    return 0;
}

static inline int collect_btrfs_commits_stats(BTRFS_NODE *node, int update_every){
    char buffer[120 + 1];
    
    int ret = read_txt_file(node->commit_stats_filename, buffer, sizeof(buffer));
    if(unlikely(ret)) {
        collector_error("BTRFS: failed to read '%s'", node->commit_stats_filename);
        node->commits_total = 0;
        node->commits_new = 0;
        node->commit_timings_last = 0;
        node->commit_timings_max = 0;
        node->commit_timings_total = 0;
        node->commits_percentage_time = 0;

        return ret;
    } 
    
    char *p = buffer;
    while(p){
        char *val = strsep_skip_consecutive_separators(&p, "\n");
        if(unlikely(!val || !*val)) break;
        char *key = strsep_skip_consecutive_separators(&val, " ");

        if(!strcmp(key, "commits")){
            long long commits_total_new = str2ull(val, NULL);
            if(likely(node->commits_total)){
                if((node->commits_new = commits_total_new - node->commits_total))
                    node->commits_total = commits_total_new;
            } else node->commits_total = commits_total_new;
        }
        else if(!strcmp(key, "last_commit_ms")) node->commit_timings_last = str2ull(val, NULL);
        else if(!strcmp(key, "max_commit_ms")) node->commit_timings_max = str2ull(val, NULL);
        else if(!strcmp(key, "total_commit_ms")) {
            long long commit_timings_total_new = str2ull(val, NULL);
            if(likely(node->commit_timings_total)){
                long time_delta = commit_timings_total_new - node->commit_timings_total;
                if(time_delta){
                    node->commits_percentage_time = time_delta * 10 / update_every;
                    node->commit_timings_total = commit_timings_total_new;
                } else node->commits_percentage_time = 0;
                
            } else node->commit_timings_total = commit_timings_total_new;           
        }
    }
    return 0;
}

static inline void btrfs_free_commits_stats(BTRFS_NODE *node){
    if(node->st_commits){
        rrdset_is_obsolete___safe_from_collector_thread(node->st_commits);
        rrdset_is_obsolete___safe_from_collector_thread(node->st_commit_timings);
    }
    freez(node->commit_stats_filename);
    node->commit_stats_filename = NULL;
}

static inline void btrfs_free_disk(BTRFS_DISK *d) {
    freez(d->name);
    freez(d->size_filename);
    freez(d);
}

static inline void btrfs_free_device(BTRFS_DEVICE *d) {
    if(d->st_error_stats)
        rrdset_is_obsolete___safe_from_collector_thread(d->st_error_stats);
    freez(d->error_stats_filename);
    freez(d);
}

static inline void btrfs_free_node(BTRFS_NODE *node) {
    // collector_info("BTRFS: destroying '%s'", node->id);

    if(node->st_allocation_disks)
        rrdset_is_obsolete___safe_from_collector_thread(node->st_allocation_disks);

    if(node->st_allocation_data)
        rrdset_is_obsolete___safe_from_collector_thread(node->st_allocation_data);

    if(node->st_allocation_metadata)
        rrdset_is_obsolete___safe_from_collector_thread(node->st_allocation_metadata);

    if(node->st_allocation_system)
        rrdset_is_obsolete___safe_from_collector_thread(node->st_allocation_system);

    freez(node->allocation_data_bytes_used_filename);
    freez(node->allocation_data_total_bytes_filename);

    freez(node->allocation_metadata_bytes_used_filename);
    freez(node->allocation_metadata_total_bytes_filename);

    freez(node->allocation_system_bytes_used_filename);
    freez(node->allocation_system_total_bytes_filename);

    btrfs_free_commits_stats(node);

    while(node->disks) {
        BTRFS_DISK *d = node->disks;
        node->disks = node->disks->next;
        btrfs_free_disk(d);
    }

     while(node->devices) {
        BTRFS_DEVICE *d = node->devices;
        node->devices = node->devices->next;
        btrfs_free_device(d);
    }

    freez(node->label);
    freez(node->id);
    freez(node);
}

static inline int find_btrfs_disks(BTRFS_NODE *node, const char *path) {
    char filename[FILENAME_MAX + 1];

    node->all_disks_total = 0;

    BTRFS_DISK *d;
    for(d = node->disks ; d ; d = d->next)
        d->exists = 0;

    DIR *dir = opendir(path);
    if (!dir) {
        if (!node->logged_error) {
            nd_log(NDLS_COLLECTORS, errno == ENOENT ? NDLP_INFO : NDLP_ERR, "BTRFS: Cannot open directory '%s'.", path);
            node->logged_error = 1;
        }
        return 1;
    }
    node->logged_error = 0;

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type != DT_LNK
            || !strcmp(de->d_name, ".")
            || !strcmp(de->d_name, "..")
                ) {
            // collector_info("BTRFS: ignoring '%s'", de->d_name);
            continue;
        }

        uint32_t hash = simple_hash(de->d_name);

        // --------------------------------------------------------------------
        // search for it

        for(d = node->disks ; d ; d = d->next) {
            if(hash == d->hash && !strcmp(de->d_name, d->name))
                break;
        }

        // --------------------------------------------------------------------
        // did we find it?

        if(!d) {
            d = callocz(sizeof(BTRFS_DISK), 1);

            d->name = strdupz(de->d_name);
            d->hash = simple_hash(d->name);

            snprintfz(filename, FILENAME_MAX, "%s/%s/size", path, de->d_name);
            d->size_filename = strdupz(filename);

            // link it
            d->next = node->disks;
            node->disks = d;
        }

        d->exists = 1;


        // --------------------------------------------------------------------
        // update the values

        if(read_single_number_file(d->size_filename, &d->size) != 0) {
            collector_error("BTRFS: failed to read '%s'", d->size_filename);
            d->exists = 0;
            continue;
        }

        // /sys/block/<name>/size is in fixed-size sectors of 512 bytes
        // https://github.com/torvalds/linux/blob/v6.2/block/genhd.c#L946-L950
        // https://github.com/torvalds/linux/blob/v6.2/include/linux/types.h#L120-L121
        // (also see #3481, #3483)
        node->all_disks_total += d->size * 512;
    }
    closedir(dir);

    // ------------------------------------------------------------------------
    // cleanup

    BTRFS_DISK *last = NULL;
    d = node->disks;

    while(d) {
        if(unlikely(!d->exists)) {
            if(unlikely(node->disks == d)) {
                node->disks = d->next;
                btrfs_free_disk(d);
                d = node->disks;
                last = NULL;
            }
            else {
                last->next = d->next;
                btrfs_free_disk(d);
                d = last->next;
            }

            continue;
        }

        last = d;
        d = d->next;
    }

    return 0;
}

static inline int find_btrfs_devices(BTRFS_NODE *node, const char *path) {
    char filename[FILENAME_MAX + 1];

    BTRFS_DEVICE *d;
    for(d = node->devices ; d ; d = d->next)
        d->exists = 0;

    DIR *dir = opendir(path);
    if (!dir) {
        if (!node->logged_error) {
            nd_log(NDLS_COLLECTORS, errno == ENOENT ? NDLP_INFO : NDLP_ERR, "BTRFS: Cannot open directory '%s'.", path);
            node->logged_error = 1;
        }
        return 1;
    }
    node->logged_error = 0;

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type != DT_DIR
            || !strcmp(de->d_name, ".")
            || !strcmp(de->d_name, "..")
                ) {
            // collector_info("BTRFS: ignoring '%s'", de->d_name);
            continue;
        }

        // internal_error("BTRFS: device found '%s'", de->d_name);

        // --------------------------------------------------------------------
        // search for it

        for(d = node->devices ; d ; d = d->next) {
            if(str2ll(de->d_name, NULL) == d->id){
                // collector_info("BTRFS: existing device id '%d'", d->id);
                break;
            }
        }

        // --------------------------------------------------------------------
        // did we find it?

        if(!d) {
            d = callocz(sizeof(BTRFS_DEVICE), 1);

            d->id = str2ll(de->d_name, NULL);
            // collector_info("BTRFS: new device with id '%d'", d->id);

            snprintfz(filename, FILENAME_MAX, "%s/%d/error_stats", path, d->id);
            d->error_stats_filename = strdupz(filename);
            // collector_info("BTRFS: error_stats_filename '%s'", filename);

            // link it
            d->next = node->devices;
            node->devices = d;
        }

        d->exists = 1;


        // --------------------------------------------------------------------
        // update the values

        if(unlikely(collect_btrfs_error_stats(d)))
            d->exists = 0; // 'd' will be garbaged collected in loop below
    }
    closedir(dir);

    // ------------------------------------------------------------------------
    // cleanup

    BTRFS_DEVICE *last = NULL;
    d = node->devices;

    while(d) {
        if(unlikely(!d->exists)) {
            if(unlikely(node->devices == d)) {
                node->devices = d->next;
                btrfs_free_device(d);
                d = node->devices;
                last = NULL;
            }
            else {
                last->next = d->next;
                btrfs_free_device(d);
                d = last->next;
            }

            continue;
        }

        last = d;
        d = d->next;
    }

    return 0;
}


static inline int find_all_btrfs_pools(const char *path, int update_every) {
    static int logged_error = 0;
    char filename[FILENAME_MAX + 1];

    BTRFS_NODE *node;
    for(node = nodes ; node ; node = node->next)
        node->exists = 0;

    DIR *dir = opendir(path);
    if (!dir) {
        if (!logged_error) {
            nd_log(NDLS_COLLECTORS, errno == ENOENT ? NDLP_INFO : NDLP_ERR, "BTRFS: Cannot open directory '%s'.", path);
            logged_error = 1;
        }
        return 1;
    }
    logged_error = 0;

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if(de->d_type != DT_DIR
           || !strcmp(de->d_name, ".")
           || !strcmp(de->d_name, "..")
           || !strcmp(de->d_name, "features")
                ) {
            // collector_info("BTRFS: ignoring '%s'", de->d_name);
            continue;
        }

        uint32_t hash = simple_hash(de->d_name);

        // search for it
        for(node = nodes ; node ; node = node->next) {
            if(hash == node->hash && !strcmp(de->d_name, node->id))
                break;
        }

        // did we find it?
        if(node) {
            // collector_info("BTRFS: already exists '%s'", de->d_name);
            node->exists = 1;

            // update the disk sizes
            snprintfz(filename, FILENAME_MAX, "%s/%s/devices", path, de->d_name);
            find_btrfs_disks(node, filename);

            // update devices
            snprintfz(filename, FILENAME_MAX, "%s/%s/devinfo", path, de->d_name);
            find_btrfs_devices(node, filename);  

            continue;
        }

        // collector_info("BTRFS: adding '%s'", de->d_name);

        // not found, create it
        node = callocz(sizeof(BTRFS_NODE), 1);

        node->id = strdupz(de->d_name);
        node->hash = simple_hash(node->id);
        node->exists = 1;

        {
            char label[FILENAME_MAX + 1] = "";

            snprintfz(filename, FILENAME_MAX, "%s/%s/label", path, de->d_name);
            if(read_txt_file(filename, label, sizeof(label)) != 0) {
                collector_error("BTRFS: failed to read '%s'", filename);
                btrfs_free_node(node);
                continue;
            }

            char *s = label;
            if (s[0])
                s = trim(label);

            if(s && s[0])
                node->label = strdupz(s);
            else
                node->label = strdupz(node->id);
        }

        // --------------------------------------------------------------------
        // macros to simplify our life

        #define init_btrfs_allocation_field(FIELD) {\
            snprintfz(filename, FILENAME_MAX, "%s/%s/allocation/" #FIELD, path, de->d_name); \
            if(read_single_number_file(filename, &node->allocation_ ## FIELD) != 0) {\
                collector_error("BTRFS: failed to read '%s'", filename);\
                btrfs_free_node(node);\
                continue;\
            }\
            if(!node->allocation_ ## FIELD ## _filename)\
                node->allocation_ ## FIELD ## _filename = strdupz(filename);\
        }

        #define init_btrfs_allocation_section_field(SECTION, FIELD) {\
            snprintfz(filename, FILENAME_MAX, "%s/%s/allocation/" #SECTION "/" #FIELD, path, de->d_name); \
            if(read_single_number_file(filename, &node->allocation_ ## SECTION ## _ ## FIELD) != 0) {\
                collector_error("BTRFS: failed to read '%s'", filename);\
                btrfs_free_node(node);\
                continue;\
            }\
            if(!node->allocation_ ## SECTION ## _ ## FIELD ## _filename)\
                node->allocation_ ## SECTION ## _ ## FIELD ## _filename = strdupz(filename);\
        }

        // --------------------------------------------------------------------
        // allocation/data

        init_btrfs_allocation_section_field(data, total_bytes);
        init_btrfs_allocation_section_field(data, bytes_used);
        init_btrfs_allocation_section_field(data, disk_total);
        init_btrfs_allocation_section_field(data, disk_used);


        // --------------------------------------------------------------------
        // allocation/metadata

        init_btrfs_allocation_section_field(metadata, total_bytes);
        init_btrfs_allocation_section_field(metadata, bytes_used);
        init_btrfs_allocation_section_field(metadata, disk_total);
        init_btrfs_allocation_section_field(metadata, disk_used);

        init_btrfs_allocation_field(global_rsv_size);
        // init_btrfs_allocation_field(global_rsv_reserved);


        // --------------------------------------------------------------------
        // allocation/system

        init_btrfs_allocation_section_field(system, total_bytes);
        init_btrfs_allocation_section_field(system, bytes_used);
        init_btrfs_allocation_section_field(system, disk_total);
        init_btrfs_allocation_section_field(system, disk_used);

        // --------------------------------------------------------------------
        // commit stats

        snprintfz(filename, FILENAME_MAX, "%s/%s/commit_stats", path, de->d_name);
        if(!node->commit_stats_filename) node->commit_stats_filename = strdupz(filename);
        if(unlikely(collect_btrfs_commits_stats(node, update_every))){
            collector_error("BTRFS: failed to collect commit stats for '%s'", node->id);
            btrfs_free_commits_stats(node);
        }     

        // --------------------------------------------------------------------
        // find all disks related to this node
        // and collect their sizes

        snprintfz(filename, FILENAME_MAX, "%s/%s/devices", path, de->d_name);
        find_btrfs_disks(node, filename);

        // --------------------------------------------------------------------
        // find all devices related to this node

        snprintfz(filename, FILENAME_MAX, "%s/%s/devinfo", path, de->d_name);
        find_btrfs_devices(node, filename);  

        // --------------------------------------------------------------------
        // link it

        // collector_info("BTRFS: linking '%s'", node->id);
        node->next = nodes;
        nodes = node;
    }
    closedir(dir);


    // ------------------------------------------------------------------------
    // cleanup

    BTRFS_NODE *last = NULL;
    node = nodes;

    while(node) {
        if(unlikely(!node->exists)) {
            if(unlikely(nodes == node)) {
                nodes = node->next;
                btrfs_free_node(node);
                node = nodes;
                last = NULL;
            }
            else {
                last->next = node->next;
                btrfs_free_node(node);
                node = last->next;
            }

            continue;
        }

        last = node;
        node = node->next;
    }

    return 0;
}

static void add_labels_to_btrfs(BTRFS_NODE *n, RRDSET *st) {
    rrdlabels_add(st->rrdlabels, "filesystem_uuid", n->id, RRDLABEL_SRC_AUTO);
    rrdlabels_add(st->rrdlabels, "filesystem_label", n->label, RRDLABEL_SRC_AUTO);
}

int do_sys_fs_btrfs(int update_every, usec_t dt) {
    static int initialized = 0
        , do_allocation_disks = CONFIG_BOOLEAN_AUTO
        , do_allocation_system = CONFIG_BOOLEAN_AUTO
        , do_allocation_data = CONFIG_BOOLEAN_AUTO
        , do_allocation_metadata = CONFIG_BOOLEAN_AUTO
        , do_commit_stats = CONFIG_BOOLEAN_AUTO
        , do_error_stats = CONFIG_BOOLEAN_AUTO;

    static usec_t refresh_delta = 0, refresh_every = 60 * USEC_PER_SEC;
    static const char *btrfs_path = NULL;

    (void)dt;

    if(unlikely(!initialized)) {
        initialized = 1;

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/fs/btrfs");
        btrfs_path = inicfg_get(&netdata_config, "plugin:proc:/sys/fs/btrfs", "path to monitor", filename);

        refresh_every = inicfg_get_duration_seconds(&netdata_config, "plugin:proc:/sys/fs/btrfs", "check for btrfs changes every", refresh_every / USEC_PER_SEC) * USEC_PER_SEC;
        refresh_delta = refresh_every;

        do_allocation_disks = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/sys/fs/btrfs", "physical disks allocation", do_allocation_disks);
        do_allocation_data = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/sys/fs/btrfs", "data allocation", do_allocation_data);
        do_allocation_metadata = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/sys/fs/btrfs", "metadata allocation", do_allocation_metadata);
        do_allocation_system = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/sys/fs/btrfs", "system allocation", do_allocation_system);
        do_commit_stats = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/sys/fs/btrfs", "commit stats", do_commit_stats);
        do_error_stats = inicfg_get_boolean_ondemand(&netdata_config, "plugin:proc:/sys/fs/btrfs", "error stats", do_error_stats);
    }

    refresh_delta += dt;
    if(refresh_delta >= refresh_every) {
        refresh_delta = 0;
        find_all_btrfs_pools(btrfs_path, update_every);
    }

    BTRFS_NODE *node;
    for(node = nodes; node ; node = node->next) {
        // --------------------------------------------------------------------
        // allocation/system

        #define collect_btrfs_allocation_field(FIELD) \
            read_single_number_file(node->allocation_ ## FIELD ## _filename, &node->allocation_ ## FIELD)

        #define collect_btrfs_allocation_section_field(SECTION, FIELD) \
            read_single_number_file(node->allocation_ ## SECTION ## _ ## FIELD ## _filename, &node->allocation_ ## SECTION ## _ ## FIELD)

        if(do_allocation_disks != CONFIG_BOOLEAN_NO) {
            if(     collect_btrfs_allocation_section_field(data, disk_total) != 0
                 || collect_btrfs_allocation_section_field(data, disk_used) != 0
                 || collect_btrfs_allocation_section_field(metadata, disk_total) != 0
                 || collect_btrfs_allocation_section_field(metadata, disk_used) != 0
                 || collect_btrfs_allocation_section_field(system, disk_total) != 0
                 || collect_btrfs_allocation_section_field(system, disk_used) != 0) {
                collector_error("BTRFS: failed to collect physical disks allocation for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_data != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(data, total_bytes) != 0
                || collect_btrfs_allocation_section_field(data, bytes_used) != 0) {
                collector_error("BTRFS: failed to collect allocation/data for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_metadata != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(metadata, total_bytes) != 0
                || collect_btrfs_allocation_section_field(metadata, bytes_used) != 0
                || collect_btrfs_allocation_field(global_rsv_size) != 0
                    ) {
                collector_error("BTRFS: failed to collect allocation/metadata for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_system != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(system, total_bytes) != 0
                || collect_btrfs_allocation_section_field(system, bytes_used) != 0) {
                collector_error("BTRFS: failed to collect allocation/system for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_commit_stats != CONFIG_BOOLEAN_NO && node->commit_stats_filename) {
            if (unlikely(collect_btrfs_commits_stats(node, update_every))) {
                collector_error("BTRFS: failed to collect commit stats for '%s'", node->id);
                btrfs_free_commits_stats(node);
            }
        }

        if(do_error_stats != CONFIG_BOOLEAN_NO) {
            for(BTRFS_DEVICE *d = node->devices ; d ; d = d->next) {
                if(unlikely(collect_btrfs_error_stats(d))){
                    collector_error("BTRFS: failed to collect error stats for '%s', devid:'%d'", node->id, d->id);
                    /* make it refresh btrfs at the next iteration, 
                     * btrfs_free_device(d) will be called in 
                     * find_btrfs_devices() as part of the garbage collection */
                    refresh_delta = refresh_every;
                }
            }
        }

        // --------------------------------------------------------------------
        // allocation/disks

        if (do_allocation_disks == CONFIG_BOOLEAN_YES || do_allocation_disks == CONFIG_BOOLEAN_AUTO) {
            do_allocation_disks = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_disks)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "disk_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "disk_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS Physical Disk Allocation");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_disks = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.disk"
                        , title
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_DISK
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_disks_unallocated   = rrddim_add(node->st_allocation_disks, "unallocated", NULL,      1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_data_free     = rrddim_add(node->st_allocation_disks, "data_free", "data free", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_data_used     = rrddim_add(node->st_allocation_disks, "data_used", "data used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_metadata_free = rrddim_add(node->st_allocation_disks, "meta_free", "meta free", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_metadata_used = rrddim_add(node->st_allocation_disks, "meta_used", "meta used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_system_free   = rrddim_add(node->st_allocation_disks, "sys_free",  "sys free",  1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_system_used   = rrddim_add(node->st_allocation_disks, "sys_used",  "sys used",  1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_allocation_disks);
            }

            // unsigned long long disk_used = node->allocation_data_disk_used + node->allocation_metadata_disk_used + node->allocation_system_disk_used;
            unsigned long long disk_total = node->allocation_data_disk_total + node->allocation_metadata_disk_total + node->allocation_system_disk_total;
            unsigned long long disk_unallocated = node->all_disks_total - disk_total;

            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_unallocated, disk_unallocated);
            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_data_used, node->allocation_data_disk_used);
            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_data_free, node->allocation_data_disk_total - node->allocation_data_disk_used);
            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_metadata_used, node->allocation_metadata_disk_used);
            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_metadata_free, node->allocation_metadata_disk_total - node->allocation_metadata_disk_used);
            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_system_used, node->allocation_system_disk_used);
            rrddim_set_by_pointer(node->st_allocation_disks, node->rd_allocation_disks_system_free, node->allocation_system_disk_total - node->allocation_system_disk_used);
            rrdset_done(node->st_allocation_disks);
        }

        // --------------------------------------------------------------------
        // allocation/data

        if (do_allocation_data == CONFIG_BOOLEAN_YES || do_allocation_data == CONFIG_BOOLEAN_AUTO) {
            do_allocation_data = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_data)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "data_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "data_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS Data Allocation");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_data = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.data"
                        , title
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_DATA
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_data_free = rrddim_add(node->st_allocation_data, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_data_used = rrddim_add(node->st_allocation_data, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_allocation_data);
            }

            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_free, node->allocation_data_total_bytes - node->allocation_data_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_used, node->allocation_data_bytes_used);
            rrdset_done(node->st_allocation_data);
        }

        // --------------------------------------------------------------------
        // allocation/metadata

        if (do_allocation_metadata == CONFIG_BOOLEAN_YES || do_allocation_metadata == CONFIG_BOOLEAN_AUTO) {
            do_allocation_metadata = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_metadata)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "metadata_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "metadata_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS Metadata Allocation");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_metadata = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.metadata"
                        , title
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_METADATA
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_metadata_free = rrddim_add(node->st_allocation_metadata, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_used = rrddim_add(node->st_allocation_metadata, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_reserved = rrddim_add(node->st_allocation_metadata, "reserved", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_allocation_metadata);
            }

            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_free, node->allocation_metadata_total_bytes - node->allocation_metadata_bytes_used - node->allocation_global_rsv_size);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_used, node->allocation_metadata_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_reserved, node->allocation_global_rsv_size);
            rrdset_done(node->st_allocation_metadata);
        }

        // --------------------------------------------------------------------
        // allocation/system

        if (do_allocation_system == CONFIG_BOOLEAN_YES || do_allocation_system == CONFIG_BOOLEAN_AUTO) {
            do_allocation_system = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_system)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "system_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "system_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS System Allocation");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_system = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.system"
                        , title
                        , "MiB"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_SYSTEM
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_system_free = rrddim_add(node->st_allocation_system, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_system_used = rrddim_add(node->st_allocation_system, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_allocation_system);
            }

            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_free, node->allocation_system_total_bytes - node->allocation_system_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_used, node->allocation_system_bytes_used);
            rrdset_done(node->st_allocation_system);
        }

        // --------------------------------------------------------------------
        // commit_stats

        if (do_commit_stats == CONFIG_BOOLEAN_YES || do_commit_stats == CONFIG_BOOLEAN_AUTO) {
            do_commit_stats = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_commits)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "commits_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "commits_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS Commits");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_commits = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.commits"
                        , title
                        , "commits"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_COMMITS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                node->rd_commits = rrddim_add(node->st_commits, "commits", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_commits);
            }

            rrddim_set_by_pointer(node->st_commits, node->rd_commits, node->commits_new);
            rrdset_done(node->st_commits);

            if(unlikely(!node->st_commits_percentage_time)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "commits_perc_time_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "commits_perc_time_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS Commits Time Share");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_commits_percentage_time = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.commits_perc_time"
                        , title
                        , "percentage"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_COMMITS_PERC_TIME
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                node->rd_commits_percentage_time = rrddim_add(node->st_commits_percentage_time, "commits", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_commits_percentage_time);
            }

            rrddim_set_by_pointer(node->st_commits_percentage_time, node->rd_commits_percentage_time, node->commits_percentage_time);
            rrdset_done(node->st_commits_percentage_time);


            if(unlikely(!node->st_commit_timings)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintfz(id, RRD_ID_LENGTH_MAX, "commit_timings_%s", node->id);
                snprintfz(name, RRD_ID_LENGTH_MAX, "commit_timings_%s", node->label);
                snprintfz(title, sizeof(title) - 1, "BTRFS Commit Timings");

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_commit_timings = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.commit_timings"
                        , title
                        , "ms"
                        , PLUGIN_PROC_NAME
                        , PLUGIN_PROC_MODULE_BTRFS_NAME
                        , NETDATA_CHART_PRIO_BTRFS_COMMIT_TIMINGS
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                node->rd_commit_timings_last = rrddim_add(node->st_commit_timings, "last", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                node->rd_commit_timings_max = rrddim_add(node->st_commit_timings, "max", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                add_labels_to_btrfs(node, node->st_commit_timings);
            }

            rrddim_set_by_pointer(node->st_commit_timings, node->rd_commit_timings_last, node->commit_timings_last);
            rrddim_set_by_pointer(node->st_commit_timings, node->rd_commit_timings_max, node->commit_timings_max);
            rrdset_done(node->st_commit_timings);
        }

        // --------------------------------------------------------------------
        // error_stats per device

        if (do_error_stats == CONFIG_BOOLEAN_YES || do_error_stats == CONFIG_BOOLEAN_AUTO) {
            do_error_stats = CONFIG_BOOLEAN_YES;

            for(BTRFS_DEVICE *d = node->devices ; d ; d = d->next) {

                if(unlikely(!d->st_error_stats)) {
                    char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                    snprintfz(id, RRD_ID_LENGTH_MAX, "device_errors_dev%d_%s", d->id, node->id);
                    snprintfz(name, RRD_ID_LENGTH_MAX, "device_errors_dev%d_%s", d->id, node->label);
                    snprintfz(title, sizeof(title) - 1, "BTRFS Device Errors");

                    netdata_fix_chart_id(id);
                    netdata_fix_chart_name(name);

                    d->st_error_stats = rrdset_create_localhost(
                            "btrfs"
                            , id
                            , name
                            , node->label
                            , "btrfs.device_errors"
                            , title
                            , "errors"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_BTRFS_NAME
                            , NETDATA_CHART_PRIO_BTRFS_ERRORS
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    char rd_id[RRD_ID_LENGTH_MAX + 1];
                    snprintfz(rd_id, RRD_ID_LENGTH_MAX, "write_errs");
                    d->rd_write_errs = rrddim_add(d->st_error_stats, rd_id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    snprintfz(rd_id, RRD_ID_LENGTH_MAX, "read_errs");
                    d->rd_read_errs = rrddim_add(d->st_error_stats, rd_id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    snprintfz(rd_id, RRD_ID_LENGTH_MAX, "flush_errs");
                    d->rd_flush_errs = rrddim_add(d->st_error_stats, rd_id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    snprintfz(rd_id, RRD_ID_LENGTH_MAX, "corruption_errs");
                    d->rd_corruption_errs = rrddim_add(d->st_error_stats, rd_id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    snprintfz(rd_id, RRD_ID_LENGTH_MAX, "generation_errs");
                    d->rd_generation_errs = rrddim_add(d->st_error_stats, rd_id, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                    char dev_id[5];
                    snprintfz(dev_id, 4, "%d", d->id);
                    rrdlabels_add(d->st_error_stats->rrdlabels, "device_id", dev_id, RRDLABEL_SRC_AUTO);
                    add_labels_to_btrfs(node, d->st_error_stats);
                }

                rrddim_set_by_pointer(d->st_error_stats, d->rd_write_errs, d->write_errs);
                rrddim_set_by_pointer(d->st_error_stats, d->rd_read_errs, d->read_errs);
                rrddim_set_by_pointer(d->st_error_stats, d->rd_flush_errs, d->flush_errs);
                rrddim_set_by_pointer(d->st_error_stats, d->rd_corruption_errs, d->corruption_errs);
                rrddim_set_by_pointer(d->st_error_stats, d->rd_generation_errs, d->generation_errs);

                rrdset_done(d->st_error_stats);
            }
        }
    }

    return 0;
}

