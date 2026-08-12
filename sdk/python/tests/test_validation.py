import unittest

from scheduler_connector.validation import SnapshotValidationError, validate_snapshot


def valid_snapshot() -> dict:
    return {
        "schema_version": "1.0",
        "snapshot_id": "snapshot-1",
        "generated_at": "2026-08-06T12:00:00Z",
        "institution": {
            "external_id": "demo",
            "name": "Demo University",
            "timezone": "Europe/Moscow",
        },
        "term": {
            "external_id": "2026-fall",
            "name": "Fall 2026",
            "starts_on": "2026-09-01",
            "ends_on": "2027-01-31",
        },
        "groups": [{
            "external_id": "group-1",
            "name": "1/1",
            "lessons": [{
                "external_id": "lesson-1",
                "subject": "Mathematics",
                "type": "lecture",
                "schedule": {
                    "day_of_week": 1,
                    "starts_at": "09:00",
                    "ends_at": "10:30",
                    "recurrence": {"kind": "every"},
                },
            }],
        }],
    }


class ValidationTest(unittest.TestCase):
    def test_valid_snapshot(self) -> None:
        validate_snapshot(valid_snapshot())

    def test_duplicate_lesson_is_rejected(self) -> None:
        snapshot = valid_snapshot()
        snapshot["groups"][0]["lessons"].append(snapshot["groups"][0]["lessons"][0].copy())
        with self.assertRaises(SnapshotValidationError):
            validate_snapshot(snapshot)


if __name__ == "__main__":
    unittest.main()
