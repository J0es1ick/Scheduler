from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
import base64
import gzip
import hashlib
import json
import secrets
import time
from urllib import error, parse, request

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

from .validation import validate_snapshot


class ConnectorError(RuntimeError):
    pass


@dataclass(frozen=True)
class ConnectorConfig:
    base_url: str
    connector_id: str
    key_id: str
    private_key: str

    @classmethod
    def from_file(cls, path: str) -> "ConnectorConfig":
        with open(path, "r", encoding="utf-8") as stream:
            return cls(**json.load(stream))


class ConnectorClient:
    def __init__(
        self,
        config: ConnectorConfig,
        timeout: float = 30.0,
        max_attempts: int = 4,
        base_delay: float = 0.5,
    ):
        self.config = config
        self.timeout = timeout
        self.max_attempts = max(1, max_attempts)
        self.base_delay = max(0.01, base_delay)
        key = base64.urlsafe_b64decode(_pad(config.private_key))
        self.private_key = Ed25519PrivateKey.from_private_bytes(key[:32])

    def submit(self, snapshot: dict) -> dict:
        validate_snapshot(snapshot)
        raw = json.dumps(snapshot, ensure_ascii=False, separators=(",", ":")).encode()
        body = gzip.compress(raw) if len(raw) >= 4096 else raw
        path = f"/api/v1/connectors/{parse.quote(self.config.connector_id, safe='')}/snapshots"
        headers = {"Content-Type": "application/json", "Idempotency-Key": snapshot["snapshot_id"]}
        if body is not raw:
            headers["Content-Encoding"] = "gzip"
        return self._request("POST", path, body, headers)

    def status(self, run_id: str) -> dict:
        path = f"/api/v1/connectors/{parse.quote(self.config.connector_id, safe='')}/runs/{parse.quote(run_id, safe='')}"
        return self._request("GET", path, b"", {})

    def heartbeat(self) -> dict:
        path = f"/api/v1/connectors/{parse.quote(self.config.connector_id, safe='')}/heartbeat"
        return self._request("POST", path, b"{}", {"Content-Type": "application/json"})

    def _request(self, method: str, path: str, body: bytes, headers: dict[str, str]) -> dict:
        target = self.config.base_url.rstrip("/") + path
        last_error: Exception | None = None
        for attempt in range(self.max_attempts):
            signed_headers = dict(headers)
            timestamp = datetime.now(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")
            nonce = secrets.token_urlsafe(18)
            digest = hashlib.sha256(body).hexdigest()
            canonical = "\n".join((method, path, timestamp, nonce, digest)).encode()
            signature = base64.urlsafe_b64encode(self.private_key.sign(canonical)).decode().rstrip("=")
            signed_headers.update({
                "X-Scheduler-Key-ID": self.config.key_id,
                "X-Scheduler-Timestamp": timestamp,
                "X-Scheduler-Nonce": nonce,
                "X-Scheduler-Content-SHA256": digest,
                "X-Scheduler-Signature": signature,
            })
            http_request = request.Request(
                target,
                data=body if method != "GET" else None,
                method=method,
                headers=signed_headers,
            )
            retry_after: float | None = None
            try:
                with request.urlopen(http_request, timeout=self.timeout) as response:
                    return json.load(response)
            except error.HTTPError as exc:
                payload = exc.read().decode("utf-8", errors="replace")
                try:
                    message = json.loads(payload).get("error", payload)
                except json.JSONDecodeError:
                    message = payload
                last_error = ConnectorError(f"Scheduler HTTP {exc.code}: {message}")
                retryable = exc.code == 429 or exc.code >= 500
                raw_retry_after = exc.headers.get("Retry-After", "")
                try:
                    retry_after = float(raw_retry_after)
                except ValueError:
                    retry_after = None
                if not retryable:
                    raise last_error from exc
            except error.URLError as exc:
                last_error = ConnectorError(f"Scheduler connection failed: {exc.reason}")

            if attempt == self.max_attempts - 1:
                raise last_error
            delay = retry_after if retry_after and retry_after > 0 else min(
                5.0,
                self.base_delay * (2**attempt) + secrets.randbelow(100) / 1000,
            )
            time.sleep(delay)
        raise last_error or ConnectorError("Scheduler request failed")


def _pad(value: str) -> str:
    return value + "=" * (-len(value) % 4)
