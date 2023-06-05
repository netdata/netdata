#ifndef NETDATA_EBPF_PLUGIN_UNITTEST_H_
# define NETDATA_EBPF_PLUGIN_UNITTEST_H_ 1

#include "ebpf.h"

void ebpf_ut_initialize_structure(netdata_run_mode_t mode);
int ebpf_ut_load_real_binary();
#endif
