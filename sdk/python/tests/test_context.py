from protomcp.context import ToolContext

def test_report_progress_sends_notification():
    sent = []
    ctx = ToolContext(progress_token="pt-1", send_fn=lambda msg: sent.append(msg))
    ctx.report_progress(progress=5, total=10, message="Working")
    assert len(sent) == 1
    assert sent[0].progress.progress_token == "pt-1"
    assert sent[0].progress.progress == 5
    assert sent[0].progress.total == 10

def test_report_progress_noop_without_token():
    sent = []
    ctx = ToolContext(progress_token="", send_fn=lambda msg: sent.append(msg))
    ctx.report_progress(progress=1)
    assert len(sent) == 0

def test_is_cancelled():
    ctx = ToolContext(progress_token="", send_fn=lambda msg: None)
    assert not ctx.is_cancelled()
    ctx._cancelled = True
    assert ctx.is_cancelled()
