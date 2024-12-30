// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOCAL_SOCKETS_H
#define NETDATA_LOCAL_SOCKETS_H

#include "libnetdata/libnetdata.h"

#ifndef _countof
#define _countof(x) (sizeof(x) / sizeof(*(x)))
#endif

#define LOCAL_SOCKETS_USE_SETNS
#define USE_LIBMNL_AFTER_SETNS

#if defined(HAVE_LIBMNL)
#include <linux/rtnetlink.h>
#include <linux/inet_diag.h>
#include <linux/sock_diag.h>
#include <linux/unix_diag.h>
#include <linux/netlink.h>
#include <libmnl/libmnl.h>
#endif

#define UID_UNSET (uid_t)(UINT32_MAX)

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping the namespaces
// key and value is the namespace inode

#define SIMPLE_HASHTABLE_KEY_TYPE uint64_t
#define SIMPLE_HASHTABLE_VALUE_TYPE_IS_NOT_POINTER
#define SIMPLE_HASHTABLE_VALUE_TYPE uint64_t
#define SIMPLE_HASHTABLE_NAME _NET_NS
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping the sockets of PIDs
// key is the inode

struct pid_socket;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct pid_socket *
#define SIMPLE_HASHTABLE_NAME _PID_SOCKET
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping all the sockets
// key is the inode

struct local_socket;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct local_socket *
#define SIMPLE_HASHTABLE_NAME _LOCAL_SOCKET
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping all local IPs
// key is XXH3_64bits hash of the IP

union ipv46;
#define SIMPLE_HASHTABLE_VALUE_TYPE union ipv46 *
#define SIMPLE_HASHTABLE_NAME _LOCAL_IP
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------
// hashtable for keeping all listening ports
// key is XXH3_64bits hash of the family, protocol, port number, namespace

struct local_port;
#define SIMPLE_HASHTABLE_VALUE_TYPE struct local_port *
#define SIMPLE_HASHTABLE_NAME _LISTENING_PORT
#include "libnetdata/simple_hashtable/simple_hashtable.h"

// --------------------------------------------------------------------------------------------------------------------

struct local_socket_state;
typedef void (*local_sockets_cb_t)(struct local_socket_state *state, const struct local_socket *n, void *data);

struct local_sockets_config {
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
    bool uid;
    bool namespaces;
    bool tcp_info;
    bool no_mnl;
    bool procfile;
    bool report;

    size_t max_errors;
    size_t max_concurrent_namespaces;

    local_sockets_cb_t cb;
    void *data;

    const char *host_prefix;
};

struct local_sockets_state {
    uint32_t nl_seq;
    uint64_t net_ns_inode;
    pid_t net_ns_pid;
};

struct timing_work {
    usec_t start_ut;
    usec_t end_ut;
    const char *name;
};

struct local_sockets_ns_req {
    struct local_sockets_config config;
    struct local_sockets_state ns_state;
};

typedef struct local_socket_state {
    struct local_sockets_config config;
    struct local_sockets_state ns_state;

    struct {
        size_t mnl_sends;
        size_t tcp_info_received;
        size_t pid_fds_processed;
        size_t pid_fds_opendir_failed;
        size_t pid_fds_readlink_failed;
        size_t pid_fds_parse_failed;
        size_t errors_encountered;

        size_t sockets_added;

        size_t namespaces_found;
        size_t namespaces_absent;
        size_t namespaces_invalid;
#if defined(LOCAL_SOCKETS_USE_SETNS)
        size_t namespaces_forks_attempted;
        size_t namespaces_forks_failed;
        size_t namespaces_forks_unresponsive;
        size_t namespaces_sockets_new;
        size_t namespaces_sockets_existing;
#endif

        struct procfile_stats ff;
    } stats;

    size_t timings_idx;
    struct timing_work timings[30];

#if defined(LOCAL_SOCKETS_USE_SETNS)
    bool spawn_server_is_mine;
    SPAWN_SERVER *spawn_server;
#endif

#if defined(HAVE_LIBMNL)
    uint16_t tmp_protocol;
#endif

    procfile *ff;

    ARAL *local_socket_aral;
    ARAL *pid_socket_aral;
    SPINLOCK spinlock; // for namespaces

    uint64_t proc_self_net_ns_inode;

    SIMPLE_HASHTABLE_NET_NS ns_hashtable;
    SIMPLE_HASHTABLE_PID_SOCKET pid_sockets_hashtable;
    SIMPLE_HASHTABLE_LOCAL_SOCKET sockets_hashtable;
    SIMPLE_HASHTABLE_LOCAL_IP local_ips_hashtable;
    SIMPLE_HASHTABLE_LISTENING_PORT listening_ports_hashtable;
} LS_STATE;

// --------------------------------------------------------------------------------------------------------------------

typedef enum __attribute__((packed)) {
    SOCKET_DIRECTION_NONE = 0,
    SOCKET_DIRECTION_LISTEN = (1 << 0),         // a listening socket
    SOCKET_DIRECTION_INBOUND = (1 << 1),        // an inbound socket connecting a remote system to a local listening socket
    SOCKET_DIRECTION_OUTBOUND = (1 << 2),       // a socket initiated by this system, connecting to another system
    SOCKET_DIRECTION_LOCAL_INBOUND = (1 << 3),  // the socket connecting 2 localhost applications
    SOCKET_DIRECTION_LOCAL_OUTBOUND = (1 << 4), // the socket connecting 2 localhost applications
} SOCKET_DIRECTION;

#ifndef TASK_COMM_LEN
#define TASK_COMM_LEN 16
#endif

struct pid_socket {
    uint64_t inode;
    pid_t pid;
    uid_t uid;
    uint64_t net_ns_inode;
    char *cmdline;
    char comm[TASK_COMM_LEN];
};

struct local_port {
    uint16_t protocol;
    uint16_t family;
    uint16_t port;
    uint64_t net_ns_inode;
};

union ipv46 {
    uint32_t ipv4;
    struct in6_addr ipv6;
};

struct socket_endpoint {
    uint16_t protocol;
    uint16_t family;
    uint16_t port;
    union ipv46 ip;
};

static inline void ipv6_to_in6_addr(const char *ipv6_str, struct in6_addr *d) {
    char buf[9];

    for (size_t k = 0; k < 4; ++k) {
        memcpy(buf, ipv6_str + (k * 8), 8);
        buf[sizeof(buf) - 1] = '\0';
        d->s6_addr32[k] = str2uint32_hex(buf, NULL);
    }
}

typedef struct local_socket {
    uint64_t inode;
    uint64_t net_ns_inode;

    int state;
    struct socket_endpoint local;
    struct socket_endpoint remote;
    pid_t pid;

    SOCKET_DIRECTION direction;

    uint8_t timer;
    uint8_t retransmits; // the # of packets currently queued for retransmission (not yet acknowledged)
    uint32_t expires;
    uint32_t rqueue;
    uint32_t wqueue;
    uid_t uid;

    struct {
        bool checked;
        bool ipv46;
    } ipv6ony;

    union {
        struct tcp_info tcp;
    } info;

    char comm[TASK_COMM_LEN];
    STRING *cmdline;

    struct local_port local_port_key;

    XXH64_hash_t local_ip_hash;
    XXH64_hash_t remote_ip_hash;
    XXH64_hash_t local_port_hash;

#ifdef LOCAL_SOCKETS_EXTENDED_MEMBERS
    LOCAL_SOCKETS_EXTENDED_MEMBERS
#endif
} LOCAL_SOCKET;

