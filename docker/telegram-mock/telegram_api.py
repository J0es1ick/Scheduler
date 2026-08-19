import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class TelegramAPIHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)

        method = self.path.rsplit("/", 1)[-1]
        if method == "getMe":
            result = {
                "id": 123456,
                "is_bot": True,
                "first_name": "Scheduler CI",
                "username": "schedule_free_bot",
            }
        elif method == "getUpdates":
            time.sleep(0.25)
            result = []
        else:
            result = True

        body = json.dumps({"ok": True, "result": result}).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format, *_args):
        return


ThreadingHTTPServer(("0.0.0.0", 8080), TelegramAPIHandler).serve_forever()
