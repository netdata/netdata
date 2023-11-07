# windows_10min_cpu_usage

## Windows | CPU

This alarm calculates the average of CPU utilization over a period of 10 minutes.

It is raised into warning if the value exceeds 85%.  
If the average exceeds 95%, then the alert gets raised into critical.

### Troubleshooting Section

<details>
<summary>Processes slowing down your CPU</summary>

In Windows, you can open up the Task Manager from the menu or by pressing
`ctrl`+`shift`+`esc`.

- Under the processes tab, you can see a list of the processes currently running on the machine.
    - To get a better picture of the main consumers, order them by their total CPU usage by
      clicking the CPU column. That will sort the top main processes utilizing your CPU.


- To get a more detailed look, click the "Performance" tab (next to the processes tab) and
  then click on the bottom of the window "Open Resource Monitor".
    - That will open up a window with a more detailed view on the processes.
    - On the "Processes" table, look for the column "Average CPU". Clicking this will order the
      processes again by CPU utilization.

> It would be helpful to close any of the main consumer processes, but Netdata strongly suggests
> knowing exactly what processes you are closing and being certain that they are not necessary to
> your workflow or system.
</details>
