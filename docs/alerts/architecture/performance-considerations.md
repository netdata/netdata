# 13.5 Performance Considerations

Alert evaluation consumes CPU and memory resources. Understanding resource usage helps in capacity planning and optimization.

## CPU Usage

Alert evaluation is CPU-intensive. Each evaluation iterates through all enabled alerts, executing lookup queries and calculation expressions.

The evaluation interval affects CPU usage inversely. More frequent evaluation increases CPU consumption but reduces detection latency.

Profile alert performance using `netdata-agent-bench` to identify heavy alerts.

## Memory Usage

Alert configuration consumes memory proportional to alert count and complexity. Each alert instance requires space for expression evaluation state, variable contexts, and historical lookups.

Complex lookups that retrieve large time windows consume more memory than simple lookups.

## Evaluation Latency

The health engine should complete evaluation within its configured interval. An evaluation cycle that exceeds its interval causes evaluation backlog.

Monitor evaluation latency using `netdatacli health status`. The output shows recent evaluation timing and can identify backlogs before they impact detection.