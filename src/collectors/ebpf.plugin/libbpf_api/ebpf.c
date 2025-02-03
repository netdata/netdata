// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <dlfcn.h>
#include <sys/utsname.h>

#include "ebpf.h"
#include "libnetdata/libnetdata.h"

char *ebpf_user_config_dir = CONFIG_DIR;
char *ebpf_stock_config_dir = LIBCONFIG_DIR;

/*
static int clean_kprobe_event(FILE *out, char *filename, char *father_pid, netdata_ebpf_events_t *ptr)
{
    int fd = open(filename, O_WRONLY | O_APPEND, 0);
    if (fd < 0) {
        if (out) {
            fprintf(out, "Cannot open %s : %s\n", filename, strerror(errno));
        }
        return 1;
    }

    char cmd[1024];
    int length = snprintf(cmd, 1023, "-:kprobes/%c_netdata_%s_%s", ptr->type, ptr->name, father_pid);
    int ret = 0;
    if (length > 0) {
        ssize_t written = write(fd, cmd, strlen(cmd));
        if (written < 0) {
            if (out) {
                fprintf(
                    out, "Cannot remove the event (%d, %d) '%s' from %s : %s\n", getppid(), getpid(), cmd, filename,
                    strerror((int)errno));
            }
            ret = 1;
        }
    }

    close(fd);

    return ret;
}

int clean_kprobe_events(FILE *out, int pid, netdata_ebpf_events_t *ptr)
{
    char filename[FILENAME_MAX + 1];
    snprintf(filename, FILENAME_MAX, "%s%s", NETDATA_DEBUGFS, "kprobe_events");

    char removeme[16];
    snprintf(removeme, 15, "%d", pid);

    int i;
    for (i = 0; ptr[i].name; i++) {
        if (clean_kprobe_event(out, filename, removeme, &ptr[i])) {
            break;
        }
    }

    return 0;
}
*/

//----------------------------------------------------------------------------------------------------------------------

/**
 * Get Kernel version
 *
 * Get the current kernel from /proc and returns an integer value representing it
 *
 * @return it returns a value representing the kernel version.
 */
int ebpf_get_kernel_version()
{
    char major[16], minor[16], patch[16];
    char ver[VERSION_STRING_LEN];
    char *version = ver;

    int fd = open("/proc/sys/kernel/osrelease", O_RDONLY | O_CLOEXEC);
    if (fd < 0)
        return -1;

    ssize_t len = read(fd, ver, sizeof(ver));
    if (len < 0) {
        close(fd);
        return -1;
    }

    close(fd);

    char *move = major;
    while (*version && *version != '.')
        *move++ = *version++;
    *move = '\0';

    version++;
    move = minor;
    while (*version && *version != '.')
        *move++ = *version++;
    *move = '\0';

    if (*version)
        version++;
    else
        return -1;

    move = patch;
    while (*version && *version != '\n' && *version != '-')
        *move++ = *version++;
    *move = '\0';

    // This new rule is fixing kernel version according the formula:
    //     KERNEL_VERSION(a,b,c) (((a) << 16) + ((b) << 8) + ((c) > 255 ? 255 : (c)))
    // that was extracted from /usr/include/linux/version.h
    int ipatch = (int)str2l(patch);
    if (ipatch > 255)
        ipatch = 255;

    return ((int)(str2l(major) * 65536) + (int)(str2l(minor) * 256) + ipatch);
}

/**
 * Get RH release
 *
 * Read Red Hat release from /etc/redhat-release
 *
 * @return It returns RH release on success and -1 otherwise
 */
int get_redhat_release()
{
    char buffer[VERSION_STRING_LEN + 1];
    int major, minor;
    FILE *fp = fopen("/etc/redhat-release", "r");

    if (fp) {
        major = 0;
        minor = -1;
        size_t length = fread(buffer, sizeof(char), VERSION_STRING_LEN, fp);
        if (length > 4) {
            buffer[length] = '\0';
            char *end = strchr(buffer, '.');
            char *start;
            if (end) {
                *end = 0x0;

                if (end > buffer) {
                    start = end - 1;

                    major = strtol(start, NULL, 10);
                    start = ++end;

                    end++;
                    if (end) {
                        end = 0x00;
                        minor = strtol(start, NULL, 10);
                    } else {
                        minor = -1;
                    }
                }
            }
        }

        fclose(fp);
        return ((major * 256) + minor);
    } else {
        return -1;
    }
}

/**
 * Check if the kernel is in a list of rejected ones
 *
 * @return Returns 1 if the kernel is rejected, 0 otherwise.
 */
static int kernel_is_rejected()
{
    // Get kernel version from system
    char version_string[VERSION_STRING_LEN + 1];
    int version_string_len = 0;

    if (read_txt_file("/proc/version_signature", version_string, sizeof(version_string))) {
        if (read_txt_file("/proc/version", version_string, sizeof(version_string))) {
            struct utsname uname_buf;
            if (!uname(&uname_buf)) {
                netdata_log_info("Cannot check kernel version");
                return 0;
            }
            version_string_len =
                snprintfz(version_string, VERSION_STRING_LEN, "%s %s", uname_buf.release, uname_buf.version);
        }
    }

    if (!version_string_len)
        version_string_len = strlen(version_string);

    // Open a file with a list of rejected kernels
    char *config_dir = getenv("NETDATA_USER_CONFIG_DIR");
    if (config_dir == NULL) {
        config_dir = CONFIG_DIR;
    }

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/ebpf.d/%s", config_dir, EBPF_KERNEL_REJECT_LIST_FILE);
    FILE *kernel_reject_list = fopen(filename, "r");

    if (!kernel_reject_list) {
        // Keep this to have compatibility with old versions
        snprintfz(filename, FILENAME_MAX, "%s/%s", config_dir, EBPF_KERNEL_REJECT_LIST_FILE);
        kernel_reject_list = fopen(filename, "r");

        if (!kernel_reject_list) {
            config_dir = getenv("NETDATA_STOCK_CONFIG_DIR");
            if (config_dir == NULL) {
                config_dir = LIBCONFIG_DIR;
            }

            snprintfz(filename, FILENAME_MAX, "%s/ebpf.d/%s", config_dir, EBPF_KERNEL_REJECT_LIST_FILE);
            kernel_reject_list = fopen(filename, "r");

            if (!kernel_reject_list)
                return 0;
        }
    }

    // Find if the kernel is in the reject list
    char *reject_string = NULL;
    size_t buf_len = 0;
    ssize_t reject_string_len;
    while ((reject_string_len = getline(&reject_string, &buf_len, kernel_reject_list) - 1) > 0) {
        if (version_string_len >= reject_string_len) {
            if (!strncmp(version_string, reject_string, reject_string_len)) {
                netdata_log_info("A buggy kernel is detected");
                fclose(kernel_reject_list);
                freez(reject_string);
                return 1;
            }
        }
    }

    fclose(kernel_reject_list);
    free(reject_string);

    return 0;
}

