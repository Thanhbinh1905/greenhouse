import json
import unittest
from pathlib import Path

from tools.posse_review_regression import matches


class RegressionCasesTest(unittest.TestCase):
    def test_cases_cover_expected_misses_false_positive_and_clean_controls(self):
        cases = json.loads((Path(__file__).parent / "posse_review_regression.json").read_text())
        by_pr = {case["pr"]: case for case in cases}
        self.assertEqual(set(by_pr), {"1131", "1146", "1152", "1156", "1158", "1159", "1164", "1165", "1167", "1168", "1241"})
        self.assertEqual(by_pr["1131"]["expected"]["issue"], 1129)
        self.assertEqual(by_pr["1156"]["expected"]["issue"], 1145)
        self.assertEqual(by_pr["1168"]["expected"]["issue"], 1157)
        self.assertEqual(by_pr["1146"]["expected"], {"kind": "no-p1", "path": "scripts/preflight.sh", "start": 44, "end": 45})
        self.assertEqual(by_pr["1241"]["head"], "ad347566e8b01d45e7d7345055b7476ae3247864")
        self.assertEqual(by_pr["1241"]["target"], "e54f20d484aa4b8b3490282c2002f761db0ef739")
        self.assertEqual(by_pr["1241"]["expected"], {"kind": "no-p0-p2", "path": "internal/taskplan/plan.go", "start": 891, "end": 899})
        self.assertEqual(sum(case["expected"]["kind"] == "clean" for case in cases), 6)
        for case in cases:
            self.assertRegex(case["head"], r"^[0-9a-f]{40}$")
            self.assertRegex(case["target"], r"^[0-9a-f]{40}$")
            self.assertTrue(case["intent"])

    def test_expected_findings_require_matching_p1_location(self):
        expected = {"kind": "finding", "path": "x.go", "start": 10, "end": 12, "priority": "P1"}
        finding = {"file": "x.go", "start": 12, "end": 13, "priority": "P1"}
        self.assertTrue(matches(expected, {"verdict": "findings", "findings": [finding]}))
        finding["priority"] = "P2"
        self.assertFalse(matches(expected, {"verdict": "findings", "findings": [finding]}))

    def test_clean_controls_reject_inconclusive_and_no_p1_is_range_specific(self):
        clean = {"kind": "clean"}
        self.assertTrue(matches(clean, {"verdict": "clean"}))
        self.assertFalse(matches(clean, {"verdict": "inconclusive"}))
        no_p1 = {"kind": "no-p1", "path": "preflight.sh", "start": 44, "end": 45}
        other_range = {"file": "preflight.sh", "start": 68, "end": 71, "priority": "P1"}
        target_range = {"file": "preflight.sh", "start": 45, "end": 48, "priority": "P1"}
        self.assertTrue(matches(no_p1, {"verdict": "findings", "findings": [other_range]}))
        self.assertFalse(matches(no_p1, {"verdict": "findings", "findings": [target_range]}))
        self.assertFalse(matches(no_p1, {"verdict": "partial", "findings": []}))
        no_p0_p2 = {"kind": "no-p0-p2", "path": "plan.go", "start": 891, "end": 899}
        p3 = {"file": "plan.go", "start": 891, "end": 899, "priority": "P3"}
        p2 = {"file": "plan.go", "start": 891, "end": 899, "priority": "P2"}
        self.assertTrue(matches(no_p0_p2, {"verdict": "findings", "findings": [p3]}))
        for priority in ("P0", "P1", "P2"):
            finding = {**p2, "priority": priority}
            self.assertFalse(matches(no_p0_p2, {"verdict": "findings", "findings": [finding]}))


if __name__ == "__main__":
    unittest.main()
