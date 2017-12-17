#include "common.h"

#define BTRFS_MAX_ID_LENGTH 50
#define BTRFS_MAX_LABEL_LENGTH 200

typedef struct btrfs_node {
    int exists;

    char id[BTRFS_MAX_ID_LENGTH + 1];
    uint32_t hash;

    char label[BTRFS_MAX_LABEL_LENGTH + 1];

    // unsigned long long int sectorsize;
    // unsigned long long int nodesize;
    // unsigned long long int quota_override;

    #define declare_btrfs_allocation_section_field(SECTION, FIELD) \
        char *allocation_ ## SECTION ## _ ## FIELD ## _filename; \
        unsigned long long int allocation_ ## SECTION ## _ ## FIELD; \
        RRDDIM *rd_allocation_ ## SECTION ## _ ## FIELD;

    #define declare_btrfs_allocation_field(FIELD) \
        char *allocation_ ## FIELD ## _filename; \
        unsigned long long int allocation_ ## FIELD; \
        RRDDIM *rd_allocation_ ## FIELD;

    RRDSET *st_allocation_system;
    declare_btrfs_allocation_section_field(system, total_bytes)
    declare_btrfs_allocation_section_field(system, bytes_used)
    declare_btrfs_allocation_section_field(system, disk_total)
    declare_btrfs_allocation_section_field(system, disk_used)

    RRDSET *st_allocation_data;
    declare_btrfs_allocation_section_field(data, total_bytes)
    declare_btrfs_allocation_section_field(data, bytes_used)
    declare_btrfs_allocation_section_field(data, disk_total)
    declare_btrfs_allocation_section_field(data, disk_used)

    RRDSET *st_allocation_metadata;
    declare_btrfs_allocation_section_field(metadata, total_bytes)
    declare_btrfs_allocation_section_field(metadata, bytes_used)
    declare_btrfs_allocation_section_field(metadata, disk_total)
    declare_btrfs_allocation_section_field(metadata, disk_used)

    RRDSET *st_allocation_global_rsv;
    declare_btrfs_allocation_field(global_rsv_reserved)
    declare_btrfs_allocation_field(global_rsv_size)

    struct btrfs_node *next;
} BTRFS_NODE;

static BTRFS_NODE *nodes = NULL;