/**
 * Check Kernel Version
 *
 * Test kernel version
 *
 * @param version current kernel version
 *
 * @return It returns 1 when kernel is supported and 0 otherwise
 */
int ebpf_check_kernel_version(int version)
{
    if (kernel_is_rejected())
        return 0;

    // Kernel 4.11.0 or RH > 7.5
    return (version >= NETDATA_MINIMUM_EBPF_KERNEL || get_redhat_release() >= NETDATA_MINIMUM_RH_VERSION);
}

/**
 * Am I running as Root
 *
 * Verify the user that is running the collector.
 *
 * @return It returns 1 for root and 0 otherwise.
 */
int is_ebpf_plugin_running_as_root()
{
    uid_t uid = getuid(), euid = geteuid();

    if (uid == 0 || euid == 0) {
        return 1;
    }

    return 0;
}

/**
 * Can the plugin run eBPF code
 *
 * This function checks kernel version and permissions.
 *
 * @param kver  the kernel version
 * @param name  the plugin name.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_can_plugin_load_code(int kver, char *plugin_name)
{
    if (!ebpf_check_kernel_version(kver)) {
        netdata_log_error("The current collector cannot run on this kernel.");
        return -1;
    }

    if (!is_ebpf_plugin_running_as_root()) {
        netdata_log_error(
            "%s should either run as root (now running with uid %u, euid %u) or have special capabilities.",
            plugin_name, (unsigned int)getuid(), (unsigned int)geteuid());
        return -1;
    }

    return 0;
}

/**
 * Adjust memory
 *
 * Adjust memory values to load eBPF programs.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_adjust_memory_limit()
{
    struct rlimit r = { RLIM_INFINITY, RLIM_INFINITY };
    if (setrlimit(RLIMIT_MEMLOCK, &r)) {
        netdata_log_error("Setrlimit(RLIMIT_MEMLOCK)");
        return -1;
    }

    return 0;
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Kernel Name
 *
 * Select kernel name used by eBPF programs
 *
 * Netdata delivers for users eBPF programs with specific suffixes that represent the kernels they were
 * compiled, when we load the eBPF program, the suffix must be the nereast possible of the kernel running.
 *
 * @param selector select the kernel version.
 *
 * @return It returns the string to load kernel.
 */
static char *ebpf_select_kernel_name(uint32_t selector)
{
    static char *kernel_names[] = { NETDATA_IDX_STR_V3_10, NETDATA_IDX_STR_V4_14, NETDATA_IDX_STR_V4_16,
                                    NETDATA_IDX_STR_V4_18, NETDATA_IDX_STR_V5_4,  NETDATA_IDX_STR_V5_10,
                                    NETDATA_IDX_STR_V5_11, NETDATA_IDX_STR_V5_14, NETDATA_IDX_STR_V5_15,
                                    NETDATA_IDX_STR_V5_16, NETDATA_IDX_STR_V6_8
                                  };

    return kernel_names[selector];
}

/**
 * Select Max Index
 *
 * Select last index that will be tested on host.
 *
 * @param is_rhf is Red Hat fammily?
 * @param kver   the kernel version
 *
 * @return it returns the index to access kernel string.
 */
static int ebpf_select_max_index(int is_rhf, uint32_t kver)
{
    if (is_rhf > 0) { // Is Red Hat family
        if (kver >= NETDATA_EBPF_KERNEL_5_14)
            return NETDATA_IDX_V5_14;
        else if (kver >= NETDATA_EBPF_KERNEL_5_4 && kver < NETDATA_EBPF_KERNEL_5_5) // For Oracle Linux
            return NETDATA_IDX_V5_4;
        else if (kver >= NETDATA_EBPF_KERNEL_4_11)
            return NETDATA_IDX_V4_18;
    } else { // Kernels from kernel.org
        if (kver >= NETDATA_EBPF_KERNEL_6_8)
            return NETDATA_IDX_V6_8;
	else if (kver >= NETDATA_EBPF_KERNEL_5_16)
            return NETDATA_IDX_V5_16;
        else if (kver >= NETDATA_EBPF_KERNEL_5_15)
            return NETDATA_IDX_V5_15;
        else if (kver >= NETDATA_EBPF_KERNEL_5_11)
            return NETDATA_IDX_V5_11;
        else if (kver >= NETDATA_EBPF_KERNEL_5_10)
            return NETDATA_IDX_V5_10;
        else if (kver >= NETDATA_EBPF_KERNEL_4_17)
            return NETDATA_IDX_V5_4;
        else if (kver >= NETDATA_EBPF_KERNEL_4_15)
            return NETDATA_IDX_V4_16;
        else if (kver >= NETDATA_EBPF_KERNEL_4_11)
            return NETDATA_IDX_V4_14;
    }

    return NETDATA_IDX_V3_10;
}

/**
 * Select Index
 *
 * Select index to load data.
 *
 * @param kernels is the variable with kernel versions.
 * @param is_rhf  is Red Hat fammily?
 * param  kver    the kernel version
 */
static uint32_t ebpf_select_index(uint32_t kernels, int is_rhf, uint32_t kver)
{
    uint32_t start = ebpf_select_max_index(is_rhf, kver);
    uint32_t idx;

    if (is_rhf == -1)
        kernels &= ~NETDATA_V5_14;

    for (idx = start; idx; idx--) {
        if (kernels & 1 << idx)
            break;
    }

    return idx;
}

/**
 *  Mount Name
 *
 *  Mount name of eBPF program to be loaded.
 *
 *  Netdata eBPF programs has the following format:
 *
 *      Tnetdata_ebpf_N.V.o
 *
 *  where:
 *     T - Is the eBPF type. When starts with 'p', this means we are only adding probes,
 *         and when they start with 'r' we are using retprobes.
 *     N - The eBPF program name.
 *     V - The kernel version in string format.
 *
 *  @param out       the vector where the name will be stored
 *  @param len       the size of the out vector.
 *  @param path      where the binaries are stored
 *  @param kver      the kernel version
 *  @param name      the eBPF program name.
 *  @param is_return is return or entry ?
 */
