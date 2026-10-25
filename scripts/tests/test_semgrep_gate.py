from contextlib import redirect_stdout
from copy import deepcopy
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import semgrep_gate


def finding(rule_id="security-rule", **metadata):
    return {
        "check_id": rule_id,
        "path": "app.py",
        "start": {"line": 5},
        "extra": {
            "message": "Unsafe operation",
            "severity": "WARNING",
            "metadata": {
                "category": "security",
                "confidence": "HIGH",
                "subcategory": ["vuln"],
                **metadata,
            },
        },
    }


def reports(findings, errors=None):
    report = {"version": "1.182.0", "results": findings, "errors": errors or []}
    rules = [
        {"id": rule_id, "defaultConfiguration": {"level": "warning"}}
        for rule_id in dict.fromkeys(item["check_id"] for item in findings)
    ]
    sarif = {
        "version": "2.1.0",
        "runs": [{
            "tool": {"driver": {"name": "Semgrep", "rules": rules}},
            "results": [
                {"ruleId": item["check_id"], "message": {"text": "Unsafe operation"}}
                for item in findings
            ],
        }],
    }
    return report, sarif


class FindingPolicyTests(unittest.TestCase):
    def test_high_confidence_vulnerabilities_block(self):
        for severity in ("WARNING", "ERROR", "MEDIUM", "HIGH", "CRITICAL"):
            with self.subTest(severity=severity):
                item = finding()
                item["extra"]["severity"] = severity
                self.assertTrue(semgrep_gate.is_blocking(item))

    def test_ambiguous_findings_and_recommendations_warn(self):
        cases = [
            {"confidence": "MEDIUM"},
            {"confidence": "LOW"},
            {"confidence": None},
            {"category": "best-practice"},
            {"subcategory": ["audit"]},
            {"subcategory": ["audit", "vuln"]},
            {"subcategory": []},
            {"subcategory": None},
            {"subcategory": {"vuln": True}},
        ]
        for metadata in cases:
            with self.subTest(metadata=metadata):
                item = finding(**metadata)
                item["extra"]["severity"] = "ERROR"
                self.assertFalse(semgrep_gate.is_blocking(item))

    def test_incomplete_metadata_warns(self):
        for field in ("category", "confidence", "subcategory"):
            with self.subTest(field=field):
                item = finding()
                del item["extra"]["metadata"][field]
                self.assertFalse(semgrep_gate.is_blocking(item))
        for metadata in (None, {}, [], "HIGH"):
            with self.subTest(metadata=metadata):
                item = finding()
                item["extra"]["metadata"] = metadata
                self.assertFalse(semgrep_gate.is_blocking(item))

    def test_low_or_unknown_severity_warns(self):
        for severity in ("INFO", "LOW", "INVENTORY", None, ""):
            with self.subTest(severity=severity):
                item = finding()
                item["extra"]["severity"] = severity
                self.assertFalse(semgrep_gate.is_blocking(item))

    def test_metadata_normalization(self):
        item = finding(category=" Security ", confidence=" high ", subcategory=" VuLn ")
        item["extra"]["severity"] = " warning "
        self.assertTrue(semgrep_gate.is_blocking(item))

    def test_sarif_keeps_all_findings_and_distinguishes_levels(self):
        report, sarif = reports([
            finding("vulnerability"),
            finding("recommendation", subcategory=["audit"]),
            finding("ambiguous", confidence="MEDIUM"),
        ])
        original = deepcopy(sarif)
        normalized, summary = semgrep_gate.classify_report(report, sarif)
        self.assertEqual(summary, {"blocking": 1, "warnings": 2, "scanner_errors": 0})
        run = normalized["runs"][0]
        self.assertEqual([item["level"] for item in run["results"]], ["error", "warning", "warning"])
        self.assertEqual([item["properties"]["ci_blocking"] for item in run["results"]], [True, False, False])
        self.assertEqual([rule["defaultConfiguration"]["level"] for rule in run["tool"]["driver"]["rules"]], ["error", "warning", "warning"])
        self.assertEqual(sarif, original)

    def test_repeated_rule_counts_are_preserved(self):
        report, sarif = reports([finding(), finding()])
        _, summary = semgrep_gate.classify_report(report, sarif)
        self.assertEqual(summary["blocking"], 2)
        sarif["runs"][0]["results"].pop()
        with self.assertRaisesRegex(ValueError, "do not match"):
            semgrep_gate.classify_report(report, sarif)

    def test_conflicting_rule_metadata_is_rejected(self):
        report, sarif = reports([finding(), finding(confidence="LOW")])
        with self.assertRaisesRegex(ValueError, "Conflicting"):
            semgrep_gate.classify_report(report, sarif)

    def test_unknown_sarif_finding_is_rejected(self):
        report, sarif = reports([finding()])
        sarif["runs"][0]["results"][0]["ruleId"] = "another-rule"
        with self.assertRaisesRegex(ValueError, "unmatched"):
            semgrep_gate.classify_report(report, sarif)


