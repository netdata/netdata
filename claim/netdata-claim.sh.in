#!/usr/bin/env bash
# netdata
# real-time performance and health monitoring, done right!
# (C) 2017 Costa Tsaousis <costa@tsaousis.gr>
# SPDX-License-Identifier: GPL-3.0-or-later

# Exit code: 0 - Success
# Exit code: 1 - Unknown argument
# Exit code: 2 - Problems with claiming working directory
# Exit code: 3 - Missing dependencies
# Exit code: 4 - Failure to connect to endpoint
# Exit code: 5 - The CLI didn't work
# Exit code: 6 - Wrong user
# Exit code: 7 - Unknown HTTP error message
#
# OK: Agent claimed successfully
# HTTP Status code: 204
# Exit code: 0
#
# Unknown HTTP error message
# HTTP Status code: 422
# Exit code: 7
ERROR_KEYS[7]="None"
ERROR_MESSAGES[7]="Unknown HTTP error message"

# Error: The agent id is invalid; it does not fulfill the constraints
# HTTP Status code: 422
# Exit code: 8
ERROR_KEYS[8]="ErrInvalidNodeID"
ERROR_MESSAGES[8]="invalid node id"

# Error: The agent hostname is invalid; it does not fulfill the constraints
# HTTP Status code: 422
# Exit code: 9
ERROR_KEYS[9]="ErrInvalidNodeName"
ERROR_MESSAGES[9]="invalid node name"

# Error: At least one of the given rooms ids is invalid; it does not fulfill the constraints
# HTTP Status code: 422
# Exit code: 10
ERROR_KEYS[10]="ErrInvalidRoomID"
ERROR_MESSAGES[10]="invalid room id"

# Error: Invalid public key; the public key is empty or not present
# HTTP Status code: 422
# Exit code: 11
ERROR_KEYS[11]="ErrInvalidPublicKey"
ERROR_MESSAGES[11]="invalid public key"
#
# Error: Expired, missing or invalid token
# HTTP Status code: 403
# Exit code: 12
ERROR_KEYS[12]="ErrForbidden"
ERROR_MESSAGES[12]="token expired/token not found/invalid token"

# Error: Duplicate agent id; an agent with the same id is already registered in the cloud
# HTTP Status code: 409
# Exit code: 13
ERROR_KEYS[13]="ErrAlreadyClaimed"
ERROR_MESSAGES[13]="already claimed"

# Error: The node claiming process is still in progress.
# HTTP Status code: 102
# Exit code: 14
ERROR_KEYS[14]="ErrProcessingClaim"
ERROR_MESSAGES[14]="processing claiming"

# Error: Internal server error. Any other unexpected error (DB problems, etc.)
# HTTP Status code: 500
# Exit code: 15
ERROR_KEYS[15]="ErrInternalServerError"
ERROR_MESSAGES[15]="Internal Server Error"

# Error: There was a timout processing the claim.
# HTTP Status code: 504
# Exit code: 16
ERROR_KEYS[16]="ErrGatewayTimeout"
ERROR_MESSAGES[16]="Gateway Timeout"

# Error: The service cannot handle the claiming request at this time.
# HTTP Status code: 503
# Exit code: 17
ERROR_KEYS[17]="ErrServiceUnavailable"
ERROR_MESSAGES[17]="Service Unavailable"

# Exit code: 18 - Agent unique id is not generated yet.

get_config_value() {
    conf_file="${1}"
    section="${2}"
    key_name="${3}"
    config_result=$(@sbindir_POST@/netdatacli 2>/dev/null read-config "$conf_file|$section|$key_name"; exit $?)
    # shellcheck disable=SC2181
    if [ "$?" != "0" ]; then
       echo >&2 "cli failed, assume netdata is not running and query the on-disk config"
       config_result=$(@sbindir_POST@/netdata 2>/dev/null -W get2 "$conf_file" "$section" "$key_name" unknown_default)
    fi
    echo "$config_result"
}
if command -v curl >/dev/null 2>&1 ; then
        URLTOOL="curl"
elif command -v wget >/dev/null 2>&1 ; then
        URLTOOL="wget"
else
        echo >&2 "I need curl or wget to proceed, but neither is available on this system."
        exit 3
fi
if ! command -v openssl >/dev/null 2>&1 ; then
        echo >&2 "I need openssl to proceed, but it is not available on this system."
        exit 3
fi

# shellcheck disable=SC2050
if [ "@enable_cloud_POST@" = "no" ]; then
    echo >&2 "This agent was built with --disable-cloud and cannot be claimed"
    exit 3
