# examples/python/full_showcase.py
# A project management assistant — the full protomcp showcase.
#
# Tools:     login, create_task, list_tasks, assign_task, complete_task, search_tasks
# Resources: project://config, project://tasks/{id}, project://team
# Prompts:   standup_report, sprint_review, task_breakdown
# Completions: task IDs, team members, task statuses
#
# Run: pmcp dev examples/python/full_showcase.py

import json
import time
from protomcp import (
    tool, ToolResult, ToolContext, log,
    resource, resource_template, ResourceContent,
    prompt, PromptArg, PromptMessage,
    completion, CompletionResult,
    tool_manager,
)

# ─── In-Memory Data ─────────────────────────────────────────────────────────

PROJECT = {
    "name": "protomcp v2",
    "sprint": "Sprint 14",
    "started": "2026-03-10",
    "ends": "2026-03-24",
}

TEAM = {
    "alice": {"name": "Alice Chen", "role": "Engineering Lead"},
    "bob": {"name": "Bob Park", "role": "Backend Engineer"},
    "carol": {"name": "Carol Diaz", "role": "Frontend Engineer"},
    "dave": {"name": "Dave Kim", "role": "QA Engineer"},
}

TASKS: dict[str, dict] = {
    "PROJ-101": {
        "title": "Implement WebSocket hub",
        "status": "in_progress",
        "assignee": "bob",
        "priority": "high",
        "description": "Build the WebSocket event broadcasting system for the playground backend.",
    },
    "PROJ-102": {
        "title": "Design trace panel UI",
        "status": "in_progress",
        "assignee": "carol",
        "priority": "high",
        "description": "Create the protocol trace visualization panel with color-coded entries and expand/collapse.",
    },
    "PROJ-103": {
        "title": "Write e2e test suite",
        "status": "todo",
        "assignee": "dave",
        "priority": "medium",
        "description": "End-to-end tests covering tool calls, resource reads, and prompt gets through the full stack.",
    },
    "PROJ-104": {
        "title": "Add sampling support",
        "status": "done",
        "assignee": "alice",
        "priority": "high",
        "description": "Bidirectional sampling: SDK process requests LLM calls from the MCP client.",
    },
    "PROJ-105": {
        "title": "Performance benchmarks",
        "status": "todo",
        "assignee": None,
        "priority": "low",
        "description": "Measure latency overhead of the bridge layer vs direct SDK calls.",
    },
}

_next_id = 106
_authenticated = False


# ─── Resources ───────────────────────────────────────────────────────────────

@resource(
    uri="project://config",
    description="Current project and sprint configuration",
    mime_type="application/json",
)
def project_config(uri: str) -> ResourceContent:
    return ResourceContent(uri=uri, text=json.dumps(PROJECT, indent=2), mime_type="application/json")


@resource(
    uri="project://team",
    description="Team members and their roles",
    mime_type="application/json",
)
def team_info(uri: str) -> ResourceContent:
    team_list = [{"id": k, **v} for k, v in TEAM.items()]
    return ResourceContent(uri=uri, text=json.dumps(team_list, indent=2), mime_type="application/json")


@resource_template(
    uri_template="project://tasks/{task_id}",
    description="Read a specific task by ID (e.g. PROJ-101)",
    mime_type="application/json",
)
def read_task(uri: str) -> ResourceContent:
    task_id = uri.replace("project://tasks/", "")
    task = TASKS.get(task_id)
    if not task:
        return ResourceContent(uri=uri, text=json.dumps({"error": f"Task {task_id} not found"}))
    return ResourceContent(
        uri=uri,
        text=json.dumps({"id": task_id, **task}, indent=2),
        mime_type="application/json",
    )


# ─── Prompts ─────────────────────────────────────────────────────────────────

