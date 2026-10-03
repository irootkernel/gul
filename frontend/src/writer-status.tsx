import {WriterAccessMode} from "../../api/generated/ts/gul/v1/gul_pb";

// The backend's shared evaluator owns the mode; the component never infers
// write authority from connection health or a pending request.
export function WriterStatus({mode}: {mode: WriterAccessMode}) {
  const label = mode === WriterAccessMode.WRITE ? "WRITE"
    : mode === WriterAccessMode.READ_ONLY ? "Read-only"
    : mode === WriterAccessMode.UNVERIFIED ? "Policy unverified" : "Blocked";
  return <span role="status" aria-label="Writer access">{label}</span>;
}
