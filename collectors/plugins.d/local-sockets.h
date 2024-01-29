// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOCAL_SOCKETS_H
#define NETDATA_LOCAL_SOCKETS_H

#include "libnetdata/libnetdata.h"

struct local_socket;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct local_socket
#define SIMPLE_HASHTABLE_NAME _LOCAL_SOCKET
#include "libnetdata/simple_hashtable.h"

union ipv46;
#define SIMPLE_HASHTABLE_VALUE_TYPE union ipv46
#define SIMPLE_HASHTABLE_NAME _LOCAL_IP
#include "libnetdata/simple_hashtable.h"

struct local_port;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct local_port
#define SIMPLE_HASHTABLE_NAME _LOCAL_PORT
#include "libnetdata/simple_hashtable.h"

struct local_socket_state;
typedef void (*local_sockets_cb_t)(struct local_socket_state *state, struct local_socket *n, void *data);

typedef struct local_socket_state {
    struct {
        bool listening;
        bool inbound;
        bool outbound;
        bool local;
        bool tcp4;
        bool tcp6;
        bool udp4;
        bool udp6;
        bool pid;
        bool cmdline;
        bool comm;
        size_t max_errors;

        local_sockets_cb_t cb;
        void *data;
    } config;

    struct {
        size_t pid_fds_processed;
        size_t pid_fds_failed;
        size_t errors_encountered;
    } stats;

    SIMPLE_HASHTABLE_LOCAL_SOCKET sockets_hashtable;
    SIMPLE_HASHTABLE_LOCAL_IP local_ips_hashtable;
    SIMPLE_HASHTABLE_LOCAL_PORT listening_ports_hashtable;
} LS_STATE;

// --------------------------------------------------------------------------------------------------------------------

typedef enum __attribute__((packed)) {
    SOCKET_DIRECTION_LISTEN = (1 << 0),     // a listening socket
    SOCKET_DIRECTION_INBOUND = (1 << 1),    // an inbound socket connecting a remote system to a local listening socket
    SOCKET_DIRECTION_OUTBOUND = (1 << 2),   // a socket initiated by this system, connecting to another system
    SOCKET_DIRECTION_LOCAL = (1 << 3),      // the socket connecting 2 localhost applications
} SOCKET_DIRECTION;

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

union ipv46 {
    uint32_t ipv4;
    struct in6_addr ipv6;
};

struct local_port {
    uint16_t protocol;
    uint16_t family;
    uint16_t port;
};

struct socket_endpoint {
    uint16_t port;
    union ipv46 ip;
};

static inline void ipv6_to_in6_addr(const char *ipv6_str, struct in6_addr *d) {
    char buf[9];

    for (size_t k = 0; k < 4; ++k) {
        memcpy(buf, ipv6_str + (k * 8), 8);
        buf[sizeof(buf) - 1] = '\0';
        d->s6_addr32[k] = strtoul(buf, NULL, 16);
    }
}

typedef struct local_socket {
    unsigned int inode;

    uint16_t protocol;
    uint16_t family;
    int state;
    struct socket_endpoint local;
    struct socket_endpoint remote;
    pid_t pid;

    SOCKET_DIRECTION direction;

    char comm[TASK_COMM_LEN];
    char *cmdline;

    struct local_port local_port_key;

    XXH64_hash_t local_ip_hash;
    XXH64_hash_t remote_ip_hash;
    XXH64_hash_t local_port_hash;
} LOCAL_SOCKET;

// --------------------------------------------------------------------------------------------------------------------

static inline void ll_log(LS_STATE *ls, const char *format, ...) __attribute__ ((format(__printf__, 2, 3)));
static inline void ll_log(LS_STATE *ls, const char *format, ...) {
    if(++ls->stats.errors_encountered >= ls->config.max_errors)
        return;

    char buf[16384];
    va_list args;
    va_start(args, format);
    vsnprintf(buf, sizeof(buf), format, args);
    va_end(args);

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-LISTENERS: %s", buf);
}

// --------------------------------------------------------------------------------------------------------------------

