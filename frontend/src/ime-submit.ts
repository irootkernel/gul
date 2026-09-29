import {useRef, type KeyboardEvent} from "react";

type KeySignal = Pick<KeyboardEvent<HTMLElement>, "key" | "code" | "keyCode"> & {isComposing: boolean};

function enterKey(event: Pick<KeySignal, "key" | "code" | "keyCode">) {
  return event.key === "Enter" || event.code === "Enter" || event.keyCode === 13;
}

// WebKit can end composition before the Enter keydown that commits it.
// A lost keyup must not leave later explicit submissions disabled forever.
export function createImeSubmitGuard(now = () => performance.now()) {
  let composing = false;
  let compositionEndedAt = -Infinity;
  let commitGuardUntil = 0;
  return {
    compositionStart() {composing = true; compositionEndedAt = -Infinity; commitGuardUntil = 0;},
    compositionEnd() {composing = false; compositionEndedAt = now();},
    inputBlur() {composing = false; compositionEndedAt = -Infinity; commitGuardUntil = 0;},
    keyDown(event: KeySignal) {
      if (enterKey(event) && (composing || event.isComposing || now() - compositionEndedAt <= 250))
        commitGuardUntil = now() + 1000;
    },
    keyUp(event: KeySignal) {if (enterKey(event)) {commitGuardUntil = 0; compositionEndedAt = -Infinity;}},
    explicitSubmit() {if (!composing) commitGuardUntil = 0;},
    blocksSubmit() {return composing || now() < commitGuardUntil;},
  };
}

export function useImeSubmitGuard() {
  const guard = useRef<ReturnType<typeof createImeSubmitGuard> | null>(null);
  guard.current ??= createImeSubmitGuard();
  return {
    onCompositionStart() {guard.current?.compositionStart();},
    onCompositionEnd() {guard.current?.compositionEnd();},
    onInputBlur() {guard.current?.inputBlur();},
    onKeyDown(event: KeyboardEvent<HTMLElement>) {
      guard.current?.keyDown({key: event.key, code: event.code, keyCode: event.keyCode,
        isComposing: event.nativeEvent.isComposing});
    },
    onKeyUp(event: KeyboardEvent<HTMLElement>) {
      guard.current?.keyUp({key: event.key, code: event.code, keyCode: event.keyCode,
        isComposing: event.nativeEvent.isComposing});
    },
    onSubmitButtonKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
      if (!event.nativeEvent.isComposing && (enterKey(event) || event.key === " " || event.code === "Space")) {
        guard.current?.explicitSubmit();
        // Keep the button's deliberate keypress out of the form's commit-Enter guard.
        event.stopPropagation();
      }
    },
    explicitSubmit() {guard.current?.explicitSubmit();},
    blocksSubmit() {return guard.current?.blocksSubmit() ?? false;},
  };
}
