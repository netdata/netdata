#include "common.h"
#include <libgen.h>

#ifdef HAVE_SETNS
#ifndef _GNU_SOURCE
#define _GNU_SOURCE             /* See feature_test_macros(7) */
#endif
#include <sched.h>
#endif

// ----------------------------------------------------------------------------
// callback required by fatal()

void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}
void health_reload(void) {};
void rrdhost_save_all(void) {};


struct iface {
    const char *device;
    uint32_t hash;

    unsigned int ifindex;
    unsigned int iflink;

    struct iface *next;
};

unsigned int read_iface_iflink(const char *prefix, const char *iface) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/iflink", prefix?prefix:"", iface);

    unsigned long long iflink = 0;
    int ret = read_single_number_file(filename, &iflink);
    if(ret) error("Cannot read '%s'.", filename);

    return (unsigned int)iflink;
}

unsigned int read_iface_ifindex(const char *prefix, const char *iface) {
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/sys/class/net/%s/ifindex", prefix?prefix:"", iface);

    unsigned long long ifindex = 0;
    int ret = read_single_number_file(filename, &ifindex);
    if(ret) error("Cannot read '%s'.", filename);

    return (unsigned int)ifindex;
}

struct iface *read_proc_net_dev(const char *prefix) {
    procfile *ff = NULL;
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, FILENAME_MAX, "%s%s", prefix?prefix:"", "/proc/net/dev");
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
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/proc/%d/%s", prefix?prefix:"", (int)pid, ns);
    int fd = open(filename, O_RDONLY);

    if(fd == -1)
        error("Cannot open file '%s'", filename);

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

    // FIXME: this code cannot switch user namespace
    // Fortunately, we don't need it.

    int pass, errors = 0;
    for(pass = 0; pass < 2 ;pass++) {
        for(i = 0; all_ns[i].name ; i++) {
            if (all_ns[i].fd != -1 && all_ns[i].status == -1) {
                if(setns(all_ns[i].fd, all_ns[i].nstype) == -1) {
                    if(pass == 1) {
                        all_ns[i].status = 0;
                        error("Cannot switch to %s namespace of pid %d", all_ns[i].name, (int) pid);
                        errors++;
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
    FILE *fp = fopen(filename, "r");
    if(!fp) {
        error("Cannot read file '%s'.", filename);
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
    uint32_t hash = simple_hash(host);

    if(guest && (!*guest || strcmp(host, guest) == 0))
        guest = NULL;

    struct found_device *f;
    for(f = detected_devices; f ; f = f->next) {
        if(f->host_device_hash == hash && strcmp(host, f->host_device) == 0) {

            if(guest && !f->guest_device)
                f->guest_device = strdupz(guest);

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
    struct iface *host, *cgroup, *h, *c;
    const char *prefix = getenv("NETDATA_HOST_PREFIX");

    host = read_proc_net_dev(prefix);
    if(!host) {
        errno = 0;
        error("cannot read host interface list.");
        return;
    }

    if(!eligible_ifaces(host)) {
        errno = 0;
        error("there are no double-linked host interfaces available.");
        goto cleanup;
    }

    if(switch_namespace(prefix, pid)) {
        errno = 0;
        error("cannot switch to the namespace of pid %u", (unsigned int) pid);
        goto cleanup;
    }

    cgroup = read_proc_net_dev(NULL);
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
    free_host_ifaces(host);
}

// ----------------------------------------------------------------------------
// call the external helper

#define CGROUP_NETWORK_INTERFACE_MAX_LINE 2048
void call_the_helper(const char *me, pid_t pid, const char *cgroup) {
    const char *pluginsdir = getenv("NETDATA_PLUGINS_DIR");
    char *m = NULL;

    if(!pluginsdir || !*pluginsdir) {
        m = strdupz(me);
        pluginsdir = dirname(m);
    }

    if(setresuid(0, 0, 0) == -1)
        error("setresuid(0, 0, 0) failed.");

    char buffer[CGROUP_NETWORK_INTERFACE_MAX_LINE + 1];
    if(cgroup)
        snprintfz(buffer, CGROUP_NETWORK_INTERFACE_MAX_LINE, "exec %s/cgroup-network-helper.sh --cgroup '%s'", pluginsdir, cgroup);
    else
        snprintfz(buffer, CGROUP_NETWORK_INTERFACE_MAX_LINE, "exec %s/cgroup-network-helper.sh --pid %d", pluginsdir, pid);

    info("running: %s", buffer);

    pid_t cgroup_pid;
    FILE *fp = mypopen(buffer, &cgroup_pid);
    if(fp) {
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
        error("cannot execute cgroup-network helper script: %s", buffer);

    freez(m);
}


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

        call_the_helper(argv[0], pid, NULL);
    }
    else if(!strcmp(argv[1], "--cgroup")) {
        pid = read_pid_from_cgroup(argv[2]);
        call_the_helper(argv[0], pid, argv[2]);

        if(pid <= 0 && !detected_devices) {
            errno = 0;
            error("Cannot find a cgroup PID from cgroup '%s'", argv[2]);
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
