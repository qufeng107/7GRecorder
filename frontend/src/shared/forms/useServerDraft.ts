import { useState, type SetStateAction } from "react";

// Plain immutable form values only. Server refreshes never replace a local draft.
export function useServerDraft<T>(server: T) {
  const [draft, setDraft] = useState<T | null>(null);
  const form = draft ?? server;
  return {
    form,
    dirty: draft !== null,
    change(value: SetStateAction<T>) {
      setDraft((current) =>
        typeof value === "function"
          ? (value as (previous: T) => T)(current ?? server)
          : value,
      );
    },
    discard() {
      setDraft(null);
    },
    saved(submitted: T) {
      // A later edit is a new object and must survive an older request's response.
      setDraft((current) => (current === submitted ? null : current));
    },
  };
}
