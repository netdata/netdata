// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"
#include "libnetdata/required_dummies.h"

SPAWN_SERVER *spawn_server = NULL;

char env_netdata_host_prefix[FILENAME_MAX + 50] = "";
char env_netdata_log_method[FILENAME_MAX + 50] = "";
char env_netdata_log_format[FILENAME_MAX + 50] = "";
char env_netdata_log_level[FILENAME_MAX + 50] = "";
char *environment[] = {
        "PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin",
        env_netdata_host_prefix,
        env_netdata_log_method,
        env_netdata_log_format,
        env_netdata_log_level,
        NULL
};

struct iface {
    const char *device;
    uint32_t hash;

    unsigned int ifindex;
    unsigned int iflink;

    struct iface *next;
};

unsigned int calc_num_ifaces(struct iface *root) {
    unsigned int num = 0;
    for (struct iface *h = root; h; h = h->next) {
        num++;
    }
    return num;
}

unsigned int read_iface_iflink(const char *prefix, const char *iface) {
    if(!prefix) prefix = "";

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/iflink", prefix, iface);

    unsigned long long iflink = 0;
    int ret = read_single_number_file(filename, &iflink);
    if(ret) nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot read '%s'.", filename);

    return (unsigned int)iflink;
}

unsigned int read_iface_ifindex(const char *prefix, const char *iface) {
    if(!prefix) prefix = "";

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/ifindex", prefix, iface);

    unsigned long long ifindex = 0;
    int ret = read_single_number_file(filename, &ifindex);
    if(ret) nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot read '%s'.", filename);

    return (unsigned int)ifindex;
}

struct iface *read_proc_net_dev(const char *scope __maybe_unused, const char *prefix) {
    if(!prefix) prefix = "";

    procfile *ff = NULL;
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", prefix, (*prefix)?"/proc/1/net/dev":"/proc/net/dev");

    ff = procfile_open(filename, " \t,:|", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open file '%s'", filename);
        return NULL;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot read file '%s'", filename);
        return NULL;
    }

    size_t lines = procfile_lines(ff), l;
    struct iface *root = NULL;
    for(l = 2; l < lines ;l++) {
        if (unlikely(procfile_linewords(ff, l) < 1)) continue;

        struct iface *t = callocz(1, sizeof(struct iface));
        t->device = strdupz(procfile_lineword(ff, l, 0));
        t->hash = simple_hash(t->device);
        t->ifindex = read_iface_ifindex(prefix, t->device);
        t->iflink  = read_iface_iflink(prefix, t->device);
        t->next = root;
        root = t;

        nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "added %s interface '%s', ifindex %u, iflink %u", scope, t->device, t->ifindex, t->iflink);
    }

    procfile_close(ff);

    return root;
}

void free_iface(struct iface *iface) {
    freez((void *)iface->device);
    freez(iface);
}

void free_host_ifaces(struct iface *iface) {
    while(iface) {
        struct iface *t = iface->next;
        free_iface(iface);
        iface = t;
    }
}

int iface_is_eligible(struct iface *iface) {
    if(iface->iflink != iface->ifindex)
        return 1;

    return 0;
}

int eligible_ifaces(struct iface *root) {
    int eligible = 0;

    struct iface *t;
    for(t = root; t ; t = t->next)
        if(iface_is_eligible(t))
            eligible++;

    return eligible;
}

static void continue_as_child(void) {
    pid_t child = fork();
    int status;
    pid_t ret;

    if (child < 0) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "fork() failed");
        exit(1);
    }

    if (child == 0) {
        // the child returns
        gettid_uncached();
        return;
    }

    // here is the parent
    for (;;) {
        ret = waitpid(child, &status, WUNTRACED);
        if ((ret == child) && (WIFSTOPPED(status))) {
            /* The child suspended so suspend us as well */
            kill(getpid(), SIGSTOP);
            kill(child, SIGCONT);
        } else {
            break;
        }
        tinysleep();
    }

    /* Return the child's exit code if possible */

