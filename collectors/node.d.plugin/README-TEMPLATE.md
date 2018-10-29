# Node.js Module README Template

_The title must be replaced with the plugin name_

Parameter | Value |
:---------|:------|
Short Description | SHORT_DESCRIPTION |
Category | CATEGORY |
Sub-Category | SUBCATEGORY | 
Orchestrator | node.d.plugin |
Module | COLLECTOR_IDENTIFIER |
Prog. Language | JavaScript | 
Config file | CONFIG_FILE |
Alarms config | ALARM_CONFIG_FILE |
System Requirements | REQUIREMENTS |
External Dependencies |  DEPENDENCIES |

_The README always starts with the table above after the main header. Any other content in this section will not be present in the documentation.
- CATEGORY is one of the following: Web, Cloud, Data Store, Messaging (Queues), Monitoring Tools, Operating System, Application Instrumentation, Other
- SUBCATEGORY could be Application, Metric type (e.g.) 
- PLUGIN_IDENTIFIER The first part of the directory the plugin is in. e.g. freeipmi, python.d etc.
- REQUIREMENTS lists what's needed on the system on which netdata is installed, for the module to run. 
- DEPENDENCIES contains a one line description of what is needed on the system monitored by netdata, for the particular plugin to be able to collect metrics
_

## Introduction

_This can contain subsections and will appear as is. It should give a high level explanation of what is monitored and how, without going into extreme details._

## Charts

_Detailed list of metrics collected, one line per type of CHART generated. Notice the exact match with the parameters expected to the CHART output line. The "options" column does not need to be detailed, but we want the table to explain what we will see in each chart, so use your judgement _

_For explanation of the columns in the table below, see the [CHART output documentation](../plugins.d/#CHART)_

title | units | family | context |
:-----|:------|:-------|:--------|
System Celcius Temperatures read by IPMI | Celcius | temperatures | ipmi.temperatures_c |
System Voltages read by IPMI | Volts | voltages | ipmi.voltages |

## Alarms

_Explanation of the default alarms configured in the ALARM_CONFIG_FILE_

## Installation

_e.g. do I need to install an external application, do I need to configure my application so it can be monitored by netdata, do I need to restart netdata etc._

## Usage

_Anything specific that we want to say about what we see in the charts, disclaimers, use cases in which the data may be misinterpreted etc. May include screenshots of charts._

## Notes

_Anything additional we believe the users should know about this plugin_