static void foreach_local_socket_call_cb_and_cleanup(LS_STATE *ls) {
    for (unsigned int i = 0; i < ls->sockets_hashtable.size; i++) {
        SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = &ls->sockets_hashtable.hashtable[i];
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(!n) continue;

        if((ls->config.listening && n->direction & SOCKET_DIRECTION_LISTEN) ||
            (ls->config.local && n->direction & SOCKET_DIRECTION_LOCAL) ||
            (ls->config.inbound && n->direction & SOCKET_DIRECTION_INBOUND) ||
            (ls->config.outbound && n->direction & SOCKET_DIRECTION_OUTBOUND)
        ) {
            // we have to call the callback for this socket
            if (ls->config.cb)
                ls->config.cb(ls, n, ls->config.data);
        }

        freez(n->cmdline);
        freez(n);
    }
}

// --------------------------------------------------------------------------------------------------------------------

static inline void fix_cmdline(char* str) {
    char *s = str;

    // map invalid characters to underscores
    while(*s) {
        if(*s == '|' || iscntrl(*s)) *s = '_';
        s++;
    }
}

static inline bool associate_inode_with_pid(LS_STATE *ls, unsigned int inode, pid_t pid) {
    SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, inode, &inode, false);
    LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(!n) return false;

    n->pid = pid;

    if(ls->config.cmdline || ls->config.comm) {
        char cmdline[8192] = "";
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s/proc/%d/cmdline", netdata_configured_host_prefix, pid);

        if(ls->config.cmdline) {
            if (read_proc_cmdline(filename, cmdline, sizeof(cmdline)))
                ll_log(ls, "cannot open file: %s\n", filename);
            else {
                fix_cmdline(cmdline);

                char *s = trim(cmdline);

                if(s) {
                    // replace it
                    freez(n->cmdline);
                    n->cmdline = strdupz(s);
                }
            }
        }

        if(ls->config.comm) {
            n->comm[0] = '\0';
            snprintfz(filename, FILENAME_MAX, "%s/proc/%d/comm", netdata_configured_host_prefix, pid);
            if (read_txt_file(filename, n->comm, sizeof(n->comm)))
                ll_log(ls, "cannot open file: %s\n", filename);
            else {
                size_t len = strlen(n->comm);
                if(n->comm[len - 1] == '\n')
                    n->comm[len - 1] = '\0';
            }
        }
    }

    return true;
}

// ----------------------------------------------------------------------------

static inline bool find_all_sockets_in_proc(LS_STATE *ls, const char *proc_filename) {
    DIR *proc_dir, *fd_dir;
    struct dirent *proc_entry, *fd_entry;
    char path_buffer[FILENAME_MAX + 1];

    proc_dir = opendir(proc_filename);
    if (proc_dir == NULL) {
        ll_log(ls, "cannot opendir() '%s'", proc_filename);
        ls->stats.pid_fds_failed++;
        return false;
    }

    while ((proc_entry = readdir(proc_dir)) != NULL) {
        // Check if directory entry is a PID by seeing if the name is made up of digits only
        int is_pid = 1;
        for (char *c = proc_entry->d_name; *c != '\0'; c++) {
            if (*c < '0' || *c > '9') {
                is_pid = 0;
                break;
            }
        }

        if (!is_pid)
            continue;

        // Build the path to the fd directory of the process
        snprintfz(path_buffer, FILENAME_MAX, "%s/%s/fd/", proc_filename, proc_entry->d_name);

        fd_dir = opendir(path_buffer);
        if (fd_dir == NULL) {
            ll_log(ls, "cannot opendir() '%s'", path_buffer);

            ls->stats.pid_fds_failed++;
            continue;
        }

        while ((fd_entry = readdir(fd_dir)) != NULL) {
            if(!strcmp(fd_entry->d_name, ".") || !strcmp(fd_entry->d_name, ".."))
                continue;

            char link_path[FILENAME_MAX + 1];
            char link_target[FILENAME_MAX + 1];
            unsigned inode;

            // Build the path to the file descriptor link
            snprintfz(link_path, FILENAME_MAX, "%s/%s", path_buffer, fd_entry->d_name);

            ssize_t len = readlink(link_path, link_target, sizeof(link_target) - 1);
            if (len == -1) {
                ll_log(ls, "cannot read link '%s'", link_path);

                ls->stats.pid_fds_failed++;
                continue;
            }
            link_target[len] = '\0';

            ls->stats.pid_fds_processed++;

            // If the link target indicates a socket, print its inode number
            if (sscanf(link_target, "socket:[%u]", &inode) == 1)
                associate_inode_with_pid(ls, inode, (pid_t)strtoul(proc_entry->d_name, NULL, 10));
        }

        closedir(fd_dir);
    }

    closedir(proc_dir);
    return true;
}

