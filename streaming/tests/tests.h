// SPDX-License-Identifier: GPL-3.0-or-later
#include <gtest/gtest.h>

extern "C" {
#include "../../libnetdata/libnetdata.h"
#include "../rrdpush.h"
#include "../replication.h"
}

#define QUEUE_SIZE 5
#define QUEUE_MEMBER_GAP 100

typedef struct s{
	int x;
	int y;
} st;
