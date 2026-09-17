import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useServerDraft } from "./useServerDraft";

describe("server-backed form drafts", () => {
  it("follows refreshes until edited, preserves edits, and discards to latest server", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useServerDraft({ value }),
      { initialProps: { value: 1 } },
    );
    rerender({ value: 2 });
    expect(result.current.form.value).toBe(2);
    act(() => result.current.change({ value: 3 }));
    rerender({ value: 4 });
    expect(result.current.form.value).toBe(3);
    expect(result.current.dirty).toBe(true);
    act(() => result.current.discard());
    expect(result.current.form.value).toBe(4);
    expect(result.current.dirty).toBe(false);
  });
  it("only clears the submitted snapshot and preserves edits during save", () => {
    const { result, rerender } = renderHook(
      ({ value }) => useServerDraft({ value }),
      { initialProps: { value: 1 } },
    );
    act(() => result.current.change({ value: 2 }));
    const submitted = result.current.form;
    act(() =>
      result.current.change((previous) => ({ value: previous.value + 1 })),
    );
    rerender({ value: 2 });
    act(() => result.current.saved(submitted));
    expect(result.current.form.value).toBe(3);
    expect(result.current.dirty).toBe(true);
    const next = result.current.form;
    rerender({ value: 3 });
    act(() => result.current.saved(next));
    expect(result.current.form.value).toBe(3);
    expect(result.current.dirty).toBe(false);
  });
});
