# RRD engine tooling documentation

This documentation is targeted for netdata developers, not users.

## .ksv files in /netdata/database/engine

You can use them to be able to view netdata files in hex editor which also shows structure and interprets data.

1. build netdata, this will generate `journalfile_v2.ksy` and `journalfile_v2_virtmemb.ksy` from `journalfile_v2.ksy.in`
1. install Kaitai struct visualizer as per instructions [here](https://github.com/kaitai-io/kaitai_struct_visualizer) or [there](http://kaitai.io/)
1. add it to PATH and run `ksv $NETDATA_INSTALL_DIR/netdata/var/cache/netdata/dbengine-tier2/journalfile-1-0000000002.njfv2 netdata_journalfile.ksy`

## tools in /netdata/database/engine/tooling

Main purpose of this tools is to serve as reference on how the `.ksv` files can be used programatically from Ruby and Python. This is usefull during debugging and or analysis.

### building

1. ensure you have `kaitai-struct-compiler` installed and in PATH
1. install following ruby gems `kaitai-struct tty-cursor pastel`
1. build netdata
1. Run `make` in this folder. If you have `kaitai-struct-compiler` installed and in `ENV[PATH]` it will generate parsers for netdata datafiles and netdata journalfiles for Ruby and Python languages. This is in turn used by various small ruby/python scripts used in this foled.

### tools

#### `njfv2_effectiness.rb`

<details>
    <summary>Sample output</summary>

```
Stats Per File
==============
   31.06 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000001.njfv2"
   32.43 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000002.njfv2"
   18.64 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000003.njfv2"
   33.79 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000004.njfv2"
   31.51 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000005.njfv2"
   43.57 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000006.njfv2"
   45.24 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000007.njfv2"
   33.29 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000008.njfv2"
   36.33 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000009.njfv2"
   26.86 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000010.njfv2"
    9.26 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000011.njfv2"
    5.36 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000012.njfv2"
   28.71 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000013.njfv2"
   17.03 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000014.njfv2"
   28.89 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000015.njfv2"
   32.86 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000016.njfv2"
   24.48 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000017.njfv2"
   25.10 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000018.njfv2"
   32.23 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000019.njfv2"
   33.71 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000020.njfv2"
   23.88 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000021.njfv2"
   23.10 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000022.njfv2"
   24.16 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000023.njfv2"
   25.70 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000024.njfv2"
   30.46 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000025.njfv2"
   31.30 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000026.njfv2"
   20.39 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000027.njfv2"
   38.53 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000028.njfv2"
   20.11 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000029.njfv2"
   34.13 % is padding in "/var/cache/netdata/dbengine/journalfile-1-0000000030.njfv2"
   35.69 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000058.njfv2"
   35.37 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000059.njfv2"
   34.20 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000060.njfv2"
   34.31 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000061.njfv2"
   35.61 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000062.njfv2"
   36.62 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000063.njfv2"
   39.04 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000064.njfv2"
   32.92 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000065.njfv2"
   36.21 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000066.njfv2"
   36.45 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000067.njfv2"
   34.61 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000068.njfv2"
   39.34 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000069.njfv2"
   34.85 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000070.njfv2"
   35.10 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000071.njfv2"
   39.81 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000072.njfv2"
   24.07 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000073.njfv2"
   39.49 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000074.njfv2"
   16.98 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000075.njfv2"
   45.20 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000076.njfv2"
   17.03 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000077.njfv2"
   39.72 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000078.njfv2"
   35.09 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000079.njfv2"
   35.97 % is padding in "/var/cache/netdata/dbengine-tier1/journalfile-1-0000000080.njfv2"
   50.54 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000103.njfv2"
   40.53 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000104.njfv2"
   50.25 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000105.njfv2"
   35.26 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000106.njfv2"
   51.92 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000107.njfv2"
   50.41 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000108.njfv2"
   50.24 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000109.njfv2"
   50.55 % is padding in "/var/cache/netdata/dbengine-tier2/journalfile-1-0000000110.njfv2"

Totals:
==============
  AVG % per file:       33.04 %
  Padding total:        17130.81 MB out of total 46515.11 MB
  Padding total:        36.83 %
```

</details>
