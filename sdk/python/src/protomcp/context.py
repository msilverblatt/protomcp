import json
import threading
import uuid

import protomcp.protomcp_pb2 as pb

# Global registry for pending sampling responses.
# The runner's main loop will check incoming envelopes for SamplingResponse
# and deliver them here.
_pending_sampling: dict[str, threading.Event] = {}
_sampling_results: dict[str, pb.SamplingResponse] = {}
_sampling_lock = threading.Lock()


def _deliver_sampling_response(request_id: str, resp: pb.SamplingResponse):
    """Called by the runner when a SamplingResponse arrives."""
    with _sampling_lock:
        _sampling_results[request_id] = resp
        event = _pending_sampling.get(request_id)
        if event:
            event.set()


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

    def sample(
        self,
        messages: list[dict],
        system_prompt: str = "",
        max_tokens: int = 1000,
        model_preferences: dict | None = None,
        timeout: float = 60.0,
    ) -> dict:
        """Request an LLM completion from the MCP client.

        Args:
            messages: List of {"role": str, "content": str} dicts.
            system_prompt: Optional system prompt.
            max_tokens: Maximum tokens to generate.
            model_preferences: Optional model hints (client may ignore).
            timeout: Seconds to wait for response.

        Returns:
            Dict with keys: role, content, model, stop_reason, error.
        """
        req_id = str(uuid.uuid4())

        event = threading.Event()
        with _sampling_lock:
            _pending_sampling[req_id] = event

        envelope = pb.Envelope(
            request_id=req_id,
            sampling_request=pb.SamplingRequest(
                messages_json=json.dumps(messages),
                system_prompt=system_prompt,
                max_tokens=max_tokens,
                model_preferences_json=json.dumps(model_preferences) if model_preferences else "",
            ),
        )
        self._send_fn(envelope)

        if not event.wait(timeout=timeout):
            with _sampling_lock:
                _pending_sampling.pop(req_id, None)
            return {"error": f"sampling request timed out after {timeout}s"}

        with _sampling_lock:
            _pending_sampling.pop(req_id, None)
            resp = _sampling_results.pop(req_id, None)

        if resp is None:
            return {"error": "no sampling response received"}

        if resp.error:
            return {"error": resp.error}

        # Parse content_json back to text
        content = resp.content_json
        try:
            parsed = json.loads(content)
            if isinstance(parsed, dict) and "text" in parsed:
                content = parsed["text"]
        except (json.JSONDecodeError, TypeError):
            pass

        return {
            "role": resp.role,
            "content": content,
            "model": resp.model,
            "stop_reason": resp.stop_reason,
        }
