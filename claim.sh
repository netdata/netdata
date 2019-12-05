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

TARGET_URL="https://$URL_BASE"
KEY=$(cat public.pem | tr '\n' '!' | sed -e 's/!/\\n/g')
cat >tmp.txt <<EMBED_JSON
{
    "agent": {
        "id": "$ID",
        "hostname": "$HOSTNAME"
    },
    "token": "0543be24-c3b7-408d-9514-997e96c74a28",
    "rooms" : [ "a man", "a plan", "a canal" ],
    "key" : "$KEY"
}
EMBED_JSON

curl --trace-ascii - -X PUT --data-binary tmp.txt @public.pem $TARGET_URL
