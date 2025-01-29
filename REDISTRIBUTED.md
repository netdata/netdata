# Redistributed software

Netdata copyright info:<br/>
Copyright 2018-2025 Netdata Inc.<br/>
Released under [GPL v3 or later](https://raw.githubusercontent.com/netdata/netdata/master/LICENSE).

Netdata uses SPDX license tags to identify the license for its files.
Individual licenses referenced in the tags are available on the [SPDX project site](http://spdx.org/licenses/).

Netdata redistributes the Netdata Cloud UI, licensed under [Netdata Cloud UI License v1.0 (NCUL1)](https://app.netdata.cloud/LICENSE.txt). Netdata Cloud UI includes [third party open-source software](https://app.netdata.cloud/3D_PARTY_LICENSES.txt).

Netdata redistributes the following third-party software.
We have decided to redistribute all these, instead of using them through a CDN, to allow Netdata to work in cases where Internet connectivity is not available.

**Netdata Agent**:

| Name                                                                    | Copyright                                                                          | License                                                                                  |
|-------------------------------------------------------------------------|------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------|
| [Kolmogorov-Smirnov distribution](http://simul.iro.umontreal.ca/ksdir/) | Copyright March 2010 by Université de Montréal, Richard Simard and Pierre L'Ecuyer | [GPL 3.0](https://www.gnu.org/licenses/gpl-3.0.en.html)                                  |
| [xxHash](https://github.com/Cyan4973/xxHash)                            | Copyright (c) 2012-2021 Yann Collet                                                | [BSD](https://github.com/Cyan4973/xxHash/blob/dev/LICENSE)                               |
| [Judy Arrays](https://sourceforge.net/projects/judy/)                   | Copyright (C) 2000 - 2002 Hewlett-Packard Company                                  | [LGPLv2](https://www.gnu.org/licenses/old-licenses/lgpl-2.1.en.html)                     |
| [dlib](https://github.com/davisking/dlib/)                              | Copyright (C) 2019  Davis E. King (davis@dlib.net), Nils Labugt                    | [Boost Software License](https://github.com/davisking/dlib/blob/master/dlib/LICENSE.txt) |

---

**python.d.plugin**:

| Name                                                                                                                                                                                     | Copyright                                             | License                                                          |
|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------|------------------------------------------------------------------|
| [PyYAML](https://pypi.org/project/PyYAML/)                                                                                                                                               | Copyright 2006, Kirill Simonov                        | [MIT](https://github.com/yaml/pyyaml/blob/master/LICENSE)        |
| [urllib3](https://github.com/shazow/urllib3)                                                                                                                                             | Copyright 2008-2016 Andrey Petrov and contributors    | [MIT](https://github.com/shazow/urllib3/blob/master/LICENSE.txt) |
| [Utilities for writing code that runs on Python 2 and 3](https://raw.githubusercontent.com/netdata/netdata/master/src/collectors/python.d.plugin/python_modules/urllib3/packages/six.py) | Copyright (c) 2010-2015 Benjamin Peterson             | [MIT](https://github.com/benjaminp/six/blob/master/LICENSE)      |
| [monotonic](https://github.com/atdt/monotonic)                                                                                                                                           | Copyright 2014, 2015, 2016 Ori Livneh                 | [Apache-2.0](http://www.apache.org/licenses/LICENSE-2.0)         |
| [filelock](https://github.com/benediktschmitt/py-filelock)                                                                                                                               | Copyright 2015, Benedikt Schmitt                      | [Unlicense](https://unlicense.org/)                              |

---

**go.d.plugin**:

| Name                                               | Copyright                   | License                                                             |
|----------------------------------------------------|-----------------------------|---------------------------------------------------------------------|
| [lmsensors](https://github.com/mdlayher/lmsensors) | Copyright 2016, Matt Layher | [MIT](https://github.com/mdlayher/lmsensors/blob/master/LICENSE.md) |
