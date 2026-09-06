import { Radio } from "lucide-react";
import { useParams } from "react-router-dom";

export function PublicStreamer() {
  const { slug } = useParams();

  return (
    <main className="min-h-screen bg-[#f7f8f5] text-ink">
      <div className="mx-auto flex max-w-4xl flex-col gap-4 px-4 py-8 sm:px-6">
        <Radio className="h-7 w-7 text-accent" aria-hidden="true" />
        <h1 className="text-3xl font-semibold tracking-normal">@{slug}</h1>
        <p className="text-sm leading-6 text-muted">
          公开主播页已预留。第一版只展示明确公开的录播归档与已确认歌曲，不暴露本地或 COS 原始下载地址。
        </p>
        <p className="text-sm leading-6 text-muted">
          Public streamer pages are reserved. The first version will only show explicitly public recording archives
          and confirmed songs, without exposing local or COS source download URLs.
        </p>
      </div>
    </main>
  );
}
