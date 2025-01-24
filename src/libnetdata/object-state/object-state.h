// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OBJECT_STATE_ID_H
#define NETDATA_OBJECT_STATE_ID_H

#include "libnetdata/libnetdata.h"

typedef uint32_t OBJECT_STATE_ID;

typedef struct {
    OBJECT_STATE_ID state_id;
    REFCOUNT state_refcount;
} OBJECT_STATE;

#define OBJECT_STATE_DEACTIVATED (-INT32_MAX)

#define OBJECT_STATE_INIT_ACTIVATED (OBJECT_STATE){ .state_id = 0, .state_refcount = 0, };
#define OBJECT_STATE_INIT_DEACTIVATED (OBJECT_STATE){ .state_id = 0, .state_refcount = OBJECT_STATE_DEACTIVATED, };

// get the current state id of the object
OBJECT_STATE_ID object_state_id(OBJECT_STATE *os);

// increments the object's state id
// enables using the object - users may acquire and release the object
void object_state_activate(OBJECT_STATE *os);
void object_state_activate_if_not_activated(OBJECT_STATE *os);

// increments the object's state id
// prevents users from acquiring it, and waits until all of its holders have released it
void object_state_deactivate(OBJECT_STATE *os);

bool object_state_acquire(OBJECT_STATE *os, OBJECT_STATE_ID wanted_state_id);
void object_state_release(OBJECT_STATE *os);

#endif //NETDATA_OBJECT_STATE_ID_H
