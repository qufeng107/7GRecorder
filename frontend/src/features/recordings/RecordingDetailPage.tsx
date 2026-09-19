import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { useParams } from "react-router-dom";
import { useLanguage } from "../../app/preferences";
import { requestJson } from "../../shared/api/client";
import type { LiveCaptureSession, UploadSourceItem } from "../../shared/api/contracts.generated";
import { PageStatus } from "../../shared/ui/PageStatus";
import { formatBytes } from "../../shared/console/format";

type List<T> = { items: T[]; total: number };

export default function RecordingDetailPage() {
  const { sourceId = "" } = useParams(), id = Number(sourceId);
  const language = useLanguage(), en = language === "en";
  const source = useQuery({ queryKey: ["upload-source", id], queryFn: () => requestJson<UploadSourceItem>(`/api/v1/upload-sources/${id}`), enabled: id > 0 });
  const sessions = useQuery({ queryKey: ["live-analytics-sessions", source.data?.recording_profile_id], queryFn: () => requestJson<List<LiveCaptureSession>>(`/api/v1/live-analytics/sessions?profile_id=${source.data?.recording_profile_id}`), enabled: Boolean(source.data?.recording_profile_id), refetchInterval: 5000 });
  if (source.isPending) return <PageStatus loading />;
  if (source.isError || !source.data) return <PageStatus retry={() => void source.refetch()} />;
  const item = source.data;
  const relatedSessions = (sessions.data?.items ?? []).filter((session) => {
    const started = Date.parse(session.started_at), sourceStart = Date.parse(item.started_at), sourceEnd = Date.parse(item.completed_at || new Date().toISOString());
    return started <= sourceEnd && (Date.parse(session.ended_at || new Date().toISOString()) >= sourceStart);
  });
  return <div className="feature-page space-y-6">
    <header><p className="text-xs font-semibold uppercase tracking-widest text-accent">{en ? "Live session" : "直播场次"}</p><h1 className="mt-2 text-3xl font-semibold">{item.title || `${item.profile_name} · ${item.started_at}`}</h1><p className="mt-2 text-sm text-muted">{item.profile_name} · {item.room_id} · {item.started_at} — {item.completed_at}</p></header>
    <div className="grid gap-4 md:grid-cols-3"><Metric label={en ? "Source segments" : "原始分段"} value={String(item.segments?.length ?? 0)} /><Metric label={en ? "Publish parts" : "发布分片"} value={String(item.outputs?.length ?? 0)} /><Metric label={en ? "Overlapping collector sessions" : "重叠采集连接"} value={String(relatedSessions.length)} /></div>
    <Section title={en ? "Overview" : "场次概览"}><dl className="grid gap-3 text-sm md:grid-cols-3"><Fact label={en ? "Status" : "状态"} value={item.status} /><Fact label={en ? "Duration" : "时长"} value={`${Math.round((item.duration_ms ?? 0) / 60000)} min`} /><Fact label={en ? "Local size" : "本地大小"} value={formatBytes((item.outputs ?? []).reduce((sum, output) => sum + (output.size_bytes ?? 0), 0))} /></dl></Section>
    <Section title={en ? "Video and files" : "视频与文件"}><div className="space-y-2 text-sm">{(item.outputs ?? []).map((output) => <div key={output.id} className="rounded-md border border-border p-3"><p className="font-medium">{output.relative_path}</p><p className="mt-1 text-muted">{formatBytes(output.size_bytes ?? 0)} · COS {output.cos_status || "—"} · Bilibili {output.bilibili_status || "—"}</p></div>)}</div></Section>
    <Section title={en ? "Interaction trends" : "互动趋势"}><AnalyticsPending en={en} sessions={relatedSessions} /></Section>
    <div className="grid gap-4 lg:grid-cols-2"><Section title={en ? "Danmaku hotspots" : "弹幕热点"}><AnalyticsPending en={en} sessions={relatedSessions} /></Section><Section title={en ? "Gifts, SC and guards" : "礼物、SC 与上舰"}><AnalyticsPending en={en} sessions={relatedSessions} /></Section></div>
    <Section title={en ? "Capture quality" : "采集质量"}>{relatedSessions.length ? <div className="space-y-2 text-sm">{relatedSessions.map((session) => <div key={session.id} className="rounded-md border border-border p-3"><p className="font-medium">{session.status} · {session.event_count} {en ? "events" : "个事件"}</p><p className="mt-1 text-muted">{en ? "Raw evidence" : "原始证据"}: {session.raw_status} · {formatBytes(session.raw_size_bytes)}{session.raw_deleted_at ? ` · ${en ? "cleaned" : "已清理"} ${session.raw_deleted_at}` : ""}</p><p className="mt-1 text-muted">{en ? "Unknown" : "未知事件"}: {session.unknown_event_count} · {en ? "Gaps" : "采集缺口"}: {session.gap_count} · {en ? "Last event" : "最后事件"}: {session.last_event_at || "—"}</p></div>)}</div> : <p className="text-sm text-muted">{en ? "No overlapping OpenLive capture session was found. Missing collection is not counted as zero engagement." : "未找到时间重叠的 OpenLive 采集场次；缺少采集不会按零互动处理。"}</p>}</Section>
  </div>;
}

function Section({ title, children }: { title: string; children: ReactNode }) { return <section className="console-card p-5"><h2 className="mb-4 text-lg font-semibold">{title}</h2>{children}</section>; }
function Metric({ label, value }: { label: string; value: string }) { return <div className="console-card p-4"><p className="text-xs uppercase text-muted">{label}</p><p className="mt-2 text-2xl font-semibold">{value}</p></div>; }
function Fact({ label, value }: { label: string; value: string }) { return <div><dt className="text-muted">{label}</dt><dd className="mt-1 font-medium">{value}</dd></div>; }
function AnalyticsPending({ en, sessions }: { en: boolean; sessions: LiveCaptureSession[] }) { return <p className="text-sm text-muted">{sessions.length ? (en ? "Raw events are being retained. Aggregation will be enabled after real event samples validate the metric definitions." : "原始事件已保留；待真实样本验证指标口径后启用聚合分析。") : (en ? "No collected data is available for this time range." : "该时间范围暂无已采集数据。")}</p>; }