fi
# shellcheck disable=SC2050
if [ "@can_enable_aclk_POST@" != "yes" ]; then
    echo >&2 "This agent was built without the dependencies for Cloud and cannot be claimed"
    exit 3
fi

# -----------------------------------------------------------------------------
# defaults to allow running this script by hand

[ -z "${NETDATA_VARLIB_DIR}" ] && NETDATA_VARLIB_DIR="@varlibdir_POST@"
MACHINE_GUID_FILE="@registrydir_POST@/netdata.public.unique.id"
CLAIMING_DIR="${NETDATA_VARLIB_DIR}/cloud.d"
TOKEN="unknown"
URL_BASE=$(get_config_value cloud global "cloud base url")
[ -z "$URL_BASE" ] && URL_BASE="https://app.netdata.cloud"  # Cover post-install with --dont-start
ID="unknown"
ROOMS=""
[ -z "$HOSTNAME" ] && HOSTNAME=$(hostname)
CLOUD_CERTIFICATE_FILE="${CLAIMING_DIR}/cloud_fullchain.pem"
VERBOSE=0
INSECURE=0
RELOAD=1
NETDATA_USER=$(get_config_value netdata global "run as user")
[ -z "$EUID" ] && EUID="$(id -u)"


# get the MACHINE_GUID by default
if [ -r "${MACHINE_GUID_FILE}" ]; then
        ID="$(cat "${MACHINE_GUID_FILE}")"
        MGUID=$ID
else
        echo >&2 "netdata.public.unique.id is not generated yet or not readable. Please run agent at least once before attempting to claim. Agent generates this file on first startup. If the ID is generated already make sure you have rights to read it (Filename: ${MACHINE_GUID_FILE})."
        exit 18
fi

# get token from file
if [ -r "${CLAIMING_DIR}/token" ]; then
        TOKEN="$(cat "${CLAIMING_DIR}/token")"
fi

# get rooms from file
if [ -r "${CLAIMING_DIR}/rooms" ]; then
        ROOMS="$(cat "${CLAIMING_DIR}/rooms")"
fi

for arg in "$@"
do
        case $arg in
                -token=*) TOKEN=${arg:7} ;;
                -url=*) URL_BASE=${arg:5} ;;
                -id=*) ID=${arg:4} ;;
                -rooms=*) ROOMS=${arg:7} ;;
                -hostname=*) HOSTNAME=${arg:10} ;;
                -verbose) VERBOSE=1 ;;
                -insecure) INSECURE=1 ;;
                -proxy=*) PROXY=${arg:7} ;;
                -noproxy) NOPROXY=yes ;;
                -noreload) RELOAD=0 ;;
                -user=*) NETDATA_USER=${arg:6} ;;
                *)  echo >&2 "Unknown argument ${arg}"
                    exit 1 ;;
        esac
        shift 1
done

if [ "$EUID" != "0" ] && [ "$(whoami)" != "$NETDATA_USER" ]; then
    echo >&2 "This script must be run by the $NETDATA_USER user account"
    exit 6
fi

# if curl not installed give warning SOCKS can't be used
if [[ "${URLTOOL}" != "curl" && "${PROXY:0:5}" = socks ]] ; then
        echo >&2 "wget doesn't support SOCKS. Please install curl or disable SOCKS proxy."
        exit 1
fi

echo >&2 "Token: ****************"
echo >&2 "Base URL: $URL_BASE"
echo >&2 "Id: $ID"
echo >&2 "Rooms: $ROOMS"
echo >&2 "Hostname: $HOSTNAME"
echo >&2 "Proxy: $PROXY"
echo >&2 "Netdata user: $NETDATA_USER"

# create the claiming directory for this user
if [ ! -d "${CLAIMING_DIR}" ] ; then
        mkdir -p "${CLAIMING_DIR}" && chmod 0770 "${CLAIMING_DIR}"
# shellcheck disable=SC2181
        if [ $? -ne 0 ] ; then
                echo >&2 "Failed to create claiming working directory ${CLAIMING_DIR}"
                exit 2
        fi
fi
if [ ! -w "${CLAIMING_DIR}" ] ; then
        echo >&2 "No write permission in claiming working directory ${CLAIMING_DIR}"
        exit 2
fi

if [ ! -f "${CLAIMING_DIR}/private.pem" ] ; then
        echo >&2 "Generating private/public key for the first time."
        if ! openssl genrsa -out "${CLAIMING_DIR}/private.pem" 2048 ; then
                echo >&2 "Failed to generate private/public key pair."
                exit 2
        fi
