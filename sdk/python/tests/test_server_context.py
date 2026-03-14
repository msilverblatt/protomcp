from protomcp.server_context import server_context, get_registered_contexts, clear_context_registry, resolve_contexts

def test_register_context():
    clear_context_registry()
    @server_context("project_dir")
    def resolve_project_dir(args: dict):
        explicit = args.pop("project_dir", None)
        return explicit or "/default/path"
    contexts = get_registered_contexts()
    assert len(contexts) == 1
    assert contexts[0].param_name == "project_dir"

def test_resolve_pops_from_args():
    clear_context_registry()
    @server_context("project_dir")
    def resolve_project_dir(args: dict):
        explicit = args.pop("project_dir", None)
        return explicit or "/default/path"
    args = {"project_dir": "/custom", "other": "value"}
    resolved = resolve_contexts(args)
    assert resolved == {"project_dir": "/custom"}
    assert args == {"other": "value"}

def test_resolve_default():
    clear_context_registry()
    @server_context("project_dir")
    def resolve_project_dir(args: dict):
        explicit = args.pop("project_dir", None)
        return explicit or "/default/path"
    args = {"other": "value"}
    resolved = resolve_contexts(args)
    assert resolved == {"project_dir": "/default/path"}

def test_expose_true():
    clear_context_registry()
    @server_context("project_dir", expose=True)
    def resolve_project_dir(args: dict):
        return args.pop("project_dir", "/default")
    assert get_registered_contexts()[0].expose is True

def test_expose_false():
    clear_context_registry()
    @server_context("db_conn", expose=False)
    def resolve_db(args: dict):
        return "connection_object"
    assert get_registered_contexts()[0].expose is False

def test_multiple_contexts():
    clear_context_registry()
    @server_context("project_dir")
    def resolve_project_dir(args: dict):
        return args.pop("project_dir", "/default")
    @server_context("db_conn", expose=False)
    def resolve_db(args: dict):
        return "db_connection"
    args = {"project_dir": "/custom", "query": "SELECT 1"}
    resolved = resolve_contexts(args)
    assert resolved == {"project_dir": "/custom", "db_conn": "db_connection"}
    assert args == {"query": "SELECT 1"}
