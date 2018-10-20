# netdata ini config files

Configuration files `netdata.conf` and `stream.conf` are netdata ini files.

## Motivation

The whole idea came up when we were evaluating the documentation involved
in maintaining a complex configuration system. Our intention was to give
configuration options for everything imaginable. But then, documenting all
these options would require a tremendous amount of time, users would have
to search through endless pages for the option they need, etc.

We concluded then that **configuring software like that is a waste of time
and effort**. Of course there must be plenty of configuration options, but
the implementation itself should require a lot less effort for both the
developers and the users.

So, we did this:

1. No configuration is required to run netdata
2. There are plenty of options to tweak
3. There is minimal documentation (or no at all)

## Why this works?

The configuration file is a `name = value` dictionary with `[sections]`.
Write whatever you like there as long as it follows this simple format.

Netdata loads this dictionary and then when the code needs a value from
it, it just looks up the `name` in the dictionary at the proper `section`.
In all places, in the code, there are both the `names` and their
`default values`, so if something is not found in the configuration
file, the default is used. The lookup is made using B-Trees and hashes
(no string comparisons), so they are super fast. Also the `names` of the
settings can be `my super duper setting that once set to yes, will turn the world upside down = no`
- so goodbye to most of the documentation involved.

Next, netdata can generate a valid configuration for the user to edit.
No need to remember anything or copy and paste settings. Just get the
configuration from the server (`/netdata.conf` on your netdata server),
edit it and save it.

Last, what about options you believe you have set, but you misspelled?
When you get the configuration file from the server, there will be a
comment above all `name = value` pairs the server does not use.
So you know that whatever you wrote there, is not used.
