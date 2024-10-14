# Windows Events plugin

[KEY FEATURES](#key-features) | [EVENTS SOURCES](#events-sources) | [EVENT FIELDS](#event-fields) |
[PLAY MODE](#play-mode) | [FULL TEXT SEARCH](#full-text-search) | [PERFORMANCE](#query-performance) |
[CONFIGURATION](#configuration-and-maintenance) | [FAQ](#faq)

The Windows Events plugin by Netdata makes viewing, exploring and analyzing Windows Events simple and
efficient.

IMAGE

## Key features

- Supports Windows Events Logs (WEL)
- Supports Events Tracing for Windows (ETW) and TraceLogging (TL) when events are routed to Event Viewer.
- Allows filtering on all System Events fields.
- Allows **full text search** (`grep`) on all fields (including all User Data).
- Provides a **histogram** for log entries over time, with a break down per field-value, for any System Event field and any
  time-frame.
- Supports coloring log entries based on severity.
- In PLAY mode it "tails" all the Events, showing new log entries immediately after they are received.

### Prerequisites

`windows-events.plugin` is a Netdata Function Plugin.

To protect your privacy, as with all Netdata Functions, a free Netdata Cloud user account is required to access it.
For more information check [this discussion](https://github.com/netdata/netdata/discussions/16136).

## Events Sources

The plugin automatically detects the available providers.

IMAGE_OF_SOURCES

The plugin, by default, merges all event sources together, to provide a unified view of all log messages available.

> To improve query performance, we recommend selecting the relevant logs source, before doing more analysis on the
> logs.

## Event Fields

Events have a fixed number of system fields:

1. **EventRecordID**  
   A unique, sequential identifier for the event within the channel. This ID increases as new events are logged.

2. **Version**  
   The version of the event, indicating possible structural changes or updates to the event definition.

3. **Level**  
   The severity or importance of the event. Levels can include:
   - 0: LogAlways (reserved)
   - 1: Critical
   - 2: Error
   - 3: Warning
   - 4: Informational
   - 5: Verbose
   
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

13. **RelatedActivityID**  
    A GUID that links related operations or transactions, allowing for tracing complex workflows where one event triggers or relates to another.

14. **Timestamp**  
    The timestamp when the event was created. This provides precise timing information about when the event occurred.

15. **User**
    The system user who logged this event.

    Netdata provides 3 fields: `UserAccount`, `UserDomain` and `UserSID`.

Event Logs can also include up to 100 user defined fields. Unfortunately accessing these fields is significantly slower,
to a level that is not practical, so Netdata presents them with lazy loading, but cannot filter on them. However, full
text search does filter on their values (not their field name).

