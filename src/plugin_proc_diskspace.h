#ifndef NETDATA_PLUGIN_PROC_DISKSPACE_H
#define NETDATA_PLUGIN_PROC_DISKSPACE_H

/**
 * @file plugin_proc_diskspace.h
 * @brief The proc plugin diskspace thread.
 */
 
/** 
 * Method run by the proc plugin diskspace thread.
 *
 * Collecting diskspace is not done by proc_main() because this may be slow.
 * Collecting it in another thread stops freezing all proc datacollection if
 * this is slow.
 *
 * @param ptr to struct netdata_static_thread
 * @return NULL
 */
extern void *proc_diskspace_main(void *ptr);

#endif //NETDATA_PLUGIN_PROC_DISKSPACE_H
