"""Deliver completion notifications without putting credentials in project state."""
import fcntl
import json
import os
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def credentials():
    secrets = Path.home() / ".local/share/ralph-codex/secrets"
    token_file = Path(os.environ.get("TELEGRAM_TOKEN_FILE", str(secrets / "telegram-token")))
    chat_file = Path(os.environ.get("TELEGRAM_CHAT_ID_FILE", str(secrets / "telegram-chat-id")))
    if not token_file.exists() or not chat_file.exists():
        return None
    return token_file.read_text().strip(), chat_file.read_text().strip()


def send_message(text):
    configured = credentials()
    if not configured:
        return False
    token, chat_id = configured
    request = urllib.request.Request(
        "https://api.telegram.org/bot" + token + "/sendMessage",
        data=json.dumps({"chat_id": chat_id, "text": text[:4096]}).encode(),
        headers={"Content-Type": "application/json"}, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=12) as response:
            result = json.load(response)
        if not result.get("ok"):
            print("Telegram delivery pending: API rejected the message.", file=sys.stderr)
            return False
        return True
    except urllib.error.HTTPError as error:
        print(f"Telegram delivery pending: HTTP {error.code}.", file=sys.stderr)
    except (OSError, ValueError):
        # Transport exceptions may contain the request URL and bot token.
        print("Telegram delivery pending: connection unavailable.", file=sys.stderr)
    return False


def flush_pending(issue=None, commit=None, limit=None):
    if not credentials():
        return
    state = ROOT / ".ralph"
    state.mkdir(exist_ok=True)
    with (state / "notifications.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        outbox = state / "notifications.json"
        messages = json.loads(outbox.read_text()) if outbox.exists() else {}
        if issue:
            key = issue["id"]
            text = "✅ Выполнено: " + issue["title"] + "\nПроверки пройдены. Задача отмечена выполненной в YouGile."
            if commit:
                text += "\nКоммит: " + commit + "\nСтори перенесена в Done."
            messages.setdefault(key, {"text": text, "sent_at": None})
            save(outbox, messages)
        attempted = 0
        for message in messages.values():
            if message["sent_at"] is not None:
                continue
            if limit is not None and attempted >= limit:
                break
            attempted += 1
            if send_message(message["text"]):
                message["sent_at"] = int(time.time())
                save(outbox, messages)


def save(path, value):
    temporary = path.with_suffix(".tmp")
    temporary.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")
    temporary.replace(path)


def notify_completed(issue, commit=None):
    try:
        flush_pending(issue, commit)
    except (OSError, ValueError):
        print("Telegram notification storage unavailable; task completion is preserved.", file=sys.stderr)


if __name__ == "__main__":
    flush_pending()
