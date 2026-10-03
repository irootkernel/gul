# Embedded into the resident upstream native fake, never a production adapter.
import json
import os
import uuid

gul_root = pathlib.Path(FIXTURE_ROOT)
gul_original_dispatch = server_module.FakeAppServer.dispatch
gul_original_emit = server_module.FakeAppServer.emit

def gul_emit(server, connection, emission):
    if emission.get("method") == "turn/completed":
        params = server.scenario.substitute(emission.get("params", {}))
        turn = params.get("turn", {})
        with (gul_root / "native-completions.jsonl").open("a") as sink:
            sink.write(json.dumps({"pid": os.getpid(), "thread": params.get("threadId"),
                                  "turn": turn.get("id"), "status": turn.get("status")}) + "\n")
    return gul_original_emit(server, connection, emission)

def gul_dispatch(server, connection, message):
    method = message.get("method")
    params = message.get("params", {})
    if method in ("thread/start", "thread/fork"):
        thread_id = "gul-thread-" + uuid.uuid4().hex
        server.scenario.steps = [s for s in server.scenario.steps if s["method"] != method] + [
            {"method": method, "respond": {"result": {"thread": {"id": thread_id}}}}]
    if isinstance(params.get("threadId"), str):
        server.scenario.bind("thread_id", params["threadId"])
    if method == "turn/start":
        inputs = params.get("input", [])
        text = "\n".join(item.get("text", "") for item in inputs if item.get("type") == "text")
        turn_id = "gul-turn-" + uuid.uuid4().hex
        policy = params.get("sandboxPolicy", {})
        if policy.get("type") == "workspaceWrite":
            roots = policy.get("writableRoots", [])
            if roots != [str(gul_root / "workspace")]:
                raise RuntimeError("fake WRITE received unexpected authority")
            (gul_root / "workspace/provider-write.txt").write_text("upstream native fake WRITE effect\n")
        with (gul_root / "native-inputs.jsonl").open("a") as sink:
            sink.write(json.dumps({"pid": os.getpid(), "thread": params.get("threadId"),
                                  "turn": turn_id, "input": inputs, "effort": params.get("effort"),
                                  "sandbox": policy.get("type")}) + "\n")
        answer = "Gul native fake completed the accepted prompt."
        if "Gul qualification Specialist result" in text:
            answer = "Gul Specialist result\n" + ("verified large result line\n" * 50000)
        completed = {"kind": "notification", "method": "turn/completed", "params": {
            "threadId": "${thread_id}", "turn": {"id": turn_id, "status": "completed", "items": [
                {"type": "agentMessage", "phase": "final_answer", "status": "completed", "threadId":params["threadId"], "turnId":turn_id,"text": answer}]}}}
        if not hasattr(server, "gul_turns"):
            server.gul_turns = {}
        saved_turn = completed["params"]["turn"]
        if "hold-busy" in text:
            saved_turn = {"id": turn_id, "status": "inProgress", "items": []}
        server.gul_turns.setdefault(params["threadId"], []).append(saved_turn)
        emissions = [completed]
        if "wait-for-approval" in text:
            completed["await_reply"] = True
            emissions = [{"kind": "request", "id": 7001, "method": "item/commandExecution/requestApproval", "params": {
                "threadId": "${thread_id}", "turnId": turn_id, "command": ["git", "status"]}}, completed]
        if "hold-busy" in text:
            emissions = []
        if "publish-specialist" in text:
            def tool(identifier, operation, arguments, wait=False):
                return {"kind": "request", "id": identifier, "method": "item/tool/call", "await_reply": wait,
                        "params": {"threadId": "${thread_id}", "turnId": turn_id, "callId": "gul-" + str(identifier),
                                   "tool": "dolgorae_orchestration", "arguments": {"operation": operation, **arguments}}}
            emissions = [
                tool(8001, "request_specialist", {"role_ref": "observer", "objective": "Return the Gul qualification Specialist result",
                    "expected_output": ["One verified result"], "requested_access": "read_only", "deadline_seconds": 60}),
                tool(8002, "list_specialists", {}, True),
                tool(8003, "assign_specialist_task", {"target": {"run_id": "${specialist_run_id}"},
                    "objective": "Return the Gul qualification Specialist result", "context_refs": [],
                    "expected_output": ["One verified result"], "execution_intent": "read_only", "blocking": False,
                    "deadline_seconds": 60}, True),
                tool(8004, "await_specialist_tasks", {"task_ids": ["${specialist_task_id}"], "return_when": "all", "transport_wait_seconds": 10}, True),
                tool(8005, "collect_specialist_results", {"after_sequence": 0, "limit": 10}, True),
                {**completed, "await_reply": True}]
        server.scenario.steps = [s for s in server.scenario.steps if s["method"] != "turn/start"] + [
            {"method": "turn/start", "respond": {"result": {"turn": {"id": turn_id}}}, "emit": emissions}]
    if method == "turn/interrupt":
        for turn in getattr(server, "gul_turns", {}).get(params["threadId"], []):
            if turn["id"] == params["turnId"]:
                turn["status"], turn["items"] = "interrupted", []
        server.scenario.steps = [s for s in server.scenario.steps if s["method"] != "turn/interrupt"] + [
            {"method": "turn/interrupt", "respond": {"result": {}}, "emit": [{"kind": "notification", "method": "turn/completed", "params": {
                "threadId": "${thread_id}", "turn": {"id": params["turnId"], "status": "interrupted", "items": []}}}]}]
    if method == "thread/read":
        thread = params["threadId"]
        turns = getattr(server,"gul_turns",{}).get(thread,[])
        response = {"result": {"thread": {"id": thread, "turns": turns}}} if thread.startswith("gul-thread-") else {
            "error": {"code": -32600, "message": "thread not found"}}
        server.scenario.steps = [s for s in server.scenario.steps if s["method"] != "thread/read"] + [
            {"method":"thread/read","respond":response}]
    return gul_original_dispatch(server, connection, message)

server_module.FakeAppServer.dispatch = gul_dispatch
server_module.FakeAppServer.emit = gul_emit
