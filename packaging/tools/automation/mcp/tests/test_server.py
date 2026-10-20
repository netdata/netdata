import pytest

from netdata_mcp.server import parse_args


def test_parse_args_defaults_to_stdio():
    a = parse_args([])
    assert a.transport == "stdio"
    assert a.host == "127.0.0.1"
    assert a.port == 8000


def test_parse_args_http_with_host_port():
    a = parse_args(["--transport", "http", "--host", "0.0.0.0", "--port", "9000"])
    assert a.transport == "http"
    assert a.host == "0.0.0.0"
    assert a.port == 9000


def test_parse_args_rejects_unknown_transport():
    with pytest.raises(SystemExit):
        parse_args(["--transport", "ws"])
