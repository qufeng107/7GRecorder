import type { ReactNode } from "react";
import { useSession } from "../features/session/useSession";
import { useLanguage } from "./preferences";
export function FeatureGate({
  access,
  children,
}: {
  access?: "admin" | "uploads";
  children: ReactNode;
}) {
  const { data } = useSession();
  const language = useLanguage();
  const allowed =
    !access ||
    data?.user.role === "SUPER_ADMIN" ||
    (access === "uploads" &&
      (data?.policy?.can_edit_bilibili_module ||
        data?.policy?.can_edit_cos_module));
  if (!allowed)
    return (
      <section className="console-card p-8" role="alert">
        {language === "zh"
          ? "你没有访问此页面的权限。"
          : "You do not have access to this page."}
      </section>
    );
  return children;
}
