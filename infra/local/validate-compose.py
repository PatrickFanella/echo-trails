#!/usr/bin/env python3
"""Lightweight fallback validation for the local Docker Compose file.

Docker Compose performs the authoritative validation when available. This script
keeps `make verify` deterministic on Linux runners that do not provide Docker.
It intentionally checks only repo-owned invariants and obvious syntax hazards.
"""

from __future__ import annotations

import re
import sys
from pathlib import Path


REQUIRED_SNIPPETS = (
    "services:",
    "  postgres:",
    "postgis/postgis:",
    "  redis:",
    "redis:",
    "  object-storage:",
    "minio/minio:",
    "  object-storage-init:",
    "volumes:",
    "  postgres-data:",
    "  redis-data:",
    "  object-storage-data:",
)

FORBIDDEN_PRODUCTION_WORDS = ("production", "prod-beta", "staging")


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: validate-compose.py <compose-file>", file=sys.stderr)
        return 2

    compose_path = Path(sys.argv[1])
    if not compose_path.is_file():
        print(f"missing compose file: {compose_path}", file=sys.stderr)
        return 1

    text = compose_path.read_text(encoding="utf-8")
    missing = [snippet for snippet in REQUIRED_SNIPPETS if snippet not in text]
    if missing:
        print("compose file missing required local service entries:", file=sys.stderr)
        for snippet in missing:
            print(f"  - {snippet}", file=sys.stderr)
        return 1

    for line_number, line in enumerate(text.splitlines(), start=1):
        if "\t" in line:
            print(f"tabs are not allowed in compose YAML: line {line_number}", file=sys.stderr)
            return 1
        if line.rstrip() != line:
            print(f"trailing whitespace in compose YAML: line {line_number}", file=sys.stderr)
            return 1

    lowered = text.lower()
    leaked_words = [word for word in FORBIDDEN_PRODUCTION_WORDS if word in lowered]
    if leaked_words:
        print(
            "local compose must not contain staging/production environment names: "
            + ", ".join(leaked_words),
            file=sys.stderr,
        )
        return 1

    if not re.search(r"POSTGRES_PASSWORD:\s+echotrails_dev_password", text):
        print("local Postgres must use the documented non-secret dev password", file=sys.stderr)
        return 1
    if not re.search(r"MINIO_ROOT_PASSWORD:\s+echotrails_dev_password", text):
        print("local MinIO must use the documented non-secret dev password", file=sys.stderr)
        return 1

    print(f"local compose fallback check passed: {compose_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