#if defined(LOCAL_SOCKETS_USE_SETNS)
static inline int local_sockets_spawn_server_callback(SPAWN_REQUEST *request);
#endif

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_log(LS_STATE *ls, const char *format, ...) PRINTFLIKE(2, 3);
static inline void local_sockets_log(LS_STATE *ls, const char *format, ...) {
    if(ls && ++ls->stats.errors_encountered == ls->config.max_errors) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-SOCKETS: max number of logs reached. Not logging anymore");
        return;
    }

    if(ls && ls->stats.errors_encountered > ls->config.max_errors)
        return;

    char buf[16384];
    va_list args;
    va_start(args, format);
    vsnprintf(buf, sizeof(buf), format, args);
    va_end(args);

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-SOCKETS: %s", buf);
}

// --------------------------------------------------------------------------------------------------------------------

static bool local_sockets_is_ipv4_mapped_ipv6_address(const struct in6_addr *addr) {
    // An IPv4-mapped IPv6 address starts with 80 bits of zeros followed by 16 bits of ones
    static const unsigned char ipv4_mapped_prefix[12] = { 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF };
    return memcmp(addr->s6_addr, ipv4_mapped_prefix, 12) == 0;
}

static bool local_sockets_is_loopback_address(const struct socket_endpoint *se) {
    if (se->family == AF_INET) {
        // For IPv4, loopback addresses are in the 127.0.0.0/8 range
        return (ntohl(se->ip.ipv4) >> 24) == 127; // Check if the first byte is 127
    } else if (se->family == AF_INET6) {
        // Check if the address is an IPv4-mapped IPv6 address
        if (local_sockets_is_ipv4_mapped_ipv6_address(&se->ip.ipv6)) {
            // Extract the last 32 bits (IPv4 address) and check if it's in the 127.0.0.0/8 range
            uint8_t *ip6 = (uint8_t *)&se->ip.ipv6;
            const uint32_t ipv4_addr = *((const uint32_t *)(ip6 + 12));
            return (ntohl(ipv4_addr) >> 24) == 127;
        }

        // For IPv6, loopback address is ::1
        return memcmp(&se->ip.ipv6, &in6addr_loopback, sizeof(se->ip.ipv6)) == 0;
    }

    return false;
}

static inline bool local_sockets_is_ipv4_reserved_address(uint32_t ip) {
    // Check for the reserved address ranges
    ip = ntohl(ip);
    return (
            (ip >> 24 == 10) ||                         // Private IP range (A class)
            (ip >> 20 == (172 << 4) + 1) ||             // Private IP range (B class)
            (ip >> 16 == (192 << 8) + 168) ||           // Private IP range (C class)
            (ip >> 24 == 127) ||                        // Loopback address (127.0.0.0)
            (ip >> 24 == 0) ||                          // Reserved (0.0.0.0)
            (ip >> 24 == 169 && (ip >> 16) == 254) ||   // Link-local address (169.254.0.0)
            (ip >> 16 == (192 << 8) + 0)                // Test-Net (192.0.0.0)
            );
}

static inline bool local_sockets_is_private_address(const struct socket_endpoint *se) {
    if (se->family == AF_INET) {
        return local_sockets_is_ipv4_reserved_address(se->ip.ipv4);
    }
    else if (se->family == AF_INET6) {
        uint8_t *ip6 = (uint8_t *)&se->ip.ipv6;

        // Check if the address is an IPv4-mapped IPv6 address
        if (local_sockets_is_ipv4_mapped_ipv6_address(&se->ip.ipv6)) {
            // Extract the last 32 bits (IPv4 address) and check if it's in the 127.0.0.0/8 range
            const uint32_t ipv4_addr = *((const uint32_t *)(ip6 + 12));
            return local_sockets_is_ipv4_reserved_address(ipv4_addr);
        }

        // Check for link-local addresses (fe80::/10)
        if ((ip6[0] == 0xFE) && ((ip6[1] & 0xC0) == 0x80))
            return true;

        // Check for Unique Local Addresses (ULA) (fc00::/7)
        if ((ip6[0] & 0xFE) == 0xFC)
            return true;

        // Check for multicast addresses (ff00::/8)
        if (ip6[0] == 0xFF)
            return true;

        // For IPv6, loopback address is :: or ::1
        return memcmp(&se->ip.ipv6, &in6addr_any, sizeof(se->ip.ipv6)) == 0 ||
               memcmp(&se->ip.ipv6, &in6addr_loopback, sizeof(se->ip.ipv6)) == 0;
    }

    return false;
}

static bool local_sockets_is_multicast_address(const struct socket_endpoint *se) {
    if (se->family == AF_INET) {
        // For IPv4, check if the address is 0.0.0.0
        uint32_t ip = htonl(se->ip.ipv4);
        return (ip >= 0xE0000000 && ip <= 0xEFFFFFFF);   // Multicast address range (224.0.0.0/4)
    }
    else if (se->family == AF_INET6) {
        // For IPv6, check if the address is ff00::/8
        uint8_t *ip6 = (uint8_t *)&se->ip.ipv6;
        return ip6[0] == 0xff;
    }

    return false;
}

static bool local_sockets_is_zero_address(const struct socket_endpoint *se) {
    if (se->family == AF_INET) {
        // For IPv4, check if the address is 0.0.0.0
        return se->ip.ipv4 == 0;
    }
    else if (se->family == AF_INET6) {
        // For IPv6, check if the address is ::
        return memcmp(&se->ip.ipv6, &in6addr_any, sizeof(se->ip.ipv6)) == 0;
    }

    return false;
}

static inline const char *local_sockets_address_space(const struct socket_endpoint *se) {
    if(local_sockets_is_zero_address(se))
        return "zero";
    else if(local_sockets_is_loopback_address(se))
        return "loopback";
    else if(local_sockets_is_multicast_address(se))
        return "multicast";
    else if(local_sockets_is_private_address(se))
        return "private";
    else
        return "public";
}