static inline void btrfs_free_node(BTRFS_NODE *node) {
    // info("BTRFS: destroying '%s'", node->id);

    if(node->st_allocation_system)
        rrdset_is_obsolete(node->st_allocation_system);

    if(node->st_allocation_data)
        rrdset_is_obsolete(node->st_allocation_data);

    if(node->st_allocation_metadata)
        rrdset_is_obsolete(node->st_allocation_metadata);

    if(node->st_allocation_global_rsv)
        rrdset_is_obsolete(node->st_allocation_global_rsv);

    freez(node->allocation_system_bytes_used_filename);
    freez(node->allocation_system_total_bytes_filename);

    freez(node->allocation_data_bytes_used_filename);
    freez(node->allocation_data_total_bytes_filename);

    freez(node->allocation_metadata_bytes_used_filename);
    freez(node->allocation_metadata_total_bytes_filename);

    freez(node);
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
            break;
        }

        // info("BTRFS: adding '%s'", de->d_name);

        // not found, create it
        node = callocz(sizeof(BTRFS_NODE), 1);

        strncpyz(node->id, de->d_name, BTRFS_MAX_ID_LENGTH);
        node->hash = simple_hash(node->id);
        node->exists = 1;

        snprintfz(filename, FILENAME_MAX, "%s/%s/label", path, de->d_name);
        if(read_file(filename, node->label, sizeof(node->label)) != 0) {
            error("BTRFS: failed to read '%s'", filename);
            btrfs_free_node(node);
            continue;
        }
        if(!node->label[0])
            strncpyz(node->label, node->id, BTRFS_MAX_LABEL_LENGTH);

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


        // --------------------------------------------------------------------
        // allocation/system

        init_btrfs_allocation_section_field(system, total_bytes);
        init_btrfs_allocation_section_field(system, bytes_used);
        init_btrfs_allocation_section_field(system, disk_total);
        init_btrfs_allocation_section_field(system, disk_used);


        // --------------------------------------------------------------------
        // allocation/global_rsv

        init_btrfs_allocation_field(global_rsv_size);
        init_btrfs_allocation_field(global_rsv_reserved);


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
    static int initialized = 0, do_allocation_system = 1, do_allocation_data = 1, do_allocation_metadata = 1, do_allocation_global_rsv = 1;
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

        do_allocation_data = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "data allocation", CONFIG_BOOLEAN_AUTO);
        do_allocation_metadata = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "metadata allocation", CONFIG_BOOLEAN_AUTO);
        do_allocation_system = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "system allocation", CONFIG_BOOLEAN_AUTO);
        do_allocation_global_rsv = config_get_boolean_ondemand("plugin:proc:/sys/fs/btrfs", "global reserve", CONFIG_BOOLEAN_AUTO);
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

        if(do_allocation_data != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(data, total_bytes) != 0
                || collect_btrfs_allocation_section_field(data, bytes_used) != 0
                || collect_btrfs_allocation_section_field(data, disk_total) != 0
                || collect_btrfs_allocation_section_field(data, disk_used) != 0) {
                error("BTRFS: failed to collect allocation/data for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_metadata != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(metadata, total_bytes) != 0
                || collect_btrfs_allocation_section_field(metadata, bytes_used) != 0
                || collect_btrfs_allocation_section_field(metadata, disk_total) != 0
                || collect_btrfs_allocation_section_field(metadata, disk_used) != 0) {
                error("BTRFS: failed to collect allocation/metadata for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_system != CONFIG_BOOLEAN_NO) {
            if (collect_btrfs_allocation_section_field(system, total_bytes) != 0
                || collect_btrfs_allocation_section_field(system, bytes_used) != 0
                || collect_btrfs_allocation_section_field(system, disk_total) != 0
                || collect_btrfs_allocation_section_field(system, disk_used) != 0) {
                error("BTRFS: failed to collect allocation/system for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        if(do_allocation_global_rsv != CONFIG_BOOLEAN_NO) {
            if(collect_btrfs_allocation_field(global_rsv_reserved) != 0
               || collect_btrfs_allocation_field(global_rsv_size) != 0) {
                error("BTRFS: failed to collect global reserve for '%s'", node->id);
                // make it refresh btrfs at the next iteration
                refresh_delta = refresh_every;
                continue;
            }
        }

        // --------------------------------------------------------------------
        // allocation/data

        if(do_allocation_data == CONFIG_BOOLEAN_YES || (do_allocation_data == CONFIG_BOOLEAN_AUTO && (node->allocation_data_total_bytes || node->allocation_data_bytes_used))) {
            do_allocation_data = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_data)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "data_bytes_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "data_bytes_%s", node->label);
                snprintf(title, 200, "BTRFS Data Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_data = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.system_bytes"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2300
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                node->rd_allocation_data_total_bytes = rrddim_add(node->st_allocation_data, "total", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_data_bytes_used = rrddim_add(node->st_allocation_data, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_data_disk_total = rrddim_add(node->st_allocation_data, "disk_total", "disk total", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_data_disk_used = rrddim_add(node->st_allocation_data, "disk_used", "disk used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_data);

            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_total_bytes, node->allocation_data_total_bytes);
            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_bytes_used, node->allocation_data_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_disk_total, node->allocation_data_disk_total);
            rrddim_set_by_pointer(node->st_allocation_data, node->rd_allocation_data_disk_used, node->allocation_data_disk_used);
            rrdset_done(node->st_allocation_data);
        }

        // --------------------------------------------------------------------
        // allocation/system

        if(do_allocation_system == CONFIG_BOOLEAN_YES || (do_allocation_system == CONFIG_BOOLEAN_AUTO && (node->allocation_system_total_bytes || node->allocation_system_bytes_used))) {
            do_allocation_system = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_system)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "system_bytes_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "system_bytes_%s", node->label);
                snprintf(title, 200, "BTRFS System Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_system = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.system_bytes"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2301
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                node->rd_allocation_system_total_bytes = rrddim_add(node->st_allocation_system, "total", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_system_bytes_used = rrddim_add(node->st_allocation_system, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_system_disk_total = rrddim_add(node->st_allocation_system, "disk_total", "disk total", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_system_disk_used = rrddim_add(node->st_allocation_system, "disk_used", "disk used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_system);

            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_total_bytes, node->allocation_system_total_bytes);
            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_bytes_used, node->allocation_system_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_disk_total, node->allocation_system_disk_total);
            rrddim_set_by_pointer(node->st_allocation_system, node->rd_allocation_system_disk_used, node->allocation_system_disk_used);
            rrdset_done(node->st_allocation_system);
        }

        // --------------------------------------------------------------------
        // allocation/metadata

        if(do_allocation_metadata == CONFIG_BOOLEAN_YES || (do_allocation_metadata == CONFIG_BOOLEAN_AUTO && (node->allocation_metadata_total_bytes || node->allocation_metadata_bytes_used))) {
            do_allocation_metadata = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_metadata)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "metadata_bytes_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "metadata_bytes_%s", node->label);
                snprintf(title, 200, "BTRFS Metadata Allocation for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_metadata = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.system_bytes"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2302
                        , update_every
                        , RRDSET_TYPE_LINE
                );

                node->rd_allocation_metadata_total_bytes = rrddim_add(node->st_allocation_metadata, "total", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_bytes_used = rrddim_add(node->st_allocation_metadata, "used", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_disk_total = rrddim_add(node->st_allocation_metadata, "disk_total", "disk total", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_metadata_disk_used = rrddim_add(node->st_allocation_metadata, "disk_used", "disk used", 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_metadata);

            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_total_bytes, node->allocation_metadata_total_bytes);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_bytes_used, node->allocation_metadata_bytes_used);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_disk_total, node->allocation_metadata_disk_total);
            rrddim_set_by_pointer(node->st_allocation_metadata, node->rd_allocation_metadata_disk_used, node->allocation_metadata_disk_used);
            rrdset_done(node->st_allocation_metadata);
        }


        // --------------------------------------------------------------------
        // allocation/global_rsv

        if(do_allocation_global_rsv == CONFIG_BOOLEAN_YES || (do_allocation_global_rsv == CONFIG_BOOLEAN_AUTO && (node->allocation_global_rsv_size || node->allocation_global_rsv_reserved))) {
            do_allocation_global_rsv = CONFIG_BOOLEAN_YES;

            if(unlikely(!node->st_allocation_global_rsv)) {
                char id[RRD_ID_LENGTH_MAX + 1], name[RRD_ID_LENGTH_MAX + 1], title[200 + 1];

                snprintf(id, RRD_ID_LENGTH_MAX, "global_reserve_%s", node->id);
                snprintf(name, RRD_ID_LENGTH_MAX, "global_reserve_%s", node->label);
                snprintf(title, 200, "BTRFS Global Reserve for %s", node->label);

                netdata_fix_chart_id(id);
                netdata_fix_chart_name(name);

                node->st_allocation_global_rsv = rrdset_create_localhost(
                        "btrfs"
                        , id
                        , name
                        , node->label
                        , "btrfs.system_bytes"
                        , title
                        , "MB"
                        , "proc"
                        , "sys/fs/btrfs"
                        , 2303
                        , update_every
                        , RRDSET_TYPE_LINE
                );
                node->rd_allocation_global_rsv_size = rrddim_add(node->st_allocation_global_rsv, "size", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
                node->rd_allocation_global_rsv_reserved = rrddim_add(node->st_allocation_global_rsv, "reserved", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            }
            else rrdset_next(node->st_allocation_global_rsv);

            rrddim_set_by_pointer(node->st_allocation_global_rsv, node->rd_allocation_global_rsv_size, node->allocation_global_rsv_size);
            rrddim_set_by_pointer(node->st_allocation_global_rsv, node->rd_allocation_global_rsv_reserved, node->allocation_global_rsv_reserved);
            rrdset_done(node->st_allocation_global_rsv);
        }
    }

    return 0;
}

