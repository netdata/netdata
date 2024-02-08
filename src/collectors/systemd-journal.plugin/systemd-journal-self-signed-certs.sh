#!/usr/bin/env bash

me="${0}"
dst="/etc/ssl/systemd-journal"

show_usage() {
      cat <<EOFUSAGE

${me} [options] server_name alias1 alias2 ...

server_name
      the canonical name of the server on the certificates

aliasN
      a hostname or IP this server is reachable with
      DNS names should be like DNS:hostname
      IPs should be like IP:1.2.3.4
      Any number of aliases are accepted per server

options can be:

  -h, --help
      show this message

  -d, --directory DIRECTORY
      change the default certificates install dir
      default: ${dst}

EOFUSAGE
}

while [ ! -z "${1}" ]; do
      case "${1}" in
            -h|--help)
                  show_usage
                  exit 0
                  ;;

            -d|--directory)
                  dst="${2}"
                  echo >&2 "directory set to: ${dst}"
                  shift
                  ;;

            *)
                  break 2
                  ;;
      esac

      shift
done

if [ -z "${1}" ]; then
      show_usage
      exit 1
fi


# Define a regular expression pattern for a valid canonical name
valid_canonical_name_pattern="^[a-zA-Z0-9][a-zA-Z0-9.-]+$"

# Check if ${1} matches the pattern
if [[ ! "${1}" =~ ${valid_canonical_name_pattern} ]]; then
    echo "Certificate name '${1}' is not valid."
    exit 1
fi

# -----------------------------------------------------------------------------
# Create the CA

# stop on all errors
set -e

if [ $UID -ne 0 ]
then
      echo >&2 "Hey! sudo me: sudo ${me}"
      exit 1
fi

if ! getent group systemd-journal >/dev/null 2>&1; then
      echo >&2 "Missing system group: systemd-journal. Did you install systemd-journald?"
      exit 1
fi

if ! getent passwd systemd-journal-remote >/dev/null 2>&1; then
      echo >&2 "Missing system user: systemd-journal-remote. Did you install systemd-journal-remote?"
      exit 1
fi

if [ ! -d "${dst}" ]
then
      mkdir -p "${dst}"
      chown systemd-journal-remote:systemd-journal "${dst}"
      chmod 750 "${dst}"
fi

cd "${dst}"

test ! -f ca.conf && cat >ca.conf <<EOF
[ ca ]
default_ca = CA_default
[ CA_default ]
new_certs_dir = .
certificate = ca.pem
database = ./index
private_key = ca.key
serial = ./serial
default_days = 3650
default_md = default
policy = policy_anything
[ policy_anything ]
countryName             = optional
stateOrProvinceName     = optional
localityName            = optional
organizationName        = optional
organizationalUnitName  = optional
commonName              = supplied
emailAddress            = optional
EOF

test ! -f index && touch index
test ! -f serial && echo 0001 >serial

if [ ! -f ca.pem -o ! -f ca.key ]; then
      echo >&2 "Generating ca.pem ..."

      openssl req -newkey rsa:2048 -days 3650 -x509 -nodes -out ca.pem -keyout ca.key -subj "/CN=systemd-journal-remote-ca/"
      chown systemd-journal-remote:systemd-journal ca.pem
      chmod 0640 ca.pem
fi

# -----------------------------------------------------------------------------
# Create a server certificate

generate_server_certificate() {
      local cn="${1}"; shift

      if [ ! -f "${cn}.pem" -o ! -f "${cn}.key" ]; then
            if [ -z "${*}" ]; then
                  echo >"${cn}.conf"
            else
                  echo "subjectAltName = $(echo "${@}" | tr " " ",")" >"${cn}.conf"
            fi

            echo >&2 "Generating server: ${cn}.pem and ${cn}.key ..."

            openssl req -newkey rsa:2048 -nodes -out "${cn}.csr" -keyout "${cn}.key" -subj "/CN=${cn}/"
            openssl ca -batch -config ca.conf -notext -in "${cn}.csr" -out "${cn}.pem" -extfile "${cn}.conf"
      else
            echo >&2 "certificates for ${cn} are already available."
      fi

      chown systemd-journal-remote:systemd-journal "${cn}.pem" "${cn}.key"
      chmod 0640 "${cn}.pem" "${cn}.key"
}


