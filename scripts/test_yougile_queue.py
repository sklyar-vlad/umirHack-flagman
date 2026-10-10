import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import AsyncMock, patch

import yougile_queue as queue


class QueueTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        for name in ("notify_completed", "flush_pending"):
            replacement = patch.object(queue, name)
            replacement.start()
            self.addCleanup(replacement.stop)
        replacement = patch.object(queue, "working_changes", return_value={})
        self.changes = replacement.start()
        self.addCleanup(replacement.stop)

    async def test_ascending_story_selection_and_assignment(self):
        stories = [
            {"id": "ten", "title": "S10. Later", "columnId": queue.TODO, "complete": False, "description": "", "subtasks": []},
            {"id": "eight", "title": "S08. Done", "columnId": queue.DONE, "complete": True, "description": "", "subtasks": []},
            {"id": "nine", "title": "S09. Next", "columnId": queue.TODO, "complete": False,
             "description": "<p>Зависимости: S08.</p>", "subtasks": [{"id": "child", "title": "S09-T01", "description": "", "complete": False}]},
        ]
        async def remote(session, tool, arguments):
            if tool == "query_tasks":
                return {"stories": stories}
            return {"result": "ok"}
        with patch.object(queue, "issues", return_value=[]), patch.object(queue, "command", return_value="baseline"), \
             patch.object(queue, "bd", return_value="umi-s09.1") as bd, patch.object(queue, "call", side_effect=remote) as call:
            await queue.prepare(None)
            created = [c.args for c in bd.call_args_list if c.args[0] == "create"]
            self.assertEqual(created[0][1], "S09. Next")
            assignments = [c.args[2] for c in call.call_args_list if c.args[1] == "set_assigned"]
            self.assertEqual({a["id"] for a in assignments}, {"nine", "child"})
            self.assertTrue(all(a["users"] == queue.USER for a in assignments))
            mutations = [c.args[2] for c in call.call_args_list if c.args[1] == "manage_tasks"]
            self.assertEqual(mutations[0]["tasks"][0]["columnId"], queue.PROGRESS)

    async def test_unfinished_dependency_does_not_claim_or_move(self):
        stories = [
            {"id": "eight", "title": "S08. Busy", "columnId": queue.PROGRESS, "complete": False, "description": "", "subtasks": []},
            {"id": "nine", "title": "S09. Next", "columnId": queue.TODO, "complete": False, "description": "<p>Зависимости: S08.</p>", "subtasks": []},
        ]
        with patch.object(queue, "issues", return_value=[]), patch.object(queue, "bd") as bd, \
             patch.object(queue, "call", new=AsyncMock(return_value={"stories": stories})) as call:
            with self.assertRaisesRegex(RuntimeError, "unfinished S08"):
                await queue.prepare(None)
            bd.assert_not_called()
            self.assertEqual(call.await_count, 1)

    async def test_boardless_subtask_requires_authorized_parent(self):
        issue = {"id": "umi-s09.1", "issue_type": "task", "external_ref": "yougile:child"}
        async def remote(session, tool, arguments):
            if tool == "get_task_data":
                if arguments["id"] == "child":
                    return {"taskFullInfo": {"projectId": "", "boardId": "", "parentTaskIds": ["parent"]}}
                return {"taskFullInfo": {"projectId": queue.PROJECT, "boardId": queue.BOARD, "subtasks": ["child"]}}
            return {"result": "ok"}
        with patch.object(queue, "bd", return_value=json.dumps([issue])) as bd, \
             patch.object(queue, "verify") as verify, patch.object(queue, "call", side_effect=remote) as call:
            await queue.complete(None, issue["id"])
            verify.assert_called_once()
            self.assertTrue(any(c.args[0] == "close" for c in bd.call_args_list))
            mutations = [c.args[2] for c in call.call_args_list if c.args[1] == "manage_tasks"]
            self.assertTrue(mutations[0]["tasks"][0]["complete"])
            self.assertIsNone(mutations[0]["tasks"][0]["columnId"])

    async def test_failed_verification_never_closes_subtask(self):
        issue = {"id": "umi-s09.1", "issue_type": "task", "external_ref": "yougile:child"}
        with patch.object(queue, "bd", return_value=json.dumps([issue])) as bd, \
             patch.object(queue, "verify", side_effect=RuntimeError("tests failed")), \
             patch.object(queue, "call", new=AsyncMock(return_value={"taskFullInfo": {"projectId": queue.PROJECT, "boardId": queue.BOARD}})) as call:
            with self.assertRaisesRegex(RuntimeError, "tests failed"):
                await queue.complete(None, issue["id"])
            self.assertFalse(any(c.args[0] == "close" for c in bd.call_args_list))
            self.assertFalse(any(c.args[1] == "manage_tasks" for c in call.call_args_list))

    async def test_story_without_new_commit_is_not_closed(self):
        issue = {"id": "umi-s09", "title": "S09. Next", "issue_type": "story", "external_ref": "yougile:story", "metadata": {"base_commit": "abc"}}
        def git(*args):
            return "" if args[1] == "status" else "abcdef"
        with patch.object(queue, "bd", return_value=json.dumps([issue])) as bd, patch.object(queue, "verify"), \
             patch.object(queue, "command", side_effect=git), \
             patch.object(queue, "call", new=AsyncMock(return_value={"taskFullInfo": {"projectId": queue.PROJECT, "boardId": queue.BOARD}})) as call:
            with self.assertRaisesRegex(RuntimeError, "new story commit"):
                await queue.complete(None, issue["id"], story=True)
            self.assertFalse(any(c.args[0] == "close" for c in bd.call_args_list))
            self.assertFalse(any(c.args[1] == "manage_tasks" for c in call.call_args_list))

    async def test_setup_commit_after_story_does_not_prevent_completion(self):
        issue = {"id": "umi-s09", "title": "S09. Next", "issue_type": "story", "external_ref": "yougile:story", "metadata": {"base_commit": "base"}}
        def git(*args):
            if args[1] == "status": return ""
            if args[1] == "log": return "chore(workflow): configure hooks\nfeat(orders): check basket (S09)"
            return "new-head" if args[2] == "HEAD" else "old-base"
        def beads(*args):
            return json.dumps([issue]) if args[0] == "show" else "[]"
        with patch.object(queue, "bd", side_effect=beads), patch.object(queue, "verify"), \
             patch.object(queue, "command", side_effect=git), patch.object(queue, "prepare", new=AsyncMock()) as prepare, \
             patch.object(queue, "call", new=AsyncMock(return_value={"taskFullInfo": {"projectId": queue.PROJECT, "boardId": queue.BOARD}})) as call:
            await queue.complete(None, issue["id"], story=True)
            mutations = [c.args[2] for c in call.call_args_list if c.args[1] == "manage_tasks"]
            self.assertEqual(mutations[0]["tasks"][0]["columnId"], queue.DONE)
            prepare.assert_awaited_once()

    async def test_unrelated_pending_changes_are_preserved_on_completion(self):
        issue = {"id": "umi-s09", "title": "S09. Next", "issue_type": "story", "external_ref": "yougile:story",
                 "metadata": {"base_commit": "base", "baseline_changes": {".gitignore": "unchanged"}}}
        self.changes.return_value = {".gitignore": "unchanged"}
        def git(*args):
            if args[1] == "log": return "feat(orders): check basket (S09)"
            return "new-head" if args[2] in ("HEAD", "--short") else "old-base"
        def beads(*args):
            return json.dumps([issue]) if args[0] == "show" else "[]"
        with patch.object(queue, "bd", side_effect=beads) as bd, patch.object(queue, "verify"), \
             patch.object(queue, "command", side_effect=git), patch.object(queue, "prepare", new=AsyncMock()), \
             patch.object(queue, "call", new=AsyncMock(return_value={"taskFullInfo": {"projectId": queue.PROJECT, "boardId": queue.BOARD}})):
            await queue.complete(None, issue["id"], story=True)
            self.assertTrue(any(c.args[0] == "close" for c in bd.call_args_list))

    async def test_new_or_changed_pending_files_prevent_done(self):
        issue = {"id": "umi-s09", "title": "S09. Next", "issue_type": "story", "external_ref": "yougile:story",
                 "metadata": {"base_commit": "base", "baseline_changes": {".gitignore": "original"}}}
        for changes in ({".gitignore": "changed"}, {"backend/order.go": "uncommitted"}, {}):
            self.changes.return_value = changes
            with patch.object(queue, "bd", return_value=json.dumps([issue])) as bd, patch.object(queue, "verify"), \
                 patch.object(queue, "call", new=AsyncMock(return_value={"taskFullInfo": {"projectId": queue.PROJECT, "boardId": queue.BOARD}})) as call:
                with self.assertRaisesRegex(RuntimeError, "Commit all story changes"):
                    await queue.complete(None, issue["id"], story=True)
                self.assertFalse(any(c.args[0] == "close" for c in bd.call_args_list))
                self.assertFalse(any(c.args[1] == "manage_tasks" for c in call.call_args_list))


class FingerprintTests(unittest.TestCase):
    def test_content_and_index_changes_are_detected(self):
        with tempfile.TemporaryDirectory() as directory, patch.object(queue, "ROOT", Path(directory)):
            root = Path(directory)
            subprocess.run(["git", "init", "-q", directory], check=True)
            target = root / "settings.txt"
            target.write_text("original")
            subprocess.run(["git", "add", "settings.txt"], cwd=root, check=True)
            first = queue.working_changes()
            self.assertEqual(first, queue.working_changes())
            target.write_text("modified")
            second = queue.working_changes()
            self.assertNotEqual(first, second)
            subprocess.run(["git", "add", "settings.txt"], cwd=root, check=True)
            self.assertNotEqual(second, queue.working_changes())


if __name__ == "__main__":
    unittest.main()