// ----------------------------------------------------------------------------

static bool is_ipv4_mapped_ipv6_address(const struct in6_addr *addr) {
    // An IPv4-mapped IPv6 address starts with 80 bits of zeros followed by 16 bits of ones
    static const unsigned char ipv4_mapped_prefix[12] = { 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF };
    return memcmp(addr->s6_addr, ipv4_mapped_prefix, 12) == 0;
}

static bool is_loopback_address(const void *ip, uint16_t family) {
    if (family == AF_INET) {
        // For IPv4, loopback addresses are in the 127.0.0.0/8 range
        const uint32_t addr = ntohl(*((const uint32_t *)ip)); // Convert to host byte order for comparison
        return (addr >> 24) == 127; // Check if the first byte is 127
    } else if (family == AF_INET6) {
        // Check if the address is an IPv4-mapped IPv6 address
        const struct in6_addr *ipv6_addr = (const struct in6_addr *)ip;
        if (is_ipv4_mapped_ipv6_address(ipv6_addr)) {
            // Extract the last 32 bits (IPv4 address) and check if it's in the 127.0.0.0/8 range
            const uint32_t ipv4_addr = ntohl(*((const uint32_t *)(ipv6_addr->s6_addr + 12)));
            return (ipv4_addr >> 24) == 127;
        }

        // For IPv6, loopback address is ::1
        const struct in6_addr loopback_ipv6 = IN6ADDR_LOOPBACK_INIT;
        return memcmp(ipv6_addr, &loopback_ipv6, sizeof(struct in6_addr)) == 0;
    }

    return false;
}

static bool is_zero_address(const void *ip, uint16_t family) {
    if (family == AF_INET) {
        // For IPv4, check if the address is not 0.0.0.0
        const uint32_t zero_ipv4 = 0; // Zero address in network byte order
        return memcmp(ip, &zero_ipv4, sizeof(uint32_t)) == 0;
    } else if (family == AF_INET6) {
        // For IPv6, check if the address is not ::
        const struct in6_addr zero_ipv6 = IN6ADDR_ANY_INIT;
        return memcmp(ip, &zero_ipv6, sizeof(struct in6_addr)) == 0;
    }

    return false;
}

