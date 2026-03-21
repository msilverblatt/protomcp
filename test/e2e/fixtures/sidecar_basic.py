import urllib.request
from protomcp import tool
from protomcp.sidecar import sidecar
from protomcp.runner import run

@sidecar(
    name="test_http",
    command=["python3", "-m", "http.server", "18932"],
    health_check="http://localhost:18932/",
    start_on="server_start",
    health_timeout=10,
)
class TestHTTPSidecar:
    pass

@tool(description="Check if sidecar is reachable")
def check_sidecar() -> str:
    try:
        resp = urllib.request.urlopen("http://localhost:18932/", timeout=5)
        return f"sidecar status: {resp.status}"
    except Exception as e:
        return f"sidecar unreachable: {e}"

if __name__ == "__main__":
    run()
