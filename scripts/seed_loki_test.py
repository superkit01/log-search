import json
import unittest

import seed_loki


class SeedLokiTest(unittest.TestCase):
    def test_default_options_match_query_ui_filters(self):
        args = seed_loki.parse_args([])

        self.assertEqual(args.env, "test")
        self.assertEqual(args.apps, "service-x,service-y")
        self.assertEqual(args.url, "http://localhost:3100")

    def test_push_url_trims_base_slash(self):
        self.assertEqual(
            seed_loki.push_url("http://localhost:3100/"),
            "http://localhost:3100/loki/api/v1/push",
        )

    def test_build_payload_uses_loki_push_shape(self):
        payload = seed_loki.build_payload(
            apps=["service-x"],
            env="test",
            count=4,
            start_ns=1_000_000_000,
            step_ns=1_000_000,
            seed=7,
        )

        self.assertIn("streams", payload)
        self.assertEqual(len(payload["streams"]), 4)
        for stream in payload["streams"]:
            labels = stream["stream"]
            self.assertEqual(labels["env"], "test")
            self.assertEqual(labels["app"], "service-x")
            self.assertIn(labels["level"], {"debug", "info", "warn", "error"})
            self.assertNotIn("tid", labels)
            self.assertNotIn("trace_id", labels)
            self.assertEqual(len(stream["values"]), 1)
            ts, line, metadata = stream["values"][0]
            self.assertTrue(ts.isdigit())
            self.assertTrue(metadata["tid"].startswith("tid-"))
            parsed = json.loads(line)
            self.assertEqual(parsed["env"], "test")
            self.assertEqual(parsed["app"], "service-x")
            self.assertEqual(parsed["level"], labels["level"])
            self.assertEqual(parsed["tid"], metadata["tid"])
            self.assertNotIn("trace_id", parsed)

    def test_build_payload_distributes_apps(self):
        payload = seed_loki.build_payload(
            apps=["service-x", "service-y"],
            env="test",
            count=8,
            start_ns=1_000_000_000,
            step_ns=1_000_000,
            seed=3,
        )

        apps = {stream["stream"]["app"] for stream in payload["streams"]}
        self.assertEqual(apps, {"service-x", "service-y"})

    def test_build_payload_includes_multiline_messages(self):
        payload = seed_loki.build_payload(
            apps=["service-x"],
            env="test",
            count=12,
            start_ns=1_000_000_000,
            step_ns=1_000_000,
            seed=5,
        )

        messages = [
            json.loads(stream["values"][0][1])["message"]
            for stream in payload["streams"]
        ]
        self.assertTrue(any("\n" in message for message in messages))


if __name__ == "__main__":
    unittest.main()
