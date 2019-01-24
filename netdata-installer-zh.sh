#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# shellcheck disable=SC1090,SC1091,SC1117,SC2002,SC2034,SC2044,SC2046,SC2086,SC2129,SC2162,SC2166,SC2181

netdata_source_dir="$(pwd)"

rm ${netdata_source_dir}/web/gui/dashboard_info.js
cp ${netdata_source_dir}/web/gui/dashboard_info_zh.js ${netdata_source_dir}/web/gui/dashboard_info.js
./netdata-installer.sh
