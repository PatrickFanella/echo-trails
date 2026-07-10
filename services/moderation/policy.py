from dataclasses import dataclass
import json
import re


@dataclass(frozen=True)
class SafetyScores:
    text_disallowed: float = 0.0
    vision_disallowed: float = 0.0
    nsfw: float = 0.0


@dataclass(frozen=True)
class SafetyDecision:
    status: str
    risk_level: str
    categories: dict[str, bool]


def decide(scores: SafetyScores, approve_max: float = 0.02, reject_min: float = 0.98) -> SafetyDecision:
    """Conservative three-way policy: uncertain results always need review."""
    values = {
        "text_safety": clamp(scores.text_disallowed),
        "vision_safety": clamp(scores.vision_disallowed),
        "sexual_content": clamp(scores.nsfw),
    }
    maximum = max(values.values())
    categories = {name: score >= reject_min for name, score in values.items()}
    if maximum >= reject_min:
        return SafetyDecision("rejected", "high", categories)
    if maximum <= approve_max:
        return SafetyDecision("approved", "low", categories)
    return SafetyDecision("needs_review", "medium", categories)


def clamp(value: float) -> float:
    return max(0.0, min(float(value), 1.0))


def guard_probability(output) -> float:
    """Read the final explicit ShieldGemma verdict; ambiguity stays uncertain."""
    serialized = json.dumps(output).lower()
    verdicts = re.findall(r"\b(unsafe|safe|yes|no)\b", serialized)
    if not verdicts:
        return 0.5
    if verdicts[-1] in {"unsafe", "yes"}:
        return 0.99
    if verdicts[-1] in {"safe", "no"}:
        return 0.01
    return 0.5