#ifdef __SANITIZE_ADDRESS__
    /*
     * With sanitization, exiting leads to an infinite loop (100% cpu) here:
     *
     * #0  0x00007ffff690ea8b in sched_yield () from /usr/lib/libc.so.6
     * #1  0x00007ffff792c4a6 in __sanitizer::StopTheWorld (callback=<optimized out>, argument=<optimized out>) at /usr/src/debug/gcc/gcc/libsanitizer/sanitizer_common/sanitizer_stoptheworld_linux_libcdep.cpp:457
     * #2  0x00007ffff793f6f9 in __lsan::LockStuffAndStopTheWorldCallback (info=<optimized out>, size=<optimized out>, data=0x7fffffffde20) at /usr/src/debug/gcc/gcc/libsanitizer/lsan/lsan_common_linux.cpp:127
     * #3  0x00007ffff6977909 in dl_iterate_phdr () from /usr/lib/libc.so.6
     * #4  0x00007ffff793fb24 in __lsan::LockStuffAndStopTheWorld (callback=callback@entry=0x7ffff793d9d0 <__lsan::CheckForLeaksCallback(__sanitizer::SuspendedThreadsList const&, void*)>, argument=argument@entry=0x7fffffffdea0)
     *     at /usr/src/debug/gcc/gcc/libsanitizer/lsan/lsan_common_linux.cpp:142
     * #5  0x00007ffff793c965 in __lsan::CheckForLeaks () at /usr/src/debug/gcc/gcc/libsanitizer/lsan/lsan_common.cpp:778
     * #6  0x00007ffff793cc68 in __lsan::DoLeakCheck () at /usr/src/debug/gcc/gcc/libsanitizer/lsan/lsan_common.cpp:821
     * #7  0x00007ffff684e340 in __cxa_finalize () from /usr/lib/libc.so.6
     * #8  0x00007ffff7838c58 in __do_global_dtors_aux () from /usr/lib/libasan.so.8
     * #9  0x00007fffffffdfe0 in ?? ()
     *
     * Probably is something related to switching name spaces.
     * So, we kill -9 self.
     *
     */

    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "sanitizers detected, killing myself to avoid lockup");
    kill(getpid(), SIGKILL);
#endif

    if (WIFEXITED(status)) {
        exit(WEXITSTATUS(status));
    } else if (WIFSIGNALED(status)) {
        kill(getpid(), WTERMSIG(status));
    }

    exit(EXIT_FAILURE);
}

int proc_pid_fd(const char *prefix, const char *ns, pid_t pid) {
    if(!prefix) prefix = "";

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%d/%s", prefix, (int)pid, ns);
    int fd = open(filename, O_RDONLY | O_CLOEXEC);

    if(fd == -1)
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open proc_pid_fd() file '%s'", filename);

    return fd;
}

static struct ns {
    int nstype;
    int fd;
    int status;
    const char *name;
    const char *path;
} all_ns[] = {
        // { .nstype = CLONE_NEWUSER,   .fd = -1, .status = -1, .name = "user",    .path = "ns/user"   },
        // { .nstype = CLONE_NEWCGROUP, .fd = -1, .status = -1, .name = "cgroup",  .path = "ns/cgroup" },
        // { .nstype = CLONE_NEWIPC,    .fd = -1, .status = -1, .name = "ipc",     .path = "ns/ipc"    },
        // { .nstype = CLONE_NEWUTS,    .fd = -1, .status = -1, .name = "uts",     .path = "ns/uts"    },
        { .nstype = CLONE_NEWNET,    .fd = -1, .status = -1, .name = "network", .path = "ns/net"    },
        { .nstype = CLONE_NEWPID,    .fd = -1, .status = -1, .name = "pid",     .path = "ns/pid"    },
        { .nstype = CLONE_NEWNS,     .fd = -1, .status = -1, .name = "mount",   .path = "ns/mnt"    },

        // terminator
        { .nstype = 0,               .fd = -1, .status = -1, .name = NULL,      .path = NULL        }
};

