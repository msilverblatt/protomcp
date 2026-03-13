import threading
import sys, os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', '..', 'gen'))
import protomcp_pb2 as pb

class ToolContext:
    def __init__(self, progress_token: str, send_fn):
        self._progress_token = progress_token
        self._send_fn = send_fn
        self._cancelled = False
        self._lock = threading.Lock()

    def report_progress(self, progress: int, total: int = 0, message: str = ""):
        if not self._progress_token:
            return
        envelope = pb.Envelope(
            progress=pb.ProgressNotification(
                progress_token=self._progress_token,
                progress=progress,
                total=total,
                message=message,
            )
        )
        self._send_fn(envelope)

    def is_cancelled(self) -> bool:
        with self._lock:
            return self._cancelled

    def _set_cancelled(self):
        with self._lock:
            self._cancelled = True
