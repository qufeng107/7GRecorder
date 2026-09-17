import { uiCopy } from "../../shared/console/copy";
import { OverviewPanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function OverviewPage() {
  const language = useLanguage();
  const ui = uiCopy[language];
  return (
    <div className="feature-page space-y-6">
      <OverviewPanel statusRows={ui.statusRows} />
    </div>
  );
}