static int switch_namespace(const char *prefix, pid_t pid) {
#ifdef HAVE_SETNS
    int i;
    for(i = 0; all_ns[i].name ; i++)
        all_ns[i].fd = proc_pid_fd(prefix, all_ns[i].path, pid);

    int root_fd = proc_pid_fd(prefix, "root", pid);
    int cwd_fd  = proc_pid_fd(prefix, "cwd", pid);

    setgroups(0, NULL);

    // 2 passes - found it at nsenter source code
    // this is related CLONE_NEWUSER functionality

    // This code cannot switch user namespace (it can all the other namespaces)
    // Fortunately, we don't need to switch user namespaces.

    int pass;
    for(pass = 0; pass < 2 ;pass++) {
        for(i = 0; all_ns[i].name ; i++) {
            if (all_ns[i].fd != -1 && all_ns[i].status == -1) {
                if(setns(all_ns[i].fd, all_ns[i].nstype) == -1) {
                    if(pass == 1) {
                        all_ns[i].status = 0;
                        nd_log(NDLS_COLLECTORS, NDLP_ERR,
                               "Cannot switch to %s namespace of pid %d",
                               all_ns[i].name, (int) pid);
                    }
                }
                else
                    all_ns[i].status = 1;
            }
        }
    }

    gettid_uncached();
    setgroups(0, NULL);

    if(root_fd != -1) {
        if(fchdir(root_fd) < 0)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot fchdir() to pid %d root directory", (int)pid);

        if(chroot(".") < 0)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot chroot() to pid %d root directory", (int)pid);

        close(root_fd);
    }

    if(cwd_fd != -1) {
        if(fchdir(cwd_fd) < 0)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot fchdir() to pid %d current working directory", (int)pid);

        close(cwd_fd);
    }

    int do_fork = 0;
    for(i = 0; all_ns[i].name ; i++)
        if(all_ns[i].fd != -1) {

            // CLONE_NEWPID requires a fork() to become effective
            if(all_ns[i].nstype == CLONE_NEWPID && all_ns[i].status)
                do_fork = 1;

            close(all_ns[i].fd);
        }

    if(do_fork)
        continue_as_child();

    return 0;

#else

    errno = ENOSYS;
    nd_log(NDLS_COLLECTORS, NDLP_ERR, "setns() is missing on this system.");
    return 1;
#endif
}

pid_t read_pid_from_cgroup_file(const char *filename) {
    int fd = open(filename, procfile_open_flags);
    if(fd == -1) {
        if (errno != ENOENT)
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot open pid_from_cgroup() file '%s'.", filename);
        return 0;
    }

    FILE *fp = fdopen(fd, "r");
    if(!fp) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot upgrade fd to fp for file '%s'.", filename);
        return 0;
    }

    char buffer[100 + 1];
    pid_t pid = 0;
    char *s;
    while((s = fgets(buffer, 100, fp))) {
        buffer[100] = '\0';
        pid = atoi(s);
        if(pid > 0) break;
    }

    fclose(fp);

    if(pid > 0)
        nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "found pid %d on file '%s'", pid, filename);

    return pid;
}

pid_t read_pid_from_cgroup_files(const char *path) {
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s/cgroup.procs", path);
    pid_t pid = read_pid_from_cgroup_file(filename);
    if(pid > 0) return pid;

    snprintfz(filename, FILENAME_MAX, "%s/tasks", path);
    return read_pid_from_cgroup_file(filename);
}

pid_t read_pid_from_cgroup(const char *path) {
    pid_t pid = read_pid_from_cgroup_files(path);
    if (pid > 0) return pid;

    DIR *dir = opendir(path);
    if (!dir) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot read directory '%s'", path);
        return 0;
    }

    struct dirent *de = NULL;
    while ((de = readdir(dir))) {
        if (de->d_type == DT_DIR
            && (
                    (de->d_name[0] == '.' && de->d_name[1] == '\0')
                    || (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0')
            ))
            continue;

        if (de->d_type == DT_DIR) {
            char filename[FILENAME_MAX + 1];
            snprintfz(filename, FILENAME_MAX, "%s/%s", path, de->d_name);
            pid = read_pid_from_cgroup(filename);
            if(pid > 0) break;
        }
    }
    closedir(dir);
    return pid;
}

// ----------------------------------------------------------------------------
// send the result to netdata

struct found_device {
    const char *host_device;
    const char *guest_device;

    uint32_t host_device_hash;

    struct found_device *next;
} *detected_devices = NULL;

void add_device(const char *host, const char *guest) {
    errno_clear();
    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "adding device with host '%s', guest '%s'", host, guest);

    uint32_t hash = simple_hash(host);

    if(guest && (!*guest || strcmp(host, guest) == 0))
        guest = NULL;

    struct found_device *f;
    for(f = detected_devices; f ; f = f->next) {
        if(f->host_device_hash == hash && !strcmp(host, f->host_device)) {

            if(guest && (!f->guest_device || !strcmp(f->host_device, f->guest_device))) {
                if(f->guest_device) freez((void *)f->guest_device);
                f->guest_device = strdupz(guest);
            }

            return;
        }
    }

    f = mallocz(sizeof(struct found_device));
    f->host_device = strdupz(host);
    f->host_device_hash = hash;
    f->guest_device = (guest)?strdupz(guest):NULL;
    f->next = detected_devices;
    detected_devices = f;
}

