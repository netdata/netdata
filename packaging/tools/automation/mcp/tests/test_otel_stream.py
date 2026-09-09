from netdata_mcp.tools.otel_stream import validate_source_params


def test_valid_params_per_source():
    assert validate_source_params("certstream", url="ws://x", collections=None, start=None, rate=None) is None
    assert validate_source_params("jetstream", url="wss://y", collections="c", start=None, rate=None) is None
    assert validate_source_params("github", url=None, collections=None, start="2024-06-01-12", rate=0) is None
    # all-None is always fine (use defaults)
    assert validate_source_params("certstream", url=None, collections=None, start=None, rate=None) is None


def test_collections_rejected_off_jetstream():
    err = validate_source_params("certstream", url=None, collections="c", start=None, rate=None)
    assert err is not None and "collections" in err and "jetstream only" in err


def test_github_params_rejected_off_github():
    err = validate_source_params("certstream", url=None, collections=None, start="2024-06-01-12", rate=5)
    assert err is not None and "start" in err and "rate" in err


def test_url_rejected_on_github():
    err = validate_source_params("github", url="http://x", collections=None, start=None, rate=None)
    assert err is not None and "url" in err
