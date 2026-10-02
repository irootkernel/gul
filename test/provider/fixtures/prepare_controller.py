"""Provision an isolated published-provider test with a native fake app-server.

Only Codex version/schema inspection uses the installed exact schema generator.
Protocol calls use the matching upstream fake; no live model, Operator, child
credential, or neighboring checkout is used.
"""

import json
import os
import pathlib
import subprocess
import sys

binary, root, source = map(pathlib.Path, sys.argv[1:4])
root = root.resolve()
for name in ("home", "workspace", "codex-home", "bin", "tmp"):
    directory = root / name
    directory.mkdir(mode=0o700, exist_ok=True)
    directory.chmod(0o700)
os.environ["HOME"] = str(root / "home")
os.environ["TMPDIR"] = str(root / "tmp")
sys.path.insert(0, str(source / "tests/e2e"))
import native_codex

def cli(*arguments):
    result = subprocess.run(
        [str(binary), *arguments], capture_output=True, timeout=90
    )
    document = json.loads(result.stdout)
    if result.returncode or not document.get("ok"):
        raise RuntimeError(document.get("error", {}).get("code", "FIXTURE_SETUP_FAILED"))
    return document["data"]

cli("init", str(root / "workspace"), "--non-git")
native_codex.create_native_codex(
    root / "bin/codex",
    scenario=native_codex.scenario_path("run_start_model_list.json"),
    codex_home=root / "codex-home",
    schema_source=native_codex.installed_codex(),
)

# Every fixture app-server stops itself after the private fixture disappears.
# The native image remains resident and retains the configured spawn identity.
sentinel = root / "alive"
sentinel.write_text("fixture active\n")
driver = root / "bin/codex-driver.py"
text = driver.read_text()
watchdog = f'''
    import os, threading, time
    def stop_with_fixture():
        while pathlib.Path({str(sentinel)!r}).exists():
            time.sleep(0.1)
        os._exit(0)
    threading.Thread(target=stop_with_fixture, daemon=True).start()
'''
if text.count("    fake.bind()\n") != 1:
    raise RuntimeError("FIXTURE_WATCHDOG_BINDING_CHANGED")
text = text.replace("    fake.bind()\n", watchdog + "    fake.bind()\n")
driver.write_text(text)

cli("profile", "add", "default", "--codex-home", str(root / "codex-home"),
    "--native-subagents", "enabled", "--env", "PATH=/usr/bin:/bin:/usr/sbin:/sbin",
    "--env", "LANG=en_US.UTF-8", "--env", "LC_ALL=en_US.UTF-8", "--",
    str(root / "bin/codex"))
cli("profile", "server", "start", "default")

# Role source is user-authored project configuration, not a provider ledger.
roles = root / "workspace/.dolgorae/roles"
roles.mkdir(mode=0o700)
(roles / "observer.json").write_text(json.dumps({
    "schema_version": 1, "name": "observer", "display_name": "Observer",
    "description": "Returns bounded test observations.",
    "instructions": "Return a bounded observation without changing the workspace."
}))
policy = root / "policy.json"
policy.write_text(json.dumps({
    "schema_version": 2, "policy_name": "carrier-test", "revision": 1,
    "approval_policy": "fully_delegated", "max_active_specialists": 1,
    "roles": [{
        "role_ref": "observer", "role_source": {"scope": "project", "name": "observer"},
        "agent_configuration": {
            "schema_version": 2, "selected_profile": "default", "model": "gpt-5.6",
            "default_effort": "medium", "purpose": "review", "purpose_label": None,
            "required_capabilities": [], "execution_lane": "dedicated",
            "required_assurance": "best_effort_personal_alpha",
            "native_subagent_policy": "enabled"
        },
        "max_active_instances": 1, "reuse_policy": "never", "allowed_access": ["read_only"],
        "activation_policy": "keep_resident", "primary_may_request": False,
        "collaboration_source": False, "collaboration_target": False,
        "auto_approve_when_fully_delegated": True
    }]
}))
cli("specialist", "policy", "add", "carrier-test", "--workspace",
    str(root / "workspace"), "--file", str(policy))
print("isolated fixture ready")
