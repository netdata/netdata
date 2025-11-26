// Netdata Alternative UI - Dashboard Application with Kubernetes Support

class NetdataAltUI {
    constructor() {
        this.nodes = new Map();
        this.selectedNode = null;
        this.selectedK8sCluster = null;
        this.ws = null;
        this.charts = new Map();
        this.k8sCharts = new Map();
        this.timeRange = 1800000; // 30 minutes default
        this.k8sTimeRange = 1800000;
        this.k8sNamespaceFilter = '';
        this.k8sResourceFilter = '';
        this.colors = [
            '#00d9ff', '#00ff88', '#ff6b6b', '#ffd93d', '#6bcb77',
            '#4d96ff', '#ff6b9c', '#c9b1ff', '#ff9f43', '#54a0ff'
        ];
        this.theme = localStorage.getItem('theme') || 'dark';

        this.init();
    }

    init() {
        // Apply theme
        if (this.theme === 'light') {
            document.documentElement.setAttribute('data-theme', 'light');
        }

        // Set up event listeners
        document.getElementById('nodeSearch').addEventListener('input', (e) => {
            this.filterNodes(e.target.value);
        });

        document.getElementById('timeRange').addEventListener('change', (e) => {
            this.timeRange = parseInt(e.target.value);
            this.refreshCharts();
        });

        // Connect WebSocket
        this.connectWebSocket();

        // Start chart refresh interval
        setInterval(() => {
            if (this.selectedNode) {
                this.refreshCharts();
            }
            if (this.selectedK8sCluster) {
                this.refreshK8sCharts();
            }
        }, 1000);
    }

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            this.updateConnectionStatus('connected');
        };

        this.ws.onclose = () => {
            this.updateConnectionStatus('disconnected');
            setTimeout(() => this.connectWebSocket(), 3000);
        };

        this.ws.onerror = () => {
            this.updateConnectionStatus('disconnected');
        };

        this.ws.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                this.handleWebSocketMessage(msg);
            } catch (e) {
                console.error('Failed to parse WebSocket message:', e);
            }
        };
    }

    handleWebSocketMessage(msg) {
        switch (msg.type) {
            case 'init':
                this.nodes.clear();
                if (Array.isArray(msg.payload)) {
                    msg.payload.forEach(node => {
                        this.nodes.set(node.id, node);
                    });
                }
                this.renderNodeList();
                break;

            case 'node_added':
                this.nodes.set(msg.payload.id, msg.payload);
                this.renderNodeList();
                break;

            case 'node_online':
            case 'node_offline':
                const node = this.nodes.get(msg.payload.node_id);
                if (node) {
                    node.online = msg.type === 'node_online';
                    this.renderNodeList();
                }
                break;

            case 'metrics_update':
                if (this.selectedNode && msg.payload.node_id === this.selectedNode) {
                    this.updateChartsRealtime(msg.payload);
                }
                if (this.selectedK8sCluster && msg.payload.node_id === this.selectedK8sCluster) {
                    this.updateK8sChartsRealtime(msg.payload);
                }
                break;
        }
    }

    updateConnectionStatus(status) {
        const el = document.getElementById('connectionStatus');
        el.className = 'connection-status ' + status;
        el.textContent = status === 'connected' ? 'Connected' :
                         status === 'disconnected' ? 'Disconnected' : 'Connecting...';
    }

    isKubernetesNode(node) {
        return node.os === 'kubernetes' ||
               (node.labels && node.labels.type === 'kubernetes');
    }

    renderNodeList() {
        const k8sSection = document.getElementById('k8sSection');
        const k8sClusters = document.getElementById('k8sClusters');
        const nodeList = document.getElementById('nodeList');
        const count = document.getElementById('nodeCount');

        const allNodes = Array.from(this.nodes.values());
        const k8sNodes = allNodes.filter(n => this.isKubernetesNode(n));
        const appNodes = allNodes.filter(n => !this.isKubernetesNode(n));

        count.textContent = `${this.nodes.size} node${this.nodes.size !== 1 ? 's' : ''}`;

        // Render Kubernetes clusters
        if (k8sNodes.length > 0) {
            k8sSection.style.display = 'block';
            k8sClusters.innerHTML = k8sNodes
                .sort((a, b) => a.name.localeCompare(b.name))
                .map(node => `
                    <div class="k8s-cluster-item ${node.id === this.selectedK8sCluster ? 'selected' : ''}"
                         onclick="app.selectK8sCluster('${node.id}')">
                        <div class="k8s-cluster-header">
                            <span class="k8s-cluster-name">${this.escapeHtml(node.name)}</span>
                            <span class="node-status ${node.online ? 'online' : ''}"></span>
                        </div>
                        <div class="k8s-cluster-meta">
                            <span class="k8s-cluster-stat">${node.chart_count || 0} charts</span>
                        </div>
                    </div>
                `).join('');
        } else {
            k8sSection.style.display = 'none';
        }

        // Render application nodes
        if (appNodes.length === 0) {
            nodeList.innerHTML = '<div class="empty-state">No applications connected</div>';
        } else {
            nodeList.innerHTML = appNodes
                .sort((a, b) => a.name.localeCompare(b.name))
                .map(node => `
                    <div class="node-item ${node.id === this.selectedNode ? 'selected' : ''}"
                         onclick="app.selectNode('${node.id}')">
                        <div class="node-item-header">
                            <span class="node-name">${this.escapeHtml(node.name)}</span>
                            <span class="node-status ${node.online ? 'online' : ''}"></span>
                        </div>
                        <div class="node-meta">
                            ${node.hostname ? this.escapeHtml(node.hostname) : node.id}
                            ${node.chart_count ? ` - ${node.chart_count} charts` : ''}
                        </div>
                    </div>
                `).join('');
        }
    }

    filterNodes(query) {
        const items = document.querySelectorAll('.node-item, .k8s-cluster-item');
        const lowerQuery = query.toLowerCase();

        items.forEach(item => {
            const name = item.querySelector('.node-name, .k8s-cluster-name')?.textContent.toLowerCase() || '';
            const meta = item.querySelector('.node-meta, .k8s-cluster-meta')?.textContent.toLowerCase() || '';
            const visible = name.includes(lowerQuery) || meta.includes(lowerQuery);
            item.style.display = visible ? '' : 'none';
        });
    }

    async selectNode(nodeId) {
        this.selectedNode = nodeId;
        this.selectedK8sCluster = null;
        this.renderNodeList();

        const node = this.nodes.get(nodeId);
        if (!node) return;

        // Hide K8s dashboard, show standard dashboard
        document.getElementById('welcomeScreen').style.display = 'none';
        document.getElementById('k8sDashboard').style.display = 'none';
        document.getElementById('dashboard').style.display = 'block';
        document.getElementById('selectedNodeName').textContent = node.name;

        await this.loadCharts();
    }

    async selectK8sCluster(nodeId) {
        this.selectedK8sCluster = nodeId;
        this.selectedNode = null;
        this.renderNodeList();

        const node = this.nodes.get(nodeId);
        if (!node) return;

        // Show K8s dashboard, hide standard dashboard
        document.getElementById('welcomeScreen').style.display = 'none';
        document.getElementById('dashboard').style.display = 'none';
        document.getElementById('k8sDashboard').style.display = 'block';
        document.getElementById('k8sClusterName').textContent = node.name;

        await this.loadK8sCharts();
    }

    async loadCharts() {
        if (!this.selectedNode) return;

        const chartsGrid = document.getElementById('chartsGrid');
        chartsGrid.innerHTML = '<div class="chart-loading loading">Loading charts...</div>';

        try {
            const response = await fetch(`/api/v1/charts/${this.selectedNode}`);
            const charts = await response.json();

            if (!charts || charts.length === 0) {
                chartsGrid.innerHTML = '<div class="empty-state">No charts available</div>';
                return;
            }

            charts.sort((a, b) => (a.priority || 0) - (b.priority || 0));

            chartsGrid.innerHTML = '';
            this.charts.clear();

            for (const chart of charts) {
                this.createChartCard(chart, 'chartsGrid');
            }

            await this.refreshCharts();
        } catch (error) {
            chartsGrid.innerHTML = `<div class="chart-error">Failed to load charts: ${error.message}</div>`;
        }
    }

    async loadK8sCharts() {
        if (!this.selectedK8sCluster) return;

        const chartsGrid = document.getElementById('k8sChartsGrid');
        chartsGrid.innerHTML = '<div class="chart-loading loading">Loading Kubernetes metrics...</div>';

        try {
            const response = await fetch(`/api/v1/charts/${this.selectedK8sCluster}`);
            const charts = await response.json();

            if (!charts || charts.length === 0) {
                chartsGrid.innerHTML = '<div class="empty-state">No charts available</div>';
                return;
            }

            charts.sort((a, b) => (a.priority || 0) - (b.priority || 0));

            // Extract namespaces for filter
            this.updateK8sNamespaceFilter(charts);

            chartsGrid.innerHTML = '';
            this.k8sCharts.clear();

            // Group charts by family
            const families = new Map();
            for (const chart of charts) {
                const family = chart.family || 'other';
                if (!families.has(family)) {
                    families.set(family, []);
                }
                families.get(family).push(chart);
            }

            // Render by family
            const familyOrder = ['cluster', 'nodes', 'pods', 'deployments', 'storage'];
            const sortedFamilies = Array.from(families.keys()).sort((a, b) => {
                const aIdx = familyOrder.indexOf(a);
                const bIdx = familyOrder.indexOf(b);
                if (aIdx === -1 && bIdx === -1) return a.localeCompare(b);
                if (aIdx === -1) return 1;
                if (bIdx === -1) return -1;
                return aIdx - bIdx;
            });

            for (const family of sortedFamilies) {
                const familyCharts = families.get(family);

                // Check if any charts match the current filter
                const filteredCharts = familyCharts.filter(c => this.matchesK8sFilter(c));
                if (filteredCharts.length === 0) continue;

                // Add family header
                const header = document.createElement('div');
                header.className = 'chart-family-header';
                header.innerHTML = `<div class="chart-family-title">${this.formatFamilyName(family)}</div>`;
                chartsGrid.appendChild(header);

                // Add charts
                for (const chart of filteredCharts) {
                    this.createChartCard(chart, 'k8sChartsGrid', true);
                }
            }

            // Update overview cards
            this.updateK8sOverview(charts);

            await this.refreshK8sCharts();
        } catch (error) {
            chartsGrid.innerHTML = `<div class="chart-error">Failed to load charts: ${error.message}</div>`;
        }
    }

    matchesK8sFilter(chart) {
        // Filter by resource type
        if (this.k8sResourceFilter && chart.family !== this.k8sResourceFilter) {
            return false;
        }

        // Filter by namespace (check chart ID for namespace)
        if (this.k8sNamespaceFilter) {
            const chartId = chart.id || '';
            // K8s charts have namespace in the ID like k8s.pods.default.status
            if (!chartId.includes(`.${this.k8sNamespaceFilter}.`)) {
                // Allow cluster-level charts that don't have namespaces
                if (chart.family !== 'cluster' && chart.family !== 'nodes') {
                    return false;
                }
            }
        }

        return true;
    }

    updateK8sNamespaceFilter(charts) {
        const namespaces = new Set();

        for (const chart of charts) {
            const id = chart.id || '';
            // Extract namespace from chart ID like k8s.pods.default.status
            const match = id.match(/k8s\.\w+\.(\w+)\./);
            if (match && match[1] && !['cpu', 'memory', 'status', 'replicas', 'ready'].includes(match[1])) {
                namespaces.add(match[1]);
            }
        }

        const select = document.getElementById('k8sNamespace');
        const currentValue = select.value;

        select.innerHTML = '<option value="">All Namespaces</option>';
        Array.from(namespaces).sort().forEach(ns => {
            const option = document.createElement('option');
            option.value = ns;
            option.textContent = ns;
            select.appendChild(option);
        });

        select.value = currentValue;
    }

    updateK8sOverview(charts) {
        // Extract counts from charts
        let nodesCount = '-';
        let podsCount = '-';
        let deploymentsCount = '-';
        let servicesCount = '-';

        for (const chart of charts) {
            if (chart.id === 'k8s.nodes.status') {
                nodesCount = Object.keys(chart.dimensions || {}).length;
            }
            if (chart.id && chart.id.includes('.pods.') && chart.id.includes('.status')) {
                // Sum pods from status charts
                const dims = chart.dimensions || {};
                let total = 0;
                for (const dim of Object.values(dims)) {
                    total += dim.last_value || 0;
                }
                if (podsCount === '-') podsCount = 0;
                podsCount += total;
            }
            if (chart.id && chart.id.includes('.deployments.') && chart.id.includes('.replicas')) {
                if (deploymentsCount === '-') deploymentsCount = 0;
                deploymentsCount += Object.keys(chart.dimensions || {}).length;
            }
            if (chart.id === 'k8s.cluster.services') {
                const dims = chart.dimensions || {};
                for (const dim of Object.values(dims)) {
                    servicesCount = Math.round(dim.last_value || 0);
                }
            }
        }

        document.getElementById('k8sNodesCount').textContent = nodesCount;
        document.getElementById('k8sPodsCount').textContent = Math.round(podsCount) || '-';
        document.getElementById('k8sDeploymentsCount').textContent = deploymentsCount;
        document.getElementById('k8sServicesCount').textContent = servicesCount;
    }

    formatFamilyName(family) {
        const names = {
            'cluster': 'Cluster Overview',
            'nodes': 'Nodes',
            'pods': 'Pods',
            'deployments': 'Deployments',
            'storage': 'Storage',
            'cpu': 'CPU',
            'memory': 'Memory',
            'http': 'HTTP',
            'load': 'Load',
            'disk': 'Disk',
            'network': 'Network',
        };
        return names[family] || family.charAt(0).toUpperCase() + family.slice(1);
    }

    createChartCard(chart, gridId, isK8s = false) {
        const chartsGrid = document.getElementById(gridId);
        const card = document.createElement('div');
        card.className = 'chart-card';
        card.id = `chart-${chart.id}`;

        const dims = Object.values(chart.dimensions || {});
        const legendHtml = dims.slice(0, 10).map((dim, i) => `
            <div class="legend-item">
                <span class="legend-color" style="background: ${this.colors[i % this.colors.length]}"></span>
                <span class="legend-name">${this.escapeHtml(dim.name || dim.id)}</span>
                <span class="legend-value" id="legend-${chart.id}-${dim.id}">--</span>
            </div>
        `).join('');

        const moreCount = dims.length > 10 ? `<span class="legend-item">+${dims.length - 10} more</span>` : '';

        card.innerHTML = `
            <div class="chart-header">
                <span class="chart-title">${this.escapeHtml(chart.title || chart.name || chart.id)}</span>
                <span class="chart-units">${this.escapeHtml(chart.units || '')}</span>
            </div>
            <div class="chart-body">
                <canvas class="chart-canvas" id="canvas-${chart.id}"></canvas>
            </div>
            <div class="chart-legend">${legendHtml}${moreCount}</div>
        `;

        chartsGrid.appendChild(card);

        const chartStore = isK8s ? this.k8sCharts : this.charts;
        chartStore.set(chart.id, {
            chart: chart,
            canvas: card.querySelector(`#canvas-${chart.id}`),
            data: null
        });
    }

    async refreshCharts() {
        if (!this.selectedNode) return;

        const now = Date.now();
        const after = now - this.timeRange;

        for (const [chartId, chartInfo] of this.charts) {
            try {
                const response = await fetch(
                    `/api/v1/data/${this.selectedNode}/${chartId}?after=${after}&before=${now}&points=300`
                );
                const data = await response.json();
                chartInfo.data = data;
                this.renderChart(chartId, this.charts);
            } catch (error) {
                console.error(`Failed to load data for chart ${chartId}:`, error);
            }
        }
    }

    async refreshK8sCharts() {
        if (!this.selectedK8sCluster) return;

        const now = Date.now();
        const after = now - this.k8sTimeRange;

        for (const [chartId, chartInfo] of this.k8sCharts) {
            try {
                const response = await fetch(
                    `/api/v1/data/${this.selectedK8sCluster}/${chartId}?after=${after}&before=${now}&points=300`
                );
                const data = await response.json();
                chartInfo.data = data;
                this.renderChart(chartId, this.k8sCharts);
            } catch (error) {
                console.error(`Failed to load data for chart ${chartId}:`, error);
            }
        }
    }

    updateChartsRealtime(payload) {
        this.updateChartsRealtimeInternal(payload, this.charts);
    }

    updateK8sChartsRealtime(payload) {
        this.updateChartsRealtimeInternal(payload, this.k8sCharts);
    }

    updateChartsRealtimeInternal(payload, chartStore) {
        for (const chartPush of payload.charts) {
            const chartInfo = chartStore.get(chartPush.id);
            if (!chartInfo || !chartInfo.data) continue;

            const ts = payload.timestamp;
            chartInfo.data.labels.push(ts);

            for (let i = 0; i < chartInfo.data.data.length; i++) {
                const dimId = Object.keys(chartInfo.chart.dimensions)[i];
                const dimPush = chartPush.dimensions.find(d => d.id === dimId);
                chartInfo.data.data[i].push(dimPush ? dimPush.value : 0);
            }

            const cutoff = Date.now() - (chartStore === this.k8sCharts ? this.k8sTimeRange : this.timeRange);
            while (chartInfo.data.labels.length > 0 && chartInfo.data.labels[0] < cutoff) {
                chartInfo.data.labels.shift();
                for (const dimData of chartInfo.data.data) {
                    dimData.shift();
                }
            }

            for (const dim of chartPush.dimensions) {
                const legendEl = document.getElementById(`legend-${chartPush.id}-${dim.id}`);
                if (legendEl) {
                    legendEl.textContent = this.formatValue(dim.value, chartInfo.chart.units);
                }
            }

            this.renderChart(chartPush.id, chartStore);
        }
    }

    renderChart(chartId, chartStore) {
        const chartInfo = chartStore.get(chartId);
        if (!chartInfo || !chartInfo.data) return;

        const canvas = chartInfo.canvas;
        if (!canvas) return;

        const ctx = canvas.getContext('2d');
        const data = chartInfo.data;

        const rect = canvas.parentElement.getBoundingClientRect();
        canvas.width = rect.width * window.devicePixelRatio;
        canvas.height = rect.height * window.devicePixelRatio;
        ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

        const width = rect.width;
        const height = rect.height;
        const padding = { top: 10, right: 10, bottom: 25, left: 50 };
        const chartWidth = width - padding.left - padding.right;
        const chartHeight = height - padding.top - padding.bottom;

        ctx.clearRect(0, 0, width, height);

        if (!data.labels || data.labels.length === 0) {
            ctx.fillStyle = getComputedStyle(document.documentElement)
                .getPropertyValue('--text-secondary');
            ctx.font = '12px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No data', width / 2, height / 2);
            return;
        }

        let minVal = Infinity;
        let maxVal = -Infinity;
        for (const dimData of data.data) {
            for (const val of dimData) {
                if (val < minVal) minVal = val;
                if (val > maxVal) maxVal = val;
            }
        }

        const range = maxVal - minVal || 1;
        minVal = minVal - range * 0.1;
        maxVal = maxVal + range * 0.1;
        if (minVal > 0 && minVal < range * 0.2) minVal = 0;

        ctx.strokeStyle = getComputedStyle(document.documentElement)
            .getPropertyValue('--border-color');
        ctx.lineWidth = 0.5;
        ctx.beginPath();

        for (let i = 0; i <= 4; i++) {
            const y = padding.top + (chartHeight * i / 4);
            ctx.moveTo(padding.left, y);
            ctx.lineTo(width - padding.right, y);
        }
        ctx.stroke();

        ctx.fillStyle = getComputedStyle(document.documentElement)
            .getPropertyValue('--text-secondary');
        ctx.font = '10px sans-serif';
        ctx.textAlign = 'right';
        ctx.textBaseline = 'middle';

        for (let i = 0; i <= 4; i++) {
            const y = padding.top + (chartHeight * i / 4);
            const value = maxVal - (range * i / 4);
            ctx.fillText(this.formatValue(value, chartInfo.chart.units), padding.left - 5, y);
        }

        ctx.textAlign = 'center';
        ctx.textBaseline = 'top';
        const timeLabels = 5;
        for (let i = 0; i <= timeLabels; i++) {
            const x = padding.left + (chartWidth * i / timeLabels);
            const idx = Math.floor((data.labels.length - 1) * i / timeLabels);
            if (idx >= 0 && idx < data.labels.length) {
                const date = new Date(data.labels[idx]);
                ctx.fillText(this.formatTime(date), x, height - padding.bottom + 5);
            }
        }

        const chartType = chartInfo.chart.chart_type || 'line';

        for (let dimIdx = 0; dimIdx < data.data.length; dimIdx++) {
            const dimData = data.data[dimIdx];
            const color = this.colors[dimIdx % this.colors.length];

            ctx.strokeStyle = color;
            ctx.fillStyle = color + '40';
            ctx.lineWidth = 2;
            ctx.lineJoin = 'round';
            ctx.lineCap = 'round';

            ctx.beginPath();
            let started = false;

            for (let i = 0; i < dimData.length; i++) {
                const x = padding.left + (chartWidth * i / (dimData.length - 1 || 1));
                const y = padding.top + chartHeight * (1 - (dimData[i] - minVal) / (maxVal - minVal));

                if (!started) {
                    ctx.moveTo(x, y);
                    started = true;
                } else {
                    ctx.lineTo(x, y);
                }
            }

            if (chartType === 'area' || chartType === 'stacked') {
                const lastX = padding.left + chartWidth;
                const baseY = padding.top + chartHeight;
                ctx.lineTo(lastX, baseY);
                ctx.lineTo(padding.left, baseY);
                ctx.closePath();
                ctx.fill();

                ctx.beginPath();
                started = false;
                for (let i = 0; i < dimData.length; i++) {
                    const x = padding.left + (chartWidth * i / (dimData.length - 1 || 1));
                    const y = padding.top + chartHeight * (1 - (dimData[i] - minVal) / (maxVal - minVal));
                    if (!started) {
                        ctx.moveTo(x, y);
                        started = true;
                    } else {
                        ctx.lineTo(x, y);
                    }
                }
            }

            ctx.stroke();

            const dims = Object.keys(chartInfo.chart.dimensions);
            if (dimIdx < dims.length && dimData.length > 0) {
                const dimId = dims[dimIdx];
                const lastValue = dimData[dimData.length - 1];
                const legendEl = document.getElementById(`legend-${chartInfo.chart.id}-${dimId}`);
                if (legendEl) {
                    legendEl.textContent = this.formatValue(lastValue, chartInfo.chart.units);
                }
            }
        }
    }

    formatValue(value, units) {
        if (value === null || value === undefined) return '--';

        let formatted;
        const abs = Math.abs(value);

        if (abs >= 1000000000) {
            formatted = (value / 1000000000).toFixed(2) + 'G';
        } else if (abs >= 1000000) {
            formatted = (value / 1000000).toFixed(2) + 'M';
        } else if (abs >= 1000) {
            formatted = (value / 1000).toFixed(2) + 'K';
        } else if (abs >= 1) {
            formatted = value.toFixed(2);
        } else if (abs >= 0.01) {
            formatted = value.toFixed(3);
        } else if (abs === 0) {
            formatted = '0';
        } else {
            formatted = value.toExponential(2);
        }

        return units ? `${formatted} ${units}` : formatted;
    }

    formatTime(date) {
        const h = date.getHours().toString().padStart(2, '0');
        const m = date.getMinutes().toString().padStart(2, '0');
        const s = date.getSeconds().toString().padStart(2, '0');
        return `${h}:${m}:${s}`;
    }

    escapeHtml(text) {
        if (!text) return '';
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    toggleTheme() {
        this.theme = this.theme === 'dark' ? 'light' : 'dark';
        localStorage.setItem('theme', this.theme);

        if (this.theme === 'light') {
            document.documentElement.setAttribute('data-theme', 'light');
        } else {
            document.documentElement.removeAttribute('data-theme');
        }

        for (const chartId of this.charts.keys()) {
            this.renderChart(chartId, this.charts);
        }
        for (const chartId of this.k8sCharts.keys()) {
            this.renderChart(chartId, this.k8sCharts);
        }
    }

    changeTimeRange() {
        const select = document.getElementById('timeRange');
        this.timeRange = parseInt(select.value);
        this.refreshCharts();
    }

    changeK8sTimeRange() {
        const select = document.getElementById('k8sTimeRange');
        this.k8sTimeRange = parseInt(select.value);
        this.refreshK8sCharts();
    }

    filterK8sNamespace() {
        const select = document.getElementById('k8sNamespace');
        this.k8sNamespaceFilter = select.value;
        this.loadK8sCharts();
    }

    filterK8sResource() {
        const select = document.getElementById('k8sResourceType');
        this.k8sResourceFilter = select.value;
        this.loadK8sCharts();
    }
}

// Initialize app
const app = new NetdataAltUI();
