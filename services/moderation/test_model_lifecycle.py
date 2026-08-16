import unittest

from model_lifecycle import ModelLifecycle


class ModelLifecycleTests(unittest.TestCase):
    def test_only_one_named_model_is_retained(self):
        released = []
        lifecycle = ModelLifecycle(release=lambda: released.append(True))
        text = object()
        vision = object()

        self.assertIs(lifecycle.activate("text", lambda: text), text)
        self.assertIs(lifecycle.activate("text", lambda: self.fail("reloaded text")), text)
        self.assertIs(lifecycle.activate("vision", lambda: vision), vision)
        self.assertEqual(lifecycle.active_name, "vision")
        self.assertEqual(len(released), 1)

    def test_unload_clears_active_model(self):
        lifecycle = ModelLifecycle(release=lambda: None)
        lifecycle.activate("whisper", object)
        lifecycle.unload()
        self.assertIsNone(lifecycle.active_name)


if __name__ == "__main__":
    unittest.main()