static void ebpf_mount_name(char *out, size_t len, char *path, uint32_t kver, const char *name,
                            int is_return, int is_rhf)
{
    char *version = ebpf_select_kernel_name(kver);
    snprintfz(out, len, "%s/ebpf.d/%cnetdata_ebpf_%s.%s%s.o",
              path,
              (is_return) ? 'r' : 'p',
              name,
              version,
              (is_rhf != -1) ? ".rhf" : "");
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Statistics from targets
 *
 * Count the information from targets.
 *
 * @param report  the output structure
 * @param targets vector with information about the eBPF plugin.
 * @param value    factor used to update calculation
 */
static void ebpf_stats_targets(ebpf_plugin_stats_t *report, netdata_ebpf_targets_t *targets, int value)
{
    if (!targets) {
        report->probes = report->tracepoints = report->trampolines = 0;
        return;
    }

    int i = 0;
    while (targets[i].name) {
        switch (targets[i].mode) {
            case EBPF_LOAD_PROBE: {
                report->probes += value;
                break;
            }
            case EBPF_LOAD_RETPROBE: {
                report->retprobes += value;
                break;
            }
            case EBPF_LOAD_TRACEPOINT: {
                report->tracepoints += value;
                break;
            }
            case EBPF_LOAD_TRAMPOLINE: {
                report->trampolines += value;
                break;
            }
        }

        i++;
    }
}

/**
 * Update General stats
 *
 * Update eBPF plugin statistics that has relationship with the thread.
 *
 * This function must be called with mutex associated to charts is locked.
 *
 * @param report  the output structure
 * @param em      the structure with information about how the module/thread is working.
 */
void ebpf_update_stats(ebpf_plugin_stats_t *report, ebpf_module_t *em)
{
    int value;

    // It is not necessary to report more information.
    if (em->enabled > NETDATA_THREAD_EBPF_FUNCTION_RUNNING)
        value = -1;
    else
        value = 1;

    report->threads += value;
    report->running += value;

    // In theory the `else if` is useless, because when this function is called, the module should not stay in
    // EBPF_LOAD_PLAY_DICE. We have this additional condition to detect errors from developers.
    if (em->load & EBPF_LOAD_LEGACY)
        report->legacy += value;
    else if (em->load & EBPF_LOAD_CORE)
        report->core += value;

    if (em->maps_per_core)
        report->hash_percpu += value;
    else
        report->hash_unique += value;

    ebpf_stats_targets(report, em->targets, value);
}

/**
 * Update Kernel memory with memory
 *
 * This algorithm is an adaptation of https://elixir.bootlin.com/linux/v6.1.14/source/tools/bpf/bpftool/common.c#L402
 * to get 'memlock' data and update report.
 *
 * @param report  the output structure
 * @param map     pointer to a map.
 * @param action  What action will be done with this map.
 */
void ebpf_update_kernel_memory(ebpf_plugin_stats_t *report, ebpf_local_maps_t *map, ebpf_stats_action_t action)
{
    char filename[FILENAME_MAX+1];
    snprintfz(filename, FILENAME_MAX, "/proc/self/fdinfo/%d", map->map_fd);
    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        netdata_log_error("Cannot open %s", filename);
        return;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return;

    unsigned long j, lines = procfile_lines(ff);
    char *memlock = { "memlock" };
    for (j = 0; j < lines ; j++) {
        char *cmp = procfile_lineword(ff, j,0);
        if (!strncmp(memlock, cmp, 7)) {
            uint64_t memsize = (uint64_t) str2l(procfile_lineword(ff, j,1));
            switch (action) {
                case EBPF_ACTION_STAT_ADD: {
                    report->memlock_kern += memsize;
                    report->hash_tables += 1;
#ifdef NETDATA_DEV_MODE
                    netdata_log_info("Hash table %u: %s (FD = %d) is consuming %lu bytes totalizing %lu bytes",
                         report->hash_tables, map->name, map->map_fd, memsize, report->memlock_kern);
#endif
                    break;
                }
                case EBPF_ACTION_STAT_REMOVE: {
                    report->memlock_kern -= memsize;
                    report->hash_tables -= 1;
#ifdef NETDATA_DEV_MODE
                    netdata_log_info("Hash table %s (FD = %d) was removed releasing %lu bytes, now we have %u tables loaded totalizing %lu bytes.",
                         map->name, map->map_fd, memsize, report->hash_tables, report->memlock_kern);
#endif
                    break;
                }
                default: {
                    break;
                }
            }
            break;
        }
    }

    procfile_close(ff);
}

/**
 * Update Kernel memory with memory
 *
 * This algorithm is an adaptation of https://elixir.bootlin.com/linux/v6.1.14/source/tools/bpf/bpftool/common.c#L402
 * to get 'memlock' data and update report.
 *
 * @param report  the output structure
 * @param map     pointer to a map. Last map must fish with name = NULL
 * @param action  should plugin add or remove values from amount.
 */
void ebpf_update_kernel_memory_with_vector(ebpf_plugin_stats_t *report,
                                           ebpf_local_maps_t *maps,
                                           ebpf_stats_action_t action)
{
    if (!maps)
        return;

    ebpf_local_maps_t *map;
    int i = 0;
    for (map = &maps[i]; maps[i].name; i++, map = &maps[i]) {
        int fd = map->map_fd;
        if (fd == ND_EBPF_MAP_FD_NOT_INITIALIZED)
            continue;

        ebpf_update_kernel_memory(report, map, action);
    }
}

//----------------------------------------------------------------------------------------------------------------------

void ebpf_update_pid_table(ebpf_local_maps_t *pid, ebpf_module_t *em)
{
    pid->user_input = em->pid_map_size;
}

/**
 * Update map size
 *
 * Update map size with information read from configuration files.
 *
 * @param map       the structure with file descriptor to update.
 * @param lmap      the structure with information from configuration files.
 * @param em        the structure with information about how the module/thread is working.
 * @param map_name  the name of the file used to log.
 */
void ebpf_update_map_size(struct bpf_map *map, ebpf_local_maps_t *lmap, ebpf_module_t *em, const char *map_name __maybe_unused)
{
    uint32_t define_size = 0;
    uint32_t apps_type = NETDATA_EBPF_MAP_PID | NETDATA_EBPF_MAP_RESIZABLE;
    if (lmap->user_input && lmap->user_input != lmap->internal_input) {
        define_size = lmap->internal_input;
#ifdef NETDATA_INTERNAL_CHECKS
        netdata_log_info("Changing map %s from size %u to %u ", map_name, lmap->internal_input, lmap->user_input);
#endif
    } else if (((lmap->type & apps_type) == apps_type) && (!em->apps_charts) && (!em->cgroup_charts)) {
        lmap->user_input = ND_EBPF_DEFAULT_MIN_PID;
    } else if (((em->apps_charts) || (em->cgroup_charts)) && (em->apps_level != NETDATA_APPS_NOT_SET)) {
        switch (em->apps_level) {
            case NETDATA_APPS_LEVEL_ALL: {
                define_size = lmap->user_input;
                break;
            }
            case NETDATA_APPS_LEVEL_PARENT: {
                define_size = ND_EBPF_DEFAULT_PID_SIZE / 2;
                break;
            }
            case NETDATA_APPS_LEVEL_REAL_PARENT:
            default: {
                define_size = ND_EBPF_DEFAULT_PID_SIZE / 3;
            }
        }
    }

    if (!define_size)
        return;

#ifdef LIBBPF_MAJOR_VERSION
    bpf_map__set_max_entries(map, define_size);
#else
    bpf_map__resize(map, define_size);
#endif
}

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Update map type
 *
 * Update map type with information given.
 *
 * @param map   the map we want to modify
 * @param w     a structure with user input
 */
void ebpf_update_map_type(struct bpf_map *map, ebpf_local_maps_t *w)
{
    if (bpf_map__set_type(map, w->map_type)) {
        netdata_log_error("Cannot modify map type for %s", w->name);
    }
}

/**
 * Define map type
 *
 * This PR defines the type used by hash tables according user input.
 *
 * @param maps          the list of maps used with a hash table.
 * @param maps_per_core define if map type according user specification.
 * @param kver          kernel version host is running.
 */
void ebpf_define_map_type(ebpf_local_maps_t *maps, int maps_per_core, int kver)
{
    if (!maps)
        return;

    // Before kernel 4.06 there was not percpu hash tables
    if (kver < NETDATA_EBPF_KERNEL_4_06)
        maps_per_core = CONFIG_BOOLEAN_NO;

    int i = 0;
    while (maps[i].name) {
        ebpf_local_maps_t *map = &maps[i];
        // maps_per_core is a boolean value in configuration files.
        if (maps_per_core) {
            if (map->map_type == BPF_MAP_TYPE_HASH)
                map->map_type = BPF_MAP_TYPE_PERCPU_HASH;
            else if (map->map_type == BPF_MAP_TYPE_ARRAY)
                map->map_type = BPF_MAP_TYPE_PERCPU_ARRAY;
        } else {
            if (map->map_type == BPF_MAP_TYPE_PERCPU_HASH)
                map->map_type = BPF_MAP_TYPE_HASH;
            else if (map->map_type == BPF_MAP_TYPE_PERCPU_ARRAY)
                map->map_type = BPF_MAP_TYPE_ARRAY;
        }

        i++;
    }
}
#endif

/**
 * Update Legacy map
 *
 * Update map for eBPF legacy code.
 *
 * @param program the structure with values read from binary.
 * @param em      the structure with information about how the module/thread is working.
 */
static void ebpf_update_legacy_map(struct bpf_object *program, ebpf_module_t *em)
{
    struct bpf_map *map;
    ebpf_local_maps_t *maps = em->maps;
    if (!maps)
        return;

    bpf_map__for_each(map, program)
    {
        const char *map_name = bpf_map__name(map);
        int i = 0;
        while (maps[i].name) {
            ebpf_local_maps_t *w = &maps[i];

            if (!strcmp(w->name, map_name)) {
                // Modify size
                if (w->type & NETDATA_EBPF_MAP_RESIZABLE) {
                    ebpf_update_map_size(map, w, em, map_name);
                }

#ifdef LIBBPF_MAJOR_VERSION
                ebpf_update_map_type(map, w);
#endif
            }

            i++;
        }
    }
}

size_t ebpf_count_programs(struct bpf_object *obj)
{
    size_t tot = 0;
    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj)
    {
        tot++;
    }

    return tot;
}

