import { CaptureTimeline } from "./CaptureTimeline";
import { useState } from "react";
import { useParams, Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { requestJson } from "../../shared/api/client";
import type { LiveCaptureSession } from "../../shared/api/contracts.generated";
import { useLanguage } from "../../app/preferences";
import { PageStatus } from "../../shared/ui/PageStatus";
import { Button } from "../../shared/ui/Button";
import { formatBytes } from "../../shared/console/format";

type RawFile = { id: number; status: string; size_bytes: number; created_at: string; deleted_at?: string };

type EventPage = {
  items: Array<{ received_at: string; cmd: string; payload: unknown }>;
  next_offset: number;
  has_more: boolean;
};

export default function CaptureSessionPage() {
  const { sessionId } = useParams();
  const en = useLanguage() === "en";
  const [selected, setSelected] = useState<{ sessionId: string; fileId: number }>();
  const [paging, setPaging] = useState<{ fileId: number; cursors: number[] }>();
  const files = useQuery({
    queryKey: ["capture-files", sessionId],
    queryFn: () => requestJson<{ items: RawFile[]; truncated: boolean }>(`/api/v1/live-analytics/sessions/${sessionId}/raw-files`),
    refetchInterval: 5000,
  });
  const file = files.data?.items.find((item) => selected?.sessionId === sessionId && item.id === selected?.fileId) ?? files.data?.items[0];
  const cursors = paging?.fileId === file?.id ? paging?.cursors ?? [0] : [0];
  const offset = cursors[cursors.length - 1];
  const setCursors = (next: number[]) => { if (file) setPaging({ fileId: file.id, cursors: next }); };
  const session = useQuery({
    queryKey: ["capture-session", sessionId],
    queryFn: () => requestJson<LiveCaptureSession>(`/api/v1/live-analytics/sessions/${sessionId}`),
    refetchInterval: 5000,
  });
  const available = file?.status === "WRITING" || file?.status === "AVAILABLE";
  const events = useQuery({
    queryKey: ["capture-events", sessionId, file?.id, offset],
    queryFn: () => requestJson<EventPage>(`/api/v1/live-analytics/sessions/${sessionId}/events?file_id=${file?.id}&offset=${offset}&limit=50`),
    enabled: available,
    retry: false,
  });
  if (session.isPending) return <PageStatus loading />;
  if (session.isError) return <PageStatus retry={() => void session.refetch()} />;
  const item = session.data;
  return <div className="feature-page space-y-6">
    <header>
      <Link className="text-accent" to="/admin/recordings">{en ? "Recordings" : "录像管理"}</Link>
      <h1 className="mt-2 text-3xl font-semibold">{en ? "Capture evidence" : "采集事件"} #{item.id}</h1>
      <p className="mt-2 text-sm text-muted">{item.anchor_name} · {item.room_id} · {item.status}</p>
    </header>
    <section className="console-card grid gap-4 p-5 sm:grid-cols-3">
      <div>{en ? "Received events" : "接收事件"}<p className="text-2xl">{item.event_count}</p></div>
      <div>{en ? "Capture gaps" : "采集缺口"}<p className="text-2xl">{item.gap_count}</p></div>
      <div>{en ? "Raw evidence" : "原始证据"}<p>{item.raw_status} · {formatBytes(item.raw_size_bytes)}</p></div>
    </section>
    <CaptureTimeline sessionId={item.id} />
    <section className="console-card space-y-3 p-5">
      <label className="block" htmlFor="raw-file">{en ? "Evidence file" : "原文分片"}</label>
      {files.isPending ? <PageStatus loading /> : files.isError ? <PageStatus retry={() => void files.refetch()} /> : <select id="raw-file" className="w-full rounded-md border border-border bg-surface p-2" value={file?.id ?? ""} onChange={(event) => {
        setSelected({ sessionId: sessionId!, fileId: Number(event.target.value) });
        setPaging(undefined);
      }}>
        {!files.data.items.length ? <option value="">{en ? "No evidence files" : "尚无原文分片"}</option> : files.data.items.map((part) => <option key={part.id} value={part.id}>#{part.id} · {part.created_at} · {part.status} · {formatBytes(part.size_bytes)}</option>)}
      </select>}
      <p className="text-sm text-muted">{en ? "Closed files roll off independently. Minute totals remain; available files do not guarantee complete evidence for the whole session." : "已关闭分片会独立滚动清理，分钟计数保留。存在可读分片不代表整场直播原文完整。"}</p>
      {files.data?.truncated ? <p>{en ? "Showing the latest 500 files." : "仅显示最近 500 个分片。"}</p> : null}
    </section>
    {!available ? <section role="status" className="console-card p-5">
      {en ? "Raw evidence is unavailable. Retained counts do not mean that event details can still be reconstructed." : "原始证据尚未生成、已清理或已缺失。历史计数仍保留，但无法据此还原事件详情。"}
      {item.raw_deleted_at ? <p>{en ? "Cleaned at: " : "清理时间："}{item.raw_deleted_at}</p> : null}
    </section> : <section className="console-card space-y-4 p-5">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="mr-auto text-lg font-semibold">{en ? "Events in receipt order" : "按接收顺序查看事件"}</h2>
        <Button disabled={events.isFetching} onClick={() => void events.refetch()}>{en ? "Refresh" : "刷新"}</Button>
        <Button disabled={cursors.length === 1 || events.isFetching} onClick={() => setCursors(cursors.slice(0, -1))}>{en ? "Previous" : "上一页"}</Button>
        <Button disabled={!events.data?.has_more || events.data.next_offset === offset || events.isFetching} onClick={() => setCursors([...cursors, events.data!.next_offset])}>{en ? "Next" : "下一页"}</Button>
      </div>
      {events.isPending ? <PageStatus loading /> : events.isError ? <p role="alert">{en ? "Could not read evidence; it may have been cleaned. Refresh the session to check." : "事件读取失败，原文可能已被滚动清理，请刷新页面检查状态。"}</p> : <>
        {!events.data.items.length ? <p>{en ? "No complete event records on this page." : "本页暂无完整事件。"}</p> : null}
        {events.data.items.map((event, index) => <details key={`${offset}-${index}`} className="rounded-md border border-border p-3">
          <summary className="cursor-pointer break-all text-sm">{event.received_at} · {event.cmd}</summary>
          <pre className="mt-3 max-h-96 overflow-auto whitespace-pre-wrap break-all text-xs">{JSON.stringify(event.payload, null, 2)}</pre>
        </details>)}
      </>}
    </section>}
  </div>;
}
