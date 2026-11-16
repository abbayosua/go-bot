#!/usr/bin/env python3
"""Scrapling sidecar for go-bot: POST /fetch {"url": ...} -> rendered text.
StealthyFetcher beats Turnstile/captcha where plain HTTP fails."""
import json
from http.server import BaseHTTPRequestHandler, HTTPServer

from scrapling import StealthyFetcher

PORT = 8765
LIMIT = 12000


class H(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def do_POST(self):
        if self.path != "/fetch":
            self.send_response(404)
            self.end_headers()
            return
        try:
            body = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0)) or 0))
        except Exception:
            body = {}
        url = (body.get("url") or "").strip()
        if not url:
            self._out(400, {"error": "url is required"})
            return
        try:
            page = StealthyFetcher.fetch(url, headless=True, network="idle")
            heads = [((a.text) or "").strip() for a in page.css("a h3")][:15]
            paras = [((p.text) or "").strip() for p in page.css("p")][:20]
            text = "\n".join([h for h in heads if h] + [p for p in paras if p])[:LIMIT]
            self._out(200, {"status": getattr(page, "status", 200), "url": url, "text": text})
        except Exception as e:
            self._out(502, {"error": str(e)[:500]})

    def do_GET(self):
        if self.path == "/health":
            self._out(200, {"status": "ok"})
            return
        self.send_response(404)
        self.end_headers()

    def _out(self, code, obj):
        raw = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(raw)))
        self.end_headers()
        self.wfile.write(raw)


if __name__ == "__main__":
    print(f"scrapling sidecar on 127.0.0.1:{PORT}", flush=True)
    HTTPServer(("127.0.0.1", PORT), H).serve_forever()
