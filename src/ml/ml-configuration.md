# ML Configuration

You can use Netdata's [Machine Learning](/src/ml/README.md) capabilities to detect anomalies in your infrastructure metrics. This feature is enabled by default if your [Database mode](/src/database/README.md) is set to `db = dbengine`.

## Enabling or Disabling Machine Learning

To enable or disable Machine Learning capabilities on your node:

1. [Edit `netdata.conf`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config).
2. In the `[ml]` section:
    - Set `enabled` to `yes` to enable ML.
    - Set `enabled` to `no` to disable ML.
    - Leave it at the default `auto` to enable ML only when [Database mode](/src/database/README.md) is set to `dbengine`.
3. [Restart Netdata](/docs/netdata-agent/start-stop-restart.md).

## Technical Implementation

Netdata implements machine learning using the lightweight [dlib](https://github.com/davisking/dlib) C++ library. This implementation choice allows Netdata to:

```mermaid
flowchart TD
    %% Node styling
    classDef neutral fill:#f9f9f9,stroke:#000000,color:#000000,stroke-width:2px
    classDef success fill:#4caf50,stroke:#000000,color:#000000,stroke-width:2px
    classDef warning fill:#ffeb3b,stroke:#000000,color:#000000,stroke-width:2px
    classDef danger fill:#f44336,stroke:#000000,color:#000000,stroke-width:2px
    
    Dlib[dlib C++ Library Implementation]
    
    Dlib --> Efficiency[Run efficiently on<br/>any system without<br/>heavy dependencies]
    Dlib --> Resources[Minimize resource usage<br/>while maintaining<br/>high accuracy]
    Dlib --> Codebase[Operate within the<br/>constraints of agent's<br/>C/C++ codebase]
    
    %% Apply styles
    class Dlib warning
    class Efficiency,Resources,Codebase success
```

:::note

The ML implementation prioritizes minimal storage impact. Model objects are designed to be small, with each trained model typically requiring only a few kilobytes of memory.

:::

## Available Configuration Parameters

Below is a list of all available configuration parameters and their default values:

```bash
[ml]
        # enabled = auto
        # maximum num samples to train = 21600
        # minimum num samples to train = 900
        # train every = 3h
        # number of models per dimension = 18
        # dbengine anomaly rate every = 30
        # num samples to diff = 1
        # num samples to smooth = 3
        # num samples to lag = 5
        # random sampling ratio = 0.2
        # maximum number of k-means iterations = 1000
        # dimension anomaly score threshold = 0.99
        # host anomaly rate threshold = 1.0
        # anomaly detection grouping method = average
        # anomaly detection grouping duration = 5m
        # hosts to skip from training = !*
        # charts to skip from training = netdata.*
        # dimension anomaly rate suppression window = 15m
        # dimension anomaly rate suppression threshold = 450
        # delete models older than = 7d
```

## Multiple Models and False Positive Reduction

One of the **key strengths of Netdata's ML implementation is its use of multiple models** to virtually eliminate false positives.

When you configure the `number of models per dimension` parameter (default: 18), you're specifying how many trained models Netdata maintains for each metric.

:::note

The default setting of 18 models spanning approximately 54 hours (with models trained every 3 hours) ensures that anomalies are verified across different time frames before being flagged.

:::

### How Multiple Models Work Together

When your system detects a potential anomaly:

1. **The new data point is evaluated against all trained models** (up to 18 by default)
2. **Each model independently determines if the data point is anomalous** based on its training period
3. Only if **ALL** models agree that the data point is anomalous will Netdata set the anomaly bit to 1 (anomalous)

```mermaid
flowchart TD
    %% Node styling
    classDef neutral fill:#f9f9f9,stroke:#000000,color:#000000,stroke-width:2px
    classDef success fill:#4caf50,stroke:#000000,color:#000000,stroke-width:2px
    classDef warning fill:#ffeb3b,stroke:#000000,color:#000000,stroke-width:2px
    classDef danger fill:#f44336,stroke:#000000,color:#000000,stroke-width:2px
    
    %% Input data
    NewData[New Metric Data Point] --> M1 & M2 & M3 & M4
    
    %% Models
    subgraph Models[Multiple Time-Scale Models]
        M1[Model 1<br/>Recent 3h]
        M2[Model 2<br/>3-6h ago]
        M3[Model 3<br/>6-9h ago]
        M4[Model N<br/>Older periods]
    end
    
    %% Decisions
    M1 --> D1{Anomalous?}
    M2 --> D2{Anomalous?}
    M3 --> D3{Anomalous?}
    M4 --> D4{Anomalous?}
    
    %% Results
    D1 -->|Yes| R1[Model 1: Anomaly]
    D1 -->|No| N1[Model 1: Normal]
    D2 -->|Yes| R2[Model 2: Anomaly]
    D2 -->|No| N2[Model 2: Normal]
    D3 -->|Yes| R3[Model 3: Anomaly]
    D3 -->|No| N3[Model 3: Normal]
    D4 -->|Yes| R4[Model N: Anomaly]
    D4 -->|No| N4[Model N: Normal]
    
    %% Consensus
    R1 & R2 & R3 & R4 --> AllYes{All<br/>Models<br/>Agree?}
    N1 & N2 & N3 & N4 ---> AllNo[Set Anomaly Bit = 0<br/>Normal]
    AllYes -->|Yes| AllAnom[Set Anomaly Bit = 1<br/>Anomalous]
    AllYes -->|No| AllNo
    
    %% Apply styles
    class NewData neutral
    class M1,M2,M3,M4 success
    class D1,D2,D3,D4,AllYes warning
    class R1,R2,R3,R4,AllAnom danger
    class N1,N2,N3,N4,AllNo neutral
```

:::note

This consensus approach **reduces false positives by approximately 99%**, ensuring that you only see alerts for genuine anomalies that persist across different time scales.

:::

**Models trained on different time frames capture various patterns in your data**, making the system robust against temporary fluctuations.

## Configuration Examples

If you want to **run ML on a parent instead of at the edge**, the examples below illustrate various configurations.

This example assumes three child nodes [streaming](/docs/observability-centralization-points/metrics-centralization-points/README.md) to one parent node. It shows different ways to configure ML:

- Running ML on the parent for some or all children.
- Running ML on the children themselves.
- A mixed approach.

```mermaid
flowchart BT
    %% Node styling
    classDef neutral fill:#f9f9f9,stroke:#000000,color:#000000,stroke-width:2px
    classDef success fill:#4caf50,stroke:#000000,color:#000000,stroke-width:2px
    classDef warning fill:#ffeb3b,stroke:#000000,color:#000000,stroke-width:2px
    classDef danger fill:#f44336,stroke:#000000,color:#000000,stroke-width:2px
    
    C1["Netdata Child 0\nML<br/>enabled"]
    C2["Netdata Child 1\nML<br/>enabled"]
    C3["Netdata Child 2\nML<br/>disabled"]
    P1["Netdata Parent\n<br/>(ML enabled for itself<br/>and Child 1 & 2)"]
    C1 --> P1
    C2 --> P1
    C3 --> P1
    
    %% Apply styles
    class C1,C2 success
    class C3 neutral
    class P1 warning
```

```text
# Parent will run ML for itself and Child 1 & 2, but skip Child 0.
# Child 0 and Child 1 will run ML independently.
# Child 2 will rely on the parent for ML and will not run it itself.

# Parent configuration
[ml]
        enabled = yes
        hosts to skip from training = child-0-ml-enabled

# Child 0 configuration
[ml]
        enabled = yes

# Child 1 configuration
[ml]
        enabled = yes

# Child 2 configuration
[ml]
        enabled = no
```

## Resource Considerations

When configuring machine learning, it's important to understand the resource implications:

:::note

Netdata's ML is designed to be lightweight, using approximately 1-2% of a single CPU core under default settings on a typical system.

:::

Several configuration options directly impact resource usage:

- **Increasing `number of models per dimension`** increases memory usage as more models are stored
- **Decreasing `train every`** increases CPU usage as models are trained more frequently
- **Increasing `maximum num samples to train`** increases memory usage during training but may improve accuracy
- **Adjusting `random sampling ratio`** allows you to control how much data is used during training (lower values reduce CPU usage)

For resource-constrained systems, consider these adjustments:

- Set `random sampling ratio = 0.1` to reduce training data by half from default
- Use `number of models per dimension = 6` to maintain multiple model consensus with reduced memory footprint
- Increase `train every = 6h` to reduce training frequency

## Parameter Descriptions (Min/Max Values)

<details>
<summary><strong>General Settings</strong></summary><br/>

- **`enabled`**: Controls whether ML is enabled for your node.
    - `yes` to enable.
    - `no` to disable.
    - `auto` lets Netdata decide based on database mode.

- **`maximum num samples to train`** (`3600` - `86400`): Defines the maximum training period. The default of `21600` trains on your last 6 hours of data.

- **`minimum num samples to train`** (`900` - `21600`): The minimum amount of data needed to train a model for your metrics. If less than `900` samples (15 minutes of data) are available, training is skipped.

- **`train every`** (`3h` - `6h`): Determines how often your models are retrained. The default of `3h` means retraining occurs every three hours. Training is staggered to distribute system load.

</details>

<details>
<summary><strong>Model Behavior</strong></summary><br/>

- **`number of models per dimension`** (`1` - `168`): Specifies how many trained models per dimension are used for anomaly detection in your infrastructure. The default of `18` means models trained over the last ~54 hours are considered.

- **`dbengine anomaly rate every`** (`30` - `900`): Defines how frequently Netdata aggregates your anomaly bits into a single chart.

</details>

<details>
<summary><strong>Feature Processing</strong></summary><br/>

- **`num samples to diff`** (`0` - `1`): Determines whether ML operates on your raw data (`0`) or differences (`1`). Using differences helps detect anomalies in cyclical patterns in your metrics.

- **`num samples to smooth`** (`0` - `5`): Controls data smoothing. The default of `3` averages the last three values to reduce noise in your metrics.

- **`num samples to lag`** (`0` - `5`): Defines how many past values are included in the feature vector. The default `5` helps the model detect patterns over time in your data.

</details>

<details>
<summary><strong>Training Efficiency</strong></summary><br/>

- **`random sampling ratio`** (`0.2` - `1.0`): Controls the fraction of your data used for training. The default `0.2` means 20% of available data is used, reducing system load while maintaining accuracy.

- **`maximum number of k-means iterations`**: Limits iterations during k-means clustering (leave at default in most cases).

</details>

<details>
<summary><strong>Anomaly Detection Sensitivity</strong></summary><br/>

- **`dimension anomaly score threshold`** (`0.01` - `5.00`): Sets the threshold for flagging an anomaly in your metrics. The default `0.99` flags values that are in the top 1% of anomalies based on training data.

- **`host anomaly rate threshold`** (`0.1` - `10.0`): Defines the percentage of dimensions that must be anomalous for your host to be considered anomalous. The default `1.0` means more than 1% must be anomalous.

</details>

<details>
<summary><strong>Anomaly Detection Grouping</strong></summary><br/>

- **`anomaly detection grouping method`**: Defines the method used to calculate your node-level anomaly rate.

- **`anomaly detection grouping duration`** (`1m` - `15m`): Determines the time window for calculating your anomaly rates. The default `5m` calculates anomalies over a 5-minute rolling window.

</details>

<details>
<summary><strong>Skipping Hosts and Charts</strong></summary><br/>

- **`hosts to skip from training`**: Allows excluding specific child hosts from training. The default `!*` means no hosts are skipped.

- **`charts to skip from training`**: Excludes charts from anomaly detection. By default, Netdata-related charts are excluded to prevent false anomalies caused by normal dashboard activity.

</details>

<details>
<summary><strong>Model Retention</strong></summary><br/>

- **`delete models older than`** (`1d` - `7d`): Defines how long old models are stored. The default `7d` removes unused models after seven days.

</details>
