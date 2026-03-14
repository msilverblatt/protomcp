import os
import tempfile
from protomcp.discovery import discover_handlers, configure, get_config, reset_config

def test_configure():
    reset_config()
    configure(handlers_dir="handlers/", hot_reload=True)
    cfg = get_config()
    assert cfg["handlers_dir"] == "handlers/"
    assert cfg["hot_reload"] is True

def test_discover_from_directory():
    reset_config()
    with tempfile.TemporaryDirectory() as tmpdir:
        with open(os.path.join(tmpdir, "calc.py"), "w") as f:
            f.write('''
from protomcp.group import tool_group, action

@tool_group("calc", description="Calculator")
class CalcTools:
    @action("add", description="Add two numbers")
    def add(self, a: int, b: int) -> str:
        return str(a + b)
''')
        from protomcp.group import clear_group_registry, get_registered_groups
        clear_group_registry()
        configure(handlers_dir=tmpdir)
        discover_handlers()
        groups = get_registered_groups()
        assert len(groups) == 1
        assert groups[0].name == "calc"

def test_skip_underscore_files():
    reset_config()
    with tempfile.TemporaryDirectory() as tmpdir:
        with open(os.path.join(tmpdir, "_private.py"), "w") as f:
            f.write("x = 1\n")
        with open(os.path.join(tmpdir, "__init__.py"), "w") as f:
            f.write("")
        with open(os.path.join(tmpdir, "valid.py"), "w") as f:
            f.write('''
from protomcp.group import tool_group, action

@tool_group("valid", description="Valid")
class ValidTools:
    @action("ping", description="Ping")
    def ping(self) -> str:
        return "pong"
''')
        from protomcp.group import clear_group_registry, get_registered_groups
        clear_group_registry()
        configure(handlers_dir=tmpdir)
        discover_handlers()
        groups = get_registered_groups()
        assert len(groups) == 1
        assert groups[0].name == "valid"

def test_rediscover():
    reset_config()
    with tempfile.TemporaryDirectory() as tmpdir:
        with open(os.path.join(tmpdir, "calc.py"), "w") as f:
            f.write('''
from protomcp.group import tool_group, action

@tool_group("calc", description="Calculator")
class CalcTools:
    @action("add", description="Add")
    def add(self, a: int, b: int) -> str:
        return str(a + b)
''')
        from protomcp.group import clear_group_registry, get_registered_groups
        clear_group_registry()
        configure(handlers_dir=tmpdir, hot_reload=True)
        discover_handlers()
        assert len(get_registered_groups()) == 1

        with open(os.path.join(tmpdir, "calc.py"), "w") as f:
            f.write('''
from protomcp.group import tool_group, action

@tool_group("calc_v2", description="Calculator V2")
class CalcTools:
    @action("add", description="Add")
    def add(self, a: int, b: int) -> str:
        return str(a + b)
    @action("sub", description="Subtract")
    def sub(self, a: int, b: int) -> str:
        return str(a - b)
''')
        discover_handlers()
        groups = get_registered_groups()
        assert len(groups) == 1
        assert groups[0].name == "calc_v2"
        assert len(groups[0].actions) == 2
