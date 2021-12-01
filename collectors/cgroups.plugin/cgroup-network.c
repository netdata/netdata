// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

#ifdef HAVE_SETNS
#ifndef _GNU_SOURCE
#define _GNU_SOURCE             /* See feature_test_macros(7) */
#endif
#include <sched.h>
#endif

char environment_variable2[FILENAME_MAX + 50] = "";
char *environment[] = {
        "PATH=/bin:/sbin:/usr/bin:/usr/sbin:/usr/local/bin:/usr/local/sbin",
        environment_variable2,
        NULL
};


// ----------------------------------------------------------------------------

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    (void) action;
    (void) action_result;
    (void) action_data;
    return;
}

// callbacks required by popen()
void signals_block(void) {};
void signals_unblock(void) {};
void signals_reset(void) {};

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// ----------------------------------------------------------------------------

struct iface {
    const char *device;
    uint32_t hash;

    unsigned int ifindex;
    unsigned int iflink;

    struct iface *next;
};

unsigned int read_iface_iflink(const char *prefix, const char *iface) {
    if(!prefix) prefix = "";

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/iflink", prefix, iface);

    unsigned long long iflink = 0;
    int ret = read_single_number_file(filename, &iflink);
    if(ret) error("Cannot read '%s'.", filename);

    return (unsigned int)iflink;
}

unsigned int read_iface_ifindex(const char *prefix, const char *iface) {
    if(!prefix) prefix = "";

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/ifindex", prefix, iface);

    unsigned long long ifindex = 0;
    int ret = read_single_number_file(filename, &ifindex);
    if(ret) error("Cannot read '%s'.", filename);

    return (unsigned int)ifindex;
}

struct iface *read_proc_net_dev(const char *scope __maybe_unused, const char *prefix) {
    if(!prefix) prefix = "";

    procfile *ff = NULL;
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", prefix, (*prefix)?"/proc/1/net/dev":"/proc/net/dev");

#ifdef NETDATA_INTERNAL_CHECKS
    info("parsing '%s'", filename);
#endif

    ff = procfile_open(filename, " \t,:|", PROCFILE_FLAG_DEFAULT);
    if(unlikely(!ff)) {
        error("Cannot open file '%s'", filename);
        return NULL;
    }

    ff = procfile_readall(ff);
    if(unlikely(!ff)) {
        error("Cannot read file '%s'", filename);
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

#ifdef NETDATA_INTERNAL_CHECKS
        info("added %s interface '%s', ifindex %u, iflink %u", scope, t->device, t->ifindex, t->iflink);
#endif
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

    if (child < 0)
        error("fork() failed");

    /* Only the child returns */
    if (child == 0)
        return;

    for (;;) {
        ret = waitpid(child, &status, WUNTRACED);
        if ((ret == child) && (WIFSTOPPED(status))) {
            /* The child suspended so suspend us as well */
            kill(getpid(), SIGSTOP);
            kill(child, SIGCONT);
        } else {
            break;
        }
    }

    /* Return the child's exit code if possible */
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
    int fd = open(filename, O_RDONLY);

    if(fd == -1)
        error("Cannot open proc_pid_fd() file '%s'", filename);

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

int switch_namespace(const char *prefix, pid_t pid) {

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
                        error("Cannot switch to %s namespace of pid %d", all_ns[i].name, (int) pid);
                    }
                }
                else
                    all_ns[i].status = 1;
            }
        }
    }

    setgroups(0, NULL);

    if(root_fd != -1) {
        if(fchdir(root_fd) < 0)
            error("Cannot fchdir() to pid %d root directory", (int)pid);

        if(chroot(".") < 0)
            error("Cannot chroot() to pid %d root directory", (int)pid);

        close(root_fd);
    }

    if(cwd_fd != -1) {
        if(fchdir(cwd_fd) < 0)
            error("Cannot fchdir() to pid %d current working directory", (int)pid);

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
    error("setns() is missing on this system.");
    return 1;

#endif
}

