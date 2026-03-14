"""
Workflow Example: Deployment Pipeline
=====================================
Demonstrates @workflow with multi-step state machine.
The agent only sees valid next steps at each point.
Run with: pmcp dev -- python workflow_deploy.py
"""
from protomcp import workflow, step, StepResult, tool, ToolResult
from protomcp.runner import run


@workflow("deploy", allow_during=["status"])
class DeployWorkflow:
    def __init__(self):
        self.pr_url = None
        self.test_results = None

    @step(initial=True, next=["approve", "reject"],
          description="Review changes before deployment")
    def review(self, pr_url: str) -> StepResult:
        self.pr_url = pr_url
        return StepResult(result=f"Reviewing {pr_url}: 5 files changed")

    @step(next=["run_tests"],
          description="Approve the changes for deployment")
    def approve(self, reason: str) -> StepResult:
        return StepResult(result=f"Approved: {reason}")

    @step(terminal=True,
          description="Reject the changes")
    def reject(self, reason: str) -> StepResult:
        return StepResult(result=f"Rejected: {reason}")

    @step(next=["promote", "rollback"], no_cancel=True,
          description="Run test suite against staging")
    def run_tests(self) -> StepResult:
        self.test_results = {"passed": 42, "failed": 0}
        if self.test_results["failed"] == 0:
            return StepResult(result="All 42 tests passed", next=["promote"])
        return StepResult(result="Tests failed", next=["rollback"])

    @step(terminal=True, no_cancel=True,
          description="Deploy to production")
    def promote(self) -> StepResult:
        return StepResult(result=f"Deployed {self.pr_url} to production")

    @step(terminal=True,
          description="Roll back staging deployment")
    def rollback(self) -> StepResult:
        return StepResult(result="Rolled back staging")

    def on_cancel(self, current_step, history):
        return f"Deploy cancelled at step '{current_step}'"

    def on_complete(self, history):
        steps = " -> ".join(s[0] for s in history)
        print(f"[audit] Deploy complete: {steps}")


@tool("Check deployment status", read_only=True)
def status() -> ToolResult:
    return ToolResult(result="All systems nominal")


if __name__ == "__main__":
    run()
