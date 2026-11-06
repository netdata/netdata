#!/bin/bash

set -e

fakeroot_dir="${1}"
shift 1

lockfile="${fakeroot_dir}/lock"
envfile="${fakeroot_dir}/env"

(
    flock -x 3;
    touch "${envfile}"
    fakeroot -i "${envfile}" -s "${envfile}" -u -b 4 -- "${@}" ;
) 3>"${lockfile}"
