"""Render Ralph's plan from Beads JSON and validate portable Markdown checkboxes."""

import json
import os
from pathlib import Path
import re
import sys
import tempfile


def render(tasks: list[dict]) -> str:
    if not isinstance(tasks, list) or not tasks:
        raise ValueError("Beads returned no tasks for the execution plan")
    lines = ["# Ralph execution plan", "", "Generated from Beads; edit tasks in Beads, not this mirror.", ""]
    for task in sorted(tasks, key=lambda item: item["id"]):
        identity = " ".join(task["id"].split())
        title = " ".join(task["title"].split())
        status = task["status"]
        if not identity or not title or not isinstance(status, str) or not status:
            raise ValueError("Beads returned a task without an ID, title, or status")
        mark = "x" if status == "closed" else " "
        lines.append(f"- [{mark}] {identity}: {title} ({status})")
    return "\n".join(lines) + "\n"


def counts(text: str) -> tuple[int, int, int]:
    matches = re.findall(r"^\s*[-*+]\s+\[([ x])\]", text, re.MULTILINE)
    if not matches:
        raise ValueError("Execution plan has no checkbox items")
    checked = matches.count("x")
    return len(matches), checked, len(matches) - checked


def write_plan(path: Path, tasks: list[dict]) -> None:
    text = render(tasks)
    counts(text)
    path.parent.mkdir(parents=True, exist_ok=True)
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=path.parent, delete=False) as stream:
            temporary = Path(stream.name)
            stream.write(text)
        os.replace(temporary, path)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def main() -> int:
    try:
        operation, filename = sys.argv[1:]
        path = Path(filename)
        if operation == "sync":
            write_plan(path, json.load(sys.stdin))
        elif operation == "validate":
            print(*counts(path.read_text(encoding="utf-8")))
        else:
            raise ValueError(f"Unknown operation: {operation}")
    except (ValueError, KeyError, TypeError, AttributeError, OSError) as error:
        print(str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
