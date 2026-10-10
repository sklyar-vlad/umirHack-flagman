"""Mirror one frontend story at a time from YouGile into Beads.

Credentials are inherited through YOUGILE_TOKEN, never saved in project state.
All mutations are limited to the selected story and its existing subtasks.
"""
import asyncio
import html
import json
import os
import re
import subprocess
import sys
from pathlib import Path

from mcp import ClientSession
from mcp.client.streamable_http import streamablehttp_client

PROJECT = os.environ["YOUGILE_PROJECT_ID"]
BOARD = os.environ["YOUGILE_BOARD_ID"]
TODO = os.environ["YOUGILE_TODO_COLUMN_ID"]
PROGRESS = os.environ["YOUGILE_PROGRESS_COLUMN_ID"]
DONE = os.environ["YOUGILE_DONE_COLUMN_ID"]
USER = os.environ["YOUGILE_USER_ID"]
ROOT = Path(__file__).resolve().parent.parent


def command(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def bd(*args):
    return command("bd", "--sandbox", *args)


def issues():
    return json.loads(bd("list", "--all", "--limit", "0", "--json"))


def plaintext(description):
    return html.unescape(re.sub(r"<[^>]+>", "", description.replace("</p>", "\n")))


def dependency_numbers(text):
    numbers = set()
    for match in re.finditer(r"F(\d+)(?:\s*[–—-]\s*F?(\d+))?", text):
        first = int(match[1])
        last = int(match[2]) if match[2] else first
        if last < first or first < 1 or last > 31:
            raise RuntimeError("Invalid frontend dependency range")
        numbers.update(range(first, last + 1))
    return sorted(numbers)


async def call(session, tool, arguments):
    result = await session.call_tool(tool, arguments)
    if result.isError:
        raise RuntimeError(f"YouGile {tool} failed")
    for content in result.content:
        if content.type == "text":
            value = json.loads(content.text)
            if isinstance(value, dict) and value.get("error"):
                raise RuntimeError(f"YouGile {tool} returned an error")
            return value
    raise RuntimeError(f"YouGile {tool} returned no data")


def update(task_id, column=None, complete=None):
    return {"id": task_id, "title": None, "columnId": column,
            "description": None, "color": None, "complete": complete, "archived": None}


async def assign(session, task_id):
    await call(session, "set_assigned", {"id": task_id, "addMode": "add", "users": USER})


async def prepare(session):
    local = issues()
    pending = [i for i in local if i.get("external_ref", "").startswith("yougile:") and i["status"] != "closed"]
    if pending:
        if any(i['status'] == 'blocked' for i in pending):
            raise RuntimeError('Current story is blocked; inspect Beads before restarting')
        stories = [i for i in pending if i["issue_type"] == "story"]
        print(json.dumps({"status": "active", "stories": [{"id": i["id"], "title": i["title"]} for i in stories]}, ensure_ascii=False))
        return
    code = """return {stories:tasks.filter(t=>t.projectId==='%s'&&t.boardId==='%s'&&!t.archived&&/^F\\d+\\./.test(t.title)).map(t=>({id:t.id,title:t.title,columnId:t.columnId,complete:t.complete,assignedUserIds:t.assignedUserIds,description:t.description,subtasks:t.getSubtasks().map(s=>({id:s.id,title:s.title,complete:s.complete,description:s.description}))}))};""" % (PROJECT, BOARD)
    data = await call(session, "query_tasks", {"jsCode": code, "cursor": ""})
    # query_tasks wraps non-array results in `result` on some server versions.
    data = data.get("result", data)
    remote = data["stories"]
    if len(remote) != 31:
        raise RuntimeError(f"Expected all 31 frontend stories, received {len(remote)}; refusing to change the queue")
    for task in remote:
        if task["columnId"] == PROGRESS and task.get("assignedUserIds") and USER not in task["assignedUserIds"]:
            raise RuntimeError(f"{task['title']} is assigned to another person")
    candidates = [t for t in remote if t["columnId"] in (TODO, PROGRESS) and not t["complete"]]
    candidates.sort(key=lambda t: int(re.match(r"F(\d+)", t["title"])[1]))
    if not candidates:
        if any(not t["complete"] and t["columnId"] != DONE for t in remote):
            raise RuntimeError("Unfinished frontend tasks exist outside the configured queue")
        print(json.dumps({"status": "drained"}))
        return
    story = candidates[0]
    description = plaintext(story["description"])
    dependencies = re.search(r"Зависимости FE:\s*([^\n.]+)", description)
    if dependencies:
        for number in dependency_numbers(dependencies[1]):
            prerequisite = next((t for t in remote if re.match(r"F0*" + str(int(number)) + r"\.", t["title"])), None)
            if prerequisite is None or not (prerequisite["complete"] or prerequisite["columnId"] == DONE):
                raise RuntimeError(f"{story['title']} depends on unfinished F{number}")
    number = int(re.match(r"F(\d+)", story["title"])[1])
    issue_id = f"umi-f{number:02d}"
    external = "yougile:" + story["id"]
    metadata = json.dumps({"base_commit": command("git", "rev-parse", "HEAD"), "yougile_id": story["id"]})
    bd("create", story["title"], "--id", issue_id, "--type", "story", "--external-ref", external,
       "--description", description, "--metadata", metadata)
    for task in sorted(story["subtasks"], key=lambda t: t["title"]):
        sub_id = bd("create", task["title"], "--parent", issue_id, "--silent",
                    "--external-ref", "yougile:" + task["id"], "--description", plaintext(task["description"]) or task["title"])
        if task["complete"]:
            bd("close", sub_id, "--reason", "Already completed in YouGile")
        await assign(session, task["id"])
    await assign(session, story["id"])
    bd("update", issue_id, "--claim")
    await call(session, "manage_tasks", {"tasks": [update(story["id"], PROGRESS)]})
    print(json.dumps({"status": "claimed", "id": issue_id, "title": story["title"]}, ensure_ascii=False))


def verify():
    subprocess.run(["bash", "scripts/frontend-checks.sh"], cwd=ROOT, check=True)


async def complete(session, issue_id, story=False):
    issue = json.loads(bd("show", issue_id, "--json"))[0]
    external = issue.get("external_ref", "")
    if not external.startswith("yougile:"):
        raise RuntimeError("Issue has no YouGile reference")
    remote_id = external.split(":", 1)[1]
    full = await call(session, "get_task_data", {"id": remote_id, "cursor": ""})
    task = full["taskFullInfo"]
    if task.get("projectId") != PROJECT or task.get("boardId") != BOARD:
        # Boardless subtasks report empty project/board fields. Validate their
        # actual parent relationship before mutating the subtask.
        authorized = False
        if not story:
            for parent_id in task.get("parentTaskIds", []):
                parent = (await call(session, "get_task_data", {"id": parent_id, "cursor": ""}))["taskFullInfo"]
                if parent.get("projectId") == PROJECT and parent.get("boardId") == BOARD and remote_id in parent.get("subtasks", []):
                    authorized = True
                    break
        if not authorized:
            raise RuntimeError("Task is outside the authorized board")
    verify()
    if story:
        if issue["issue_type"] != "story":
            raise RuntimeError("Expected a story")
        if command("git", "status", "--porcelain", "--untracked-files=normal"):
            raise RuntimeError("Commit all story changes before finishing")
        metadata = issue.get("metadata") or {}
        if isinstance(metadata, str):
            metadata = json.loads(metadata)
        if not metadata.get("base_commit") or command("git", "rev-parse", "HEAD") == command("git", "rev-parse", metadata["base_commit"]):
            raise RuntimeError("A new story commit is required")
        number = re.match(r"F\d+", issue["title"])[0]
        subjects = command("git", "log", metadata["base_commit"] + "..HEAD", "--format=%s")
        if len(subjects.splitlines()) != 1 or not re.match(r"^(feat|fix|refactor|test|docs|chore|perf|style)(\([^)]*\))?!?: ", subjects):
            raise RuntimeError("Exactly one conventional commit is required for the current story")
        if not re.search(r"\b" + re.escape(number) + r"\b", subjects):
            raise RuntimeError(f"Include {number} in the story commit subject")
        children = json.loads(bd("list", "--parent", issue_id, "--all", "--limit", "0", "--json"))
        if any(child["status"] != "closed" for child in children):
            raise RuntimeError("Complete all subtasks first")
        await call(session, "manage_tasks", {"tasks": [update(remote_id, DONE, True)]})
    else:
        if issue["issue_type"] == "story":
            raise RuntimeError("Use finish for a story")
        await call(session, "manage_tasks", {"tasks": [update(remote_id, complete=True)]})
    bd("close", issue_id, "--reason", "Verified" + (" and committed" if story else ""))
    if story:
        print(json.dumps({"status":"finished", "id":issue_id}))


async def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "prepare"
    if mode == "prepare":
        active = [i for i in issues() if i.get("external_ref", "").startswith("yougile:") and i["status"] != "closed"]
        if active:
            if any(i['status'] == 'blocked' for i in active):
                raise RuntimeError('Current story is blocked; inspect Beads before restarting')
            print(json.dumps({"status": "active", "issues": [{"id": i["id"], "title": i["title"]} for i in active]}, ensure_ascii=False))
            return
    token = os.environ["YOUGILE_TOKEN"].strip()
    async with streamablehttp_client("https://ru.yougile.com/data/ai/mcp", headers={"Authorization": "Bearer " + token}) as (read, write, _):
        async with ClientSession(read, write) as session:
            await session.initialize()
            if mode == "prepare":
                await prepare(session)
            elif mode in ("subtask", "finish") and len(sys.argv) == 3:
                await complete(session, sys.argv[2], story=mode == "finish")
            else:
                raise RuntimeError("Usage: yougile-queue.sh prepare | subtask <bd-id> | finish <bd-story-id>")


if __name__ == "__main__":
    try:
        asyncio.run(asyncio.wait_for(main(), 300))
    except Exception as error:
        # Do not print transport exceptions or request headers containing secrets.
        message = str(error).replace(os.environ.get("YOUGILE_TOKEN", ""), "[REDACTED]")
        if isinstance(error, BaseExceptionGroup):
            def causes(group):
                for item in group.exceptions:
                    if isinstance(item, BaseExceptionGroup):
                        yield from causes(item)
                    else:
                        yield type(item).__name__ + ": " + str(item)
            message = "; ".join(causes(error)).replace(os.environ.get("YOUGILE_TOKEN", ""), "[REDACTED]")
        message = re.sub(r"ygmcp_[A-Za-z0-9_-]+", "[REDACTED]", message)
        print(message, file=sys.stderr)
        sys.exit(1)