pid_t read_pid_from_cgroup_file(const char *filename) {
    int fd = open(filename, procfile_open_flags);
    if(fd == -1) {
        error("Cannot open pid_from_cgroup() file '%s'.", filename);
        return 0;
    }

    FILE *fp = fdopen(fd, "r");
    if(!fp) {
        error("Cannot upgrade fd to fp for file '%s'.", filename);
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

#ifdef NETDATA_INTERNAL_CHECKS
    if(pid > 0) info("found pid %d on file '%s'", pid, filename);
#endif

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
        error("cannot read directory '%s'", path);
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
#ifdef NETDATA_INTERNAL_CHECKS
    info("adding device with host '%s', guest '%s'", host, guest);
#endif

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
        errno = 0;
        error("cannot read host interface list.");
        goto cleanup;
    }

    if(!eligible_ifaces(host)) {
        errno = 0;
        info("there are no double-linked host interfaces available.");
        goto cleanup;
    }

    if(switch_namespace(netdata_configured_host_prefix, pid)) {
        errno = 0;
        error("cannot switch to the namespace of pid %u", (unsigned int) pid);
        goto cleanup;
    }

#ifdef NETDATA_INTERNAL_CHECKS
    info("switched to namespaces of pid %d", pid);
#endif

    cgroup = read_proc_net_dev("cgroup", NULL);
    if(!cgroup) {
        errno = 0;
        error("cannot read cgroup interface list.");
        goto cleanup;
    }

    if(!eligible_ifaces(cgroup)) {
        errno = 0;
        error("there are not double-linked cgroup interfaces available.");
        goto cleanup;
    }

    for(h = host; h ; h = h->next) {
        if(iface_is_eligible(h)) {
            for (c = cgroup; c; c = c->next) {
                if(iface_is_eligible(c) && h->ifindex == c->iflink && h->iflink == c->ifindex) {
                    add_device(h->device, c->device);
                }
            }
        }
    }

cleanup:
    free_host_ifaces(cgroup);
    free_host_ifaces(host);
}

// ----------------------------------------------------------------------------
// call the external helper

