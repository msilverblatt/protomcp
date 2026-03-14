import pytest

from protomcp.workflow import _matches_visibility, clear_workflow_registry
from protomcp.tool import clear_registry


@pytest.fixture(autouse=True)
def clean_registries():
    clear_workflow_registry()
    clear_registry()
    yield
    clear_workflow_registry()
    clear_registry()


def test_no_filters_returns_false():
    """Neither allow nor block specified = fully exclusive."""
    assert _matches_visibility("some_tool", None, None) is False


def test_allow_patterns():
    assert _matches_visibility("deploy_tool", ["deploy_tool"], None) is True
    assert _matches_visibility("other_tool", ["deploy_tool"], None) is False


def test_block_patterns():
    assert _matches_visibility("dangerous", None, ["dangerous"]) is False
    # block_during with no allow_during: if tool is not blocked, it passes
    # Actually, with allow=None and block specified, we need to check the logic:
    # allow is None → skip allow check (not filtered out by allow)
    # block has entries → check block
    assert _matches_visibility("safe_tool", None, ["dangerous"]) is True


def test_allow_then_block():
    """Allow lets it through, then block filters it out."""
    assert _matches_visibility("admin_tool", ["admin_*"], ["admin_tool"]) is False
    assert _matches_visibility("admin_other", ["admin_*"], ["admin_tool"]) is True


def test_wildcard_patterns():
    assert _matches_visibility("deploy.start", ["deploy.*"], None) is True
    assert _matches_visibility("monitor.check", ["deploy.*"], None) is False
    assert _matches_visibility("logs.view", ["*"], None) is True


def test_exact_match():
    assert _matches_visibility("my_tool", ["my_tool"], None) is True
    assert _matches_visibility("my_tool_extra", ["my_tool"], None) is False