class GateIntegrationTests(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        environment = patch.dict(os.environ, {
            "GITHUB_OUTPUT": str(self.root / "output"),
            "GITHUB_STEP_SUMMARY": str(self.root / "summary"),
        })
        environment.start()
        self.addCleanup(environment.stop)

    def write_reports(self, findings, errors=None):
        report, sarif = reports(findings, errors)
        for name, data in (("semgrep.json", report), ("semgrep.raw.sarif", sarif)):
            (self.root / name).write_text(json.dumps(data), encoding="utf-8")

    def run_gate(self):
        output = io.StringIO()
        with patch.object(sys, "argv", ["semgrep_gate.py", str(self.root)]), redirect_stdout(output):
            status = semgrep_gate.main()
        return status, output.getvalue()

    def test_clean_scan_passes(self):
        self.write_reports([])
        status, output = self.run_gate()
        self.assertEqual(status, 0)
        self.assertIn("0 blocking, 0 warnings, 0 scanner errors", output)
        self.assertEqual((self.root / "output").read_text(), "report=true\n")

    def test_recommendation_only_passes_with_warning(self):
        self.write_reports([finding(subcategory=["audit"])])
        status, output = self.run_gate()
        self.assertEqual(status, 0)
        self.assertIn("::warning title=Semgrep,file=app.py,line=5::", output)
        self.assertNotIn("::error", output)
        normalized = json.loads((self.root / "semgrep.sarif").read_text())
        self.assertEqual(normalized["runs"][0]["results"][0]["level"], "warning")

    def test_vulnerability_fails_but_publishes_reports(self):
        self.write_reports([finding()])
        status, output = self.run_gate()
        self.assertEqual(status, 1)
        self.assertIn("::error title=Semgrep,file=app.py,line=5::", output)
        self.assertEqual((self.root / "output").read_text(), "report=true\n")
        self.assertIn("1 blocking, 0 warnings", (self.root / "summary").read_text())
        self.assertEqual(json.loads((self.root / "semgrep-policy.json").read_text())["blocking"], 1)

    def test_scanner_errors_fail_even_without_findings(self):
        self.write_reports([], [{"message": "Could not parse source file"}])
        status, output = self.run_gate()
        self.assertEqual(status, 2)
        self.assertIn("::error title=Semgrep::Scanner error: Could not parse source file", output)

    def test_missing_or_corrupt_reports_fail(self):
        self.write_reports([])
        (self.root / "semgrep.json").unlink()
        for content in (None, "invalid json", "{}"):
            with self.subTest(content=content):
                if content is not None:
                    (self.root / "semgrep.json").write_text(content)
                status, output = self.run_gate()
                self.assertEqual(status, 2)
                self.assertIn("Cannot evaluate scan reports", output)
                self.assertFalse((self.root / "output").exists())

    def test_mismatched_reports_fail_without_publishing(self):
        self.write_reports([finding()])
        _, empty_sarif = reports([])
        (self.root / "semgrep.raw.sarif").write_text(json.dumps(empty_sarif))
        status, output = self.run_gate()
        self.assertEqual(status, 2)
        self.assertIn("do not match", output)
        self.assertFalse((self.root / "output").exists())

    def test_annotations_escape_untrusted_text(self):
        item = finding(subcategory=["audit"])
        item["path"] = "C:/app,a%\r\n::error::injected.py"
        item["extra"]["message"] = "Advice%\r\n::error::injected"
        self.write_reports([item])
        status, output = self.run_gate()
        self.assertEqual(status, 0)
        self.assertEqual(len(output.splitlines()), 2)
        self.assertIn("file=C%3A/app%2Ca%25%0D%0A%3A%3Aerror%3A%3Ainjected.py", output)
        self.assertIn("Advice%25%0D%0A::error::injected", output)

    def test_invalid_line_cannot_inject_an_annotation(self):
        item = finding()
        item["start"]["line"] = "5\n::error::injected"
        self.write_reports([item])
        status, output = self.run_gate()
        self.assertEqual(status, 2)
        self.assertNotIn("injected", output)
        self.assertFalse((self.root / "output").exists())


if __name__ == "__main__":
    unittest.main()
