// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LOCAL_SOCKETS_H
#define NETDATA_LOCAL_SOCKETS_H

#include "libnetdata/libnetdata.h"

// disable libmnl for the moment
#undef HAVE_LIBMNL

#ifdef HAVE_LIBMNL
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
        bool uid;
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

#ifdef HAVE_LIBMNL
    bool use_nl;
    struct mnl_socket *nl;
    uint16_t tmp_protocol;
#endif

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
    bool use_ebpf;
    ebpf_module_t *ebpf_module;
#endif

    ARAL *local_socket_aral;
    ARAL *pid_socket_aral;

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
    uint8_t retransmits;
    uint32_t expires;
    uint32_t rqueue;
    uint32_t wqueue;
    uid_t uid;

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

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_log(LS_STATE *ls, const char *format, ...) PRINTFLIKE(2, 3);
#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
enum ebpf_nv_tables_list {
    NETWORK_VIEWER_EBPF_NV_SOCKET,
    NETWORK_VIEWER_EBPF_NV_CONTROL
};

typedef struct ebpf_nv_idx {
    union ipv46 saddr;
    uint16_t sport;
    union ipv46 daddr;
    uint16_t dport;
} ebpf_nv_idx_t;

typedef struct ebpf_nv_data {
    int state;

    uint32_t pid;
    uint32_t uid;
    uint64_t ts;

    uint8_t timer;
    uint8_t retransmits;
    uint32_t expires;
    uint32_t rqueue;
    uint32_t wqueue;

    char name[TASK_COMM_LEN];

    uint32_t direction;

    uint16_t family;
    uint16_t protocol;
} ebpf_nv_data_t;

static inline void local_sockets_ebpf_selector(LS_STATE *ls) {
    // We loaded with success eBPF codes
    if (ls->ebpf_module->maps && ls->ebpf_module->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd != -1)
        ls->use_ebpf = true;
}

static inline bool local_sockets_ebpf_get_sockets(LS_STATE *ls) {
    ebpf_nv_idx_t key =  { };
    ebpf_nv_idx_t next_key = { };
    ebpf_nv_data_t stored = {};
    int fd = ls->ebpf_module->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd;
    while (!bpf_map_get_next_key(fd, &key, &next_key)) {
        if (bpf_map_lookup_elem(fd, &key, &stored)) {
            goto end_socket_read_loop;
        }

        LOCAL_SOCKET n = {
            .inode = stored.ts,
            .direction = SOCKET_DIRECTION_NONE,
            .state = stored.state,
            .local = {
                .family = stored.family,
                .protocol = stored.protocol,
                .port = key.sport,
             },
            .remote = {
                .family = stored.family,
                .protocol = stored.protocol,
                .port = key.dport,
            },
            .timer = stored.timer,
            .retransmits = stored.retransmits,
            .expires = stored.expires,
            .rqueue = stored.rqueue,
            .wqueue = stored.wqueue,
            .uid = stored.uid,
           };

        if (stored.family == AF_INET) {
            memcpy(&n.local.ip.ipv4, &key.saddr.ipv4, sizeof(n.local.ip.ipv4));
            memcpy(&n.remote.ip.ipv4, &key.daddr.ipv4, sizeof(n.remote.ip.ipv4));
        }
        else if (stored.family == AF_INET6) {
            memcpy(&n.local.ip.ipv6, &key.saddr.ipv6, sizeof(n.local.ip.ipv6));
            memcpy(&n.remote.ip.ipv6, &key.daddr.ipv6, sizeof(n.remote.ip.ipv6));
        }

        strncpyz(n.comm, stored.name, sizeof(n.comm) - 1);

end_socket_read_loop:
        key = next_key;
        // cleanup avoiding garbage from previous socket
        memset(&stored, 0, sizeof(stored));
    }
    return true;
}

#endif // defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)

// --------------------------------------------------------------------------------------------------------------------