static inline void ipv6_address_to_txt(const struct in6_addr *in6_addr, char *dst) {
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

static inline bool is_local_socket_ipv46(const LOCAL_SOCKET *n) {
    return n->local.family == AF_INET6 &&
           n->direction == SOCKET_DIRECTION_LISTEN &&
           local_sockets_is_zero_address(&n->local) &&
           n->ipv6ony.checked &&
           n->ipv6ony.ipv46;
}

static inline const char *local_sockets_protocol_name(LOCAL_SOCKET *n) {
    if(n->local.family == AF_INET) {
        if(n->local.protocol == IPPROTO_TCP)
            return "TCP";
        else if(n->local.protocol == IPPROTO_UDP)
            return "UDP";
        else
            return "UNKNOWN_IPV4";
    }
    else if(is_local_socket_ipv46(n)) {
        if (n->local.protocol == IPPROTO_TCP)
            return "TCP46";
        else if(n->local.protocol == IPPROTO_UDP)
            return "UDP46";
        else
            return "UNKNOWN_IPV46";
    }
    else if(n->local.family == AF_INET6) {
        if (n->local.protocol == IPPROTO_TCP)
            return "TCP6";
        else if(n->local.protocol == IPPROTO_UDP)
            return "UDP6";
        else
            return "UNKNOWN_IPV6";
    }
    else
        return "UNKNOWN";
}

static inline void local_listeners_print_socket(LS_STATE *ls __maybe_unused, const LOCAL_SOCKET *nn, void *data __maybe_unused) {
    LOCAL_SOCKET *n = (LOCAL_SOCKET *)nn;

    char local_address[INET6_ADDRSTRLEN] = "";
    char remote_address[INET6_ADDRSTRLEN] = "";

    if(n->local.family == AF_INET) {
        ipv4_address_to_txt(n->local.ip.ipv4, local_address);
        ipv4_address_to_txt(n->remote.ip.ipv4, remote_address);
    }
    else if(n->local.family == AF_INET6) {
        ipv6_address_to_txt(&n->local.ip.ipv6, local_address);
        ipv6_address_to_txt(&n->remote.ip.ipv6, remote_address);
    }

    printf("%s, direction=%s%s%s%s%s pid=%d, state=0x%0x, ns=%"PRIu64", local=%s[:%u], remote=%s[:%u], uid=%u, inode=%"PRIu64", comm=%s\n",
        local_sockets_protocol_name(n),
           (n->direction & SOCKET_DIRECTION_LISTEN) ? "LISTEN," : "",
           (n->direction & SOCKET_DIRECTION_INBOUND) ? "INBOUND," : "",
           (n->direction & SOCKET_DIRECTION_OUTBOUND) ? "OUTBOUND," : "",
           (n->direction & (SOCKET_DIRECTION_LOCAL_INBOUND|SOCKET_DIRECTION_LOCAL_OUTBOUND)) ? "LOCAL," : "",
           (n->direction == 0) ? "NONE," : "",
           n->pid,
           (unsigned int)n->state,
           n->net_ns_inode,
           local_address, n->local.port,
           remote_address, n->remote.port,
           n->uid,
           n->inode,
           n->comm);
}

// --------------------------------------------------------------------------------------------------------------------

static void local_sockets_foreach_local_socket_call_cb(LS_STATE *ls) {
    for(SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_first_read_only_LOCAL_SOCKET(&ls->sockets_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_LOCAL_SOCKET(&ls->sockets_hashtable, sl)) {
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(!n) continue;

        if((ls->config.listening && n->direction & SOCKET_DIRECTION_LISTEN) ||
            (ls->config.local && n->direction & (SOCKET_DIRECTION_LOCAL_INBOUND|SOCKET_DIRECTION_LOCAL_OUTBOUND)) ||
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

// --------------------------------------------------------------------------------------------------------------------

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
            closedir(fd_dir);
            continue;
        }
        net_ns_inode = 0;
        uid_t uid = UID_UNSET;

        struct dirent *fd_entry;
        while ((fd_entry = readdir(fd_dir)) != NULL) {
            if(fd_entry->d_type != DT_LNK)
                continue;

            snprintfz(filename, sizeof(filename), "%s/%s/fd/%s", proc_filename, proc_entry->d_name, fd_entry->d_name);
            uint64_t inode = 0;
            if(!local_sockets_read_proc_inode_link(ls, filename, &inode, "socket"))
                continue;

            // fprintf(stderr, "%d: PID %d is using socket inode %"PRIu64"\n", gettid_uncached(), pid, inode);
            XXH64_hash_t inode_hash = XXH3_64bits(&inode, sizeof(inode));
            SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl = simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, inode_hash, &inode, true);
            struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl);
            if(!ps || (ps->pid == 1 && pid != 1)) {
                if(uid == UID_UNSET && ls->config.uid) {
                    char status_buf[512];
                    snprintfz(filename, sizeof(filename), "%s/%s/status", proc_filename, proc_entry->d_name);
                    if (read_txt_file(filename, status_buf, sizeof(status_buf)))
                        local_sockets_log(ls, "cannot open file: %s\n", filename);
                    else {
                        char *u = strstr(status_buf, "Uid:");
                        if(u) {
                            u += 4;
                            while(isspace(*u)) u++;                     // skip spaces
                            while(*u >= '0' && *u <= '9') u++;          // skip the first number (real uid)
                            while(isspace(*u)) u++;                     // skip spaces again
                            uid = strtol(u, NULL, 10);   // parse the 2nd number (effective uid)
                        }
                    }
                }
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
                        XXH64_hash_t net_ns_inode_hash = XXH3_64bits(&net_ns_inode, sizeof(net_ns_inode));
                        SIMPLE_HASHTABLE_SLOT_NET_NS *sl_ns = simple_hashtable_get_slot_NET_NS(&ls->ns_hashtable, net_ns_inode_hash, &net_ns_inode, true);
                        simple_hashtable_set_slot_NET_NS(&ls->ns_hashtable, sl_ns, net_ns_inode, net_ns_inode);
                    }
                }

                if(!ps)
                    ps = aral_callocz(ls->pid_socket_aral);

                ps->inode = inode;
                ps->pid = pid;
                ps->uid = uid;
                ps->net_ns_inode = net_ns_inode;
                strncpyz(ps->comm, comm, sizeof(ps->comm) - 1);

                if(ps->cmdline)
                    freez(ps->cmdline);

                ps->cmdline = cmdline_trimmed ? strdupz(cmdline_trimmed) : NULL;
                simple_hashtable_set_slot_PID_SOCKET(&ls->pid_sockets_hashtable, sl, inode_hash, ps);
                // fprintf(stderr, "%d: PID %d indexed for using socket inode %"PRIu64"\n", gettid_uncached(), pid, inode);
            }
        }

        closedir(fd_dir);
    }

    closedir(proc_dir);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

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

static inline bool local_sockets_add_socket(LS_STATE *ls, LOCAL_SOCKET *tmp) {
    if(!tmp->inode) return false;

    XXH64_hash_t inode_hash = XXH3_64bits(&tmp->inode, sizeof(tmp->inode));
    SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, inode_hash, &tmp->inode, true);
    LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(n) {
        local_sockets_log(ls, "inode %" PRIu64" already exists in hashtable - ignoring duplicate", tmp->inode);
        return false;
    }

    ls->stats.sockets_added++;

    n = aral_mallocz(ls->local_socket_aral);
    *n = *tmp; // copy all contents

    // fix the key
    n->local_port_key.port = n->local.port;
    n->local_port_key.family = n->local.family;
    n->local_port_key.protocol = n->local.protocol;
    n->local_port_key.net_ns_inode = ls->proc_self_net_ns_inode;

    n->local_ip_hash = XXH3_64bits(&n->local.ip, sizeof(n->local.ip));
    n->remote_ip_hash = XXH3_64bits(&n->remote.ip, sizeof(n->remote.ip));
    n->local_port_hash = XXH3_64bits(&n->local_port_key, sizeof(n->local_port_key));

    // --- look up a pid for it -----------------------------------------------------------------------------------

    SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl_pid = simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, inode_hash, &n->inode, false);
    struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl_pid);
    if(ps) {
        n->net_ns_inode = ps->net_ns_inode;
        n->pid = ps->pid;

        if(ps->uid != UID_UNSET && n->uid == UID_UNSET)
            n->uid = ps->uid;

        if(ps->cmdline) {
            if(n->cmdline) string_freez(n->cmdline);
            n->cmdline = string_strdupz(ps->cmdline);
        }

        strncpyz(n->comm, ps->comm, sizeof(n->comm) - 1);
    }
//    else
//        fprintf(stderr, "%d: No PID found for inode %"PRIu64"\n", gettid_uncached(), n->inode);

    // --- index it -----------------------------------------------------------------------------------------------

    simple_hashtable_set_slot_LOCAL_SOCKET(&ls->sockets_hashtable, sl, inode_hash, n);

    if(!local_sockets_is_zero_address(&n->local)) {
        // put all the local IPs into the local_ips hashtable
        // so, we learn all local IPs the system has

        SIMPLE_HASHTABLE_SLOT_LOCAL_IP *sl_ip =
            simple_hashtable_get_slot_LOCAL_IP(&ls->local_ips_hashtable, n->local_ip_hash, &n->local.ip, true);

        union ipv46 *ip = SIMPLE_HASHTABLE_SLOT_DATA(sl_ip);
        if(!ip)
            simple_hashtable_set_slot_LOCAL_IP(&ls->local_ips_hashtable, sl_ip, n->local_ip_hash, &n->local.ip);
    }

    // --- 1st phase for direction detection ----------------------------------------------------------------------

    if((n->local.protocol == IPPROTO_TCP && n->state == TCP_LISTEN) ||
        local_sockets_is_zero_address(&n->local) ||
        local_sockets_is_zero_address(&n->remote)) {
        // the socket is either in a TCP LISTEN, or
        // the remote address is zero
        n->direction |= SOCKET_DIRECTION_LISTEN;
    }
    else {
        // we can't say yet if it is inbound or outboud
        // so, mark it as both inbound and outbound
        n->direction |= SOCKET_DIRECTION_INBOUND | SOCKET_DIRECTION_OUTBOUND;
    }

    // --- index it in LISTENING_PORT -----------------------------------------------------------------------------

    local_sockets_index_listening_port(ls, n);

    return true;
}

