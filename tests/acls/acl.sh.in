#!/bin/bash -x
# SPDX-License-Identifier: GPL-3.0-or-later

BASICURL="http://127.0.0.1"
BASICURLS="https://127.0.0.1"

NETDATA_VARLIB_DIR="/var/lib/netdata"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;43m'

#change the previous acl file and with a new 
#and store it on a new file
change_file(){
    sed "s/$1/$2/g" netdata.cfg > "$4"
}

change_ssl_file(){
    KEYROW="ssl key = $3/key.pem"
    CERTROW="ssl certificate = $3/cert.pem"
    sed "s@ssl key =@$KEYROW@g" netdata.ssl.cfg > tmp
    sed "s@ssl certificate =@$CERTROW@g" tmp > tmp2
    sed "s/$1/$2/g" tmp2 > "$4"
}

run_acl_tests() {
    #Give a time for netdata start properly
    sleep 2

    curl -v -k --tls-max 1.2 --create-dirs -o index.html "$2" 2> log_index.txt
    curl -v -k --tls-max 1.2 --create-dirs -o netdata.txt "$2/netdata.conf" 2> log_nc.txt
    curl -v -k --tls-max 1.2 --create-dirs -o badge.csv "$2/api/v1/badge.svg?chart=cpu.cpu0_interrupts" 2> log_badge.txt
    curl -v -k --tls-max 1.2 --create-dirs -o info.txt "$2/api/v1/info" 2> log_info.txt
    curl -H "X-Auth-Token: $1" -v -k --tls-max 1.2 --create-dirs -o health.csv "$2/api/v1/manage/health?cmd=LIST" 2> log_health.txt

    TOT=$(grep -c "HTTP/1.1 301" log_*.txt | cut -d: -f2| grep -c 1)
    if [ "$TOT" -ne "$4" ]; then
        echo -e "${RED}I got a wrong number of redirects($TOT) when SSL is activated, It was expected $4"
        rm log_* netdata.conf.test* netdata.txt health.csv index.html badge.csv tmp* key.pem cert.pem info.txt
        killall netdata
        exit 1
    elif [ "$TOT" -eq "$4" ] && [ "$4" -ne "0" ]; then
        echo -e "${YELLOW}I got the correct number of redirects($4) when SSL is activated and I try to access with HTTP."
        return
    fi

    TOT=$(grep -c "HTTP/1.1 200 OK" log_* | cut -d: -f2| grep -c 1)
    if [ "$TOT" -ne "$3" ]; then
        echo -e "${RED}I got a wrong number of \"200 OK\" from the queries, it was expected $3."
        killall netdata
        rm log_* netdata.conf.test* netdata.txt health.csv index.html badge.csv tmp* key.pem cert.pem info.txt
        exit 1
    fi

    echo -e "${GREEN}ACLs were applied correctly"
}

CONF=$(grep "bind" netdata.cfg)
MUSER=$(grep run netdata.cfg | cut -d= -f2|sed 's/^[ \t]*//')

openssl req -new -newkey rsa:2048 -days 365 -nodes -x509 -sha512 -subj "/C=US/ST=Denied/L=Somewhere/O=Dis/CN=www.example.com" -keyout key.pem  -out cert.pem
chown "$MUSER" key.pem cert.pem
CWD=$(pwd)

if [ -f "${NETDATA_VARLIB_DIR}/netdata.api.key" ] ;then
    read -r TOKEN < "${NETDATA_VARLIB_DIR}/netdata.api.key"
else
    TOKEN="NULL"
fi

change_file "$CONF" "    bind to = *" "$CWD" "netdata.conf.test0"
netdata -c "netdata.conf.test0" 
run_acl_tests $TOKEN "$BASICURL:19999" 5 0
killall netdata

change_ssl_file "$CONF" "    bind to = *=dashboard|registry|badges|management|netdata.conf *:20000=dashboard|registry|badges|management *:20001=dashboard|registry|netdata.conf^SSL=optional *:20002=dashboard|registry" "$CWD" "netdata.conf.test1"
netdata -c "netdata.conf.test1"
run_acl_tests $TOKEN "$BASICURL:19999" 5 5
run_acl_tests $TOKEN "$BASICURLS:19999" 5 0

run_acl_tests $TOKEN "$BASICURL:20000" 4 5
run_acl_tests $TOKEN "$BASICURLS:20000" 4 0

run_acl_tests $TOKEN "$BASICURL:20001" 4 0
run_acl_tests $TOKEN "$BASICURLS:20001" 4 0

run_acl_tests $TOKEN "$BASICURL:20002" 3 5 
run_acl_tests $TOKEN "$BASICURLS:20002" 3 0 
killall netdata

change_ssl_file "$CONF" "    bind to = *=dashboard|registry|badges|management|netdata.conf *:20000=dashboard|registry|badges|management *:20001=dashboard|registry|netdata.conf^SSL=force *:20002=dashboard|registry" "$CWD" "netdata.conf.test2"
netdata -c "netdata.conf.test2"
run_acl_tests $TOKEN "$BASICURL:19999" 5 5
run_acl_tests $TOKEN "$BASICURLS:19999" 5 0

run_acl_tests $TOKEN "$BASICURL:20000" 4 5
run_acl_tests $TOKEN "$BASICURLS:20000" 4 0

run_acl_tests $TOKEN "$BASICURL:20001" 4 5
run_acl_tests $TOKEN "$BASICURLS:20001" 4 0

run_acl_tests $TOKEN "$BASICURL:20002" 3 5 
run_acl_tests $TOKEN "$BASICURLS:20002" 3 0 
killall netdata

change_ssl_file "$CONF" "    bind to = *=dashboard|registry|badges|management|netdata.conf *:20000=dashboard|registry|badges|management^SSL=optional *:20001=dashboard|registry|netdata.conf^SSL=force" "$CWD" "netdata.conf.test3"
netdata -c "netdata.conf.test3"
run_acl_tests $TOKEN "$BASICURL:19999" 5 5
run_acl_tests $TOKEN "$BASICURLS:19999" 5 0

run_acl_tests $TOKEN "$BASICURL:20000" 4 0
run_acl_tests $TOKEN "$BASICURLS:20000" 4 0

run_acl_tests $TOKEN "$BASICURL:20001" 4 5
run_acl_tests $TOKEN "$BASICURLS:20001" 4 0
killall netdata

rm log_* netdata.conf.test* netdata.txt health.csv index.html badge.csv tmp* key.pem cert.pem info.txt
echo "All the tests were successful"
