# Private moderation service

This service runs only on the private moderation host. It downloads private originals directly from S3-compatible storage and exposes authenticated internal endpoints for `faster-whisper`, ShieldGemma, ShieldGemma 2, and OpenNSFW2. Unknown responses and all service/model failures remain private and require human review; callers never fail open.

Production requires a CUDA host, model access prepared before deployment, a random `ECHOTRAILS_MODERATION_SERVICE_TOKEN`, private network ingress, and S3 read-only credentials. Do not expose port 8081 publicly.
## Calibration gate

`python3 calibrate.py` expands the checked-in non-graphic synthetic corpus to
200 cases and verifies the policy thresholds: at least 99% of disallowed cases
must avoid auto-approval, and auto-limited high-risk cases must have at least
95% precision. This offline mode tests policy mechanics with fixed scores.

Before a release candidate, run the same corpus through the loaded models:

```bash
python3 calibrate.py --service-url http://private-moderation-host:8081
```

The command must pass against the exact model revisions and thresholds used by
the release. Set all three `ECHOTRAILS_*_REVISION` values to immutable Hub
commit revisions; production startup and live calibration reject moving
`main`/`master` revisions. A scanner/model outage is a failed gate and runtime requests remain
private in `pending_review`.