#if defined(HAVE_LIBMNL)

static inline int local_sockets_libmnl_cb_data(const struct nlmsghdr *nlh, void *data) {
    LS_STATE *ls = data;

    struct inet_diag_msg *diag_msg = mnl_nlmsg_get_payload(nlh);

    LOCAL_SOCKET n = {
        .inode = diag_msg->idiag_inode,
        .direction = SOCKET_DIRECTION_NONE,
        .state = diag_msg->idiag_state,
        .ipv6ony = {
            .checked = false,
            .ipv46 = false,
        },
        .local = {
            .protocol = ls->tmp_protocol,
            .family = diag_msg->idiag_family,
            .port = ntohs(diag_msg->id.idiag_sport),
        },
        .remote = {
            .protocol = ls->tmp_protocol,
            .family = diag_msg->idiag_family,
            .port = ntohs(diag_msg->id.idiag_dport),
        },
        .timer = diag_msg->idiag_timer,
        .retransmits = diag_msg->idiag_retrans,
        .expires = diag_msg->idiag_expires,
        .rqueue = diag_msg->idiag_rqueue,
        .wqueue = diag_msg->idiag_wqueue,
        .uid = diag_msg->idiag_uid,
    };

    if (diag_msg->idiag_family == AF_INET) {
        memcpy(&n.local.ip.ipv4, diag_msg->id.idiag_src, sizeof(n.local.ip.ipv4));
        memcpy(&n.remote.ip.ipv4, diag_msg->id.idiag_dst, sizeof(n.remote.ip.ipv4));
    }
    else if (diag_msg->idiag_family == AF_INET6) {
        memcpy(&n.local.ip.ipv6, diag_msg->id.idiag_src, sizeof(n.local.ip.ipv6));
        memcpy(&n.remote.ip.ipv6, diag_msg->id.idiag_dst, sizeof(n.remote.ip.ipv6));
    }

    struct rtattr *attr = (struct rtattr *)(diag_msg + 1);
    int rtattrlen = nlh->nlmsg_len - NLMSG_LENGTH(sizeof(*diag_msg));
    for (; !n.ipv6ony.checked && RTA_OK(attr, rtattrlen); attr = RTA_NEXT(attr, rtattrlen)) {
        switch (attr->rta_type) {
            case INET_DIAG_INFO: {
                if(ls->tmp_protocol == IPPROTO_TCP) {
                    struct tcp_info *info = (struct tcp_info *)RTA_DATA(attr);
                    n.info.tcp = *info;
                    ls->stats.tcp_info_received++;
                }
            }
            break;

            case INET_DIAG_SKV6ONLY: {
                n.ipv6ony.checked = true;
                int ipv6only = *(int *)RTA_DATA(attr);
                n.ipv6ony.ipv46 = !ipv6only;
            }
            break;

            default:
                break;
        }
    }

    local_sockets_add_socket(ls, &n);

    return MNL_CB_OK;
}

static inline bool local_sockets_libmnl_get_sockets(LS_STATE *ls, uint16_t family, uint16_t protocol) {
    ls->tmp_protocol = protocol;

    struct mnl_socket *nl = mnl_socket_open(NETLINK_INET_DIAG);
    if (nl == NULL) {
        local_sockets_log(ls, "mnl_socket_open() failed");
        return false;
    }

    if (mnl_socket_bind(nl, 0, MNL_SOCKET_AUTOPID) < 0) {
        local_sockets_log(ls, "mnl_socket_bind() failed");
        mnl_socket_close(nl);
        return false;
    }

    char buf[MNL_SOCKET_BUFFER_SIZE];
    struct nlmsghdr *nlh = mnl_nlmsg_put_header(buf);
    nlh->nlmsg_type = SOCK_DIAG_BY_FAMILY;
    nlh->nlmsg_flags = NLM_F_REQUEST | NLM_F_DUMP;
    nlh->nlmsg_seq = ls->ns_state.nl_seq ? ls->ns_state.nl_seq++ : (uint32_t)time(NULL);

    struct inet_diag_req_v2 req = {
        .sdiag_family = family,
        .sdiag_protocol = protocol,
        .idiag_states = ~0,  // Request all socket states
        .idiag_ext = 0,
    };

    if(family == AF_INET6)
        req.idiag_ext |= 1 << (INET_DIAG_SKV6ONLY - 1);

    if(protocol == IPPROTO_TCP && ls->config.tcp_info)
        req.idiag_ext |= 1 << (INET_DIAG_INFO - 1);

    mnl_nlmsg_put_extra_header(nlh, sizeof(req));
    memcpy(mnl_nlmsg_get_payload(nlh), &req, sizeof(req));

    ls->stats.mnl_sends++;
    if (mnl_socket_sendto(nl, nlh, nlh->nlmsg_len) < 0) {
        local_sockets_log(ls, "mnl_socket_sendto() failed");
        mnl_socket_close(nl);
        return false;
    }

    bool rc = true;
    size_t received = 0;
    ssize_t ret;
    while ((ret = mnl_socket_recvfrom(nl, buf, sizeof(buf))) > 0) {
        ret = mnl_cb_run(buf, ret, 0, 0, local_sockets_libmnl_cb_data, ls);
        if (ret == MNL_CB_ERROR) {
            local_sockets_log(ls, "mnl_cb_run() failed");
            rc = false;
            break;
        }
        else if (ret <= MNL_CB_STOP)
            break;

        received++;
    }
    mnl_socket_close(nl);

    if (ret == -1) {
        local_sockets_log(ls, "mnl_socket_recvfrom() failed");
        rc = false;
    }

    return rc;
}
#endif // HAVE_LIBMNL

