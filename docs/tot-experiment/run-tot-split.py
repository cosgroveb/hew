#!/usr/bin/env python3
"""Run ToT-Split evaluation: orchestrator creates branches, GPT-OSS implements each."""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import time

FIXTURES_DIR = os.path.expanduser("~/code/hew/docs/tot-experiment/fixtures")
RESULTS_BASE = os.path.expanduser("~/code/hew/docs/tot-experiment/results")

APPROACHES = {
    "task1-ratelimiter": [
        "Implement a token bucket rate limiter. Maintain a bucket of tokens per key that refills at Rate tokens per Window. Allow() consumes a token if available, false otherwise. Burst sets maximum bucket size. Use the Clock interface for time.",
        "Implement a sliding window counter rate limiter. Track request counts in sub-windows per key. Allow() checks total count in the last Window duration. Burst allows initial burst. Use the Clock interface.",
        "Implement a fixed window counter rate limiter. Divide time into Window-sized intervals. Count requests per key per window. Allow() returns true if under Rate. Burst allows exceeding at window start. Use the Clock interface.",
    ],
    "task2-refactor": [
        "Refactor by extracting three packages by concern: create a notification/ package for email sending, a validation/ package for input validation, and keep service/ for core business logic. Define interfaces in service/ that the new packages implement. Wire in the constructor.",
        "Refactor by splitting the god object into multiple files within the service/ package: users.go for user management, orders.go for order processing, notifications.go for notification sending, validation.go for validation logic. Extract shared state into a store struct passed to each.",
        "Refactor by defining interfaces first: Notifier, Validator, and Store interfaces in service/interfaces.go. Create concrete implementations in separate files within service/. The UserOrderService constructor wires them together via dependency injection.",
    ],
    "task3-multibug": [
        "Fix bugs in dependency order. First fix the TTL calculation in Set() (change Add(-ttl) to Add(ttl)). Then fix the race condition in evictExpired() by holding RLock during iteration. Finally fix the nil pointer in evictOldest() by checking the entry still exists before accessing it.",
        "Fix all three bugs with a unified locking strategy. Hold the write lock for the entire eviction cycle. Fix the TTL calculation. In evictOldest, collect candidates under lock and verify they still exist before deletion.",
        "Rewrite Set(), evictExpired(), and evictOldest() from scratch. Keep the same method signatures but use correct locking (RLock for reads, Lock for writes), correct TTL math (Add not Sub), and nil-safe patterns (check map membership before access) throughout.",
    ],
}


def run_branch(fixture_dir, work_dir, approach, trajectory_path, event_log_path, stderr_path):
    """Run a single branch: copy fixture, write approach, run hu."""
    # Write approach instructions
    approach_file = os.path.join(work_dir, "APPROACH.md")
    with open(approach_file, "w") as f:
        f.write(f"# Implementation Approach\n\n{approach}\n\n")
        f.write("## Instructions\n\n")
        f.write("Read README.md for the full problem statement.\n")
        f.write("Implement the solution using the approach described above.\n")
        f.write("Run `make test` to verify your work.\n")
        f.write("Do NOT modify files marked DO NOT MODIFY.\n")

    prompt = "Read APPROACH.md and README.md in the current directory. Implement the solution using the specified approach. Run make test to verify. Only modify files you are allowed to change."

    env = os.environ.copy()
    env["HEW_BASE_URL"] = "http://localhost:8000/v1"
    env["HEW_API_KEY"] = "not-needed"
    env["HEW_MODEL"] = "gpt-oss-120b"

    start = time.time()
    with open(stderr_path, "w") as stderr_f:
        result = subprocess.run(
            ["hu", "-p", prompt,
             "--trajectory", trajectory_path,
             "--event-log", event_log_path,
             "--max-steps", "30"],
            cwd=work_dir,
            env=env,
            stderr=stderr_f,
            timeout=600,
        )
    elapsed = time.time() - start
    return elapsed, result.returncode


