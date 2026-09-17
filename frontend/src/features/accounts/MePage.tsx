import { useQuery } from "@tanstack/react-query";
import { requestJson } from "../../shared/api/client";
import { type MeResponse } from "../../shared/console/types";
import { uiCopy } from "../../shared/console/copy";
import { MyAccountPanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function MePage() {
  const language = useLanguage();
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false,
  });
  const user = meQuery.data?.user;
  const ownPolicy = meQuery.data?.policy;
  const canManageSystemSettings = user?.role === "SUPER_ADMIN";
  const ui = uiCopy[language];
  if (!user) return null;
  return (
    <div className="feature-page space-y-6">
      <MyAccountPanel
        canManageSystemSettings={Boolean(canManageSystemSettings)}
        labels={ui}
        policy={ownPolicy}
        user={user}
      />
    </div>
  );
}
