import { useQuery } from "@tanstack/react-query";
import { requestJson } from "../../shared/api/client";
import { useLanguage } from "../../app/preferences";
import { Button } from "../../shared/ui/Button";

type Timeline = { items: Array<{ minute: string; cmd: string; event_count: number }>; truncated: boolean };
export function CaptureTimeline({ sessionId }: { sessionId: string | number }) {
  const en = useLanguage() === "en";
  const query = useQuery({
    queryKey: ["capture-timeline", sessionId],
    queryFn: () => requestJson<Timeline>(`/api/v1/live-analytics/sessions/${sessionId}/timeline`),
    refetchInterval: 10000,
  });
  const byMinute = new Map<string, number>();
  const byCommand = new Map<string, number>();
  for (const item of query.data?.items ?? []) {
    byMinute.set(item.minute, (byMinute.get(item.minute) ?? 0) + item.event_count);
    byCommand.set(item.cmd, (byCommand.get(item.cmd) ?? 0) + item.event_count);
  }
  const minutes = [...byMinute.entries()];
  const recent = minutes.slice(-120);
  const peak = Math.max(1, ...recent.map(([, count]) => count));
  return <section className="console-card space-y-4 p-5">
    <h2 className="text-lg font-semibold">{en ? "Received events by minute" : "每分钟接收事件"}</h2>
    <p className="text-sm text-muted">{en ? "Receipt-time counts, including repeated deliveries. These are not unique viewers or net gift revenue. Missing collection intervals are not evidence of zero engagement." : "按接收时间统计消息包，可能包含重复推送；不代表独立观众或礼物净收入。采集中断时段不代表零互动。"}</p>
    {query.isError ? <><p role="alert">{en ? "Could not load timeline." : "分钟趋势加载失败。"}</p><Button onClick={() => void query.refetch()}>{en ? "Retry" : "重试"}</Button></> : query.isPending ? <p>{en ? "Loading…" : "正在加载…"}</p> : <>
      {query.data.truncated ? <p role="status">{en ? "Showing the latest 1440 observed minutes." : "仅显示最近 1440 个有事件的分钟。"}</p> : null}
      {recent.length ? <>
        <p className="text-xs text-muted">{en ? "Latest 120 observed minutes · UTC" : "最近 120 个有事件的分钟 · UTC"}</p>
        <svg role="img" aria-label={en ? "Received event counts by minute" : "每分钟接收事件柱状图"} viewBox="0 0 720 180" className="h-44 w-full text-accent">
          {recent.map(([minute, count], index) => <rect key={minute} x={index * 720 / recent.length} y={170 - count / peak * 150} width={Math.max(1, 720 / recent.length - 2)} height={count / peak * 150} fill="currentColor"><title>{minute}: {count}</title></rect>)}
        </svg>
        <div className="flex justify-between gap-2 text-xs text-muted"><span>{recent[0][0]}</span><span>{recent[recent.length - 1][0]}</span></div>
      </> : <p>{en ? "No persisted minute counts yet." : "暂无已保存的分钟计数。"}</p>}
      <div className="flex flex-wrap gap-2">{[...byCommand.entries()].map(([cmd, count]) => <span key={cmd} className="break-all rounded-md border border-border px-3 py-2 text-xs">{cmd.replace("LIVE_OPEN_PLATFORM_", "")}: {count}</span>)}</div>
      {minutes.length ? <details><summary className="cursor-pointer text-sm">{en ? "Minute data table" : "查看分钟数据表"}</summary><div className="mt-3 max-h-72 overflow-auto"><table className="w-full text-left text-sm"><thead><tr><th>UTC</th><th>{en ? "Events" : "事件包数"}</th></tr></thead><tbody>{minutes.map(([minute, count]) => <tr key={minute}><td>{minute}</td><td>{count}</td></tr>)}</tbody></table></div></details> : null}
    </>}
  </section>;
}
