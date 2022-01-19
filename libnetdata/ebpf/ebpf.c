// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <dlfcn.h>
#include <sys/utsname.h>

#include "../libnetdata.h"

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
    debug(D_EXIT, "Cleaning parent process events.");
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

    int fd = open("/proc/sys/kernel/osrelease", O_RDONLY);
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

    if (read_file("/proc/version_signature", version_string, VERSION_STRING_LEN)) {
        if (read_file("/proc/version", version_string, VERSION_STRING_LEN)) {
            struct utsname uname_buf;
            if (!uname(&uname_buf)) {
                info("Cannot check kernel version");
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
                info("A buggy kernel is detected");
                fclose(kernel_reject_list);
                freez(reject_string);
                return 1;
            }
        }
    }

    fclose(kernel_reject_list);
    freez(reject_string);

    return 0;
}

static int has_ebpf_kernel_version(int version)
{
    if (kernel_is_rejected())
        return 0;

    // Kernel 4.11.0 or RH > 7.5
    return (version >= NETDATA_MINIMUM_EBPF_KERNEL || get_redhat_release() >= NETDATA_MINIMUM_RH_VERSION);
}

int has_condition_to_run(int version)
{
    if (!has_ebpf_kernel_version(version))
        return 0;

    return 1;
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
                                    NETDATA_IDX_STR_V5_11, NETDATA_IDX_STR_V5_15
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
        if (kver >= NETDATA_EBPF_KERNEL_4_11)
            return NETDATA_IDX_V4_18;
    } else { // Kernels from kernel.org
        if (kver >= NETDATA_EBPF_KERNEL_5_15)
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
 *  @param path
 *  @param len       the size of the out vector.
 *  @param kver      the kernel version
 *  @param name      the eBPF program name.
 *  @param is_return is return or entry ?
 */
static void ebpf_mount_name(char *out, size_t len, char *path, uint32_t kver, const char *name, int is_return)
{
    char *version = ebpf_select_kernel_name(kver);
    snprintfz(out, len, "%s/ebpf.d/%cnetdata_ebpf_%s.%s.o",
              path,
              (is_return) ? 'r' : 'p',
              name,
              version);
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Statistics from targets
 *
 * Count the information from targets.
 *
 * @param report  the output structure
 * @param targets  vector with information about the eBPF plugin.
 */
static void ebpf_stats_targets(ebpf_plugin_stats_t *report, netdata_ebpf_targets_t *targets)
{
    if (!targets) {
        report->probes = report->tracepoints = report->trampolines = 0;
        return;
    }

    int i = 0;
    while (targets[i].name) {
        switch (targets[i].mode) {
            case EBPF_LOAD_PROBE: {
                report->probes++;
                break;
            }
            case EBPF_LOAD_RETPROBE: {
                report->retprobes++;
                break;
            }
            case EBPF_LOAD_TRACEPOINT: {
                report->tracepoints++;
                break;
            }
            case EBPF_LOAD_TRAMPOLINE: {
                report->trampolines++;
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
    report->threads++;

    // It is not necessary to report more information.
    if (!em->enabled)
        return;

    report->running++;

    // In theory the `else if` is useless, because when this function is called, the module should not stay in
    // EBPF_LOAD_PLAY_DICE. We have this additional condition to detect errors from developers.
    if (em->load == EBPF_LOAD_LEGACY)
        report->legacy++;
    else if (em->load == EBPF_LOAD_CORE)
        report->core++;

    ebpf_stats_targets(report, em->targets);
}

//----------------------------------------------------------------------------------------------------------------------

void ebpf_update_pid_table(ebpf_local_maps_t *pid, ebpf_module_t *em)
{
    pid->user_input = em->pid_map_size;
}

void ebpf_update_map_sizes(struct bpf_object *program, ebpf_module_t *em)
{
    struct bpf_map *map;
    ebpf_local_maps_t *maps = em->maps;
    if (!maps)
        return;

    uint32_t apps_type = NETDATA_EBPF_MAP_PID | NETDATA_EBPF_MAP_RESIZABLE;
    bpf_map__for_each(map, program)
    {
        const char *map_name = bpf_map__name(map);
        int i = 0; ;
        while (maps[i].name) {
            ebpf_local_maps_t *w = &maps[i];
            if (w->type & NETDATA_EBPF_MAP_RESIZABLE) {
                if (!strcmp(w->name, map_name)) {
                    if (w->user_input && w->user_input != w->internal_input) {
#ifdef NETDATA_INTERNAL_CHECKS
                        info("Changing map %s from size %u to %u ", map_name, w->internal_input, w->user_input);
#endif
                        bpf_map__resize(map, w->user_input);
                    } else if (((w->type & apps_type) == apps_type) && (!em->apps_charts) && (!em->cgroup_charts)) {
                        w->user_input = ND_EBPF_DEFAULT_MIN_PID;
                        bpf_map__resize(map, w->user_input);
                    }
                }
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
            int j = 0; ;
            while (maps[j].name) {
                ebpf_local_maps_t *w = &maps[j];
                if (w->map_fd == ND_EBPF_MAP_FD_NOT_INITIALIZED && !strcmp(map_name, w->name))
                    w->map_fd = fd;

                j++;
            }
        }
    }
}

static void ebpf_update_controller(ebpf_module_t *em, struct bpf_object *obj)
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

                uint32_t key = NETDATA_CONTROLLER_APPS_ENABLED;
                int value = em->apps_charts | em->cgroup_charts;
                int ret = bpf_map_update_elem(w->map_fd, &key, &value, 0);
                if (ret)
                    error("Add key(%u) for controller table failed.", key);
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

    ebpf_mount_name(lpath, 4095, plugins_dir, idx, em->thread_name, em->mode);

    *obj = bpf_object__open_file(lpath, NULL);
    if (libbpf_get_error(obj)) {
        error("Cannot open BPF object %s", lpath);
        bpf_object__close(*obj);
        return NULL;
    }

    ebpf_update_map_sizes(*obj, em);

    if (bpf_object__load(*obj)) {
        error("ERROR: loading BPF object file failed %s\n", lpath);
        bpf_object__close(*obj);
        return NULL;
    }

    ebpf_update_maps(em, *obj);
    ebpf_update_controller(em, *obj);

    size_t count_programs =  ebpf_count_programs(*obj);

#ifdef NETDATA_INTERNAL_CHECKS
    info("eBPF program %s loaded with success!", lpath);
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
        error("Cannot open %s%s", netdata_configured_host_prefix, NETDATA_KALLSYMS);
        return ret;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff))
        return ret;

    unsigned long i, lines = procfile_lines(ff);
    size_t length = strlen(search);
    for(i = 0; i < lines ; i++) {
        char *cmp = procfile_lineword(ff, i,2);;
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
    return appconfig_load(config, filename, 0, NULL);
}


static netdata_run_mode_t ebpf_select_mode(char *mode)
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
 * @param modules   structure that will be updated
 */
void ebpf_update_module_using_config(ebpf_module_t *modules)
{
    char default_value[EBPF_MAX_MODE_LENGTH + 1];
    ebpf_select_mode_string(default_value, EBPF_MAX_MODE_LENGTH, modules->mode);
    char *mode = appconfig_get(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_LOAD_MODE, default_value);
    modules->mode = ebpf_select_mode(mode);

    modules->update_every = (int)appconfig_get_number(modules->cfg, EBPF_GLOBAL_SECTION,
                                                     EBPF_CFG_UPDATE_EVERY, modules->update_every);

    modules->apps_charts = appconfig_get_boolean(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_APPLICATION,
                                                 modules->apps_charts);

    modules->pid_map_size = (uint32_t)appconfig_get_number(modules->cfg, EBPF_GLOBAL_SECTION, EBPF_CFG_PID_SIZE,
                                                           modules->pid_map_size);
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
 */
void ebpf_update_module(ebpf_module_t *em)
{
    char filename[FILENAME_MAX+1];
    ebpf_mount_config_name(filename, FILENAME_MAX, ebpf_user_config_dir, em->config_file);
    if (!ebpf_load_config(em->cfg, filename)) {
        ebpf_mount_config_name(filename, FILENAME_MAX, ebpf_stock_config_dir, em->config_file);
        if (!ebpf_load_config(em->cfg, filename)) {
            error("Cannot load the ebpf configuration file %s", em->config_file);
            return;
        }
    }

    ebpf_update_module_using_config(em);
}

//----------------------------------------------------------------------------------------------------------------------

/**
 * Load Address
 *
 * Helper used to get address from /proc/kallsym
 *
 * @param fa address structure
 * @param fd file descriptor loaded inside kernel.
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
            char addr[128];
            snprintf(addr, 127, "0x%s", procfile_lineword(ff, l, 0));
            fa->addr = (unsigned long) strtoul(addr, NULL, 16);
            uint32_t key = 0;
            bpf_map_update_elem(fd, &key, &fa->addr, BPF_ANY);
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
    return open(filename, flags, 0);
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
        error("Invalid value given to either enable or disable a tracepoint.");
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
