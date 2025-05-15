#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import sys
import asyncio
import websockets

async def bridge(uri):
    try:
        async with websockets.connect(uri, compression='deflate') as ws:
            async def stdin_to_ws():
                loop = asyncio.get_running_loop()
                reader = asyncio.StreamReader()
                await loop.connect_read_pipe(lambda: asyncio.StreamReaderProtocol(reader), sys.stdin)
                while True:
                    line = await reader.readline()
                    if not line:
                        break
                    await ws.send(line.decode().strip())

            async def ws_to_stdout():
                async for message in ws:
                    print(message, flush=True)

            await asyncio.gather(stdin_to_ws(), ws_to_stdout())
    except Exception as e:
        print(f"Connection error: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: nd_mcp.py ws://host/path", file=sys.stderr)
        sys.exit(1)
    asyncio.run(bridge(sys.argv[1]))
