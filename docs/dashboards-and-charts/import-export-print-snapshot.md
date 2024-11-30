# Import, export, and print a snapshot

> ❗This feature is only available on v1 dashboards, it hasn't been port-forwarded to v2.
> For more information on accessing dashboards, check [this documentation](/docs/dashboards-and-charts/README.md).

Netdata can export snapshots of the contents of your dashboard at a given time, which you can then import into any other
node running Netdata. Or, you can create a print-ready version of your dashboard to save to PDF or actually print to
paper.

Snapshots can be incredibly useful for diagnosing anomalies after they've already happened. Let's say Netdata triggered a warning alert while you were asleep. In the morning, you can [select the
timeframe](/docs/dashboards-and-charts/visualization-date-and-time-controls.md) when the alert triggered, export a snapshot, and send it to a

colleague for further analysis.

Or, send the Netdata team a snapshot of your dashboard when [filing a bug report](https://github.com/netdata/netdata/issues/new?assignees=&labels=bug%2Cneeds+triage&template=BUG_REPORT.yml) on GitHub.

![The export, import, and print
buttons](https://user-images.githubusercontent.com/1153921/114218399-360fb600-991e-11eb-8dea-fabd2bffc5b3.gif)

## Import a snapshot

To import a snapshot, click on the **import** icon ![Import
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/upload.svg)
in the top panel.

Select the Netdata snapshot file to import. Once the file is loaded, the modal updates with information about the
snapshot and the system from which it was taken. Click **Import** to begin to process.

Netdata takes the data embedded inside the snapshot and re-creates a static replica on your dashboard. When the import
finishes, you're free to move around and examine the charts.

Some warnings and tips to keep in mind:

- Only metrics in the export timeframe are available to you. If you zoom out or pan through time, you'll see the
  beginning and end of the snapshot.
- Charts won't update with new information, as you're looking at a static replica, not the live dashboard.
- The import is only temporary. Reload your browser tab to return to your node's real-time dashboard.

## Export a snapshot

To export a snapshot, first pan/zoom any chart to an appropriate _visible timeframe_. The export snapshot will only
contain the metrics you see in charts, so choose the most relevant timeframe.

Next, click on the **export** icon ![Export
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/download.svg)
in the top panel.

Select the metrics resolution to export. The default is 1-second, equal to how often Netdata collects and stores
metrics. Lowering the resolution will reduce the number of data points, and thus the snapshot's overall size.

Edit the snapshot file name and select your desired compression method. Click on **Export**. When the export is
complete, your browser will prompt you to save the `.snapshot` file to your machine.

## Print a snapshot

To print a snapshot, click on the **print** icon ![Import
icon](https://raw.githubusercontent.com/netdata/netdata-ui/98e31799c1ec0983f433537ff16d2ac2b0d994aa/src/components/icon/assets/print.svg)
in the top panel.

When you click **Print**, Netdata opens a new window to render every chart. This might take some time. When finished,
Netdata opens a browser print dialog for you to save to PDF or print.