fi
if [ ! -f "${CLAIMING_DIR}/public.pem" ] ; then
        echo >&2 "Extracting public key from private key."
        if ! openssl rsa -in "${CLAIMING_DIR}/private.pem" -outform PEM -pubout -out "${CLAIMING_DIR}/public.pem" ; then
                echo >&2 "Failed to extract public key."
                exit 2
        fi
fi

TARGET_URL="${URL_BASE%/}/api/v1/spaces/nodes/${ID}"
# shellcheck disable=SC2002
KEY=$(cat "${CLAIMING_DIR}/public.pem" | tr '\n' '!' | sed -e 's/!/\\n/g')
# shellcheck disable=SC2001
[ -n "$ROOMS" ] && ROOMS=\"$(echo "$ROOMS" | sed s'/,/", "/g')\"

cat > "${CLAIMING_DIR}/tmpin.txt" <<EMBED_JSON
{
    "node": {
        "id": "$ID",
        "hostname": "$HOSTNAME"
    },
    "token": "$TOKEN",
    "rooms" : [ $ROOMS ],
    "publicKey" : "$KEY",
    "mGUID" : "$MGUID"
}
EMBED_JSON

if [ "${VERBOSE}" == 1 ] ; then
    echo "Request to server:"
    cat "${CLAIMING_DIR}/tmpin.txt"
fi


if [ "${URLTOOL}" = "curl" ] ; then
        URLCOMMAND="curl --connect-timeout 5 --retry 0 -s -i -X PUT -d \"@${CLAIMING_DIR}/tmpin.txt\""
        if [ "${NOPROXY}" = "yes" ] ; then
                URLCOMMAND="${URLCOMMAND} -x \"\""
        elif [ -n "${PROXY}" ] ; then
                URLCOMMAND="${URLCOMMAND} -x \"${PROXY}\""
        fi
else
        URLCOMMAND="wget -T 15 -O -  -q --save-headers --content-on-error=on --method=PUT \
        --body-file=\"${CLAIMING_DIR}/tmpin.txt\""
        if [ "${NOPROXY}" = "yes" ] ; then
                URLCOMMAND="${URLCOMMAND} --no-proxy"
        elif [ "${PROXY:0:4}" = http ] ; then
                URLCOMMAND="export http_proxy=${PROXY}; ${URLCOMMAND}"
        fi
fi

if [ "${INSECURE}" == 1 ] ; then
    if [ "${URLTOOL}" = "curl" ] ; then
        URLCOMMAND="${URLCOMMAND} --insecure"
    else
        URLCOMMAND="${URLCOMMAND} --no-check-certificate"
    fi
fi

if [ -r "${CLOUD_CERTIFICATE_FILE}" ] ; then
        if [ "${URLTOOL}" = "curl" ] ; then
                URLCOMMAND="${URLCOMMAND} --cacert \"${CLOUD_CERTIFICATE_FILE}\""
        else
                URLCOMMAND="${URLCOMMAND} --ca-certificate \"${CLOUD_CERTIFICATE_FILE}\""
        fi
fi

if [ "${VERBOSE}" == 1 ]; then
    echo "${URLCOMMAND} \"${TARGET_URL}\""
fi
eval "${URLCOMMAND} \"${TARGET_URL}\"" >"${CLAIMING_DIR}/tmpout.txt"
URLCOMMAND_EXIT_CODE=$?
if [ "${URLTOOL}" = "wget" ] && [ "${URLCOMMAND_EXIT_CODE}" -eq 8 ] ; then
# We consider the server issuing an error response a successful attempt at communicating
        URLCOMMAND_EXIT_CODE=0
fi

rm -f "${CLAIMING_DIR}/tmpin.txt"

# Check if URLCOMMAND connected and received reply
if [ "${URLCOMMAND_EXIT_CODE}" -ne 0 ] ; then
        echo >&2 "Failed to connect to ${URL_BASE}, return code ${URLCOMMAND_EXIT_CODE}"
        rm -f "${CLAIMING_DIR}/tmpout.txt"
        exit 4
fi

if [ "${VERBOSE}" == 1 ] ; then
    echo "Response from server:"
    cat "${CLAIMING_DIR}/tmpout.txt"
fi

