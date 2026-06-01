#!/usr/bin/env python3
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]

REQUIRED = {
    "apps/public-web/src/App.tsx": ["Request TestFlight invite", "Full public browsing is not available", "Lawyer review required"],
    "docs/security/threat-model.md": ["## Assets", "## Trust boundaries", "## Key risks and mitigations", "Pre-beta checks"],
    "docs/runbooks/deployment.md": ["## Preconditions", "## Deploy steps", "## Rollback", "## Verification"],
    "docs/runbooks/backups.md": ["## Backup policy", "## Restore drill"],
    "docs/runbooks/monitoring.md": ["## Signals", "## Alerts", "## Privacy"],
    "docs/runbooks/testflight-beta.md": ["## External prerequisites", "## Smoke matrix", "## Known not implemented"],
    "infra/deploy/env.example": ["provided-by-secret-store", "ECHOTRAILS_ENABLE_DEV_MEDIA_WRITES=false"],
}

def main() -> int:
    for rel, snippets in REQUIRED.items():
        path = ROOT / rel
        if not path.exists():
            print(f"missing {rel}", file=sys.stderr)
            return 1
        text = path.read_text(encoding="utf-8")
        for snippet in snippets:
            if snippet not in text:
                print(f"{rel} missing {snippet}", file=sys.stderr)
                return 1
    print("phase5 readiness check passed")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
