"""Keep tool execution off the socket reader while preserving serial handler execution."""
import asyncio
from concurrent.futures import ThreadPoolExecutor
import threading

from protomcp.context import ToolContext


async def await_result(result, ctx: ToolContext):
    """Await a handler/middleware result and deliver cancellation on its owning loop."""
    loop = asyncio.get_running_loop()
    task = asyncio.current_task()

    def cancel():
        loop.call_soon_threadsafe(task.cancel)

    ctx._bind_cancel(cancel)
    try:
        return await result
    finally:
        ctx._bind_cancel(None)


class ToolDispatcher:
    """One worker preserves existing sync middleware/state semantics.

    The main thread remains the sole socket reader, so cancellation and reverse
    requests can be delivered while a tool is running. Sync tools must cooperate
    via ctx.is_cancelled(); Python cannot safely interrupt arbitrary sync code.
    """

    def __init__(self, transport, handler):
        self._transport = transport
        self._handler = handler
        self._executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="protomcp-tool")
        self._calls = {}
        self._lock = threading.Lock()

    def submit(self, env):
        ctx = ToolContext(env.call_tool.progress_token, self._transport.send)
        with self._lock:
            if env.request_id in self._calls:
                raise ValueError(f"Duplicate tool request ID: {env.request_id}")
            self._calls[env.request_id] = ctx
        self._executor.submit(self._execute, env, ctx)

    def _execute(self, env, ctx):
        try:
            if not ctx.is_cancelled():
                self._handler(self._transport, env, ctx)
        finally:
            with self._lock:
                self._calls.pop(env.request_id, None)

    def cancel(self, request_id):
        with self._lock:
            ctx = self._calls.get(request_id)
        if ctx is not None:
            ctx._set_cancelled()

    def close(self):
        with self._lock:
            contexts = list(self._calls.values())
        for ctx in contexts:
            ctx._set_cancelled()
        self._executor.shutdown(wait=True)
