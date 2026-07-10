import unittest

from policy import SafetyScores, decide, guard_probability


class PolicyTests(unittest.TestCase):
    def test_only_clear_safe_scores_auto_approve(self):
        self.assertEqual(decide(SafetyScores(0.01, 0.02, 0.0)).status, "approved")
        self.assertEqual(decide(SafetyScores(0.021, 0.0, 0.0)).status, "needs_review")

    def test_clear_disallowed_scores_reject(self):
        result = decide(SafetyScores(nsfw=0.99))
        self.assertEqual(result.status, "rejected")
        self.assertTrue(result.categories["sexual_content"])

    def test_unknown_scores_never_fail_open(self):
        self.assertEqual(decide(SafetyScores(0.5, 0.0, 0.0)).status, "needs_review")

    def test_guard_uses_final_verdict_not_prompt_words(self):
        prompt = "Classify as safe or unsafe only"
        self.assertEqual(guard_probability({"prompt": prompt, "generated_text": "safe"}), 0.01)
        self.assertEqual(guard_probability({"prompt": prompt, "generated_text": "unsafe"}), 0.99)
        self.assertEqual(guard_probability({"generated_text": "uncertain"}), 0.5)


if __name__ == "__main__":
    unittest.main()
