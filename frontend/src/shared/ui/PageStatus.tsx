import { useLanguage } from "../../app/preferences";
import { Button } from "./Button";
export function PageStatus({
  loading,
  retry,
}: {
  loading?: boolean;
  retry?: () => void;
}) {
  const en = useLanguage() === "en";
  return (
    <section
      className="console-card p-12 text-center"
      role={loading ? "status" : "alert"}
    >
      <p>
        {loading
          ? en
            ? "Loading…"
            : "正在加载…"
          : en
            ? "Unable to load this page."
            : "页面数据加载失败，请重试。"}
      </p>
      {retry && (
        <Button className="mt-4" onClick={retry}>
          {en ? "Try again" : "重新加载"}
        </Button>
      )}
    </section>
  );
}
