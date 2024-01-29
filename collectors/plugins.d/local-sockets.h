// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOCAL_SOCKETS_H
#define NETDATA_LOCAL_SOCKETS_H

#include "libnetdata/libnetdata.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping the namespaces
// key and value is the namespace inode

#define SIMPLE_HASHTABLE_VALUE_TYPE uint64_t
#define SIMPLE_HASHTABLE_NAME _NET_NS
#include "libnetdata/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping the sockets of PIDs
// key is the inode

struct pid_socket;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct pid_socket
#define SIMPLE_HASHTABLE_NAME _PID_SOCKET
#include "libnetdata/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping all the sockets
// key is the inode

struct local_socket;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct local_socket
#define SIMPLE_HASHTABLE_NAME _LOCAL_SOCKET
#include "libnetdata/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping all local IPs
// key is XXH3_64bits hash of the IP

union ipv46;
#define SIMPLE_HASHTABLE_VALUE_TYPE union ipv46
#define SIMPLE_HASHTABLE_NAME _LOCAL_IP
#include "libnetdata/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping all listening ports
// key is XXH3_64bits hash of the family, protocol, port number, namespace

struct local_port;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct local_port
#define SIMPLE_HASHTABLE_NAME _LISTENING_PORT
#include "libnetdata/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------

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
        bool namespaces;
        size_t max_errors;

        local_sockets_cb_t cb;
        void *data;

        const char *host_prefix;
    } config;

    struct {
        size_t pid_fds_processed;
        size_t pid_fds_opendir_failed;
        size_t pid_fds_readlink_failed;
        size_t pid_fds_parse_failed;
        size_t errors_encountered;
    } stats;

    uint64_t proc_self_net_ns_inode;

    SIMPLE_HASHTABLE_NET_NS ns_hashtable;
    SIMPLE_HASHTABLE_PID_SOCKET pid_sockets_hashtable;
    SIMPLE_HASHTABLE_LOCAL_SOCKET sockets_hashtable;
    SIMPLE_HASHTABLE_LOCAL_IP local_ips_hashtable;
    SIMPLE_HASHTABLE_LISTENING_PORT listening_ports_hashtable;
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

struct pid_socket {
    uint64_t inode;
    pid_t pid;
    uint64_t net_ns_inode;
    char *cmdline;
    char comm[TASK_COMM_LEN];
};

union ipv46 {
    uint32_t ipv4;
    struct in6_addr ipv6;
};

struct local_port {
    uint16_t protocol;
    uint16_t family;
    uint16_t port;
    uint64_t net_ns_inode;
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
    uint64_t inode;
    uint64_t net_ns_inode;

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

static inline void local_sockets_log(LS_STATE *ls, const char *format, ...) __attribute__ ((format(__printf__, 2, 3)));
static inline void local_sockets_log(LS_STATE *ls, const char *format, ...) {
    if(++ls->stats.errors_encountered == ls->config.max_errors) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-LISTENERS: max number of logs reached. Not logging anymore");
        return;
    }

    if(ls->stats.errors_encountered > ls->config.max_errors)
        return;

    char buf[16384];
    va_list args;
    va_start(args, format);
    vsnprintf(buf, sizeof(buf), format, args);
    va_end(args);

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-LISTENERS: %s", buf);
}

// --------------------------------------------------------------------------------------------------------------------

