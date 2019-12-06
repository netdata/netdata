#!/usr/bin/env bash

TOKEN="unknown"
URL_BASE="unknown"
ID="unknown"
ROOMS=""
HOSTNAME=$(hostname)

for arg in "$@"
do
    case $arg in
        -token=*) TOKEN=${arg:7} ;;
        -url=*) URL_BASE=${arg:5} ;;
        -id=*) ID=${arg:4} ;;
        -rooms=*) ROOMS=${arg:7} ;;
        -hostname=*) HOSTNAME=${arg:10} ;;
        *)  echo "Unknown arg ${arg}"
            exit -1 ;;
    esac
    shift 1
done

echo "Token: $TOKEN"
echo "Url: $URL_BASE"
echo "Id: $ID"
echo "ROOMS: $ROOMS"
echo "HOSTNAME: $HOSTNAME"

openssl genrsa -out private.pem 2048
openssl rsa -in private.pem -outform PEM -pubout -out public.pem

#TARGET_URL="https://$URL_BASE"
KEY=$(cat public.pem | tr '\n' '!' | sed -e 's/!/\\n/g')
[ ! -z "$ROOMS" ] && ROOMS=\"$(echo $ROOMS | sed s'/,/", "/g')\"
cat >tmp.txt <<EMBED_JSON
{
    "agent": {
        "id": "$ID",
        "hostname": "$HOSTNAME"
    },
    "token": "$TOKEN",
    "rooms" : [ $ROOMS ],
    "public_key" : "$KEY"
}
EMBED_JSON

#curl --trace-ascii - -X PUT -d "@tmp.txt" @public.pem $TARGET_URL
curl --trace-ascii - -X PUT -d "@tmp.txt" @public.pem $URL_BASE
