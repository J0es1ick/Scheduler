from __future__ import annotations

import argparse
import json

from .client import ConnectorClient, ConnectorConfig
from .validation import validate_snapshot


def main() -> None:
    parser = argparse.ArgumentParser(prog="scheduler-connector-python")
    commands = parser.add_subparsers(dest="command", required=True)
    validate = commands.add_parser("validate")
    validate.add_argument("snapshot")
    push = commands.add_parser("push")
    push.add_argument("config")
    push.add_argument("snapshot")
    status = commands.add_parser("status")
    status.add_argument("config")
    status.add_argument("run_id")
    heartbeat = commands.add_parser("heartbeat")
    heartbeat.add_argument("config")
    arguments = parser.parse_args()
    if arguments.command == "validate":
        payload = _json(arguments.snapshot)
        validate_snapshot(payload)
        print("valid")
    elif arguments.command == "push":
        result = ConnectorClient(ConnectorConfig.from_file(arguments.config)).submit(_json(arguments.snapshot))
        print(json.dumps(result, ensure_ascii=False, indent=2))
    elif arguments.command == "status":
        result = ConnectorClient(ConnectorConfig.from_file(arguments.config)).status(arguments.run_id)
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        result = ConnectorClient(ConnectorConfig.from_file(arguments.config)).heartbeat()
        print(json.dumps(result, ensure_ascii=False, indent=2))


def _json(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as stream:
        return json.load(stream)
