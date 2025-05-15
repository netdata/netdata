#!/usr/bin/env node

const WebSocket = require('ws');

if (process.argv.length !== 3) {
    console.error("Usage: node nd_mcp.js ws://host/path");
    process.exit(1);
}

const ws = new WebSocket(process.argv[2], {
    perMessageDeflate: true
});

ws.on('open', () => {
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (data) => {
        data.split(/\r?\n/).forEach(line => {
            if (line.trim() !== '') ws.send(line);
        });
    });
});

ws.on('message', (message) => {
    process.stdout.write(message + "\n");
});

ws.on('close', () => {
    console.error("WebSocket closed");
    process.exit(1);
});

ws.on('error', (err) => {
    console.error("WebSocket error:", err.message);
    process.exit(1);
});
