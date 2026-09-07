import json
import copy
from pathlib import Path
import unittest

from scheduler_connector.validation import SnapshotValidationError, validate_snapshot


class ConformanceTests(unittest.TestCase):
    def test_shared_limits(self):
        root = Path(__file__).resolve().parents[3] / "testdata"
        base = json.loads((root / "connector/valid-example.json").read_text(encoding="utf-8"))
        cases = json.loads((root / "connector-limits.json").read_text(encoding="utf-8"))
        for case in cases:
            with self.subTest(case=case["name"]):
                snapshot = copy.deepcopy(base)
                snapshot["groups"] = [{**base["groups"][0], "external_id": f"g{i}", "name": f"g{i}", "lessons": []} for i in range(case["groups"])]
                snapshot["groups"][0]["lessons"] = [{**base["groups"][0]["lessons"][0], "external_id": f"l{i}"} for i in range(case["lessons"])]
                if case["valid"]:
                    validate_snapshot(snapshot)
                else:
                    with self.assertRaises(SnapshotValidationError):
                        validate_snapshot(snapshot)

    def test_shared_corpus(self):
        files = sorted((Path(__file__).resolve().parents[3] / "testdata" / "connector").glob("*.json"))
        self.assertTrue(files)
        for path in files:
            with self.subTest(case=path.name):
                value = json.loads(path.read_text(encoding="utf-8"))
                if path.name.startswith("valid-"):
                    validate_snapshot(value)
                else:
                    with self.assertRaises(SnapshotValidationError):
                        validate_snapshot(value)
