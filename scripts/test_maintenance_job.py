import contextlib
import importlib.util
import io
import pathlib
import unittest
from unittest.mock import patch
import urllib.error

spec = importlib.util.spec_from_file_location("maintenance_driver", pathlib.Path(__file__).with_name("maintenance-job.py"))
driver = importlib.util.module_from_spec(spec)
spec.loader.exec_module(driver)


def job(revision=0, status="ready"):
    return {"id": "test", "revision": revision, "status": status, "next_allowed_at": "2000-01-01T00:00:00Z"}


class DriverTest(unittest.TestCase):
    def test_go_timestamp_fractional_widths(self):
        for fraction in ["", ".1", ".12", ".123", ".1234", ".12345", ".123456"]:
            utc = driver.parse_timestamp("2026-09-15T05:08:34" + fraction + "Z")
            local = driver.parse_timestamp("2026-09-15T13:08:34" + fraction + "+08:00")
            self.assertEqual(utc, local)

    def test_lost_response_retries_original_revision(self):
        responses = [(200, {"job": job()}), urllib.error.URLError("lost response"),
                     (409, {"job": job(1)})]
        with patch.object(driver, "request", side_effect=responses) as request, patch.object(driver.time, "sleep"), contextlib.redirect_stdout(io.StringIO()):
            result = driver.drive("http://127.0.0.1:1234", "test", 1, 2, 10)
        self.assertEqual(result["revision"], 1)
        self.assertEqual(request.call_args_list[1].args[2], {"revision": 0})
        self.assertEqual(request.call_args_list[2].args[2], {"revision": 0})
        self.assertEqual(request.call_count, 3)

    def test_retry_budget_stops_busy_loop(self):
        responses = [(200, {"job": job()})] + [(409, {"error": "busy", "retryable": True})] * 3
        with patch.object(driver, "request", side_effect=responses) as request, patch.object(driver.time, "sleep"), contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaises(RuntimeError):
                driver.drive("http://127.0.0.1:1234", "test", 1, 2, 10)
        self.assertEqual(request.call_count, 4)

    def test_transient_failure_resumes_using_persisted_revision(self):
        responses = [(200, {"job": job()}),
                     (503, {"job": job(1, "paused"), "retryable": True}),
                     (200, {"job": job(2)}), (200, {"job": job(3, "completed")})]
        with patch.object(driver, "request", side_effect=responses) as request, patch.object(driver.time, "sleep"), contextlib.redirect_stdout(io.StringIO()):
            result = driver.drive("http://127.0.0.1:1234", "test", 1, 2, 10)
        self.assertEqual(result["status"], "completed")
        self.assertEqual(request.call_args_list[2].args[2], {"revision": 1})
        self.assertEqual(request.call_args_list[3].args[2], {"revision": 2})

    def test_does_not_resume_operator_pause_automatically(self):
        with patch.object(driver, "request", return_value=(200, {"job": job(1, "paused")})) as request, contextlib.redirect_stdout(io.StringIO()):
            with self.assertRaises(RuntimeError):
                driver.drive("http://127.0.0.1:1234", "test", 1, 3, 10)
        self.assertEqual(request.call_count, 1)


if __name__ == "__main__":
    unittest.main()
