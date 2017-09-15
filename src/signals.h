#ifndef NETDATA_SIGNALS_H
#define NETDATA_SIGNALS_H

extern void sig_handler_exit(int signo);
extern void sig_handler_save(int signo);
extern void sig_handler_logrotate(int signo);
extern void sig_handler_reload_health(int signo);

extern void signals_init(void);
extern void signals_block(void);
extern void signals_unblock(void);
extern void signals_handle(void);
extern void signals_reset(void);

#endif //NETDATA_SIGNALS_H