static ebpf_specify_name_t *ebpf_find_names(ebpf_specify_name_t *names, const char *prog_name)
{
    size_t i = 0;
    while (names[i].program_name) {
        if (!strcmp(prog_name, names[i].program_name))
            return &names[i];

        i++;
    }

    return NULL;
}

static struct bpf_link **ebpf_attach_programs(struct bpf_object *obj, size_t length, ebpf_specify_name_t *names)
{
    struct bpf_link **links = callocz(length , sizeof(struct bpf_link *));
    size_t i = 0;
    struct bpf_program *prog;
    ebpf_specify_name_t *w;
    bpf_object__for_each_program(prog, obj)
    {
        if (names) {
            const char *name = bpf_program__name(prog);
            w = ebpf_find_names(names, name);
        } else
            w = NULL;

        if (w) {
            enum bpf_prog_type type = bpf_program__get_type(prog);
            if (type == BPF_PROG_TYPE_KPROBE)
                links[i] = bpf_program__attach_kprobe(prog, w->retprobe, w->optional);
        } else
            links[i] = bpf_program__attach(prog);

        if (libbpf_get_error(links[i])) {
            links[i] = NULL;
        }

        i++;
    }

    return links;
}

static void ebpf_update_maps(ebpf_module_t *em, struct bpf_object *obj)
{
    if (!em->maps)
        return;

    ebpf_local_maps_t *maps = em->maps;
    struct bpf_map *map;
    bpf_map__for_each(map, obj)
    {
        int fd = bpf_map__fd(map);
        if (maps) {
            const char *map_name = bpf_map__name(map);
            int j = 0;
            while (maps[j].name) {
                ebpf_local_maps_t *w = &maps[j];
                if (w->map_fd == ND_EBPF_MAP_FD_NOT_INITIALIZED && !strcmp(map_name, w->name))
                    w->map_fd = fd;

                j++;
            }
        }
    }
}

/**
 * Update Controller
 *
 * Update controller value with user input.
 *
 * @param fd   the table file descriptor
 * @param em   structure with information about eBPF program we will load.
 */
void ebpf_update_controller(int fd, ebpf_module_t *em)
{
    uint32_t values[NETDATA_CONTROLLER_END] = {
        (em->apps_charts & NETDATA_EBPF_APPS_FLAG_YES) | em->cgroup_charts,
        em->apps_level, 0, 0, 0, 0
    };
    uint32_t key;
    uint32_t end = NETDATA_CONTROLLER_PID_TABLE_ADD;

    for (key = NETDATA_CONTROLLER_APPS_ENABLED; key < end; key++) {
        int ret = bpf_map_update_elem(fd, &key, &values[key], BPF_ANY);
        if (ret)
            netdata_log_error("Add key(%u) for controller table failed.", key);
    }
}

/**
 * Update Legacy controller
 *
 * Update legacy controller table when eBPF program has it.
 *
 * @param em   structure with information about eBPF program we will load.
 * @param obj  bpf object with tables.
 */
