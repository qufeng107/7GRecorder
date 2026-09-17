import { useLanguage } from "../../app/preferences";
import { Button } from "../ui/Button";
export function DraftFeedback({
  title,
  dirty,
  saved,
  pending,
  onDiscard,
}: {
  title: string;
  dirty: boolean;
  saved: boolean;
  pending: boolean;
  onDiscard: () => void;
}) {
  const en = useLanguage() === "en";
  if (!dirty && !saved && !pending) return null;
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
      <p role="status">
        {title} ·{" "}
        {pending
          ? en
            ? "Saving…"
            : "正在保存…"
          : dirty
            ? en
              ? "Unsaved changes"
              : "有未保存的修改"
            : en
              ? "Saved"
              : "已保存"}
      </p>
      {dirty && (
        <Button disabled={pending} onClick={onDiscard}>
          {en ? "Discard changes" : "放弃修改"}
        </Button>
      )}
    </div>
  );
}
