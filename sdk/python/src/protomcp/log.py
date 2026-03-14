import json

from . import protomcp_pb2 as pb

class ServerLogger:
    def __init__(self, send_fn, name: str = ""):
        self._send_fn = send_fn
        self._name = name

    def _log(self, level: str, message: str, data=None):
        data_json = json.dumps(data) if data else json.dumps({"message": message})
        envelope = pb.Envelope(
            log=pb.LogMessage(
                level=level,
                logger=self._name,
                data_json=data_json,
            )
        )
        self._send_fn(envelope)

    def debug(self, message, **kwargs): self._log("debug", message, kwargs.get("data"))
    def info(self, message, **kwargs): self._log("info", message, kwargs.get("data"))
    def notice(self, message, **kwargs): self._log("notice", message, kwargs.get("data"))
    def warning(self, message, **kwargs): self._log("warning", message, kwargs.get("data"))
    def error(self, message, **kwargs): self._log("error", message, kwargs.get("data"))
    def critical(self, message, **kwargs): self._log("critical", message, kwargs.get("data"))
    def alert(self, message, **kwargs): self._log("alert", message, kwargs.get("data"))
    def emergency(self, message, **kwargs): self._log("emergency", message, kwargs.get("data"))
