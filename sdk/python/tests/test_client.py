import base64
import io
import unittest
from unittest.mock import patch
from urllib.error import HTTPError

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

from scheduler_connector.client import ConnectorClient, ConnectorConfig


class ConnectorClientTest(unittest.TestCase):
    def test_transient_http_error_is_retried(self):
        private_key = Ed25519PrivateKey.generate().private_bytes_raw()
        encoded = base64.urlsafe_b64encode(private_key).decode().rstrip("=")
        client = ConnectorClient(
            ConnectorConfig("https://scheduler.test", "connector", "key", encoded),
            max_attempts=2,
            base_delay=0.01,
        )
        unavailable = HTTPError(
            "https://scheduler.test/test",
            503,
            "unavailable",
            {},
            io.BytesIO(b'{"error":"temporary"}'),
        )
        with patch(
            "scheduler_connector.client.request.urlopen",
            side_effect=[unavailable, io.BytesIO(b'{"status":"ok"}')],
        ) as urlopen:
            result = client._request("GET", "/test", b"", {})

        self.assertEqual(result["status"], "ok")
        self.assertEqual(urlopen.call_count, 2)
        first_nonce = urlopen.call_args_list[0].args[0].get_header("X-scheduler-nonce")
        second_nonce = urlopen.call_args_list[1].args[0].get_header("X-scheduler-nonce")
        self.assertNotEqual(first_nonce, second_nonce)


if __name__ == "__main__":
    unittest.main()
