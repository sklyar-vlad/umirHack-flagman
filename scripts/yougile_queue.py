"""Mirror one backend story at a time from YouGile into Beads.

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

PROJECT = "1b6742cb-3f7b-46da-bf4a-9128875e31c7"
BOARD = "bf67c16d-7fa2-42c6-9de4-3d93df641280"
TODO = "e3098b98-92cb-4483-9ece-115bce1de035"
PROGRESS = "262d8d76-b655-4442-9365-fdf0a5f85b02"
DONE = "e9fdc652-adf1-46f7-8ea5-bdcda509f59c"
USER = "70a23043-5e62-46cc-8bfa-adfff0365380"
ROOT = Path(__file__).resolve().parent.parent


def command(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def bd(*args):
    return command("bd", "--sandbox", *args)


def issues():
    return json.loads(bd("list", "--all", "--limit", "0", "--json"))


def plaintext(description):
    return html.unescape(re.sub(r"<[^>]+>", "", description.replace("</p>", "\n")))


async def call(session, tool, arguments):
    result = await session.call_tool(tool, arguments)
    if result.isError:
        raise RuntimeError(f"YouGile {tool} failed")
    for content in result.content:
        if content.type == "text":
            return json.loads(content.text)
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
        stories = [i for i in pending if i["issue_type"] == "story"]
        print(json.dumps({"status": "active", "stories": [{"id": i["id"], "title": i["title"]} for i in stories]}, ensure_ascii=False))
        return
    code = """return {stories:tasks.filter(t=>t.projectId==='%s'&&t.boardId==='%s'&&!t.archived&&/^S\\d+\\./.test(t.title)).map(t=>({id:t.id,title:t.title,columnId:t.columnId,complete:t.complete,description:t.description,subtasks:t.getSubtasks().map(s=>({id:s.id,title:s.title,complete:s.complete,description:s.description}))}))};""" % (PROJECT, BOARD)
    data = await call(session, "query_tasks", {"jsCode": code, "cursor": ""})
    # query_tasks wraps non-array results in `result` on some server versions.
    data = data.get("result", data)
    remote = data["stories"]
    candidates = [t for t in remote if t["columnId"] == TODO and not t["complete"] and int(re.match(r"S(\d+)", t["title"])[1]) >= 8]
    candidates.sort(key=lambda t: int(re.match(r"S(\d+)", t["title"])[1]))
    if not candidates:
        print(json.dumps({"status": "drained"}))
        return
    story = candidates[0]
    description = plaintext(story["description"])
    dependencies = re.search(r"Зависимости:\s*([^\n]+)", description)
    if dependencies:
        for number in re.findall(r"S(\d+)", dependencies[1]):
            prerequisite = next((t for t in remote if re.match(r"S0*" + str(int(number)) + r"\.", t["title"])), None)
            if prerequisite is None or not (prerequisite["complete"] or prerequisite["columnId"] == DONE):
                raise RuntimeError(f"{story['title']} depends on unfinished S{number}")
    number = int(re.match(r"S(\d+)", story["title"])[1])
    issue_id = f"umi-s{number:02d}"
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
    for action in ("test", "vet", "build"):
        subprocess.run(["go", "-C", "backend", action, "./..."], cwd=ROOT, check=True)


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
        number = re.match(r"S\d+", issue["title"])[0]
        subjects = command("git", "log", metadata["base_commit"] + "..HEAD", "--format=%s")
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
        await prepare(session)


async def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "prepare"
    if mode == "prepare":
        active = [i for i in issues() if i.get("external_ref", "").startswith("yougile:") and i["status"] != "closed"]
        if active:
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
            message = "YouGile connection or operation failed; tasks remain unfinished"
        print(message, file=sys.stderr)
        sys.exit(1)