@prompt(
    description="Generate a standup report for a team member",
    arguments=[
        PromptArg(name="member", description="Team member ID (e.g. alice, bob)", required=True),
    ],
)
def standup_report(member: str) -> list[PromptMessage]:
    person = TEAM.get(member, {"name": member, "role": "Unknown"})
    their_tasks = [
        f"- [{t['status']}] {tid}: {t['title']}"
        for tid, t in TASKS.items()
        if t.get("assignee") == member
    ]
    task_list = "\n".join(their_tasks) if their_tasks else "  (no tasks assigned)"
    return [
        PromptMessage(
            role="user",
            content=f"Generate a standup report for {person['name']} ({person['role']}).\n\nTheir current tasks:\n{task_list}\n\nInclude: what they did yesterday, what they're doing today, any blockers.",
        ),
    ]


@prompt(
    description="Write a sprint review summary",
    arguments=[
        PromptArg(name="include_metrics", description="Include velocity metrics (true/false)"),
    ],
)
def sprint_review(include_metrics: str = "true") -> list[PromptMessage]:
    done = [f"- {tid}: {t['title']}" for tid, t in TASKS.items() if t["status"] == "done"]
    in_progress = [f"- {tid}: {t['title']}" for tid, t in TASKS.items() if t["status"] == "in_progress"]
    todo = [f"- {tid}: {t['title']}" for tid, t in TASKS.items() if t["status"] == "todo"]

    summary = f"Sprint: {PROJECT['sprint']} ({PROJECT['started']} to {PROJECT['ends']})\n\n"
    summary += f"Done ({len(done)}):\n" + ("\n".join(done) or "  (none)") + "\n\n"
    summary += f"In Progress ({len(in_progress)}):\n" + ("\n".join(in_progress) or "  (none)") + "\n\n"
    summary += f"Todo ({len(todo)}):\n" + ("\n".join(todo) or "  (none)")

    messages = [PromptMessage(role="user", content=f"Write a sprint review summary based on this data:\n\n{summary}")]
    if include_metrics.lower() == "true":
        total = len(TASKS)
        velocity = len(done) / max(total, 1) * 100
        messages.append(
            PromptMessage(role="user", content=f"Also include: velocity is {velocity:.0f}% ({len(done)}/{total} tasks complete)")
        )
    return messages


@prompt(
    description="Break down a feature into implementation tasks",
    arguments=[
        PromptArg(name="feature", description="Feature to break down", required=True),
        PromptArg(name="complexity", description="Expected complexity: small, medium, large"),
    ],
)
def task_breakdown(feature: str, complexity: str = "medium") -> list[PromptMessage]:
    return [
        PromptMessage(
            role="user",
            content=f"Break down this feature into implementation tasks:\n\nFeature: {feature}\nComplexity: {complexity}\n\nFor each task, provide: title, description, estimated effort, and suggested assignee from the team (alice=lead, bob=backend, carol=frontend, dave=qa).",
        ),
    ]


# ─── Completions ─────────────────────────────────────────────────────────────

@completion("ref/prompt", "standup_report", "member")
def complete_member(value: str) -> CompletionResult:
    matches = [m for m in TEAM if m.startswith(value)]
    return CompletionResult(values=matches, total=len(matches))


@completion("ref/prompt", "sprint_review", "include_metrics")
def complete_bool(value: str) -> list[str]:
    return [v for v in ["true", "false"] if v.startswith(value)]


@completion("ref/prompt", "task_breakdown", "complexity")
def complete_complexity(value: str) -> list[str]:
    return [v for v in ["small", "medium", "large"] if v.startswith(value)]


@completion("ref/resource", "project://tasks/{task_id}", "task_id")
def complete_task_id(value: str) -> CompletionResult:
    matches = [tid for tid in TASKS if tid.startswith(value)]
    return CompletionResult(values=matches, total=len(matches))


# ─── Tools ───────────────────────────────────────────────────────────────────

@tool(
    "Authenticate to unlock project management tools",
    title="Login",
    read_only=True,
)
def login(token: str) -> ToolResult:
    global _authenticated
    if token == "admin":
        _authenticated = True
        log.info("User authenticated successfully")
        return ToolResult(
            result="Authenticated. Project management tools are now available.",
            enable_tools=["create_task", "assign_task", "complete_task"],
        )
    return ToolResult(
        result="Invalid token. Use 'admin' to authenticate.",
        is_error=True,
        error_code="AUTH_FAILED",
        suggestion="Try: login with token 'admin'",
    )


