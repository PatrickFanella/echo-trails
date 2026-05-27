#!/usr/bin/env python3
"""Small dependency-free OpenAPI baseline validator.

The contract is intentionally stored as JSON-compatible YAML so repo baseline
checks do not need network access or generated caches during scaffold phase.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import Any


def fail(message: str) -> None:
    print(f"OpenAPI lint failed: {message}", file=sys.stderr)
    raise SystemExit(1)


def require_mapping(value: Any, path: str) -> dict[str, Any]:
    if not isinstance(value, dict):
        fail(f"{path} must be an object")
    return value


def require_string(value: Any, path: str) -> str:
    if not isinstance(value, str) or not value.strip():
        fail(f"{path} must be a non-empty string")
    return value


def walk_refs(value: Any, refs: set[str]) -> None:
    if isinstance(value, dict):
        ref = value.get("$ref")
        if isinstance(ref, str):
            refs.add(ref)
        for child in value.values():
            walk_refs(child, refs)
    elif isinstance(value, list):
        for child in value:
            walk_refs(child, refs)


def resolve_local_ref(spec: dict[str, Any], ref: str) -> bool:
    if not ref.startswith("#/"):
        return False
    current: Any = spec
    for raw_part in ref[2:].split("/"):
        part = raw_part.replace("~1", "/").replace("~0", "~")
        if not isinstance(current, dict) or part not in current:
            return False
        current = current[part]
    return True


def lint(path: Path) -> None:
    try:
        spec = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as err:
        fail(f"{path} must be JSON-compatible YAML: {err}")
        return

    root = require_mapping(spec, "root")
    openapi = require_string(root.get("openapi"), "openapi")
    if not openapi.startswith("3."):
        fail("openapi version must be 3.x")
    info = require_mapping(root.get("info"), "info")
    require_string(info.get("title"), "info.title")
    require_string(info.get("version"), "info.version")

    paths = require_mapping(root.get("paths"), "paths")
    if not paths:
        fail("paths must not be empty")
    for path_name, path_item in paths.items():
        if not path_name.startswith("/"):
            fail(f"path {path_name!r} must start with /")
        path_ops = require_mapping(path_item, f"paths.{path_name}")
        operations = {k: v for k, v in path_ops.items() if k in {"get", "post", "put", "patch", "delete"}}
        if not operations:
            fail(f"{path_name} must define at least one operation")
        for method, operation in operations.items():
            op = require_mapping(operation, f"paths.{path_name}.{method}")
            require_string(op.get("operationId"), f"paths.{path_name}.{method}.operationId")
            responses = require_mapping(op.get("responses"), f"paths.{path_name}.{method}.responses")
            if not responses:
                fail(f"{path_name} {method} must define responses")
            if not any(str(code)[0] in {"2", "3"} for code in responses):
                fail(f"{path_name} {method} must define a success response")

    components = require_mapping(root.get("components"), "components")
    schemas = require_mapping(components.get("schemas"), "components.schemas")
    required_schemas = {"ErrorResponse", "HealthResponse", "AuthTokenResponse", "Place", "Echo"}
    missing = sorted(required_schemas.difference(schemas))
    if missing:
        fail(f"missing required schemas: {', '.join(missing)}")

    refs: set[str] = set()
    walk_refs(root, refs)
    unresolved = sorted(ref for ref in refs if not resolve_local_ref(root, ref))
    if unresolved:
        fail(f"unresolved refs: {', '.join(unresolved)}")

    print(f"OpenAPI lint passed: {path}")


def main() -> None:
    if len(sys.argv) != 2:
        fail("usage: lint-openapi.py <openapi.yaml>")
    lint(Path(sys.argv[1]))


if __name__ == "__main__":
    main()
