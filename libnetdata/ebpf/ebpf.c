#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>

#include "../libnetdata.h"

static int clean_kprobe_event(FILE *out, char *filename, char *father_pid, netdata_ebpf_events_t *ptr) {
    int fd =  open(filename, O_WRONLY | O_APPEND, 0);
    if (fd < 0) {
        fprintf(out, "Cannot open %s : %s\n", filename, strerror(errno));
        return 1;
    }

    char cmd[1024];
    int length = sprintf(cmd, "-:kprobes/%c_netdata_%s_%s", ptr->type, ptr->name, father_pid);
    int ret = 0;
    if (length > 0) {
        ssize_t written = write(fd, cmd, strlen(cmd));
        if (written < 0) {
            fprintf(out, "Cannot remove the event (%d, %d) '%s' from %s : %s\n", getppid(), getpid(), cmd, filename, strerror((int)errno));
            ret = 1;
        }
    }

    close(fd);

    return ret;
}

int clean_kprobe_events(FILE *out, int pid, netdata_ebpf_events_t *ptr) {
    //
    debug(D_EXIT, "R.I.P. dad I will clean your events.");
    char filename[FILENAME_MAX +1];
    snprintf(filename, FILENAME_MAX, "%s%s", NETDATA_DEBUGFS, "kprobe_events");

    char removeme[16];
    snprintf(removeme, 15,"%d", pid);

    int i;
    for (i = 0 ; ptr[i].name ; i++) {
        if (clean_kprobe_event(out, filename, removeme, &ptr[i])) {
            break;
        }
    }

    return 0;
}
