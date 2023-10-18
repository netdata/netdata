#!/usr/bin/env bash

issue_certificates() {
      # --- EDIT THIS SECTION ---

      # All the servers involved in journals logs management
      # For each server you can add as many DNS: or IP: aliases as required.
      # The "server" command, just creates certificates for these servers.

      # Format:
      # server CanonicalName Alias1 Alias2 Alias3 Alias4 ...

      server "server1" "DNS:server-hostname1" "DNS:server-hostname1.domain" "IP:172.16.1.1" "IP:10.1.1.1"
      server "server2" "DNS:client-hostname2" "DNS:server-hostname2.domain" "IP:172.16.1.2" "IP:10.1.1.2"
      server "server3" "DNS:client-hostname3" "DNS:server-hostname3.domain" "IP:172.16.1.3" "IP:10.1.1.3"
      server "server4" "DNS:client-hostname4" "DNS:server-hostname4.domain" "IP:172.16.1.4" "IP:10.1.1.4"
}

# comment this line to make sure you configured the script
echo "COMMENT OUT THIS LINE TO RUN ME AND CREATE FILES IN THIS DIRECTORY: $(pwd)" && exit 1

# -----------------------------------------------------------------------------
# Create the CA

# stop on all errors
set -e

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
      chgrp systemd-journal-remote ca.pem
      chmod 0640 ca.pem
fi

# -----------------------------------------------------------------------------
# Create a server certificate

generate_server_certificate() {
      local cn="${1}"; shift

      if [ ! -f "${cn}.pem" -o ! -f "${cn}.key" ]; then
            echo "subjectAltName = $(echo "${@}" | tr " " ",")" >"${cn}.conf"

            echo >&2 "Generating server: ${cn}.pem and ${cn}.key ..."

            openssl req -newkey rsa:2048 -nodes -out "${cn}.csr" -keyout "${cn}.key" -subj "/CN=${cn}/"
            openssl ca -batch -config ca.conf -notext -in "${cn}.csr" -out "${cn}.pem" -extfile "${cn}.conf"

            chgrp systemd-journal-remote "${cn}.pem" "${cn}.key"
            chmod 0640 "${cn}.pem" "${cn}.key"
      fi
}


# -----------------------------------------------------------------------------
# Create a script to install the certificate on each server

generate_install_script() {
      local cn="${2}"
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

if ! grep -q "^systemd-journal:" /etc/group; then
      echo >&2 "adding missing system group: systemd-journal"
      groupadd --system systemd-journal
fi

for usr in systemd-journal-remote systemd-journal-upload systemd-journal-gateway
do
      # make sure the user exists

      if ! id -u \${usr} &>/dev/null; then
            echo >&2 "adding missing system user: \${usr}"
            useradd --system --user-group --no-create-home --home-dir /run/systemd --shell /usr/sbin/nologin \${usr} --groups systemd-journal
      fi

      # make sure the user is part of the systemd-journal group

      if ! id -nG \${usr} | grep -q -w systemd-journal; then
            echo >&2 "adding user: \${usr} to group: systemd-journal"
            sudo usermod -aG systemd-journal \${usr}
      fi
done

if [ ! -d ${dst} ]; then
      echo >&2 "creating directory: ${dst}"
      mkdir -p "${dst}"
fi
chgrp systemd-journal "${dst}"
chmod 750 "${dst}"
cd "${dst}"

echo >&2 "saving trusted certificate file as: ${dst}/ca.pem"
cat >ca.pem <<EOFCAPEM
$(cat ca.pem)
EOFCAPEM

chgrp systemd-journal ca.pem
chmod 0640 ca.pem

echo >&2 "saving server ${cn} certificate file as: ${dst}/${cn}.pem"
cat >"${cn}.pem" <<EOFSERPEM
$(cat "${cn}.pem")
EOFSERPEM

chgrp systemd-journal "${cn}.pem"
chmod 0640 "${cn}.pem"

echo >&2 "saving server ${cn} key file as: ${dst}/${cn}.key"
cat >"${cn}.key" <<EOFSERKEY
$(cat "${cn}.key")
EOFSERKEY

chgrp systemd-journal "${cn}.key"
chmod 0640 "${cn}.key"

for cfg in /etc/systemd/journal-remote.conf /etc/systemd-journal-upload.conf
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

echo >&2 "all done"
EOFC1

      chmod 0700 "runme-on-${cn}.sh"
}

# -----------------------------------------------------------------------------
# Create the client certificates

server() {
      generate_server_certificate "${@}"
      generate_install_script "${1}"
}

# -----------------------------------------------------------------------------
# issue the requested certificates

issue_certificates

echo >&2
echo >&2 "ALL DONE"
echo >&2 "Check the runme-on-XXX.sh scripts:"
echo >&2
ls -l runme-on-*.sh
