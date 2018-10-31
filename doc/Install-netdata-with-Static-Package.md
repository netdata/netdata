# Install netdata from a static package

* x86_64 pre-built binary for any Linux *

You can install a pre-compiled static binary of netdata for any Intel/AMD 64bit Linux system (even those that don't have a package manager, like CoreOS, CirrOS, busybox systems, etc). You can also use these packages on systems with broken or unsupported package managers.

To install the latest version use this:

```sh
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh)
```

For automated installs, append a space + `--dont-wait` to the command line. You can also append `--dont-start-it` to prevent the installer from starting netdata. Example:

```sh
bash <(curl -Ss https://my-netdata.io/kickstart-static64.sh) --dont-wait --dont-start-it
```

If your shell fails to handle the above one liner, do this:

```sh
# download the script with curl
curl https://my-netdata.io/kickstart-static64.sh >/tmp/kickstart-static64.sh

# or, download the script with wget
wget -O /tmp/kickstart-static64.sh https://my-netdata.io/kickstart-static64.sh

# run the downloaded script (any sh is fine, no need for bash)
sh /tmp/kickstart-static64.sh
```

> **The static builds install netdata at `/opt/netdata`**.

The static binary files are kept in this repo: https://github.com/firehol/binary-packages

Download any of the `.run` files, and run it. These files are self-extracting shell scripts built with [makeself](https://github.com/megastep/makeself).

The target system does **not** need to have bash installed.

The same files can be used for updates too.
