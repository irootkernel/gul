"""Extend the exact upstream native fake for assembled Gul qualification.

Only private fixture copies are changed. Public Dolgorae runs, writer events,
approvals, history, broker tasks and artifact publication remain producer-owned.
The fake's workspace effect is an explicit protocol simulation, not live Codex.
"""
import json
import pathlib
import subprocess
import sys

binary, root, source = map(pathlib.Path, sys.argv[1:4])
root = root.resolve()
root.mkdir(mode=0o700, parents=True, exist_ok=True)
scenario = json.loads((source / "tools/fake_app_server/scenarios/run_start_model_list.json").read_text())
scenario["name"] = "gul_session_qualification"
# The shared Profile serves concurrent Primary and Specialist connections.
scenario["concurrent_connections"] = True
scenario["steps"] = [step for step in scenario["steps"] if step["method"] != "model/list"]
scenario["steps"].append({"method": "model/list", "respond": {"result": {
    "data": [{"model": "gpt-5.6", "isDefault": True, "supportedReasoningEfforts": [
        {"reasoningEffort": effort} for effort in ("low", "medium", "high")]}], "nextCursor": None}}})
scenario_path = root / "session-scenario.json"
scenario_path.write_text(json.dumps(scenario))
subprocess.run([sys.executable, str(pathlib.Path(__file__).with_name("prepare_controller.py")),
                str(binary), str(root), str(source), str(scenario_path)], check=True, timeout=100)
