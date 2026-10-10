import json
import unittest
from unittest.mock import AsyncMock, patch

import yougile_queue as queue


class QueueTests(unittest.IsolatedAsyncioTestCase):
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


if __name__ == "__main__":
    unittest.main()
