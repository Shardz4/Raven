import ast
from dataclasses import dataclass
from typing import Iterable, Optional, Set


@dataclass(frozen=True)
class ValidationResult:
    ok: bool
    reason: str = ""


_DEFAULT_FORBIDDEN_MODULES: Set[str] = {
    # OS / process / network
    "os",
    "subprocess",
    "socket",
    "ssl",
    "asyncio",
    "multiprocessing",
    "threading",
    "signal",
    # Filesystem / execution
    "pathlib",
    "shutil",
    "glob",
    "tempfile",
    "importlib",
    "pickle",
    "marshal",
    # HTTP / external IO
    "requests",
    "urllib",
    "http",
    "ftplib",
    # Containers / host integration
    "docker",
}

_DEFAULT_FORBIDDEN_CALLS: Set[str] = {
    "eval",
    "exec",
    "compile",
    "__import__",
    "open",
}


def validate_patch_code(
    code: str,
    *,
    require_function: str = "fix_issue",
    max_chars: int = 20_000,
    forbidden_modules: Optional[Iterable[str]] = None,
    forbidden_calls: Optional[Iterable[str]] = None,
) -> ValidationResult:
    """
    Lightweight safety validation before running untrusted patch code.

    Notes:
    - This is NOT a complete sandbox. It is a guardrail to reduce obvious risk.
    - Docker sandboxing is still required.
    """
    if not code or not code.strip():
        return ValidationResult(False, "Empty patch")

    if len(code) > max_chars:
        return ValidationResult(False, f"Patch too large (> {max_chars} chars)")

    try:
        tree = ast.parse(code)
    except SyntaxError as e:
        return ValidationResult(False, f"Syntax error: {e}")

    forbidden_modules_set = set(forbidden_modules) if forbidden_modules is not None else _DEFAULT_FORBIDDEN_MODULES
    forbidden_calls_set = set(forbidden_calls) if forbidden_calls is not None else _DEFAULT_FORBIDDEN_CALLS

    has_required = False

    for node in ast.walk(tree):
        if isinstance(node, ast.FunctionDef) and node.name == require_function:
            has_required = True

        if isinstance(node, ast.Import):
            for alias in node.names:
                mod = (alias.name or "").split(".")[0]
                if mod in forbidden_modules_set:
                    return ValidationResult(False, f"Forbidden import: {mod}")

        if isinstance(node, ast.ImportFrom):
            mod = (node.module or "").split(".")[0]
            if mod in forbidden_modules_set:
                return ValidationResult(False, f"Forbidden import: {mod}")

        if isinstance(node, ast.Call):
            fn = node.func
            if isinstance(fn, ast.Name) and fn.id in forbidden_calls_set:
                return ValidationResult(False, f"Forbidden call: {fn.id}()")

    if not has_required:
        return ValidationResult(False, f"Missing required function: {require_function}()")

    return ValidationResult(True, "OK")