def score_branch(fixture_dir, work_dir, trajectory_path):
    """Score a branch by running tests and collecting metrics."""
    env = os.environ.copy()
    # Bypass RTK
    env.pop("RTK_REWRITE", None)

    # Run tests
    test_result = subprocess.run(
        ["go", "test", "./...", "-v", "-count=1", "-race"],
        cwd=work_dir, capture_output=True, text=True, timeout=120, env=env
    )
    test_output = test_result.stdout + test_result.stderr

    tests_passed = test_output.count("--- PASS:")
    tests_failed = test_output.count("--- FAIL:")
    tests_total = tests_passed + tests_failed
    race_clean = "DATA RACE" not in test_output

    # Vet
    vet_result = subprocess.run(
        ["go", "vet", "./..."],
        cwd=work_dir, capture_output=True, text=True, timeout=30, env=env
    )
    vet_clean = vet_result.returncode == 0

    # File metrics
    max_lines = 0
    packages = set()
    for root, dirs, files in os.walk(work_dir):
        dirs[:] = [d for d in dirs if d != "vendor" and d != ".git"]
        for f in files:
            if f.endswith(".go"):
                path = os.path.join(root, f)
                with open(path) as fh:
                    lines = len(fh.readlines())
                max_lines = max(max_lines, lines)
                packages.add(root)

    # Steps from trajectory
    steps = 0
    if os.path.exists(trajectory_path):
        with open(trajectory_path) as f:
            msgs = json.load(f)
            steps = len(msgs) // 2

    return {
        "tests_total": tests_total,
        "tests_passed": tests_passed,
        "tests_failed": tests_failed,
        "race_clean": race_clean,
        "vet_clean": vet_clean,
        "max_file_lines": max_lines,
        "package_count": len(packages),
        "steps": steps,
    }


def run_task_split(task_name, run_id):
    """Run ToT-Split for one task: 3 branches, pick best."""
    fixture_dir = os.path.join(FIXTURES_DIR, task_name)
    results_dir = os.path.join(RESULTS_BASE, task_name, "tot-split", f"run-{run_id}")
    os.makedirs(results_dir, exist_ok=True)

    approaches = APPROACHES[task_name]
    best_score = -1
    best_idx = -1

    for i, approach in enumerate(approaches):
        print(f"  Branch {i}: {approach[:60]}...")
        work_dir = tempfile.mkdtemp(prefix=f"tot-{task_name}-b{i}-")
        shutil.copytree(fixture_dir, work_dir, dirs_exist_ok=True)

        traj = os.path.join(results_dir, f"branch-{i}-trajectory.json")
        events = os.path.join(results_dir, f"branch-{i}-events.jsonl")
        stderr = os.path.join(results_dir, f"branch-{i}-stderr.txt")

        try:
            elapsed, rc = run_branch(fixture_dir, work_dir, approach, traj, events, stderr)
            score = score_branch(fixture_dir, work_dir, traj)
            score["time"] = round(elapsed, 1)
            score["exit_code"] = rc

            with open(os.path.join(results_dir, f"branch-{i}-score.json"), "w") as f:
                json.dump(score, f, indent=2)

            print(f"    passed={score['tests_passed']}/{score['tests_total']} "
                  f"race={score['race_clean']} time={score['time']}s steps={score['steps']}")

            if score["tests_passed"] > best_score:
                best_score = score["tests_passed"]
                best_idx = i

        except subprocess.TimeoutExpired:
            print(f"    TIMEOUT")
            with open(os.path.join(results_dir, f"branch-{i}-score.json"), "w") as f:
                json.dump({"error": "timeout"}, f)
        except Exception as e:
            print(f"    ERROR: {e}")
            with open(os.path.join(results_dir, f"branch-{i}-score.json"), "w") as f:
                json.dump({"error": str(e)}, f)
        finally:
            shutil.rmtree(work_dir, ignore_errors=True)

    print(f"  Winner: branch {best_idx} ({best_score} tests passed)")
    with open(os.path.join(results_dir, "winner.txt"), "w") as f:
        f.write(f"{best_idx}\n")
    if best_idx >= 0:
        src = os.path.join(results_dir, f"branch-{best_idx}-score.json")
        dst = os.path.join(results_dir, "score.json")
        shutil.copy(src, dst)


def main():
    tasks = list(APPROACHES.keys())
    runs = 5

    if len(sys.argv) > 1:
        tasks = [sys.argv[1]]
    if len(sys.argv) > 2:
        runs = int(sys.argv[2])

    for task in tasks:
        print(f"\n=== {task} tot-split ===")
        for run_id in range(1, runs + 1):
            print(f"\n--- Run {run_id}/{runs} ---")
            run_task_split(task, run_id)


if __name__ == "__main__":
    main()