static inline bool local_sockets_process_proc_line(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol, size_t line, char **words, size_t num_words) {
    // char *sl_txt = get_word(words, num_words, 0);
    char *local_ip_txt = get_word(words, num_words, 1);
    char *local_port_txt = get_word(words, num_words, 2);
    char *remote_ip_txt = get_word(words, num_words, 3);
    char *remote_port_txt = get_word(words, num_words, 4);
    char *state_txt = get_word(words, num_words, 5);
    char *tx_queue_txt = get_word(words, num_words, 6);
    char *rx_queue_txt = get_word(words, num_words, 7);
    char *tr_txt = get_word(words, num_words, 8);
    char *tm_when_txt = get_word(words, num_words, 9);
    char *retrans_txt = get_word(words, num_words, 10);
    char *uid_txt = get_word(words, num_words, 11);
    // char *timeout_txt = get_word(words, num_words, 12);
    char *inode_txt = get_word(words, num_words, 13);

    if(!local_ip_txt || !local_port_txt || !remote_ip_txt || !remote_port_txt || !state_txt ||
        !tx_queue_txt || !rx_queue_txt || !tr_txt || !tm_when_txt || !retrans_txt || !uid_txt || !inode_txt) {
        local_sockets_log(ls, "cannot parse ipv4 line No %zu of filename '%s'", line, filename);
        return false;
    }

    LOCAL_SOCKET n = {
        .direction = SOCKET_DIRECTION_NONE,
        .ipv6ony = {
            .checked = false,
            .ipv46 = false,
        },
        .local = {
            .family = family,
            .protocol = protocol,
        },
        .remote = {
            .family = family,
            .protocol = protocol,
        },
        .uid = UID_UNSET,
    };

    n.local.port = str2uint32_hex(local_port_txt, NULL);
    n.remote.port = str2uint32_hex(remote_port_txt, NULL);
    n.state = str2uint32_hex(state_txt, NULL);
    n.wqueue = str2uint32_hex(tx_queue_txt, NULL);
    n.rqueue = str2uint32_hex(rx_queue_txt, NULL);
    n.timer = str2uint32_hex(tr_txt, NULL);
    n.expires = str2uint32_hex(tm_when_txt, NULL);
    n.retransmits = str2uint32_hex(retrans_txt, NULL);
    n.uid = str2uint32_t(uid_txt, NULL);
    n.inode = str2uint64_t(inode_txt, NULL);

    if(family == AF_INET) {
        n.local.ip.ipv4 = str2uint32_hex(local_ip_txt, NULL);
        n.remote.ip.ipv4 = str2uint32_hex(remote_ip_txt, NULL);
    }
    else if(family == AF_INET6) {
        ipv6_to_in6_addr(local_ip_txt, &n.local.ip.ipv6);
        ipv6_to_in6_addr(remote_ip_txt, &n.remote.ip.ipv6);
    }

    local_sockets_add_socket(ls, &n);
    return true;
}

static inline bool local_sockets_read_proc_net_x_getline(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
    static bool is_space[256] = {
        [':'] = true,
        [' '] = true,
    };

    if(family != AF_INET && family != AF_INET6)
        return false;

    FILE *fp = fopen(filename, "r");
    if (fp == NULL)
        return false;

    char *line = malloc(1024);  // no mallocz() here because getline() may resize
    if(!line) {
        fclose(fp);
        return false;
    }

    size_t len = 1024;
    ssize_t read;

    ssize_t min_line_length = (family == AF_INET) ? 105 : 155;
    size_t counter = 0;

    // Read line by line
    while ((read = getline(&line, &len, fp)) != -1) {
        if(counter++ == 0) continue; // skip the first line

        if(read < min_line_length) {
            local_sockets_log(ls, "too small line No %zu of filename '%s': %s", counter, filename, line);
            continue;
        }

        char *words[32];
        size_t num_words = quoted_strings_splitter(line, words, 32, is_space);
        local_sockets_process_proc_line(ls, filename, family, protocol, counter, words, num_words);
    }

    fclose(fp);

    if (line)
        free(line); // no freez() here because getline() may resize

    return true;
}

#define INITIALLY_EXPECTED_PROC_NET_LINES 16384
#define PROC_NET_BYTES_PER_LINE 155 // 105 for IPv4, 155 for IPv6
#define PROC_NET_WORDS_PER_LINE 22
#define INITIALLY_EXPECTED_PROC_NET_WORDS (INITIALLY_EXPECTED_PROC_NET_LINES * PROC_NET_WORDS_PER_LINE)
#define INITIALLY_EXPECTED_PROC_NET_BYTES (INITIALLY_EXPECTED_PROC_NET_LINES * PROC_NET_BYTES_PER_LINE)