static void local_sockets_foreach_local_socket_call_cb(LS_STATE *ls) {
    for(SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_first_read_only_LOCAL_SOCKET(&ls->sockets_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_LOCAL_SOCKET(&ls->sockets_hashtable, sl)) {
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
    }
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_fix_cmdline(char* str) {
    char *s = str;

    // map invalid characters to underscores
    while(*s) {
        if(*s == '|' || iscntrl(*s)) *s = '_';
        s++;
    }
}

// ----------------------------------------------------------------------------

static inline bool
local_sockets_read_proc_inode_link(LS_STATE *ls, const char *filename, uint64_t *inode, const char *type) {
    char link_target[FILENAME_MAX + 1];

    *inode = 0;

    ssize_t len = readlink(filename, link_target, sizeof(link_target) - 1);
    if (len == -1) {
        local_sockets_log(ls, "cannot read '%s' link '%s'", type, filename);

        ls->stats.pid_fds_readlink_failed++;
        return false;
    }
    link_target[len] = '\0';

    len = strlen(type);
    if(strncmp(link_target, type, len) == 0 && link_target[len] == ':' && link_target[len + 1] == '[' && isdigit(link_target[len + 2])) {
        *inode = strtoull(&link_target[len + 2], NULL, 10);
        // ll_log(ls, "read link of type '%s' '%s' from '%s', inode = %"PRIu64, type, link_target, filename, *inode);
        return true;
    }
    else {
        // ll_log(ls, "cannot read '%s' link '%s' from '%s'", type, link_target, filename);
        ls->stats.pid_fds_processed++;
        return false;
    }
}

static inline bool local_sockets_is_path_a_pid(const char *s) {
    if(!s || !*s) return false;

    while(*s) {
        if(!isdigit(*s++))
            return false;
    }

    return true;
}

static inline bool local_sockets_find_all_sockets_in_proc(LS_STATE *ls, const char *proc_filename) {
    DIR *proc_dir;
    struct dirent *proc_entry;
    char filename[FILENAME_MAX + 1];
    char comm[TASK_COMM_LEN];
    char cmdline[8192];
    const char *cmdline_trimmed;
    uint64_t net_ns_inode;

    proc_dir = opendir(proc_filename);
    if (proc_dir == NULL) {
        local_sockets_log(ls, "cannot opendir() '%s'", proc_filename);
        ls->stats.pid_fds_readlink_failed++;
        return false;
    }

    while ((proc_entry = readdir(proc_dir)) != NULL) {
        if(proc_entry->d_type != DT_DIR)
            continue;

        if(!strcmp(proc_entry->d_name, ".") || !strcmp(proc_entry->d_name, ".."))
            continue;

        if(!local_sockets_is_path_a_pid(proc_entry->d_name))
            continue;

        // Build the path to the fd directory of the process
        snprintfz(filename, FILENAME_MAX, "%s/%s/fd/", proc_filename, proc_entry->d_name);
        DIR *fd_dir = opendir(filename);
        if (fd_dir == NULL) {
            local_sockets_log(ls, "cannot opendir() '%s'", filename);
            ls->stats.pid_fds_opendir_failed++;
            continue;
        }

        comm[0] = '\0';
        cmdline[0] = '\0';
        cmdline_trimmed = NULL;
        pid_t pid = (pid_t)strtoul(proc_entry->d_name, NULL, 10);
        if(!pid) {
            local_sockets_log(ls, "cannot parse pid of '%s'", proc_entry->d_name);
            continue;
        }
        net_ns_inode = 0;

        struct dirent *fd_entry;
        while ((fd_entry = readdir(fd_dir)) != NULL) {
            if(fd_entry->d_type != DT_LNK)
                continue;

            snprintfz(filename, sizeof(filename), "%s/%s/fd/%s", proc_filename, proc_entry->d_name, fd_entry->d_name);
            uint64_t inode = 0;
            if(!local_sockets_read_proc_inode_link(ls, filename, &inode, "socket"))
                continue;

            SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl = simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, inode, &inode, true);
            struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl);
            if(!ps || (ps->pid == 1 && pid != 1)) {
                if(!comm[0] && ls->config.comm) {
                    snprintfz(filename, sizeof(filename), "%s/%s/comm", proc_filename, proc_entry->d_name);
                    if (read_txt_file(filename, comm, sizeof(comm)))
                        local_sockets_log(ls, "cannot open file: %s\n", filename);
                    else {
                        size_t clen = strlen(comm);
                        if(comm[clen - 1] == '\n')
                            comm[clen - 1] = '\0';
                    }
                }
                if(!cmdline[0] && ls->config.cmdline) {
                    snprintfz(filename, sizeof(filename), "%s/%s/cmdline", proc_filename, proc_entry->d_name);
                    if (read_proc_cmdline(filename, cmdline, sizeof(cmdline)))
                        local_sockets_log(ls, "cannot open file: %s\n", filename);
                    else {
                        local_sockets_fix_cmdline(cmdline);
                        cmdline_trimmed = trim(cmdline);
                    }
                }
                if(!net_ns_inode && ls->config.namespaces) {
                    snprintfz(filename, sizeof(filename), "%s/%s/ns/net", proc_filename, proc_entry->d_name);
                    if(local_sockets_read_proc_inode_link(ls, filename, &net_ns_inode, "net")) {
                        SIMPLE_HASHTABLE_SLOT_NET_NS *sl_ns = simple_hashtable_get_slot_NET_NS(&ls->ns_hashtable, net_ns_inode, (uint64_t *)net_ns_inode, true);
                        simple_hashtable_set_slot_NET_NS(&ls->ns_hashtable, sl_ns, net_ns_inode, (uint64_t *)net_ns_inode);
                    }
                }

                if(!ps)
                    ps = callocz(1, sizeof(*ps));

                ps->inode = inode;
                ps->pid = pid;
                ps->net_ns_inode = net_ns_inode;
                strncpyz(ps->comm, comm, sizeof(ps->comm) - 1);

                if(ps->cmdline)
                    freez(ps->cmdline);

                ps->cmdline = cmdline_trimmed ? strdupz(cmdline_trimmed) : NULL;
                simple_hashtable_set_slot_PID_SOCKET(&ls->pid_sockets_hashtable, sl, inode, ps);
            }
        }

        closedir(fd_dir);
    }

    closedir(proc_dir);
    return true;
}

