"""HTTP read-load probe against release-capacity-seed.sql; never proves all v1 gates.

Uses only loopback endpoints and an explicit synthetic access key from environment.
Report excludes cookies, keys, Telegram IDs and response bodies. Run beside isolated
Compose, recording docker stats and queue completion separately. Publication,
interactive Telegram requests, reminders, faults and soak are separate scenarios.
"""
import argparse
import concurrent.futures
import http.cookiejar
import json
import os
import random
import threading
import time
import urllib.request
import urllib.parse
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--admin', default='http://127.0.0.1:18280')
    parser.add_argument('--site', default='http://127.0.0.1:18281')
    parser.add_argument('--seconds', type=int, default=1800)
    parser.add_argument('--clients', type=int, default=20)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    for endpoint in (args.admin, args.site):
        if urllib.parse.urlparse(endpoint).hostname not in ('127.0.0.1', 'localhost'):
            parser.error('isolated loopback endpoints required')
    if not 1 <= args.clients <= 100 or not 1 <= args.seconds <= 86400:
        parser.error('clients must be 1..100; seconds must be 1..86400')
    jar = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar))
    payload = json.dumps({'access_key': os.environ['SCHEDULER_TEST_ACCESS_KEY']}).encode()
    with opener.open(urllib.request.Request(args.admin+'/api/auth/access-key', data=payload, headers={'Content-Type': 'application/json'}), timeout=10) as response:
        json.load(response)
    request = urllib.request.Request(args.admin+'/api/auth/me')
    jar.add_cookie_header(request)
    cookie = request.get_header('Cookie', '')
    latencies, errors = [], []
    lock = threading.Lock()
    started = time.monotonic()

    def client(index):
        rng = random.Random(index)
        while time.monotonic()-started < args.seconds:
            group = rng.randrange(1000)
            url = rng.choices([
                args.admin+f'/api/editor/schedule?group=capacity-g-{group}',
                args.admin+'/api/dashboard',
                args.site+'/api/public-info',
            ], weights=[8, 1, 1])[0]
            begin = time.monotonic()
            try:
                headers = {'Cookie': cookie} if url.startswith(args.admin) else {}
                with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=15) as response:
                    response.read()
                error = None
            except Exception as exc:
                error = getattr(exc, 'code', type(exc).__name__)
            with lock:
                latencies.append(time.monotonic()-begin)
                if error is not None:
                    errors.append(str(error))
            time.sleep(max(0, 1-(time.monotonic()-begin)))

    with concurrent.futures.ThreadPoolExecutor(args.clients) as pool:
        list(pool.map(client, range(args.clients)))
    latencies.sort()
    p95 = latencies[min(len(latencies)-1, int(len(latencies)*.95))]
    report = {'scenario': 'HTTP reads during separately measured queue drain',
              'elapsed_seconds': round(time.monotonic()-started, 2), 'clients': args.clients,
              'requests': len(latencies), 'errors': len(errors), 'error_kinds': sorted(set(errors)),
              'error_rate': len(errors)/len(latencies), 'p95_seconds': round(p95, 4),
              'read_targets_met': p95 <= 1 and len(errors)/len(latencies) < .001}
    args.output.write_text(json.dumps(report, indent=2)+'\n', encoding='utf-8')
    print(json.dumps(report))
    return 0 if report['read_targets_met'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
