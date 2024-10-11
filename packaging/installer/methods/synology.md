# Install Netdata on Synology

> 💡 This document is maintained by Netdata's community, and may not be completely up-to-date. Please double-check the
> details of the installation process, before proceeding.
>
> You can help improve this document by
> [submitting a PR](https://github.com/netdata/netdata/edit/master/packaging/installer/methods/synology.md)
> with your recommended improvements or changes. Thank you!

The good news is that our
[one-line installation script](/packaging/installer/methods/kickstart.md)
works fine if your NAS is one that uses the amd64 architecture. It
will install the content into `/opt/netdata`, making future removal safe and simple.

## Run as netdata user

When Netdata is first installed, it will run as _root_. This may or may not be acceptable for you, and since other
installations run it as the `netdata` user, you might wish to do the same. This requires some extra work:

1. Create a group `netdata` via the Synology group interface. Give it no access to anything.
2. Create a user `netdata` via the Synology user interface. Give it no access to anything and a random password. Assign
    the user to the `netdata` group. Netdata will chuid to this user when running.
3. Change ownership of the following directories:

    ```sh
    chown -R root:netdata /opt/netdata/usr/share/netdata
    chown -R netdata:netdata /opt/netdata/var/lib/netdata /opt/netdata/var/cache/netdata
    chown -R netdata:root /opt/netdata/var/log/netdata
    ```

4. Restart Netdata

    ```sh
    /etc/rc.netdata restart
    ```

## Create startup script

Additionally, as of 2018/06/24, the Netdata installer doesn't recognize DSM as an operating system, so no init script is
installed. You'll have to do this manually:

1. Add [this file](https://gist.github.com/oskapt/055d474d7bfef32c49469c1b53e8225f) as `/etc/rc.netdata`. Make it
    executable with `chmod 0755 /etc/rc.netdata`.
2. Add or edit `/etc/rc.local` and add a line calling `/etc/rc.netdata` to have it start on boot:

    ```text
    # Netdata startup
    [ -x /etc/rc.netdata ] && /etc/rc.netdata start
    ```

3. Make sure `/etc/rc.local` is executable: `chmod 0755 /etc/rc.local`.
