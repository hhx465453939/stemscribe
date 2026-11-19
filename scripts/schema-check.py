#!/usr/bin/env python3
"""Validate registry.json against model-registry.schema.json.

Why this exists: verify-contract.sh used to only check that the schema file was
valid JSON. Nothing validated the instances, so registry.json could violate its
own contract indefinitely -- which it did: ffmpeg's build id "N-129046" never
matched the semver-only version pattern, and no test noticed.

Exit codes: 0 pass, 1 violation, 2 missing dependency (jsonschema).
"""
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

try:
    import jsonschema
except ImportError:
    print("schema-check: SKIP (pip install jsonschema to enable)")
    sys.exit(2)

PAIRS = [
    ("docs/schema/model-registry.schema.json", "internal/registry/data/registry.json"),
]

failed = False
for schema_rel, instance_rel in PAIRS:
    schema = json.loads((ROOT / schema_rel).read_text(encoding="utf-8"))
    instance = json.loads((ROOT / instance_rel).read_text(encoding="utf-8"))
    try:
        jsonschema.validate(instance, schema)
    except jsonschema.ValidationError as exc:
        failed = True
        print(f"FAIL {instance_rel} vs {schema_rel}")
        print(f"     at {'/'.join(str(p) for p in exc.absolute_path)}: {exc.message}")
    else:
        print(f"ok   {instance_rel} conforms to {schema_rel}")

if failed:
    sys.exit(1)
print("schema-check passed")
