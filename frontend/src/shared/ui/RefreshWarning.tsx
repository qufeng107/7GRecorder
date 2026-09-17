import { useLanguage } from "../../app/preferences";
import { Button } from "./Button";

export function RefreshWarning({
  failed,
  retry,
}: {
  failed: boolean;
  retry: () => void;
}) {
  const en = useLanguage() === "en";
  if (!failed) return null;
  return (
    <div
      role="alert"
      className="console-card flex flex-wrap items-center justify-between gap-3 p-4 text-sm"
    >
      <p>
        {en
          ? "Refresh failed. Previous data and unsaved edits are preserved."
          : "刷新失败，当前显示上次数据；未保存的修改已保留。"}
      </p>
      <Button onClick={retry}>{en ? "Try again" : "重新加载"}</Button>
    </div>
  );
}