static inline void local_sockets_log(LS_STATE *ls, const char *format, ...) {
    if(++ls->stats.errors_encountered == ls->config.max_errors) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-SOCKETS: max number of logs reached. Not logging anymore");
        return;
    }

    if(ls->stats.errors_encountered > ls->config.max_errors)
        return;

    char buf[16384];
    va_list args;
    va_start(args, format);
    vsnprintf(buf, sizeof(buf), format, args);
    va_end(args);

    nd_log(NDLS_COLLECTORS, NDLP_ERR, "LOCAL-SOCKETS: %s", buf);
}

// --------------------------------------------------------------------------------------------------------------------

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
static void local_sockets_ebpf_store_sockets(LS_STATE *ls, LOCAL_SOCKET *n);
#endif

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

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
            local_sockets_ebpf_store_sockets(ls, n);
#endif
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

            SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl = simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, inode, &inode, true);
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
                        SIMPLE_HASHTABLE_SLOT_NET_NS *sl_ns = simple_hashtable_get_slot_NET_NS(&ls->ns_hashtable, net_ns_inode, (uint64_t *)net_ns_inode, true);
                        simple_hashtable_set_slot_NET_NS(&ls->ns_hashtable, sl_ns, net_ns_inode, (uint64_t *)net_ns_inode);
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
                simple_hashtable_set_slot_PID_SOCKET(&ls->pid_sockets_hashtable, sl, inode, ps);
            }
        }

        closedir(fd_dir);
    }

    closedir(proc_dir);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------

static bool local_sockets_is_ipv4_mapped_ipv6_address(const struct in6_addr *addr) {
    // An IPv4-mapped IPv6 address starts with 80 bits of zeros followed by 16 bits of ones
    static const unsigned char ipv4_mapped_prefix[12] = { 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF };
    return memcmp(addr->s6_addr, ipv4_mapped_prefix, 12) == 0;
}