#define CGROUP_NETWORK_INTERFACE_MAX_LINE 2048
void call_the_helper(pid_t pid, const char *cgroup) {
    if(setresuid(0, 0, 0) == -1)
        error("setresuid(0, 0, 0) failed.");

    char command[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
    if(cgroup)
        snprintfz(command, CGROUP_NETWORK_INTERFACE_MAX_LINE, "exec " PLUGINS_DIR "/cgroup-network-helper.sh --cgroup '%s'", cgroup);
    else
        snprintfz(command, CGROUP_NETWORK_INTERFACE_MAX_LINE, "exec " PLUGINS_DIR "/cgroup-network-helper.sh --pid %d", pid);

    info("running: %s", command);

    pid_t cgroup_pid;
    FILE *fp = mypopene(command, &cgroup_pid, environment);
    if(fp) {
        char buffer[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
        char *s;
        while((s = fgets(buffer, CGROUP_NETWORK_INTERFACE_MAX_LINE, fp))) {
            trim(s);

            if(*s && *s != '\n') {
                char *t = s;
                while(*t && *t != ' ') t++;
                if(*t == ' ') {
                    *t = '\0';
                    t++;
                }

                if(!*s || !*t) continue;
                add_device(s, t);
            }
        }

        mypclose(fp, cgroup_pid);
    }
    else
        error("cannot execute cgroup-network helper script: %s", command);
}

int is_valid_path_symbol(char c) {
    switch(c) {
        case '/':   // path separators
        case '\\':  // needed for virsh domains \x2d1\x2dname
        case ' ':   // space
        case '-':   // hyphen
        case '_':   // underscore
        case '.':   // dot
        case ',':   // comma
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

    char c;
    const char *s = path;
    while((c = *s++)) {
        if(!( isalnum(c) || is_valid_path_symbol(c) )) {
            error("invalid character in path '%s'", path);
            return -1;
        }
    }

    if(strstr(path, "\\") && !strstr(path, "\\x")) {
        error("invalid escape sequence in path '%s'", path);
        return 1;
    }

    if(strstr(path, "/../")) {
        error("invalid parent path sequence detected in '%s'", path);
        return 1;
    }

    if(path[0] != '/') {
        error("only absolute path names are supported - invalid path '%s'", path);
        return -1;
    }

    if (stat(path, &sb) == -1) {
        error("cannot stat() path '%s'", path);
        return -1;
    }

    if((sb.st_mode & S_IFMT) != S_IFDIR) {
        error("path '%s' is not a directory", path);
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
                error("the PATH variable includes an invalid path '%s' - removed it.", s);
            }
            else {
                info("the PATH variable includes a valid path '%s'.", s);
                if(added) strcat(safe_path, ":");
                strcat(safe_path, s);
                added++;
            }
        }
    }

    info("unsafe PATH:      '%s'.", path);
    info("  safe PATH: '%s'.", safe_path);

    freez(p);
    return safe_path;
}
*/

// ----------------------------------------------------------------------------
// main

void usage(void) {
    fprintf(stderr, "%s [ -p PID | --pid PID | --cgroup /path/to/cgroup ]\n", program_name);
    exit(1);
}

int main(int argc, char **argv) {
    pid_t pid = 0;

    program_name = argv[0];
    program_version = VERSION;
    error_log_syslog = 0;

    // since cgroup-network runs as root, prevent it from opening symbolic links
    procfile_open_flags = O_RDONLY|O_NOFOLLOW;

    // ------------------------------------------------------------------------
    // make sure NETDATA_HOST_PREFIX is safe

    netdata_configured_host_prefix = getenv("NETDATA_HOST_PREFIX");
    if(verify_netdata_host_prefix() == -1) exit(1);

    if(netdata_configured_host_prefix[0] != '\0' && verify_path(netdata_configured_host_prefix) == -1)
        fatal("invalid NETDATA_HOST_PREFIX '%s'", netdata_configured_host_prefix);

    // ------------------------------------------------------------------------
    // build a safe environment for our script

    // the first environment variable is a fixed PATH=
    snprintfz(environment_variable2, sizeof(environment_variable2) - 1, "NETDATA_HOST_PREFIX=%s", netdata_configured_host_prefix);

    // ------------------------------------------------------------------------

    if(argc == 2 && (!strcmp(argv[1], "version") || !strcmp(argv[1], "-version") || !strcmp(argv[1], "--version") || !strcmp(argv[1], "-v") || !strcmp(argv[1], "-V"))) {
        fprintf(stderr, "cgroup-network %s\n", VERSION);
        exit(0);
    }

    if(argc != 3)
        usage();

    if(!strcmp(argv[1], "-p") || !strcmp(argv[1], "--pid")) {
        pid = atoi(argv[2]);

        if(pid <= 0) {
            errno = 0;
            error("Invalid pid %d given", (int) pid);
            return 2;
        }

        call_the_helper(pid, NULL);
    }
    else if(!strcmp(argv[1], "--cgroup")) {
        char *cgroup = argv[2];
        if(verify_path(cgroup) == -1) {
            error("cgroup '%s' does not exist or is not valid.", cgroup);
            return 1;
        }

        pid = read_pid_from_cgroup(cgroup);
        call_the_helper(pid, cgroup);

        if(pid <= 0 && !detected_devices) {
            errno = 0;
            error("Cannot find a cgroup PID from cgroup '%s'", cgroup);
        }
    }
    else
        usage();

    if(pid > 0)
        detect_veth_interfaces(pid);

    int found = send_devices();
    if(found <= 0) return 1;
    return 0;
}
