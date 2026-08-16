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
    "docs/runbooks/testflight-beta.md": ["## External prerequisites", "## Smoke matrix", "## Remaining engineering validation"],
    "docs/runbooks/field-demo.md": ["## Preconditions", "## Deploy NUC", "## Deploy Almaz moderation", "## Rollback"],
    "infra/deploy/env.example": ["provided-by-secret-store", "ECHOTRAILS_ENABLE_DEV_MEDIA_WRITES=false"],
    "apps/ios/project.yml": ["EchoTrailsUITests", "Validate Release Configuration", "FieldTest", "tv.subcult.EchoTrails.fieldtest"],
    "infra/deploy/compose.nuc.yaml": ["10.0.0.56:18080:8080", "ECHOTRAILS_APNS_ENABLED: \"false\""],
    "infra/deploy/compose.almaz-moderation.yaml": ["10.0.0.200:18081:8081", "/mnt/impuls/echotrails/model-cache"],
    "infra/deploy/Caddyfile.echotrails": ["api.echotrails.subcult.tv", "import authelia"],
    "infra/deploy/almaz-echotrails-firewall.sh": ["ALMAZ-ECHOTRAILS-GUARD", "--ctorigdstport"],
    "apps/ios/AppUITests/EchoTrailsUITests.swift": ["testAuthenticationEntryPointsAreReachable", "testAuthenticatedReleaseSurfacesAreReachable"],
    "apps/ios/App/AppModel.swift": ["APNsTokenPrivacy.idempotencyKey(for: token)", "OpenAPIPaths.logoutSession"],
}

FORBIDDEN = {
    "apps/api/cmd/api/main.go": ["ECHOTRAILS_USE_MEMORY_STORES", "ECHOTRAILS_AUTH_EMAIL_DEV"],
    "apps/api/internal/api/server.go": ["http.Error("],
    "apps/api/internal/api/handler.go": ["http.Error("],
    "apps/worker/cmd/worker/main.go": ["ECHOTRAILS_USE_MEMORY_WORKER"],
    "apps/ios/App/AppModel.swift": ["register-device-\\(token)"],
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
    for rel, snippets in FORBIDDEN.items():
        path = ROOT / rel
        text = path.read_text(encoding="utf-8")
        for snippet in snippets:
            if snippet in text:
                print(f"{rel} contains forbidden release runtime path {snippet}", file=sys.stderr)
                return 1
    print("phase5 readiness check passed")
    return 0

if __name__ == "__main__":
    raise SystemExit(main())