@tool(
    "List all tasks in the current sprint, optionally filtered by status",
    title="List Tasks",
    read_only=True,
)
def list_tasks(status: str = "") -> ToolResult:
    log.debug(f"Listing tasks with filter: status={status or 'all'}")
    filtered = {
        tid: t for tid, t in TASKS.items()
        if not status or t["status"] == status
    }
    result = []
    for tid, t in filtered.items():
        assignee = TEAM.get(t["assignee"], {}).get("name", "Unassigned") if t["assignee"] else "Unassigned"
        result.append({
            "id": tid,
            "title": t["title"],
            "status": t["status"],
            "assignee": assignee,
            "priority": t["priority"],
        })
    return ToolResult(result=json.dumps(result, indent=2))


@tool(
    "Search tasks by keyword in title or description",
    title="Search Tasks",
    read_only=True,
)
def search_tasks(ctx: ToolContext, query: str) -> ToolResult:
    log.info(f"Searching tasks for: {query}")
    matches = {}
    items = list(TASKS.items())
    for i, (tid, t) in enumerate(items):
        ctx.report_progress(i + 1, len(items), f"Searching {tid}...")
        if query.lower() in t["title"].lower() or query.lower() in t["description"].lower():
            matches[tid] = {
                "title": t["title"],
                "status": t["status"],
                "match_in": "title" if query.lower() in t["title"].lower() else "description",
            }
        time.sleep(0.05)  # Simulate work

    return ToolResult(result=json.dumps(matches, indent=2))


@tool(
    "Create a new task in the current sprint",
    title="Create Task",
    destructive=True,
    hidden=True,
)
def create_task(title: str, description: str, priority: str = "medium") -> ToolResult:
    global _next_id
    if not _authenticated:
        return ToolResult(result="Not authenticated", is_error=True, error_code="UNAUTHORIZED")

    task_id = f"PROJ-{_next_id}"
    _next_id += 1
    TASKS[task_id] = {
        "title": title,
        "description": description,
        "status": "todo",
        "assignee": None,
        "priority": priority,
    }
    log.info(f"Created task {task_id}: {title}")
    return ToolResult(result=json.dumps({"id": task_id, "title": title, "status": "todo"}))


@tool(
    "Assign a task to a team member",
    title="Assign Task",
    hidden=True,
)
def assign_task(task_id: str, member: str) -> ToolResult:
    if not _authenticated:
        return ToolResult(result="Not authenticated", is_error=True, error_code="UNAUTHORIZED")
    if task_id not in TASKS:
        return ToolResult(result=f"Task {task_id} not found", is_error=True, error_code="NOT_FOUND")
    if member not in TEAM:
        return ToolResult(
            result=f"Unknown member '{member}'",
            is_error=True,
            error_code="INVALID_MEMBER",
            suggestion=f"Valid members: {', '.join(TEAM.keys())}",
        )

    TASKS[task_id]["assignee"] = member
    log.info(f"Assigned {task_id} to {TEAM[member]['name']}")
    return ToolResult(result=json.dumps({
        "task": task_id,
        "assignee": TEAM[member]["name"],
        "role": TEAM[member]["role"],
    }))


@tool(
    "Mark a task as complete",
    title="Complete Task",
    destructive=True,
    hidden=True,
)
def complete_task(ctx: ToolContext, task_id: str) -> ToolResult:
    if not _authenticated:
        return ToolResult(result="Not authenticated", is_error=True, error_code="UNAUTHORIZED")
    if task_id not in TASKS:
        return ToolResult(result=f"Task {task_id} not found", is_error=True, error_code="NOT_FOUND")

    ctx.report_progress(1, 3, "Validating task...")
    time.sleep(0.2)
    ctx.report_progress(2, 3, "Running completion checks...")
    time.sleep(0.2)
    ctx.report_progress(3, 3, "Marking complete")

    old_status = TASKS[task_id]["status"]
    TASKS[task_id]["status"] = "done"
    log.info(f"Completed {task_id} (was: {old_status})")
    return ToolResult(result=json.dumps({
        "task": task_id,
        "previous_status": old_status,
        "new_status": "done",
    }))


if __name__ == "__main__":
    from protomcp.runner import run
    run()