int send_devices(void) {
    int found = 0;

    struct found_device *f;
    for(f = detected_devices; f ; f = f->next) {
        found++;
        printf("%s %s\n", f->host_device, (f->guest_device)?f->guest_device:f->host_device);
    }

    return found;
}

// ----------------------------------------------------------------------------
// this function should be called only **ONCE**
// also it has to be the **LAST** to be called
// since it switches namespaces, so after this call, everything is different!

void detect_veth_interfaces(pid_t pid) {
    struct iface *cgroup = NULL;
    struct iface *host, *h, *c;

    host = read_proc_net_dev("host", netdata_configured_host_prefix);
    if(!host) {
        errno_clear();
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "no host interface list.");
        goto cleanup;
    }

    if(!eligible_ifaces(host)) {
        errno_clear();
        nd_log(NDLS_COLLECTORS, NDLP_WARNING, "no double-linked host interfaces available.");
        goto cleanup;
    }

    if(switch_namespace(netdata_configured_host_prefix, pid)) {
        errno_clear();
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot switch to the namespace of pid %u", (unsigned int) pid);
        goto cleanup;
    }

    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "switched to namespaces of pid %d", pid);

    cgroup = read_proc_net_dev("cgroup", NULL);
    if(!cgroup) {
        errno_clear();
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot read cgroup interface list.");
        goto cleanup;
    }

    if(!eligible_ifaces(cgroup)) {
        errno_clear();
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "there are not double-linked cgroup interfaces available.");
        goto cleanup;
    }

     unsigned int host_dev_num = calc_num_ifaces(host);
     unsigned int cgroup_dev_num = calc_num_ifaces(cgroup);
    // host ifaces == guest ifaces => we are still in the host namespace
    // and we can't really identify which ifaces belong to the cgroup (e.g. Proxmox VM).
    if (host_dev_num == cgroup_dev_num) {
        unsigned int m = 0;
        for (h = host; h; h = h->next) {
            for (c = cgroup; c; c = c->next) {
                if (h->ifindex == c->ifindex && h->iflink == c->iflink) {
                    m++;
                    break;
                }
            }
        }
        if (host_dev_num == m) {
            goto cleanup;
        }
    }

    for(h = host; h ; h = h->next) {
        if(iface_is_eligible(h)) {
            for (c = cgroup; c; c = c->next) {
                if(iface_is_eligible(c) && h->ifindex == c->iflink && h->iflink == c->ifindex) {
                    printf("%s %s\n", h->device, c->device);
                    // add_device(h->device, c->device);
                }
            }
        }
    }

    printf("EXIT DONE\n");
    fflush(stdout);

cleanup:
    free_host_ifaces(cgroup);
    free_host_ifaces(host);
}

struct send_to_spawned_process {
    pid_t pid;
    char host_prefix[FILENAME_MAX];
};


static int spawn_callback(SPAWN_REQUEST *request) {
    const struct send_to_spawned_process *d = request->data;
    detect_veth_interfaces(d->pid);
    return 0;
}

