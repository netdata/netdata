#ifndef NETDATA_RRDPUSH_H
#define NETDATA_RRDPUSH_H

extern int rrdpush_enabled;
extern int rrdpush_exclusive;

extern int rrdpush_init();
extern void rrdset_done_push(RRDSET *st);
extern void *central_netdata_push_thread(void *ptr);

#endif //NETDATA_RRDPUSH_H
