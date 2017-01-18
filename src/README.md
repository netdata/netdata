Netdata Server Code {#servercode}
==============================================

`src/` contains the sources for building `netdata` and `apps.plugin`

# Overview

- `appconfig.h`: The global API for the configuration.
- `plugin_*`: Internal data collection plugins.
- `proc_*`: Data collection of files in the proc file system.
- `regestry*`: The netdata registry.
- `rrd.h`: Public API of the round robin database.
- `unit_test.h`: Public API to call tests.
- `web_*`: The web server.

# Coding Conventions
 
## Documenation
Header files must be documented with [doxygen](http://www.stack.nl/~dimitri/doxygen/manual/docblocks.html).

## Searching for strings
To search for `string` in a collection always do this for performance:
- Store a hash of the string in the object: `uint32_t hash = simple_hash(string);`
- All `strcmp()` should be `if( hash == object->hash && !strcmp(string, object->string) )`.