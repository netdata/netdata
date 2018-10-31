# Contributing

As with all open source projects, **the more people using it, the better the project is**. So give it a github star, post about it on facebook, twitter, reddit, google+, hacker news, etc. Spreading the word costs you nothing and helps the project improve. It is the minimum you should give back for using a project that has thousands of hours of work in it and you get it for free.

Also important is to **open github issues** for the things that are not working well for you. This will help us make netdata better.

These are others areas we need help:

- _Can you code?_
 - you can **write plugins for data collection**. This is very easy and any language can be used. Please see [External Plugins](External-Plugins.md)
 - you can **write dashboards**, specially optimised for monitoring the applications you use.

- _Do you have special skills?_
 - are you a **marketing** guy? Help us setup a **social media strategy** to build and grow the netdata community.
 - are you a **devops** guy? Help us setup and maintain the public global servers.
 - are you a **linux packaging** guy? Help us **distribute pre-compiled packages** of netdata for all major distributions, or help netdata be included in official distributions.

## Code of Conduct

This project and everyone participating in it is governed by the [netdata Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to [costa@netdata.cloud](mailto:costa@netdata.cloud).

## Pull Request Checklists

### Node.js Plugins

This is a generic checklist for submitting a new Node.js plugin for Netdata.  It is by no means comprehensive.

At minimum, to be buildable and testable, the PR needs to include:

* The module itself, following proper naming conventions: `node.d/<module_dir>/<module_name>.node.js`
* A README.md file for the plugin, using the [template](README-Template). 
* The configuration file for the module
* A basic configuration for the plugin in the appropriate global config file: `conf.d/node.d.conf`, which is also in JSON format.  If the module should be enabled by default, add a section for it in the `modules` dictionary.
* A line for the plugin in the appropriate `Makefile.am` file: `node.d/Makefile.am` under `dist_node_DATA`.
* A line for the plugin configuration file in `conf.d/Makefile.am`: under `dist_nodeconfig_DATA`
* Optionally, chart information in `web/dashboard_info.js`.  This generally involves specifying a name and icon for the section, and may include descriptions for the section or individual charts.

### Python Plugins

This is a generic checklist for submitting a new Python or Node.js plugin for Netdata.  It is by no means comprehensive.

At minimum, to be buildable and testable, the PR needs to include:

* The module itself, following proper naming conventions: `python.d/<module_dir>/<module_name>.chart.py`
* A README.md file for the plugin under `python.d/<module_dir>` using the [template](README-Template). 
* The configuration file for the module: `conf.d/python.d/<module_name>.conf`. Python config files are in YAML format, and should include comments describing what options are present. The instructions are also needed in the configuration section of the README.md 
* A basic configuration for the plugin in the appropriate global config file: `conf.d/python.d.conf`, which is also in YAML format.  Either add a line that reads `# <module_name>: yes` if the module is to be enabled by default, or one that reads `<module_name>: no` if it is to be disabled by default.
* A line for the plugin in `python.d/Makefile.am` under `dist_python_DATA`.
* A line for the plugin configuration file in `conf.d/Makefile.am`, under `dist_pythonconfig_DATA`
* Optionally, chart information in `web/dashboard_info.js`.  This generally involves specifying a name and icon for the section, and may include descriptions for the section or individual charts.