static void ebpf_update_legacy_controller(ebpf_module_t *em, struct bpf_object *obj)
{
    ebpf_local_maps_t *maps = em->maps;
    if (!maps)
        return;

    struct bpf_map *map;
    bpf_map__for_each(map, obj)
    {
        size_t i = 0;
        while (maps[i].name) {
            ebpf_local_maps_t *w = &maps[i];
            if (w->map_fd != ND_EBPF_MAP_FD_NOT_INITIALIZED && (w->type & NETDATA_EBPF_MAP_CONTROLLER)) {
                w->type &= ~NETDATA_EBPF_MAP_CONTROLLER;
                w->type |= NETDATA_EBPF_MAP_CONTROLLER_UPDATED;

                ebpf_update_controller(w->map_fd, em);
            }
            i++;
        }
    }
}

/**
 * Load Program
 *
 * Load eBPF program into kernel
 *
 * @param plugins_dir    directory where binary are stored
 * @param em             structure with information about eBPF program we will load.
 * @param kver           the kernel version according /usr/include/linux/version.h
 * @param is_rhf         is a kernel from Red Hat Family?
 * @param obj            structure where we will store object loaded.
 *
 * @return it returns a link for each target we associated an eBPF program.
 */
struct bpf_link **ebpf_load_program(char *plugins_dir, ebpf_module_t *em, int kver, int is_rhf,
                                    struct bpf_object **obj)
{
    char lpath[4096];

    uint32_t idx = ebpf_select_index(em->kernels, is_rhf, kver);

    ebpf_mount_name(lpath, 4095, plugins_dir, idx, em->info.thread_name, em->mode, is_rhf);

    // When this function is called ebpf.plugin is using legacy code, so we should reset the variable
    em->load &= ~ NETDATA_EBPF_LOAD_METHODS;
    em->load |= EBPF_LOAD_LEGACY;

    *obj = bpf_object__open_file(lpath, NULL);
    if (!*obj)
        return NULL;

    if (libbpf_get_error(obj)) {
        bpf_object__close(*obj);
        return NULL;
    }

    ebpf_update_legacy_map(*obj, em);

    if (bpf_object__load(*obj)) {
        netdata_log_error("ERROR: loading BPF object file failed %s\n", lpath);
        bpf_object__close(*obj);
        return NULL;
    }

    ebpf_update_maps(em, *obj);
    ebpf_update_legacy_controller(em, *obj);

    size_t count_programs =  ebpf_count_programs(*obj);

#ifdef NETDATA_INTERNAL_CHECKS
    netdata_log_info("eBPF program %s loaded with success!", lpath);
#endif

    return ebpf_attach_programs(*obj, count_programs, em->names);
}

char *ebpf_find_symbol(char *search)
{
    char filename[FILENAME_MAX + 1];
    char *ret = NULL;
    snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, NETDATA_KALLSYMS);
    procfile *ff = procfile_open(filename, " \t", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        netdata_log_error("Cannot open %s%s", netdata_configured_host_prefix, NETDATA_KALLSYMS);
        return ret;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return ret;

    unsigned long i, lines = procfile_lines(ff);
    size_t length = strlen(search);
    for(i = 0; i < lines ; i++) {
        char *cmp = procfile_lineword(ff, i,2);
        if (!strncmp(search, cmp, length)) {
            ret = strdupz(cmp);
            break;
        }
    }

    procfile_close(ff);

    return ret;
}

void ebpf_update_names(ebpf_specify_name_t *opt, ebpf_module_t *em)
{
    int mode = em->mode;
    em->names = opt;

    size_t i = 0;
    while (opt[i].program_name) {
        opt[i].retprobe = (mode == MODE_RETURN);
        opt[i].optional = ebpf_find_symbol(opt[i].function_to_attach);

        i++;
    }
}

//----------------------------------------------------------------------------------------------------------------------

void ebpf_mount_config_name(char *filename, size_t length, char *path, const char *config)
{
    snprintf(filename, length, "%s/ebpf.d/%s", path, config);
}

int ebpf_load_config(struct config *config, char *filename)
{
    return inicfg_load(config, filename, 0, NULL);
}


static netdata_run_mode_t ebpf_select_mode(const char *mode)
{
    if (!strcasecmp(mode,EBPF_CFG_LOAD_MODE_RETURN ))
        return MODE_RETURN;
    else if  (!strcasecmp(mode, "dev"))
        return MODE_DEVMODE;

    return MODE_ENTRY;
}

static void ebpf_select_mode_string(char *output, size_t len, netdata_run_mode_t  sel)
{
    if (sel == MODE_RETURN)
        strncpyz(output, EBPF_CFG_LOAD_MODE_RETURN, len);
    else
        strncpyz(output, EBPF_CFG_LOAD_MODE_DEFAULT, len);
}

/**
 * Convert string to load mode
 *
 * Convert the string given as argument to value present in enum.
 *
 * @param str  value read from configuration file.
 *
 * @return It returns the value to be used.
 */
netdata_ebpf_load_mode_t epbf_convert_string_to_load_mode(const char *str)
{
    if (!strcasecmp(str, EBPF_CFG_CORE_PROGRAM))
        return EBPF_LOAD_CORE;
    else if (!strcasecmp(str, EBPF_CFG_LEGACY_PROGRAM))
        return EBPF_LOAD_LEGACY;

    return EBPF_LOAD_PLAY_DICE;
}

/**
 * Convert load mode to string
 *
 * @param mode value that will select the string
 *
 * @return It returns the string associated to mode.
 */
static char *ebpf_convert_load_mode_to_string(netdata_ebpf_load_mode_t mode)
{
    if (mode & EBPF_LOAD_CORE)
        return EBPF_CFG_CORE_PROGRAM;
    else if (mode & EBPF_LOAD_LEGACY)
        return EBPF_CFG_LEGACY_PROGRAM;

    return EBPF_CFG_DEFAULT_PROGRAM;
}

/**
 * Convert collect pid to string
 *
 * @param level value that will select the string
 *
 * @return It returns the string associated to level.
 */
static char *ebpf_convert_collect_pid_to_string(netdata_apps_level_t level)
{
    if (level == NETDATA_APPS_LEVEL_REAL_PARENT)
        return EBPF_CFG_PID_REAL_PARENT;
    else if (level == NETDATA_APPS_LEVEL_PARENT)
        return EBPF_CFG_PID_PARENT;
    else if (level == NETDATA_APPS_LEVEL_ALL)
        return EBPF_CFG_PID_ALL;

    return EBPF_CFG_PID_INTERNAL_USAGE;
}

