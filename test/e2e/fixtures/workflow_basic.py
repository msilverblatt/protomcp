from protomcp import tool, ToolResult
from protomcp.workflow import workflow, step
from protomcp.runner import run

@workflow("deploy", description="Deployment workflow")
class DeployWorkflow:
    def __init__(self):
        self.approved = False
        self.reviewed = False

    @step("review", description="Review the deployment", initial=True, next=["approve", "reject"])
    def review(self, pr_number: str) -> str:
        self.reviewed = True
        return f"Reviewed PR #{pr_number}"

    @step("approve", description="Approve the deployment", next=["execute"])
    def approve(self) -> str:
        self.approved = True
        return "Deployment approved"

    @step("reject", description="Reject the deployment", terminal=True)
    def reject(self, reason: str) -> str:
        return f"Deployment rejected: {reason}"

    @step("execute", description="Execute the deployment", terminal=True)
    def execute(self) -> str:
        if not self.approved:
            return "ERROR: not approved"
        return "Deployment executed successfully"

@tool(description="Check server status")
def status() -> str:
    return "all systems operational"

if __name__ == "__main__":
    run()
