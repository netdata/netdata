# Windows Events plugin

[KEY FEATURES](#key-features) | [EVENTS SOURCES](#events-sources) | [EVENT FIELDS](#event-fields) |
[PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [PERFORMANCE](#query-performance) |
[CONFIGURATION](#configuration-and-maintenance) | [FAQ](#faq)

The Windows Events plugin by Netdata makes viewing, exploring and analyzing Windows Events simple and
efficient.

![image](https://github.com/user-attachments/assets/71a1ab1d-5b7b-477e-a4e6-a30275a5710b)

## Key features

- Supports **Windows Event Logs (WEL)**.
- Supports **Event Tracing for Windows (ETW)** and **TraceLogging (TL)**, when events are routed to Event Log.
- Allows filtering on all System Events fields.
- Allows **full text search** (`grep`) on all System and User fields.
- Provides a **histogram** for log entries over time, with a break down per field-value, for any System Event field and any
  time-frame.
- Supports coloring log entries based on severity.
- In PLAY mode it "tails" all the Events, showing new log entries immediately after they are received.

### Prerequisites

`windows-events.plugin` is a Netdata Function Plugin.

To protect your privacy, as with all Netdata Functions, a free Netdata Cloud user account is required to access it.
For more information check [this discussion](https://github.com/netdata/netdata/discussions/16136).

## Events Sources

The plugin automatically detects all the available channels and offers a list of "Event Channels".

By default, it aggregates events from all event channels, providing a unified view of all events.

> To improve query performance, we recommend selecting the relevant event channels, before doing more
> analysis on the events.

In the list of events channels, several shortcuts are added, aggregating events according to various attributes:

- `All`, aggregates events from all available channels. This provides a holistic view of all events in the system. 
- `All-Admin`, `All-Operational`, `All-Analytic` and `All-Debug` aggregates events from channels marked `Admin`, `Operational`, `Analytic` and `Debug`, respectively.
- `All-Windows`, aggregates events from `Application`, `Security`, `System` and `Setup`.
- `All-Enabled` and `All-Disabled` aggregates events from channels depending on their status.
- `All-Forwarded` aggregates events from channels owned by `Microsoft-Windows-EventCollector`.
- `All-Classic` aggregates events from channels using the Classic Event Log API.
- `All-Of-X`, where `X` is a provider name, is offered for all providers having more than a channel.
- `All-In-X`, where `X` is `Backup-Mode`, `Overwrite-Mode`, `StopWhenFull-Mode` and `RetainAndBackup-Mode`, aggregate events based on their channel retention policy.

Channels that are configured but are not queryable, and channels that do not have any events in them, are automatically excluded from the channels list.

## Event Fields

Windows Events are structured with both system-defined fields and user-defined fields.
The Windows Events plugin primarily works with the system-defined fields, which are consistently available
across all event types.

### System-defined fields 

The system-defined fields are:

1. **EventRecordID**  
   A unique, sequential identifier for the event within the channel. This ID increases as new events are logged.

2. **Version**  
   The version of the event, indicating possible structural changes or updates to the event definition.

   Netdata adds this field automatically when it is not zero.

3. **Level**  
   The severity or importance of the event. Levels can include:
   - 0: LogAlways (reserved)
   - 1: Critical
   - 2: Error
   - 3: Warning
   - 4: Informational
   - 5: Verbose
   
   Additionally, applications may define their own levels.

   Netdata provides 2 fields: `Level` and `LevelID` for the text and numeric representation of it.

4. **Opcode**  
   The action or state within a provider when the event was logged.
   
   Netdata provides 2 fields: `Opcode` and `OpcodeID` for the text and numeric representation of it.

5. **EventID**  
   This identifies the event template, linking it to a specific message or event type. Event IDs are provider-specific.

6. **Task**  
   Defines a higher-level categorization or logical grouping for the event, often related to a specific function within the application or provider.

   Netdata provides 2 fields: `Task` and `TaskID` for the text and numeric representation of it.

7. **Qualifiers**  
   Provides additional detail for interpreting the event and is often specific to the event source.

   Netdata adds this field automatically when it is not zero.

8. **ProcessID**
   The ID of the process that generated the event, useful for pinpointing the source of the event within the system.

9. **ThreadID**  
   The ID of the thread within the process that generated the event, which helps in more detailed debugging scenarios.

10. **Keywords**  
    A categorization field that can be used for event filtering. Keywords are bit flags that represent categories or purposes of the event, providing additional context.

    Netdata provides 2 fields: `Keywords` and `keywordsID` for the text and numeric representation of it.

11. **Provider**  
    The unique identifier (GUID) of the event provider. This is essential for knowing which application or system component generated the event.

    Netdata provides 2 fields: `Provider` and `ProviderGUID` for its name and GUID of it.

12. **ActivityID**  
    A GUID that correlates events generated as part of the same operation or transaction, helping to track activities across different components or stages.

    Netdata adds this field automatically when it is not zero.

13. **RelatedActivityID**  
    A GUID that links related operations or transactions, allowing for tracing complex workflows where one event triggers or relates to another.

    Netdata adds this field automatically when it is not zero.

14. **Timestamp**  
    The timestamp when the event was created. This provides precise timing information about when the event occurred.

15. **User**
    The system user who logged this event.

    Netdata provides 3 fields: `UserAccount`, `UserDomain` and `UserSID`.

### User-defined fields
Each event log entry can include up to 100 user-defined fields (per event-id).

Unfortunately, accessing these fields is significantly slower, to a level that is not practical to do so
when there are more than few thousand log entries to explore. So, Netdata presents them
with lazy loading.

This prevents Netdata for offering filtering for user-defined fields, although Netdata does support
full text search on user-defined field values.

### Event fields as columns in the table

The system fields mentioned above are offered as columns on the UI. Use the gear button above the table to
select visible columns.

### Event fields as filters

The plugin presents the system fields as filters for the query, with counters for each of the possible values
for the field. This list can be used to quickly check which fields and values are available for the entire
time-frame of the query, across multiple providers and channels.

### Event fields as histogram sources

The histogram can be based on any of the system fields that are available as filters. This allows you to
visualize the distribution of events over time based on different criteria such as Level, Provider, or EventID.

## PLAY mode

The PLAY mode in this plugin allows real-time monitoring of new events as they are added to the Windows Event
Log. This feature works by continuously querying for new events and updating the display.

## Full-text search

The plugin supports searching for text within all system and user fields of the events. This means that while
user-defined fields are not directly filterable, they are searchable through the full-text search feature.

Keep in mind that query performance is slower while doing full text search, mainly because the plugin
needs to ask from the system to provide all the user fields values.

## Query performance

The plugin is optimized to work efficiently with Event Logs. It uses several layers of caching and
similar techniques to offload as much work as possible from the system, offering quick responses even when
hundreds of thousands of events are within the visible timeframe.

To achieve this level of efficiency, the plugin:

- pre-loads ETW providers' manifests for resolving numeric Levels, Opcodes, Tasks and Keywords to text.
- caches number to text maps for Levels, Opcodes, Tasks and Keywords per provider for WEL providers.
- caches user SID to account and domain maps.
- lazy loads the "expensive" event Message and XML, so that the system is queried only for the visible events.  

For Full Text Search:

- requests only the Message and the values of the user-fields from the system, avoiding the "expensive" XML call (which is still lazy-loaded). 

The result is a system that is highly efficient for working with moderate volumes (hundreds of thousands) of events.

## Configuration and maintenance

This Netdata plugin does not require any specific configuration. It automatically detects available event logs
on the system.

## FAQ

### Can I use this plugin on event centralization servers?

Yes. You can centralize your Windows Events using Windows Event Forwarding (WEF) or other event collection
mechanisms, and then install Netdata on this events centralization server to explore the events of all your
infrastructure.

This plugin will automatically provide multi-node views of your events and also give you the ability to
combine the events of multiple servers, as you see fit.

### Can I use this plugin from a parent Netdata?

Yes. When your nodes are connected to a Netdata parent, all their functions are available via the parent's UI.
So, from the parent UI, you can access the functions of all your nodes.

Keep in mind that to protect your privacy, in order to access Netdata functions, you need a free Netdata Cloud
account.

### Is any of my data exposed to Netdata Cloud from this plugin?

No. When you access the Agent directly, none of your data passes through Netdata Cloud. You need a free Netdata
Cloud account only to verify your identity and enable the use of Netdata Functions. Once this is done, all the
data flow directly from your Netdata Agent to your web browser.

When you access Netdata via https://app.netdata.cloud, your data travel via Netdata Cloud, but they are not stored
in Netdata Cloud. This is to allow you access your Netdata Agents from anywhere. All communication from/to
Netdata Cloud is encrypted.

### What are the different types of event logs supported by this plugin?

The plugin supports all the kinds of event logs currently supported by the Windows Event Viewer:

- Windows Event Logs (WEL): The traditional event logging system in Windows.
- Event Tracing for Windows (ETW): A more detailed and efficient event tracing system.
- TraceLogging (TL): An extension of ETW that simplifies the process of adding events to your code.

The plugin can access all of these when they are routed to the Windows Event Log.

### How does this plugin handle user-defined fields in Windows Events?

User-defined fields are not directly exposed as table columns or filters in the plugin interface. However,
they are included in the XML representation of each event, which can be viewed in the info sidebar when
clicking on an event entry. Additionally, the full-text search feature does search through these
user-defined fields, allowing you to find specific information even if it's not in the main system fields.

### Can I use this plugin to monitor real-time events?

Yes, the plugin supports a PLAY mode that allows you to monitor events in real-time. When activated, it
continuously updates to show new events as they are logged, similar to the "tail" functionality in
Unix-like systems.

### How does the plugin handle large volumes of events?

The plugin is designed to handle moderate volumes of events (hundreds of thousands of events) efficiently.

It is in our roadmap to port the `systemd-journal` sampling techniques to it, for working with very large
datasets to provide quick responses while still giving accurate representations of the data. However, for
the best performance, we recommend querying smaller time frames or using more specific filters when dealing
with extremely large event volumes.

### Can I use this plugin to analyze events from multiple servers?

Yes, if you have set up Windows Event Forwarding (WEF) or another method of centralizing your Windows Events,
you can use this plugin on the central server to analyze events from multiple sources. The plugin will
automatically detect the available event sources.

### How does the histogram feature work in this plugin?

The histogram feature provides a visual representation of event frequency over time. You can base the
histogram on any of the system fields available as filters (such as Level, Provider, or EventID). This
allows you to quickly identify patterns or anomalies in your event logs.

### Is it possible to export or share the results from this plugin?

While the plugin doesn't have a direct export feature, you can use browser-based methods to save or share
the results. This could include taking screenshots, using browser print/save as PDF functionality, or
copying data from the table view. For more advanced data export needs, you might need to use the Windows
Event Log API directly or other Windows administrative tools.

### How often does the plugin update its data?

The plugin updates its data in real-time when in PLAY mode. In normal mode, it refreshes data based on the
query you've submitted. The plugin is designed to provide the most up-to-date information available in the
Windows Event Logs at the time of the query.

## TODO

1. Support Sampling, so that the plugin can respond faster even on very busy systems (millions of events visible).
2. Support exploring events from live Tracing sessions.
3. Support exploring events in saved Event Trace Log files (`.etl` files).
4. Support exploring events in saved Event Logs files (`.evtx` files).
