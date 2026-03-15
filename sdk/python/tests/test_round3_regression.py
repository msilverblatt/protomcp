"""Round 3 regression tests for bugs found during stress testing."""

import os
import tempfile

import pytest

from protomcp.tool import tool, get_registered_tools, get_hidden_tool_names, clear_registry
from protomcp.workflow import (
    step,
    workflow,
    StepResult,
    get_registered_workflows,
    clear_workflow_registry,
    workflows_to_tool_defs,
)
from protomcp.group import (
    tool_group,
    action,
    clear_group_registry,
    groups_to_tool_defs,
)
from protomcp.resource import (
    resource,
    resource_template,
    get_registered_resources,
    get_registered_resource_templates,
    clear_resource_registry,
    clear_template_registry,
)
from protomcp.prompt import prompt, get_registered_prompts, clear_prompt_registry, PromptArg, PromptMessage
from protomcp.discovery import configure, discover_handlers, reset_config
from protomcp.runner import _uri_matches_template
from protomcp.local_middleware import clear_local_middleware
from protomcp.server_context import clear_context_registry
from protomcp.telemetry import clear_telemetry_sinks
from protomcp.completion import clear_completion_registry
from protomcp.sidecar import clear_sidecar_registry
from protomcp.middleware import clear_middleware_registry


def _clear_all():
    clear_registry()
    clear_group_registry()
    clear_workflow_registry()
    clear_resource_registry()
    clear_template_registry()
    clear_prompt_registry()
    clear_local_middleware()
    clear_context_registry()
    clear_telemetry_sinks()
    clear_completion_registry()
    clear_sidecar_registry()
    clear_middleware_registry()
    reset_config()


@pytest.fixture(autouse=True)
def clean_registries():
    _clear_all()
    yield
    _clear_all()


# ---------------------------------------------------------------------------
# 1. Hidden tool detection includes workflows
# ---------------------------------------------------------------------------


class TestHiddenToolDetectionIncludesWorkflows:
    def test_workflow_hidden_steps_in_get_hidden_tool_names(self):
        """Register a workflow and verify get_hidden_tool_names() returns the hidden step names."""
        @workflow(name="deploy", description="Deploy workflow")
        class Deploy:
            @step("start", description="Start deploy", initial=True, next=["approve"])
            def start(self, env: str) -> StepResult:
                return StepResult(result=f"Deploying to {env}")

            @step("approve", description="Approve deploy", terminal=True)
            def approve(self) -> StepResult:
                return StepResult(result="Approved")

        hidden = get_hidden_tool_names()
        # Non-initial steps and cancel should be hidden
        assert "deploy.approve" in hidden
        assert "deploy.cancel" in hidden
        # Initial step should NOT be hidden
        assert "deploy.start" not in hidden

    def test_workflow_with_multiple_non_initial_steps(self):
        """All non-initial steps should appear in hidden tool names."""
        @workflow(name="pipeline", description="Pipeline")
        class Pipeline:
            @step("init", description="Init", initial=True, next=["build"])
            def init(self) -> StepResult:
                return StepResult()

            @step("build", description="Build", next=["test"])
            def build(self) -> StepResult:
                return StepResult()

            @step("test", description="Test", next=["deploy"])
            def test_step(self) -> StepResult:
                return StepResult()

            @step("deploy", description="Deploy", terminal=True)
            def deploy(self) -> StepResult:
                return StepResult()

        hidden = get_hidden_tool_names()
        assert "pipeline.build" in hidden
        assert "pipeline.test" in hidden
        assert "pipeline.deploy" in hidden
        assert "pipeline.cancel" in hidden
        assert "pipeline.init" not in hidden


# ---------------------------------------------------------------------------
# 2. Hidden tool detection includes groups with hidden tools
# ---------------------------------------------------------------------------


class TestHiddenToolDetectionIncludesGroups:
    def test_hidden_group_in_get_hidden_tool_names(self):
        """A group registered with hidden=True should appear in get_hidden_tool_names()."""
        @tool_group(name="secret_ops", description="Secret operations", hidden=True)
        class SecretOps:
            @action("do_secret", description="Do secret thing")
            def do_secret(self) -> str:
                return "done"

        hidden = get_hidden_tool_names()
        assert "secret_ops" in hidden

    def test_non_hidden_group_not_in_hidden_names(self):
        """A group registered without hidden=True should NOT appear in hidden tool names."""
        @tool_group(name="public_ops", description="Public operations")
        class PublicOps:
            @action("do_public", description="Do public thing")
            def do_public(self) -> str:
                return "done"

        hidden = get_hidden_tool_names()
        assert "public_ops" not in hidden

    def test_hidden_individual_tool_in_hidden_names(self):
        """An individual tool with hidden=True should appear in get_hidden_tool_names()."""
        @tool(description="A hidden tool", hidden=True)
        def secret_tool() -> str:
            return "secret"

        hidden = get_hidden_tool_names()
        assert "secret_tool" in hidden

    def test_combined_hidden_from_tools_groups_workflows(self):
        """Hidden tools from all sources should be collected together."""
        @tool(description="Hidden tool", hidden=True)
        def hidden_tool() -> str:
            return "x"

        @tool_group(name="hidden_group", description="Hidden group", hidden=True)
        class HiddenGroup:
            @action("act", description="Act")
            def act(self) -> str:
                return "y"

        @workflow(name="wf", description="Workflow")
        class WF:
            @step("start", description="Start", initial=True, next=["end"])
            def start(self) -> StepResult:
                return StepResult()

            @step("end", description="End", terminal=True)
            def end(self) -> StepResult:
                return StepResult()

        hidden = get_hidden_tool_names()
        assert "hidden_tool" in hidden
        assert "hidden_group" in hidden
        assert "wf.end" in hidden
        assert "wf.cancel" in hidden


