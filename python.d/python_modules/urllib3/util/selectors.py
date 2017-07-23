# Backport of selectors.py from Python 3.5+ to support Python < 3.4
# Also has the behavior specified in PEP 475 which is to retry syscalls
# in the case of an EINTR error. This module is required because selectors34
# does not follow this behavior and instead returns that no dile descriptor
# events have occurred rather than retry the syscall. The decision to drop
# support for select.devpoll is made to maintain 100% test coverage.

import errno
import math
import select
import socket
import sys
import time
from collections import namedtuple, Mapping

try:
    monotonic = time.monotonic
except (AttributeError, ImportError):  # Python 3.3<
    monotonic = time.time

EVENT_READ = (1 << 0)
EVENT_WRITE = (1 << 1)

HAS_SELECT = True  # Variable that shows whether the platform has a selector.
_SYSCALL_SENTINEL = object()  # Sentinel in case a system call returns None.
_DEFAULT_SELECTOR = None


class SelectorError(Exception):
    def __init__(self, errcode):
        super(SelectorError, self).__init__()
        self.errno = errcode

    def __repr__(self):
        return "<SelectorError errno={0}>".format(self.errno)

    def __str__(self):
        return self.__repr__()


def _fileobj_to_fd(fileobj):
    """ Return a file descriptor from a file object. If
    given an integer will simply return that integer back. """
    if isinstance(fileobj, int):
        fd = fileobj
    else:
        try:
            fd = int(fileobj.fileno())
        except (AttributeError, TypeError, ValueError):
            raise ValueError("Invalid file object: {0!r}".format(fileobj))
    if fd < 0:
        raise ValueError("Invalid file descriptor: {0}".format(fd))
    return fd


# Determine which function to use to wrap system calls because Python 3.5+
# already handles the case when system calls are interrupted.
if sys.version_info >= (3, 5):
    def _syscall_wrapper(func, _, *args, **kwargs):
        """ This is the short-circuit version of the below logic
        because in Python 3.5+ all system calls automatically restart
        and recalculate their timeouts. """
        try:
            return func(*args, **kwargs)
        except (OSError, IOError, select.error) as e:
            errcode = None
            if hasattr(e, "errno"):
                errcode = e.errno
            raise SelectorError(errcode)
else:
    def _syscall_wrapper(func, recalc_timeout, *args, **kwargs):
        """ Wrapper function for syscalls that could fail due to EINTR.
        All functions should be retried if there is time left in the timeout
        in accordance with PEP 475. """
        timeout = kwargs.get("timeout", None)
        if timeout is None:
            expires = None
            recalc_timeout = False
        else:
            timeout = float(timeout)
            if timeout < 0.0:  # Timeout less than 0 treated as no timeout.
                expires = None
            else:
                expires = monotonic() + timeout

        args = list(args)
        if recalc_timeout and "timeout" not in kwargs:
            raise ValueError(
                "Timeout must be in args or kwargs to be recalculated")

        result = _SYSCALL_SENTINEL
        while result is _SYSCALL_SENTINEL:
            try:
                result = func(*args, **kwargs)
            # OSError is thrown by select.select
            # IOError is thrown by select.epoll.poll
            # select.error is thrown by select.poll.poll
            # Aren't we thankful for Python 3.x rework for exceptions?
            except (OSError, IOError, select.error) as e:
                # select.error wasn't a subclass of OSError in the past.
                errcode = None
                if hasattr(e, "errno"):
                    errcode = e.errno
                elif hasattr(e, "args"):
                    errcode = e.args[0]

                # Also test for the Windows equivalent of EINTR.
                is_interrupt = (errcode == errno.EINTR or (hasattr(errno, "WSAEINTR") and
                                                           errcode == errno.WSAEINTR))

                if is_interrupt:
                    if expires is not None:
                        current_time = monotonic()
                        if current_time > expires:
                            raise OSError(errno=errno.ETIMEDOUT)
                        if recalc_timeout:
                            if "timeout" in kwargs:
                                kwargs["timeout"] = expires - current_time
                    continue
                if errcode:
                    raise SelectorError(errcode)
                else:
                    raise
        return result


SelectorKey = namedtuple('SelectorKey', ['fileobj', 'fd', 'events', 'data'])


class _SelectorMapping(Mapping):
    """ Mapping of file objects to selector keys """

    def __init__(self, selector):
        self._selector = selector

    def __len__(self):
        return len(self._selector._fd_to_key)

    def __getitem__(self, fileobj):
        try:
            fd = self._selector._fileobj_lookup(fileobj)
            return self._selector._fd_to_key[fd]
        except KeyError:
            raise KeyError("{0!r} is not registered.".format(fileobj))

    def __iter__(self):
        return iter(self._selector._fd_to_key)