ERROR_KEY=$(grep "\"errorMsgKey\":" "${CLAIMING_DIR}/tmpout.txt" | awk -F "errorMsgKey\":\"" '{print $2}' | awk -F "\"" '{print $1}')
case ${ERROR_KEY} in
        "ErrInvalidNodeID") EXIT_CODE=8 ;;
        "ErrInvalidNodeName") EXIT_CODE=9 ;;
        "ErrInvalidRoomID") EXIT_CODE=10 ;;
        "ErrInvalidPublicKey") EXIT_CODE=11 ;;
        "ErrForbidden") EXIT_CODE=12 ;;
        "ErrAlreadyClaimed") EXIT_CODE=13 ;;
        "ErrProcessingClaim") EXIT_CODE=14 ;;
        "ErrInternalServerError") EXIT_CODE=15 ;;
        "ErrGatewayTimeout") EXIT_CODE=16 ;;
        "ErrServiceUnavailable") EXIT_CODE=17 ;;
        *) EXIT_CODE=7 ;;
esac

HTTP_STATUS_CODE=$(grep "HTTP" "${CLAIMING_DIR}/tmpout.txt" | awk -F " " '{print $2}')
if [ "${HTTP_STATUS_CODE}" = "204" ] ; then
        EXIT_CODE=0
fi

if [ "${HTTP_STATUS_CODE}" = "204" ] || [ "${ERROR_KEY}" = "ErrAlreadyClaimed" ] ; then
        rm -f "${CLAIMING_DIR}/tmpout.txt"
        echo -n "${ID}" >"${CLAIMING_DIR}/claimed_id" || (echo >&2 "Claiming failed"; set -e; exit 2)
        rm -f "${CLAIMING_DIR}/token" || (echo >&2 "Claiming failed"; set -e; exit 2)

        # Rewrite the cloud.conf on the disk
        cat > "$CLAIMING_DIR/cloud.conf" <<HERE_DOC
[global]
  enabled = yes
  cloud base url = $URL_BASE
HERE_DOC
        if [ "$EUID" == "0" ]; then
            chown -R "${NETDATA_USER}:${NETDATA_USER}" ${CLAIMING_DIR} || (echo >&2 "Claiming failed"; set -e; exit 2)
        fi
        if [ "${RELOAD}" == "0" ] ; then
            exit $EXIT_CODE
        fi

        if [ -z "${PROXY}" ]; then
           PROXYMSG=""
        else
           PROXYMSG="You have attempted to claim this node through a proxy - please update your the proxy setting in your netdata.conf to ${PROXY}. "
        fi
        # Update cloud.conf in the agent memory
        @sbindir_POST@/netdatacli write-config 'cloud|global|enabled|yes' && \
        @sbindir_POST@/netdatacli write-config "cloud|global|cloud base url|$URL_BASE" && \
        @sbindir_POST@/netdatacli reload-claiming-state && \
        if [ "${HTTP_STATUS_CODE}" = "204" ] ; then
                echo >&2 "${PROXYMSG}Node was successfully claimed."
        else
                echo >&2 "The agent cloud base url is set to the url provided."
                echo >&2 "The cloud may have different credentials already registered for this agent ID and it cannot be reclaimed under different credentials for security reasons. If you are unable to connect use -id=\$(uuidgen) to overwrite this agent ID with a fresh value if the original credentials cannot be restored."
                echo >&2 "${PROXYMSG}Failed to claim node with the following error message:\"${ERROR_MESSAGES[$EXIT_CODE]}\""
        fi && exit $EXIT_CODE

        if [ "${ERROR_KEY}" = "ErrAlreadyClaimed" ] ; then
                echo >&2 "The cloud may have different credentials already registered for this agent ID and it cannot be reclaimed under different credentials for security reasons. If you are unable to connect use -id=\$(uuidgen) to overwrite this agent ID with a fresh value if the original credentials cannot be restored."
                echo >&2 "${PROXYMSG}Failed to claim node with the following error message:\"${ERROR_MESSAGES[$EXIT_CODE]}\""
                exit $EXIT_CODE
        fi
        echo >&2 "${PROXYMSG}The claim was successful but the agent could not be notified ($?)- it requires a restart to connect to the cloud."
        exit 5
fi

echo >&2 "Failed to claim node with the following error message:\"${ERROR_MESSAGES[$EXIT_CODE]}\""
if [ "${VERBOSE}" == 1 ]; then
    echo >&2 "Error key was:\"${ERROR_KEYS[$EXIT_CODE]}\""
fi
rm -f "${CLAIMING_DIR}/tmpout.txt"
exit $EXIT_CODE
