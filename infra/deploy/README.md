# EchoTrails Deployment Infrastructure

Target location for deployment scaffolding and environment documentation.

Keep production credentials, signing material, host-specific secrets, and deploy hooks out of git unless a later issue explicitly defines a safe, secret-free configuration boundary.

Current Phase 5 contents are secret-free templates only:

- `env.example` documents required environment keys without secret values.
- `monitoring-dashboard.fixture.json` documents expected dashboard signals.

No live deployment, alert, backup, or restore has been executed from these files.
