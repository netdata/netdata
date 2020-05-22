#!/bin/bash
# This script pulls the latest versions of the packages needed by netdata for PFSense and then prints out a list of commands
# to run with the versions that are in the repo.

# This grabs the complete list of py37 packages and drops some that caused problems
PY37PACKAGES=$(curl http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/ 2>/dev/null|grep "py37-" |grep -v cryptography-vectors|awk '{ print $3 }'|awk -F\" '{ print $2; printf "\n" }')
# This grabs the latest version of Netdata
NETDATAPACKAGE=$(curl http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/ 2>/dev/null|grep "netdata"|grep -v go |awk '{ print $3 }'|awk -F\" '{ print $2 }')
# This grabs the latest version of Judy
JUDYPACKAGE=$(curl http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/ 2>/dev/null|grep "Judy"|awk '{ print $3 }'|awk -F\" '{ print $2 }')
# These are the packages that netdata relies on.
PY37PACKAGELIST="py37-certifi py37-asn1crypto py37-pycparser py37-cffi py37-six py37-cryptography py37-idna py37-openssl py37-pysocks py37-urllib3 py37-yaml"

# echoing this command for completeness and didn't change anything
echo pkg install -y pkgconf bash e2fsprogs-libuuid libuv nano
# Updated to the current version of JUDY
echo pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/${JUDYPACKAGE}

# The PY37 list is large and we don't know what might be needed in the future this first loop take the list of required
# packages and uses it to search through the complete list from FREEBSD
for i in ${PY37PACKAGELIST}
do
    # This loops through looking for a match to the PACKAGELIST name in the complete list of packages and when it's found
    # put it in the PKG variable to print out after the loop.
    for j in ${PY37PACKAGES}
    do
        if [[ ${j} == *${i}* ]]
        then
            #echo ${j}
            PKG=${j}
        fi
    done
    echo pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/${PKG}
done
# Now print out the latest NETDATA Package
echo pkg add http://pkg.freebsd.org/FreeBSD:11:amd64/latest/All/${NETDATAPACKAGE}
# The End