class BaseSelector(object):
    """ Abstract Selector class

    A selector supports registering file objects to be monitored
    for specific I/O events.

    A file object is a file descriptor or any object with a
    `fileno()` method. An arbitrary object can be attached to the
    file object which can be used for example to store context info,
    a callback, etc.

    A selector can use various implementations (select(), poll(), epoll(),
    and kqueue()) depending on the platform. The 'DefaultSelector' class uses
    the most efficient implementation for the current platform.
    """
    def __init__(self):
        # Maps file descriptors to keys.
        self._fd_to_key = {}

        # Read-only mapping returned by get_map()
        self._map = _SelectorMapping(self)

    def _fileobj_lookup(self, fileobj):
        """ Return a file descriptor from a file object.
        This wraps _fileobj_to_fd() to do an exhaustive
        search in case the object is invalid but we still
        have it in our map. Used by unregister() so we can
        unregister an object that was previously registered
        even if it is closed. It is also used by _SelectorMapping
        """
        try:
            return _fileobj_to_fd(fileobj)
        except ValueError:

            # Search through all our mapped keys.
            for key in self._fd_to_key.values():
                if key.fileobj is fileobj:
                    return key.fd

            # Raise ValueError after all.
            raise

    def register(self, fileobj, events, data=None):
        """ Register a file object for a set of events to monitor. """
        if (not events) or (events & ~(EVENT_READ | EVENT_WRITE)):
            raise ValueError("Invalid events: {0!r}".format(events))

        key = SelectorKey(fileobj, self._fileobj_lookup(fileobj), events, data)

        if key.fd in self._fd_to_key:
            raise KeyError("{0!r} (FD {1}) is already registered"
                           .format(fileobj, key.fd))

        self._fd_to_key[key.fd] = key
        return key

    def unregister(self, fileobj):
        """ Unregister a file object from being monitored. """
        try:
            key = self._fd_to_key.pop(self._fileobj_lookup(fileobj))
        except KeyError:
            raise KeyError("{0!r} is not registered".format(fileobj))

        # Getting the fileno of a closed socket on Windows errors with EBADF.
        except socket.error as e:  # Platform-specific: Windows.
            if e.errno != errno.EBADF:
                raise
            else:
                for key in self._fd_to_key.values():
                    if key.fileobj is fileobj:
                        self._fd_to_key.pop(key.fd)
                        break
                else:
                    raise KeyError("{0!r} is not registered".format(fileobj))
        return key

    def modify(self, fileobj, events, data=None):
        """ Change a registered file object monitored events and data. """
        # NOTE: Some subclasses optimize this operation even further.
        try:
            key = self._fd_to_key[self._fileobj_lookup(fileobj)]
        except KeyError:
            raise KeyError("{0!r} is not registered".format(fileobj))

        if events != key.events:
            self.unregister(fileobj)
            key = self.register(fileobj, events, data)

        elif data != key.data:
            # Use a shortcut to update the data.
            key = key._replace(data=data)
            self._fd_to_key[key.fd] = key

        return key

    def select(self, timeout=None):
        """ Perform the actual selection until some monitored file objects
        are ready or the timeout expires. """
        raise NotImplementedError()

    def close(self):
        """ Close the selector. This must be called to ensure that all
        underlying resources are freed. """
        self._fd_to_key.clear()
        self._map = None

    def get_key(self, fileobj):
        """ Return the key associated with a registered file object. """
        mapping = self.get_map()
        if mapping is None:
            raise RuntimeError("Selector is closed")
        try:
            return mapping[fileobj]
        except KeyError:
            raise KeyError("{0!r} is not registered".format(fileobj))

    def get_map(self):
        """ Return a mapping of file objects to selector keys """
        return self._map

    def _key_from_fd(self, fd):
        """ Return the key associated to a given file descriptor
         Return None if it is not found. """
        try:
            return self._fd_to_key[fd]
        except KeyError:
            return None

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()