// ----------------------------------------------------------------------------

static bool local_sockets_is_ipv4_mapped_ipv6_address(const struct in6_addr *addr) {
    // An IPv4-mapped IPv6 address starts with 80 bits of zeros followed by 16 bits of ones
    static const unsigned char ipv4_mapped_prefix[12] = { 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF };
    return memcmp(addr->s6_addr, ipv4_mapped_prefix, 12) == 0;
}

static bool local_sockets_is_loopback_address(const void *ip, uint16_t family) {
    if (family == AF_INET) {
        // For IPv4, loopback addresses are in the 127.0.0.0/8 range
        const uint32_t addr = ntohl(*((const uint32_t *)ip)); // Convert to host byte order for comparison
        return (addr >> 24) == 127; // Check if the first byte is 127
    } else if (family == AF_INET6) {
        // Check if the address is an IPv4-mapped IPv6 address
        const struct in6_addr *ipv6_addr = (const struct in6_addr *)ip;
        if (local_sockets_is_ipv4_mapped_ipv6_address(ipv6_addr)) {
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

static bool local_sockets_is_zero_address(const void *ip, uint16_t family) {
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

static inline void local_sockets_index_listening_port(LS_STATE *ls, LOCAL_SOCKET *n) {
    if(n->direction & SOCKET_DIRECTION_LISTEN) {
        // for the listening sockets, keep a hashtable with all the local ports
        // so that we will be able to detect INBOUND sockets

        SIMPLE_HASHTABLE_SLOT_LISTENING_PORT *sl_port =
            simple_hashtable_get_slot_LISTENING_PORT(&ls->listening_ports_hashtable, n->local_port_hash, &n->local_port_key, true);

        struct local_port *port = SIMPLE_HASHTABLE_SLOT_DATA(sl_port);
        if(!port)
            simple_hashtable_set_slot_LISTENING_PORT(&ls->listening_ports_hashtable, sl_port, n->local_port_hash, &n->local_port_key);
    }
}

static inline bool local_sockets_read_proc_net_x(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
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
            local_sockets_log(ls, "too small line No %zu of filename '%s': %s", counter, filename, line);
            continue;
        }

        unsigned int local_address, local_port, state, remote_address, remote_port;
        uint64_t  inode = 0;
        char local_address6[33], remote_address6[33];

        if(family == AF_INET) {
            if (sscanf(line, "%*d: %X:%X %X:%X %X %*X:%*X %*X:%*X %*X %*d %*d %"PRIu64,
                       &local_address, &local_port, &remote_address, &remote_port, &state, &inode) != 6) {
                local_sockets_log(ls, "cannot parse ipv4 line No %zu of filename '%s': %s", counter, filename, line);
                continue;
            }
        }
        else if(family == AF_INET6) {
            if(sscanf(line, "%*d: %32[0-9A-Fa-f]:%X %32[0-9A-Fa-f]:%X %X %*X:%*X %*X:%*X %*X %*d %*d %"PRIu64,
                       local_address6, &local_port, remote_address6, &remote_port, &state, &inode) != 6) {
                local_sockets_log(ls, "cannot parse ipv6 line No %zu of filename '%s': %s", counter, filename, line);
                continue;
            }
        }
        if(!inode) continue;

        SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, inode, &inode, true);
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(n) {
            local_sockets_log(
                ls,
                "inode %" PRIu64
                " given on line %zu of filename '%s', already exists in hashtable - ignoring duplicate",
                inode,
                counter,
                filename);
            continue;
        }

        // allocate a new socket and index it

        n = (LOCAL_SOCKET *)callocz(1, sizeof(LOCAL_SOCKET));

        // --- initialize it ------------------------------------------------------------------------------------------

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
        n->local_port_key.net_ns_inode = ls->proc_self_net_ns_inode;

        n->local_ip_hash = XXH3_64bits(&n->local.ip, sizeof(n->local.ip));
        n->remote_ip_hash = XXH3_64bits(&n->remote.ip, sizeof(n->remote.ip));
        n->local_port_hash = XXH3_64bits(&n->local_port_key, sizeof(n->local_port_key));

        // --- look up a pid for it -----------------------------------------------------------------------------------

        SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl_pid = simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, inode, &inode, false);
        struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl_pid);
        if(ps) {
            n->net_ns_inode = ps->net_ns_inode;
            n->pid = ps->pid;
            if(ps->cmdline)
                n->cmdline = strdupz(ps->cmdline);
            strncpyz(n->comm, ps->comm, sizeof(n->comm) - 1);
        }

        // --- index it -----------------------------------------------------------------------------------------------

        simple_hashtable_set_slot_LOCAL_SOCKET(&ls->sockets_hashtable, sl, inode, n);

        if(!local_sockets_is_zero_address(&n->local.ip, n->family)) {
            // put all the local IPs into the local_ips hashtable
            // so, we learn all local IPs the system has

            SIMPLE_HASHTABLE_SLOT_LOCAL_IP *sl_ip =
                simple_hashtable_get_slot_LOCAL_IP(&ls->local_ips_hashtable, n->local_ip_hash, &n->local.ip, true);

            union ipv46 *ip = SIMPLE_HASHTABLE_SLOT_DATA(sl_ip);
            if(!ip)
                simple_hashtable_set_slot_LOCAL_IP(&ls->local_ips_hashtable, sl_ip, n->local_ip_hash, &n->local.ip);
        }

        // --- 1st phase for direction detection ----------------------------------------------------------------------

        if((n->protocol == IPPROTO_TCP && n->state == TCP_LISTEN) ||
            local_sockets_is_zero_address(&n->local.ip, n->family) ||
            local_sockets_is_zero_address(&n->remote.ip, n->family)) {
            // the socket is either in a TCP LISTEN, or
            // the remote address is zero
            n->direction |= SOCKET_DIRECTION_LISTEN;
        }
        else if(
            local_sockets_is_loopback_address(&n->local.ip, n->family) ||
            local_sockets_is_loopback_address(&n->remote.ip, n->family)) {
            // the local IP address is loopback
            n->direction |= SOCKET_DIRECTION_LOCAL;
        }
        else {
            // we can't say yet if it is inbound or outboud
            // so, mark it as both inbound and outbound
            n->direction |= SOCKET_DIRECTION_INBOUND | SOCKET_DIRECTION_OUTBOUND;
        }

        // --- index it in LISTENING_PORT -----------------------------------------------------------------------------

        local_sockets_index_listening_port(ls, n);
    }

    fclose(fp);

    if (line)
        freez(line);

    return true;
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_detect_directions(LS_STATE *ls) {
    for(SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_first_read_only_LOCAL_SOCKET(&ls->sockets_hashtable);
         sl ;
         sl = simple_hashtable_next_read_only_LOCAL_SOCKET(&ls->sockets_hashtable, sl)) {
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
            SIMPLE_HASHTABLE_SLOT_LISTENING_PORT *sl_port =
                simple_hashtable_get_slot_LISTENING_PORT(&ls->listening_ports_hashtable, n->local_port_hash, &n->local_port_key, false);

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

static inline void local_sockets_init(LS_STATE *ls) {
    simple_hashtable_init_NET_NS(&ls->ns_hashtable, 1024);
    simple_hashtable_init_PID_SOCKET(&ls->pid_sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_SOCKET(&ls->sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_IP(&ls->local_ips_hashtable, 4096);
    simple_hashtable_init_LISTENING_PORT(&ls->listening_ports_hashtable, 4096);
}

static inline void local_sockets_cleanup(LS_STATE *ls) {
    // free the sockets hashtable data
    for(SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_first_read_only_LOCAL_SOCKET(&ls->sockets_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_LOCAL_SOCKET(&ls->sockets_hashtable, sl)) {
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(!n) continue;

        freez(n->cmdline);
        freez(n);
    }

    // free the pid_socket hashtable data
    for(SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl = simple_hashtable_first_read_only_PID_SOCKET(&ls->pid_sockets_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_PID_SOCKET(&ls->pid_sockets_hashtable, sl)) {
        struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(!ps) continue;

        freez(ps->cmdline);
        freez(ps);
    }

    // free the hashtable
    simple_hashtable_destroy_NET_NS(&ls->ns_hashtable);
    simple_hashtable_destroy_PID_SOCKET(&ls->pid_sockets_hashtable);
    simple_hashtable_destroy_LISTENING_PORT(&ls->listening_ports_hashtable);
    simple_hashtable_destroy_LOCAL_IP(&ls->local_ips_hashtable);
    simple_hashtable_destroy_LOCAL_SOCKET(&ls->sockets_hashtable);
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_read_sockets_from_proc(LS_STATE *ls) {
    char path[FILENAME_MAX + 1];

    if(ls->config.namespaces) {
        snprintfz(path, sizeof(path), "%s/proc/self/ns/net", ls->config.host_prefix);
        local_sockets_read_proc_inode_link(ls, path, &ls->proc_self_net_ns_inode, "net");
    }

    if(ls->config.cmdline || ls->config.comm || ls->config.pid || ls->config.namespaces) {
        snprintfz(path, sizeof(path), "%s/proc", ls->config.host_prefix);
        local_sockets_find_all_sockets_in_proc(ls, path);
    }

    if(ls->config.tcp4) {
        snprintfz(path, sizeof(path), "%s/proc/net/tcp", ls->config.host_prefix);
        local_sockets_read_proc_net_x(ls, path, AF_INET, IPPROTO_TCP);
    }

    if(ls->config.udp4) {
        snprintfz(path, sizeof(path), "%s/proc/net/udp", ls->config.host_prefix);
        local_sockets_read_proc_net_x(ls, path, AF_INET, IPPROTO_UDP);
    }

    if(ls->config.tcp6) {
        snprintfz(path, sizeof(path), "%s/proc/net/tcp6", ls->config.host_prefix);
        local_sockets_read_proc_net_x(ls, path, AF_INET6, IPPROTO_TCP);
    }

    if(ls->config.udp6) {
        snprintfz(path, sizeof(path), "%s/proc/net/udp6", ls->config.host_prefix);
        local_sockets_read_proc_net_x(ls, path, AF_INET6, IPPROTO_UDP);
    }
}

// --------------------------------------------------------------------------------------------------------------------

struct local_sockets_child_work {
    int fd;
    uint64_t net_ns_inode;
};

static inline void local_sockets_send_to_parent(struct local_socket_state *ls __maybe_unused, struct local_socket *n, void *data) {
    struct local_sockets_child_work *cw = data;
    int fd = cw->fd;

    if(n->net_ns_inode != cw->net_ns_inode)
        return;

    // local_sockets_log(ls, "child is sending inode %"PRIu64" of namespace %"PRIu64, n->inode, n->net_ns_inode);

    if(write(fd, n, sizeof(*n)) != sizeof(*n))
        local_sockets_log(ls, "failed to write local socket to pipe");

    size_t len = n->cmdline ? strlen(n->cmdline) + 1 : 0;
    if(write(fd, &len, sizeof(len)) != sizeof(len))
        local_sockets_log(ls, "failed to write cmdline length to pipe");

    if(len)
        if(write(fd, n->cmdline, len) != (ssize_t)len)
            local_sockets_log(ls, "failed to write cmdline to pipe");
}

static inline bool local_sockets_get_namespace_sockets(LS_STATE *ls, struct pid_socket *ps, pid_t *pid) {
    char filename[1024];
    snprintfz(filename, sizeof(filename), "%s/proc/%d/ns/net", ls->config.host_prefix, ps->pid);

    // verify the pid is in the target namespace
    struct stat statbuf;
    if (stat(filename, &statbuf) == -1 || statbuf.st_ino != ps->net_ns_inode) {
        local_sockets_log(ls, "pid %d is not in the wanted network namespace", ps->pid);
        return false;
    }

    int fd = open(filename, O_RDONLY);
    if(!fd) {
        local_sockets_log(ls, "cannot open file '%s'", filename);
        return false;
    }

    int pipefd[2];
    if (pipe(pipefd) != 0) {
        local_sockets_log(ls, "cannot create pipe");
        return false;
    }

    *pid = fork();
    if (*pid == 0) {
        // Child process
        close(pipefd[0]);

        // local_sockets_log(ls, "child is here for inode %"PRIu64" and namespace %"PRIu64, ps->inode, ps->net_ns_inode);

        struct local_sockets_child_work cw = {
            .net_ns_inode = ps->net_ns_inode,
            .fd = pipefd[1],
        };

        ls->config.host_prefix = ""; // we need the /proc of the container
        ls->config.cb = local_sockets_send_to_parent;
        ls->config.data = &cw;
        ls->config.cmdline = false; // we have these already
        ls->config.comm = false; // we have these already
        ls->config.pid = false; // we have these already
        ls->config.namespaces = false;
        ls->proc_self_net_ns_inode = ps->net_ns_inode;


        // switch namespace
        if (setns(fd, CLONE_NEWNET) == -1) {
            local_sockets_log(ls, "failed to switch network namespace at child process");
            exit(EXIT_FAILURE);
        }

        // read all sockets from /proc
        local_sockets_read_sockets_from_proc(ls);

        // send all sockets to parent
        local_sockets_foreach_local_socket_call_cb(ls);

        // send the terminating socket
        struct local_socket zero = {
            .net_ns_inode = ps->net_ns_inode,
        };
        local_sockets_send_to_parent(ls, &zero, &cw);

        close(pipefd[1]); // Close write end of pipe
        exit(EXIT_SUCCESS);
    }
    // parent

    close(fd);
    close(pipefd[1]);

    size_t received = 0;
    struct local_socket buf;
    while(read(pipefd[0], &buf, sizeof(buf)) == sizeof(buf)) {
        size_t len = 0;
        if(read(pipefd[0], &len, sizeof(len)) != sizeof(len))
            local_sockets_log(ls, "failed to read cmdline length from pipe");

        if(len) {
            buf.cmdline = mallocz(len);
            if(read(pipefd[0], buf.cmdline, len) != (ssize_t)len)
                local_sockets_log(ls, "failed to read cmdline from pipe");
        }
        else
            buf.cmdline = NULL;

        received++;

        struct local_socket zero = {
            .net_ns_inode = ps->net_ns_inode,
        };
        if(memcmp(&buf, &zero, sizeof(buf)) == 0) {
            // the terminator
            break;
        }

        SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, buf.inode, &buf, true);
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(n) {
            if(buf.cmdline)
                freez(buf.cmdline);

//            local_sockets_log(ls,
//                              "ns inode %" PRIu64" (comm: '%s', pid: %u, ns: %"PRIu64") already exists in hashtable (comm: '%s', pid: %u, ns: %"PRIu64") - ignoring duplicate",
//                              buf.inode, buf.comm, buf.pid, buf.net_ns_inode, n->comm, n->pid, n->net_ns_inode);
            continue;
        }
        else {
            n = mallocz(sizeof(*n));
            memcpy(n, &buf, sizeof(*n));
            simple_hashtable_set_slot_LOCAL_SOCKET(&ls->sockets_hashtable, sl, n->inode, n);

            local_sockets_index_listening_port(ls, n);
        }
    }

    close(pipefd[0]);

    return received > 0;
}

static inline void local_socket_waitpid(LS_STATE *ls, pid_t pid) {
    if(!pid) return;

    int status;
    waitpid(pid, &status, 0);

    if (WIFEXITED(status) && WEXITSTATUS(status) != 0)
        local_sockets_log(ls, "Child exited with status %d", WEXITSTATUS(status));
    else if (WIFSIGNALED(status))
        local_sockets_log(ls, "Child terminated by signal %d", WTERMSIG(status));
}

static inline void local_sockets_namespaces(LS_STATE *ls) {
    pid_t children[5] = { 0 };
    size_t last_child = 0;

    for(SIMPLE_HASHTABLE_SLOT_NET_NS *sl = simple_hashtable_first_read_only_NET_NS(&ls->ns_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_NET_NS(&ls->ns_hashtable, sl)) {
        uint64_t inode = (uint64_t)SIMPLE_HASHTABLE_SLOT_DATA(sl);

        if(inode == ls->proc_self_net_ns_inode)
            continue;

        // find a pid_socket that has this namespace
        for(SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl_pid = simple_hashtable_first_read_only_PID_SOCKET(&ls->pid_sockets_hashtable) ;
            sl_pid ;
            sl_pid = simple_hashtable_next_read_only_PID_SOCKET(&ls->pid_sockets_hashtable, sl_pid)) {
            struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl_pid);
            if(!ps || ps->net_ns_inode != inode) continue;

            if(++last_child >= 5)
                last_child = 0;

            local_socket_waitpid(ls, children[last_child]);
            children[last_child] = 0;

            // now we have a pid that has the same namespace inode
            if(local_sockets_get_namespace_sockets(ls, ps, &children[last_child]))
                break;
        }
    }

    for(size_t i = 0; i < 5 ;i++)
        local_socket_waitpid(ls, children[i]);
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_process(LS_STATE *ls) {
    ls->config.host_prefix = netdata_configured_host_prefix;

    // initialize our hashtables
    local_sockets_init(ls);

    // read all sockets from /proc
    local_sockets_read_sockets_from_proc(ls);

    // check all socket namespaces
    if(ls->config.namespaces)
        local_sockets_namespaces(ls);

    // detect the directions of the sockets
    if(ls->config.inbound || ls->config.outbound || ls->config.local)
        local_sockets_detect_directions(ls);

    // call the callback for each socket
    local_sockets_foreach_local_socket_call_cb(ls);

    // free all memory
    local_sockets_cleanup(ls);
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
