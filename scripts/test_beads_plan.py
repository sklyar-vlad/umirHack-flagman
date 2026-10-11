import tempfile
from pathlib import Path
import unittest

from beads_plan import counts, render, write_plan


class BeadsPlanTests(unittest.TestCase):
    def test_only_closed_tasks_are_checked(self):
        tasks = [
            {"id": f"umi-{index}", "title": "Форма\nвхода", "status": status}
            for index, status in enumerate(["closed", "open", "in_progress", "blocked", "deferred"])
        ]
        plan = render(tasks)
        self.assertEqual(counts(plan), (5, 1, 4))
        self.assertIn("- [ ] umi-3: Форма входа (blocked)", plan)

    def test_checkbox_whitespace_works_without_gnu_awk(self):
        self.assertEqual(counts("- [x] Done\n\t* [ ] Pending\n  + [ ] Blocked"), (3, 1, 2))

    def test_invalid_plan_does_not_pass_validation(self):
        for text in ["", "✨ No open issues", "✓ umi-f02", "- [] task"]:
            with self.subTest(text=text), self.assertRaises(ValueError):
                counts(text)

    def test_failed_sync_preserves_previous_plan(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "plan.md"
            path.write_text("previous plan")
            with self.assertRaises(ValueError):
                write_plan(path, [])
            self.assertEqual(path.read_text(), "previous plan")
            write_plan(path, [{"id": "umi-f02", "title": "UI kit", "status": "closed"}])
            self.assertEqual(counts(path.read_text()), (1, 1, 0))
            self.assertEqual(list(Path(directory).iterdir()), [path])


if __name__ == "__main__":
    unittest.main()
