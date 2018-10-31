# COLLECTOR PLUGIN TEMPLATE

_The title must be replaced with the plugin name_

_For a filled in sample, see [[freeipmi.plugin/README]]_

Parameter | Value |
:---------|:------|
Short Description | SHORT_DESCRIPTION |
Category | CATEGORY |
Sub-Category | SUBCATEGORY | 
Plugin | PLUGIN_IDENTIFIER.plugin |
Prog. Language | PROG_LANGUAGE | 
Config file | CONFIG_FILE |
Alarms config | ALARM_CONFIG_FILE |
Dependencies |  DEPENDENCIES |
Live Demo | URL_OR_SUBURL |

_The README always starts with the table above after the main header. Any other content in this section will not be present in the documentation.
- CATEGORY is one of the following: Web, Cloud, Data Store, Messaging (Queues), Monitoring Tools, Operating System, Application Instrumentation, Other
- SUBCATEGORY could be Application, Metric type (e.g.) 
- PLUGIN_IDENTIFIER The first part of the directory the plugin is in. e.g. freeipmi, python.d etc.
- DEPENDENCIES contains a one line description of what is needed outside of netdata for the plugin to work
- URL_OR_SUBURL: If a full URL appears here, we will show it as is and also parse the path after the hash tag. If it's not a link format, we'll assume that we are given the path after the hash tag.
_

## Introduction

_This can contain subsections and will appear as is. It should give a high level explanation of what is monitored and how, without going into extreme details._

## Charts

_Detailed list of metrics collected, one line per type of CHART generated. Notice the exact match with the parameters expected to the CHART output line. The "options" column does not need to be detailed, but we want the table to explain what we will see in each chart, so use your judgement _

type.id | name | title | units | family | context | charttype | options |
:-------|:-----|:------|:------|:-------|:--------|:----------|:------|
ipmi.temperatures_c | | System Celcius Temperatures read by IPMI | Celcius | temperatures | ipmi.temperatures_c | line | |
ipmi.volts | | System Voltages read by IPMI | Volts | voltages | ipmi.voltages | line | |

## Alarms

_Explanation of the default alarms configured in the ALARM_CONFIG_FILE_

## Installation

_e.g. do I need to install an external application, do I need to configure my application so it can be monitored by netdata, do I need to restart netdata etc._

## Usage

_Anything specific that we want to say about what we see in the charts, disclaimers, use cases in which the data may be misinterpreted etc. May include screenshots of charts._

## Notes

_Anything additional we believe the users should know about this plugin_
