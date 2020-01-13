This is the agent cloud link (ACLK) information file.

The installer does not yet have steps to pull and build the local library.
cd project_root
git clone ssh://git@github.com/netdata/mosquitto mosquitto/
(cd mosquitto/lib && make)      # Ignore the cpp error

This will leave mosquitto/lib/libmosquitto.a for the build process to use.
