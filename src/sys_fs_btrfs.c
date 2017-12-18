#include "common.h"

typedef struct btrfs_disk {
    char *name;
    uint32_t hash;
    int exists;

    char *size_filename;
    char *hw_sector_size_filename;
    unsigned long long size;
    unsigned long long hw_sector_size;

    struct btrfs_disk *next;
} BTRFS_DISK;

typedef struct btrfs_node {
    int exists;
    int logged_error;

    char *id;
    uint32_t hash;

    char *label;

    // unsigned long long int sectorsize;
    // unsigned long long int nodesize;
    // unsigned long long int quota_override;

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

    BTRFS_DISK *disks;

    struct btrfs_node *next;
} BTRFS_NODE;

static BTRFS_NODE *nodes = NULL;

static inline void btrfs_free_disk(BTRFS_DISK *d) {
    freez(d->name);
    freez(d->size_filename);
    freez(d->hw_sector_size_filename);
    freez(d);
}

static inline void btrfs_free_node(BTRFS_NODE *node) {
    // info("BTRFS: destroying '%s'", node->id);

    if(node->st_allocation_disks)
        rrdset_is_obsolete(node->st_allocation_disks);

    if(node->st_allocation_data)
        rrdset_is_obsolete(node->st_allocation_data);

    if(node->st_allocation_metadata)
        rrdset_is_obsolete(node->st_allocation_metadata);

    if(node->st_allocation_system)
        rrdset_is_obsolete(node->st_allocation_system);

    freez(node->allocation_data_bytes_used_filename);
    freez(node->allocation_data_total_bytes_filename);

    freez(node->allocation_metadata_bytes_used_filename);
    freez(node->allocation_metadata_total_bytes_filename);

    freez(node->allocation_system_bytes_used_filename);
    freez(node->allocation_system_total_bytes_filename);

    while(node->disks) {
        BTRFS_DISK *d = node->disks;
        node->disks = node->disks->next;
        btrfs_free_disk(d);
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
        if(!node->logged_error) {
            error("BTRFS: Cannot open directory '%s'.", path);
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
            // info("BTRFS: ignoring '%s'", de->d_name);
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

            // for disks
            snprintfz(filename, FILENAME_MAX, "%s/%s/queue/hw_sector_size", path, de->d_name);
            struct stat sb;
            if(stat(filename, &sb) == -1)
                // for partitions
                snprintfz(filename, FILENAME_MAX, "%s/%s/../queue/hw_sector_size", path, de->d_name);

            d->hw_sector_size_filename = strdupz(filename);

            // link it
            d->next = node->disks;
            node->disks = d;
        }

        d->exists = 1;


        // --------------------------------------------------------------------
        // update the values

        if(read_single_number_file(d->size_filename, &d->size) != 0) {
            error("BTRFS: failed to read '%s'", d->size_filename);
            d->exists = 0;
            continue;
        }

        if(read_single_number_file(d->hw_sector_size_filename, &d->hw_sector_size) != 0) {
            error("BTRFS: failed to read '%s'", d->hw_sector_size_filename);
            d->exists = 0;
            continue;
        }

        node->all_disks_total += d->size * d->hw_sector_size;
    }

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


static inline int find_all_btrfs_pools(const char *path) {
    static int logged_error = 0;
    char filename[FILENAME_MAX + 1];

    BTRFS_NODE *node;
    for(node = nodes ; node ; node = node->next)
        node->exists = 0;

    DIR *dir = opendir(path);
    if (!dir) {
        if(!logged_error) {
            error("BTRFS: Cannot open directory '%s'.", path);
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
            // info("BTRFS: ignoring '%s'", de->d_name);
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
            // info("BTRFS: already exists '%s'", de->d_name);
            node->exists = 1;

            // update the disk sizes
            snprintfz(filename, FILENAME_MAX, "%s/%s/devices", path, de->d_name);
            find_btrfs_disks(node, filename);

            continue;
        }

        // info("BTRFS: adding '%s'", de->d_name);

        // not found, create it
        node = callocz(sizeof(BTRFS_NODE), 1);

        node->id = strdupz(de->d_name);
        node->hash = simple_hash(node->id);
        node->exists = 1;

        {
            char label[FILENAME_MAX + 1] = "";

            snprintfz(filename, FILENAME_MAX, "%s/%s/label", path, de->d_name);
            read_file(filename, label, FILENAME_MAX);

            char *s = label;
            if (s[0])
                s = trim(label);

            if(s && s[0])
                node->label = strdupz(s);
            else
                node->label = strdupz(node->id);
        }

        //snprintfz(filename, FILENAME_MAX, "%s/%s/sectorsize", path, de->d_name);
        //if(read_single_number_file(filename, &node->sectorsize) != 0) {
        //    error("BTRFS: failed to read '%s'", filename);
        //    btrfs_free_node(node);
        //    continue;
        //}

        //snprintfz(filename, FILENAME_MAX, "%s/%s/nodesize", path, de->d_name);
        //if(read_single_number_file(filename, &node->nodesize) != 0) {
        //    error("BTRFS: failed to read '%s'", filename);
        //    btrfs_free_node(node);
        //    continue;
        //}

        //snprintfz(filename, FILENAME_MAX, "%s/%s/quota_override", path, de->d_name);
        //if(read_single_number_file(filename, &node->quota_override) != 0) {
        //    error("BTRFS: failed to read '%s'", filename);
        //    btrfs_free_node(node);
        //    continue;
        //}

        // --------------------------------------------------------------------
        // macros to simplify our life

        #define init_btrfs_allocation_field(FIELD) {\
            snprintfz(filename, FILENAME_MAX, "%s/%s/allocation/" #FIELD, path, de->d_name); \
            if(read_single_number_file(filename, &node->allocation_ ## FIELD) != 0) {\
                error("BTRFS: failed to read '%s'", filename);\
                btrfs_free_node(node);\
                continue;\
            }\
            if(!node->allocation_ ## FIELD ## _filename)\
                node->allocation_ ## FIELD ## _filename = strdupz(filename);\
        }

        #define init_btrfs_allocation_section_field(SECTION, FIELD) {\
            snprintfz(filename, FILENAME_MAX, "%s/%s/allocation/" #SECTION "/" #FIELD, path, de->d_name); \
            if(read_single_number_file(filename, &node->allocation_ ## SECTION ## _ ## FIELD) != 0) {\
                error("BTRFS: failed to read '%s'", filename);\
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
        // find all disks related to this node
        // and collect their sizes

        snprintfz(filename, FILENAME_MAX, "%s/%s/devices", path, de->d_name);
        find_btrfs_disks(node, filename);


        // --------------------------------------------------------------------
        // link it

        // info("BTRFS: linking '%s'", node->id);
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

int do_sys_fs_btrfs(int update_every, usec_t dt) {
    static int initialized = 0
        , do_allocation_disks = CONFIG_BOOLEAN_AUTO
        , do_allocation_system = CONFIG_BOOLEAN_AUTO
        , do_allocation_data = CONFIG_BOOLEAN_AUTO
        , do_allocation_metadata = CONFIG_BOOLEAN_AUTO;

    static usec_t refresh_delta = 0, refresh_every = 60 * USEC_PER_SEC;
    static char *btrfs_path = NULL;

    (void)dt;

    if(unlikely(!initialized)) {
        initialized = 1;

        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/fs/btrfs");
        btrfs_path = config_get("plugin:proc:/sys/fs/btrfs", "path to monitor", filename);

        refresh_every = config_get_number("plugin:proc:/sys/fs/btrfs", "check for btrfs changes every", refresh_every / USEC_PER_SEC) * USEC_PER_SEC;
        refresh_delta = refresh_every;

        do_allocation_disks = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "physical disks allocation", do_allocation_disks);
        do_allocation_data = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "data allocation", do_allocation_data);
        do_allocation_metadata = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "metadata allocation", do_allocation_metadata);
        do_allocation_system = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "system allocation", do_allocation_system);
    }

    refresh_delta += dt;
    if(refresh_delta >= refresh_every) {
        refresh_delta = 0;
        find_all_btrfs_pools(btrfs_path);
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
                error("BTRFS: failed to collect physical disks allocation for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_data != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(data, total_bytes) != 0
                || collect_btrfs_allocation_section_field(data, bytes_used) != 0) {
                error("BTRFS: failed to collect allocation/data for '%s'", node->id);
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
                error("BTRFS: failed to collect allocation/metadata for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_system != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(system, total_bytes) != 0
                || collect_btrfs_allocation_section_field(system, bytes_used) != 0) {
                error("BTRFS: failed to collect allocation/system for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        // --------------------------------------------------------------------
        // allocation/disks

        if(do_allocation_disks == CONFIG_BOOLEAN_YES || (do_allocation_disks == CONFIG_BOOLEAN_AUTO && node->all_disks_total && node->allocation_data_disk_total)) {
            do_allocation_disks = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_disks)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "disk_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "disk_%s", node->label);
                snprintf(title, 200, "BTRFS Disk Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_disks = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.disk"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2300
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_disks_unallocated = rrddim_add(node->st_allocation_disks, "unallocated", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_data_used = rrddim_add(node->st_allocation_disks, "data_used", "data used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_data_free = rrddim_add(node->st_allocation_disks, "data_free", "data free", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_metadata_used = rrddim_add(node->st_allocation_disks, "meta_used", "meta used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_metadata_free = rrddim_add(node->st_allocation_disks, "meta_free", "meta free", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_system_used = rrddim_add(node->st_allocation_disks, "sys_used", "sys used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_disks_system_free = rrddim_add(node->st_allocation_disks, "sys_free", "sys free", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_disks);

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

        if(do_allocation_data == CONFIG_BOOLEAN_YES || (do_allocation_data == CONFIG_BOOLEAN_AUTO && node->allocation_data_total_bytes)) {
            do_allocation_data = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_data)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "data_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "data_%s", node->label);
                snprintf(title, 200, "BTRFS Data Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_data = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.data"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2301
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_data_free = rrddim_add(node->st_allocation_data, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_data_used = rrddim_add(node->st_allocation_data, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_data);

            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_free, node->allocation_data_total_bytes - node->allocation_data_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_used, node->allocation_data_bytes_used);
            rrdset_done(node->st_allocation_data);
        }

        // --------------------------------------------------------------------
        // allocation/metadata

        if(do_allocation_metadata == CONFIG_BOOLEAN_YES || (do_allocation_metadata == CONFIG_BOOLEAN_AUTO && node->allocation_metadata_total_bytes)) {
            do_allocation_metadata = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_metadata)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "metadata_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "metadata_%s", node->label);
                snprintf(title, 200, "BTRFS Metadata Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_metadata = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.metadata"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2302
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_metadata_free = rrddim_add(node->st_allocation_metadata, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_used = rrddim_add(node->st_allocation_metadata, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_reserved = rrddim_add(node->st_allocation_metadata, "reserved", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_metadata);

            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_free, node->allocation_metadata_total_bytes - node->allocation_metadata_bytes_used - node->allocation_global_rsv_size);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_used, node->allocation_metadata_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_reserved, node->allocation_global_rsv_size);
            rrdset_done(node->st_allocation_metadata);
        }

        // --------------------------------------------------------------------
        // allocation/system

        if(do_allocation_system == CONFIG_BOOLEAN_YES || (do_allocation_system == CONFIG_BOOLEAN_AUTO && node->allocation_system_total_bytes)) {
            do_allocation_system = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_system)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "system_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "system_%s", node->label);
                snprintf(title, 200, "BTRFS System Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_system = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.system"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2303
                        , update_every
                        , RRDSET_TYPE_STACKED
                );

                node->rd_allocation_system_free = rrddim_add(node->st_allocation_system, "free", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_system_used = rrddim_add(node->st_allocation_system, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_system);

            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_free, node->allocation_system_total_bytes - node->allocation_system_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_used, node->allocation_system_bytes_used);
            rrdset_done(node->st_allocation_system);
        }
    }

    return 0;
}

