"""Deterministic stand-in for the AI Explainer used by end-to-end tests.

Logs every request as one JSON line to the file given as the first argument
and streams a short explanation in the same SSE format as the real service.
"""
import json, os, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
LOG = sys.argv[1]
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        with open(LOG, 'a') as f: f.write(json.dumps(body) + "\n")
        if self.path == "/internal/complete-json":
            return self.complete_json(body)
        self.send_response(200); self.send_header('Content-Type', 'text/event-stream'); self.end_headers()
        text = f"**What happened:** exit {body.get('exitCode')} from `{body.get('command','')[:60]}`. (fake model, auto={body.get('auto', False)})"
        for tok in [text, ""]:
            self.wfile.write(("data: " + json.dumps({"token": tok, "done": tok == ""}) + "\n\n").encode()); self.wfile.flush()
def generated_spec(prompt):
    """First attempt: a spec with a defect static validation must catch.
    After feedback: a valid spec derived from the community lab."""
    here = os.path.dirname(os.path.abspath(__file__))
    with open(os.path.join(here, "../../services/session-manager/labs/community/stopped-worker.json")) as f:
        spec = json.load(f)
    spec["id"] = "paused-consumer"
    spec["title"] = "The paused consumer"
    if "previous attempt was rejected" not in prompt:
        spec["setup"] = "echo x > /tmp/run.sh && chmod +x /tmp/run.sh\n" + spec["setup"]
    return "Here is the lab:\n```json\n" + json.dumps(spec) + "\n```"


def _complete_json(self, body):
    data = json.dumps({"json": generated_spec(body.get("prompt", ""))}).encode()
    self.send_response(200); self.send_header('Content-Type', 'application/json')
    self.send_header('Content-Length', str(len(data))); self.end_headers(); self.wfile.write(data)


H.complete_json = _complete_json

HTTPServer(("127.0.0.1", int(sys.argv[2]) if len(sys.argv) > 2 else 8093), H).serve_forever()