static inline bool local_sockets_read_proc_net_x_procfile(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
    if(family != AF_INET && family != AF_INET6)
        return false;

    procfile_set_adaptive_allocation(true, INITIALLY_EXPECTED_PROC_NET_BYTES, INITIALLY_EXPECTED_PROC_NET_LINES, INITIALLY_EXPECTED_PROC_NET_WORDS);

    bool copy_initial_ff_stats = ls->ff == NULL && ls->stats.ff.memory > 0;
    ls->ff = procfile_reopen(ls->ff, filename, ls->ff ? NULL :" :", PROCFILE_FLAG_DEFAULT);

    // we just created ff, copy our old stats to it
    if(ls->ff && copy_initial_ff_stats) ls->ff->stats = ls->stats.ff;

    ls->ff = procfile_readall(ls->ff);
    if(!ls->ff) return false;

    // get the latest stats from ff;
    ls->stats.ff = ls->ff->stats;

    for(size_t l = 1; l < procfile_lines(ls->ff) ;l++) {
        size_t w = procfile_linewords(ls->ff, l);
        if(!w) continue;
        if(w < 14) {
            local_sockets_log(ls, "too small line No %zu of filename '%s' (has %zu words)", l, filename, w);
            continue;
        }

        char *words[14] = { 0 };
        words[0] = procfile_lineword(ls->ff, l, 0);
        words[1] = procfile_lineword(ls->ff, l, 1);
        words[2] = procfile_lineword(ls->ff, l, 2);
        words[3] = procfile_lineword(ls->ff, l, 3);
        words[4] = procfile_lineword(ls->ff, l, 4);
        words[5] = procfile_lineword(ls->ff, l, 5);
        words[6] = procfile_lineword(ls->ff, l, 6);
        words[7] = procfile_lineword(ls->ff, l, 7);
        words[8] = procfile_lineword(ls->ff, l, 8);
        words[9] = procfile_lineword(ls->ff, l, 9);
        words[10] = procfile_lineword(ls->ff, l, 10);
        words[11] = procfile_lineword(ls->ff, l, 11);
        words[12] = procfile_lineword(ls->ff, l, 12);
        words[13] = procfile_lineword(ls->ff, l, 13);
        local_sockets_process_proc_line(ls, filename, family, protocol, l, words, _countof(words));
    }

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

        // check if the remote IP is one of our local IPs
        {
            SIMPLE_HASHTABLE_SLOT_LOCAL_IP *sl_ip =
                simple_hashtable_get_slot_LOCAL_IP(&ls->local_ips_hashtable, n->remote_ip_hash, &n->remote.ip, false);

            union ipv46 *d = SIMPLE_HASHTABLE_SLOT_DATA(sl_ip);
            if (d) {
                // the remote IP of this socket is one of our local IPs
                if(n->direction & SOCKET_DIRECTION_INBOUND) {
                    n->direction &= ~SOCKET_DIRECTION_INBOUND;
                    n->direction |= SOCKET_DIRECTION_LOCAL_INBOUND;
                }
                else if(n->direction & SOCKET_DIRECTION_OUTBOUND) {
                    n->direction &= ~SOCKET_DIRECTION_OUTBOUND;
                    n->direction |= SOCKET_DIRECTION_LOCAL_OUTBOUND;
                }
                continue;
            }
        }

        if (local_sockets_is_loopback_address(&n->local) ||
            local_sockets_is_loopback_address(&n->remote)) {
            // both IP addresses are loopback
            if(n->direction & SOCKET_DIRECTION_INBOUND) {
                n->direction &= ~SOCKET_DIRECTION_INBOUND;
                n->direction |= SOCKET_DIRECTION_LOCAL_INBOUND;
            }
            else if(n->direction & SOCKET_DIRECTION_OUTBOUND) {
                n->direction &= ~SOCKET_DIRECTION_OUTBOUND;
                n->direction |= SOCKET_DIRECTION_LOCAL_OUTBOUND;
            }
        }
    }
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_init(LS_STATE *ls) {
    ls->config.host_prefix = netdata_configured_host_prefix;

    spinlock_init(&ls->spinlock);

    simple_hashtable_init_NET_NS(&ls->ns_hashtable, 1024);
    simple_hashtable_init_PID_SOCKET(&ls->pid_sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_SOCKET(&ls->sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_IP(&ls->local_ips_hashtable, 4096);
    simple_hashtable_init_LISTENING_PORT(&ls->listening_ports_hashtable, 4096);

    ls->local_socket_aral = aral_create(
        "local-sockets",
        sizeof(LOCAL_SOCKET),
        65536 / sizeof(LOCAL_SOCKET),
        65536,
        NULL, NULL, NULL, false, true, true);

    ls->pid_socket_aral = aral_create(
        "pid-sockets",
        sizeof(struct pid_socket),
        65536 / sizeof(struct pid_socket),
        65536,
        NULL, NULL, NULL, false, true, true);

    memset(&ls->stats, 0, sizeof(ls->stats));

#if defined(HAVE_LIBMNL)
    ls->tmp_protocol = 0;
#endif

#if defined(LOCAL_SOCKETS_USE_SETNS)
    if(ls->config.namespaces && ls->spawn_server == NULL) {
        ls->spawn_server = spawn_server_create(SPAWN_SERVER_OPTION_CALLBACK, NULL, local_sockets_spawn_server_callback, 0, NULL);
        ls->spawn_server_is_mine = true;
    }
    else
        ls->spawn_server_is_mine = false;
#endif
}

static inline void local_sockets_cleanup(LS_STATE *ls) {
    if(ls->ff) {
        ls->stats.ff = ls->ff->stats;
        procfile_close(ls->ff);
        ls->ff = NULL;
    }

#if defined(LOCAL_SOCKETS_USE_SETNS)
    if(ls->spawn_server_is_mine) {
        spawn_server_destroy(ls->spawn_server);
        ls->spawn_server = NULL;
        ls->spawn_server_is_mine = false;
    }
#endif

    // free the sockets hashtable data
    for(SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_first_read_only_LOCAL_SOCKET(&ls->sockets_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_LOCAL_SOCKET(&ls->sockets_hashtable, sl)) {
        LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(!n) continue;

        string_freez(n->cmdline);
        aral_freez(ls->local_socket_aral, n);
    }

    // free the pid_socket hashtable data
    for(SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl = simple_hashtable_first_read_only_PID_SOCKET(&ls->pid_sockets_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_PID_SOCKET(&ls->pid_sockets_hashtable, sl)) {
        struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl);
        if(!ps) continue;

        freez(ps->cmdline);
        aral_freez(ls->pid_socket_aral, ps);
    }

    // free the hashtable
    simple_hashtable_destroy_NET_NS(&ls->ns_hashtable);
    simple_hashtable_destroy_PID_SOCKET(&ls->pid_sockets_hashtable);
    simple_hashtable_destroy_LISTENING_PORT(&ls->listening_ports_hashtable);
    simple_hashtable_destroy_LOCAL_IP(&ls->local_ips_hashtable);
    simple_hashtable_destroy_LOCAL_SOCKET(&ls->sockets_hashtable);

    aral_destroy(ls->local_socket_aral);
    aral_destroy(ls->pid_socket_aral);
}

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_track_time(LS_STATE *ls, const char *name) {
    if(!ls->config.report || ls->timings_idx >= _countof(ls->timings))
        return;

    usec_t now_ut = now_monotonic_usec();

    if(ls->timings_idx == 0 && !ls->timings[0].start_ut) {
        ls->timings[0].start_ut = now_ut;
        ls->timings[0].name = name;
    }
    else if(ls->timings_idx + 1 < _countof(ls->timings)) {
        ls->timings[ls->timings_idx].end_ut = now_ut;
        ls->timings_idx++;
        ls->timings[ls->timings_idx].start_ut = now_ut;
        ls->timings[ls->timings_idx].name = name;
    }
    else if(ls->timings_idx + 1 == _countof(ls->timings)) {
        ls->timings[ls->timings_idx].end_ut = now_ut;
        ls->timings_idx++; // out of bounds
    }
}

static void local_sockets_track_time_by_protocol(LS_STATE *ls, bool mnl, uint16_t family, uint16_t protocol) {
    if(mnl) {
        if(family == AF_INET) {
            if(protocol == IPPROTO_TCP)
                local_sockets_track_time(ls, "mnl_read_tcp4");
            else if(protocol == IPPROTO_UDP)
                local_sockets_track_time(ls, "mnl_read_udp4");
        }
        else if(family == AF_INET6) {
            if(protocol == IPPROTO_TCP)
                local_sockets_track_time(ls, "mnl_read_tcp6");
            else if(protocol == IPPROTO_UDP)
                local_sockets_track_time(ls, "mnl_read_udp6");
        }
        else
            local_sockets_track_time(ls, "mnl_read_unknown");
    }
    else {
        if(family == AF_INET) {
            if(protocol == IPPROTO_TCP)
                local_sockets_track_time(ls, "proc_read_tcp4");
            else if(protocol == IPPROTO_UDP)
                local_sockets_track_time(ls, "proc_read_udp4");
        }
        else if(family == AF_INET6) {
            if(protocol == IPPROTO_TCP)
                local_sockets_track_time(ls, "proc_read_tcp6");
            else if(protocol == IPPROTO_UDP)
                local_sockets_track_time(ls, "proc_read_udp6");
        }
        else
            local_sockets_track_time(ls, "proc_read_unknown");
    }
}

static inline void local_sockets_do_family_protocol(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
#if defined(HAVE_LIBMNL)
    if(!ls->config.no_mnl) {
        local_sockets_track_time_by_protocol(ls, true, family, protocol);
        if(local_sockets_libmnl_get_sockets(ls, family, protocol))
            return;

        // else, do proc
    }
#endif

    local_sockets_track_time_by_protocol(ls, false, family, protocol);

    if(ls->config.procfile)
        local_sockets_read_proc_net_x_procfile(ls, filename, family, protocol);
    else
        local_sockets_read_proc_net_x_getline(ls, filename, family, protocol);
}

static inline void local_sockets_read_all_system_sockets(LS_STATE *ls) {
    char path[FILENAME_MAX + 1];

    if(ls->config.namespaces) {
        local_sockets_track_time(ls, "read_namespaces");
        snprintfz(path, sizeof(path), "%s/proc/self/ns/net", ls->config.host_prefix);
        local_sockets_read_proc_inode_link(ls, path, &ls->proc_self_net_ns_inode, "net");
    }

    if(ls->config.cmdline || ls->config.comm || ls->config.pid || ls->config.namespaces) {
        local_sockets_track_time(ls, "proc_read_pids");
        snprintfz(path, sizeof(path), "%s/proc", ls->config.host_prefix);
        local_sockets_find_all_sockets_in_proc(ls, path);
    }

    if(ls->config.tcp4) {
        snprintfz(path, sizeof(path), "%s/proc/net/tcp", ls->config.host_prefix);
        local_sockets_do_family_protocol(ls, path, AF_INET, IPPROTO_TCP);
    }

    if(ls->config.udp4) {
        snprintfz(path, sizeof(path), "%s/proc/net/udp", ls->config.host_prefix);
        local_sockets_do_family_protocol(ls, path, AF_INET, IPPROTO_UDP);
    }

    if(ls->config.tcp6) {
        snprintfz(path, sizeof(path), "%s/proc/net/tcp6", ls->config.host_prefix);
        local_sockets_do_family_protocol(ls, path, AF_INET6, IPPROTO_TCP);
    }

    if(ls->config.udp6) {
        snprintfz(path, sizeof(path), "%s/proc/net/udp6", ls->config.host_prefix);
        local_sockets_do_family_protocol(ls, path, AF_INET6, IPPROTO_UDP);
    }
}

// --------------------------------------------------------------------------------------------------------------------
// switch namespaces to read namespace sockets

#if defined(LOCAL_SOCKETS_USE_SETNS)

struct local_sockets_child_work {
    int fd;
    uint64_t net_ns_inode;
};

#define LOCAL_SOCKET_TERMINATOR (struct local_socket) { \
    .expires = UINT32_MAX,                              \
    .timer = UINT8_MAX,                                 \
    .inode = UINT64_MAX,                                \
    .net_ns_inode = UINT64_MAX,                         \
}

static inline bool local_socket_is_terminator(const struct local_socket *n) {
    static const struct local_socket t = LOCAL_SOCKET_TERMINATOR;
    return (n->expires == t.expires &&
            n->timer == t.timer &&
            n->inode == t.inode &&
            n->net_ns_inode == t.net_ns_inode);
}

static inline void local_sockets_send_to_parent(struct local_socket_state *ls, const struct local_socket *n, void *data) {
    struct local_sockets_child_work *cw = data;
    int fd = cw->fd;

    if(!local_socket_is_terminator(n)) {
        ls->stats.errors_encountered = 0;
//        local_sockets_log(
//            ls,
//            "child is sending inode %"PRIu64" of namespace %"PRIu64", from namespace %"PRIu64" for pid %d",
//            n->inode, n->net_ns_inode, ls->proc_self_net_ns_inode, ls->ns_state.net_ns_pid);
    }

    if(write(fd, n, sizeof(*n)) != sizeof(*n))
        local_sockets_log(ls, "failed to write local socket to pipe");

    size_t len = n->cmdline ? string_strlen(n->cmdline) + 1 : 0;
    if(write(fd, &len, sizeof(len)) != sizeof(len))
        local_sockets_log(ls, "failed to write cmdline length to pipe");

    if(len)
        if(write(fd, string2str(n->cmdline), len) != (ssize_t)len)
            local_sockets_log(ls, "failed to write cmdline to pipe");
}

static inline int local_sockets_spawn_server_callback(SPAWN_REQUEST *request) {
    static const struct local_socket terminator = LOCAL_SOCKET_TERMINATOR;

    struct local_sockets_ns_req *req = (struct local_sockets_ns_req *)request->data;

    LS_STATE ls = { 0 };
    ls.config = req->config;
    ls.ns_state = req->ns_state;
    ls.ns_state.nl_seq += gettid_uncached() * 10;

    // we don't need these inside namespaces
    ls.config.cmdline = false;
    ls.config.comm = false;
    ls.config.pid = false;
    ls.config.namespaces = false;

#if !defined(USE_LIBMNL_AFTER_SETNS)
    ls.config.no_mnl = true; // disable mnl since this collects all sockets from the entire system
#endif

    // initialize local sockets
    local_sockets_init(&ls);
    ls.proc_self_net_ns_inode = ls.ns_state.net_ns_inode;
    ls.config.host_prefix = ""; // we need the /proc of the container

    struct local_sockets_child_work cw = {
        .net_ns_inode = ls.proc_self_net_ns_inode,
        .fd = request->fds[1], // stdout
    };

    ls.config.cb = local_sockets_send_to_parent;
    ls.config.data = &cw;

    // switch namespace using the custom fd passed via the spawn server
    if (setns(request->fds[3], CLONE_NEWNET) == -1) {
        local_sockets_log(&ls, "failed to switch network namespace at child process using fd %d", request->fds[3]);
        return EXIT_FAILURE;
    }

    // close the custom fd
    close(request->fds[3]); request->fds[3] = -1;

    // read all sockets from /proc
    local_sockets_read_all_system_sockets(&ls);

    // send all sockets to parent
    local_sockets_foreach_local_socket_call_cb(&ls);

    // send the terminating socket
    local_sockets_send_to_parent(&ls, &terminator, &cw);

    local_sockets_cleanup(&ls);

    return EXIT_SUCCESS;
}

static inline bool local_sockets_get_namespace_sockets_with_pid(LS_STATE *ls, struct pid_socket *ps) {
    char filename[1024];
    snprintfz(filename, sizeof(filename), "%s/proc/%d/ns/net", ls->config.host_prefix, ps->pid);

    // verify the pid is in the target namespace
    int fd = open(filename, O_RDONLY | O_CLOEXEC);
    if (fd == -1) {
        local_sockets_log(ls, "cannot open file '%s'", filename);
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_absent, 1, __ATOMIC_RELAXED);
        return false;
    }

    struct stat statbuf;
    if (fstat(fd, &statbuf) == -1) {
        close(fd);
        local_sockets_log(ls, "failed to get file statistics for '%s'", filename);
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_absent, 1, __ATOMIC_RELAXED);
        return false;
    }

    if (statbuf.st_ino != ps->net_ns_inode) {
        close(fd);
        local_sockets_log(ls, "pid %d is not in the wanted network namespace", ps->pid);
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_invalid, 1, __ATOMIC_RELAXED);
        return false;
    }

    if(ls->spawn_server == NULL) {
        close(fd);
        local_sockets_log(ls, "spawn server is not available");
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_forks_failed, 1, __ATOMIC_RELAXED);
        return false;
    }

    struct local_sockets_ns_req req = {
        .config = ls->config,
        .ns_state = ls->ns_state,
    };
    req.ns_state.net_ns_pid = ps->pid;
    req.ns_state.net_ns_inode = ps->net_ns_inode;

    SPAWN_INSTANCE *si = spawn_server_exec(ls->spawn_server, STDERR_FILENO, fd, NULL, &req, sizeof(req), SPAWN_INSTANCE_TYPE_CALLBACK);
    close(fd); fd = -1;

    if(ls->config.report)
        __atomic_add_fetch(&ls->stats.namespaces_forks_attempted, 1, __ATOMIC_RELAXED);

    if(si == NULL) {
        local_sockets_log(ls, "cannot create spawn instance");

        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_forks_failed, 1, __ATOMIC_RELAXED);

        return false;
    }

    size_t received = 0;
    struct local_socket buf;
    while(read(spawn_server_instance_read_fd(si), &buf, sizeof(buf)) == sizeof(buf)) {
        size_t len = 0;
        if(read(spawn_server_instance_read_fd(si), &len, sizeof(len)) != sizeof(len))
            local_sockets_log(ls, "failed to read cmdline length from pipe");

        if(len) {
            char cmdline[len + 1];
            if(read(spawn_server_instance_read_fd(si), cmdline, len) != (ssize_t)len)
                local_sockets_log(ls, "failed to read cmdline from pipe");
            else {
                cmdline[len] = '\0';
                buf.cmdline = string_strdupz(cmdline);
            }
        }
        else
            buf.cmdline = NULL;

        received++;

        if(local_socket_is_terminator(&buf))
            // the child finished
            break;

        // overwrite the net_ns_inode we receive
        buf.net_ns_inode = ps->net_ns_inode;

        spinlock_lock(&ls->spinlock);

        if(!local_sockets_add_socket(ls, &buf)) {
            // fprintf(stderr, "Failed to add duplicate namespace socket inode %"PRIu64"\n", buf.inode);
            string_freez(buf.cmdline);
            if(ls->config.report)
                __atomic_add_fetch(&ls->stats.namespaces_sockets_existing, 1, __ATOMIC_RELAXED);
        }
        else {
            // fprintf(stderr, "Added namespace socket inode %"PRIu64"\n", buf.inode);
            if(ls->config.report)
                __atomic_add_fetch(&ls->stats.namespaces_sockets_new, 1, __ATOMIC_RELAXED);
        }

        spinlock_unlock(&ls->spinlock);
    }

    spawn_server_exec_kill(ls->spawn_server, si, 0);

    if(ls->config.report && received == 0)
        __atomic_add_fetch(&ls->stats.namespaces_forks_unresponsive, 1, __ATOMIC_RELAXED);

    return received > 0;
}

