"""HTTP API of the reversible mode."""

import json
import logging
import threading
import urllib.error
import urllib.request
from http.server import ThreadingHTTPServer

import pytest

from anonymizer.server import Handler

SECRET = "Шмыгло"


@pytest.fixture(scope="module")
def base_url():
    httpd = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    thread.start()
    yield f"http://127.0.0.1:{httpd.server_address[1]}"
    httpd.shutdown()


def post(url: str, body) -> tuple[int, dict]:
    raw = body if isinstance(body, bytes) else json.dumps(body).encode()
    req = urllib.request.Request(url, data=raw, headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as err:
        return err.code, json.loads(err.read())


def test_health(base_url):
    with urllib.request.urlopen(base_url + "/health", timeout=10) as resp:
        assert json.loads(resp.read())["ok"] is True


def test_detect_returns_spans_with_offsets(base_url):
    text = f"Мама {SECRET} звонила с +7 912 000-11-22"
    status, body = post(base_url + "/v1/detect", {"text": text, "lang": "auto",
                                                  "known": [{"kind": "PERSON", "value": SECRET}]})
    assert status == 200 and body["residual"] == 0 and body["lang"] == "ru"
    found = {text[s["start"]:s["end"]] for s in body["spans"]}
    assert SECRET in found and "+7 912 000-11-22" in found
    assert all({"kind", "canon", "key"} <= set(s) for s in body["spans"])


def test_restore(base_url):
    status, body = post(base_url + "/v1/restore", {
        "text": "Поговорите с ЛИЦО_1 и ЛИЦО_7.",
        "entries": [{"kind": "PERSON", "number": 1, "canon": "Люда"}],
    })
    assert status == 200
    assert body["text"].startswith("Поговорите с Людой") and body["unknown"] == ["ЛИЦО_7"]


def test_bad_request_does_not_echo_input(base_url):
    status, body = post(base_url + "/v1/detect", f"{{\"text\": \"{SECRET}".encode())
    assert status == 400 and SECRET not in json.dumps(body, ensure_ascii=False)
    status, body = post(base_url + "/v1/detect", {"text": 42})
    assert status == 400


def test_unknown_route(base_url):
    status, _ = post(base_url + "/v1/nope", {"text": "x"})
    assert status == 404


def test_logs_have_no_content(base_url, caplog):
    caplog.set_level(logging.DEBUG, logger="anonymizer")
    post(base_url + "/v1/detect", {"text": f"Позвоните {SECRET}у на 89120001122"})
    post(base_url + "/v1/restore", {"text": "ЛИЦО_1", "entries": [{"kind": "PERSON", "number": 1, "canon": SECRET}]})
    for record in caplog.records:
        msg = record.getMessage()
        assert SECRET not in msg and "8912" not in msg
