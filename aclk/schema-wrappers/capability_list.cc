// SPDX-License-Identifier: GPL-3.0-or-later

#include "capability_list.h"

#include "proto/aclk/v1/lib.pb.h"

//#include "libnetdata/libnetdata.h"

//#include "schema_wrapper_utils.h"

using namespace aclk_lib::v1;

capabilities_list *new_capabilities_list(){
    return new std::vector<struct capability>;
}