# -----------------------------------------------------------------------------
# Create a script to install the certificate on each server

generate_install_script() {
      local cn="${1}"
      local dst="/etc/ssl/systemd-journal"

      cat >"runme-on-${cn}.sh" <<EOFC1
#!/usr/bin/env bash

# stop on all errors
set -e

if [ \$UID -ne 0 ]; then
      echo >&2 "Hey! sudo me: sudo \${0}"
      exit 1
fi

# make sure the systemd-journal group exists
# all certificates will be owned by this group
if ! getent group systemd-journal >/dev/null 2>&1; then
      echo >&2 "Missing system group: systemd-journal. Did you install systemd-journald?"
      exit 1
fi

if ! getent passwd systemd-journal-remote >/dev/null 2>&1; then
      echo >&2 "Missing system user: systemd-journal-remote. Did you install systemd-journal-remote?"
      exit 1
fi

if [ ! -d ${dst} ]; then
      echo >&2 "creating directory: ${dst}"
      mkdir -p "${dst}"
fi
chown systemd-journal-remote:systemd-journal "${dst}"
chmod 750 "${dst}"
cd "${dst}"

echo >&2 "saving trusted certificate file as: ${dst}/ca.pem"
cat >ca.pem <<EOFCAPEM
$(cat ca.pem)
EOFCAPEM

chown systemd-journal-remote:systemd-journal ca.pem
chmod 0640 ca.pem

echo >&2 "saving server ${cn} certificate file as: ${dst}/${cn}.pem"
cat >"${cn}.pem" <<EOFSERPEM
$(cat "${cn}.pem")
EOFSERPEM

chown systemd-journal-remote:systemd-journal "${cn}.pem"
chmod 0640 "${cn}.pem"

echo >&2 "saving server ${cn} key file as: ${dst}/${cn}.key"
cat >"${cn}.key" <<EOFSERKEY
$(cat "${cn}.key")
EOFSERKEY

chown systemd-journal-remote:systemd-journal "${cn}.key"
chmod 0640 "${cn}.key"

for cfg in /etc/systemd/journal-remote.conf /etc/systemd/journal-upload.conf
do
      if [ -f \${cfg} ]; then
            # keep a backup of the file
            test ! -f \${cfg}.orig && cp \${cfg} \${cfg}.orig

            # fix its contents
            echo >&2 "updating the certificates in \${cfg}"
            sed -i "s|^#\\?\\s*ServerKeyFile=.*$|ServerKeyFile=${dst}/${cn}.key|" \${cfg}
            sed -i "s|^#\\?\\s*ServerCertificateFile=.*$|ServerCertificateFile=${dst}/${cn}.pem|" \${cfg}
            sed -i "s|^#\\?\\s*TrustedCertificateFile=.*$|TrustedCertificateFile=${dst}/ca.pem|" \${cfg}
      fi
done

echo >&2 "certificates installed - you may need to restart services to active them"
echo >&2
echo >&2 "If this is a central server:"
echo >&2 "# systemctl restart systemd-journal-remote.socket"
echo >&2
echo >&2 "If this is a passive client:"
echo >&2 "# systemctl restart systemd-journal-upload.service"
echo >&2
echo >&2 "If this is an active client:"
echo >&2 "# systemctl restart systemd-journal-gateway.socket"
EOFC1

      chmod 0700 "runme-on-${cn}.sh"
}

# -----------------------------------------------------------------------------
# Create the client certificates

generate_server_certificate "${@}"
generate_install_script "${1}"


# Set ANSI escape code for colors
yellow_color="\033[1;33m"
green_color="\033[0;32m"
# Reset ANSI color after the message
reset_color="\033[0m"


echo >&2 -e "use this script to install it on ${1}: ${yellow_color}$(ls ${dst}/runme-on-${1}.sh)${reset_color}"
echo >&2 "copy it to your server ${1}, like this:"
echo >&2 -e "# ${green_color}scp ${dst}/runme-on-${1}.sh ${1}:/tmp/${reset_color}"
echo >&2 "and then run it on that server to install the certificates"
echo >&2
