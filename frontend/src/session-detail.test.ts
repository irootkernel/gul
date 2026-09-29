import {expect, test} from "bun:test";
import {Code, ConnectError} from "@connectrpc/connect";
import {ActionClass, DomainErrorSchema, ErrorCode} from "../../api/generated/ts/gul/v1/gul_pb";
import {externalActionRequired, operatorError} from "./domain-errors";

test("typed blockers give distinct safe external actions without leaking provider diagnostics", () => {
  const cases: [ErrorCode, string][] = [
    [ErrorCode.WORKSPACE_NOT_PROVISIONED, "Provision it in Dolgorae"],
    [ErrorCode.PROFILE_MISSING, "Configure a supported provider profile"],
    [ErrorCode.CONTROLLER_MISMATCH, "Verify or reset the Controller"],
    [ErrorCode.PROVIDER_BLOCKED, "Review provider permissions"],
    [ErrorCode.PROTOCOL_INCOMPATIBLE, "Upgrade or migrate the provider"],
    [ErrorCode.PROFILE_SERVER_UNAVAILABLE, "Restore and verify the profile server"],
    [ErrorCode.OUTCOME_UNKNOWN, "Outcome unresolved"],
  ];
  for (const [code, expected] of cases) {
    const action = code === ErrorCode.OUTCOME_UNKNOWN ? ActionClass.REFRESH_SNAPSHOT : ActionClass.OPERATOR_REPAIR;
    const error = new ConnectError("private diagnostic", Code.FailedPrecondition, undefined,
      [{desc: DomainErrorSchema, value: {code, action}}]);
    const shown = operatorError(error);
    expect(shown).toContain(expected);
    expect(shown).not.toContain("private diagnostic");
    expect(externalActionRequired(error)).toBe(code !== ErrorCode.OUTCOME_UNKNOWN);
  }
  expect(externalActionRequired(new Error("transport"))).toBe(false);
  const refreshable = new ConnectError("private diagnostic", Code.FailedPrecondition, undefined,
    [{desc: DomainErrorSchema, value: {code: ErrorCode.PROVIDER_BLOCKED, action: ActionClass.REFRESH_SNAPSHOT}}]);
  expect(externalActionRequired(refreshable)).toBe(false);
  expect(operatorError(refreshable)).toContain("Refresh the snapshot");
  const newExternal = new ConnectError("private diagnostic", Code.FailedPrecondition, undefined,
    [{desc: DomainErrorSchema, value: {code: ErrorCode.PERSISTENCE_UNAVAILABLE, action: ActionClass.OPERATOR_REPAIR}}]);
  expect(externalActionRequired(newExternal)).toBe(true);
  expect(operatorError(newExternal)).toContain("outside Gul");
  for (const code of [ErrorCode.CONTROLLER_CARRIER_INVALID, ErrorCode.OPERATOR_ACTION_REQUIRED,
    ErrorCode.WORKSPACE_IDENTITY_MISMATCH, ErrorCode.RECOVERY_REQUIRED]) {
    expect(externalActionRequired(new ConnectError("private diagnostic", Code.FailedPrecondition, undefined,
      [{desc: DomainErrorSchema, value: {code, action: ActionClass.OPERATOR_REPAIR}}]))).toBe(true);
  }
  expect(externalActionRequired(new ConnectError("private diagnostic", Code.FailedPrecondition, undefined,
    [{desc: DomainErrorSchema, value: {code: ErrorCode.PROFILE_MISSING, action: ActionClass.OPERATOR_REPAIR}},
      {desc: DomainErrorSchema, value: {code: ErrorCode.PROFILE_MISSING, action: ActionClass.OPERATOR_REPAIR}}]))).toBe(false);
});