static bool local_sockets_is_loopback_address(struct socket_endpoint *se) {
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

static inline bool local_sockets_is_private_address(struct socket_endpoint *se) {
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

static bool local_sockets_is_multicast_address(struct socket_endpoint *se) {
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

static bool local_sockets_is_zero_address(struct socket_endpoint *se) {
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

static inline const char *local_sockets_address_space(struct socket_endpoint *se) {
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

    SIMPLE_HASHTABLE_SLOT_LOCAL_SOCKET *sl = simple_hashtable_get_slot_LOCAL_SOCKET(&ls->sockets_hashtable, tmp->inode, &tmp->inode, true);
    LOCAL_SOCKET *n = SIMPLE_HASHTABLE_SLOT_DATA(sl);
    if(n) {
        local_sockets_log(ls, "inode %" PRIu64" already exists in hashtable - ignoring duplicate", tmp->inode);
        return false;
    }

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

    SIMPLE_HASHTABLE_SLOT_PID_SOCKET *sl_pid = simple_hashtable_get_slot_PID_SOCKET(&ls->pid_sockets_hashtable, n->inode, &n->inode, false);
    struct pid_socket *ps = SIMPLE_HASHTABLE_SLOT_DATA(sl_pid);
    if(ps) {
        n->net_ns_inode = ps->net_ns_inode;
        n->pid = ps->pid;

        if(ps->uid != UID_UNSET && n->uid == UID_UNSET)
            n->uid = ps->uid;

        if(ps->cmdline)
            n->cmdline = string_strdupz(ps->cmdline);

        strncpyz(n->comm, ps->comm, sizeof(n->comm) - 1);
    }

    // --- index it -----------------------------------------------------------------------------------------------

    simple_hashtable_set_slot_LOCAL_SOCKET(&ls->sockets_hashtable, sl, n->inode, n);

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

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
enum ebpf_nv_tables_list {
    NETWORK_VIEWER_EBPF_NV_SOCKET,
    NETWORK_VIEWER_EBPF_NV_CONTROL
};

enum ebpf_nv_load_data {
    NETWORK_VIEWER_EBPF_NV_LOAD_DATA = 1<<0,
    NETWORK_VIEWER_EBPF_NV_ONLY_READ = 1<<1,
    NETWORK_VEIWER_EBPF_NV_CLEANUP = 1<<2
};

#define NETWORK_VIEWER_EBPF_ACTION_LIMIT 10

typedef struct ebpf_nv_idx {
    union ipv46 saddr;
    uint16_t sport;
    union ipv46 daddr;
    uint16_t dport;
} ebpf_nv_idx_t;

typedef struct ebpf_nv_data {
    int state;

    uint32_t pid;
    uint32_t uid;
    uint64_t ts;

    uint8_t timer;
    uint8_t retransmits;
    uint16_t closed;
    uint32_t expires;
    uint32_t rqueue;
    uint32_t wqueue;

    char name[TASK_COMM_LEN];

    SOCKET_DIRECTION direction;

    uint16_t family;
    uint16_t protocol;
} ebpf_nv_data_t;

static inline void local_sockets_ebpf_selector(LS_STATE *ls) {
    // We loaded with success eBPF codes
    if (ls->ebpf_module->maps && ls->ebpf_module->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd != -1)
        ls->use_ebpf = true;
}

static inline bool local_sockets_ebpf_use_protocol(LS_STATE *ls, ebpf_nv_data_t *data) {
    if (data->protocol == IPPROTO_TCP && (ls->config.tcp4 || ls->config.tcp6))
        return  true;
    else if (data->protocol == IPPROTO_UDP && (ls->config.udp4 || ls->config.udp6))
        return  true;

    return false;
}

static inline void local_sockets_reset_ebpf_value(LS_STATE *ls, uint64_t removed)
{
    int ctrl_fd = ls->ebpf_module->maps[NETWORK_VIEWER_EBPF_NV_CONTROL].map_fd;
    uint32_t control = NETDATA_CONTROLLER_PID_TABLE_ADD;
    uint64_t current_value = 0;
    if (!bpf_map_lookup_elem(ctrl_fd, &control, &current_value)) {
        current_value -= removed;
        if (bpf_map_update_elem(ctrl_fd, &control, &current_value, 0))
            nd_log(NDLS_COLLECTORS, NDLP_ERR, "PLUGIN: cannot reset value inside table.");
    }
}

static inline bool local_sockets_ebpf_get_sockets(LS_STATE *ls, enum ebpf_nv_load_data action) {
    ebpf_nv_idx_t key =  { };
    ebpf_nv_idx_t next_key = { };
    ebpf_nv_data_t stored = {};

    char path[FILENAME_MAX + 1];

    if(ls->config.namespaces) {
        snprintfz(path, sizeof(path), "%s/proc/self/ns/net", ls->config.host_prefix);
        local_sockets_read_proc_inode_link(ls, path, &ls->proc_self_net_ns_inode, "net");
    }

    int fd = ls->ebpf_module->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd;
    uint64_t counter = 0;
    uint64_t removed = 0;
    bool cleanup = (action & NETWORK_VEIWER_EBPF_NV_CLEANUP);
    while (!bpf_map_get_next_key(fd, &key, &next_key)) {
        if (bpf_map_lookup_elem(fd, &key, &stored)) {
            goto end_socket_read_loop;
        }

        if (!local_sockets_ebpf_use_protocol(ls, &stored)) {
            // Socket not allowed, let us remove it
            bpf_map_delete_elem(fd, &key);
            goto end_socket_read_loop;
        }

        counter++;
        LOCAL_SOCKET n = {
            .inode = stored.ts,
            .direction = stored.direction,
            .state = stored.state,
            .local = {
                .family = stored.family,
                .protocol = stored.protocol,
                .port = key.sport,
            },
            .remote = {
                .family = stored.family,
                .protocol = stored.protocol,
                .port = key.dport,
            },
            .timer = stored.timer,
            .retransmits = stored.retransmits,
            .expires = stored.expires,
            .rqueue = stored.rqueue,
            .wqueue = stored.wqueue,
            .uid = stored.uid,
        };

        if (stored.family == AF_INET) {
            memcpy(&n.local.ip.ipv4, &key.saddr.ipv4, sizeof(n.local.ip.ipv4));
            memcpy(&n.remote.ip.ipv4, &key.daddr.ipv4, sizeof(n.remote.ip.ipv4));
        }
        else if (stored.family == AF_INET6) {
            memcpy(&n.local.ip.ipv6, &key.saddr.ipv6, sizeof(n.local.ip.ipv6));
            memcpy(&n.remote.ip.ipv6, &key.daddr.ipv6, sizeof(n.remote.ip.ipv6));
        }

        strncpyz(n.comm, stored.name, sizeof(n.comm) - 1);
        local_sockets_add_socket(ls, &n);

end_socket_read_loop:
        key = next_key;
        if (cleanup && stored.closed) {
            removed++;
            bpf_map_delete_elem(fd, &key);
        }
    }

    if (removed) {
        local_sockets_reset_ebpf_value(ls, removed);
    }

    return (!counter) ? false : true;
}

static inline void local_sockets_ebpf_store_sockets(LS_STATE *ls, LOCAL_SOCKET *n) {
    ebpf_nv_idx_t key =  { };
    ebpf_nv_data_t data = {};

    key.sport = n->local_port_key.port;
    key.dport = n->remote.port;
    if (n->local.family == AF_INET) {
        key.saddr.ipv4 = n->local.ip.ipv4;
        key.daddr.ipv4 = n->remote.ip.ipv4;
    }
    else {
        memcpy(&key.saddr.ipv6, &n->local.ip.ipv6, sizeof(n->local.ip.ipv6));
        memcpy(&key.daddr.ipv6, &n->remote.ip.ipv6, sizeof(n->local.ip.ipv6));
    }

    data.protocol = n->local.protocol;
    data.family = n->local.family;
    data.uid = n->uid;
    data.pid = n->pid;
    data.direction = n->direction;
    data.ts = n->inode;
    strncpyz(data.name, n->comm, sizeof(n->comm) - 1);

    int fd = ls->ebpf_module->maps[NETWORK_VIEWER_EBPF_NV_SOCKET].map_fd;
    if (bpf_map_update_elem(fd, &key, &data, BPF_ANY))
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "PLUGIN: cannot insert value inside table.");
}

#endif // defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)

#ifdef HAVE_LIBMNL

static inline void local_sockets_netlink_init(LS_STATE *ls) {
    ls->use_nl = true;
    ls->nl = mnl_socket_open(NETLINK_INET_DIAG);
    if (!ls->nl) {
        local_sockets_log(ls, "cannot open netlink socket");
        ls->use_nl = false;
    }

    if (mnl_socket_bind(ls->nl, 0, MNL_SOCKET_AUTOPID) < 0) {
        local_sockets_log(ls, "cannot bind netlink socket");
        ls->use_nl = false;
    }
}

static inline void local_sockets_netlink_cleanup(LS_STATE *ls) {
    if(ls->nl) {
        mnl_socket_close(ls->nl);
        ls->nl = NULL;
    }
}

static inline int local_sockets_netlink_cb_data(const struct nlmsghdr *nlh, void *data) {
    LS_STATE *ls = data;

    struct inet_diag_msg *diag_msg = mnl_nlmsg_get_payload(nlh);

    LOCAL_SOCKET n = {
        .inode = diag_msg->idiag_inode,
        .direction = SOCKET_DIRECTION_NONE,
        .state = diag_msg->idiag_state,
        .local = {
            .protocol = ls->tmp_protocol,
            .family = diag_msg->idiag_family,
            .port = diag_msg->id.idiag_sport,
        },
        .remote = {
            .protocol = ls->tmp_protocol,
            .family = diag_msg->idiag_family,
            .port = diag_msg->id.idiag_dport,
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

    local_sockets_add_socket(ls, &n);

    return MNL_CB_OK;
}

static inline bool local_sockets_netlink_get_sockets(LS_STATE *ls, uint16_t family, uint16_t protocol) {
    ls->tmp_protocol = protocol;

    char buf[MNL_SOCKET_BUFFER_SIZE];
    struct nlmsghdr *nlh;
    struct inet_diag_req_v2 req;
    unsigned int seq, portid = mnl_socket_get_portid(ls->nl);

    memset(&req, 0, sizeof(req));
    req.sdiag_family = family;
    req.sdiag_protocol = protocol;
    req.idiag_states = -1;

    nlh = mnl_nlmsg_put_header(buf);
    nlh->nlmsg_type = SOCK_DIAG_BY_FAMILY;
    nlh->nlmsg_flags = NLM_F_DUMP | NLM_F_REQUEST;
    nlh->nlmsg_seq = seq = time(NULL);
    mnl_nlmsg_put_extra_header(nlh, sizeof(req));
    memcpy(mnl_nlmsg_get_payload(nlh), &req, sizeof(req));

    if (mnl_socket_sendto(ls->nl, nlh, nlh->nlmsg_len) < 0) {
        local_sockets_log(ls, "mnl_socket_send failed");
        return false;
    }

    ssize_t ret;
    while ((ret = mnl_socket_recvfrom(ls->nl, buf, sizeof(buf))) > 0) {
        ret = mnl_cb_run(buf, ret, seq, portid, local_sockets_netlink_cb_data, ls);
        if (ret <= MNL_CB_STOP)
            break;
    }
    if (ret == -1) {
        local_sockets_log(ls, "mnl_socket_recvfrom");
        return false;
    }

    return true;
}
#endif // HAVE_LIBMNL

static inline bool local_sockets_read_proc_net_x(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
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

        LOCAL_SOCKET n = {
            .direction = SOCKET_DIRECTION_NONE,
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

        char *words[32];
        size_t num_words = quoted_strings_splitter(line, words, 32, is_space);
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
            local_sockets_log(ls, "cannot parse ipv4 line No %zu of filename '%s'", counter, filename);
            continue;
        }

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
    }

    fclose(fp);

    if (line)
        free(line); // no freez() here because getline() may resize

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
    simple_hashtable_init_NET_NS(&ls->ns_hashtable, 1024);
    simple_hashtable_init_PID_SOCKET(&ls->pid_sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_SOCKET(&ls->sockets_hashtable, 65535);
    simple_hashtable_init_LOCAL_IP(&ls->local_ips_hashtable, 4096);
    simple_hashtable_init_LISTENING_PORT(&ls->listening_ports_hashtable, 4096);

    ls->local_socket_aral = aral_create(
        "local-sockets",
        sizeof(LOCAL_SOCKET),
        65536,
        65536,
        NULL, NULL, NULL, false, true);

    ls->pid_socket_aral = aral_create(
        "pid-sockets",
        sizeof(struct pid_socket),
        65536,
        65536,
        NULL, NULL, NULL, false, true);
}

static inline void local_sockets_cleanup(LS_STATE *ls) {
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

static inline void local_sockets_do_family_protocol(LS_STATE *ls, const char *filename, uint16_t family, uint16_t protocol) {
#ifdef HAVE_LIBMNL
    if(ls->use_nl) {
        ls->use_nl = local_sockets_netlink_get_sockets(ls, family, protocol);

        if(ls->use_nl)
            return;
    }
#endif

    local_sockets_read_proc_net_x(ls, filename, family, protocol);
}

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

    size_t len = n->cmdline ? string_strlen(n->cmdline) + 1 : 0;
    if(write(fd, &len, sizeof(len)) != sizeof(len))
        local_sockets_log(ls, "failed to write cmdline length to pipe");

    if(len)
        if(write(fd, string2str(n->cmdline), len) != (ssize_t)len)
            local_sockets_log(ls, "failed to write cmdline to pipe");
}

static inline bool local_sockets_get_namespace_sockets(LS_STATE *ls, struct pid_socket *ps, pid_t *pid) {
    char filename[1024];
    snprintfz(filename, sizeof(filename), "%s/proc/%d/ns/net", ls->config.host_prefix, ps->pid);

    // verify the pid is in the target namespace
    int fd = open(filename, O_RDONLY | O_CLOEXEC);
    if (fd == -1) {
        local_sockets_log(ls, "cannot open file '%s'", filename);
        return false;
    }

    struct stat statbuf;
    if (fstat(fd, &statbuf) == -1) {
        close(fd);
        local_sockets_log(ls, "failed to get file statistics for '%s'", filename);
        return false;
    }

    if (statbuf.st_ino != ps->net_ns_inode) {
        close(fd);
        local_sockets_log(ls, "pid %d is not in the wanted network namespace", ps->pid);
        return false;
    }

    int pipefd[2];
    if (pipe(pipefd) != 0) {
        local_sockets_log(ls, "cannot create pipe");
        close(fd);
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

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
        local_sockets_ebpf_selector(ls);
#endif

#ifdef HAVE_LIBMNL
        local_sockets_netlink_cleanup(ls);
        local_sockets_netlink_init(ls);
#endif

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
        if (ls->use_ebpf && ls->ebpf_module->optional == NETWORK_VIEWER_EBPF_NV_ONLY_READ) {
            ls->use_ebpf =  local_sockets_ebpf_get_sockets(ls, NETWORK_VEIWER_EBPF_NV_CLEANUP);
        } else
#endif
            // read all sockets from /proc
            local_sockets_read_sockets_from_proc(ls);

        // send all sockets to parent
        local_sockets_foreach_local_socket_call_cb(ls);

        // send the terminating socket
        struct local_socket zero = {
            .net_ns_inode = ps->net_ns_inode,
        };
        local_sockets_send_to_parent(ls, &zero, &cw);

#ifdef HAVE_LIBMNL
        local_sockets_netlink_cleanup(ls);
#endif

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
            char cmdline[len + 1];
            if(read(pipefd[0], cmdline, len) != (ssize_t)len)
                local_sockets_log(ls, "failed to read cmdline from pipe");
            else {
                cmdline[len] = '\0';
                buf.cmdline = string_strdupz(cmdline);
            }
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
            string_freez(buf.cmdline);

//            local_sockets_log(ls,
//                              "ns inode %" PRIu64" (comm: '%s', pid: %u, ns: %"PRIu64") already exists in hashtable (comm: '%s', pid: %u, ns: %"PRIu64") - ignoring duplicate",
//                              buf.inode, buf.comm, buf.pid, buf.net_ns_inode, n->comm, n->pid, n->net_ns_inode);
            continue;
        }
        else {
            n = aral_mallocz(ls->local_socket_aral);
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

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
    local_sockets_ebpf_selector(ls);
#endif

#ifdef HAVE_LIBMNL
    local_sockets_netlink_init(ls);
#endif

    ls->config.host_prefix = netdata_configured_host_prefix;

    // initialize our hashtables
    local_sockets_init(ls);

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
    if (ls->use_ebpf && ls->ebpf_module->optional == NETWORK_VIEWER_EBPF_NV_ONLY_READ) {
        ls->use_ebpf =  local_sockets_ebpf_get_sockets(ls, NETWORK_VIEWER_EBPF_NV_LOAD_DATA);
    } else
#endif
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

#ifdef HAVE_LIBMNL
    local_sockets_netlink_cleanup(ls);
#endif

#if defined(ENABLE_PLUGIN_EBPF) && !defined(__cplusplus)
    static int load_again = 0;
    if (ls->ebpf_module->optional == NETWORK_VIEWER_EBPF_NV_LOAD_DATA && ls->use_ebpf) {
        ls->ebpf_module->optional = NETWORK_VIEWER_EBPF_NV_ONLY_READ;
    }
    else if (load_again == NETWORK_VIEWER_EBPF_ACTION_LIMIT) {
        ls->ebpf_module->optional = NETWORK_VIEWER_EBPF_NV_LOAD_DATA;
        load_again = 0;
    }

    load_again++;
#endif
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
