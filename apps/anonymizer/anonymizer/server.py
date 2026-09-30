"""HTTP API of the reversible mode. Stateless: no database, no secrets.

    GET  /health       -> {"ok": true, "ner": bool}
    POST /v1/detect    {"text", "lang", "known": [{"kind","value","canon","key"}]}
                       -> {"spans": [...], "residual", "lang", "stats", "guard_added"}
    POST /v1/restore   {"text", "entries": [{"kind","number","canon"}], "context"?}
                       -> {"text", "restored", "inflected", "unknown": [...]}

The Go API owns everything stateful: it keeps the per-user vault (encrypted
originals, alias numbers) in Postgres and sends this service the user's
known values with each request. So the service can be restarted, scaled or
replaced without touching data, and it never sees a key.

Logging: method, path, status, sizes, counts and timings. Never a text,
never a value, never a span; error responses do not echo the input.
"""

from __future__ import annotations

import json
import logging
import os
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from . import detector, ner, restore

log = logging.getLogger("anonymizer.http")
MAX_BODY = int(os.environ.get("ANONYMIZER_MAX_BODY", str(512 * 1024)))


def handle_detect(payload: dict) -> dict:
    text = payload.get("text")
    if not isinstance(text, str):
        raise ValueError("text must be a string")
    items = detector.parse_known(payload.get("known"))
    result = detector.detect(text, str(payload.get("lang") or "auto"), items)
    return {
        "spans": [s.as_dict() for s in result.spans],
        "residual": result.residual,
        "lang": result.lang,
        "stats": result.stats,
        "guard_added": result.guard_added,
    }


def handle_restore(payload: dict) -> dict:
    text = payload.get("text")
    if not isinstance(text, str):
        raise ValueError("text must be a string")
    context = payload.get("context") or ""
    if not isinstance(context, str):
        raise ValueError("context must be a string")
    result = restore.restore(text, restore.parse_entries(payload.get("entries")), context[-200:])
    return {
        "text": result.text,
        "restored": result.restored,
        "inflected": result.inflected,
        "unknown": result.unknown,
    }


ROUTES = {"/v1/detect": handle_detect, "/v1/restore": handle_restore}


class Handler(BaseHTTPRequestHandler):
    server_version = "mindstrata-anonymizer"
    protocol_version = "HTTP/1.1"

    def log_message(self, *args) -> None:  # silence the default access log
        pass

    def _send(self, status: int, body: dict) -> None:
        raw = json.dumps(body, ensure_ascii=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)

    def do_GET(self) -> None:
        if self.path == "/health":
            self._send(200, {"ok": True, "ner": ner.enabled()})
            return
        self._send(404, {"error": "not found"})

    def do_POST(self) -> None:
        t0 = time.perf_counter()
        route = ROUTES.get(self.path)
        if route is None:
            self._send(404, {"error": "not found"})
            return
        length = int(self.headers.get("Content-Length") or 0)
        if length <= 0 or length > MAX_BODY:
            self._send(413, {"error": "body too large or empty"})
            return
        status = 200
        try:
            payload = json.loads(self.rfile.read(length))
            if not isinstance(payload, dict):
                raise ValueError("payload must be an object")
            body = route(payload)
        except (ValueError, json.JSONDecodeError):
            status, body = 400, {"error": "bad request"}
        except Exception as exc:  # noqa: BLE001 - never leak details of the input
            log.error("internal error type=%s", type(exc).__name__)
            status, body = 500, {"error": "internal error"}
        self._send(status, body)
        log.info("%s %s status=%d bytes=%d ms=%.1f", self.command, self.path, status, length,
                 (time.perf_counter() - t0) * 1000)


def serve(host: str = "0.0.0.0", port: int = 8090) -> None:
    logging.basicConfig(level=os.environ.get("LOG_LEVEL", "INFO"),
                        format="%(asctime)s %(levelname)s %(name)s %(message)s")
    ner.warm_up()
    httpd = ThreadingHTTPServer((host, port), Handler)
    log.info("listening on %s:%d ner=%s", host, port, ner.enabled())
    httpd.serve_forever()


if __name__ == "__main__":
    serve(port=int(os.environ.get("ANONYMIZER_PORT", "8090")))
