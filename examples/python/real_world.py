# examples/python/real_world.py
# A file search tool demonstrating progress, logging, and cancellation.
# Run: pmcp dev examples/python/real_world.py

import os
import fnmatch
from protomcp import tool, ToolResult, ToolContext, log

@tool("Search files in a directory by glob pattern", read_only=True)
def search_files(ctx: ToolContext, directory: str, pattern: str, max_results: int = 50) -> ToolResult:
    log.info(f"Searching {directory} for '{pattern}'")

    if not os.path.isdir(directory):
        return ToolResult(
            result=f"Directory not found: {directory}",
            is_error=True,
            error_code="INVALID_PATH",
            message="The specified directory does not exist",
            suggestion="Check the path and try again",
        )

    matches = []
    all_files = []
    for root, dirs, files in os.walk(directory):
        for f in files:
            all_files.append(os.path.join(root, f))

    total = len(all_files)
    log.debug(f"Found {total} files to scan")

    for i, filepath in enumerate(all_files):
        if ctx.is_cancelled():
            log.warning("Search cancelled by client")
            return ToolResult(
                result=f"Cancelled after scanning {i}/{total} files. Found {len(matches)} matches so far.",
                is_error=True,
                error_code="CANCELLED",
                retryable=True,
            )

        if i % 100 == 0:
            ctx.report_progress(i, total, f"Scanning... {i}/{total}")

        if fnmatch.fnmatch(os.path.basename(filepath), pattern):
            matches.append(filepath)
            if len(matches) >= max_results:
                log.info(f"Hit max_results={max_results}, stopping early")
                break

    ctx.report_progress(total, total, "Complete")
    log.info(f"Search complete: {len(matches)} matches")
    return ToolResult(result="\n".join(matches) if matches else "No files found")


if __name__ == "__main__":
    from protomcp.runner import run
    run()
