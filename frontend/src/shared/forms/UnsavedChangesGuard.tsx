import { useEffect, useRef } from "react";
import { useBlocker } from "react-router-dom";
import { useLanguage } from "../../app/preferences";
import { Button } from "../ui/Button";

export function UnsavedChangesGuard({ dirty }: { dirty: boolean }) {
  const en = useLanguage() === "en";
  const blocker = useBlocker(dirty);
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [dirty]);
  useEffect(() => {
    if (blocker.state === "blocked") dialog.current?.showModal();
  }, [blocker.state]);
  if (blocker.state !== "blocked") return null;
  return (
    <dialog
      ref={dialog}
      aria-labelledby="unsaved-title"
      className="console-card m-auto w-[min(90vw,440px)] p-6 text-inherit backdrop:bg-black/40"
      onCancel={(event) => {
        event.preventDefault();
        blocker.reset();
      }}
    >
      <h2 id="unsaved-title" className="text-lg font-semibold">
        {en ? "Discard unsaved changes?" : "放弃未保存的修改？"}
      </h2>
      <p className="mt-3 text-sm text-muted">
        {en
          ? "Changes on this page will be lost."
          : "离开后，本页未保存的内容将丢失。"}
      </p>
      <div className="mt-6 flex justify-end gap-3">
        <Button autoFocus onClick={() => blocker.reset()}>
          {en ? "Keep editing" : "继续编辑"}
        </Button>
        <Button onClick={() => blocker.proceed()}>
          {en ? "Discard and leave" : "放弃并离开"}
        </Button>
      </div>
    </dialog>
  );
}
