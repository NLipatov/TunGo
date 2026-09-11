#!/usr/bin/env python3

import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


PAYLOAD = bytes(range(256)) * (4 * 1024 * 1024 // 256)
CHECKSUM = (hashlib.sha256(PAYLOAD).hexdigest() + "\n").encode("ascii")


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/payload":
            body = PAYLOAD
            content_type = "application/octet-stream"
        elif self.path == "/sha256":
            body = CHECKSUM
            content_type = "text/plain"
        elif self.path == "/peer":
            body = (self.client_address[0] + "\n").encode("ascii")
            content_type = "text/plain"
        else:
            self.send_error(404)
            return

        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
