import json
import unittest
from pathlib import Path

from jsonschema import Draft202012Validator, FormatChecker


class SchemaTests(unittest.TestCase):
    def test_schema_formats_and_valid_documents(self):
        root = Path(__file__).resolve().parents[3]
        schema = json.loads((root / "docs/schema/schedule-snapshot-v1.json").read_text(encoding="utf8"))
        Draft202012Validator.check_schema(schema)
        validator = Draft202012Validator(schema, format_checker=FormatChecker())
        structural = {"invalid-term-name", "invalid-generated-date", "invalid-schedule-url", "invalid-boolean-subgroup", "invalid-string-day", "invalid-date", "invalid-null-term", "invalid-array-institution", "invalid-number-name", "invalid-metadata-number", "invalid-cycle-weeks-type", "invalid-null-rooms", "invalid-missing-lessons", "invalid-empty-timezone"}
        for path in sorted((root / "testdata/connector").glob("*.json")):
            if not (path.stem.startswith("valid-") or path.stem.startswith("invalid-missing-") or path.stem in structural):
                continue
            with self.subTest(document=path.name):
                errors = list(validator.iter_errors(json.loads(path.read_text(encoding="utf8"))))
                self.assertEqual(not errors, path.stem.startswith("valid-"), str(errors[:1]))
