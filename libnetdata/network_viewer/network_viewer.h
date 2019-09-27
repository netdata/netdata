#ifndef NETDATA_NETWORK_VIEWER
# define NETDATA_NETWORK_VIEWER

# include "../libnetdata.h"

# ifdef __FreeBSD__

# include <sys/param.h>
# include <sys/socketvar.h>
# include <sys/sysctl.h>
# include <sys/file.h>
# include <sys/user.h>

# include <sys/un.h>
# define _WANT_UNPCB
# include <sys/unpcb.h>

# include <net/route.h>

# include <netinet/in_pcb.h>
# include <netinet/sctp.h>
# include <netinet/tcp.h>
# define TCPSTATES /* load state names */
# include <netinet/tcp_fsm.h>
# include <netinet/tcp_seq.h>
# include <netinet/tcp_var.h>
# include <jail.h>

# else

# include <asm/types.h>
# include <linux/netlink.h>
# include <linux/rtnetlink.h>
//# include <linux/tcp.h>
# include <linux/sock_diag.h>
# include <linux/inet_diag.h>

# endif

#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <netdb.h>
#include <pwd.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#endif
