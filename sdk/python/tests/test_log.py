from protomcp.log import ServerLogger

def test_log_info():
    sent = []
    logger = ServerLogger(send_fn=lambda msg: sent.append(msg))
    logger.info("hello", data={"count": 5})
    assert len(sent) == 1
    assert sent[0].log.level == "info"
    assert '"count": 5' in sent[0].log.data_json or '"count":5' in sent[0].log.data_json

def test_log_with_logger_name():
    sent = []
    logger = ServerLogger(send_fn=lambda msg: sent.append(msg), name="cache")
    logger.debug("hit")
    assert sent[0].log.logger == "cache"

def test_all_levels():
    sent = []
    logger = ServerLogger(send_fn=lambda msg: sent.append(msg))
    for level in ["debug", "info", "notice", "warning", "error", "critical", "alert", "emergency"]:
        getattr(logger, level)("test")
    assert len(sent) == 8
