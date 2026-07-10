import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200 if self.path == "/health" else 404)
        self.end_headers()

    def do_POST(self):
        if self.headers.get("Authorization") != "Bearer integration-moderation-token":
            self.send_response(401); self.end_headers(); return
        length = int(self.headers.get("Content-Length", "0"))
        request = json.loads(self.rfile.read(length) or b"{}")
        if self.path == "/v1/transcribe":
            response = {"text": "safe integration transcript", "confidence": 0.99, "locale": "en", "model": "faster-whisper:fixture"}
        elif self.path in {"/v1/moderate", "/v1/moderate-text"}:
            response = {"status": "approved", "risk_level": "low", "categories": {}, "models": ["ShieldGemma", "ShieldGemma2", "OpenNSFW2"]}
        else:
            self.send_response(404); self.end_headers(); return
        body = json.dumps(response).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.send_header("Content-Length", str(len(body))); self.end_headers(); self.wfile.write(body)

    def log_message(self, *_):
        pass


ThreadingHTTPServer(("0.0.0.0", 8081), Handler).serve_forever()