#define CGROUP_NETWORK_INTERFACE_MAX_LINE 2048
static void read_from_spawned(SPAWN_INSTANCE *si, const char *name __maybe_unused) {
    char buffer[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
    char *s;
    FILE *fp = fdopen(spawn_server_instance_read_fd(si), "r");
    while((s = fgets(buffer, CGROUP_NETWORK_INTERFACE_MAX_LINE, fp))) {
        trim(s);

        if(*s && *s != '\n') {
            char *t = s;
            while(*t && *t != ' ') t++;
            if(*t == ' ') {
                *t = '\0';
                t++;
            }

            if(strcmp(s, "EXIT") == 0)
                break;

            if(!*s || !*t) continue;
            add_device(s, t);
        }
    }
    fclose(fp);
    spawn_server_instance_read_fd_unset(si);
    spawn_server_exec_kill(spawn_server, si, 0);
}

void detect_veth_interfaces_spawn(pid_t pid) {
    struct send_to_spawned_process d = {
        .pid = pid,
    };
    strncpyz(d.host_prefix, netdata_configured_host_prefix, sizeof(d.host_prefix) - 1);
    SPAWN_INSTANCE *si = spawn_server_exec(spawn_server, STDERR_FILENO, 0, NULL, &d, sizeof(d), SPAWN_INSTANCE_TYPE_CALLBACK);
    if(si)
        read_from_spawned(si, "switch namespace callback");
    else
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cgroup-network cannot spawn switch namespace callback");
}

// ----------------------------------------------------------------------------
// call the external helper

#define CGROUP_NETWORK_INTERFACE_MAX_LINE 2048
void call_the_helper(pid_t pid, const char *cgroup) {
    char command[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
    if(cgroup)
        snprintfz(command, CGROUP_NETWORK_INTERFACE_MAX_LINE, "exec " PLUGINS_DIR "/cgroup-network-helper.sh --cgroup '%s'", cgroup);
    else
        snprintfz(command, CGROUP_NETWORK_INTERFACE_MAX_LINE, "exec " PLUGINS_DIR "/cgroup-network-helper.sh --pid %d", pid);

    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "running: %s", command);

    SPAWN_INSTANCE *si;

    if(cgroup) {
        const char *argv[] = {
            PLUGINS_DIR "/cgroup-network-helper.sh",
            "--cgroup",
            cgroup,
            NULL,
        };
        si = spawn_server_exec(spawn_server, nd_log_collectors_fd(), 0, argv, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    }
    else {
        char buffer[100];
        snprintfz(buffer, sizeof(buffer) - 1, "%d", pid);
        const char *argv[] = {
            PLUGINS_DIR "/cgroup-network-helper.sh",
            "--pid",
            buffer,
            NULL,
        };
        si = spawn_server_exec(spawn_server, nd_log_collectors_fd(), 0, argv, NULL, 0, SPAWN_INSTANCE_TYPE_EXEC);
    }

    if(si)
        read_from_spawned(si, command);
    else
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot execute cgroup-network helper script: %s", command);
}

int ishex(char c) {
    return (c >= '0' && c <= '9') ||
           (c >= 'a' && c <= 'f') ||
           (c >= 'A' && c <= 'F');
}

int is_valid_hex_escape(const char *arg) {
    fatal_assert(arg);

    return (arg[0] == '\\') &&
           (arg[1] == 'x') &&
           ishex(arg[2]) &&
           ishex(arg[3]);
}

int is_valid_path_symbol(char c) {
    switch(c) {
        case '/':   // path separators
        case ' ':   // space
        case '-':   // hyphen
        case '_':   // underscore
        case '.':   // dot
        case ',':   // comma
        case '@':   // systemd unit template specifier (/sys/fs/cgroup/machines.slice/systemd-nspawn@NAME.service)
            return 1;

        default:
            return 0;
    }
}

// we will pass this path a shell script running as root
// so, we need to make sure the path will be valid
// and will not include anything that could allow
// the caller use shell expansion for gaining escalated
// privileges.
int verify_path(const char *path) {
    struct stat sb;

    fatal_assert(path);

    const char *s = path;
    while(*s != '\0') {
        if (isalnum(*s) || is_valid_path_symbol(*s))
            s += 1;
        else if (*s == '\\' && is_valid_hex_escape(s))
            s += 4;
        else {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "invalid character in path '%s'", path);
            return -1;
        }
    }

    if(strstr(path, "/../")) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "invalid parent path sequence detected in '%s'", path);
        return 1;
    }

    if(path[0] != '/') {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "only absolute path names are supported - invalid path '%s'", path);
        return -1;
    }

    if (stat(path, &sb) == -1) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "cannot stat() path '%s'", path);
        return -1;
    }

    if((sb.st_mode & S_IFMT) != S_IFDIR) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "path '%s' is not a directory", path);
        return -1;
    }

    return 0;
}

/*
char *fix_path_variable(void) {
    const char *path = getenv("PATH");
    if(!path || !*path) return 0;

    char *p = strdupz(path);
    char *safe_path = callocz(1, strlen(p) + strlen("PATH=") + 1);
    strcpy(safe_path, "PATH=");

    int added = 0;
    char *ptr = p;
    while(ptr && *ptr) {
        char *s = strsep(&ptr, ":");
        if(s && *s) {
            if(verify_path(s) == -1) {
                nd_log(NDLS_COLLECTORS, NDLP_ERR, "the PATH variable includes an invalid path '%s' - removed it.", s);
            }
            else {
                nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "the PATH variable includes a valid path '%s'.", s);
                if(added) strcat(safe_path, ":");
                strcat(safe_path, s);
                added++;
            }
        }
    }

    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "unsafe PATH:      '%s'.", path);
    nd_log(NDLS_COLLECTORS, NDLP_DEBUG, "  safe PATH: '%s'.", safe_path);

    freez(p);
    return safe_path;
}
*/

