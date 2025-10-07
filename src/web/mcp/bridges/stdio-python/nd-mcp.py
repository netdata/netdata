#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import sys
import asyncio
import websockets
import os
import random
import time
import signal
import json

# Get program name for logs
PROGRAM_NAME = os.path.basename(sys.argv[0]) if len(sys.argv) > 0 else "nd-mcp-python"

# Global flag to track if we're exiting due to a stdin issue
STDIN_ERROR = False

# Timeout for connection attempts when a message with ID is waiting
CONNECTION_TIMEOUT = 5  # seconds

# Parse JSON-RPC message and extract ID if present
def parse_jsonrpc_message(message):
    try:
        data = json.loads(message)
        if isinstance(data, dict) and "jsonrpc" in data and data.get("jsonrpc") == "2.0":
            return data.get("id"), data.get("method")
        return None, None
    except json.JSONDecodeError:
        return None, None

# Create a JSON-RPC error response
def create_jsonrpc_error(id, code, message, data=None):
    response = {
        "jsonrpc": "2.0",
        "id": id,
        "error": {
            "code": code,
            "message": message
        }
    }
    if data is not None:
        response["error"]["data"] = data
    return json.dumps(response)

async def connect_with_backoff(uri, bearer_token):
    max_delay = 60  # Maximum delay between reconnections in seconds
    base_delay = 1   # Initial delay in seconds
    retry_count = 0
    
    # Message queue for storing messages during disconnections
    stdin_queue = asyncio.Queue()
    
    # Dictionary to track pending requests with their IDs and timers
    pending_requests = {}
    
    # Set up stdin reader once
    async def read_stdin():
        global STDIN_ERROR
        try:
            loop = asyncio.get_running_loop()
            reader = asyncio.StreamReader()
            await loop.connect_read_pipe(lambda: asyncio.StreamReaderProtocol(reader), sys.stdin)
            
            while True:
                line = await reader.readline()
                if not line:
                    print(f"{PROGRAM_NAME}: End of stdin, exiting...", file=sys.stderr)
                    STDIN_ERROR = True
                    # Signal the main event loop to exit
                    loop.stop()
                    return
                
                # Process the received line
                line_text = line.decode().strip()
                if not line_text:
                    continue

                # Parse JSON-RPC message
                msg_id, msg_method = parse_jsonrpc_message(line_text)
                
                # Store message in queue
                await stdin_queue.put((line_text, msg_id, msg_method))
                
                # If we're disconnected, check if we need to respond with error
                if retry_count > 0:
                    print(f"{PROGRAM_NAME}: Received stdin data, attempting immediate reconnection", file=sys.stderr)
                    retry_event.set()
                    
                    if msg_id is not None:
                        # Set timer for this request
                        pending_requests[msg_id] = {
                            "message": line_text,
                            "timer": asyncio.create_task(handle_request_timeout(msg_id, CONNECTION_TIMEOUT))
                        }
                        
        except Exception as e:
            print(f"{PROGRAM_NAME}: ERROR: stdin reader: {e}", file=sys.stderr)
            STDIN_ERROR = True
            # Signal the main event loop to exit
            loop.stop()
            return
    
    # Handler for request timeout - ONLY for connection establishment
    async def handle_request_timeout(msg_id, timeout):
        await asyncio.sleep(timeout)
        
        # If we're still disconnected and the request is still pending
        if retry_count > 0 and msg_id in pending_requests:
            print(f"{PROGRAM_NAME}: Connection timeout for request ID {msg_id}, sending error response", file=sys.stderr)
            
            # Create and send error response
            error_response = create_jsonrpc_error(
                msg_id,
                -32000,  # Server error code
                "MCP server connection failed",
                {"details": "Could not establish connection to Netdata within timeout period"}
            )
            print(error_response, flush=True)
            
            # Mark this message as timed out
            if msg_id in pending_requests:
                pending_requests[msg_id]["timed_out"] = True
                
            # We'll keep it in pending_requests so we don't resend it, but mark it as handled
            print(f"{PROGRAM_NAME}: Marked request ID {msg_id} as timed out - it will not be sent to server", file=sys.stderr)
        elif msg_id in pending_requests:
            # Connection was established before timeout, just clean up the timer
            print(f"{PROGRAM_NAME}: Connection established before timeout for request ID {msg_id}", file=sys.stderr)
    
    # Create an event for signaling reconnection
    retry_event = asyncio.Event()
    
    # Start reading stdin in the background
    stdin_task = asyncio.create_task(read_stdin())
    
    while True:
        if STDIN_ERROR:
            print(f"{PROGRAM_NAME}: Stdin error detected, exiting", file=sys.stderr)
            return
            
        try:
            # Calculate backoff delay with jitter
            delay = min(max_delay, base_delay * (2 ** retry_count) * (0.5 + random.random()))
            
            if retry_count > 0:
                print(f"{PROGRAM_NAME}: Reconnecting in {delay:.1f} seconds (attempt {retry_count+1})...", file=sys.stderr)
                
                # Create a wait task, but also break on retry_event being set
                try:
                    # Wait for the delay or until retry_event is set
                    wait_task = asyncio.create_task(asyncio.sleep(delay))
                    retry_task = asyncio.create_task(retry_event.wait())
                    
                    done, pending = await asyncio.wait(
                        [wait_task, retry_task],
                        return_when=asyncio.FIRST_COMPLETED
                    )
                    
                    # Cancel the pending task
                    for task in pending:
                        task.cancel()
                        
                    # Clear the event
                    retry_event.clear()
                    
                except asyncio.CancelledError:
                    pass
                    
            print(f"{PROGRAM_NAME}: Connecting to {uri}...", file=sys.stderr)

            try:
                # Connect with timeout
                # In newer versions of websockets, connect() is already awaitable
                connect_kwargs = {
                    "compression": 'deflate',
                    "max_size": 16*1024*1024,
                    "ping_interval": 30,
                    "ping_timeout": 10,
                    "close_timeout": 5
                }

                if bearer_token:
                    connect_kwargs["extra_headers"] = {
                        "Authorization": f"Bearer {bearer_token}"
                    }

                ws = await asyncio.wait_for(
                    websockets.connect(
                        uri,
                        **connect_kwargs
                    ),
                    timeout=15  # 15 second timeout
                )
            except asyncio.TimeoutError:
                raise Exception("Connection timeout")
                
            print(f"{PROGRAM_NAME}: Connected", file=sys.stderr)
            retry_count = 0  # Reset retry counter on successful connection
            
            # Clear all pending request timers
            for req_id, req_data in list(pending_requests.items()):
                if "timer" in req_data and not req_data["timer"].done():
                    req_data["timer"].cancel()
            
            # We don't clear pending_requests entirely here, we just clear the timers
            # and keep tracking the requests until we get responses
            
            # Memory management: Limit the size of pending_requests to avoid memory leaks
            if len(pending_requests) > 1000:
                print(f"{PROGRAM_NAME}: Too many pending requests ({len(pending_requests)}), cleaning up", file=sys.stderr)
                pending_requests.clear() # In extreme case, clear everything
            
            # Processor for stdin messages
            async def process_stdin():
                try:
                    while True:
                        # Get message from queue
                        line_data = await stdin_queue.get()
                        line, msg_id, _ = line_data
                        
                        # Skip messages that have already timed out and received error responses
                        if msg_id is not None and msg_id in pending_requests and pending_requests[msg_id].get("timed_out", False):
                            print(f"{PROGRAM_NAME}: Skipping previously timed-out request with ID {msg_id}", file=sys.stderr)
                            stdin_queue.task_done()
                            continue
                        
                        # If this is a request with an ID, track it (but no timeout since we're connected)
                        if msg_id is not None and msg_id not in pending_requests:
                            pending_requests[msg_id] = {
                                "message": line,
                                "sent_time": time.time()
                            }
                        
                        try:
                            # Send to WebSocket
                            await ws.send(line)
                            stdin_queue.task_done()
                        except websockets.exceptions.ConnectionClosed:
                            # Put the message back in the queue, unless it already timed out
                            if not (msg_id is not None and msg_id in pending_requests and pending_requests[msg_id].get("timed_out", False)):
                                await stdin_queue.put(line_data)
                            # Re-raise to trigger reconnection
                            raise
                except Exception as e:
                    # In newer websockets versions, we need to check for close attribute differently
                    try:
                        # Check if the connection is still open
                        if hasattr(ws, 'closed') and not ws.closed:
                            print(f"{PROGRAM_NAME}: ERROR: stdin processor: {e}", file=sys.stderr)
                        elif hasattr(ws, 'protocol') and not ws.protocol.closed:
                            print(f"{PROGRAM_NAME}: ERROR: stdin processor: {e}", file=sys.stderr)
                        else:
                            print(f"{PROGRAM_NAME}: Websocket connection closed: {e}", file=sys.stderr)
                    except:
                        # If we can't check closed state, just log the error
                        print(f"{PROGRAM_NAME}: ERROR: stdin processor: {e}", file=sys.stderr)
                    # Don't propagate the exception, let the connection close
                    # and reconnection will be triggered
            
            # Processor for WebSocket messages
            async def process_websocket():
                try:
                    async for message in ws:
                        # Forward WebSocket messages to stdout
                        print(message, flush=True)
                        
                        # Check if this is a response to a tracked request
                        try:
                            data = json.loads(message)
                            if isinstance(data, dict) and "jsonrpc" in data and data.get("jsonrpc") == "2.0" and "id" in data:
                                msg_id = data.get("id")
                                if msg_id in pending_requests:
                                    del pending_requests[msg_id]
                        except:
                            pass
                        
                        # Memory management: Limit the size of stdin_queue to avoid memory leaks
                        if stdin_queue.qsize() > 1000:
                            print(f"{PROGRAM_NAME}: WARNING: Very large stdin queue ({stdin_queue.qsize()}), this may indicate a problem", file=sys.stderr)
                except Exception as e:
                    # In newer websockets versions, we need to check for close attribute differently
                    try:
                        # Check if the connection is still open
                        if hasattr(ws, 'closed') and not ws.closed:
                            print(f"{PROGRAM_NAME}: ERROR: websocket processor: {e}", file=sys.stderr)
                        elif hasattr(ws, 'protocol') and not ws.protocol.closed:
                            print(f"{PROGRAM_NAME}: ERROR: websocket processor: {e}", file=sys.stderr)
                        else:
                            print(f"{PROGRAM_NAME}: Websocket connection closed: {e}", file=sys.stderr)
                    except:
                        # If we can't check closed state, just log the error
                        print(f"{PROGRAM_NAME}: ERROR: websocket processor: {e}", file=sys.stderr)
                    # Don't propagate the exception, let the connection close
                    # and reconnection will be triggered
            
            # Run both tasks concurrently
            stdin_processor = asyncio.create_task(process_stdin())
            ws_processor = asyncio.create_task(process_websocket())
            
            # Wait for either task to complete (which means a failure)
            done, pending = await asyncio.wait(
                [stdin_processor, ws_processor],
                return_when=asyncio.FIRST_COMPLETED
            )
            
            # Cancel the pending task
            for task in pending:
                task.cancel()
                try:
                    await task
                except asyncio.CancelledError:
                    pass
            
            # Ensure WebSocket is closed
            try:
                # Check if websocket is already closed
                is_closed = False
                if hasattr(ws, 'closed'):
                    is_closed = ws.closed
                elif hasattr(ws, 'protocol'):
                    is_closed = ws.protocol.closed
                
                if not is_closed:
                    await ws.close()
            except Exception as e:
                print(f"{PROGRAM_NAME}: Error closing websocket: {e}", file=sys.stderr)
                
        except (websockets.exceptions.ConnectionClosed, websockets.exceptions.WebSocketException) as e:
            print(f"{PROGRAM_NAME}: WebSocket error: {e}", file=sys.stderr)
            retry_count += 1
        except Exception as e:
            print(f"{PROGRAM_NAME}: Unexpected error: {e}", file=sys.stderr)
            retry_count += 1

