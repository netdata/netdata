#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import sys
import asyncio
import websockets
import os.path

# Get program name for logs
PROGRAM_NAME = os.path.basename(sys.argv[0]) if len(sys.argv) > 0 else "nd-mcp-python"

async def bridge(uri):
    try:
        async with websockets.connect(uri, compression='deflate', max_size=16*1024*1024) as ws:
            async def stdin_to_ws():
                try:
                    loop = asyncio.get_running_loop()
                    reader = asyncio.StreamReader()
                    await loop.connect_read_pipe(lambda: asyncio.StreamReaderProtocol(reader), sys.stdin)
                    while True:
                        line = await reader.readline()
                        if not line:
                            break
                        await ws.send(line.decode().strip())
                except Exception as e:
                    print(f"{PROGRAM_NAME}: ERROR: stdin_to_ws: {e}", file=sys.stderr)
                    raise

            async def ws_to_stdout():
                try:
                    async for message in ws:
                        print(message, flush=True)
                except Exception as e:
                    print(f"{PROGRAM_NAME}: ERROR: ws_to_stdout: {e}", file=sys.stderr)
                    raise

            await asyncio.gather(stdin_to_ws(), ws_to_stdout())
    except Exception as e:
        print(f"{PROGRAM_NAME}: ERROR: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print(f"{PROGRAM_NAME}: Usage: {PROGRAM_NAME} ws://host/path", file=sys.stderr)
        sys.exit(1)
    asyncio.run(bridge(sys.argv[1]))
