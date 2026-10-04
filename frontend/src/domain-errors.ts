import {ConnectError} from "@connectrpc/connect";
import {ActionClass, DomainErrorSchema, ErrorCode, type DomainError} from "../../api/generated/ts/gul/v1/gul_pb";

const errorLabels: Partial<Record<ErrorCode, string>> = {
  [ErrorCode.WORKSPACE_NOT_PROVISIONED]: "Workspace is not provisioned. Provision it in Dolgorae, then reopen this workspace.",
  [ErrorCode.PROFILE_MISSING]: "Profile is missing. Configure a supported provider profile outside Gul.",
  [ErrorCode.CONTROLLER_MISMATCH]: "Controller ownership was lost. Verify or reset the Controller in Dolgorae.",
  [ErrorCode.CONTROLLER_CARRIER_INVALID]: "Controller credentials are invalid. Reset the Controller in Dolgorae.",
  [ErrorCode.PROVIDER_BLOCKED]: "Provider root is denied. Review provider permissions outside Gul.",
  [ErrorCode.PROTOCOL_INCOMPATIBLE]: "Provider is incompatible. Upgrade or migrate the provider outside Gul.",
  [ErrorCode.PROFILE_SERVER_UNAVAILABLE]: "Provider server is unavailable. Restore and verify the profile server outside Gul.",
  [ErrorCode.OPERATOR_ACTION_REQUIRED]: "Operator action is required in Dolgorae before continuing.",
  [ErrorCode.WORKSPACE_IDENTITY_MISMATCH]: "Workspace identity differs. Verify the workspace registration outside Gul.",
  [ErrorCode.OUTCOME_UNKNOWN]: "Outcome unresolved. Inspect current provider state before any new action.",
  [ErrorCode.RECOVERY_REQUIRED]: "Recovery is required. Follow the provider recovery procedure outside Gul.",
};

export function domainErrorMessage(code: ErrorCode, action?: ActionClass) {
  if (code === ErrorCode.PROVIDER_BLOCKED && action === ActionClass.USE_SUPPORTED_PROFILE) {
    return "Launch configuration is unsupported. Review the selected profile and launch options.";
  }
  if (action !== undefined && !externalActions.has(action) && code !== ErrorCode.OUTCOME_UNKNOWN) {
    return "Current provider state is unavailable. Refresh the snapshot.";
  }
  return errorLabels[code] ?? (action !== undefined && externalActions.has(action)
    ? "Provider repair is required outside Gul before continuing."
    : "Current provider state is unavailable. Refresh the snapshot.");
}

export function operatorError(error: unknown, fallback = "Current session state is unavailable. Refresh the snapshot.") {
  if (error instanceof ConnectError) {
    const details = error.findDetails(DomainErrorSchema);
    if (details.length === 1) return domainErrorMessage(details[0]!.code, details[0]!.action);
  }
  return fallback;
}

const externalActions = new Set<ActionClass>([
  ActionClass.OPERATOR_REPAIR, ActionClass.VERIFY_CONTROLLER,
  ActionClass.USE_COMPATIBLE_CONTROLLER, ActionClass.USE_NEW_SAME_PRINCIPAL_CONTROLLER,
  ActionClass.USE_SUPPORTED_PROFILE, ActionClass.RESTART_GATEWAY, ActionClass.FIX_SOCKET_PATH,
]);

export function domainErrorRequiresExternalAction(error: DomainError) {
  return externalActions.has(error.action);
}

export function externalActionRequired(error: unknown) {
  if (!(error instanceof ConnectError)) return false;
  const details = error.findDetails(DomainErrorSchema);
  return details.length === 1 && domainErrorRequiresExternalAction(details[0]!);
}