/**
 * Convert string to apps level
 *
 * @param str the argument read from config files
 *
 * @return it returns the level associated to the string or default when it is a wrong value
 */
netdata_apps_level_t ebpf_convert_string_to_apps_level(const char *str)
{
    if (!strcasecmp(str, EBPF_CFG_PID_REAL_PARENT))
        return NETDATA_APPS_LEVEL_REAL_PARENT;
    else if (!strcasecmp(str, EBPF_CFG_PID_PARENT))
        return NETDATA_APPS_LEVEL_PARENT;
    else if (!strcasecmp(str, EBPF_CFG_PID_ALL))
        return NETDATA_APPS_LEVEL_ALL;

    return NETDATA_APPS_NOT_SET;
}

/**
 *  CO-RE type
 *
 *  Select the preferential type of CO-RE
 *
 *  @param str    value read from configuration file.
 *  @param lmode  load mode used by collector.
 */
netdata_ebpf_program_loaded_t ebpf_convert_core_type(const char *str, netdata_run_mode_t lmode)
{
    if (!strcasecmp(str, EBPF_CFG_ATTACH_TRACEPOINT))
        return EBPF_LOAD_TRACEPOINT;
    else if (!strcasecmp(str, EBPF_CFG_ATTACH_PROBE)) {
        return (lmode == MODE_ENTRY) ? EBPF_LOAD_PROBE : EBPF_LOAD_RETPROBE;
    }

    return EBPF_LOAD_TRAMPOLINE;
}

#ifdef LIBBPF_MAJOR_VERSION
/**
 * Adjust Thread Load
 *
 * Adjust thread configuration according specified load.
 *
 * @param mod   the main structure that will be adjusted.
 * @param file  the btf file used with thread.
 */
void ebpf_adjust_thread_load(ebpf_module_t *mod, struct btf *file)
{
    if (!file) {
        mod->load &= ~EBPF_LOAD_CORE;
        mod->load |= EBPF_LOAD_LEGACY;
    } else if (mod->load == EBPF_LOAD_PLAY_DICE && file) {
        mod->load &= ~EBPF_LOAD_LEGACY;
        mod->load |= EBPF_LOAD_CORE;
    }
}

/**
 * Parse BTF file
 *
 * Parse a specific BTF file present on filesystem
 *
 * @param filename  the file that will be parsed.
 *
 * @return It returns a pointer for the file on success and NULL otherwise.
 */
struct btf *ebpf_parse_btf_file(const char *filename)
{
    struct btf *bf = btf__parse(filename, NULL);
    if (libbpf_get_error(bf)) {
        fprintf(stderr, "Cannot parse btf file");
        btf__free(bf);
        return NULL;
    }

    return bf;
}

/**
 * Load default btf file
 *
 * Load the default BTF file on environment.
 *
 * @param path     is the fullpath
 * @param filename is the file inside BTF path.
 */
struct btf *ebpf_load_btf_file(const char *path, const char *filename)
{
    char fullpath[PATH_MAX + 1];
    snprintfz(fullpath, PATH_MAX, "%s/%s", path, filename);
    struct btf *ret = ebpf_parse_btf_file(fullpath);
    if (!ret)
        netdata_log_info("Your environment does not have BTF file %s/%s. The plugin will work with 'legacy' code.",
             path, filename);

    return ret;
}

/**
 * Find BTF attach type
 *
 * Search type fr current btf file.
 *
 * @param file     is the structure for the btf file already parsed.
 */
static inline const struct btf_type *ebpf_find_btf_attach_type(struct btf *file)
{
    int id = btf__find_by_name_kind(file, "bpf_attach_type", BTF_KIND_ENUM);
    if (id < 0) {
        fprintf(stderr, "Cannot find 'bpf_attach_type'");

        return NULL;
    }

    return btf__type_by_id(file, id);
}

/**
 * Is function inside BTF
 *
 * Look for a specific function inside the given BTF file.
 *
 * @param file     is the structure for the btf file already parsed.
 * @param function is the function that we want to find.
 */
int ebpf_is_function_inside_btf(struct btf *file, char *function)
{
    const struct btf_type *type = ebpf_find_btf_attach_type(file);
    if (!type)
        return -1;

    const struct btf_enum *e = btf_enum(type);
    int i, id;
    for (id = -1, i = 0; i < btf_vlen(type); i++, e++) {
        if (!strcmp(btf__name_by_offset(file, e->name_off), "BPF_TRACE_FENTRY")) {
            id = btf__find_by_name_kind(file, function, BTF_KIND_FUNC);
            break;
        }
    }

    return (id > 0) ? 1 : 0;
}
#endif

/**
 * Update target with configuration
 *
 * Update target load mode with value.
 *
 * @param em       the module structure
 * @param value    value used to update.
 */
static void ebpf_update_target_with_conf(ebpf_module_t *em, netdata_ebpf_program_loaded_t value)
{
    netdata_ebpf_targets_t *targets = em->targets;
    if (!targets) {
        return;
    }

    int i = 0;
    while (targets[i].name) {
        targets[i].mode = value;
        i++;
    }
}

/**
 * Select Load Mode
 *
 * Select the load mode according the given inputs.
 *
 * @param btf_file a pointer to the loaded btf file.
 * @parma load     current value.
 * @param btf_file a pointer to the loaded btf file.
 * @param is_rhf is Red Hat family?
 *
 * @return it returns the new load mode.
 */
static netdata_ebpf_load_mode_t ebpf_select_load_mode(struct btf *btf_file __maybe_unused,
                                                      netdata_ebpf_load_mode_t load,
                                                      int kver __maybe_unused,
                                                      int is_rh __maybe_unused)
{
#ifdef LIBBPF_MAJOR_VERSION
    if ((load & EBPF_LOAD_CORE) || (load & EBPF_LOAD_PLAY_DICE)) {
        // Quick fix for Oracle linux 8.x
        load = (!btf_file || (is_rh && (kver >= NETDATA_EBPF_KERNEL_5_4 && kver < NETDATA_EBPF_KERNEL_5_5))) ?
               EBPF_LOAD_LEGACY : EBPF_LOAD_CORE;
    }
#else
    load = EBPF_LOAD_LEGACY;
#endif

    return load;
}

/**
 * Update Module using config
 *
 * Update configuration for a specific thread.
 *
 * @param modules   structure that will be updated
 * @param origin    specify the configuration file loaded
 * @param btf_file a pointer to the loaded btf file.
 * @param is_rhf is Red Hat family?
 */