# Almost all platforms have select.select()
if hasattr(select, "select"):
    class SelectSelector(BaseSelector):
        """ Select-based selector. """
        def __init__(self):
            super(SelectSelector, self).__init__()
            self._readers = set()
            self._writers = set()

        def register(self, fileobj, events, data=None):
            key = super(SelectSelector, self).register(fileobj, events, data)
            if events & EVENT_READ:
                self._readers.add(key.fd)
            if events & EVENT_WRITE:
                self._writers.add(key.fd)
            return key

        def unregister(self, fileobj):
            key = super(SelectSelector, self).unregister(fileobj)
            self._readers.discard(key.fd)
            self._writers.discard(key.fd)
            return key

        def _select(self, r, w, timeout=None):
            """ Wrapper for select.select because timeout is a positional arg """
            return select.select(r, w, [], timeout)

        def select(self, timeout=None):
            # Selecting on empty lists on Windows errors out.
            if not len(self._readers) and not len(self._writers):
                return []

            timeout = None if timeout is None else max(timeout, 0.0)
            ready = []
            r, w, _ = _syscall_wrapper(self._select, True, self._readers,
                                       self._writers, timeout)
            r = set(r)
            w = set(w)
            for fd in r | w:
                events = 0
                if fd in r:
                    events |= EVENT_READ
                if fd in w:
                    events |= EVENT_WRITE

                key = self._key_from_fd(fd)
                if key:
                    ready.append((key, events & key.events))
            return ready


if hasattr(select, "poll"):
    class PollSelector(BaseSelector):
        """ Poll-based selector """
        def __init__(self):
            super(PollSelector, self).__init__()
            self._poll = select.poll()

        def register(self, fileobj, events, data=None):
            key = super(PollSelector, self).register(fileobj, events, data)
            event_mask = 0
            if events & EVENT_READ:
                event_mask |= select.POLLIN
            if events & EVENT_WRITE:
                event_mask |= select.POLLOUT
            self._poll.register(key.fd, event_mask)
            return key

        def unregister(self, fileobj):
            key = super(PollSelector, self).unregister(fileobj)
            self._poll.unregister(key.fd)
            return key

        def _wrap_poll(self, timeout=None):
            """ Wrapper function for select.poll.poll() so that
            _syscall_wrapper can work with only seconds. """
            if timeout is not None:
                if timeout <= 0:
                    timeout = 0
                else:
                    # select.poll.poll() has a resolution of 1 millisecond,
                    # round away from zero to wait *at least* timeout seconds.
                    timeout = math.ceil(timeout * 1e3)

            result = self._poll.poll(timeout)
            return result

        def select(self, timeout=None):
            ready = []
            fd_events = _syscall_wrapper(self._wrap_poll, True, timeout=timeout)
            for fd, event_mask in fd_events:
                events = 0
                if event_mask & ~select.POLLIN:
                    events |= EVENT_WRITE
                if event_mask & ~select.POLLOUT:
                    events |= EVENT_READ

                key = self._key_from_fd(fd)
                if key:
                    ready.append((key, events & key.events))

            return ready


if hasattr(select, "epoll"):
    class EpollSelector(BaseSelector):
        """ Epoll-based selector """
        def __init__(self):
            super(EpollSelector, self).__init__()
            self._epoll = select.epoll()

        def fileno(self):
            return self._epoll.fileno()

        def register(self, fileobj, events, data=None):
            key = super(EpollSelector, self).register(fileobj, events, data)
            events_mask = 0
            if events & EVENT_READ:
                events_mask |= select.EPOLLIN
            if events & EVENT_WRITE:
                events_mask |= select.EPOLLOUT
            _syscall_wrapper(self._epoll.register, False, key.fd, events_mask)
            return key

        def unregister(self, fileobj):
            key = super(EpollSelector, self).unregister(fileobj)
            try:
                _syscall_wrapper(self._epoll.unregister, False, key.fd)
            except SelectorError:
                # This can occur when the fd was closed since registry.
                pass
            return key

        def select(self, timeout=None):
            if timeout is not None:
                if timeout <= 0:
                    timeout = 0.0
                else:
                    # select.epoll.poll() has a resolution of 1 millisecond
                    # but luckily takes seconds so we don't need a wrapper
                    # like PollSelector. Just for better rounding.
                    timeout = math.ceil(timeout * 1e3) * 1e-3
                timeout = float(timeout)
            else:
                timeout = -1.0  # epoll.poll() must have a float.

            # We always want at least 1 to ensure that select can be called
            # with no file descriptors registered. Otherwise will fail.
            max_events = max(len(self._fd_to_key), 1)

            ready = []
            fd_events = _syscall_wrapper(self._epoll.poll, True,
                                         timeout=timeout,
                                         maxevents=max_events)
            for fd, event_mask in fd_events:
                events = 0
                if event_mask & ~select.EPOLLIN:
                    events |= EVENT_WRITE
                if event_mask & ~select.EPOLLOUT:
                    events |= EVENT_READ

                key = self._key_from_fd(fd)
                if key:
                    ready.append((key, events & key.events))
            return ready

        def close(self):
            self._epoll.close()
            super(EpollSelector, self).close()