# ---------------------------------------------------------------------------
# 3. Resource template URI matching
# ---------------------------------------------------------------------------


class TestURIMatchesTemplate:
    def test_basic_match(self):
        assert _uri_matches_template("notes://{id}", "notes://123")

    def test_basic_no_match_wrong_scheme(self):
        assert not _uri_matches_template("notes://{id}", "other://123")

    def test_multiple_parameters(self):
        assert _uri_matches_template("users://{org}/{id}", "users://acme/42")

    def test_no_match_missing_segment(self):
        assert not _uri_matches_template("users://{org}/{id}", "users://acme")

    def test_exact_static_uri(self):
        assert _uri_matches_template("config://global", "config://global")

    def test_exact_static_no_match(self):
        assert not _uri_matches_template("config://global", "config://local")

    def test_parameter_does_not_match_slash(self):
        """A single {param} should not match across path separators."""
        assert not _uri_matches_template("files://{name}", "files://dir/file")

    def test_empty_parameter_no_match(self):
        """An empty segment should not match a template parameter."""
        assert not _uri_matches_template("notes://{id}", "notes://")

    def test_complex_template(self):
        assert _uri_matches_template(
            "repo://{owner}/{repo}/issues/{number}",
            "repo://octocat/hello-world/issues/42",
        )


# ---------------------------------------------------------------------------
# 4. Hot reload discovery clears all registries
# ---------------------------------------------------------------------------


class TestHotReloadClearsAllRegistries:
    @staticmethod
    def _write_dummy_handler(tmpdir):
        """Write a minimal handler file so discover_handlers loads something."""
        handler_path = os.path.join(tmpdir, "dummy.py")
        with open(handler_path, "w") as f:
            f.write("x = 1\n")
        return handler_path

    def test_hot_reload_clears_tools(self):
        """Registering tools then triggering hot reload should clear the tool registry."""
        @tool(description="Temp tool")
        def temp_tool() -> str:
            return "temp"

        assert any(t.name == "temp_tool" for t in get_registered_tools())

        with tempfile.TemporaryDirectory() as tmpdir:
            self._write_dummy_handler(tmpdir)
            configure(handlers_dir=tmpdir, hot_reload=True)
            # First discover loads modules (populates _loaded_modules)
            discover_handlers()
            # Second discover triggers hot reload path (clears registries)
            discover_handlers()

        # After hot reload, the manually registered tool should be gone
        assert not any(t.name == "temp_tool" for t in get_registered_tools())

    def test_hot_reload_clears_resources(self):
        """Resources should be cleared on hot reload."""
        @resource(uri="test://resource", description="Test resource")
        def test_res(uri: str):
            return "data"

        assert len(get_registered_resources()) == 1

        with tempfile.TemporaryDirectory() as tmpdir:
            self._write_dummy_handler(tmpdir)
            configure(handlers_dir=tmpdir, hot_reload=True)
            discover_handlers()
            discover_handlers()

        assert len(get_registered_resources()) == 0

    def test_hot_reload_clears_resource_templates(self):
        """Resource templates should be cleared on hot reload."""
        @resource_template(uri_template="test://{id}", description="Test template")
        def test_tmpl(uri: str):
            return "data"

        assert len(get_registered_resource_templates()) == 1

        with tempfile.TemporaryDirectory() as tmpdir:
            self._write_dummy_handler(tmpdir)
            configure(handlers_dir=tmpdir, hot_reload=True)
            discover_handlers()
            discover_handlers()

        assert len(get_registered_resource_templates()) == 0

    def test_hot_reload_clears_prompts(self):
        """Prompts should be cleared on hot reload."""
        @prompt(description="Test prompt")
        def test_prompt_fn() -> list:
            return [PromptMessage(role="user", content="hello")]

        assert len(get_registered_prompts()) == 1

        with tempfile.TemporaryDirectory() as tmpdir:
            self._write_dummy_handler(tmpdir)
            configure(handlers_dir=tmpdir, hot_reload=True)
            discover_handlers()
            discover_handlers()

        assert len(get_registered_prompts()) == 0

    def test_hot_reload_clears_groups(self):
        """Groups should be cleared on hot reload."""
        @tool_group(name="temp_group", description="Temp group")
        class TempGroup:
            @action("act", description="Action")
            def act(self) -> str:
                return "x"

        from protomcp.group import get_registered_groups
        assert len(get_registered_groups()) == 1

        with tempfile.TemporaryDirectory() as tmpdir:
            self._write_dummy_handler(tmpdir)
            configure(handlers_dir=tmpdir, hot_reload=True)
            discover_handlers()
            discover_handlers()

        assert len(get_registered_groups()) == 0

    def test_hot_reload_clears_workflows(self):
        """Workflows should be cleared on hot reload."""
        @workflow(name="temp_wf", description="Temp workflow")
        class TempWF:
            @step("start", description="Start", initial=True, terminal=True)
            def start(self) -> StepResult:
                return StepResult()

        assert len(get_registered_workflows()) == 1

        with tempfile.TemporaryDirectory() as tmpdir:
            self._write_dummy_handler(tmpdir)
            configure(handlers_dir=tmpdir, hot_reload=True)
            discover_handlers()
            discover_handlers()

        assert len(get_registered_workflows()) == 0
