import json
import time
import threading
from collections import Counter
from urllib.parse import parse_qs
from email.parser import BytesParser
from email.policy import default
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

lock = threading.Lock()
counts = Counter()
seconds = Counter()


class TelegramAPIHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != '/stats':
            self.send_error(404)
            return
        with lock:
            result = {'methods': dict(counts), 'max_sends_per_second': max(seconds.values(), default=0)}
        self.reply(result)

    def reply(self, result):
        body = json.dumps(result).encode()
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        content_type = self.headers.get('Content-Type', '')
        if 'application/json' in content_type:
            params = json.loads(raw or b'{}')
        elif 'multipart/form-data' in content_type:
            message = BytesParser(policy=default).parsebytes(('Content-Type: '+content_type+'\r\n\r\n').encode()+raw)
            params = {part.get_param('name', header='content-disposition'): part.get_payload(decode=True).decode(errors='replace') for part in message.iter_parts() if part.get_content_maintype() == 'text'}
        else:
            params = {key: values[0] for key, values in parse_qs(raw.decode()).items()}

        method = self.path.rsplit("/", 1)[-1]
        with lock:
            counts[method] += 1
            message_id = counts[method]
            if method.startswith('send'):
                seconds[int(time.monotonic())] += 1
                if len(seconds) > 86400:
                    del seconds[min(seconds)]
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
        elif method.startswith('send') or method.startswith('editMessage'):
            result = {'message_id': message_id, 'date': int(time.time()),
                      'chat': {'id': int(params.get('chat_id', 1)), 'type': 'private'},
                      'text': params.get('text', '')}
        else:
            result = True

        self.reply({'ok': True, 'result': result})

    def log_message(self, _format, *_args):
        return


if __name__ == '__main__':
    ThreadingHTTPServer(("0.0.0.0", 8080), TelegramAPIHandler).serve_forever()