void ebpf_update_module_using_config(ebpf_module_t *modules, netdata_ebpf_load_mode_t origin, struct btf *btf_file,
                                     int kver, int is_rh)
{
    char default_value[EBPF_MAX_MODE_LENGTH + 1];
    ebpf_select_mode_string(default_value, EBPF_MAX_MODE_LENGTH, modules->mode);
    const char *load_mode = inicfg_get(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_LOAD_MODE, default_value);
    modules->mode = ebpf_select_mode(load_mode);

    modules->update_every = (int)inicfg_get_number(modules->cfg, EBPF_GLOBAL_SECTION,
                                                     EBPF_CFG_UPDATE_EVERY, modules->update_every);

    modules->apps_charts = inicfg_get_boolean(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_APPLICATION,
                                                 (int) (modules->apps_charts & NETDATA_EBPF_APPS_FLAG_YES));

    modules->cgroup_charts = inicfg_get_boolean(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_CGROUP,
                                                   modules->cgroup_charts);

    modules->pid_map_size = (uint32_t)inicfg_get_number(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_PID_SIZE,
                                                           modules->pid_map_size);

    modules->lifetime = (uint32_t) inicfg_get_number(modules->cfg, EBPF_GLOBAL_SECTION,
                                                        EBPF_CFG_LIFETIME, EBPF_DEFAULT_LIFETIME);

    char *value = ebpf_convert_load_mode_to_string(modules->load & NETDATA_EBPF_LOAD_METHODS);
    const char *type_format = inicfg_get(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_TYPE_FORMAT, value);
    netdata_ebpf_load_mode_t load = epbf_convert_string_to_load_mode(type_format);
    load = ebpf_select_load_mode(btf_file, load, kver, is_rh);
    modules->load = origin | load;

    const char *core_attach = inicfg_get(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_CORE_ATTACH, EBPF_CFG_ATTACH_TRAMPOLINE);
    netdata_ebpf_program_loaded_t fill_lm = ebpf_convert_core_type(core_attach, modules->mode);
    ebpf_update_target_with_conf(modules, fill_lm);

    value = ebpf_convert_collect_pid_to_string(modules->apps_level);
    const char *collect_pid = inicfg_get(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_COLLECT_PID, value);
    modules->apps_level =  ebpf_convert_string_to_apps_level(collect_pid);

    modules->maps_per_core = inicfg_get_boolean(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_MAPS_PER_CORE,
                                                   modules->maps_per_core);
    if (kver < NETDATA_EBPF_KERNEL_4_06)
        modules->maps_per_core = CONFIG_BOOLEAN_NO;

#ifdef NETDATA_DEV_MODE
    netdata_log_info("The thread %s was configured with: mode = %s; update every = %d; apps = %s; cgroup = %s; ebpf type format = %s; ebpf co-re tracing = %s; collect pid = %s; maps per core = %s, lifetime=%u",
         modules->info.thread_name,
         load_mode,
         modules->update_every,
         (modules->apps_charts)?"enabled":"disabled",
         (modules->cgroup_charts)?"enabled":"disabled",
         type_format,
         core_attach,
         collect_pid,
         (modules->maps_per_core)?"enabled":"disabled",
         modules->lifetime
         );
#endif
}

/**
 * Update module
 *
 * When this function is called, it will load the configuration file and after this
 * it updates the global information of ebpf_module.
 * If the module has specific configuration, this function will load it, but it will not
 * update the variables.
 *
 * @param em       the module structure
 * @param btf_file a pointer to the loaded btf file.
 * @param is_rhf is Red Hat family?
 * @param kver   the kernel version
 */
void ebpf_update_module(ebpf_module_t *em, struct btf *btf_file, int kver, int is_rh)
{
    char filename[FILENAME_MAX+1];
    netdata_ebpf_load_mode_t origin;

    ebpf_mount_config_name(filename, FILENAME_MAX, ebpf_user_config_dir, em->config_file);
    if (!ebpf_load_config(em->cfg, filename)) {
        ebpf_mount_config_name(filename, FILENAME_MAX, ebpf_stock_config_dir, em->config_file);
        if (!ebpf_load_config(em->cfg, filename)) {
            netdata_log_error("Cannot load the ebpf configuration file %s", em->config_file);
            return;
        }
        // If user defined data globally, we will have here EBPF_LOADED_FROM_USER, we need to consider this, to avoid
        // forcing users to configure thread by thread.
        origin = (!(em->load & NETDATA_EBPF_LOAD_SOURCE)) ? EBPF_LOADED_FROM_STOCK : em->load & NETDATA_EBPF_LOAD_SOURCE;
    } else
        origin = EBPF_LOADED_FROM_USER;

    ebpf_update_module_using_config(em, origin, btf_file, kver, is_rh);
}

/**
 * Adjust Apps Cgroup
 *
 * Apps and cgroup has internal cleanup that needs attaching tracers to release_task, to avoid overload the function
 * we will enable this integration by default, if and only if, we are running with trampolines.
 *
 * @param em   a pointer to the main thread structure.
 * @param mode is the mode used with different
 */
