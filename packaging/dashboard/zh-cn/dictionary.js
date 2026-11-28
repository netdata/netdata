/* Simplified Chinese display strings for the Netdata dashboard. */
(() => {
  'use strict';
  const parse = text => Object.assign(Object.create(null), Object.fromEntries(text.trim().split('\n').map(line => line.split('|').map(s => s.trim()))));
  const exact = parse(`
Item actions|项目操作
Filter...|筛选…
Order groups|分组排序
Status (live first)|状态（在线优先）
Group direction|分组排序方向
Group sort direction|分组排序方向
Sort nodes by|节点排序依据
Direction|排序方向
⌘↵ apply · esc cancel|⌘↵ 应用 · Esc 取消
Severity (critical first)|严重程度（严重优先）
Commissioned first|在用节点优先
Range (lowest first)|数值区间（从低到高）
Count (largest first)|节点数（从多到少）
High → low|从高到低
Low → high|从低到高
Newest first|最新优先
Oldest first|最早优先
No grouping applied|尚未应用分组
Unsorted|未排序
Views can be saved after this Agent is added to a Netdata Cloud space.|将此代理添加到 Netdata Cloud 空间后，即可保存视图。
Search columns...|搜索列…
Fill|填充
Border|边框
Borders off|边框已关闭
No tile selected.|尚未选择节点或分组。
Map key|图例
Fill key|填充图例
Border key|边框图例
Fill color by|填充颜色依据
Border color by|边框颜色依据
Map controls|分布图操作
Locate node|定位节点
Locate node…|定位节点…
Fill Color|填充颜色
Border Color|边框颜色
Alerts status|告警状态
Higher is worse|数值越高越差
Lower is worse|数值越低越差
Empty: colors scale across the range|留空时，颜色按整个数值范围渐变。
Dim normal|淡化正常状态
Dim normal nodes|淡化正常节点
Dim normal borders|淡化正常边框
Hover / click|悬停 / 点击
Preview a tile, or keep its details open|预览节点或分组，或点击以保持详情展开
Locate a node by hostname or IP|按主机名或 IP 地址定位节点
Arrow keys|方向键
Move through groups and nodes|在分组和节点间移动
Home / End|Home / End 键
Jump to the first or last item|跳到第一项或最后一项
Resize hexagons|调整六边形大小
Enter|Enter 键
Open a node or expand a group|打开节点或展开分组
Esc|Esc 键
Clear selection|清除选择
Expand map key|展开图例
Fleet map rendering is unavailable in this browser. Switch to Table for the complete fleet view.|此浏览器无法显示节点分布图。请切换到表格查看完整节点列表。
Use Up and Down to choose options, press Enter to select the currently focused option, press Escape to exit the menu, press Tab to select the option and exit the menu.|按上、下方向键选择选项，按 Enter 选中当前选项，按 Esc 退出菜单，或按 Tab 选中选项并退出菜单。
Node ID|节点 ID
Node note|节点备注
Node notes are unavailable.|节点备注暂不可用。
Machine GUID|主机 GUID
Connection|连接
Netdata Agent:|Netdata 代理：
Cloud Instance Info|云实例信息
Cloud provider:|云服务商：
Cloud region:|云区域：
Instance type:|实例类型：
O/S version:|操作系统版本：
Hard disk size:|磁盘容量：
View node info in|查看节点信息，格式为
Set this timezone on your datetime picker|将日期时间选择器设为此时区
Severity Conditions|严重程度条件
Warning when|警告条件
Check|检查
Explanation|解释
View the documentation|查看文档
Filter by Context, Class, Component...|按指标上下文、分类、组件筛选…
More details about|详细了解
Search charts based on dimensions, instances and labels|按维度、实例和标签搜索图表
Search dimensions, labels...|搜索维度、标签…
Keep typing to narrow results|继续输入以缩小结果范围
Double-check your search or filters and dates and try again with different conditions.|请检查搜索词、筛选条件和日期，然后调整条件重试。
Please wait while charts are being initialized.|正在初始化图表，请稍候。
Couldn't load nodes|无法加载节点
The nodes request failed.|节点请求失败。
We couldn't find any transitions in this time range. Try a different range.|此时段内没有告警状态变更，请尝试其他时段。
Select the alert history data time range|选择告警历史记录的时段
This can be a temporary problem of that specific alert.|这可能是此告警的临时问题。
No data available for the selected time range|所选时段内没有可用数据
Nodes need a supported agent version and collectors to run Functions.|运行功能需要受支持的代理版本和采集器。
Check your connection and try again. We'll retry once you're back online.|请检查网络连接后重试。连接恢复后会自动重试。
Can't reach the agent|无法连接代理
No dynamic configuration here|此处没有动态配置
No running component supports dynamic configuration.|当前运行的组件均不支持动态配置。
Enable health checks on the node to manage alerts here.|请在节点上启用健康检查，以在此管理告警。
Hover a tile to preview its details, or click to keep them open. Press slash to locate a node on the map by hostname or IP address. Use arrow keys to move through displayed groups and nodes, Home or End for the boundaries, plus or minus to resize hexagons, Enter to open a selected node or expand a selected group, and Escape to clear selection. Scroll normally to move through the map. Switch to Table for the complete accessible data view.|将鼠标悬停在节点或分组上可预览详情，点击可保持详情展开。按 / 可通过主机名或 IP 地址定位节点。使用方向键在显示的分组和节点间移动，按 Home 或 End 跳到首尾，按加号或减号调整六边形大小，按 Enter 打开选中的节点或展开选中的分组，按 Esc 清除选择。滚动可浏览分布图。切换到表格可查看完整且支持无障碍访问的数据。
Schema error|配置结构错误
You need to be authenticated via Netdata Cloud Single-Sign-On (SSO) to access this feature. Sign-in on this dashboard, or access your Netdata via https://app.netdata.cloud.|使用此功能需要通过 Netdata Cloud 单点登录（SSO）认证。请在此仪表盘登录，或通过 https://app.netdata.cloud 访问 Netdata。
notification timer|通知倒计时
Critical when|严重条件
The $this value comes from the DB Lookup or the Calculation above|$this 的值来自上面的数据库查询或计算结果
Expand all groups|展开全部分组
Collapse all groups|收起全部分组
zoomReset|重置缩放
zoomIn|放大
zoomOut|缩小
Visualizing data in space from|正在显示空间中的节点数据，来源：
Local dashboard is limited to|本机仪表盘最多显示
nodes.|个节点。
Review your Space active Nodes|查看空间中的活动节点
Parent|父节点
Netdata version|Netdata 版本
Nodes on the map|分布图中的节点
No exceptional events detected in available samples. Missing data is not treated as zero; this is not a health verdict.|可用样本中未检测到异常事件。缺失数据不会按零处理，此结果不代表健康状态结论。
Netdata Agent Console|Netdata 采集代理控制台
Run correlations|运行相关性分析
Edit alert configuration|编辑告警配置
Troubleshoot alert with AI|使用 AI 排查告警
Close modal|关闭对话框
Duplicate clause|复制条件
Select the nodes that belong to the currently selected rooms|选择属于当前所选房间的节点
Test|测试
You don't have permissions to create rules for other users|你没有为其他用户创建规则的权限
Update to a paid plan in order to be able to create silencing rules|升级到付费套餐后可创建静默规则
Change to random color|更换为随机颜色
Create group|创建分组
Show alert instances|显示告警实例
Pin to tabs|固定到标签栏
Map settings|地图设置
Skip, doesn't apply to me|跳过，不适用于我
Rotate left|向左旋转
Rotate right|向右旋转
Forward|前进
No metric data is available for this node in the selected time range.|所选时段内此节点没有可用的指标数据。
The metric query failed.|指标查询失败。
The metric query returned an incomplete result, so no value is shown.|指标查询返回的结果不完整，因此未显示数值。
This metric request is too large for the current connection.|此指标请求的数据量超过当前连接可处理的范围。
This metric is unavailable.|此指标不可用。
Showing the last available value.|正在显示最近一次可用数值。
Showing the last available value because the latest refresh did not complete.|最近一次刷新未完成，正在显示上次可用数值。
Reset hexagon size|重置六边形大小
Increase hexagon size|增大六边形
Decrease hexagon size|缩小六边形
Rename|重命名
View alerts|查看告警
See details of the specific instance|查看此实例的详情
scheduled to be silenced|已安排静默
Go to configuration form|前往配置表单
Copy report URL|复制报告链接
Schedule report|安排定时报告
Add to conversation with AI|添加到 AI 对话
Share report|分享报告
Open report in new tab|在新标签页中打开报告
See report details|查看报告详情
Download report|下载报告
Close report|关闭报告
Filter reports by node|按节点筛选报告
Remove subject|移除主题
The context captured from your screen and sent with your message. Click to inspect.|从当前页面获取并随消息发送的上下文，点击可查看。
Rule affects all users|规则对所有用户生效
Limit the silencing effect to your account only|仅对你自己静默告警
AI credits enable you to use Netdata's AI features, such as alert troubleshooting, alert configuration generation, automatic investigations and insights reports. Each of which consume 1 credit per invocation.|AI 积分可用于 Netdata 的告警排查、告警配置生成、自动调查及分析报告等功能，每次调用消耗 1 个积分。
Night lights (NASA)|夜间灯光（NASA）
Daytime earth|白昼地球
Blue Marble satellite (NASA)|蓝色弹珠卫星图（NASA）
Topology relief|地形阴影
Dark monochrome|深色单色图
Country borders|国界
Hide|隐藏
Edit rule|编辑规则
Delete rule|删除规则
Edit this alert configuration|编辑此告警配置
The creator of this space|此空间的创建者
Joined space by SCIM group|通过 SCIM 用户组加入空间
Joined space by invitation|通过邀请加入空间
You have requested subscription cancellation. To undo this action you can resubscribe.|你已申请取消订阅。如需撤销，可重新订阅。
Manage your subscription on the AWS Marketplace|在 AWS Marketplace 中管理订阅
Ask Netdata AI (⌘I)|询问 Netdata AI（⌘I）
Only you see this marker and hear its sound|仅你可以看到此标记并听到提示音
Everyone in this space sees this marker|此空间的所有用户都可以看到此标记
Go to previous version|转到上一版本
Go to next version|转到下一版本
Go to latest version|转到最新版本
Compare to older version|与旧版本比较
Compare between any versions|比较任意两个版本
Preview version|预览版本
Restore to this version|恢复到此版本
Forget item|移除此记忆项
Preview memory contents|预览记忆内容
Reset background color to previous value|将背景色恢复为之前的值
Remove background color|移除背景色
You cannot delete predefined items. Only items added by user can be deleted.|无法删除预定义项目，只能删除用户自行添加的项目。
Delete selected items|删除所选项目
You can only restart specific items.|仅支持重启指定的项目。
Restart selected items|重启所选项目
Disable selected items|禁用所选项目
Enable selected items|启用所选项目
Exit node preview|退出节点预览
Schedule|计划任务
Back to overview|返回概览
Import into dashboard|导入到仪表盘
Set up duplicate|创建副本
Edit dashboard|编辑仪表盘
Collapse map key|收起地图图例
Close inspector|关闭检查器
Previous match|上一个匹配项
Next match|下一个匹配项
Close Locate|关闭定位
Undo changes|撤销更改
Minimum value in the time period|所选时段内的最小值
Average (mean) value in the time period|所选时段内的平均值
Maximum value in the time period|所选时段内的最大值
Middle value when sorted (50th percentile)|数据排序后的中间值，即第 50 百分位数
Standard deviation - measures data spread around the mean|标准差，用于衡量数据在平均值附近的离散程度
95th percentile - 95% of values are below this|第 95 百分位数，即 95% 的数值低于此值
Difference between maximum and minimum values|最大值与最小值之差
Edit comparison period|编辑比较时段
Sounds from markers shared with all users|所有用户共享标记的提示音
Edit space name|编辑空间名称
You’re not a member of this room|你不是此房间的成员
Upgrade your plan to use this feature.|升级套餐后可使用此功能。
Test your configuration|测试配置
Add item|添加项目
Switch between list and tabs layout|切换列表或标签布局
Copy current item into a new one|将当前项目复制为新项目
Move down|下移
Move up|上移
Remove item|移除项目
Add a new configuration item|添加新的配置项
Copy this item and create a new one|复制此项目以创建新项目
Restart|重启
Add a new data collection job or edit existing jobs for this collector|为此采集器添加数据采集任务，或编辑现有任务
Sort by name ascending|按名称升序排列
Sort by name descending|按名称降序排列
Sort by value ascending|按数值升序排列
Sort by value descending|按数值降序排列
has been claimed and has successfully connected to Netdata at least once|已绑定，且至少成功连接过一次 Netdata
has been claimed but never connected to the Netdata.|已绑定，但从未连接到 Netdata。
Loading weights. This might take some time.|正在加载权重，可能需要一些时间。
Refresh weights|刷新权重
Remove from your favourites|从收藏中移除
Metrics for Wireless Devices|无线设备指标
Metrics for Wireless Access Points|无线接入点指标
Metrics for Wireguard VPNs|WireGuard VPN 指标
Metrics for Libreswan IPSEC VPNs|Libreswan IPsec VPN 指标
Metrics for OpenVPN|OpenVPN 指标
Metrics for Tor|Tor 指标
Performance of firewall related technologies|防火墙相关功能的性能
Linux netfilter connection tracker metrics|Linux Netfilter 连接跟踪指标
Linux netlink metrics|Linux Netlink 指标
Linux nfacct metrics|Linux nfacct 指标
Linux SYNPROXY metrics|Linux SYNPROXY 指标
Linux IP Virtual Server (IPVS) metrics|Linux IP 虚拟服务器（IPVS）指标
BSD ipfw related metrics|BSD ipfw 相关指标
Metrics for Optical Transceiver Modules|光收发模块指标
Metrics for infiniband network interfaces|InfiniBand 网络接口指标
The I/O throughput of your disks.|磁盘 I/O 吞吐量。
The I/O operations your disks perform.|磁盘 I/O 操作次数。
The latency of your disks.|磁盘延迟。
The utilization of your disks.|磁盘利用率。
The performance of bcache on your disks.|磁盘 bcache 缓存性能。
Performance of your NVMe Disks|NVMe 磁盘性能
Usage of the mounted file systems, per mount point.|按挂载点查看已挂载文件系统的使用情况。
Disk space usage per mount point.|各挂载点的磁盘空间使用情况。
Disk inodes usage per mount point.|各挂载点的磁盘 inode 使用情况。
Status and performance of your MD devices.|MD 软件磁盘阵列的状态与性能。
Performance of your BTRFS disks|Btrfs 磁盘性能
Performance of your ZFS|ZFS 性能
Performance of your NFS Server|NFS 服务器性能
Performance of your NFS Mounts|NFS 挂载性能
Samba related metrics.|Samba 相关指标。
Metric for Ceph|Ceph 指标
S.M.A.R.T. values from your disks.|磁盘的 S.M.A.R.T. 数值。
Metrics retrieved by parsing smartd log.|从 smartd 日志中解析得到的指标。
Hard drive temperatures via hddtemp.|通过 hddtemp 获取的硬盘温度。
Mount and unmount calls status|挂载与卸载调用状态
Notification methods whose destination will be a user-specific attribute, e.g. user's e-mail.|发送至用户专属地址的通知方式，例如用户的电子邮箱。
Notification methods that the destination will be a target that usually isn't specific to a single user, e.g. Slack channel.|发送至共享目标的通知方式，例如 Slack 频道。
Zoom reset|重置缩放
Working...|正在处理…
AI assistants can make mistakes. Always verify their claims.|AI 助手可能出错，请核实其回答。
Type a message...|输入消息…
Fit to window|适应窗口
Actual size|实际大小
Fit|适应
Nodes|节点
Node|节点
Metrics|指标
Metric|指标
Live|实时
Logs|日志
Dashboard|仪表盘
Dashboards|仪表盘
Alerts|告警
Alert|告警
Events|事件
Anomalies|异常
AI Insights|AI 洞察
Insights|洞察
System|系统
Overview|概览
Summary|摘要
Settings|设置
Configurations|配置
Configuration|配置
Integrations|集成
Collectors|采集器
Collector|采集器
Functions|功能
Processes|进程
Containers|容器
Virtual Machines|虚拟机
Applications|应用
Users|用户
User Groups|用户组
Services|服务
Systemd Services|Systemd 服务
Systemd Units|Systemd 单元
Network|网络
Networking|网络
Storage|存储
Disks|磁盘
Disk|磁盘
Memory|内存
Compute|计算资源
CPU|CPU
CPUs|CPU 核心
Load|负载
Pressure Stall Information (PSI)|资源阻塞压力（PSI）
Pressure Stall Information|资源阻塞压力
Some Pressure|部分任务阻塞
Full Pressure|全部任务阻塞
Utilization|使用率
Utilisation|使用率
Bandwidth|带宽
Packets|数据包
Connections|连接
Requests|请求
Responses|响应
Errors|错误
Latency|延迟
Uptime|运行时长
Health|健康状态
Status|状态
State|状态
Name|名称
Type|类型
Description|说明
Details|详情
Value|数值
Values|数值
Dimension|维度
Dimensions|维度
dimension|维度
dimensions|维度
Instance|实例
Instances|实例
Label|标签
Labels|标签
labels|标签
Context|指标上下文
Contexts|指标上下文
Chart|图表
Charts|图表
Filters|筛选
Filter|筛选
Group by|分组依据
Group By|分组依据
Group|组
Groups|组
Aggregation|聚合
the|取
each as|各组取
AVG()|平均值 AVG()
SUM()|合计 SUM()
MIN()|最小值 MIN()
MAX()|最大值 MAX()
COUNT()|计数 COUNT()
Average|平均值
Minimum|最小值
Maximum|最大值
Sum|合计
Count|计数
Latest:|最新值：
Latest|最新值
Reset|重置
Reset all|全部重置
Clear|清除
Clear all|全部清除
Apply|应用
Cancel|取消
Close|关闭
Save|保存
Save changes|保存更改
Edit|编辑
Delete|删除
Remove|移除
Add|添加
Create|创建
Update|更新
Refresh|刷新
Reload|重新加载
Retry|重试
Back|返回
Next|下一步
Previous|上一步
Continue|继续
Done|完成
Confirm|确认
OK|确定
Yes|是
No|否
All|全部
None|无
Default|默认
Custom|自定义
Automatic|自动
Enabled|已启用
Disabled|已停用
Enable|启用
Disable|停用
Expand|展开
Collapse|收起
Expand - Compare Periods|展开并比较时段
Compare Periods|比较时段
Compare|比较
Comparison|对比
Playing|正在实时更新
Paused|已暂停
Play|播放
Pause|暂停
Stop|停止
Start|开始
Search|搜索
Search...|搜索…
Search nodes|搜索节点
Search metrics|搜索指标
Search Metrics|搜索指标
Search contexts|搜索指标上下文
Search dimensions|搜索维度
Search labels|搜索标签
Search alerts|搜索告警
Search logs|搜索日志
Search events|搜索事件
Search anything|搜索内容
Search for metrics|搜索指标
No results|没有结果
No results found|未找到结果
No results were found|未找到结果
No matches|没有匹配项
No data|暂无数据
No Data|暂无数据
No data available|暂无可用数据
No data to display|暂无可显示的数据
No chart data|暂无图表数据
No charts to display|暂无可显示的图表
No nodes found|未找到节点
No nodes|暂无节点
No alerts|暂无告警
No alerts found.|未找到告警。
No events|暂无事件
No transitions found|没有状态变更记录
No errors|没有错误
No description|暂无说明
No additional details|暂无更多详情
No connection|未连接
No internet connection|网络连接不可用
Loading|正在加载
Loading...|正在加载…
Loading data...|正在加载数据…
Loading chart...|正在加载图表…
Loading nodes...|正在加载节点…
Loading the metric value.|正在加载指标值。
Loading rooms...|正在加载房间…
Error|错误
Warning|警告
Info|信息
Reports|报告
Component|组件
Class|分类
Trigger value|触发数值
Alert explanation|告警解释
There was an error while fetching alert explanation|获取告警解释时出错
The current firmware inventory cannot be compared with a durable memory baseline|当前固件硬件清单无法与持久化内存基线比较
A node may be claimed through multiple instances on the cloud (streaming through parent) and Netdata aggregates the alerts from all the instances and displays a single Active alert based on the highest severity, latest change.|同一节点可能通过多个实例绑定到云端（例如经由父节点转发数据）。Netdata 会汇总各实例的告警，并按最高严重级别和最近变更显示一条活动告警。
Open Alert page|打开告警页面
Critical|严重
WARNING|警告
CRITICAL|严重
CLEAR|已恢复
UNDEFINED|未定义
UNINITIALIZED|尚未初始化
REMOVED|已移除
Normal|正常
Healthy|健康
Unhealthy|不健康
Active|活动
Inactive|非活动
Running|运行中
Stopped|已停止
Connected|已连接
Disconnected|已断开
Reachable|可达
Unreachable|不可达
Online|在线
Offline|离线
Stale|数据过期
Unseen|尚未连接
Collected|正在采集
Collecting|正在采集
Archived|已归档
Sign in|登录
Sign In|登录
Sign out|退出登录
Sign Out|退出登录
Sign up|注册
Get started|开始使用
Connect|连接
Connect to Cloud|连接云端
Connect to Netdata Cloud|连接 Netdata Cloud
Skip and use the dashboard anonymously.|跳过，直接使用本机仪表盘。
Sign in to use this function|登录后可使用此功能
Sign in to Netdata to use Configurations|登录 Netdata 后可使用配置功能
Sign in to Netdata to view traces.|登录 Netdata 后可查看调用链。
Settings can be saved when the agent is part of a Netdata Cloud space.|将 Agent 加入 Netdata Cloud 空间后才能保存这些设置。
Settings can be saved when the agent is part of a Netdata cloud space.|将 Agent 加入 Netdata Cloud 空间后才能保存这些设置。
Space|空间
Spaces|空间
Room|房间
Rooms|房间
Select room|选择房间
Select space|选择空间
Claim|绑定
Claiming...|正在绑定…
Agent|采集代理
Agents|采集代理
Help|帮助
Help & Resources|帮助与资源
Ask Nedi, read documentation, engage with the community and let us know about any feature requests or bugs|向 Nedi 提问、阅读文档、参与社区，或向我们提交功能建议和缺陷反馈。
Ask Nedi|询问 Nedi
Report a Bug|提交缺陷
Community|社区
Demo Space|演示空间
Operational Status|服务运行状态
View service status, incidents, and maintenance updates for Netdata Cloud|查看 Netdata Cloud 的服务状态、故障事件和维护更新。
Create New Space|创建新空间
Spaces isolate infrastructure, providing fully independent administration and user management|空间用于隔离基础设施，每个空间拥有独立的管理和用户权限。
Geo Map|地理分布图
Visualize node locations on an interactive map|在交互式地图上查看节点位置。
Node Metrics|节点指标
View real-time and historical metrics for system resources, containers, VMs, and applications on this node|查看此节点上系统资源、容器、虚拟机和应用的实时与历史指标。
Node Live View|节点实时视图
Live, on-demand insights including processes, network connections, database queries, topology maps & more|按需实时查看进程、网络连接、数据库查询、拓扑图等信息。
Node Logs|节点日志
Browse and search through system logs, application logs, and service logs from this node|浏览和搜索此节点的系统、应用及服务日志。
Node Traces|节点调用链
Search and inspect distributed traces emitted by the services running on this node|搜索并查看此节点上各服务产生的分布式调用链。
Node Alerts|节点告警
View active alerts, alert history, and alert configurations for this specific node|查看此节点的活动告警、告警历史和告警配置。
Node Anomaly Advisor|节点异常分析助手
AI-powered anomaly detection showing unusual patterns and deviations in node metrics|通过 AI 异常检测，查看节点指标中的异常模式和偏差。
Node Events|节点事件
Track configuration changes, deployments, restarts, and other significant events on this node|跟踪此节点上的配置变更、部署、重启及其他重要事件。
Group nodes|节点分组
Group nodes by attributes like status, OS, or custom host labels. You can select multiple for multi-level grouping.|按状态、操作系统或自定义主机标签等属性对节点分组，可同时选择多个属性进行多级分组。
Report mode|报告模式
You are now on report mode. You can trigger background investigation reports from the chat.|当前处于报告模式，你可以在聊天中发起后台调查并生成报告。
Is this host healthy — CPU, memory, load, processes, kernel, services?|此主机是否健康？检查 CPU、内存、负载、进程、内核和服务。
Containers & VMs|容器与虚拟机
How are my containers and VMs doing? Which one is noisy?|容器和虚拟机运行情况如何？哪个实例占用了过多资源？
Is my cluster healthy? Pods, nodes, workloads, control plane?|集群是否健康？检查 Pod、节点、工作负载和控制平面。
Is storage healthy and fast? Disks, filesystems, arrays, capacity?|存储是否健康、响应是否足够快？检查磁盘、文件系统、阵列和容量。
Is the physical hardware healthy? Sensors, power, IPMI, UPS?|物理硬件是否健康？检查传感器、电源、IPMI 和 UPS。
GPUs|GPU 设备
Is my GPU/accelerator fleet healthy and utilized?|GPU 和加速器是否健康，资源利用情况如何？
Host Networking|主机网络
Is host networking healthy? Stack, sockets, VPN, firewall, time sync?|主机网络是否健康？检查协议栈、套接字、VPN、防火墙和时间同步。
DNS & DHCP|DNS 与 DHCP
Is the DNS service I run healthy? Are DHCP leases and pools OK?|DNS 服务是否健康？DHCP 租约和地址池是否正常？
Databases|数据库
Is my data layer healthy? Which query is hurting?|数据库是否健康？哪个查询影响了性能？
Messaging|消息服务
Are my message queues and brokers healthy? Queue depth, consumers, publishers?|消息队列和消息代理是否健康？检查队列深度、消费者和发布者。
Web Servers & Proxies|Web 服务器与代理
Is my web tier and app runtimes serving traffic well?|Web 服务和应用运行环境是否能够正常处理请求？
Synthetic Checks|模拟检查
Are my endpoints reachable and certs valid — from the outside?|从外部访问时，服务端点是否可达、证书是否有效？
Journal, Windows events, macOS logs, and log pipelines.|系统日志、Windows 事件、macOS 日志及日志处理管道。
Netdata Fleet|Netdata 集群
Is my Netdata deployment itself healthy? Streaming, cardinality, API load?|Netdata 自身的部署是否健康？检查数据流、时间序列数量和 API 负载。
Network Monitor|网络监控
Real-time overview of your network performance, traffic, and health.|实时查看网络性能、流量和健康状态。
Early Access|抢先体验
Preview and enable new features before they're released|在新功能正式发布前预览并启用。
DB lookup|数据库查询
Calculation|计算
Slice and dice the source time-series metrics in multiple ways, to get different viewing angles on them. Multiple groupings can be selected at the same time to fine tune the segmentation.|以多种方式拆分和组合源时间序列指标，从不同角度查看数据。可同时选择多个分组条件，细化数据划分。
View or filter the nodes contributing time-series metrics to this chart. This menu also provides the contribution of each node to the volume of the chart, and a break down of the anomaly rate of the queried data per node.|查看或筛选为此图表提供时间序列指标的节点。此菜单还显示各节点对图表总量的贡献，以及查询数据中各节点的异常率。
View or filter the systems contributing time-series metrics to this chart. This menu also provides the contribution of each system to the volume of the chart, and a break down of the anomaly rate of the queried data per system.|查看或筛选为此图表提供时间序列指标的系统。此菜单还显示各系统对图表总量的贡献，以及查询数据中各系统的异常率。
View or filter the instances contributing time-series metrics to this chart. This menu also provides the contribution of each instance to the volume of the chart, and a break down of the anomaly rate of the queried data per instance.|查看或筛选为此图表提供时间序列指标的实例。此菜单还显示各实例对图表总量的贡献，以及查询数据中各实例的异常率。
Metrics post aggregation|指标二次聚合
Post group by|二次分组依据
Aliases aggregation over time|时间聚合参数
The percentile or percentage of the data you want to focus for the percentile or trimmed functions selected.|为所选百分位数或截尾函数指定要关注的数据百分位或比例。
Type your message...|输入消息…
Ask anything about Netdata...|询问任何关于 Netdata 的问题…
Send|发送
Copy code|复制代码
Copy conversation|复制对话
Clear conversation|清空对话
Toggle theme|切换主题
Expand diagram|展开图示
Close diagram|关闭图示
Diagram|图示
Diagram canvas|图示画布
Close chat|关闭聊天
Restore|还原
Maximize|最大化
Pan|平移
Highlight|高亮
Select and zoom|选择并缩放
Reset zoom|重置缩放
Zoom in|放大
Zoom out|缩小
Open chat|打开聊天
AI Assistant|AI 助手
The duration since this request started|自本次请求开始以来的时长
The size of models' reasoning/thinking|模型推理或思考的内容量
The size of models' output|模型输出的内容量
The size of documents read by the models|模型已读取的文档量
The number of tools executed|已执行的工具数量
The number of LLM calls made|已发起的大模型调用次数
What's on your mind today?|今天想聊什么？
How can I help?|我可以怎么帮你？
What are you working on?|你正在处理什么？
Where should we begin?|我们从哪里开始？
Ready when you are.|准备好后就可以开始。
What's on the agenda today?|今天有什么安排？
Hi! Ready to dive in?|你好，准备开始了吗？
Good to see you.|很高兴见到你。
Got a question?|有什么问题吗？
Happy to help - what's up?|我很乐意帮忙，遇到了什么问题？
What brings you here today?|今天想解决什么问题？
Anything I can help with?|有什么我能帮忙的吗？
Stuck on something?|遇到什么困难了吗？
What can I help you find?|需要我帮你查找什么？
Got something to dig into?|有什么需要深入调查的吗？
What's the puzzle today?|今天需要解决什么难题？
I can help you configure data collection plugins, retention, and Netdata Parents|我可以帮助你配置数据采集插件、数据保留策略和 Netdata 父节点。
我可以帮助你配置数据采集插件、保留策略和 Netdata Parents|我可以帮助你配置数据采集插件、数据保留策略和 Netdata 父节点。
Ask me for ways to monitor your systems, containers, and applications|可以询问如何监控系统、容器和应用。
I can write Netdata alerts tailored to your use case|我可以根据你的使用场景编写 Netdata 告警。
Tell me your use case so I can help you get the most out of Netdata|告诉我你的使用场景，我会帮助你更好地使用 Netdata。
Paste your error logs - I'll trace them through the source code|粘贴错误日志，我会结合源代码追查原因。
Configure Alerts|配置告警
Netdata Parents|Netdata 父节点
Documentation|文档
Support|支持
Feedback|反馈
About|关于
Version|版本
Theme|主题
Dark|深色
Light|浅色
Appearance|外观
Timezone|时区
Time zone|时区
Time Zone|时区
Timeframe|时间范围
Time range|时间范围
Time Range|时间范围
Date|日期
Time|时间
Duration|持续时间
From|从
To|至
Start time|开始时间
End time|结束时间
Absolute|绝对时间
Relative|相对时间
last|最近
Last|最近
Now|现在
Today|今天
Yesterday|昨天
Last 15 minutes|最近 15 分钟
Last 30 minutes|最近 30 分钟
Last 1 hour|最近 1 小时
Last 6 hours|最近 6 小时
Last 24 hours|最近 24 小时
Last 7 days|最近 7 天
Notifications|通知
Notification|通知
Notification settings|通知设置
Alert notifications|告警通知
Alert history|告警历史
Alert History|告警历史
Alert instances|告警实例
Alert Instances|告警实例
Alert configurations|告警配置
Active Alerts|活动告警
Active alerts|活动告警
All alerts|全部告警
Alert name|告警名称
Alert Name|告警名称
Alert status|告警状态
Severity|严重级别
Recipient|接收方
Recipients|接收方
Source|来源
Role|角色
Rules|规则
Rule|规则
Threshold|阈值
Thresholds|阈值
Warning threshold|警告阈值
Critical threshold|严重阈值
Raised|触发时间
Updated|更新时间
Last updated|最近更新
Transitions|状态变更
Transition|状态变更
History|历史记录
Silenced|已静默
Silence|静默
Mute|静音
Muted|已静音
Acknowledged|已确认
Unacknowledged|未确认
Acknowledgment|确认状态
Acknowledge|确认告警
Show acknowledged alerts|显示已确认告警
Show configuration|显示配置
View configuration|查看配置
View chart|查看图表
View Chart|查看图表
View details|查看详情
View Details|查看详情
View all|查看全部
Show more|显示更多
Show less|收起
Show filters|显示筛选条件
Show Legend|显示图例
Show X-Axis|显示 X 轴
Show Y-Axis|显示 Y 轴
Show zeros|显示零值
Show gaps|显示数据缺口
Show columns|显示列
Show in grid|以网格显示
Show in rows|以行显示
Columns|列
Rows|行
Sort|排序
Sort by|排序依据
Ascending|升序
Descending|降序
Download|下载
Export|导出
Import|导入
Copy|复制
Copied|已复制
Copied!|已复制！
Share|分享
Print|打印
Fullscreen|全屏
Full screen|全屏
Exit fullscreen|退出全屏
Legend|图例
Options|选项
Advanced|高级
Advanced settings|高级设置
General|常规
General settings|常规设置
Chart settings|图表设置
Chart Settings|图表设置
Chart type|图表类型
Line|折线
Area|面积
Stacked|堆叠
Bar|柱状
Table|表格
Heatmap|热力图
Units|单位
Unit|单位
Resolution|分辨率
Data points|数据点
Data collection|数据采集
Data retention|数据保留
Retention|保留时间
Replication|数据复制
Streaming|数据流
Machine Learning|机器学习
Anomaly detection|异常检测
Anomaly Detection|异常检测
Anomaly rate|异常率
Anomaly Rate|异常率
Anomaly Advisor|异常分析助手
Metric correlations|指标相关性
Metrics Explorer|指标浏览器
An overview of your infrastructure.|基础设施运行概览。
All metrics from all nodes, containers, VMs, SNMP devices, applications, and synthetic checks, in one universal, aggregated, infrastructure-level dashboard.|在统一的基础设施仪表盘中，汇总查看所有节点、容器、虚拟机、SNMP 设备、应用和模拟检查的全部指标。
Live, on-demand insights including processes, network connections, database queries, topology maps & more.|按需实时查看进程、网络连接、数据库查询、拓扑图等信息。
Traces Explorer|调用链浏览器
Search and inspect distributed traces across your infrastructure.|搜索并查看基础设施中的分布式调用链。
Kubernetes Explorer|Kubernetes 浏览器
All Kubernetes clusters, pods, containers and workloads.|查看所有 Kubernetes 集群、Pod、容器和工作负载。
Custom Dashboards|自定义仪表盘
Create, and manage personalized dashboards tailored to your needs.|按需创建和管理个性化仪表盘。
Metrics overview|指标概览
Metrics available|可用指标
Node List|节点列表
Node Grid|节点网格
Node Status|节点状态
Node info|节点信息
Node info and settings|节点信息与设置
System info|系统信息
System information|系统信息
Operating System|操作系统
Operating system|操作系统
Hostname|主机名
Architecture|架构
Kernel|内核
CPU cores|CPU 核心数
Total memory|总内存
IP address|IP 地址
IP Address|IP 地址
Virtualization|虚拟化
Hardware|硬件
Host|主机
Hosts|主机
Environment|环境
Live view|实时视图
Live View|实时视图
Traces|调用链
Topology|拓扑
Network Connections|网络连接
Journal|系统日志
Systemd Journal Logs|Systemd 日志
Total Disk Reads|磁盘总读取速率
Total Disk Writes|磁盘总写入速率
Avg CPU per Node|节点平均 CPU 使用率
Avg Used RAM per Node|节点平均内存使用率
Total Network Inbound|网络总接收速率
Total Network Outbound|网络总发送速率
Avg CPU Pressure (10 sec, full)|CPU 平均完全阻塞占比（10 秒）
Avg Memory Pressure (10 sec, some)|内存平均部分阻塞占比（10 秒）
Average Disk I/O Pressure (10 sec, some)|磁盘 I/O 平均部分阻塞占比（10 秒）
Avg System Load (1 min)|系统平均负载（1 分钟）
Top Nodes by CPU|CPU 使用率最高的节点
Top Nodes by Used RAM|已用内存最多的节点
Top Apps by CPU|CPU 使用率最高的应用
Top Apps by Memory|内存占用最高的应用
Top Containers by CPU Usage|CPU 使用率最高的容器
Top Containers by Memory Usage|内存占用最高的容器
Total CPU utilization|CPU 总使用率
System RAM|系统内存
System Load Average|系统平均负载
System Uptime|系统运行时长
CPU Context Switches|CPU 上下文切换
CPU Interrupts|CPU 中断
CPU Idle Jitter|CPU 空闲抖动
CPU some pressure|CPU 部分任务阻塞占比
CPU full pressure|CPU 全部任务阻塞占比
CPU some pressure stall time|CPU 部分任务阻塞累计时间
CPU full pressure stall time|CPU 全部任务阻塞累计时间
Memory some pressure|内存部分任务阻塞占比
Memory full pressure|内存全部任务阻塞占比
Memory some pressure stall time|内存部分任务阻塞累计时间
Memory full pressure stall time|内存全部任务阻塞累计时间
I/O some pressure|I/O 部分任务阻塞占比
I/O full pressure|I/O 全部任务阻塞占比
I/O some pressure stall time|I/O 部分任务阻塞累计时间
I/O full pressure stall time|I/O 全部任务阻塞累计时间
Available RAM for applications|应用可用内存
Committed (Allocated) Memory|已承诺分配的内存
Memory Page Faults|内存缺页
Memory Paged from/to disk|内存与磁盘间的页交换
Memory Reclaiming|内存回收
Memory Usage|内存用量
Memory Utilization|内存使用率
Memory Used by Kernel|内核使用的内存
Out of Memory Kills|内存不足触发的进程终止
Out of Memory Protection|内存不足保护
Writeback Memory|等待回写的内存
Reclaimable Kernel Memory|可回收的内核内存
Corrupted Memory, detected by ECC|ECC 检测到的损坏内存
Confirmed Memory Loss|已确认的内存容量损失
Memory Baseline Comparison Status|内存基线比较状态
Memory Inventory Availability|内存硬件清单可用状态
Firmware-Reported Installed Memory|固件报告的已安装内存
Firmware-Reported Memory Slots|固件报告的内存插槽
Disk I/O|磁盘 I/O
Disk I/O Bandwidth|磁盘 I/O 吞吐量
Disk Space Usage|磁盘空间用量
Disk Files (inodes) Usage|磁盘 inode 用量
Disk Utilization Time|磁盘忙碌时间占比
Disk Busy Time|磁盘忙碌时间
Disk Backlog|磁盘请求积压
Disk Current I/O Operations|磁盘当前 I/O 操作数
Disk Completed I/O Operations|磁盘已完成 I/O 操作速率
Disk Completed Extended I/O Operations|磁盘已完成扩展 I/O 操作速率
Disk Merged Operations|磁盘合并操作速率
Disk Merged Discard Operations|磁盘合并丢弃操作速率
Disk Total I/O Time|磁盘 I/O 总耗时
Disk Total I/O Time for Extended Operations|磁盘扩展操作 I/O 总耗时
Average Completed I/O Operation Time|已完成 I/O 操作平均耗时
Average Completed Extended I/O Operation Time|已完成扩展 I/O 操作平均耗时
Average Completed I/O Operation Bandwidth|已完成 I/O 操作平均吞吐量
Average Service Time|平均服务时间
Amount of Discarded Data|丢弃的数据量
Average Amount of Discarded Data|平均丢弃数据量
I/O Bandwidth (all disks)|全部磁盘 I/O 吞吐量
Serviced I/O Operations (all disks)|全部磁盘已处理 I/O 操作速率
Physical Network Interfaces Aggregated Bandwidth|物理网卡汇总带宽
Network Interface Events|网卡事件
Interface Drops|网卡丢包
Interface Errors|网卡错误
Interface FIFO Buffer Errors|网卡 FIFO 缓冲区错误
Interface Duplex State|网卡双工状态
Interface Physical Link State|网卡物理链路状态
Interface Operational State|网卡运行状态
Interface Speed|网卡速率
Interface MTU|网卡最大传输单元（MTU）
TCP Connections|TCP 连接数
TCP Sockets|TCP 套接字
TCP Sockets Memory|TCP 套接字内存
TCP SYN Cookies|TCP SYN Cookie
TCP Connection Aborts|TCP 连接中止
TCP Accept Queue Issues|TCP 接收队列问题
TCP SYN Queue Issues|TCP SYN 队列问题
TCP Memory Pressures|TCP 内存压力
TCP Out-Of-Order Queue|TCP 乱序队列
TCP Reordered Packets by Detection Method|按检测方式统计的 TCP 乱序数据包
HTTP Check Status|HTTP 检查状态
HTTP Current State Duration|HTTP 当前状态持续时间
HTTP Response Body Length|HTTP 响应正文长度
HTTP Response Time|HTTP 响应时间
Total Requests|请求总速率
Requests By HTTP Method|按 HTTP 方法统计请求
Requests By HTTP Version|按 HTTP 版本统计请求
Requests By IP Protocol|按 IP 协议统计请求
Requests By Port|按端口统计请求
Requests By Type|按类型统计请求
Requests By Vhost|按虚拟主机统计请求
Responses By Status Code Class|按状态码类别统计响应
Successful Responses By Status Code|按状态码统计成功响应
Redirects Responses By Status Code|按状态码统计重定向响应
Client Errors Responses By Status Code|按状态码统计客户端错误响应
Request Processing Time|请求处理时间
Upstream Response Time|上游响应时间
Excluded Requests|已排除的请求
Active Client Connections Including Waiting Connections|活动客户端连接数（含等待连接）
Active Connections Per Status|按状态统计的活动连接
Accepted And Handled Connections|已接受和已处理的连接
Client Requests|客户端请求
System Active Processes|系统活动进程数
System Processes|系统进程数
System Processes State|系统进程状态
Started Processes|新启动的进程
File Descriptors|文件描述符
File Descriptors Utilization|文件描述符使用率
CPU Usage (100%% = 1 core)|CPU 使用率（100% 表示 1 个核心）
CPU Usage within the limits|CPU 配额内的使用率
CPU Throttled Runnable Periods|CPU 被限流的可运行周期
CPU Throttled Time Duration|CPU 被限流的时长
Used Memory|已用内存
Used RAM within the limits|内存配额内的使用量
Docker container state|Docker 容器状态
Docker container health status|Docker 容器健康状态
Total number of Docker containers in various states|按状态统计的 Docker 容器数量
Total number of Docker containers in various health states|按健康状态统计的 Docker 容器数量
Total number of Docker images in various states|按状态统计的 Docker 镜像数量
Total size of all Docker images|Docker 镜像总大小
Service Unit State|服务单元状态
Post-Update Reboot Status|更新后需重启的状态
System Clock Status|系统时钟状态
System Clock Synchronization State|系统时钟同步状态
Available Entropy|可用熵
Sensor Temperature|传感器温度
Sensor Temperature Alarm Status|传感器温度告警状态
Temperature Sensors Distribution|温度传感器分布
Anomaly detection dimensions|异常检测维度数
Anomaly detection events|异常检测事件
Percentage of anomalous dimensions|异常维度占比
Percentage of anomalous dimensions by context|按指标上下文统计异常维度占比
Machine learning status|机器学习状态
Training status of dimensions|维度训练状态
Training queue size|训练队列大小
Training queue operations|训练队列操作
Training results|训练结果
Training time stats|训练耗时统计
Data Collection Duration|数据采集耗时
Data Collection Status|数据采集状态
Audit Backlog|审计队列积压
Audit Backlog Utilization|审计队列使用率
Audit Enabled State|审计启用状态
Audit Failure Mode|审计失败处理模式
Audit Lost Events|审计丢失事件数
success|成功
failed|失败
failure|失败
timeout|超时
no_connection|无法连接
bad_content|正文不匹配
bad_status|状态码异常
redirect|重定向
user|用户态
system|内核态
idle|空闲
iowait|I/O 等待
steal|虚拟机被占用
softirq|软中断
irq|硬中断
nice|低优先级用户态
guest|客户机
guest_nice|低优先级客户机
free|空闲
used|已用
cached|缓存
buffers|缓冲区
available|可用
shared|共享
active|活动
inactive|非活动
dirty|脏页
writeback|回写
slab|内核对象缓存
reclaimable|可回收
unreclaimable|不可回收
reads|读取
writes|写入
read|读取
write|写入
received|接收
sent|发送
inbound|入站
outbound|出站
in|输入
out|输出
running|运行中
blocked|阻塞
sleeping|睡眠
zombie|僵尸
stopped|已停止
threads|线程
processes|进程
seconds|秒
milliseconds|毫秒
microseconds|微秒
percent|百分比
requests/s|请求/秒
connections/s|连接/秒
operations/s|操作/秒
packets/s|包/秒
events/s|事件/秒
files|文件
sockets|套接字
load1|1 分钟负载
load5|5 分钟负载
load15|15 分钟负载
some 10|部分阻塞（10 秒）
some 60|部分阻塞（60 秒）
some 300|部分阻塞（300 秒）
full 10|全部阻塞（10 秒）
full 60|全部阻塞（60 秒）
full 300|全部阻塞（300 秒）
time|时间
local|本机
`);
  Object.assign(exact, parse(`
Servers|服务器
Server|服务器
Add nodes|添加节点
Total Servers|服务器总数
Default view|默认视图
Group by:|分组依据：
Sort:|排序：
Name A–Z|名称 A–Z
Name Z–A|名称 Z–A
Fleet map|节点分布图
Address|地址
Disk Read|磁盘读取
Disk Write|磁盘写入
Net In|网络接收
Net Out|网络发送
bytes per second|字节/秒
bits per second|比特/秒
Actions|操作
Nodes Explorer|节点浏览器
All your infrastructure nodes.|查看所有基础设施节点。
Misconfigured|配置异常
New Alert|新建告警
Latest value|最新数值
Updated at|更新时间
Triggered value|触发时数值
Triggered at|触发时间
Silencing|静默状态
Memory inventory comparison unavailable|无法比较内存硬件清单
Alerts & Notifications|告警与通知
View raised and configured alerts across your infrastructure.|查看基础设施中的活动告警和已配置规则。
Live nodes|在线节点
Live nodes |在线节点
Activity Feed|活动记录
Alert transitions and notifications, node connections and disconnections, administrative events.|记录告警状态变更与通知、节点连接与断开，以及管理操作事件。
Not connected|尚未连接
Connect this agent to use this function|连接此采集代理后可使用此功能
Functions expose sensitive information about your systems and applications. To protect your privacy, Netdata exposes this information only to logged-in users and claimed agents. When viewing Functions directly on a Netdata Agent UI, this information is sent directly from the Netdata Agent to your web browser, without exposing it to any third parties.|此功能会展示系统和应用中的敏感信息，因此 Netdata 仅向已登录用户和已绑定的采集代理开放。在 Agent 的本机界面查看时，数据由采集代理直接发送至浏览器，不经过第三方。
Function|功能
Logs Explorer|日志浏览器
Search, filter, and analyze system and application logs.|搜索、筛选并分析系统和应用日志。
Apply global filters|应用全局筛选条件
Exit|退出
Based on your setup we recommend these templates to start with|根据当前监控配置，推荐从以下模板开始
Executive Overview|运行概览
Fleet health at a glance: online %, healthy devices, alerts, and trends|集中查看节点在线率、健康设备、告警和变化趋势
NOC Operations|运维监控中心
Live fleet monitor: status heatmap, connectivity streams, and the live alert feed|实时监控节点：状态热力图、连接变化和活动告警
Support Overview|故障排查概览
Device triage: find broken devices fast, by client, status, and recent alerts|按客户端、状态和最近告警，快速定位故障设备
Streaming Uptime|数据流连接时长
Connection state, uptime and stability for every streaming child|查看每个下游采集节点的连接状态、在线时长和稳定性
or you can start building your own dashboard from scratch!|也可以从空白页面开始创建自己的仪表盘。
Add chart|添加图表
Browse templates|浏览模板
Custom Dashboard|自定义仪表盘
Edit a personalized dashboard tailored to your needs.|按需要编辑个性化仪表盘。
Nodes Anomalies Over Time|节点异常随时间变化
Metrics went anomalous concurrently across the infrastructure|查看基础设施中同时发生异常的指标
Rate %|比例 %
Retry analysis|重新分析
Findings|分析结果
Affected Nodes|受影响节点
Affected Areas|受影响范围
Select|选择
selected|已选择
Area / Context|范围 / 指标上下文
Trend|趋势
Peak|峰值
Avg|平均值
Peak %|峰值 %
Avg %|平均值 %
Anomalous|存在异常
No matching areas.|没有匹配的范围。
Ask AI|询问 AI
Anomaly analysis|异常分析
Find changes|查找变化
Volume|数据量
Machine-learning powered root cause analysis, blast radius detection, and cascading effect tracing across nodes.|通过机器学习分析根因、识别影响范围，并追踪节点间的连锁影响。
Metrics aggregation|指标聚合方式
Manage Space|管理空间
Jobs|采集任务
SecretStores|凭据存储
ServiceDiscovery|服务发现
Vnodes|虚拟节点
Unique|唯一值数
Vol %|数据量占比 %
Anomaly%|异常占比 %
Min|最小值
Max|最大值
Range|范围
Percentage|百分比
system (instance)|系统（实例）
Deselecting everything will use GROUP BY DIMENSION by default|全部取消选择后，默认按维度分组
When aggregating multiple source time-series metrics to one visible dimension on the chart, use the following aggregation function|将多个原始时间序列合并为图表中的一个维度时，使用以下聚合函数
Single-node anomaly|单节点异常
Analyzing…|正在分析…
Analyzing...|正在分析…
Waiting analysis…|等待分析结果…
Time aggregation|时间聚合方式
Sort dimensions by name or value|按名称或数值排列维度
Blast Radius Analysis|影响范围分析
Netdata Insights makes your infrastructure data understandable.|Netdata 洞察帮助你理解基础设施数据。
It uses Al to run automated investigations and generate insights. This means you can make better decisions, faster, without sifting through dashboards or writing complicated searches. Insights gives you the straightforward answers you need.|此功能使用 AI 自动调查并生成分析结果，帮助你更快作出判断，减少逐一浏览仪表盘和编写复杂查询的工作，并直接给出分析结论。
Here are some of the reports it can create:|可以生成以下分析报告：
Anomaly Analysis|异常分析
Explore in demo space|在演示空间中体验
Capacity Planning|容量规划
Infrastructure Summary|基础设施摘要
Performance Optimization|性能优化
Every report combines easy-to-read explanations, charts, and practical advice, all made whenever you need them. You can find Insights inside Netdata Cloud. It works perfectly with any Netdata agent you're already using, so there's nothing extra to set up. It's currently available for Business and Trial users.|每份报告都包含易读的说明、图表和操作建议，可按需生成。此功能位于 Netdata Cloud，可使用已有的 Netdata 采集代理。目前向 Business 和试用用户开放。
AI Insights and Reports|AI 洞察与报告
Automatically investigate, troubleshoot and generate deep dive infrastructure analysis reports using Netdata AI.|使用 Netdata AI 自动调查、排查问题，并生成基础设施深度分析报告。
clear|清除
results|条结果
results)|条结果）
Selected|已选择
Areas|范围
queried|已查询
of|共
Load Average|平均负载
Network In|网络接收
Network Out|网络发送
Select nodes|选择节点
Select contexts|选择指标上下文
Select dimensions|选择维度
Select labels|选择标签
Scope|范围
Time series|时间序列
Timeseries|时间序列
Distribution|分布
Statistics|统计
Availability|可用性
sorted ascending|已按升序排序
sorted descending|已按降序排序
sortable, not sorted|可排序，尚未排序
Clear filters|清除筛选条件
Reset filters|重置筛选条件
Node settings|节点设置
Room settings|房间设置
Space settings|空间设置
User settings|用户设置
Profile|个人资料
Preferences|偏好设置
`));
  Object.assign(exact, parse(`
Systems|系统
systems|系统
system|系统
node|节点
instance|实例
label|标签
chart|图表
metric|指标
event|事件
last|近
`));
  Object.assign(exact, parse(`
Computed Time Offset Between Local System and Reference Clock|本机与参考时钟的计算偏差
Current Poll Unique Clients|当前轮询中的独立客户端数
Current correction|当前时钟校正量
Dispersion accumulated back to stratum-1|累计至一级时间源的时钟离散度
Distance to the reference clock|与参考时钟的距离
Interval between the last two clock updates|最近两次时钟更新的间隔
Leap status|闰秒状态
Long-term average of the offset value|时钟偏差的长期平均值
NTP packets|NTP 数据包
Network path delay to stratum-1|到一级时间源的网络路径延迟
Offset on the last clock update|最近一次时钟更新的偏差
Peers activity|对等时间源活动
Residual frequency|残余频率偏差
Skew|时钟频率偏差估计
Time since the last measurement|距最近一次测量的时间
Command packets|命令数据包
Frequency|频率
Connection Tracker Changes|连接跟踪变更
Connection Tracker Connections|连接跟踪连接数
Connection Tracker Errors|连接跟踪错误
Connection Tracker Expectations|连接跟踪预期连接数
Connection Tracker New Connections|连接跟踪新连接
Connection Tracker Searches|连接跟踪查找
Sockets used for all address families|各地址族使用的套接字数
Calls to mount and umount syscalls|挂载与卸载系统调用次数
Errors to mount and umount file systems|文件系统挂载与卸载错误
IPC Semaphore Arrays|IPC 信号量数组
IPC Semaphores|IPC 信号量
IPC Shared Memory Number of Segments|IPC 共享内存段数
IPC Shared Memory Used Bytes|IPC 共享内存用量
IPC used array positions.|IPC 已使用的数组位置
Direct Memory Mappings|直接映射内存
Details of Transparent HugePages Usage|透明大页使用详情
Transparent HugePages Memory|透明大页内存
Transparent Huge Page Fault Allocations|缺页时分配的透明大页
Transparent Huge Page File Allocations|文件透明大页分配
Transparent Huge Page Splits|透明大页拆分
Transparent Huge Pages Collapsed by khugepaged|khugepaged 合并的透明大页
Transparent Huge Pages Compaction|透明大页内存整理
Transparent Huge Pages Swap Out|透明大页换出
Transparent Huge Zero Page Allocations|透明零大页分配
Memory Ballooning Operations|内存气球操作
Memory fragmentation index for each order|各阶内存碎片指数
Files out of page cache|页缓存之外的文件
Hit ratio|命中率
Number of accessed files|已访问文件数
Number of dirty pages|脏页数
Number of processes|进程数
Total number of monitored PIDs|受监控的进程 ID 总数
Logind Sessions|登录会话数
Logind Sessions By State|按状态统计登录会话
Logind Sessions By Type|按类型统计登录会话
Logind Users By State|按状态统计登录用户
System interrupts|系统硬中断
System softirqs|系统软中断
System softnet_stat|系统网络软中断统计
cgroups Discovery Scans|cgroup 发现扫描
Netdata CPU usage|Netdata CPU 使用率
Netdata Memory|Netdata 内存用量
Netdata Memory Buffers|Netdata 内存缓冲区
Netdata Network Traffic|Netdata 网络流量
Netdata Registry Entries|Netdata 注册表条目
Netdata Registry Memory|Netdata 注册表内存
Netdata Registry Sessions|Netdata 注册表会话
Netdata Time-Series Collected Samples|Netdata 时序采集样本数
Netdata Time-Series DB Queries|Netdata 时序数据库查询
Netdata Time-Series DB Samples Read|Netdata 时序数据库读取样本数
Netdata Time-Series Points Generated|Netdata 生成的时序数据点
Netdata Web API Clients|Netdata Web API 客户端数
Netdata Web API Requests Received|Netdata Web API 接收请求
Netdata Web API Response Time|Netdata Web API 响应时间
Netdata uptime|Netdata 运行时长
dbengine space and time retention|数据库引擎空间与保留时间
Apps Plugin CPU|应用采集插件 CPU 使用率
Apps Plugin Files|应用采集插件文件数
Apps Plugin APPS_LOOKUP Server Request Duration|应用查找服务端请求耗时
Apps Plugin APPS_LOOKUP Server Requests|应用查找服务端请求数
Apps Plugin CGROUPS_LOOKUP Client Cache|应用插件 cgroup 查找客户端缓存
Apps Plugin CGROUPS_LOOKUP Client Peer|应用插件 cgroup 查找客户端对端
Apps Plugin CGROUPS_LOOKUP Client Queue Depth|应用插件 cgroup 查找客户端队列深度
Apps Plugin CGROUPS_LOOKUP Client Request Duration|应用插件 cgroup 查找客户端请求耗时
Apps Plugin CGROUPS_LOOKUP Client Requests|应用插件 cgroup 查找客户端请求数
CGROUPS_LOOKUP Server Request Duration Histogram|cgroup 查找服务端请求耗时分布
CGROUPS_LOOKUP Server Requests|cgroup 查找服务端请求数
Network Viewer APPS_LOOKUP Client Cache|网络视图应用查找客户端缓存
Network Viewer APPS_LOOKUP Client Peer|网络视图应用查找客户端对端
Network Viewer APPS_LOOKUP Client Requests|网络视图应用查找客户端请求数
Network Viewer APPS_LOOKUP Function Handler Overhead|网络视图应用查找函数处理开销
Network Viewer APPS_LOOKUP Intake Depth|网络视图应用查找接收队列深度
Network Viewer APPS_LOOKUP Worker Request Duration|网络视图应用查找工作线程请求耗时
Elements inserted into PID table|插入进程 ID 表的条目数
Elements removed from PID table|从进程 ID 表移除的条目数
Controllers inside global table|全局表内的控制器数量
How threads are loading hash/array tables.|线程加载哈希表与数组表的方式
Memory allocated for hash tables.|分配给哈希表的内存
Number of hash tables loaded.|已加载的哈希表数量
Load info.|加载信息
Threads running.|运行中的线程
Time remaining for thread.|线程剩余运行时间
Bytes read by the netdata statsd server|Netdata statsd 服务读取的字节数
Events processed by the netdata statsd server|Netdata statsd 服务处理的事件数
Metrics in the netdata statsd database|Netdata statsd 数据库内的指标数
Network packets processed by the netdata statsd server|Netdata statsd 服务处理的数据包数
Private metric charts created by the netdata statsd server|Netdata statsd 服务创建的私有指标图表数
Read operations made by the netdata statsd server|Netdata statsd 服务读取操作数
Useful metrics in the netdata statsd database|Netdata statsd 数据库内的有效指标数
statsd server TCP connected sockets|statsd 服务已连接的 TCP 套接字数
statsd server TCP connects and disconnects|statsd 服务 TCP 连接与断开次数
ML running|机器学习运行状态
1 when build sequence monotonicity is currently violated|构建序列当前违反单调性时为 1
Age of the Job Manager oldest_claim_wait_age|任务管理器最早绑定等待时长
Age of the Job Manager oldest_operation_age|任务管理器最早操作时长
Age of the Job Manager oldest_task_wait_age|任务管理器最早任务等待时长
Cumulative Job Manager dirty_runs_total|任务管理器脏状态运行累计次数
Cumulative Job Manager duplicate_uid_rejected_total|任务管理器重复 UID 拒绝累计次数
Cumulative Job Manager frame_failures_total|任务管理器数据帧失败累计次数
Cumulative Job Manager frames_committed_total|任务管理器已提交数据帧累计数
Cumulative Job Manager operation_timeouts_total|任务管理器操作超时累计次数
Cumulative Job Manager operations_admitted_total|任务管理器已接纳操作累计数
Cumulative Job Manager operations_rejected_total|任务管理器已拒绝操作累计数
Cumulative Job Manager results_disposed_total|任务管理器已释放结果累计数
Cumulative Job Manager shutdown_rejected_total|任务管理器关闭期间拒绝累计次数
Cumulative Job Manager task_panics_total|任务管理器任务崩溃累计次数
Current Job Manager claim_keys_tracked|任务管理器当前跟踪的绑定键数
Current Job Manager claim_waiters|任务管理器当前等待绑定数量
Current Job Manager function_invocations_active|任务管理器当前活动函数调用数
Current Job Manager jobs_active|任务管理器当前活动作业数
Current Job Manager operations_active|任务管理器当前活动操作数
Current Job Manager process_attempts_active|任务管理器当前活动进程尝试数
Current Job Manager process_attempts_admitted|任务管理器当前已接纳进程尝试数
Current Job Manager process_attempts_contained|任务管理器当前已约束进程尝试数
Current Job Manager process_attempts_probing|任务管理器当前探测中的进程尝试数
Current Job Manager process_attempts_quarantined|任务管理器当前隔离的进程尝试数
Current Job Manager tasks_active|任务管理器当前活动任务数
Current Job Manager tasks_queued|任务管理器当前排队任务数
Build phase duration in seconds|构建各阶段耗时（秒）
BuildPlan calls skipped due failed collect cycle|因采集周期失败而跳过的构建计划调用
BuildPlan duration in seconds|构建计划耗时（秒）
Failed BuildPlan calls|失败的构建计划调用
Successful BuildPlan calls|成功的构建计划调用
Chart instances in the latest successful build rollup|最近成功构建中的图表实例数
Dimensions by metric type|按指标类型统计维度
Inferred dimensions in the latest successful build rollup|最近成功构建中推断的维度数
Planner actions by kind|按类型统计计划器操作
Route cache entries in the latest successful build rollup|最近成功构建中的路由缓存条目数
Route cache entries pruned|已清理的路由缓存条目数
Route cache entries retained after prune|清理后保留的路由缓存条目数
Route cache full-drop prune events|路由缓存完全清空事件数
Route cache lookup hits|路由缓存查找命中数
Route cache lookup misses|路由缓存查找未命中数
SNMP topology internal devices|SNMP 拓扑内部设备数
SNMP topology internal last refresh|SNMP 拓扑内部最近刷新时间
SNMP topology internal refreshes|SNMP 拓扑内部刷新次数
Series left unmatched after routing|路由后仍未匹配的序列数
Series matched by autogen fallback|自动生成回退规则匹配的序列数
Series matched by template or autogen|模板或自动生成规则匹配的序列数
Series scanned by planner|计划器扫描的序列数
`));
  const phrases = parse(`
CPU context switches|CPU 上下文切换
CPU utilization (100% = 1 core)|CPU 使用率（100% 表示 1 个核心）
CPU utilization (100%% = 1 core)|CPU 使用率（100% 表示 1 个核心）
disk logical IO|逻辑磁盘 I/O
disk physical IO|物理磁盘 I/O
memory RSS usage|常驻内存用量
memory page faults|内存缺页
memory usage without shared|不含共享部分的内存用量
open file descriptors limit|打开文件描述符上限
open files descriptors|打开的文件描述符
swap usage|交换内存用量
virtual memory size|虚拟内存大小
Disk Read/Write Bandwidth|磁盘读写带宽
Disk Read/Write Operations|磁盘读写操作
Memory Page Faults|内存缺页
Number of Processes|进程数
Writeback Memory|待回写内存
CPU utilization|CPU 使用率
context switches|上下文切换
Page Faults|缺页
FRAG Sockets Memory|分片套接字内存
UDP Sockets Memory|UDP 套接字内存
TCP Handshake Issues|TCP 握手问题
Fragments Reassembly|分片重组
Fragments Sent|已发送分片
Broadcast Bandwidth|广播带宽
Broadcast Packets|广播数据包
Multicast Bandwidth|多播带宽
Multicast Packets|多播数据包
ICMP Group Membership|ICMP 组成员消息
ICMP MLDv2 Reports|ICMP MLDv2 报告
Neighbor Messages|邻居发现消息
Router Messages|路由器消息
ICMP Redirects|ICMP 重定向
ICMP Errors|ICMP 错误
ICMP Messages|ICMP 消息
ICMP Packets|ICMP 数据包
ICMP Echo|ICMP 回显
ICMP Types|ICMP 类型
TCP Packets|TCP 数据包
UDP Packets|UDP 数据包
UDPLite Packets|UDPLite 数据包
UDPlite Packets|UDPLite 数据包
UDPLite Errors|UDPLite 错误
UDP Lite Errors|UDP Lite 错误
TCP Errors|TCP 错误
UDP Errors|UDP 错误
FRAG Sockets|分片套接字
RAW Sockets|原始套接字
UDP Sockets|UDP 套接字
TCP Sockets|TCP 套接字
UDPLITE Sockets|UDPLITE 套接字
TCP Opens|TCP 建立连接
ECN Statistics|ECN 统计
ECT Packets|ECT 数据包
Read IOPS|每秒读操作数
Write IOPS|每秒写操作数
Disk Reads|磁盘读取
Disk Writes|磁盘写入
Network Traffic|网络流量
Memory Usage|内存用量
CPU Usage|CPU 使用率
Network Inbound|网络接收
Network Outbound|网络发送
Used RAM|已用内存
Available Memory|可用内存
`);
  // Secondary panels, chart settings and live investigation status (2026-11-11).
  Object.assign(exact, parse(`
No access|无访问权限
You need a Netdata account for this|此功能需要 Netdata 账号
You need to be signed in to use this feature.|请登录后使用此功能。
Manage alerts|管理告警
Drag & drop|拖放
Show/hide post aggregations|显示或隐藏后续聚合
Source unit|原始单位
Anomaly detection percent (%)|异常检测占比（%）
Selection|选择
Host labels|主机标签
Select all|全选
Clear selected|清除所选项
Clear selected items|清除所选项
Other|其他
Configs|配置
Classes|分类
Components|组件
Types|类型
Process|进程
Workload|工作负载
HTTP_endpoint|HTTP 端点
Web_Server|Web 服务器
Web_log|Web 访问日志
uncomparable|无法比较
comparable|可比较
recovered|已恢复
was raised|已触发
with value|数值为
for|对应
Timestamp|时间戳
Tags|标签
Waiting analysis…|等待分析…
Waiting analysis...|等待分析…
Waiting for data|等待数据
Analyzing|正在分析
Additional timeframes queued|更多时段正在排队
More|更多
Timeframe analysis complete|时段分析完成
Timeframe analysis progress|时段分析进度
Waiting for the timeframe analysis to finish|正在等待时段分析完成
Loading selected-window trends|正在加载所选时段的趋势
Trend refresh is paused while this tab is hidden.|当前标签页在后台，趋势刷新已暂停。
Manage Space|管理空间
Space Settings|空间设置
User Settings|用户设置
Choose Your Theme|选择主题
Auto (matches theme)|自动（跟随主题）
Default refresh mode|默认刷新模式
Auto-refresh Mode|自动刷新模式
Configure refresh mode, data display, and number format|设置刷新方式、数据显示和数字格式
Configure your Netdata settings and preferences|设置 Netdata 选项和偏好
Choose how numbers, dates, and times are formatted. This setting applies throughout the entire application.|选择数字、日期和时间的格式，此设置适用于整个应用。
Choose the chart presentation density.|选择图表的信息密度。
Choose where line charts list their values. Automatic picks by chart width and number of series, and lets you set a layout per chart in its settings. Any other choice applies to every chart.|选择折线图数值列表的位置。自动模式根据图表宽度和序列数量选择，也可在单个图表中设置布局；其他选项适用于全部图表。
When set to As reported, every chart keeps the units its node sent. Choose Celsius or Fahrenheit to convert temperatures throughout the application. Individual charts can still override this from their settings.|选择“按原样显示”时，图表沿用节点上报的单位。选择摄氏度或华氏度可统一转换温度，单个图表仍可单独设置。
When set to Time, charts that present seconds will show them as DDd:HH:MM:SS. When set to Seconds, the raw number of seconds is presented.|选择“时间”时，以天、时、分、秒显示秒数；选择“秒”时，直接显示原始秒数。
Standard Play pauses refreshes when the tab loses focus. Force Play keeps refreshing continuously — useful for monitoring dashboards on secondary screens.|普通播放在标签页失去焦点时暂停刷新；强制播放始终保持刷新，适合副屏监控。
Click to pause automatic refreshes.|点击暂停自动刷新。
Dashboard is Auto-Refreshing|仪表盘正在自动刷新
Charts Design|图表样式
Charts design|图表样式
Charts Visualised|已显示图表
Charts visualized|已显示图表
Chart Elements|图表元素
Chart Information|图表信息
Chart info|图表信息
Chart context|图表上下文
Chart Prefix|图表前缀
Chart selection:|图表选择：
Chart couldn't be loaded|无法加载图表
Charts couldn't load|无法加载图表
Loading charts...|正在加载图表…
Change chart view|更改图表视图
Current value of the chart|图表当前值
Below the chart|图表下方
Beside the chart|图表旁边
Chart Values - View dimension values, statistics, and time ranges|图表数值：查看维度数值、统计信息和时间范围
Customize the chart display including value range, number format, and visibility of chart elements.|设置图表的数值范围、数字格式和元素显示方式。
Switch between line charts, stacked areas, heatmaps, bar charts, and more to visualize your data differently.|切换折线图、堆叠面积图、热力图、柱状图等显示方式。
Export the chart data as CSV, PDF, or PNG for reports and offline analysis.|将图表数据导出为 CSV、PDF 或 PNG，用于报告或离线分析。
Correlate - Find metrics that correlate with the current chart's behavior|关联分析：查找与当前图表变化相关的指标
Click to find correlated metrics for the selected area|点击查找与所选区域相关的指标
Hover a chart and open ⋯ to change the chart type, see chart info, download data or save settings.|将鼠标移到图表上并打开“⋯”，即可更改图表类型、查看信息、下载数据或保存设置。
Learn what a chart shows and what you can do with it|了解图表的含义和可用操作
View real-time charts and learn to filter and analyze your data|查看实时图表，了解如何筛选和分析数据
This is your infrastructure overview. Charts are organized by category showing CPU, memory, network, disk, and application metrics in real-time.|这里是基础设施概览，图表按类别实时显示 CPU、内存、网络、磁盘和应用指标。
Charts are grouped by what they measure, like CPU, memory, network and disks.|图表按监控对象分组，如 CPU、内存、网络和磁盘。
Add Chart|添加图表
Add Charts to|添加图表到
Add Chart to Dashboard|将图表添加到仪表盘
Add chart to one or more dashboards from the|从此处将图表添加到一个或多个仪表盘
Add charts from your infrastructure|从基础设施中添加图表
Add more charts|添加更多图表
Chart added in|图表已添加到
Create a new dashboard and add this chart|新建仪表盘并添加此图表
Drag a chart by its title into a custom dashboard.|拖动图表标题，将图表放入自定义仪表盘。
Drag and drop this chart into a custom dashboard to build your personalized monitoring view.|将此图表拖入自定义仪表盘，构建个性化监控视图。
Drag Chart Contents|拖动图表内容
Drag the contents of a chart, by pressing the left mouse button and moving the mouse,|按住鼠标左键并移动鼠标，可拖动图表内容，
Double Click a chart to reset all the charts to their default auto-refreshing state.|双击图表可将所有图表恢复为默认的自动刷新状态。
All the other visible charts will also show and highlight their values for the same timestamp.|其他可见图表也会显示并高亮同一时刻的数值。
All the charts will follow soon after you let the chart alone (this little delay is by design: it speeds up your browser and lets you focus on what you are exploring).|停止操作当前图表后，其他图表会稍后同步。这一短暂延迟有助于保持浏览器流畅，便于专注查看当前图表。
Touch and Drag the contents of a chart to pan it horizontally.|触摸并拖动图表内容，可水平平移图表。
With two fingers, zoom in or out.|使用双指放大或缩小。
Zoom in|放大
Zoom out|缩小
Zoom reset|重置缩放
Zoom to fit|缩放至合适大小
Highlight Timeframe|高亮时段
Clear chart selection|清除图表选择
Filter Nodes|筛选节点
Global Nodes Filter|全局节点筛选
Filter Your Data (NIDL)|筛选数据（节点、实例、维度和标签）
Filter by node|按节点筛选
Filter contexts|筛选指标上下文
Filter toolbox|筛选工具箱
Filter to these|仅显示这些项
Hide filters|隐藏筛选条件
Active Filters (|已启用的筛选条件（
Add filter on the sidebar|在侧栏添加筛选条件
Add more filters...|添加更多筛选条件…
Start typing to search filters...|输入内容搜索筛选条件…
Filter matches highlighted|已高亮筛选匹配项
About filter match highlighting|关于筛选匹配项高亮
Dynamic filters|动态筛选
All selected options have been cleared.|已清除全部所选项。
Filter is disabled. Enable to apply selections to results.|筛选条件已停用，启用后将应用于结果。
Use the global node filter to focus on specific nodes|使用全局节点筛选，专注查看指定节点
Click here to filter your view to specific nodes in your infrastructure|点击这里，筛选并查看基础设施中的指定节点
Use N-I-D-L filters to slice metrics by Nodes, Instances, Dimensions, and Labels. Each filter shows contribution % and anomaly rates.|使用节点、实例、维度和标签筛选指标；每个筛选项都会显示贡献占比和异常率。
View or filter the contributing time-series labels to this chart. This menu also presents the contribution of each label on the chart, and a break down of the anomaly rate of the data per label.|查看或筛选此图表的时间序列标签。菜单还会显示每个标签的贡献占比和异常率。
View or filter the original dimensions contributing time-series metrics to this chart. This menu also presents the contribution of each original dimension on the chart, and a break down of the anomaly rate of the data per dimension.|查看或筛选此图表的原始维度。菜单还会显示每个维度的贡献占比和异常率。
View or select the aggregation function applied when multiple source time-series metrics need to be grouped together to be presented as dimensions on this chart.|选择聚合函数，将多条原始时间序列分组为图表中显示的维度。
View or select the context applied on this chart.|查看或选择此图表的指标上下文。
Filter the alerts. Count next to each item refer to the number of alerts that will be selected.|筛选告警，各项旁的数字表示将选中的告警数量。
Filter your nodes for this page. Count next to each item refer to the number of nodes that will be selected and used across the page.|筛选当前页面使用的节点，各项旁的数字表示将选中的节点数量。
Default settings, not saved|默认设置，尚未保存
Loading settings..|正在加载设置…
Successfully saved settings|设置已保存
All settings have been successfully deleted.|已删除全部设置。
Delete all settings|删除全部设置
Delete setting|删除设置
Delete settings|删除设置
Delete this setting|删除此设置
Delete any previously saved settings.|删除此前保存的所有设置。
Yes, delete settings|是，删除设置
You are about to delete all previously saved settings. This action cannot be reverted.|即将删除此前保存的所有设置，此操作无法撤销。
Create a new setting with your changes|将更改另存为新设置
Create a new settings snapshot|创建设置快照
Create a new setting|创建设置
Add new setting|新增设置
Edit setting|编辑设置
Update this setting with your changes|将更改保存到此设置
This setting is disabled|此设置已停用
No settings found|未找到设置
Try adjusting the time range or chart settings|请尝试调整时间范围或图表设置
Something went wrong while requesting chart metadata.|获取图表元数据时出错。
You don't have any available node to retrieve charts from.|没有可用于获取图表的节点。
You don't have permission to request chart metadata for these nodes.|你无权获取这些节点的图表元数据。
You don't have permission to create dashboards|你无权创建仪表盘
You don't have permission to use Netdata AI|你无权使用 Netdata AI
You do not have permission to query this metric.|你无权查询此指标。
The query timed out. Narrow the selected nodes or timeframe and retry.|查询超时，请减少所选节点或缩短时间范围后重试。
The selected time window is unavailable.|所选时间范围不可用。
The selected calculation is not supported by the current backend.|当前后端不支持所选计算方式。
This metric uses incompatible units across the selected nodes.|所选节点上此指标的单位不兼容。
Switch to Table for the complete accessible data view.|切换到表格，查看完整且支持无障碍访问的数据。
View the Chart|查看图表
Go to chart|转到图表
Notifications Alt+T|通知 Alt+T
conversationSubject|对话主题
gear|设置
question|帮助
antenna|连接
earlyAccess|抢先体验
filterList|筛选列表
heart_hollow|健康状态
resize_handler|调整大小
Otel-logs|OpenTelemetry 日志
Systemd-journal|Systemd 日志
Mean or Average|均值（算术平均值）
Percentile|百分位数
Trimmed Average or Trimmed Mean|截尾平均值
Median|中位数
Trimmed Median|截尾中位数
Standard deviation|标准差
Coefficient of variation or Relative standard deviation|变异系数（相对标准差）
Incremental Sum or Delta|增量总和（差值）
Single exponential smoothing|单指数平滑
Double exponential smoothing|双指数平滑
When the screen resolution provides less points than the points available of the source time-series metrics, use the following aggregation function over time on each metric to reduce the number of points|当屏幕可显示的数据点少于原始时间序列的数据点时，使用以下函数按时间聚合各项指标，减少显示的数据点。
The function below does not combine values at all. Use it for metrics that represent a state rather than a quantity.|以下函数不合并数值，适用于表示状态而非数量的指标。
The functions below lose accuracy when applied on tiered data, compared to high resolution data.|与高分辨率数据相比，以下函数用于分层降采样数据时会降低精度。
Display|显示
Data|数据
Value range|数值范围
Value formatting|数值格式
Scale|刻度
Auto scale|自动刻度
Decimal Places|小数位数
Reset defaults|恢复默认值
Auto|自动
Min value (percent)|最小值（百分比）
Max value (percent)|最大值（百分比）
every|每
Null handling|缺失值处理
Data resolution|数据分辨率
Plugin and chart context|插件与图表上下文
Raw data units|原始数据单位
Total CPU utilization (all cores). 100% here means there is no CPU idle time at all. You can get per core usage at the|所有核心的 CPU 总使用率。100% 表示 CPU 完全没有空闲时间。各核心的使用率可在
section and per application usage at the|部分查看；各应用的使用率可在
Applications Monitoring|应用监控
section.|部分查看。
Download as CSV|下载为 CSV
Values converted to display units|数值已转换为显示单位
Download raw data|下载原始数据
Raw values straight from the database|直接导出数据库中的原始数值
Download as PDF|下载为 PDF
Rendered chart image, A4 page|渲染后的图表图像，A4 页面
Download as PNG|下载为 PNG
Rendered chart image, transparent-friendly|渲染后的图表图像，支持透明背景
View or select the aggregation function applied on each time-series metric when the number of points in the database are more than the points your screen resolution provides to present this chart. This function is helpful when viewing long time-frames, like days, weeks or months, to quickly spot anomalies, spikes or dives.|当数据库的数据点多于屏幕可显示的数量时，选择用于各时间序列的聚合函数。查看数天、数周或数月的数据时，这有助于快速发现异常、尖峰或骤降。
Early access|抢先体验
Previews of what is coming to Netdata.|预览 Netdata 即将推出的功能。
New navigation and charts|新版导航和图表
Use the redesigned navigation and the new charts, with the modern design drawn by the faster uPlot renderer.|使用重新设计的导航和图表，通过更快的 uPlot 渲染器呈现现代界面。
Try the new UI|体验新版界面
More previews|更多预览功能
Query and visualize OpenTelemetry traces to follow a request across services and find where the latency comes from.|查询并可视化 OpenTelemetry 调用链，跟踪请求经过的服务，定位延迟来源。
Switching a feature reloads the page.|切换功能后将重新加载页面。
Saved views|已保存的视图
Filter by Groups, Node status, Node version...|按分组、节点状态、节点版本等筛选…
Filter by Status, Name, Context...|按状态、名称、指标上下文等筛选…
Typography|文字样式
Infrastructure|基础设施
Room Overview|空间概览
Templates|模板
Untitled Dashboard|未命名仪表盘
Ranking candidate timeframes|正在对候选时段排序
About Peak|关于峰值
About Avg|关于平均值
About Findings|关于分析结果
About Peak %|关于峰值占比
About Avg %|关于平均占比
About Anomalous|关于异常状态
Search Areas|搜索范围
Find Changes method|变化检测方法
nodes_hollow|节点
cluster_spaces|空间分组
information|信息
heart|健康状态
alarmNew|新建告警
upload|上传
chevron_down|展开
ai|人工智能
User|用户
Edit your account settings and manage your notifications|编辑账号设置并管理通知
UI Theme|界面主题
Minimal|精简
Fixed|固定
Floating|悬浮
When set to Fixed, the chart filter toolbox is always visible above the chart and takes vertical space. When set to Floating, it overlays the chart and only appears while you hover the chart, freeing the vertical space and giving you a larger chart area at rest.|选择“固定”时，筛选工具箱始终显示在图表上方，占用纵向空间。选择“悬浮”时，工具箱仅在鼠标移到图表上时浮现，平时为图表保留更多空间。
Configure spaces list|设置空间列表
Reorder spaces by dragging and customize their colors using the color picker.|拖动可调整空间顺序，使用颜色选择器可设置各空间的颜色。
Which dimensions to show?|显示哪些维度？
When set to Non Zero, dimensions that have all their values (within the current view) set to zero will not be transferred from the netdata server (except if all dimensions of the chart are zero, in which case this setting does nothing - all dimensions are transferred and shown). When set to All, all dimensions will always be shown. Set it to Non Zero to lower the data transferred between netdata and your browser, lower the CPU requirements of your browser (fewer lines to draw) and increase the focus on the legends (fewer entries at the legends).|选择“非零”时，不传输当前视图中数值始终为零的维度；若图表的所有维度均为零，则仍传输并显示全部维度。选择“全部”时始终显示全部维度。“非零”可减少传输量和浏览器绘图开销，也能减少图例条目。
Non Zero|非零
Choose your preferred default refresh mode. Standard Play pauses automatic refreshes when the browser tab loses focus to save system resources. Force Play keeps refreshing continuously, even when the tab is not in focus. This is useful for monitoring dashboards on secondary screens.|选择默认刷新模式。普通播放在标签页失去焦点时暂停自动刷新，以节省资源；强制播放在后台也持续刷新，适合副屏监控。
Standard Play|普通播放
Force Play|强制播放
Number and date format|数字与日期格式
Choose how numbers, dates, and times are formatted throughout the application.|选择整个应用使用的数字、日期和时间格式。
Browser default|浏览器默认
Which units to use for temperatures?|使用哪种温度单位？
As reported|按原样显示
Celsius|摄氏度
Fahrenheit|华氏度
Convert seconds to time?|将秒数转换为时间？
Always load home page statistics|始终加载首页统计
By default, some additional statistics (data replication, alerts, available metrics etc) are not loaded in home page when the room contains a big number of nodes.|默认情况下，空间中的节点较多时，首页不会自动加载数据复制、告警、可用指标等附加统计。
Load statistics on demand|按需加载统计
Always load statistics|始终加载统计
seconds|秒
minutes|分钟
hours|小时
days|天
weeks|周
month|个月
months|个月
year|年
years|年
follows now|跟随当前时间
Beijing, Chongqing, Hong Kong, Urumqi|北京、重庆、香港、乌鲁木齐
Select a start date|选择开始日期
Select an end date|选择结束日期
startDate|开始日期
endDate|结束日期
Server Errors Responses By Status Code|按状态码统计服务器错误响应
trashcan|删除
switch_off|已关闭
switch_on|已开启
plus|添加
nav_dots|更多操作
pencilOutline|编辑
Toggle calendar|展开或收起日历
Preparing node data|正在准备节点数据
`));
  // Only used inside .dashboard-context-info; short prose fragments are not metric names.
  const prose = Object.assign(Object.create(null), {
  "The number of CPU context switches. Voluntary context switches occur when a process yields control of the CPU to another process. Nonvoluntary context switches occur when the kernel preempts a process and gives control of the CPU to another process.": "CPU 上下文切换次数。自愿切换发生在进程主动将 CPU 控制权交给其他进程时；非自愿切换发生在内核抢占进程并将 CPU 控制权交给其他进程时。",
  "The amount of time the CPU was busy executing code in": "CPU 执行代码所用的时间，运行模式为",
  "user and kernel modes": "用户态和内核态",
  "(all cores).": "（所有核心）。",
  "The amount of data that has been transferred to/from the storage layer. It includes things such as terminal I/O and is unaffected by whether or not actual physical disk I/O was required (the read/write operation might have been satisfied from pagecache).": "与存储层之间传输的数据量，包括终端 I/O 等操作，无论是否实际访问物理磁盘都会计入（读写可能由页缓存直接完成）。",
  "The amount of data that has been transferred to/from the storage layer. Actual physical disk I/O was required.": "与存储层之间传输的数据量，仅统计实际发生物理磁盘 I/O 的操作。",
  "Number of file descriptors used.": "已使用的文件描述符数量。",
  "Percentage of available file descriptors used.": "已用文件描述符占可用上限的比例。",
  "The number of": "数量：",
  "minor": "次要",
  "and": "和",
  "major": "主要",
  "page faults.": "缺页。",
  "The amount of used memory, excluding shared memory.": "已用内存量，不包含共享内存。",
  "Resident Set Size (RSS) is the amount of physical memory that is currently being used by the processes. This includes the process's code, data, stack, and shared libraries.": "常驻内存大小（RSS）是进程当前使用的物理内存量，包括进程代码、数据、栈和共享库。",
  "The amount of swapped-out virtual memory by anonymous private pages. This does not include shared swap memory.": "匿名私有页已换出到交换区的虚拟内存量，不包含共享交换内存。",
  "The period of time within which at least one process in the group has been running.": "组内至少有一个进程处于运行状态的持续时间。",
  "The amount of allocated virtual memory. Check": "已分配的虚拟内存量。请参阅",
  "this article": "这篇文章",
  "for more details.": "了解详情。",
  "Total CPU utilization within the system-wide CPU resources (all cores). The amount of time spent by tasks of the cgroup in": "控制组占整个系统 CPU 资源（所有核心）的使用率，统计控制组任务运行于以下模式的时间：",
  "user and kernel": "用户态和内核态",
  "modes.": "。",
  "Full": "全部阻塞",
  "indicates the share of time in which": "表示以下情况的时间占比：",
  "all non-idle tasks": "所有非空闲任务",
  "are stalled on CPU resource simultaneously. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.": "同时因等待 CPU 资源而阻塞。分别统计最近 10、60 和 300 秒窗口内的趋势。",
  "The amount of time all non-idle processes have been stalled due to CPU congestion.": "所有非空闲进程同时因 CPU 资源不足而阻塞的累计时间。",
  "Total CPU utilization within the configured or system-wide (if not set) limits. When the CPU utilization of a cgroup exceeds the limit for the configured period, the tasks belonging to its hierarchy will be throttled and are not allowed to run again until the next period.": "相对于已配置上限的 CPU 总使用率；未设置上限时以系统总资源为准。控制组在一个配置周期内用尽 CPU 配额后，其层级下的任务会被限流，直到下一周期才可继续运行。",
  "Some": "部分阻塞",
  "indicates the share of time in which at least": "表示以下情况的时间占比：至少有",
  "some tasks": "部分任务",
  "are stalled on CPU. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.": "因等待 CPU 而阻塞。分别统计最近 10、60 和 300 秒窗口内的趋势。",
  "The amount of time some processes have been waiting for CPU time.": "部分进程等待 CPU 时间的累计时长。",
  "The amount of data transferred to and from specific devices as seen by the CFQ scheduler. It is not updated when the CFQ scheduler is operating on a request queue.": "CFQ 调度器所统计的与指定设备之间传输的数据量。CFQ 调度器在请求队列上执行操作时，此值不会更新。",
  "line indicates the share of time in which": "曲线表示以下情况的时间占比：",
  "are stalled on I/O resource simultaneously. In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. This has severe impact on performance. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.": "同时因等待 I/O 资源而阻塞。此时 CPU 周期被浪费，工作负载若长期处于该状态，就会发生抖动并严重影响性能。分别统计最近 10、60 和 300 秒窗口内的趋势。",
  "The amount of time all non-idle processes have been stalled due to I/O congestion.": "所有非空闲进程同时因 I/O 拥塞而阻塞的累计时间。",
  "are stalled on I/O. In this state the CPU is still doing productive work. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.": "因等待 I/O 而阻塞。此时 CPU 仍能执行有效工作。分别统计最近 10、60 和 300 秒窗口内的趋势。",
  "The amount of time some processes have been waiting due to I/O congestion.": "部分进程因 I/O 拥塞而等待的累计时间。",
  "Memory usage statistics. The individual metrics are described in the memory.stat section for": "内存使用统计。各项指标的说明可参阅以下文档的 memory.stat 部分：",
  "The amount of used RAM and swap memory.": "已使用的物理内存和交换区容量。",
  "RAM usage within the configured or system-wide (if not set) limits. When the RAM usage of a cgroup exceeds the limit, OOM killer will start killing the tasks belonging to the cgroup.": "相对于已配置上限的内存用量；未设置上限时以系统总资源为准。控制组内存用量超过限制时，OOM 终止机制会开始终止该控制组中的任务。",
  "RAM utilization within the configured or system-wide (if not set) limits. When the RAM utilization of a cgroup exceeds the limit, OOM killer will start killing the tasks belonging to the cgroup.": "相对于已配置上限的内存使用率；未设置上限时以系统总资源为准。控制组内存使用量超过限制时，OOM 终止机制会开始终止该控制组中的任务。",
  "are stalled on memory resource simultaneously. In this state actual CPU cycles are going to waste, and a workload that spends extended time in this state is considered to be thrashing. This has severe impact on performance. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.": "同时因等待内存资源而阻塞。此时 CPU 周期被浪费，工作负载若长期处于该状态，就会发生抖动并严重影响性能。分别统计最近 10、60 和 300 秒窗口内的趋势。",
  "The amount of time all non-idle processes have been stalled due to memory congestion.": "所有非空闲进程同时因内存资源不足而阻塞的累计时间。",
  "are stalled on memory. In this state the CPU is still doing productive work. The ratios are tracked as recent trends over 10-, 60-, and 300-second windows.": "因等待内存而阻塞。此时 CPU 仍能执行有效工作。分别统计最近 10、60 和 300 秒窗口内的趋势。",
  "The amount of time some processes have been waiting due to memory congestion.": "部分进程因内存资源不足而等待的累计时间。",
  "page fault": "缺页",
  "statistics.": "统计。",
  "- all page faults.": "— 所有缺页。",
  "Swap": "交换区",
  "- major page faults.": "— 主要缺页。",
  "The number of processes currently in the cgroup.": "控制组当前的进程数。",
  "The number of I/O operations performed on specific devices as seen by the CFQ scheduler.": "CFQ 调度器所统计的指定设备 I/O 操作次数。",
  "The percentage of runnable periods when tasks in a cgroup have been throttled. The tasks have not been allowed to run because they have exhausted all of the available time as specified by their CPU quota.": "控制组任务被限流的可运行周期占比。任务用尽 CPU 配额所规定的可用时间后，将暂停运行。",
  "The total time duration for which tasks in a cgroup have been throttled. When an application has used its allotted CPU quota for a given period, it gets throttled until the next period.": "控制组任务被限流的累计时长。应用在一个周期内用尽获配的 CPU 配额后，会被限流，直到下一周期。",
  "Dirty": "脏页",
  "is the amount of memory waiting to be written to disk.": "表示等待写入磁盘的内存量。",
  "Writeback": "回写",
  "is how much memory is actively being written to disk.": "表示正在写入磁盘的内存量。",
  "The average I/O operation size.": "每次 I/O 操作的平均数据量。",
  "The average time for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.": "设备完成 I/O 请求所需的平均时间，包括请求排队时间和实际处理时间。",
  "Backlog is an indication of the duration of pending disk operations. On every I/O event the system is multiplying the time spent doing I/O since the last update of this field with the number of pending operations. While not accurate, this metric can provide an indication of the expected completion time of the operations in progress.": "积压量反映待处理磁盘操作的预计耗时。每次发生 I/O 事件时，系统将该字段上次更新后执行 I/O 的时间乘以待处理操作数。此指标虽不精确，但可帮助估计正在进行的操作何时完成。",
  "Disk Busy Time measures the amount of time the disk was busy with something.": "磁盘忙碌时间表示磁盘正在处理操作的累计时长。",
  "Inodes (or index nodes) are filesystem objects (e.g. files and directories). On many types of file system implementations, the maximum number of inodes is fixed at filesystem creation, limiting the maximum number of files the filesystem can hold. It is possible for a device to run out of inodes. When this happens, new files cannot be created on the device, even though there may be free space available.": "inode（索引节点）用于表示文件、目录等文件系统对象。许多文件系统在创建时就固定了 inode 上限，因此可容纳的文件数也有限。inode 可能先于磁盘空间耗尽；此时即使仍有可用空间，也无法创建新文件。",
  "The amount of data transferred to and from disk.": "磁盘读写传输的数据量。",
  "The sum of the duration of all completed I/O operations. This number can exceed the interval if the disk is able to execute I/O operations in parallel.": "所有已完成 I/O 操作的耗时总和。磁盘若能并行执行多个操作，此值可能超过统计间隔。",
  "The number of merged disk operations. The system is able to merge adjacent I/O operations, for example two 4KB reads can become one 8KB read before given to disk.": "被合并的磁盘操作数。系统可合并相邻的 I/O 操作，例如将两次 4KB 读取合并为一次 8KB 读取后再提交给磁盘。",
  "Completed disk I/O operations. Keep in mind the number of operations requested might be higher, since the system is able to merge adjacent to each other (see merged operations chart).": "已完成的磁盘 I/O 操作数。由于系统会合并相邻操作，原始请求次数可能更高，详见合并操作图表。",
  "I/O operations currently in progress. This metric is a snapshot - it is not an average over the last interval.": "当前正在进行的 I/O 操作数。这是瞬时快照，并非上一个统计间隔的平均值。",
  "Disk space utilization by mount point. Windows folder-mounted volumes, including Cluster Shared Volumes, are identified by their mount path. Reserved for root is automatically reserved by the system to prevent the root user from getting out of space.": "按挂载点统计磁盘空间使用率。Windows 文件夹挂载卷（包括群集共享卷）以挂载路径标识。“为 root 预留”是系统自动保留的空间，以免 root 用户无可用空间。",
  "The average service time for completed I/O operations. This metric is calculated using the total busy time of the disk and the number of completed operations. If the disk is able to execute multiple parallel operations the reporting average service time will be misleading.": "已完成 I/O 操作的平均服务时间，由磁盘总忙碌时间和已完成操作数计算得到。磁盘若支持并行操作，此平均值可能无法准确反映实际耗时。",
  "Disk Utilization measures the amount of time the disk was busy with something. This is not related to its performance. 100% means that the system always had an outstanding operation on the disk. Keep in mind that depending on the underlying technology of the disk, 100% here may or may not be an indication of congestion.": "磁盘使用率反映磁盘处于忙碌状态的时间占比，不能直接衡量性能。100% 表示磁盘始终有尚未完成的操作。是否意味着拥塞，还取决于磁盘的底层技术。",
  "The average discard operation size.": "每次 discard 丢弃操作的平均数据量。",
  "The average time for discard/flush requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.": "设备完成 discard 丢弃或 flush 刷写请求所需的平均时间，包括排队时间和实际处理时间。",
  "The amount of discarded data that are no longer in use by a mounted file system.": "已挂载文件系统中不再使用、已发出丢弃指令的数据量。",
  "The sum of the duration of all completed discard/flush operations. This number can exceed the interval if the disk is able to execute discard/flush operations in parallel.": "所有已完成 discard 丢弃或 flush 刷写操作的耗时总和。磁盘若支持并行处理，此值可能超过统计间隔。",
  "The number of merged discard disk operations. Discard operations which are adjacent to each other may be merged for efficiency.": "被合并的磁盘 discard 丢弃操作数。相邻的丢弃操作可能被合并，以提高效率。",
  "The number (after merges) of completed discard/flush requests.": "合并后已完成的 discard 丢弃或 flush 刷写请求数。",
  "Discard": "丢弃",
  "commands inform disks which blocks of data are no longer considered to be in use and therefore can be erased internally. They are useful for solid-state drivers (SSDs) and thinly-provisioned storage. Discarding/trimming enables the SSD to handle garbage collection more efficiently, which would otherwise slow future write operations to the involved blocks down.": "指令告知磁盘哪些数据块已不再使用、可在内部擦除，适用于固态硬盘（SSD）和精简配置存储。丢弃或 TRIM 操作可提高 SSD 垃圾回收效率，避免影响这些数据块后续的写入速度。",
  "Flush": "刷写",
  "operations transfer all modified in-core data (i.e., modified buffer cache pages) to the disk device so that all changed information can be retrieved even if the system crashes or is rebooted. Flush requests are executed by disks. Flush requests are not tracked for partitions. Before being merged, flush operations are counted as writes.": "操作将内存中已修改的数据（即已修改的缓冲区缓存页）写入磁盘，使系统即使崩溃或重启也能找回更改。flush 请求由磁盘执行，不按分区统计；合并前的 flush 操作计为写操作。",
  "HTTP Current State Duration.": "HTTP 当前状态持续时间。",
  "The": "该",
  "response length": "响应长度",
  "counts the number of characters in the response body. For static pages, this should be mostly constant.": "统计响应正文的字符数。对于静态页面，该值通常应保持稳定。",
  "response time": "响应时间",
  "describes the time passed between request and response. Currently, the accuracy of the response time is low and should be used as reference only.": "表示从发出请求到收到响应的耗时。当前测量精度较低，仅供参考。",
  "This chart verifies the response of the webserver. Each status dimension will have a value of": "此图表检查 Web 服务器的响应。某状态触发时，对应状态维度的值为",
  "if triggered. Dimension": "。维度",
  "is": "为",
  "only if all constraints are satisfied. This chart is most useful for alerts or third-party apps.": "仅当所有条件均满足时成立。此图表主要用于告警或第三方应用。",
  "Available Memory is estimated by the kernel, as the amount of RAM that can be used by userspace processes, without causing swapping.": "可用内存由内核估算，表示可供用户空间进程使用且不会引发交换的内存量。",
  "Number of": "数量：",
  "modified pages": "已修改的页",
  "Linux page cache": "Linux 页缓存",
  ". Netdata shows the dity page": "。Netdata 显示脏页统计，按",
  "application": "应用",
  "cgroup (systemd Services)": "控制组（systemd 服务）",
  "if": "，前提是",
  "apps": "应用",
  "or": "或",
  "plugins are enabled. This chart is provided by the": "插件已启用。此图表由以下组件提供：",
  "eBPF plugin": "eBPF 插件",
  "access": "访问",
  "to data in": "，数据位于",
  ". Netdata shows the hits per": "。Netdata 显示命中统计，按",
  "to data that was not present in": "，所请求数据未包含在",
  ". Netdata shows the missed access per": "。Netdata 显示未命中访问统计，按",
  "ratio": "比率",
  "shows the percentage of data accessed directly in memory. Netdata shows the ratio per": "表示直接从内存访问的数据占比。Netdata 显示此比率，按",
  "Committed Memory, is the sum of all memory which has been allocated by processes.": "已承诺内存是所有进程已分配内存的总和。",
  "The amount of memory with physical corruption problems, identified by": "存在物理损坏的内存量，由以下机制识别：",
  "and set aside by the kernel so it does not get used.": "，并由内核隔离，以避免继续使用。",
  "The total amount of memory being used by the kernel.": "内核正在使用的总内存量。",
  "Slab": "Slab 缓存",
  "- used by the kernel to cache data structures for its own use.": "— 内核用于缓存自身数据结构的内存。",
  "- allocated for each task done by the kernel.": "— 为内核执行的每个任务分配的内存。",
  "- dedicated to the lowest level of page tables (A page table is used to turn a virtual address into a physical memory address).": "— 用于最低层级页表的内存；页表用于将虚拟地址转换为物理地址。",
  "- being used as virtual address space.": "— 用作虚拟地址空间的内存。",
  "- allocated to the per-CPU allocator used to back per-CPU allocations (excludes the cost of metadata). When you create a per-CPU variable, each processor on the system gets its own copy of that variable.": "— 由每 CPU 分配器分配的内存，不含元数据开销。创建每 CPU 变量时，系统中的每个处理器都会获得该变量的一份副本。",
  "The number of processes killed by": "被终止的进程数，执行机制为",
  "Out of Memory": "内存不足",
  "Killer. The kernel's OOM killer is summoned when the system runs short of free memory and is unable to proceed without killing one or more processes. It tries to pick the process whose demise will free the most memory while causing the least misery for users of the system. This counter also includes processes within containers that have exceeded the memory limit.": "终止机制（OOM killer）。系统可用内存不足、必须终止进程才能继续运行时，内核会调用此机制，尽量选择能释放较多内存且对用户影响较小的进程。此计数也包含因超出容器内存限制而被终止的进程。",
  "is a type of interrupt, called trap, raised by computer hardware when a running program accesses a memory page that is mapped into the virtual address space, but not actually loaded into main memory.": "是一种由硬件触发的异常中断：运行中的程序访问已映射到虚拟地址空间、却尚未实际加载到主内存的页时，就可能发生。",
  "Minor": "次要",
  "- the page is loaded in memory at the time the fault is generated, but is not marked in the memory management unit as being loaded in memory.": "— 缺页发生时，该页已在内存中，但内存管理单元尚未将其标记为已加载。",
  "Major": "主要",
  "- generated when the system needs to load the memory page from disk or swap memory.": "— 系统需要从磁盘或交换区加载内存页时产生。",
  "Slab memory": "Slab 缓存内存",
  "Reclaimable": "可回收",
  "- amount of memory which the kernel can reuse.": "— 内核可以重新利用的内存量。",
  "Unreclaimable": "不可回收",
  "- can not be reused even when the kernel is lacking memory.": "— 即使内核内存不足，也无法重新利用。",
  "The current physical link state of the interface.": "接口当前的物理链路状态。",
  "The number of packets that have been dropped at the network interface level.": "在网络接口层丢弃的数据包数量。",
  "Inbound": "入站",
  "- packets received but not processed, e.g. due to": "— 已接收但未处理的数据包，例如由于",
  "softnet backlog": "网络软中断待处理队列",
  "overflow, bad/unintended VLAN tags, unknown or unregistered protocols, IPv6 frames when the server is not configured for IPv6.": "溢出、VLAN 标签错误或不符合预期、协议未知或未注册，或者服务器未配置 IPv6 却收到 IPv6 帧。",
  "Outbound": "出站",
  "- packets dropped on their way to transmission, e.g. due to lack of resources.": "— 发送过程中丢弃的数据包，例如因资源不足而丢弃。",
  "The interface's latest or current": "接口最近协商的或当前使用的",
  "duplex": "双工模式",
  "that the network adapter": "，由网络适配器",
  "negotiated": "协商确定",
  "with the device it is connected to.": "，协商对象为其连接的设备。",
  "Unknown": "未知",
  "- the duplex mode can not be determined.": "— 无法确定双工模式。",
  "Half duplex": "半双工",
  "- the communication is one direction at a time.": "— 同一时刻只能进行单向通信。",
  "Full duplex": "全双工",
  "- the interface is able to send and receive data simultaneously.": "— 接口可同时发送和接收数据。",
  "The number of errors encountered by the network interface.": "网络接口遇到的错误数。",
  "- bad packets received on this interface. It includes dropped packets due to invalid length, CRC, frame alignment, and other errors.": "— 此接口收到的错误数据包，包括因长度无效、CRC、帧对齐及其他错误而丢弃的数据包。",
  "- transmit problems. It includes frames transmission errors due to loss of carrier, FIFO underrun/underflow, heartbeat, late collisions, and other problems.": "— 发送问题，包括因载波丢失、FIFO 欠载、心跳检测错误、后期碰撞等原因导致的帧发送错误。",
  "Frames": "帧",
  "- aggregated counter for dropped packets due to invalid length, FIFO overflow, CRC, and frame alignment errors.": "— 因长度无效、FIFO 溢出、CRC 和帧对齐错误而丢弃的数据包总数。",
  "Collisions": "碰撞",
  "collisions": "碰撞",
  "during packet transmissions.": "，发生在数据包发送过程中。",
  "Carrier": "载波",
  "- aggregated counter for frame transmission errors due to excessive collisions, loss of carrier, device FIFO underrun/underflow, Heartbeat/SQE Test errors, and late collisions.": "— 因碰撞过多、载波丢失、设备 FIFO 欠载、心跳或 SQE 检测错误及后期碰撞导致的帧发送错误总数。",
  "The number of FIFO errors encountered by the network interface.": "网络接口发生的 FIFO 错误数。",
  "- packets dropped because they did not fit into buffers provided by the host, e.g. packets larger than MTU or next buffer in the ring was not available for a scatter transfer.": "— 因无法放入主机提供的缓冲区而丢弃的数据包，例如数据包大于 MTU，或环形队列的下一个缓冲区无法用于分散传输。",
  "- frame transmission errors due to device FIFO underrun/underflow. This condition occurs when the device begins transmission of a frame but is unable to deliver the entire frame to the transmitter in time for transmission.": "— 因设备 FIFO 欠载而发生的帧发送错误：设备已开始发送帧，却无法及时将整个帧交给发送器。",
  "The interface's currently configured": "接口当前配置的",
  "Maximum transmission unit": "最大传输单元",
  "(MTU) value. MTU is the size of the largest protocol data unit that can be communicated in a single network layer transaction.": "（MTU）值。MTU 表示单次网络层传输所能承载的最大协议数据单元大小。",
  "The amount of traffic transferred by the network interface.": "网络接口传输的流量。",
  "The current": "当前的",
  "operational state": "运行状态",
  "of the interface.": "，对应此接口。",
  "- the state can not be determined.": "— 无法确定状态。",
  "NotPresent": "组件缺失",
  "- the interface has missing (typically, hardware) components.": "— 接口缺少组件，通常是硬件组件。",
  "Down": "已断开",
  "- the interface is unable to transfer data on L1, e.g. ethernet is not plugged or interface is administratively down.": "— 接口无法在物理层传输数据，例如网线未连接或接口被管理员停用。",
  "LowerLayerDown": "下层链路断开",
  "- the interface is down due to state of lower-layer interface(s).": "— 接口因下层接口的状态而断开。",
  "Testing": "测试中",
  "- the interface is in testing mode, e.g. cable test. It can’t be used for normal traffic until tests complete.": "— 接口处于测试模式，例如线缆测试；测试完成前无法传输正常流量。",
  "Dormant": "等待激活",
  "- the interface is L1 up, but waiting for an external event, e.g. for a protocol to establish.": "— 接口物理层已连接，但正在等待外部事件，例如协议建立。",
  "Up": "已连接",
  "- the interface is ready to pass packets and can be used.": "— 接口已准备好传输数据包，可正常使用。",
  "The number of packets transferred by the network interface. Received": "网络接口传输的数据包数量。接收的",
  "multicast": "多播",
  "counter is commonly calculated at the device level (unlike": "计数通常在设备层计算，而非仅计算",
  ") and therefore may include packets which did not reach the host.": "），因此可能包含未实际到达本机的数据包。",
  "The interface's latest or current speed that the network adapter": "接口最近协商的或当前使用的速率，由网络适配器",
  "with the device it is connected to. This does not give the max supported speed of the NIC.": "，协商对象为其连接的设备。此值并不表示网卡支持的最高速率。",
  "The number of entries in the conntrack table.": "conntrack 连接跟踪表中的条目数。",
  "Connection Tracker Changes.": "连接跟踪条目变化。",
  "Connection Tracker Errors.": "连接跟踪错误。",
  "Connection Tracker Expectations.": "连接跟踪预期连接。",
  "Connection Tracker New Connections.": "连接跟踪新增连接。",
  "Connection Tracker Searches.": "连接跟踪查找。",
  "The number of accepted and handled connections on the nginx server. Monitoring this metric can help identify potential performance issues related to connections.": "nginx 服务器接受和处理的连接数，可用于识别与连接相关的潜在性能问题。",
  "The current status of connections on the nginx server. Monitoring this metric can help identify potential performance issues related to connections.": "nginx 服务器当前的连接状态，可用于识别与连接相关的潜在性能问题。",
  "The total number of processes in the system.": "系统中的进程总数。",
  "The kernel code can operate in various modes and with various features enabled or disabled, as selected by the": "内核代码可在多种模式下运行，并启用或停用不同功能，具体由以下调用选择：",
  "system call. The system clock status shows the value of the": "系统调用。系统时钟状态显示以下变量的值：",
  "variable in the kernel. The bits of the variable are used to control these functions and record error conditions as they exist.": "，该变量位于内核中；各标志位用于控制这些功能并记录当前错误状态。",
  "- set/cleared by the caller to indicate clock unsynchronized (e.g., when no peers are reachable). This flag is usually controlled by an application program, but the operating system may also set it.": "— 由调用者设置或清除，表示时钟未同步，例如没有可达的时间源。此标志通常由应用控制，操作系统也可能设置它。",
  "- set/cleared by the external hardware clock driver to indicate hardware fault.": "— 由外部硬件时钟驱动设置或清除，表示硬件故障。",
  "Status map": "状态映射",
  ": 0 - bit unset, 1 - bit set.": "：0 — 标志位未设置；1 — 标志位已设置。",
  "A typical NTP client regularly polls one or more NTP servers. The client must compute its": "NTP 客户端通常会定期轮询一个或多个 NTP 服务器，并计算其",
  "time offset": "时间偏移",
  "and round-trip delay. Time offset is the difference in absolute time between the two clocks.": "以及往返延迟。时间偏移表示两个时钟之间的绝对时间差。",
  "The system clock synchronization state as provided by the": "系统时钟同步状态，由以下调用提供：",
  "system call. An unsynchronized clock may be the result of synchronization issues by the NTP daemon or a hardware clock fault. It can take several minutes (usually up to 17) before NTP daemon selects a server to synchronize with.": "系统调用。时钟未同步可能源于 NTP 守护进程的同步问题或硬件时钟故障。NTP 守护进程选择用于同步的服务器可能需要数分钟，通常最长可达 17 分钟。",
  "State map": "状态映射",
  ": 0 - not synchronized, 1 - synchronized.": "：0 — 未同步；1 — 已同步。",
  "Context Switches": "上下文切换",
  ", is the switching of the CPU from one process, task or thread to another. If there are many processes or threads willing to execute and very few CPU cores available to handle them, the system is making more context switching to balance the CPU resources among them. The whole process is computationally intensive. The more the context switches, the slower the system gets.": "表示 CPU 从一个进程、任务或线程切换到另一个。当待运行的进程或线程较多、可用 CPU 核心较少时，系统需要更频繁地切换上下文，以分配 CPU 资源。切换本身会消耗计算资源，频繁切换可能拖慢系统。",
  "Entropy": "熵",
  ", is a pool of random numbers (": "表示随机数池（",
  ") that is mainly used in cryptography. If the pool of entropy gets empty, processes requiring random numbers may run a lot slower (it depends on the interface each program uses), waiting for the pool to be replenished. Ideally a system with high entropy demands should have a hardware device for that purpose (TPM is one such device). There are also several software-only options you may install, like": "），主要用于密码学。熵池耗尽时，需要随机数的进程可能等待补充而变慢，具体取决于所使用的接口。熵需求较高的系统可配备专用硬件，例如 TPM；也可安装纯软件方案，例如",
  ", although these are generally useful only in servers.": "，但这些方案通常主要适用于服务器。",
  "The number of new processes created.": "新创建的进程数。",
  "The temperature of the system as reported by the sensors. High values can indicate cooling issues.": "传感器报告的系统温度。温度偏高可能意味着散热存在问题。",
  "Idle jitter is calculated by netdata. A thread is spawned that requests to sleep for a few microseconds. When the system wakes it up, it measures how many microseconds have passed. The difference between the requested and the actual duration of the sleep, is the": "空闲抖动由 Netdata 计算：创建线程并请求休眠数微秒，唤醒后测量实际经过的时间。请求休眠时长与实际休眠时长之差就是",
  "idle jitter": "空闲抖动",
  ". This number is useful in real-time environments, where CPU jitter can affect the quality of the service (like VoIP media gateways).": "。此值对实时场景有参考意义，因为 CPU 抖动可能影响服务质量，例如 VoIP 媒体网关。",
  "CPU interrupts in detail. At the": "CPU 中断详情。在",
  "section, interrupts are analyzed": "部分，可按以下方式分析中断：",
  "per CPU core": "每个 CPU 核心",
  ". The last column in": "。以下文件的最后一列：",
  "provides an interrupt description or the device name that registered the handler for that interrupt.": "，提供中断说明，或注册该中断处理程序的设备名称。",
  "Total number of CPU interrupts. Check": "CPU 中断总数。请查看",
  "that gives more detail about each interrupt and also the": "以了解各个中断的详情；也可查看",
  "section where interrupts are analyzed": "部分，按以下方式分析中断：",
  "Total Disk I/O, for all physical disks. You can get detailed information about each disk at the": "所有物理磁盘的 I/O 总量。各磁盘详情可查看",
  "section and per application Disk usage at the": "部分；各应用的磁盘使用情况可查看",
  "section. Physical are all the disks that are listed in": "部分。物理磁盘指列于以下目录：",
  ", but do not exist in": "，但不在以下目录中的设备：",
  "Total IP traffic in the system.": "系统 IP 总流量。",
  "Number of used System V IPC semaphore arrays (sets). Semaphores support semaphore sets where each one is a counting semaphore. So when an application requests semaphores, the kernel releases them in sets. The system-wide limit on the maximum number of semaphore sets is specified in": "已使用的 System V IPC 信号量数组（集合）数。每个集合包含多个计数信号量，内核按集合分配。系统范围的最大信号量集合数由以下文件指定：",
  "file (4th field).": "文件的第 4 个字段。",
  "Number of allocated System V IPC semaphores. The system-wide limit on the number of semaphores in all semaphore sets is specified in": "已分配的 System V IPC 信号量数。所有集合中的信号量总数上限由以下文件指定：",
  "file (2nd field).": "文件的第 2 个字段。",
  "Total IPv6 Traffic.": "IPv6 总流量。",
  "Current system load, i.e. the number of processes using CPU or waiting for system resources (usually CPU and disk). The 3 metrics refer to 1, 5 and 15 minute averages. The system calculates this once every 5 seconds. For more information check": "当前系统负载，即正在使用 CPU 或等待系统资源（通常是 CPU 和磁盘）的进程数。三项指标分别为 1、5 和 15 分钟平均值，系统每 5 秒计算一次。详情请参阅",
  "this wikipedia article": "这篇维基百科文章",
  "Total bandwidth of all physical network interfaces. This does not include": "所有物理网络接口的总带宽，不包含",
  ", VPNs, network bridges, IFB devices, bond interfaces, etc. Only the bandwidth of physical network interfaces is aggregated. Physical are all the network interfaces that are listed in": "、VPN、网桥、IFB 设备和绑定接口等，仅汇总物理接口的带宽。物理接口指列于以下文件：",
  "Memory paged from/to disk. This is usually the total disk I/O of the system.": "从磁盘换入或向磁盘换出的内存页数据量，通常对应系统的磁盘 I/O 总量。",
  "Indicates whether a system reboot is required due to pending package updates or other system-level changes.": "表示系统是否因待完成的软件包更新或其他系统级变更而需要重启。",
  "System processes.": "系统进程。",
  "- running or ready to run (runnable).": "— 正在运行或已准备好运行。",
  "Blocked": "阻塞",
  "- currently blocked, waiting for I/O to complete.": "— 当前阻塞，正在等待 I/O 完成。",
  "The number of processes in different states.": "不同状态的进程数。",
  "- Process using the CPU at a particular moment.": "— 当前正在使用 CPU 的进程。",
  "Sleeping (uninterruptible)": "睡眠（不可中断）",
  "- Process will wake when a waited-upon resource becomes available or after a time-out occurs during that wait. Mostly used by device drivers waiting for disk or network I/O.": "— 等待的资源可用或等待超时后唤醒，常见于设备驱动等待磁盘或网络 I/O。",
  "Sleeping (interruptible)": "睡眠（可中断）",
  "- Process is waiting either for a particular time slot or for a particular event to occur.": "— 进程正在等待指定时间到达或某个事件发生。",
  "Zombie": "僵尸进程",
  "- Process that has completed its execution, released the system resources, but its entry is not removed from the process table. Usually occurs in child processes when the parent process still needs to read its child’s exit status. A process that stays a zombie for a long time is generally an error and causes system PID space leak.": "— 进程已执行完毕并释放系统资源，但进程表中的条目尚未移除。通常是父进程尚未读取子进程的退出状态。长期存在的僵尸进程通常表示异常，并会占用系统 PID 空间。",
  "- Process is suspended from proceeding further due to STOP or TSTP signals. In this state, a process will not do anything (not even terminate) until it receives a CONT signal.": "— 进程因 STOP 或 TSTP 信号暂停。在收到 CONT 信号前不会继续执行，甚至无法处理通常的终止请求。",
  "System Random Access Memory (i.e. physical memory) usage.": "系统随机访问内存（即物理内存）的使用情况。",
  "Amount of memory currently used by System V IPC memory segments. The run-time limit on the maximum shared memory segment size that can be created is specified in": "System V IPC 内存段当前使用的内存量。可创建的单个共享内存段大小上限由以下文件指定：",
  "file.": "文件。",
  "Number of allocated System V IPC memory segments. The system-wide maximum number of shared memory segments that can be created is specified in": "已分配的 System V IPC 内存段数量。系统可创建的共享内存段数量上限由以下文件指定：",
  "Total number of software interrupts in the system. At the": "系统软中断总数。在",
  "section, softirqs are analyzed": "部分，可按以下方式分析软中断：",
  "- high priority tasklets.": "— 高优先级 tasklet。",
  "- tasklets related to timer interrupts.": "— 与定时器中断相关的 tasklet。",
  "- used for network transmit and receive processing.": "— 用于处理网络发送和接收。",
  "- handles block I/O completion events.": "— 处理块 I/O 完成事件。",
  "- used by the IO subsystem to increase performance (a NAPI like approach for block devices).": "— I/O 子系统用于提高性能的机制，类似块设备版本的 NAPI。",
  "- handles regular tasklets.": "— 处理普通 tasklet。",
  "- used by the scheduler to perform load-balancing and other scheduling tasks.": "— 调度器用于执行负载均衡及其他调度任务。",
  "- used for high-resolution timers.": "— 用于高精度定时器。",
  "- performs read-copy-update (RCU) processing.": "— 执行读—复制—更新（RCU）处理。",
  "The amount of time the system has been running, including time spent in suspend.": "系统已运行的时间，包括挂起期间的时间。",
  "Dirty is the amount of memory waiting to be written to disk. Writeback is how much memory is actively being written to disk.": "Dirty 表示等待写入磁盘的内存量；Writeback 表示正在写入磁盘的内存量。",
  "Bandwidth.": "带宽。",
  "Current Poll Unique Clients.": "当前采集周期的独立客户端数。",
  "Excluded Requests.": "被排除的请求。",
  "Requests By HTTP Method.": "按 HTTP 方法统计请求。",
  "Requests By HTTP Version.": "按 HTTP 版本统计请求。",
  "Requests By IP Protocol.": "按 IP 协议统计请求。",
  "Requests By Port.": "按端口统计请求。",
  "Total Requests.": "请求总数。",
  "Successful Responses By Status Code.": "按状态码统计成功响应。",
  "Redirects Responses By Status Code.": "按状态码统计重定向响应。",
  "Client Errors Responses By Status Code.": "按状态码统计客户端错误响应。",
  "Responses By Status Code Class.": "按状态码类别统计响应。",
  "Web server responses by type.": "按类型统计 Web 服务器响应。",
  "includes": "包含",
  "error": "错误",
  "except": "不含",
  "bad": "客户端错误",
  "other": "其他",
  "are all the other responses.": "表示所有其他响应。",
  "Upstream Response Time.": "上游响应时间。",
  "Requests By Vhost.": "按虚拟主机统计请求。",
  "A": "一次",
  "Percentage of anomalous dimensions.": "异常维度所占比例。",
  "Flags (0 or 1) to show when an anomaly event has been triggered by the detector.": "表示检测器是否触发异常事件的标志，取值为 0 或 1。",
  "Total count of dimensions considered anomalous or normal.": "被判定为异常或正常的维度总数。",
  "The number of servers and peers that are online and offline.": "在线和离线的服务器及对等节点数量。",
  "- the server or peer is currently online (i.e. assumed by chronyd to be reachable).": "— 服务器或对等节点当前在线，即 chronyd 认为其可达。",
  "- the server or peer is currently offline (i.e. assumed by chronyd to be unreachable, and no measurements from it will be attempted).": "— 服务器或对等节点当前离线，即 chronyd 认为其不可达，暂不尝试从中获取测量数据。",
  "BurstOnline": "突发测量后恢复在线（BurstOnline）",
  "- a burst command has been initiated for the server or peer and is being performed. After the burst is complete, the server or peer will be returned to the online state.": "— 正在对服务器或对等节点执行突发测量命令，完成后将恢复在线状态。",
  "BurstOffline": "突发测量后恢复离线（BurstOffline）",
  "- a burst command has been initiated for the server or peer and is being performed. After the burst is complete, the server or peer will be returned to the offline state.": "— 正在对服务器或对等节点执行突发测量命令，完成后将恢复离线状态。",
  "Unresolved": "尚未解析（Unresolved）",
  "- the name of the server or peer was not resolved to an address yet.": "— 服务器或对等节点的名称尚未解析为地址。",
  "Any error in the system clock is corrected by slightly speeding up or slowing down the system clock until the error has been removed, and then returning to the system clock’s normal speed. A consequence of this is that there will be a period when the system clock (as read by other programs) will be different from chronyd's estimate of the current true time (which it reports to NTP clients when it is operating as a server). The reported value is the difference due to this effect.": "系统通过略微加快或减慢时钟来逐步纠正误差，消除误差后再恢复正常走时。因此在一段时间内，其他程序读取的系统时钟会与 chronyd 估算的当前真实时间不同；chronyd 作为服务器时会将后者报告给 NTP 客户端。此指标显示两者之间的差值。",
  "frequency": "频率偏差",
  "is the rate by which the system’s clock would be wrong if chronyd was not correcting it. It is expressed in ppm (parts per million). For example, a value of 1 ppm would mean that when the system’s clock thinks it has advanced 1 second, it has actually advanced by 1.000001 seconds relative to true time.": "表示没有 chronyd 校正时，系统时钟的走时偏差速率，单位为百万分之一（ppm）。例如 1 ppm 表示系统时钟认为过去了 1 秒时，相对真实时间实际已过去 1.000001 秒。",
  "The estimated local offset on the last clock update. A positive value indicates the local time (as previously estimated true time) was ahead of the time sources.": "上次时钟更新时估算的本地时间偏移。正值表示先前估算的本地真实时间领先于时间源。",
  "The current leap status of the source.": "时间源当前的闰秒状态。",
  "- indicates the normal status (no leap second).": "— 正常状态，没有闰秒调整。",
  "InsertSecond": "插入闰秒（InsertSecond）",
  "- indicates that a leap second will be inserted at the end of the month.": "— 表示本月末将插入一个闰秒。",
  "DeleteSecond": "删除闰秒（DeleteSecond）",
  "- indicates that a leap second will be deleted at the end of the month.": "— 表示本月末将删除一个闰秒。",
  "Unsynchronised": "未同步（Unsynchronised）",
  "- the server has not synchronized properly with the NTP server.": "— 服务器尚未与 NTP 服务器正确同步。",
  "The time elapsed since the last measurement from the reference source was processed.": "距离上次处理参考时间源测量数据所经过的时间。",
  "residual frequency": "剩余频率偏差",
  "for the currently selected reference source. This reflects any difference between what the measurements from the reference source indicate the frequency should be and the frequency currently being used. The reason this is not always zero is that a smoothing procedure is applied to the frequency.": "，针对当前选定的参考时间源。此值反映参考源测量结果所指示的频率与当前实际采用频率之间的差异；由于频率调整使用了平滑处理，因此该值不一定始终为零。",
  "The root mean square (RMS) offset of the system clock from true time. Large offsets may indicate a problem with the clock or network synchronization.": "系统时钟相对真实时间的均方根（RMS）偏移。偏移较大可能表示时钟或网络同步存在问题。",
  "The total of the network path delays to the stratum-1 computer from which the computer is ultimately synchronised.": "沿时间同步链路到最终一级（stratum-1）时间服务器的网络路径延迟总和。",
  "The total dispersion accumulated through all the computers back to the stratum-1 computer from which the computer is ultimately synchronised. Dispersion is due to system clock resolution, statistical measurement variations, etc.": "沿时间同步链路回溯至最终一级（stratum-1）时间服务器时，各节点累计的离散误差总和。此类误差源于时钟分辨率、测量统计波动等因素。",
  "The estimated error bound on the frequency.": "频率偏差的估计误差界限。",
  "The stratum indicates the distance (hops) to the computer with the reference clock. The higher the stratum number, the more the timing accuracy and stability degrades.": "层级（stratum）表示距离参考时钟所在计算机的跳数。层级越高，时间精度和稳定性通常越低。",
  "The interval between clock updates. Shorter intervals may improve accuracy but may also increase network load.": "时钟更新间隔。缩短间隔可能提高精度，也可能增加网络负载。",
  "Total number of Docker containers in various health states.": "各健康状态的 Docker 容器总数。",
  "The accept queue of the kernel holds the fully established TCP connections, waiting to be handled by the listening application.": "内核 accept 队列保存已完成握手的 TCP 连接，等待监听应用接收处理。",
  "Overflows": "溢出",
  "- the number of established connections that could not be handled because the receive queue of the listening application was full.": "— 因监听应用接收队列已满而无法处理的已建立连接数。",
  "Drops": "丢弃",
  "- number of incoming connections that could not be handled, including SYN floods, overflows, out of memory, security issues, no route to destination, reception of related ICMP messages, socket is broadcast or multicast.": "— 无法处理的入站连接数，原因可能包括 SYN 洪泛、队列溢出、内存不足、安全限制、无目标路由、收到相关 ICMP 消息，或套接字为广播或多播类型。",
  "The SYN queue of the kernel tracks TCP handshakes until connections get fully established. It overflows when too many incoming TCP connection requests hang in the half-open state and the server is not configured to fall back to SYN cookies. Overflows are usually caused by SYN flood DoS attacks.": "内核 SYN 队列跟踪尚未完成的 TCP 握手。大量入站请求停留在半开状态，且服务器未配置 SYN cookie 后备机制时，队列可能溢出。SYN 洪泛拒绝服务攻击是常见原因。",
  "- number of connections dropped because the SYN queue was full and SYN cookies were disabled.": "— 因 SYN 队列已满且未启用 SYN cookie 而丢弃的连接数。",
  "Cookies": "SYN cookie",
  "- number of SYN cookies sent because the SYN queue was full.": "— 因 SYN 队列已满而发送的 SYN cookie 数量。",
  "TCP connection aborts.": "TCP 连接中止。",
  "BadData": "数据异常（BadData）",
  "- happens while the connection is on FIN_WAIT1 and the kernel receives a packet with a sequence number beyond the last one for this connection - the kernel responds with RST (closes the connection).": "— 连接处于 FIN_WAIT1 状态时，内核收到序列号超出该连接最后序列号的数据包，并回复 RST 关闭连接。",
  "UserClosed": "应用已关闭（UserClosed）",
  "- happens when the kernel receives data on an already closed connection and responds with RST.": "— 内核在已关闭的连接上收到数据，并回复 RST。",
  "NoMemory": "内存不足（NoMemory）",
  "- happens when there are too many orphaned sockets (not attached to an fd) and the kernel has to drop a connection - sometimes it will send an RST, sometimes it won't.": "— 孤立套接字（未关联文件描述符）过多，内核必须丢弃连接；此时可能发送 RST，也可能不发送。",
  "Timeout": "超时",
  "- happens when a connection times out.": "— 连接超时。",
  "Linger": "延迟关闭（Linger）",
  "- happens when the kernel killed a socket that was already closed by the application and lingered around for long enough.": "— 应用已关闭套接字，但其延迟关闭状态保留过久，最终被内核清理。",
  "Failed": "失败",
  "- happens when the kernel attempted to send an RST but failed because there was no memory available.": "— 内核尝试发送 RST，但因无可用内存而失败。",
  "The number of times a socket was put in memory pressure due to a non fatal memory allocation failure (the kernel attempts to work around this situation by reducing the send buffers, etc).": "套接字因非致命的内存分配失败而进入内存压力状态的次数；内核会尝试缩小发送缓冲区等方式缓解问题。",
  "TCP maintains an out-of-order queue to keep the out-of-order packets in the TCP communication.": "TCP 使用乱序队列暂存通信中未按顺序到达的数据包。",
  "InQueue": "入队（InQueue）",
  "- the TCP layer receives an out-of-order packet and has enough memory to queue it.": "— TCP 层收到乱序数据包，且有足够内存将其放入队列。",
  "Dropped": "已丢弃",
  "- the TCP layer receives an out-of-order packet but does not have enough memory, so drops it.": "— TCP 层收到乱序数据包，但因内存不足而将其丢弃。",
  "Merged": "已合并",
  "- the received out-of-order packet has an overlay with the previous packet. The overlay part will be dropped. All these packets will also be counted into": "— 收到的乱序数据包与先前数据包重叠，重叠部分将被丢弃。这些数据包同时也会计入",
  "Pruned": "已清理",
  "- packets dropped from out-of-order queue because of socket buffer overrun.": "— 因套接字缓冲区超限而从乱序队列中丢弃的数据包。",
  "TCP Packets.": "TCP 数据包。",
  "TCP prevents out-of-order packets by either sequencing them in the correct order or by requesting the retransmission of out-of-order packets.": "TCP 通过重新排序或请求重传来处理乱序数据包。",
  "- detected re-ordering using the timestamp option.": "— 使用时间戳选项检测到的乱序。",
  "- detected re-ordering using Selective Acknowledgment algorithm.": "— 使用选择性确认（SACK）算法检测到的乱序。",
  "- detected re-ordering using Forward Acknowledgment algorithm.": "— 使用前向确认（FACK）算法检测到的乱序。",
  "- detected re-ordering using Fast Retransmit algorithm.": "— 使用快速重传算法检测到的乱序。",
  "SYN cookies": "SYN cookie",
  "are used to mitigate SYN flood.": "用于缓解 SYN 洪泛攻击。",
  "Received": "已接收",
  "- after sending a SYN cookie, it came back to us and passed the check.": "— 发出的 SYN cookie 随客户端响应返回，并通过校验。",
  "Sent": "已发送",
  "- an application was not able to accept a connection fast enough, so the kernel could not store an entry in the queue for this connection. Instead of dropping it, it sent a SYN cookie to the client.": "— 应用未能及时接收连接，内核无法在队列中为其保存条目；内核没有直接丢弃请求，而是向客户端发送 SYN cookie。",
  "- the MSS decoded from the SYN cookie is invalid. When this counter is incremented, the received packet won’t be treated as a SYN cookie.": "— 从 SYN cookie 解码得到的最大报文段大小（MSS）无效。此计数增加时，收到的数据包不会按有效 SYN cookie 处理。",
  "IPv4 ECN Statistics.": "IPv4 显式拥塞通知（ECN）统计。",
  "The number of discarded IPv4 packets.": "已丢弃的 IPv4 数据包数。",
  "InDiscards": "接收丢弃（InDiscards）",
  "OutDiscards": "发送丢弃（OutDiscards）",
  "- inbound and outbound packets which were chosen to be discarded even though no errors had been detected to prevent their being deliverable to a higher-layer protocol.": "— 收发过程中被主动丢弃的数据包；这些数据包本身未检测到会阻止其交付上层协议的错误。",
  "InHdrErrors": "接收报头错误（InHdrErrors）",
  "- input packets that have been discarded due to errors in their IP headers, including bad checksums, version number mismatch, other format errors, time-to-live exceeded, errors discovered in processing their IP options, etc.": "— 因 IP 报头错误而丢弃的入站数据包，包括校验和错误、版本不匹配、其他格式错误、生存时间超限及 IP 选项处理错误等。",
  "OutNoRoutes": "发送无路由（OutNoRoutes）",
  "- packets that have been discarded because no route could be found to transmit them to their destination. This includes any packets which a host cannot route because all of its default gateways are down.": "— 因找不到通往目的地的路由而丢弃的数据包，也包括所有默认网关不可用导致无法路由的数据包。",
  "InAddrErrors": "接收地址错误（InAddrErrors）",
  "- input packets that have been discarded due to invalid IP address or the destination IP address is not a local address and IP forwarding is not enabled.": "— 因 IP 地址无效，或目标地址不是本地地址且未启用 IP 转发而丢弃的入站数据包。",
  "InUnknownProtos": "接收未知协议（InUnknownProtos）",
  "- input packets which were discarded because of an unknown or unsupported protocol.": "— 因协议未知或不受支持而丢弃的入站数据包。",
  "IPv4 reassembly": "IPv4 分片重组",
  "statistics for this system.": "，统计范围为本系统。",
  "- packets that have been successfully reassembled.": "— 成功完成重组的数据包。",
  "- failures detected by the IP reassembly algorithm. This is not necessarily a count of discarded IP fragments since some algorithms can lose track of the number of fragments by combining them as they are received.": "— IP 重组算法检测到的失败次数。此值不一定等于丢弃的分片数，因为部分算法会边接收边合并，无法持续准确统计原始分片数。",
  "- received IP fragments which needed to be reassembled.": "— 收到的需要重组的 IP 分片。",
  "IPv4 fragmentation": "IPv4 分片",
  "- packets that have been successfully fragmented.": "— 成功完成分片的数据包。",
  "- packets that have been discarded because they needed to be fragmented but could not be, e.g. due to": "— 需要分片却无法分片，因而被丢弃的数据包，例如因为",
  "Don't Fragment": "禁止分片",
  "(DF) flag was set.": "（DF）标志已设置。",
  "Created": "已创建",
  "- fragments that have been generated as a result of fragmentation.": "— 分片操作生成的分片数。",
  "The number of transferred IPv4 ICMP messages.": "收发的 IPv4 ICMP 消息数。",
  "- ICMP messages which the host received and attempted to send. Both these counters include errors.": "— 本机收到和尝试发送的 ICMP 消息数，两个计数都包含错误。",
  "The number of IPv4 ICMP errors.": "IPv4 ICMP 错误数。",
  "InErrors": "接收错误（InErrors）",
  "- received ICMP messages but determined as having ICMP-specific errors, e.g. bad ICMP checksums, bad length, etc.": "— 收到的存在 ICMP 协议错误的消息，例如 ICMP 校验和错误或长度错误。",
  "OutErrors": "发送错误（OutErrors）",
  "- ICMP messages which this host did not send due to problems discovered within ICMP such as a lack of buffers. This counter does not include errors discovered outside the ICMP layer such as the inability of IP to route the resultant datagram.": "— 本机因 ICMP 层内部问题而未能发送的消息，例如缓冲区不足。不包含 ICMP 层之外的错误，例如 IP 层无法路由生成的数据报。",
  "InCsumErrors": "接收校验和错误（InCsumErrors）",
  "- received ICMP messages with bad checksum.": "— 收到的校验和错误的 ICMP 消息。",
  "The number of transferred": "收发数量：",
  "IPv4 ICMP control messages": "IPv4 ICMP 控制消息",
  "IPv4 packets statistics for this host.": "本机 IPv4 数据包统计。",
  "- packets received by the IP layer. This counter will be increased even if the packet is dropped later.": "— IP 层收到的数据包，即使随后被丢弃，也会计入。",
  "- packets sent via IP layer, for both single cast and multicast packets. This counter does not include any packets counted in": "— 经 IP 层发送的单播和多播数据包，不包含已计入以下类别的数据包：",
  "Forwarded": "已转发",
  "- input packets for which this host was not their final IP destination, as a result of which an attempt was made to find a route to forward them to that final destination. In hosts which do not act as IP Gateways, this counter will include only those packets which were": "— 最终 IP 目的地不是本机、需要查找路由并转发的入站数据包。在不充当 IP 网关的主机上，仅计入采用以下方式的数据包：",
  "Source-Routed": "源路由",
  "and the Source-Route option processing was successful.": "，且源路由选项处理成功。",
  "Delivered": "已交付",
  "- packets delivered to the upper layer protocols, e.g. TCP, UDP, ICMP, and so on.": "— 已交付上层协议的数据包，例如 TCP、UDP、ICMP 等。",
  "The amount of memory used for packet reassembly.": "数据包重组使用的内存量。",
  "The number of entries in hash tables that are used for packet reassembly.": "用于数据包重组的哈希表条目数。",
  "The number of used": "已使用的数量：",
  "raw sockets": "原始套接字",
  "The amount of memory used by allocated TCP sockets.": "已分配 TCP 套接字使用的内存量。",
  "The number of TCP sockets in the system in certain": "系统 TCP 套接字数量，按以下类别统计：",
  "states": "状态",
  "Alloc": "已分配（Alloc）",
  "- in any TCP state.": "— 包含所有 TCP 状态。",
  "Orphan": "孤立（Orphan）",
  "- no longer attached to a socket descriptor in any user processes, but for which the kernel is still required to maintain state in order to complete the transport protocol.": "— 已不再关联任何用户进程的套接字描述符，但内核仍需维护其状态以完成传输协议处理。",
  "InUse": "正在使用（InUse）",
  "- in any TCP state, excluding TIME-WAIT and CLOSED.": "— 包含除 TIME-WAIT 和 CLOSED 以外的所有 TCP 状态。",
  "TimeWait": "等待关闭（TimeWait）",
  "- in the TIME-WAIT state.": "— 处于 TIME-WAIT 状态。",
  "The amount of memory used by allocated UDP sockets.": "已分配 UDP 套接字使用的内存量。",
  "The number of used UDP sockets.": "已使用的 UDP 套接字数量。",
  "The number of used UDP-Lite sockets.": "已使用的 UDP-Lite 套接字数量。",
  "The number of errors encountered during transferring UDP packets.": "UDP 数据包传输中发生的错误数。",
  "RcvbufErrors": "接收缓冲区错误（RcvbufErrors）",
  "- receive buffer is full.": "— 接收缓冲区已满。",
  "SndbufErrors": "发送缓冲区错误（SndbufErrors）",
  "- send buffer is full, no kernel memory available, or the IP layer reported an error when trying to send the packet and no error queue has been setup.": "— 发送缓冲区已满、内核无可用内存，或 IP 层在尝试发送数据包时报错且未配置错误队列。",
  "- that is an aggregated counter for all errors, excluding": "— 所有错误的汇总计数，不包含",
  "NoPorts": "端口无监听（NoPorts）",
  "- no application is listening at the destination port.": "— 没有应用在目标端口监听。",
  "- a UDP checksum failure is detected.": "— 检测到 UDP 校验和错误。",
  "IgnoredMulti": "忽略多播（IgnoredMulti）",
  "- ignored multicast packets.": "— 被忽略的多播数据包。",
  "The number of transferred UDP-Lite packets.": "收发的 UDP-Lite 数据包数。",
  "The number of errors encountered during transferring UDP-Lite packets.": "UDP-Lite 数据包传输中发生的错误数。",
  "The number of transferred UDP packets.": "收发的 UDP 数据包数。",
  "Total IPv6 broadcast traffic.": "IPv6 广播总流量。",
  "Total number of received IPv6 packets with ECN bits set in the system.": "系统收到的已设置 ECN 标志位的 IPv6 数据包总数。",
  "- congestion encountered.": "— 遇到拥塞。",
  "- non ECN-capable transport.": "— 不支持 ECN 的传输。",
  "- ECN capable transport.": "— 支持 ECN 的传输。",
  "The number of discarded IPv6 packets.": "已丢弃的 IPv6 数据包数。",
  "- packets which were chosen to be discarded even though no errors had been detected to prevent their being deliverable to a higher-layer protocol.": "— 被主动丢弃的数据包；这些数据包本身未检测到会阻止其交付上层协议的错误。",
  "- errors in IP headers, including bad checksums, version number mismatch, other format errors, time-to-live exceeded, etc.": "— IP 报头错误，包括校验和错误、版本不匹配、其他格式错误和生存时间超限等。",
  "- invalid IP address or the destination IP address is not a local address and IP forwarding is not enabled.": "— IP 地址无效，或目标地址不是本地地址且未启用 IP 转发。",
  "- unknown or unsupported protocol.": "— 协议未知或不受支持。",
  "InTooBigErrors": "接收报文过大（InTooBigErrors）",
  "- the size exceeded the link MTU.": "— 数据包大小超过链路 MTU。",
  "InTruncatedPkts": "接收报文截断（InTruncatedPkts）",
  "- packet frame did not carry enough data.": "— 数据帧未携带足够的数据。",
  "InNoRoutes": "转发无路由（InNoRoutes）",
  "- no route could be found while forwarding.": "— 转发时找不到路由。",
  "- no route could be found for packets generated by this host.": "— 本机生成的数据包找不到路由。",
  "IPv6 reassembly": "IPv6 分片重组",
  "- reassembly timeouts detected.": "— 检测到重组超时。",
  "IPv6 fragmentation": "IPv6 分片",
  "The number of transferred ICMPv6 Group Membership messages.": "收发的 ICMPv6 组成员消息数。",
  "Multicast routers send Group Membership Query messages to learn which groups have members on each of their attached physical networks. Host computers respond by sending a Group Membership Report for each multicast group joined by the host. A host computer can also send a Group Membership Report when it joins a new multicast group. Group Membership Reduction messages are sent when a host computer leaves a multicast group.": "多播路由器发送组成员查询消息，以了解所连接物理网络上有哪些组成员。主机会针对已加入的每个多播组发送成员报告；加入新组时也可发送报告，离开组时则发送成员减少消息。",
  "The number of transferred ICMPv6 messages.": "收发的 ICMPv6 消息数。",
  "The number of ICMPv6 Echo messages.": "ICMPv6 回显消息数。",
  "The number of ICMPv6 errors and": "ICMPv6 错误数量，以及",
  "error messages": "错误消息",
  "- bad ICMP messages (bad ICMP checksums, bad length, etc.).": "— 无效的 ICMP 消息，例如校验和错误或长度错误。",
  "- wrong checksum.": "— 校验和错误。",
  "The number of transferred ICMPv6": "收发的 ICMPv6 消息数量，类型为",
  "Multicast Listener Discovery": "多播侦听发现",
  "(MLD) messages.": "（MLD）。",
  "Neighbour Discovery": "邻居发现",
  "messages.": "消息。",
  "Neighbor": "邻居",
  "Solicitations": "请求",
  "are used by nodes to determine the link layer address of a neighbor, or to verify that a neighbor is still reachable via a cached link layer address. Neighbor": "消息用于确定邻居的链路层地址，或验证通过缓存的链路层地址仍可到达邻居。邻居",
  "Advertisements": "通告",
  "are used by nodes to respond to a Neighbor Solicitation message.": "消息用于响应邻居请求。",
  "The number of transferred ICMPv6 Redirect messages. These messages inform a host to update its routing information (to send packets on an alternative route).": "收发的 ICMPv6 重定向消息数。这些消息通知主机更新路由信息，改用其他路由发送数据包。",
  "Router Discovery": "路由器发现",
  "Router": "路由器",
  "message is sent from a computer host to any routers on the local area network to request that they advertise their presence on the network. Router": "消息由主机发送给局域网中的路由器，请求其通告自身的存在。路由器",
  "Advertisement": "通告",
  "message is sent by a router on the local area network to announce its IP address as available for routing.": "消息由局域网中的路由器发送，通告其可用于路由的 IP 地址。",
  "The number of transferred ICMPv6 messages of": "收发的 ICMPv6 消息数量，按以下类别统计：",
  "certain types": "指定类型",
  "Total IPv6 multicast traffic.": "IPv6 多播总流量。",
  "Total transferred IPv6 multicast packets.": "收发的 IPv6 多播数据包总数。",
  "IPv6 packet statistics for this host.": "本机 IPv6 数据包统计。",
  "Delivers": "已交付",
  "The number of TCP sockets in any": "各状态 TCP 套接字的数量，状态为",
  "state": "状态",
  ", excluding TIME-WAIT and CLOSED.": "，不包含 TIME-WAIT 和 CLOSED。",
  "Local and remote sessions.": "本地和远程会话。",
  "Sessions of each session type.": "各会话类型的会话数。",
  "Graphical": "图形会话",
  "- sessions are running under one of X11, Mir, or Wayland.": "— 在 X11、Mir 或 Wayland 图形环境下运行的会话。",
  "Console": "控制台会话",
  "- sessions are usually regular text mode local logins, but depending on how the system is configured may have an associated GUI.": "— 通常为普通文本模式的本地登录，但根据系统配置，也可能关联图形界面。",
  "- sessions are those that do not fall into the above categories (such as sessions for cron jobs or systemd timer units).": "— 不属于上述类别的会话，例如 cron 作业或 systemd 定时器单元的会话。",
  "Users in each user state.": "各用户状态的人数。",
  "- users are not logged in.": "— 用户未登录。",
  "Closing": "正在退出",
  "- users are in the process of logging out without lingering.": "— 用户正在退出登录，且未启用退出后保留服务。",
  "- users are logged in, but have no active sessions.": "— 用户已登录，但没有活动会话。",
  "Lingering": "退出后保留服务",
  "- users are not logged in, but have one or more services still running.": "— 用户未登录，但仍有一个或多个服务在运行。",
  "- users are logged in, and have at least one active session.": "— 用户已登录，且至少有一个活动会话。",
  "Monitor calls to": "监控以下调用的次数：",
  "syscalls": "系统调用",
  "that are responsible for attaching (": "，用于挂载（",
  ") or removing filesystems (": "）或卸载文件系统（",
  "). This chart has relationship with": "）。此图表与以下类别相关：",
  "File systems": "文件系统",
  ". This chart is provided by the": "。此图表由以下组件提供：",
  "Monitor errors in calls to": "监控以下调用中的错误：",
  "Apps Plugin CPU.": "应用监控插件的 CPU 使用情况。",
  "Apps Plugin Files.": "应用监控插件的文件统计。",
  "Memory allocated for hash tables..": "为哈希表分配的内存。",
  "Show number of threads loaded using legacy code (independent binary) or": "显示通过旧版代码（独立二进制程序）或以下方式加载的线程数：",
  "CO-RE (Compile Once Run Everywhere)": "CO-RE（一次编译，到处运行）",
  "Show total number of threads and number of active threads. For more details about the threads, see the": "显示线程总数和活动线程数。线程详情请参阅",
  "official documentation": "官方文档",
  "The memory consumed by the Netdata agent.": "Netdata 代理占用的内存。",
  "The CPU time consumed by Netdata process in user and system space.": "Netdata 进程在用户态和内核态消耗的 CPU 时间。",
  "Service units start and control daemons and the processes they consist of. For details, see": "服务单元用于启动和控制守护进程及其所属进程。详情请参阅",
  "Server Errors Responses By Status Code.": "按状态码统计服务器错误响应。"
});
  const glossary = [
    ['CPU 使用率', '处理器忙碌程度。主机总使用率通常是 0–100%；标明“100% 表示 1 个核心”的进程或容器指标，在多核机器上可以超过 100%。'],
    ['内存', 'Linux 会把空闲内存用于缓存。判断是否紧张应结合 available（可用内存）、交换区和内存压力，不能只看 free（完全空闲）。'],
    ['负载', '1、5、15 分钟内可运行或不可中断等待任务的平均数量，不是百分比。结合 CPU 核心数、I/O 等待和 PSI 判断。'],
    ['PSI 资源阻塞压力', 'some 表示至少有任务因资源不足而等待；full 表示全部非空闲任务都在等待。10 / 60 / 300 是平滑统计窗口，单位秒；stall time 是累计阻塞时间。'],
    ['磁盘与 inode', '吞吐量是每秒读写字节数，IOPS 是每秒操作次数，延迟是每次操作耗时。空间用量与 inode 用量要分别检查。'],
    ['网络', '入站是接收，出站是发送。bit/s 与 Byte/s 相差 8 倍；图中为了区分方向而显示的负值不代表负流量。'],
    ['HTTP 检查', 'success=1 表示本轮检查成功；响应时间是请求耗时。no_connection、timeout、bad_status、bad_content 分别表示无法连接、超时、状态码异常和正文不匹配。'],
    ['告警状态', 'WARNING 为警告，CRITICAL 为严重，CLEAR 为恢复；UNDEFINED 表示规则暂不能计算，不等于服务故障。告警触发还受规则的时间窗口、阈值和恢复延迟影响。'],
    ['维度与分组', '维度是同一图表内的数据序列。AVG 是平均、SUM 是合计；按节点、实例、维度或标签分组会改变聚合方式。单位、主机名、路径、指标 ID 和原始配置保留，便于排障。'],
  ];
  globalThis.NetdataZhDictionary = { exact, phrases, prose, glossary, version: '2026-11-13.2' };
})();
