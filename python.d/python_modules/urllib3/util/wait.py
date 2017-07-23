from .selectors import (
    HAS_SELECT,
    DefaultSelector,
    EVENT_READ,
    EVENT_WRITE
)


def _wait_for_io_events(socks, events, timeout=None):
    """ Waits for IO events to be available from a list of sockets
    or optionally a single socket if passed in. Returns a list of
    sockets that can be interacted with immediately. """
    if not HAS_SELECT:
        raise ValueError('Platform does not have a selector')
    if not isinstance(socks, list):
        # Probably just a single socket.
        if hasattr(socks, "fileno"):
            socks = [socks]
        # Otherwise it might be a non-list iterable.
        else:
            socks = list(socks)
    with DefaultSelector() as selector:
        for sock in socks:
            selector.register(sock, events)
        return [key[0].fileobj for key in
                selector.select(timeout) if key[1] & events]


def wait_for_read(socks, timeout=None):
    """ Waits for reading to be available from a list of sockets
    or optionally a single socket if passed in. Returns a list of
    sockets that can be read from immediately. """
    return _wait_for_io_events(socks, EVENT_READ, timeout)


def wait_for_write(socks, timeout=None):
    """ Waits for writing to be available from a list of sockets
    or optionally a single socket if passed in. Returns a list of
    sockets that can be written to immediately. """
    return _wait_for_io_events(socks, EVENT_WRITE, timeout)