if hasattr(select, "kqueue"):
    class KqueueSelector(BaseSelector):
        """ Kqueue / Kevent-based selector """
        def __init__(self):
            super(KqueueSelector, self).__init__()
            self._kqueue = select.kqueue()

        def fileno(self):
            return self._kqueue.fileno()

        def register(self, fileobj, events, data=None):
            key = super(KqueueSelector, self).register(fileobj, events, data)
            if events & EVENT_READ:
                kevent = select.kevent(key.fd,
                                       select.KQ_FILTER_READ,
                                       select.KQ_EV_ADD)

                _syscall_wrapper(self._kqueue.control, False, [kevent], 0, 0)

            if events & EVENT_WRITE:
                kevent = select.kevent(key.fd,
                                       select.KQ_FILTER_WRITE,
                                       select.KQ_EV_ADD)

                _syscall_wrapper(self._kqueue.control, False, [kevent], 0, 0)

            return key

        def unregister(self, fileobj):
            key = super(KqueueSelector, self).unregister(fileobj)
            if key.events & EVENT_READ:
                kevent = select.kevent(key.fd,
                                       select.KQ_FILTER_READ,
                                       select.KQ_EV_DELETE)
                try:
                    _syscall_wrapper(self._kqueue.control, False, [kevent], 0, 0)
                except SelectorError:
                    pass
            if key.events & EVENT_WRITE:
                kevent = select.kevent(key.fd,
                                       select.KQ_FILTER_WRITE,
                                       select.KQ_EV_DELETE)
                try:
                    _syscall_wrapper(self._kqueue.control, False, [kevent], 0, 0)
                except SelectorError:
                    pass

            return key

        def select(self, timeout=None):
            if timeout is not None:
                timeout = max(timeout, 0)

            max_events = len(self._fd_to_key) * 2
            ready_fds = {}

            kevent_list = _syscall_wrapper(self._kqueue.control, True,
                                           None, max_events, timeout)

            for kevent in kevent_list:
                fd = kevent.ident
                event_mask = kevent.filter
                events = 0
                if event_mask == select.KQ_FILTER_READ:
                    events |= EVENT_READ
                if event_mask == select.KQ_FILTER_WRITE:
                    events |= EVENT_WRITE

                key = self._key_from_fd(fd)
                if key:
                    if key.fd not in ready_fds:
                        ready_fds[key.fd] = (key, events & key.events)
                    else:
                        old_events = ready_fds[key.fd][1]
                        ready_fds[key.fd] = (key, (events | old_events) & key.events)

            return list(ready_fds.values())

        def close(self):
            self._kqueue.close()
            super(KqueueSelector, self).close()


if not hasattr(select, 'select'):  # Platform-specific: AppEngine
    HAS_SELECT = False


def _can_allocate(struct):
    """ Checks that select structs can be allocated by the underlying
    operating system, not just advertised by the select module. We don't
    check select() because we'll be hopeful that most platforms that
    don't have it available will not advertise it. (ie: GAE) """
    try:
        # select.poll() objects won't fail until used.
        if struct == 'poll':
            p = select.poll()
            p.poll(0)

        # All others will fail on allocation.
        else:
            getattr(select, struct)().close()
        return True
    except (OSError, AttributeError) as e:
        return False


# Choose the best implementation, roughly:
# kqueue == epoll > poll > select. Devpoll not supported. (See above)
# select() also can't accept a FD > FD_SETSIZE (usually around 1024)
def DefaultSelector():
    """ This function serves as a first call for DefaultSelector to
    detect if the select module is being monkey-patched incorrectly
    by eventlet, greenlet, and preserve proper behavior. """
    global _DEFAULT_SELECTOR
    if _DEFAULT_SELECTOR is None:
        if _can_allocate('kqueue'):
            _DEFAULT_SELECTOR = KqueueSelector
        elif _can_allocate('epoll'):
            _DEFAULT_SELECTOR = EpollSelector
        elif _can_allocate('poll'):
            _DEFAULT_SELECTOR = PollSelector
        elif hasattr(select, 'select'):
            _DEFAULT_SELECTOR = SelectSelector
        else:  # Platform-specific: AppEngine
            raise ValueError('Platform does not have a selector')
    return _DEFAULT_SELECTOR()
