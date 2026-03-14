import os
from unittest.mock import patch, MagicMock
from protomcp.sidecar import sidecar, get_registered_sidecars, clear_sidecar_registry, SidecarDef, _check_health, _stop_sidecar

def test_register():
    clear_sidecar_registry()
    @sidecar(name="studio", command=["python", "-m", "http.server"], health_check="http://localhost:8421/")
    def studio(): pass
    sidecars = get_registered_sidecars()
    assert len(sidecars) == 1
    assert sidecars[0].name == "studio"
    assert sidecars[0].start_on == "first_tool_call"

def test_start_on_server_start():
    clear_sidecar_registry()
    @sidecar(name="db", command=["postgres"], start_on="server_start")
    def db(): pass
    assert get_registered_sidecars()[0].start_on == "server_start"

def test_pid_file_path():
    clear_sidecar_registry()
    @sidecar(name="studio", command=["echo"])
    def studio(): pass
    expected = os.path.expanduser("~/.protomcp/sidecars/studio.pid")
    assert get_registered_sidecars()[0].pid_file_path == expected

def test_health_check_success():
    clear_sidecar_registry()
    @sidecar(name="studio", command=["echo"], health_check="http://localhost:8421/health")
    def studio(): pass
    mock_resp = MagicMock()
    mock_resp.status = 200
    mock_resp.__enter__ = lambda s: s
    mock_resp.__exit__ = MagicMock(return_value=False)
    with patch("urllib.request.urlopen", return_value=mock_resp):
        assert _check_health(get_registered_sidecars()[0]) is True

def test_health_check_failure():
    clear_sidecar_registry()
    @sidecar(name="studio", command=["echo"], health_check="http://localhost:9999/health")
    def studio(): pass
    with patch("urllib.request.urlopen", side_effect=Exception("refused")):
        assert _check_health(get_registered_sidecars()[0]) is False

def test_stop_nonexistent():
    clear_sidecar_registry()
    @sidecar(name="ghost", command=["echo"])
    def ghost(): pass
    _stop_sidecar(get_registered_sidecars()[0])  # should not raise
