#!/usr/bin/env node

const WebSocket = require('ws');
const path = require('path');

// Get program name for logs
const PROGRAM_NAME = path.basename(process.argv[1] || 'nd-mcp-nodejs');

if (process.argv.length !== 3) {
    console.error(`${PROGRAM_NAME}: Usage: ${PROGRAM_NAME} ws://host/path`);
    process.exit(1);
}

const ws = new WebSocket(process.argv[2], {
    perMessageDeflate: true,
    maxPayload: 16 * 1024 * 1024 // 16MB to match browser limits
});

ws.on('open', () => {
    process.stdin.setEncoding('utf8');
    process.stdin.on('data', (data) => {
        try {
            data.split(/\r?\n/).forEach(line => {
                if (line.trim() !== '') ws.send(line);
            });
        } catch (err) {
            console.error(`${PROGRAM_NAME}: ERROR: Failed to send data: ${err.message}`);
            process.exit(1);
        }
    });
});

process.stdin.on('error', (err) => {
    console.error(`${PROGRAM_NAME}: ERROR: stdin: ${err.message}`);
    process.exit(1);
});

ws.on('message', (message) => {
    process.stdout.write(message + "\n");
});

ws.on('close', (code, reason) => {
    console.error(`${PROGRAM_NAME}: WebSocket closed with code ${code}${reason ? ': ' + reason : ''}`);
    process.exit(1);
});

ws.on('error', (err) => {
    console.error(`${PROGRAM_NAME}: ERROR: WebSocket error: ${err.message}`);
    process.exit(1);
});