struct local_sockets_namespace_worker {
    LS_STATE *ls;
    uint64_t inode;
};

static inline void *local_sockets_get_namespace_sockets_worker(void *arg) {
    struct local_sockets_namespace_worker *data = arg;
    LS_STATE *ls = data->ls;
    const uint64_t inode = data->inode;

    spinlock_lock(&ls->spinlock);

    // find a pid_socket that has this namespace
    for(SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl_pid = simple_hashtable_first_read_only_PID_SOCKET(&ls->pid_sockets_hashtable) ;
        sl_pid ;
        sl_pid = simple_hashtable_next_read_only_PID_SOCKET(&ls->pid_sockets_hashtable, sl_pid)) {
        struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl_pid);
        if(!ps || ps->net_ns_inode != inode) continue;

        // now we have a pid that has the same namespace inode

        spinlock_unlock(&ls->spinlock);
        const bool worked = local_sockets_get_namespace_sockets_with_pid(ls, ps);
        spinlock_lock(&ls->spinlock);

        if(worked)
            break;
    }

    spinlock_unlock(&ls->spinlock);

    return NULL;
}

static inline void local_sockets_namespaces(LS_STATE *ls) {
    size_t threads = ls->config.max_concurrent_namespaces;
    if(threads == 0) threads = 5;
    if(threads > 100) threads = 100;

    size_t last_thread = 0;
    ND_THREAD *workers[threads];
    struct local_sockets_namespace_worker workers_data[threads];
    memset(workers, 0, sizeof(workers));
    memset(workers_data, 0, sizeof(workers_data));

    spinlock_lock(&ls->spinlock);

    for(SIMPLE_HASHTABLE_SLOT_NET_NS *sl = simple_hashtable_first_read_only_NET_NS(&ls->ns_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_NET_NS(&ls->ns_hashtable, sl)) {
         const uint64_t inode = (uint64_t)SIMPLE_HASHTABLE_SLOT_DATA(sl);

        if(inode == ls->proc_self_net_ns_inode)
            // skip our own namespace, we already have them
            continue;

        spinlock_unlock(&ls->spinlock);

        ls->stats.namespaces_found++;

        if(workers[last_thread] != NULL) {
            if(++last_thread >= threads)
                last_thread = 0;

            if(workers[last_thread]) {
                nd_thread_join(workers[last_thread]);
                workers[last_thread] = NULL;
            }
        }

        workers_data[last_thread].ls = ls;
        workers_data[last_thread].inode = inode;
        workers[last_thread] = nd_thread_create(
            "local-sockets-worker",
            NETDATA_THREAD_OPTION_JOINABLE,
            local_sockets_get_namespace_sockets_worker,
            &workers_data[last_thread]);

        spinlock_lock(&ls->spinlock);
    }

    spinlock_unlock(&ls->spinlock);

    // wait all the threads running
    for(size_t i = 0; i < threads ;i++) {
        if(workers[i])
            nd_thread_join(workers[i]);
    }
}

#endif // LOCAL_SOCKETS_USE_SETNS

// --------------------------------------------------------------------------------------------------------------------
// read namespace sockets from the host's /proc

#if !defined(LOCAL_SOCKETS_USE_SETNS)

static inline bool local_sockets_namespaces_from_proc_with_pid(LS_STATE *ls, struct pid_socket *ps) {
    char filename[1024];
    snprintfz(filename, sizeof(filename), "%s/proc/%d/ns/net", ls->config.host_prefix, ps->pid);

    // verify the pid is in the target namespace
    int fd = open(filename, O_RDONLY | O_CLOEXEC);
    if (fd == -1) {
        local_sockets_log(ls, "cannot open file '%s'", filename);
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_absent, 1, __ATOMIC_RELAXED);
        return false;
    }

    struct stat statbuf;
    if (fstat(fd, &statbuf) == -1) {
        close(fd);
        local_sockets_log(ls, "failed to get file statistics for '%s'", filename);
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_absent, 1, __ATOMIC_RELAXED);
        return false;
    }

    if (statbuf.st_ino != ps->net_ns_inode) {
        close(fd);
        local_sockets_log(ls, "pid %d is not in the wanted network namespace", ps->pid);
        if(ls->config.report)
            __atomic_add_fetch(&ls->stats.namespaces_invalid, 1, __ATOMIC_RELAXED);
        return false;
    }

    char path[FILENAME_MAX + 1];

    if(ls->config.tcp4) {
        snprintfz(path, sizeof(path), "%s/proc/%d/net/tcp", ls->config.host_prefix, ps->pid);
        if(!local_sockets_read_proc_net_x(ls, path, AF_INET, IPPROTO_TCP))
            return false;
    }

    if(ls->config.udp4) {
        snprintfz(path, sizeof(path), "%s/proc/%d/net/udp", ls->config.host_prefix, ps->pid);
        if(!local_sockets_read_proc_net_x(ls, path, AF_INET, IPPROTO_UDP))
            return false;
    }

    if(ls->config.tcp6) {
        snprintfz(path, sizeof(path), "%s/proc/%d/net/tcp6", ls->config.host_prefix, ps->pid);
        if(!local_sockets_read_proc_net_x(ls, path, AF_INET6, IPPROTO_TCP))
            return false;
    }

    if(ls->config.udp6) {
        snprintfz(path, sizeof(path), "%s/proc/%d/net/udp6", ls->config.host_prefix, ps->pid);
        if(!local_sockets_read_proc_net_x(ls, path, AF_INET6, IPPROTO_UDP))
            return false;
    }

    return true;
}

