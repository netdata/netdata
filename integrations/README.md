To generate a copy of `integrations.js` locally, you will need:

- Python 3.6 or newer (only tested on Python 3.10 currently, should work
  on any version of Python newer than 3.6).
- The following third-party Python modules:
    - `jsonschema`
    - `referencing`
    - `jinja2`
    - `ruamel.yaml`
- A local checkout of https://github.com/netdata/netdata
- A local checkout of https://github.com/netdata/go.d.plugin. The script
  expects this to be checked out in a directory called `go.d.plugin`
  in the root directory of the Agent repo, though a symlink with that
  name pointing at the actual location of the repo will work as well.

The first two parts can be easily covered in a Linux environment, such
as a VM or Docker container:

- On Debian or Ubuntu: `apt-get install python3-jsonschema python3-referencing python3-jinja2 python3-ruamel.yaml`
- On Alpine: `apk add py3-jsonschema py3-referencing py3-jinja2 py3-ruamel.yaml`
- On Fedora or RHEL (EPEL is required on RHEL systems): `dnf install python3-jsonschema python3-referencing python3-jinja2 python3-ruamel-yaml`

Once the environment is set up, simply run
`integrations/gen_integrations.py` from the Agent repo. Note that the
script must be run _from this specific location_, as it uses it’s own
path to figure out where all the files it needs are.