static inline bool read_proc_net_x(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
    if(family != AF_INET && family != AF_INET6)
        return false;

    FILE *fp;
    char *line = NULL;
    size_t len = 0;
    ssize_t read;

    fp = fopen(filename, "r");
    if (fp == NULL)
        return false;

    ssize_t min_line_length = (family == AF_INET) ? 105 : 155;
    size_t counter = 0;

    // Read line by line
    while ((read = getline(&line, &len, fp)) != -1) {
        if(counter++ == 0) continue; // skip the first line

        if(read < min_line_length) {
            ll_log(ls, "too small line No %zu of filename '%s': %s", counter, filename, line);
            continue;
        }

        unsigned int local_address, local_port, state, remote_address, remote_port, inode = 0;
        char local_address6[33], remote_address6[33];

        if(family == AF_INET) {
            if (sscanf(line, "%*d: %X:%X %X:%X %X %*X:%*X %*X:%*X %*X %*d %*d %u",
                       &local_address, &local_port, &remote_address, &remote_port, &state, &inode) != 6) {
                ll_log(ls, "cannot parse ipv4 line No %zu of filename '%s': %s", counter, filename, line);
                continue;
            }
        }
        else if(family == AF_INET6) {
            if(sscanf(line, "%*d: %32[0-9A-Fa-f]:%X %32[0-9A-Fa-f]:%X %X %*X:%*X %*X:%*X %*X %*d %*d %u",
                       local_address6, &local_port, remote_address6, &remote_port, &state, &inode) != 6) {
                ll_log(ls, "cannot parse ipv6 line No %zu of filename '%s': %s", counter, filename, line);
                continue;
            }
        }
        if(!inode) continue;

        SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, inode, &inode, true);
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(n) {
            ll_log(ls, "inode %u given on line %zu of filename '%s', already exists in hashtable - ignoring duplicate", inode, counter, filename);
            continue;
        }

        // allocate a new socket and index it

        n = (LOCAL_SOCKET *)callocz(1, sizeof(LOCAL_SOCKET));

        if(family == AF_INET) {
            n->local.ip.ipv4 = local_address;
            n->remote.ip.ipv4 = remote_address;
        }
        else if(family == AF_INET6) {
            ipv6_to_in6_addr(local_address6, &n->local.ip.ipv6);
            ipv6_to_in6_addr(remote_address6, &n->remote.ip.ipv6);
        }

        n->direction = 0;
        n->protocol = protocol;
        n->family = family;
        n->state = (int)state;
        n->inode = inode;
        n->local.port = local_port;
        n->remote.port = remote_port;
        n->protocol = protocol;

        n->local_port_key.port = n->local.port;
        n->local_port_key.family = n->family;
        n->local_port_key.protocol = n->protocol;

        n->local_ip_hash = XXH3_64bits(&n->local.ip, sizeof(n->local.ip));
        n->remote_ip_hash = XXH3_64bits(&n->remote.ip, sizeof(n->remote.ip));
        n->local_port_hash = XXH3_64bits(&n->local_port_key, sizeof(n->local_port_key));

        simple_hashtable_set_slot_LOCAL_SOCKET(&ls->sockets_hashtable, sl, inode, n);

        if(!is_zero_address(&n->local.ip, n->family)) {
            // put all the local IPs into the local_ips hashtable
            // so, we learn all local IPs the system has

            SIMPLE_HASHTABLE_SLOT_LOCAL_IP *sl_ip =
                simple_hashtable_get_slot_LOCAL_IP(&ls->local_ips_hashtable, n->local_ip_hash, &n->local.ip, true);

            union ipv46 *ip = SIMPLE_HASHTABLE_SLOT_DATA(sl_ip);
            if(!ip)
                simple_hashtable_set_slot_LOCAL_IP(&ls->local_ips_hashtable, sl_ip, n->local_ip_hash, &n->local.ip);
        }

        if((n->protocol == IPPROTO_TCP && n->state == TCP_LISTEN) || is_zero_address(&n->local.ip, n->family) || is_zero_address(&n->remote.ip, n->family)) {
            // the socket is either in a TCP LISTEN, or
            // the remote address is zero
            n->direction |= SOCKET_DIRECTION_LISTEN;
        }
        else if(is_loopback_address(&n->local.ip, n->family) || is_loopback_address(&n->remote.ip, n->family)) {
            // the local IP address is loopback
            n->direction |= SOCKET_DIRECTION_LOCAL;
        }
        else {
            // we can't say yet if it is inbound or outboud
            // so, mark it as both inbound and outbound
            n->direction |= SOCKET_DIRECTION_INBOUND | SOCKET_DIRECTION_OUTBOUND;
        }

        if(n->direction & SOCKET_DIRECTION_LISTEN) {
            // for the listening sockets, keep a hashtable with all the local ports
            // so that we will be able to detect INBOUND sockets

            SIMPLE_HASHTABLE_SLOT_LOCAL_PORT *sl_port =
                simple_hashtable_get_slot_LOCAL_PORT(&ls->listening_ports_hashtable, n->local_port_hash, &n->local_port_key, true);

            struct local_port *port = SIMPLE_HASHTABLE_SLOT_DATA(sl_port);
            if(!port)
                simple_hashtable_set_slot_LOCAL_PORT(&ls->listening_ports_hashtable, sl_port, n->local_port_hash, &n->local_port_key);
        }
    }

    fclose(fp);

    if (line)
        freez(line);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_detect_directions(LS_STATE *ls) {
    for (unsigned int i = 0; i < ls->sockets_hashtable.size; i++) {
        SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = &ls->sockets_hashtable.hashtable[i];
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if (!n) continue;

        if ((n->direction & (SOCKET_DIRECTION_INBOUND|SOCKET_DIRECTION_OUTBOUND)) !=
            (SOCKET_DIRECTION_INBOUND|SOCKET_DIRECTION_OUTBOUND))
            continue;

        // check if the remote IP is one of our local IPs
        {
            SIMPLE_HASHTABLE_SLOT_LOCAL_IP *sl_ip =
                simple_hashtable_get_slot_LOCAL_IP(&ls->local_ips_hashtable, n->remote_ip_hash, &n->remote.ip, false);

            union ipv46 *d = SIMPLE_HASHTABLE_SLOT_DATA(sl_ip);
            if (d) {
                // the remote IP of this socket is one of our local IPs
                n->direction &= ~(SOCKET_DIRECTION_INBOUND|SOCKET_DIRECTION_OUTBOUND);
                n->direction |= SOCKET_DIRECTION_LOCAL;
                continue;
            }
        }

        // check if the local port is one of our listening ports
        {
            SIMPLE_HASHTABLE_SLOT_LOCAL_PORT *sl_port =
                simple_hashtable_get_slot_LOCAL_PORT(&ls->listening_ports_hashtable, n->local_port_hash, &n->local_port_key, false);

            struct local_port *port = SIMPLE_HASHTABLE_SLOT_DATA(sl_port); // do not reference this pointer - is invalid
            if(port) {
                // the local port of this socket is a port we listen to
                n->direction &= ~SOCKET_DIRECTION_OUTBOUND;
            }
            else
                n->direction &= ~SOCKET_DIRECTION_INBOUND;
        }
    }
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_process(LS_STATE *ls) {
    char path[FILENAME_MAX + 1];

    simple_hashtable_init_LOCAL_SOCKET(&ls->sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_IP(&ls->local_ips_hashtable, 1024);
    simple_hashtable_init_LOCAL_PORT(&ls->listening_ports_hashtable, 1024);

    if(ls->config.tcp4) {
        snprintfz(path, FILENAME_MAX, "%s/proc/net/tcp", netdata_configured_host_prefix);
        read_proc_net_x(ls, path, AF_INET, IPPROTO_TCP);
    }

    if(ls->config.udp4) {
        snprintfz(path, FILENAME_MAX, "%s/proc/net/udp", netdata_configured_host_prefix);
        read_proc_net_x(ls, path, AF_INET, IPPROTO_UDP);
    }

    if(ls->config.tcp6) {
        snprintfz(path, FILENAME_MAX, "%s/proc/net/tcp6", netdata_configured_host_prefix);
        read_proc_net_x(ls, path, AF_INET6, IPPROTO_TCP);
    }

    if(ls->config.udp6) {
        snprintfz(path, FILENAME_MAX, "%s/proc/net/udp6", netdata_configured_host_prefix);
        read_proc_net_x(ls, path, AF_INET6, IPPROTO_UDP);
    }

    if(ls->config.cmdline || ls->config.comm || ls->config.pid) {
        snprintfz(path, FILENAME_MAX, "%s/proc", netdata_configured_host_prefix);
        find_all_sockets_in_proc(ls, path);
    }

    // detect the directions of the sockets
    if(ls->config.inbound || ls->config.outbound || ls->config.local)
        local_sockets_detect_directions(ls);

    // this will call the callback for each socket and free the memory we use
    foreach_local_socket_call_cb_and_cleanup(ls);

    // free the hashtable
    simple_hashtable_destroy_LOCAL_PORT(&ls->listening_ports_hashtable);
    simple_hashtable_destroy_LOCAL_IP(&ls->local_ips_hashtable);
    simple_hashtable_destroy_LOCAL_SOCKET(&ls->sockets_hashtable);
}

static inline void ipv6_address_to_txt(struct in6_addr *in6_addr, char *dst) {
    struct sockaddr_in6 sa = { 0 };

    sa.sin6_family = AF_INET6;
    sa.sin6_port = htons(0);
    sa.sin6_addr = *in6_addr;

    // Convert to human-readable format
    if (inet_ntop(AF_INET6, &(sa.sin6_addr), dst, INET6_ADDRSTRLEN) == NULL)
        *dst = '\0';
}

static inline void ipv4_address_to_txt(uint32_t ip, char *dst) {
    uint8_t octets[4];
    octets[0] = ip & 0xFF;
    octets[1] = (ip >> 8) & 0xFF;
    octets[2] = (ip >> 16) & 0xFF;
    octets[3] = (ip >> 24) & 0xFF;
    sprintf(dst, "%u.%u.%u.%u", octets[0], octets[1], octets[2], octets[3]);
}

#endif //NETDATA_LOCAL_SOCKETS_H
