from protomcp.group import tool_group, action, get_registered_groups, clear_group_registry, _dispatch_group_action
from protomcp.result import ToolResult

def test_required_validation_missing():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("add", description="Add data", requires=["data_path"])
        def add(self, data_path: str | None = None) -> str:
            return f"added {data_path}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="add", data_path=None)
    assert isinstance(result, ToolResult)
    assert result.is_error
    assert "data_path" in result.message

def test_required_validation_passes():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("add", description="Add data", requires=["data_path"])
        def add(self, data_path: str | None = None) -> str:
            return f"added {data_path}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="add", data_path="/tmp/data.csv")
    assert result == "added /tmp/data.csv"

def test_enum_validation_valid():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("fill", description="Fill nulls",
                enum_fields={"strategy": ["median", "mean", "mode", "zero", "value"]})
        def fill(self, column: str, strategy: str = "median") -> str:
            return f"filled {column} with {strategy}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="fill", column="age", strategy="mean")
    assert result == "filled age with mean"

def test_enum_validation_fuzzy():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("fill", description="Fill nulls",
                enum_fields={"strategy": ["median", "mean", "mode", "zero", "value"]})
        def fill(self, column: str, strategy: str = "median") -> str:
            return f"filled {column} with {strategy}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="fill", column="age", strategy="meann")
    assert isinstance(result, ToolResult)
    assert result.is_error
    assert "Did you mean" in (result.suggestion or "")

def test_enum_validation_no_match():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("fill", description="Fill nulls",
                enum_fields={"strategy": ["median", "mean", "mode"]})
        def fill(self, column: str, strategy: str = "median") -> str:
            return f"filled {column} with {strategy}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="fill", column="age", strategy="xxxxx")
    assert isinstance(result, ToolResult)
    assert result.is_error

def test_cross_rules_violation():
    clear_group_registry()
    @tool_group("models", description="Model tools")
    class ModelTools:
        @action("create", description="Create model",
                cross_rules=[
                    (lambda args: args.get("model_type") == "regressor" and not args.get("cdf_scale"),
                     "Regressor models should set cdf_scale"),
                ])
        def create(self, name: str, model_type: str = "regressor", cdf_scale: float | None = None) -> str:
            return f"created {name}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="create", name="xgb", model_type="regressor")
    assert isinstance(result, ToolResult)
    assert result.is_error
    assert "cdf_scale" in result.message

def test_cross_rules_pass():
    clear_group_registry()
    @tool_group("models", description="Model tools")
    class ModelTools:
        @action("create", description="Create model",
                cross_rules=[
                    (lambda args: args.get("model_type") == "regressor" and not args.get("cdf_scale"),
                     "Regressor models should set cdf_scale"),
                ])
        def create(self, name: str, model_type: str = "regressor", cdf_scale: float | None = None) -> str:
            return f"created {name}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="create", name="xgb", model_type="regressor", cdf_scale=1.0)
    assert result == "created xgb"

def test_hints_appended():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("add", description="Add data",
                hints={"join_on": {"condition": lambda args: not args.get("join_on"),
                                    "message": "No join_on specified. Data will be appended as new rows."}})
        def add(self, data_path: str, join_on: list[str] | None = None) -> str:
            return f"added {data_path}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="add", data_path="/tmp/data.csv")
    assert isinstance(result, ToolResult)
    assert not result.is_error
    assert "new rows" in result.result

def test_hints_not_appended_when_condition_false():
    clear_group_registry()
    @tool_group("data", description="Data tools")
    class DataTools:
        @action("add", description="Add data",
                hints={"join_on": {"condition": lambda args: not args.get("join_on"),
                                    "message": "No join_on specified."}})
        def add(self, data_path: str, join_on: list[str] | None = None) -> str:
            return f"added {data_path}"
    groups = get_registered_groups()
    result = _dispatch_group_action(groups[0], action="add", data_path="/tmp/data.csv", join_on=["id"])
    assert result == "added /tmp/data.csv"