def usage():
    print(f"{PROGRAM_NAME}: Usage: {PROGRAM_NAME} [--bearer TOKEN] ws://host/path", file=sys.stderr)
    sys.exit(1)


def parse_args(argv):
    target = None
    bearer = None
    idx = 0

    while idx < len(argv):
        arg = argv[idx]
        if arg == '--bearer':
            if idx + 1 >= len(argv):
                usage()
            bearer = argv[idx + 1].strip()
            idx += 2
        elif arg.startswith('--bearer='):
            bearer = arg.split('=', 1)[1].strip()
            idx += 1
        else:
            if target is not None:
                usage()
            target = arg
            idx += 1

    if not target:
        usage()

    return target, bearer


def main():
    target_uri, bearer_token = parse_args(sys.argv[1:])

    if not bearer_token:
        env_token = os.environ.get("ND_MCP_BEARER_TOKEN", "")
        if env_token:
            bearer_token = env_token.strip()

    if bearer_token:
        print(f"{PROGRAM_NAME}: Authorization header enabled for MCP connection", file=sys.stderr)

    # Set up signal handling
    def signal_handler(sig, frame):
        print(f"{PROGRAM_NAME}: Received signal {sig}, exiting", file=sys.stderr)
        sys.exit(0)
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    try:
        asyncio.run(connect_with_backoff(target_uri, bearer_token))
    except KeyboardInterrupt:
        print(f"{PROGRAM_NAME}: Interrupted by user, exiting", file=sys.stderr)
    
    if STDIN_ERROR:
        print(f"{PROGRAM_NAME}: Exiting due to stdin error", file=sys.stderr)
        
if __name__ == "__main__":
    main()
