// SPDX-License-Identifier: GPL-3.0-or-later

#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <dlfcn.h>

#include "../libnetdata.h"

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

//----------------------------------------------------------------------------------------------------------------------

int get_kernel_version(char *out, int size)
{
    char major[16], minor[16], patch[16];
    char ver[256];
    char *version = ver;

    out[0] = '\0';
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
    while (*version && *version != '\n')
        *move++ = *version++;
    *move = '\0';

    fd = snprintf(out, (size_t)size, "%s.%s.%s", major, minor, patch);
    if (fd > size)
        error("The buffer to store kernel version is not smaller than necessary.");

    return ((int)(str2l(major) * 65536) + (int)(str2l(minor) * 256) + (int)str2l(patch));
}

int get_redhat_release()
{
    char buffer[256];
    int major, minor;
    FILE *fp = fopen("/etc/redhat-release", "r");

    if (fp) {
        major = 0;
        minor = -1;
        size_t length = fread(buffer, sizeof(char), 255, fp);
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

static int has_ebpf_kernel_version(int version)
{
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

char *ebpf_kernel_suffix(int version, int isrh)
{
    if (isrh) {
        if (version >= NETDATA_EBPF_KERNEL_4_11)
            return "4.18.0";
        else
            return "3.10.0";
    } else {
        if (version >= NETDATA_EBPF_KERNEL_4_17)
            return "5.4.20";
        else if (version >= NETDATA_EBPF_KERNEL_4_15)
            return "4.16.18";
        else if (version >= NETDATA_EBPF_KERNEL_4_11)
            return "4.14.171";
    }

    return NULL;
}

//----------------------------------------------------------------------------------------------------------------------

int ebpf_update_kernel(ebpf_data_t *ed)
{
    char *kernel = ebpf_kernel_suffix(ed->running_on_kernel, (ed->isrh < 0) ? 0 : 1);
    size_t length = strlen(kernel);
    strncpyz(ed->kernel_string, kernel, length);
    ed->kernel_string[length] = '\0';

    return 0;
}

static int select_file(char *name, const char *program, size_t length, int mode, char *kernel_string)
{
    int ret = -1;
    if (!mode)
        ret = snprintf(name, length, "rnetdata_ebpf_%s.%s.o", program, kernel_string);
    else if (mode == 1)
        ret = snprintf(name, length, "dnetdata_ebpf_%s.%s.o", program, kernel_string);
    else if (mode == 2)
        ret = snprintf(name, length, "pnetdata_ebpf_%s.%s.o", program, kernel_string);

    return ret;
}

int ebpf_load_program(char *plugins_dir, int event_id, int mode, char *kernel_string, const char *name, int *map_fd)
{
    UNUSED(event_id);

    char lpath[4096];
    char lname[128];
    struct bpf_object *obj;
    int prog_fd;

    int test = select_file(lname, name, (size_t)127, mode, kernel_string);
    if (test < 0 || test > 127)
        return -1;

    snprintf(lpath, 4096, "%s/%s", plugins_dir, lname);
    if (bpf_prog_load(lpath, BPF_PROG_TYPE_KPROBE, &obj, &prog_fd)) {
        info("Cannot load program: %s", lpath);
        return -1;
    } else {
        info("The eBPF program %s was loaded with success.", name);
    }

    struct bpf_map *map;
    size_t i = 0;
    bpf_map__for_each(map, obj)
    {
        map_fd[i] = bpf_map__fd(map);
        i++;
    }

    struct bpf_program *prog;
    bpf_object__for_each_program(prog, obj)
    {
        bpf_program__attach(prog);
    }

    return 0;
}
