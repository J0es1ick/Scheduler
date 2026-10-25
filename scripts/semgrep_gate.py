import argparse
from collections import Counter
from copy import deepcopy
import json
import os
from pathlib import Path
import sys


def is_blocking(finding):
    extra = finding["extra"]
    metadata = extra.get("metadata")
    if not isinstance(metadata, dict):
        return False
    subcategories = metadata.get("subcategory", [])
    if isinstance(subcategories, str):
        subcategories = [subcategories]
    if not isinstance(subcategories, list):
        return False
    subcategories = {str(value).strip().lower() for value in subcategories}
    return (
        str(metadata.get("category", "")).strip().lower() == "security"
        and str(metadata.get("confidence", "")).strip().upper() == "HIGH"
        and "vuln" in subcategories
        and "audit" not in subcategories
        and str(extra.get("severity", "")).strip().upper()
        in {"WARNING", "ERROR", "MEDIUM", "HIGH", "CRITICAL"}
    )


def classify_report(report, sarif):
    if not isinstance(report, dict) or not report.get("version"):
        raise ValueError("Missing Semgrep report version")
    findings = report.get("results")
    errors = report.get("errors")
    if not isinstance(findings, list) or not isinstance(errors, list):
        raise ValueError("Missing Semgrep findings or scanner errors")

    decisions = {}
    for finding in findings:
        if not isinstance(finding, dict) or not isinstance(finding.get("extra"), dict):
            raise ValueError("Invalid Semgrep finding")
        for field in ("check_id", "path"):
            if not isinstance(finding.get(field), str) or not finding[field]:
                raise ValueError(f"Missing finding {field}")
        if not isinstance(finding.get("start"), dict):
            raise ValueError("Missing finding location")
        line = finding["start"].get("line")
        if type(line) is not int or line < 1:
            raise ValueError("Invalid finding line")
        blocking = is_blocking(finding)
        rule_id = finding["check_id"]
        if rule_id in decisions and decisions[rule_id] != blocking:
            raise ValueError(f"Conflicting rule metadata: {rule_id}")
        decisions[rule_id] = blocking

    normalized = deepcopy(sarif)
    if not isinstance(normalized, dict) or normalized.get("version") != "2.1.0":
        raise ValueError("Invalid SARIF version")
    runs = normalized.get("runs")
    if not isinstance(runs, list) or not runs:
        raise ValueError("Missing SARIF runs")
    sarif_ids = []
    for run in runs:
        results = run.get("results")
        if not isinstance(results, list):
            raise ValueError("Missing SARIF findings")
        for result in results:
            rule_id = result.get("ruleId")
            if rule_id not in decisions:
                raise ValueError("SARIF contains an unmatched finding")
            blocking = decisions[rule_id]
            result["level"] = "error" if blocking else "warning"
            result.setdefault("properties", {})["ci_blocking"] = blocking
            sarif_ids.append(rule_id)
        for rule in run["tool"]["driver"]["rules"]:
            rule_id = rule["id"]
            if rule_id in decisions:
                rule.setdefault("defaultConfiguration", {})["level"] = (
                    "error" if decisions[rule_id] else "warning"
                )

    if Counter(sarif_ids) != Counter(finding["check_id"] for finding in findings):
        raise ValueError("JSON and SARIF findings do not match")

    blocking_count = sum(is_blocking(finding) for finding in findings)
    summary = {
        "blocking": blocking_count,
        "warnings": len(findings) - blocking_count,
        "scanner_errors": len(errors),
    }
    return normalized, summary


def escape_annotation(value, property_value=False):
    value = str(value).replace("%", "%25").replace("\r", "%0D").replace("\n", "%0A")
    if property_value:
        value = value.replace(":", "%3A").replace(",", "%2C")
    return value


def emit_annotation(level, message, finding=None):
    properties = "title=Semgrep"
    if finding is not None:
        properties += (
            f",file={escape_annotation(finding['path'], True)}"
            f",line={finding['start']['line']}"
        )
    print(f"::{level} {properties}::{escape_annotation(message)}")


def evaluate(report_dir):
    report = json.loads((report_dir / "semgrep.json").read_text(encoding="utf-8"))
    sarif = json.loads((report_dir / "semgrep.raw.sarif").read_text(encoding="utf-8"))
    normalized, summary = classify_report(report, sarif)
    (report_dir / "semgrep.sarif").write_text(
        json.dumps(normalized, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    (report_dir / "semgrep-policy.json").write_text(
        json.dumps(summary, indent=2) + "\n", encoding="utf-8"
    )
    for finding in report["results"]:
        emit_annotation(
            "error" if is_blocking(finding) else "warning",
            f"{finding['check_id']}: {finding['extra'].get('message', '')}",
            finding,
        )
    for error in report["errors"]:
        message = error.get("message", str(error)) if isinstance(error, dict) else str(error)
        emit_annotation("error", f"Scanner error: {message}")

    output = (
        f"Semgrep: {summary['blocking']} blocking, {summary['warnings']} warnings, "
        f"{summary['scanner_errors']} scanner errors."
    )
    print(output)
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with Path(os.environ["GITHUB_STEP_SUMMARY"]).open("a", encoding="utf-8") as stream:
            stream.write(
                "## Semgrep\n\n" + output + "\n\n"
                "Only high-confidence security vulnerabilities with medium or higher "
                "severity block this check. Audit recommendations, ambiguous findings "
                "and incomplete rule metadata remain warnings. Scanner errors fail "
                "the check. All findings are retained in the scan reports.\n"
            )
    if os.environ.get("GITHUB_OUTPUT"):
        with Path(os.environ["GITHUB_OUTPUT"]).open("a", encoding="utf-8") as stream:
            stream.write("report=true\n")
    if summary["scanner_errors"]:
        return 2
    return 1 if summary["blocking"] else 0


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("report_dir", type=Path)
    args = parser.parse_args()
    try:
        return evaluate(args.report_dir)
    except (OSError, ValueError, KeyError, TypeError, AttributeError) as error:
        emit_annotation("error", f"Cannot evaluate scan reports: {error}")
        return 2


if __name__ == "__main__":
    sys.exit(main())