// ----------------------------------------------------------------------------
// main

static void cleanup_spawn_server_on_fatal(void) {
    if(spawn_server) {
        spawn_server_destroy(spawn_server);
        spawn_server = NULL;
    }
}

void usage(void) {
    fprintf(stderr, "%s [ -p PID | --pid PID | --cgroup /path/to/cgroup ]\n", program_name);
    exit(1);
}

int main(int argc, const char **argv) {
    pid_t pid = 0;

    if (setresuid(0, 0, 0) == -1)
        collector_error("setresuid(0, 0, 0) failed.");

    nd_log_initialize_for_external_plugins("cgroup-network");
    spawn_server = spawn_server_create(SPAWN_SERVER_OPTION_EXEC | SPAWN_SERVER_OPTION_CALLBACK, NULL, spawn_callback, argc, argv);
    nd_log_register_fatal_final_cb(cleanup_spawn_server_on_fatal);

    // since cgroup-network runs as root, prevent it from opening symbolic links
    procfile_open_flags = O_RDONLY|O_NOFOLLOW;

    // ------------------------------------------------------------------------
    // make sure NETDATA_HOST_PREFIX is safe

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix(false) == -1) exit(1);

    if(netdata_configured_host_prefix[0] != '\0' && verify_path(netdata_configured_host_prefix) == -1)
        fatal("invalid NETDATA_HOST_PREFIX '%s'", netdata_configured_host_prefix);

    // ------------------------------------------------------------------------
    // build a safe environment for our script

    // the first environment variable is a fixed PATH=
    snprintfz(env_netdata_host_prefix, sizeof(env_netdata_host_prefix) - 1, "NETDATA_HOST_PREFIX=%s", netdata_configured_host_prefix);

    char *s;

    s = getenv("NETDATA_LOG_METHOD");
    snprintfz(env_netdata_log_method, sizeof(env_netdata_log_method) - 1, "NETDATA_LOG_METHOD=%s", nd_log_method_for_external_plugins(s));

    s = getenv("NETDATA_LOG_FORMAT");
    if (s)
        snprintfz(env_netdata_log_format, sizeof(env_netdata_log_format) - 1, "NETDATA_LOG_FORMAT=%s", s);

    s = getenv("NETDATA_LOG_LEVEL");
    if (s)
        snprintfz(env_netdata_log_level, sizeof(env_netdata_log_level) - 1, "NETDATA_LOG_LEVEL=%s", s);

    // ------------------------------------------------------------------------

    if(argc == 2 && (!strcmp(argv[1], "version") || !strcmp(argv[1], "-version") || !strcmp(argv[1], "--version") || !strcmp(argv[1], "-v") || !strcmp(argv[1], "-V"))) {
        fprintf(stderr, "cgroup-network %s\n", NETDATA_VERSION);
        exit(0);
    }

    if(argc != 3)
        usage();

    int arg = 1;
    int helper = 1;
    if (getenv("KUBERNETES_SERVICE_HOST") != NULL && getenv("KUBERNETES_SERVICE_PORT") != NULL)
        helper = 0;

    if(!strcmp(argv[arg], "-p") || !strcmp(argv[arg], "--pid")) {
        pid = atoi(argv[arg+1]);

        if(pid <= 0) {
            errno_clear();
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Invalid pid %d given", (int) pid);
            return 2;
        }

        if(helper) call_the_helper(pid, NULL);
    }
    else if(!strcmp(argv[arg], "--cgroup")) {
        const char *cgroup = argv[arg+1];
        if(verify_path(cgroup) == -1) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "cgroup '%s' does not exist or is not valid.", cgroup);
            return 1;
        }

        pid = read_pid_from_cgroup(cgroup);
        if(helper) call_the_helper(pid, cgroup);

        if(pid <= 0 && !detected_devices) {
            errno_clear();
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "Cannot find a cgroup PID from cgroup '%s'", cgroup);
        }
    }
    else
        usage();

    if(pid > 0)
        detect_veth_interfaces_spawn(pid);

    int found = send_devices();

    spawn_server_destroy(spawn_server);
    spawn_server = NULL;

    if(found <= 0) return 1;
    return 0;
}
