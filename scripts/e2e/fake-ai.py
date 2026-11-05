"""Deterministic stand-in for the AI Explainer used by end-to-end tests.

Logs every request as one JSON line to the file given as the first argument
and streams a short explanation in the same SSE format as the real service.
"""
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
LOG = sys.argv[1]
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        with open(LOG, 'a') as f: f.write(json.dumps(body) + "\n")
        self.send_response(200); self.send_header('Content-Type', 'text/event-stream'); self.end_headers()
        text = f"**What happened:** exit {body.get('exitCode')} from `{body.get('command','')[:60]}`. (fake model, auto={body.get('auto', False)})"
        for tok in [text, ""]:
            self.wfile.write(("data: " + json.dumps({"token": tok, "done": tok == ""}) + "\n\n").encode()); self.wfile.flush()
HTTPServer(("127.0.0.1", int(sys.argv[2]) if len(sys.argv) > 2 else 8093), H).serve_forever()