static inline void local_sockets_namespaces_from_proc(LS_STATE *ls) {
    for(SIMPLE_HASHTABLE_SLOT_NET_NS *sl = simple_hashtable_first_read_only_NET_NS(&ls->ns_hashtable);
         sl;
         sl = simple_hashtable_next_read_only_NET_NS(&ls->ns_hashtable, sl)) {
        const uint64_t inode = (uint64_t)SIMPLE_HASHTABLE_SLOT_DATA(sl);

        if (inode == ls->proc_self_net_ns_inode)
            // skip our own namespace, we already have them
            continue;

        ls->stats.namespaces_found++;

        for(SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl_pid = simple_hashtable_first_read_only_PID_SOCKET(&ls->pid_sockets_hashtable) ;
             sl_pid ;
             sl_pid = simple_hashtable_next_read_only_PID_SOCKET(&ls->pid_sockets_hashtable, sl_pid)) {
            struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl_pid);
            if(!ps || ps->net_ns_inode != inode) continue;

            // now we have a pid that has the same namespace inode

            if(local_sockets_namespaces_from_proc_with_pid(ls, ps))
                break;
        }
    }
}

#endif

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_process(LS_STATE *ls) {
    ls->timings_idx = 0;
    local_sockets_track_time(ls, "init");

    // initialize our hashtables
    local_sockets_init(ls);

    local_sockets_track_time(ls, "all_sockets");

    // read all sockets from /proc
    local_sockets_read_all_system_sockets(ls);

    // check all socket namespaces
    if(ls->config.namespaces) {
        local_sockets_track_time(ls, "switch_namespaces");
#if defined(LOCAL_SOCKETS_USE_SETNS)
        local_sockets_namespaces(ls);
#else
        local_sockets_namespaces_from_proc(ls);
#endif
    }

    // detect the directions of the sockets
    if(ls->config.inbound || ls->config.outbound || ls->config.local) {
        local_sockets_track_time(ls, "detect_direction");
        local_sockets_detect_directions(ls);
    }

    // call the callback for each socket
    local_sockets_track_time(ls, "output");
    local_sockets_foreach_local_socket_call_cb(ls);

    // free all memory
    local_sockets_track_time(ls, "cleanup");
    local_sockets_cleanup(ls);
}

#endif //NETDATA_LOCAL_SOCKETS_H
