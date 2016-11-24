Packaging Tools
===============

The programs in this folder are used when packaging from within git
and are not included in source or binary packages.

For the most part they are used from the git commit hooks (copy
`../hooks/*` to `../.git/hooks` to automate checking and the release
process.

The check-files script pulls in `*.functions` and `*/*.functions` to
do the actual work.

`packaging.functions` contains generic checks on e.g `ChangeLog`
and `configure.ac` and automates release version, checking, tagging
and post-release update.

Programs and packages with specific needs should create extra
`whatever.functions` and supporting scripts in a subdirectory.

The `gpg.keys` file is a list of keys that can be expected to sign
tags and packages.

Making a release
----------------
`
Just update ChangeLog and configure.ac to specify a suitable version
suffix:

    empty - final release
    pre.# - pre-release candidate
    rc.# - pre-release candidate

If it is a final release and there is a package.spec.in, add a new
entry to the top of the %changelog section and update:
    PACKAGE_RPM_RELEASE="1"

The hooks will take over and if everything is OK will tag the release
(you will be asked to sign the tag) and then update the files ready
for further development.

The release is not pushed out automatically, so if you want to undo
it, run:

~~~~
git reset --hard HEAD^^
git tag -d vx.y.z
~~~~

Otherwise you can just push the results; the script outputs the required
instructions upon success.

Once pushed the infrastructure will build a set of tar-files on the server.
For information on how to verify, sign and make these available, see:

    https://github.com/firehol/infrastructure/raw/master/doc/release.txt
