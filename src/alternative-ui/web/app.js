// Netdata Alternative UI - Dashboard Application

class NetdataAltUI {
    constructor() {
        this.nodes = new Map();
        this.selectedNode = null;
        this.ws = null;
        this.charts = new Map();
        this.timeRange = 1800000; // 30 minutes default
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
            // Reconnect after delay
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
                // Initial node list
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
                // Update charts if this is the selected node
                if (this.selectedNode && msg.payload.node_id === this.selectedNode) {
                    this.updateChartsRealtime(msg.payload);
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

    renderNodeList() {
        const container = document.getElementById('nodeList');
        const count = document.getElementById('nodeCount');

        count.textContent = `${this.nodes.size} node${this.nodes.size !== 1 ? 's' : ''}`;

        if (this.nodes.size === 0) {
            container.innerHTML = '<div class="empty-state">No nodes connected</div>';
            return;
        }

        const html = Array.from(this.nodes.values())
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

        container.innerHTML = html;
    }

    filterNodes(query) {
        const items = document.querySelectorAll('.node-item');
        const lowerQuery = query.toLowerCase();

        items.forEach(item => {
            const name = item.querySelector('.node-name').textContent.toLowerCase();
            const meta = item.querySelector('.node-meta').textContent.toLowerCase();
            const visible = name.includes(lowerQuery) || meta.includes(lowerQuery);
            item.style.display = visible ? '' : 'none';
        });
    }

    async selectNode(nodeId) {
        this.selectedNode = nodeId;
        this.renderNodeList();

        const node = this.nodes.get(nodeId);
        if (!node) return;

        // Show dashboard
        document.getElementById('welcomeScreen').style.display = 'none';
        document.getElementById('dashboard').style.display = 'block';
        document.getElementById('selectedNodeName').textContent = node.name;

        // Fetch charts
        await this.loadCharts();
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

            // Sort charts by priority
            charts.sort((a, b) => (a.priority || 0) - (b.priority || 0));

            chartsGrid.innerHTML = '';
            this.charts.clear();

            for (const chart of charts) {
                this.createChartCard(chart);
            }

            // Load data for all charts
            await this.refreshCharts();
        } catch (error) {
            chartsGrid.innerHTML = `<div class="chart-error">Failed to load charts: ${error.message}</div>`;
        }
    }

    createChartCard(chart) {
        const chartsGrid = document.getElementById('chartsGrid');
        const card = document.createElement('div');
        card.className = 'chart-card';
        card.id = `chart-${chart.id}`;

        const dims = Object.values(chart.dimensions || {});
        const legendHtml = dims.map((dim, i) => `
            <div class="legend-item">
                <span class="legend-color" style="background: ${this.colors[i % this.colors.length]}"></span>
                <span class="legend-name">${this.escapeHtml(dim.name || dim.id)}</span>
                <span class="legend-value" id="legend-${chart.id}-${dim.id}">--</span>
            </div>
        `).join('');

        card.innerHTML = `
            <div class="chart-header">
                <span class="chart-title">${this.escapeHtml(chart.title || chart.name || chart.id)}</span>
                <span class="chart-units">${this.escapeHtml(chart.units || '')}</span>
            </div>
            <div class="chart-body">
                <canvas class="chart-canvas" id="canvas-${chart.id}"></canvas>
            </div>
            <div class="chart-legend">${legendHtml}</div>
        `;

        chartsGrid.appendChild(card);

        // Initialize chart renderer
        this.charts.set(chart.id, {
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
                this.renderChart(chartId);
            } catch (error) {
                console.error(`Failed to load data for chart ${chartId}:`, error);
            }
        }
    }

    updateChartsRealtime(payload) {
        // Update chart data in real-time
        for (const chartPush of payload.charts) {
            const chartInfo = this.charts.get(chartPush.id);
            if (!chartInfo || !chartInfo.data) continue;

            // Add new data point
            const ts = payload.timestamp;
            chartInfo.data.labels.push(ts);

            // Update dimension values
            for (let i = 0; i < chartInfo.data.data.length; i++) {
                const dimId = Object.keys(chartInfo.chart.dimensions)[i];
                const dimPush = chartPush.dimensions.find(d => d.id === dimId);
                chartInfo.data.data[i].push(dimPush ? dimPush.value : 0);
            }

            // Remove old data points outside time range
            const cutoff = Date.now() - this.timeRange;
            while (chartInfo.data.labels.length > 0 && chartInfo.data.labels[0] < cutoff) {
                chartInfo.data.labels.shift();
                for (const dimData of chartInfo.data.data) {
                    dimData.shift();
                }
            }

            // Update legend values
            for (const dim of chartPush.dimensions) {
                const legendEl = document.getElementById(`legend-${chartPush.id}-${dim.id}`);
                if (legendEl) {
                    legendEl.textContent = this.formatValue(dim.value, chartInfo.chart.units);
                }
            }

            this.renderChart(chartPush.id);
        }
    }

    renderChart(chartId) {
        const chartInfo = this.charts.get(chartId);
        if (!chartInfo || !chartInfo.data) return;

        const canvas = chartInfo.canvas;
        const ctx = canvas.getContext('2d');
        const data = chartInfo.data;

        // Set canvas size
        const rect = canvas.parentElement.getBoundingClientRect();
        canvas.width = rect.width * window.devicePixelRatio;
        canvas.height = rect.height * window.devicePixelRatio;
        ctx.scale(window.devicePixelRatio, window.devicePixelRatio);

        const width = rect.width;
        const height = rect.height;
        const padding = { top: 10, right: 10, bottom: 25, left: 50 };
        const chartWidth = width - padding.left - padding.right;
        const chartHeight = height - padding.top - padding.bottom;

        // Clear canvas
        ctx.clearRect(0, 0, width, height);

        if (!data.labels || data.labels.length === 0) {
            ctx.fillStyle = getComputedStyle(document.documentElement)
                .getPropertyValue('--text-secondary');
            ctx.font = '12px sans-serif';
            ctx.textAlign = 'center';
            ctx.fillText('No data', width / 2, height / 2);
            return;
        }

        // Calculate value range
        let minVal = Infinity;
        let maxVal = -Infinity;
        for (const dimData of data.data) {
            for (const val of dimData) {
                if (val < minVal) minVal = val;
                if (val > maxVal) maxVal = val;
            }
        }

        // Add some padding to the range
        const range = maxVal - minVal || 1;
        minVal = minVal - range * 0.1;
        maxVal = maxVal + range * 0.1;
        if (minVal > 0 && minVal < range * 0.2) minVal = 0;

        // Draw grid lines
        ctx.strokeStyle = getComputedStyle(document.documentElement)
            .getPropertyValue('--border-color');
        ctx.lineWidth = 0.5;
        ctx.beginPath();

        // Horizontal grid lines
        for (let i = 0; i <= 4; i++) {
            const y = padding.top + (chartHeight * i / 4);
            ctx.moveTo(padding.left, y);
            ctx.lineTo(width - padding.right, y);
        }
        ctx.stroke();

        // Draw Y-axis labels
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

        // Draw X-axis labels
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

        // Draw lines
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
                // Fill area under the line
                const lastX = padding.left + chartWidth;
                const baseY = padding.top + chartHeight;
                ctx.lineTo(lastX, baseY);
                ctx.lineTo(padding.left, baseY);
                ctx.closePath();
                ctx.fill();

                // Redraw the line on top
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

            // Update legend
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

        // Re-render all charts
        for (const chartId of this.charts.keys()) {
            this.renderChart(chartId);
        }
    }

    changeTimeRange() {
        const select = document.getElementById('timeRange');
        this.timeRange = parseInt(select.value);
        this.refreshCharts();
    }
}

// Initialize app
const app = new NetdataAltUI();
