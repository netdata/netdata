import WebSocket from 'ws';
export class WebSocketTransport {
    url;
    headers;
    ws;
    messageHandlers = new Set();
    closeHandlers = new Set();
    errorHandlers = new Set();
    constructor(url, headers) {
        this.url = url;
        this.headers = headers;
        this.ws = new WebSocket(url, { headers });
        this.setupEventHandlers();
    }
    setupEventHandlers() {
        this.ws.on('message', (data) => {
            try {
                let text = '';
                if (typeof data === 'string')
                    text = data;
                else if (Buffer.isBuffer(data))
                    text = data.toString('utf8');
                else if (Array.isArray(data))
                    text = Buffer.concat(data).toString('utf8');
                const message = JSON.parse(text);
                this.messageHandlers.forEach((handler) => { handler(message); });
            }
            catch (error) {
                const err = new Error(`Failed to parse WebSocket message: ${error instanceof Error ? error.message : String(error)}`);
                this.errorHandlers.forEach((handler) => { handler(err); });
            }
        });
        this.ws.on('close', () => {
            this.closeHandlers.forEach((handler) => { handler(); });
        });
        this.ws.on('error', (error) => {
            const err = new Error(`WebSocket error: ${error instanceof Error ? error.message : String(error)}`);
            this.errorHandlers.forEach((handler) => { handler(err); });
        });
    }
    async start() {
        if (this.ws.readyState === WebSocket.OPEN)
            return;
        await new Promise((resolve, reject) => {
            const onError = (error) => {
                this.ws.off('open', onOpen);
                reject(new Error(`Failed to connect to WebSocket ${this.url}: ${error.message}`));
            };
            const onOpen = () => {
                this.ws.off('error', onError);
                resolve();
            };
            this.ws.once('open', onOpen);
            this.ws.once('error', onError);
        });
    }
    async send(message) {
        if (this.ws.readyState !== WebSocket.OPEN)
            throw new Error('WebSocket is not connected');
        await new Promise((resolve, reject) => {
            this.ws.send(JSON.stringify(message), (error) => {
                if (error != null)
                    reject(new Error(`Failed to send WebSocket message: ${error.message}`));
                else
                    resolve();
            });
        });
    }
    async close() {
        if (this.ws.readyState === WebSocket.CLOSED)
            return;
        await new Promise((resolve) => {
            if (this.ws.readyState === WebSocket.OPEN)
                this.ws.close();
            const onClose = () => { resolve(); };
            this.ws.once('close', onClose);
            setTimeout(() => {
                if (this.ws.readyState !== WebSocket.CLOSED) {
                    this.ws.terminate();
                    resolve();
                }
            }, 5000);
        });
    }
    onMessage(handler) {
        this.messageHandlers.add(handler);
    }
    onClose(handler) {
        this.closeHandlers.add(handler);
    }
    onError(handler) {
        this.errorHandlers.add(handler);
    }
}
export async function createWebSocketTransport(url, headers) {
    const t = new WebSocketTransport(url, headers);
    await t.start();
    return t;
}