void ebpf_adjust_apps_cgroup(ebpf_module_t *em, netdata_ebpf_program_loaded_t mode)
{
    if ((em->load & EBPF_LOADED_FROM_STOCK) &&
    (em->apps_charts || em->cgroup_charts) &&
    mode != EBPF_LOAD_TRAMPOLINE) {
        em->apps_charts = NETDATA_EBPF_APPS_FLAG_NO;
        em->cgroup_charts = 0;
    }
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Load Address
 *
 * Helper used to get address from /proc/kallsym
 *
 * @param fa address structure
 * @param fd file descriptor loaded inside kernel. If a negative value is given
 *           the function will load address and it won't update hash table.
 */
void ebpf_load_addresses(ebpf_addresses_t *fa, int fd)
{
    if (fa->addr)
        return ;

    procfile *ff = procfile_open("/proc/kallsyms", " \t:", PROCFILE_FLAG_DEFAULT);
    if (!ff)
        return;

    ff = procfile_readall(ff);
    if (!ff)
        return;

    fa->hash = simple_hash(fa->function);

    size_t lines = procfile_lines(ff), l;
    for(l = 0; l < lines ;l++) {
        char *fcnt = procfile_lineword(ff, l, 2);
        uint32_t hash = simple_hash(fcnt);
        if (fa->hash == hash && !strcmp(fcnt, fa->function)) {
            char *type = procfile_lineword(ff, l, 2);
            fa->type = type[0];
            if (fd > 0) {
                char addr[128];
                snprintf(addr, 127, "0x%s", procfile_lineword(ff, l, 0));
                fa->addr = (unsigned long) strtoul(addr, NULL, 16);
                uint32_t key = 0;
                bpf_map_update_elem(fd, &key, &fa->addr, BPF_ANY);
            } else
                fa->addr = 1;
            break;
        }
    }

    procfile_close(ff);
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Fill Algorithms
 *
 * Set one unique dimension for all vector position.
 *
 * @param algorithms the output vector
 * @param length     number of elements of algorithms vector
 * @param algorithm  algorithm used on charts.
*/
void ebpf_fill_algorithms(int *algorithms, size_t length, int algorithm)
{
    size_t i;
    for (i = 0; i < length; i++) {
        algorithms[i] = algorithm;
    }
}

/**
 * Fill Histogram dimension
 *
 * Fill the histogram dimension with the specified ranges
 */
char **ebpf_fill_histogram_dimension(size_t maximum)
{
    char *dimensions[] = { "us", "ms", "s"};
    int previous_dim = 0, current_dim = 0;
    uint32_t previous_level = 1000, current_level = 1000;
    uint32_t previous_divisor = 1, current_divisor = 1;
    uint32_t current = 1, previous = 0;
    uint32_t selector;
    char **out = callocz(maximum, sizeof(char *));
    char range[128];
    size_t end = maximum - 1;
    for (selector = 0; selector < end; selector++) {
        snprintf(range, 127, "%u%s->%u%s", previous/previous_divisor, dimensions[previous_dim],
                 current/current_divisor, dimensions[current_dim]);
        out[selector] = strdupz(range);
        previous = current;
        current <<= 1;

        if (previous_dim != 2 && previous > previous_level) {
            previous_dim++;

            previous_divisor *= 1000;
            previous_level *= 1000;
        }

        if (current_dim != 2 && current > current_level) {
            current_dim++;

            current_divisor *= 1000;
            current_level *= 1000;
        }
    }
    snprintf(range, 127, "%u%s->+Inf", previous/previous_divisor, dimensions[previous_dim]);
    out[selector] = strdupz(range);

    return out;
}

/**
 * Histogram dimension cleanup
 *
 * Cleanup dimensions allocated with function ebpf_fill_histogram_dimension
 *
 * @param ptr
 * @param length
 */
void ebpf_histogram_dimension_cleanup(char **ptr, size_t length)
{
    size_t i;
    for (i = 0; i < length; i++) {
        freez(ptr[i]);
    }
    freez(ptr);
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Open tracepoint path
 *
 * @param filename   pointer to store the path
 * @param length     file length
 * @param subsys     is the name of your subsystem.
 * @param eventname  is the name of the event to trace.
 * @param flags      flags used with syscall open
 *
 * @return it returns a positive value on success and a negative otherwise.
 */
static inline int ebpf_open_tracepoint_path(char *filename, size_t length, char *subsys, char *eventname, int flags)
{
    snprintfz(filename, length, "%s/events/%s/%s/enable", NETDATA_DEBUGFS, subsys, eventname);
    return open(filename, flags | O_CLOEXEC, 0);
}

/**
 * Is tracepoint enabled
 *
 * Check whether the tracepoint is enabled.
 *
 * @param subsys     is the name of your subsystem.
 * @param eventname  is the name of the event to trace.
 *
 * @return  it returns 1 when it is enabled, 0 when it is disabled and -1 on error.
 */
int ebpf_is_tracepoint_enabled(char *subsys, char *eventname)
{
    char text[FILENAME_MAX + 1];
    int fd = ebpf_open_tracepoint_path(text, FILENAME_MAX, subsys, eventname, O_RDONLY);
    if (fd < 0) {
        return -1;
    }

    ssize_t length = read(fd, text, 1);
    if (length != 1) {
        close(fd);
        return -1;
    }
    close(fd);

    return (text[0] == '1') ? CONFIG_BOOLEAN_YES : CONFIG_BOOLEAN_NO;
}

/**
 *  Change Tracing values
 *
 * Change value for specific tracepoint enabling or disabling it according value given.
 *
 * @param subsys     is the name of your subsystem.
 * @param eventname  is the name of the event to trace.
 * @param value      a value to enable (1) or disable (0) a tracepoint.
 *
 * @return It returns 0 on success and -1 otherwise
 */
static int ebpf_change_tracing_values(char *subsys, char *eventname, char *value)
{
    if (strcmp("0", value) && strcmp("1", value)) {
        netdata_log_error("Invalid value given to either enable or disable a tracepoint.");
        return -1;
    }

    char filename[1024];
    int fd = ebpf_open_tracepoint_path(filename, 1023, subsys, eventname, O_WRONLY);
    if (fd < 0) {
        return -1;
    }

    ssize_t written = write(fd, value, strlen(value));
    if (written < 0) {
        close(fd);
        return -1;
    }

    close(fd);
    return 0;
}

/**
 * Enable tracing values
 *
 * Enable a tracepoint on a system
 *
 * @param subsys     is the name of your subsystem.
 * @param eventname  is the name of the event to trace.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_enable_tracing_values(char *subsys, char *eventname)
{
    return ebpf_change_tracing_values(subsys, eventname, "1");
}

/**
 * Disable tracing values
 *
 * Disable tracing points enabled by collector
 *
 * @param subsys     is the name of your subsystem.
 * @param eventname  is the name of the event to trace.
 *
 * @return It returns 0 on success and -1 otherwise
 */
int ebpf_disable_tracing_values(char *subsys, char *eventname)
{
    return ebpf_change_tracing_values(subsys, eventname, "0");
}

/**
 * Select PC prefix
 *
 * Identify the prefix to run on PC architecture.
 *
 * @return It returns 32 or 64 according to host arch.
 */
static uint32_t ebpf_select_pc_prefix()
{
#if SIZE_OF_VOID_P == 4
    return 32;
#else
    return 64;
#endif
}

/**
 * Select Host Prefix
 *
 * Select prefix to syscall when host is running a kernel newer than 4.17.0
 *
 * @param output the vector to store data.
 * @param length length of output vector.
 * @param syscall the syscall that prefix will be attached;
 * @param kver    the current kernel version in format MAJOR*65536 + MINOR*256 + PATCH
 */
void ebpf_select_host_prefix(char *output, size_t length, char *syscall, int kver)
{
    if (kver < NETDATA_EBPF_KERNEL_4_17)
        snprintfz(output, length, "sys_%s", syscall);
    else {
        uint32_t arch = ebpf_select_pc_prefix();
        // Prefix selected according https://www.kernel.org/doc/html/latest/process/adding-syscalls.html
        char *prefix = (arch == 32) ? "__ia32" : "__x64";
        snprintfz(output, length, "%s_sys_%s", prefix, syscall);
    }
}

