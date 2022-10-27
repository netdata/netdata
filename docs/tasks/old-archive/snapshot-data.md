<!--
title: "Snapshot data"
sidebar_label: "Snapshot data"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/miscellaneous/snapshot-data.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "operations"
learn_docs_purpose: "Instructions on how to take snapshots of data"
-->

Netdata can export snapshots of the contents of your dashboard at a given time, which you can then import into any other
node running Netdata. Or, you can create a print-ready version of your dashboard to save to PDF or actually print to
paper.

Snapshots can be incredibly useful for diagnosing anomalies after they've already happened. Let's say Netdata triggered
a warning alarm while you were asleep. In the morning, you can select the timeframe when the alarm triggered, export a
snapshot, and send it to a colleague for further analysis.

## Prerequisites

- A node with the Agent installed, and access to that node's local dashboard

## Export a snapshot

To export a snapshot:

1. Pan/zoom any chart to an appropriate _visible timeframe_. The export snapshot will only
   contain the metrics you see in charts, so choose the most relevant timeframe.
2. Click on the **export** icon in the top panel.
3. Select the metrics resolution to export. The default is 1-second, equal to how often Netdata collects and stores
   metrics. Lowering the resolution will reduce the number of data points, and thus the snapshot's overall size.
4. Edit the snapshot file name and select your desired compression method. Click on **Export**. When the export is
   complete, your browser will prompt you to save the `.snapshot` file to your machine.

## Import a snapshot

To import a snapshot:

1. Click on the **import** icon in the top panel.
2. Select the Netdata snapshot file to import.
3. Once the file is loaded, the modal updates with information about the snapshot and the system from which it was
   taken.
4. Click **Import** to begin the processing.

Netdata takes the data embedded inside the snapshot and re-creates a static replica on your dashboard. When the import
finishes, you're free to move around and examine the charts.

:::note

- Only metrics in the export timeframe are available to you. If you zoom out or pan through time, you'll see the
  beginning and end of the snapshot.
- Charts won't update with new information, as you're looking at a static replica, not the live dashboard.
- The import is only temporary. Reload your browser tab to return to your node's real-time dashboard.

:::

## Print a snapshot

To print a snapshot:

1. Click on the **print** icon in the top panel.
2. When you click **Print**, Netdata opens a new window to render every chart. This might take some time. When finished,
   Netdata opens a browser print dialog for you to save to PDF or print.