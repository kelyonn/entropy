#!/usr/bin/env python3
"""Entropy production stack: a small, real service topology for incident labs.

Roles (one process each, started by stackctl):
  db      TCP line protocol on 127.0.0.1:5432 with a connection limit
  app     HTTP API on 127.0.0.1:8000 that queries db per request
  proxy   HTTP reverse proxy on 127.0.0.1:8080 in front of app
  loadgen steady traffic to the proxy; writes one stats line per second

Configuration lives in /etc/stack/*.conf (key=value). Logs go to
/var/log/stack/<role>.log. Traffic stats: /var/log/stack/traffic.log with
"epoch total ok errors p50_ms p95_ms" per line.
"""
import http.client
import os
import random
import socket
import socketserver
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CONF_DIR = "/etc/stack"
LOG_DIR = "/var/log/stack"


def conf(name):
    values = {}
    try:
        with open(os.path.join(CONF_DIR, name + ".conf")) as f:
            for line in f:
                line = line.split("#", 1)[0].strip()
                if "=" in line:
                    k, v = line.split("=", 1)
                    values[k.strip()] = v.strip()
    except OSError:
        pass
    return values


class Log:
    def __init__(self, role):
        self.path = os.path.join(LOG_DIR, role + ".log")
        self.lock = threading.Lock()

    def write(self, level, msg):
        line = "%s %s %s\n" % (time.strftime("%H:%M:%S"), level, msg)
        with self.lock:
            try:
                with open(self.path, "a") as f:
                    f.write(line)
            except OSError:
                # A full disk must hurt the service, as it would in production.
                raise


# ---------------------------------------------------------------- db

def run_db():
    cfg = conf("db")
    log = Log("db")
    max_conn = int(cfg.get("max_connections", "20"))
    indexed = cfg.get("index_orders", "on") == "on"
    active = [0]
    lock = threading.Lock()

    class Handler(socketserver.StreamRequestHandler):
        def handle(self):
            with lock:
                if active[0] >= max_conn:
                    self.wfile.write(b"ERR too many connections\n")
                    log.write("ERROR", "connection refused: too many connections (max_connections=%d)" % max_conn)
                    return
                active[0] += 1
            try:
                for raw in self.rfile:
                    q = raw.decode(errors="replace").strip()
                    if q == "QUIT":
                        break
                    if q.startswith("SELECT orders"):
                        if not indexed:
                            time.sleep(0.35 + random.random() * 0.15)
                            log.write("WARN", "slow query 400ms: full scan on orders (no index on orders.customer_id)")
                        else:
                            time.sleep(0.002)
                        self.wfile.write(b"OK 3 rows\n")
                    elif q == "PING":
                        self.wfile.write(b"PONG\n")
                    else:
                        self.wfile.write(b"ERR syntax\n")
            finally:
                with lock:
                    active[0] -= 1

    class Server(socketserver.ThreadingTCPServer):
        allow_reuse_address = True
        daemon_threads = True

    log.write("INFO", "db listening on 127.0.0.1:5432 max_connections=%d index_orders=%s" % (max_conn, "on" if indexed else "off"))
    Server(("127.0.0.1", 5432), Handler).serve_forever()


# ---------------------------------------------------------------- app

def run_app():
    cfg = conf("app")
    log = Log("app")
    port = int(cfg.get("listen_port", "8000"))
    db_port = int(cfg.get("db_port", "5432"))
    pool = threading.BoundedSemaphore(int(cfg.get("pool_size", "4")))
    hold = float(cfg.get("hold_connection_ms", "0")) / 1000.0
    debug = cfg.get("log_level", "info") == "debug"

    def query():
        with pool:
            s = socket.create_connection(("127.0.0.1", db_port), timeout=2)
            try:
                f = s.makefile("rwb")
                f.write(b"SELECT orders WHERE customer_id=42\n")
                f.flush()
                reply = f.readline().decode().strip()
                if hold:
                    time.sleep(hold)
                f.write(b"QUIT\n")
                f.flush()
            finally:
                s.close()
        if not reply.startswith("OK"):
            raise RuntimeError(reply or "empty reply from db")
        return reply

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def reply(self, code, body):
            data = (body + "\n").encode()
            self.send_response(code)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def do_GET(self):
            start = time.time()
            if self.path == "/health":
                return self.reply(200, "ok")
            try:
                if debug:
                    log.write("DEBUG", "request %s headers=%r payload=%s" % (self.path, dict(self.headers), "x" * 2048))
                result = query()
                self.reply(200, '{"orders": "%s"}' % result)
            except Exception as e:  # noqa: BLE001 - surface every failure as 500
                try:
                    log.write("ERROR", "GET %s failed: %s" % (self.path, e))
                except OSError:
                    pass
                self.reply(500, '{"error": "%s"}' % e)
            finally:
                if (time.time() - start) > 0.3:
                    try:
                        log.write("WARN", "GET %s took %dms" % (self.path, (time.time() - start) * 1000))
                    except OSError:
                        pass

    log.write("INFO", "app listening on 127.0.0.1:%d db=127.0.0.1:%d pool_size=%s log_level=%s"
              % (port, db_port, cfg.get("pool_size", "4"), cfg.get("log_level", "info")))
    ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()


