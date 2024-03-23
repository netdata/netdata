#ifndef LIBNETDATA_BT
#define LIBNETDATA_BT

#include "../libnetdata.h"

#ifdef __cplusplus
extern "C" {
#endif

void bt_init(const char *exepath, const char *cache_dir);
void bt_collect(const uuid_t *uuid);
void bt_dump(const uuid_t *uuid);

extern const char *bt_path;

#ifdef __cplusplus
}
#endif

#endif /* LIBNETDATA_BT */
