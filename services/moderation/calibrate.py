#!/usr/bin/env python3
"""Evaluate conservative thresholds against synthetic text safety cases."""

import argparse
import json
import os
import urllib.request
from pathlib import Path

from policy import SafetyScores, decide


def service_score(url: str, token: str, text: str, index: int) -> str:
    body = json.dumps({"echo_id": f"calibration-{index}", "title": text[:80], "body": text[80:580], "caption": ""}).encode()
    request = urllib.request.Request(url.rstrip("/") + "/v1/moderate-text", data=body, method="POST", headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"})
    with urllib.request.urlopen(request, timeout=120) as response:
        return json.load(response)["status"]


def service_has_pinned_revisions(url: str) -> bool:
    with urllib.request.urlopen(url.rstrip("/") + "/health", timeout=30) as response:
        evidence = json.load(response)
    models = evidence.get("models", [])
    return evidence.get("revisions_pinned") is True and len(models) == 4 and all(model.strip() for model in models)


def evaluate(corpus: dict, service_url: str | None, token: str | None) -> dict:
    outcomes = []
    index = 0
    for case in corpus["cases"]:
        for suffix in corpus["suffixes"]:
            index += 1
            text = f'{case["text"]} {suffix}.'
            status = service_score(service_url, token, text, index) if service_url else decide(SafetyScores(text_disallowed=case["fixture_score"])).status
            outcomes.append((case["label"], status))
    disallowed = [status for label, status in outcomes if label == "disallowed"]
    rejected = [label for label, status in outcomes if status == "rejected"]
    recall = sum(status != "approved" for status in disallowed) / len(disallowed)
    precision = sum(label == "disallowed" for label in rejected) / len(rejected) if rejected else 0.0
    pinned = service_has_pinned_revisions(service_url) if service_url else None
    return {"cases": len(outcomes), "disallowed_recall": recall, "high_risk_precision": precision, "model_revisions_pinned": pinned, "passed": recall >= 0.99 and precision >= 0.95 and pinned is not False}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--service-url")
    parser.add_argument("--token", default=os.getenv("ECHOTRAILS_MODERATION_SERVICE_TOKEN"))
    args = parser.parse_args()
    if args.service_url and not args.token:
        parser.error("--token or ECHOTRAILS_MODERATION_SERVICE_TOKEN is required with --service-url")
    corpus = json.loads((Path(__file__).parent / "calibration-corpus.json").read_text())
    result = evaluate(corpus, args.service_url, args.token)
    print(json.dumps(result, sort_keys=True))
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