# ---------------------------------------------------------------- proxy

def run_proxy():
    cfg = conf("proxy")
    log = Log("proxy")
    host, _, port = cfg.get("upstream", "127.0.0.1:8000").partition(":")
    port = int(port or "80")
    timeout = float(cfg.get("upstream_timeout_s", "2"))

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_GET(self):
            try:
                c = http.client.HTTPConnection(host, port, timeout=timeout)
                c.request("GET", self.path)
                r = c.getresponse()
                body = r.read()
                self.send_response(r.status)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
            except socket.timeout:
                log.write("ERROR", "upstream %s:%d timed out" % (host, port))
                self.send_error(504, "upstream timed out")
            except OSError as e:
                log.write("ERROR", "upstream %s:%d unavailable: %s" % (host, port, e))
                self.send_error(502, "bad gateway")

    log.write("INFO", "proxy listening on 127.0.0.1:8080 upstream=%s:%d" % (host, port))
    ThreadingHTTPServer(("127.0.0.1", 8080), Handler).serve_forever()


# ---------------------------------------------------------------- loadgen

def run_loadgen():
    """Open-loop load: requests start on schedule regardless of how slow the
    previous ones are, so slow or saturated backends build real concurrency
    (as real users do) instead of politely waiting."""
    rps = int(conf("loadgen").get("rps", "8"))
    out = os.path.join(LOG_DIR, "traffic.log")

    def one(results, lock):
        t0 = time.time()
        ok = False
        try:
            c = http.client.HTTPConnection("127.0.0.1", 8080, timeout=3)
            c.request("GET", "/api/orders")
            ok = c.getresponse().status < 400
        except OSError:
            ok = False
        with lock:
            results.append((ok, (time.time() - t0) * 1000))

    def batch(second):
        results, lock, threads = [], threading.Lock(), []
        for i in range(rps):
            target = second + i / rps
            delay = target - time.time()
            if delay > 0:
                time.sleep(delay)
            t = threading.Thread(target=one, args=(results, lock), daemon=True)
            t.start()
            threads.append(t)
        for t in threads:
            t.join(4)
        with lock:
            snapshot = list(results)
        lat = sorted(ms for _, ms in snapshot)
        ok = sum(1 for good, _ in snapshot if good)
        err = rps - ok
        p50 = lat[len(lat) // 2] if lat else 0
        p95 = lat[min(len(lat) - 1, int(len(lat) * 0.95))] if lat else 3000
        try:
            with open(out, "a") as f:
                f.write("%d %d %d %d %d %d\n" % (int(time.time()), rps, ok, err, p50, p95))
            if os.path.getsize(out) > 200_000:
                with open(out) as f:
                    tail = f.readlines()[-300:]
                with open(out, "w") as f:
                    f.writelines(tail)
        except OSError:
            pass

    while True:
        second = int(time.time()) + 1
        time.sleep(max(0, second - time.time()))
        threading.Thread(target=batch, args=(second,), daemon=True).start()


ROLES = {"db": run_db, "app": run_app, "proxy": run_proxy, "loadgen": run_loadgen}

if __name__ == "__main__":
    role = sys.argv[1] if len(sys.argv) > 1 else ""
    if role not in ROLES:
        sys.exit("usage: stack.py db|app|proxy|loadgen")
    os.makedirs(LOG_DIR, exist_ok=True)
    try:
        ROLES[role]()
    except OSError as e:
        Log(role).write("FATAL", str(e)) if role != "loadgen" else None
        sys.exit(1)
