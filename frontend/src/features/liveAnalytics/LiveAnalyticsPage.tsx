import { useServerDraft } from "../../shared/forms/useServerDraft";
import { UnsavedChangesGuard } from "../../shared/forms/UnsavedChangesGuard";
import { Link } from "react-router-dom";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLanguage } from "../../app/preferences";
import { requestJson } from "../../shared/api/client";
import type {
  Credential,
  LiveAnalyticsConfig,
  LiveCaptureSession,
  RecordingProfile,
} from "../../shared/api/contracts.generated";
import { Button } from "../../shared/ui/Button";
import { PageStatus } from "../../shared/ui/PageStatus";
import { formatBytes } from "../../shared/console/format";

type List<T> = { items: T[]; total: number };

export default function LiveAnalyticsPage() {
  const language = useLanguage(), en = language === "en";
  const client = useQueryClient();
  const [profileID, setProfileID] = useState(0);
  const [label, setLabel] = useState("");
  const [accessKeyID, setAccessKeyID] = useState("");
  const [accessKeySecret, setAccessKeySecret] = useState("");
  const [identityCode, setIdentityCode] = useState("");

  const profiles = useQuery({ queryKey: ["profiles"], queryFn: () => requestJson<List<RecordingProfile>>("/api/v1/recording-profiles") });
  const credentials = useQuery({ queryKey: ["credentials"], queryFn: () => requestJson<List<Credential>>("/api/v1/credentials") });
  useEffect(() => {
    if (!profileID && profiles.data?.items[0]) setProfileID(profiles.data.items[0].id);
  }, [profileID, profiles.data?.items]);
  const config = useQuery({
    queryKey: ["live-analytics-config", profileID],
    queryFn: () => requestJson<LiveAnalyticsConfig>(`/api/v1/recording-profiles/${profileID}/live-analytics`),
    enabled: profileID > 0,
  });
  const sessions = useQuery({
    queryKey: ["live-analytics-sessions", profileID],
    queryFn: () => requestJson<List<LiveCaptureSession>>(`/api/v1/live-analytics/sessions?profile_id=${profileID}`),
    enabled: profileID > 0,
    refetchInterval: 5000,
  });
  const draft = useServerDraft({
    appID: config.data?.app_id ? String(config.data.app_id) : "",
    credentialID: config.data?.credential_id ?? 0,
    enabled: config.data?.enabled ?? false,
  });
  const { appID, credentialID, enabled } = draft.form;
  const setAppID = (value: string) => draft.change(current => ({...current, appID: value}));
  const setCredentialID = (value: number) => draft.change(current => ({...current, credentialID: value}));
  const setEnabled = (value: boolean) => draft.change(current => ({...current, enabled: value}));
  const secretDirty = Boolean(label || accessKeyID || accessKeySecret || identityCode);
  const openLiveCredentials = useMemo(() => (credentials.data?.items ?? []).filter((item) => item.platform === "bilibili_open_live" && item.purpose === "LIVE_ANALYTICS"), [credentials.data?.items]);

  const createCredential = useMutation({
    mutationFn: () => requestJson<Credential>("/api/v1/credentials", { method: "POST", body: JSON.stringify({
      scope: "SYSTEM", platform: "bilibili_open_live", purpose: "LIVE_ANALYTICS",
      account_label: label, secret: { access_key_id: accessKeyID, access_key_secret: accessKeySecret, identity_code: identityCode },
    }) }),
    onSuccess: (created) => {
      setCredentialID(created.id); setAccessKeyID(""); setAccessKeySecret(""); setIdentityCode(""); setLabel("");
      void client.invalidateQueries({ queryKey: ["credentials"] });
    },
  });
  const save = useMutation({
    mutationFn: (submitted: { profileID: number; form: typeof draft.form }) => requestJson<LiveAnalyticsConfig>(`/api/v1/recording-profiles/${submitted.profileID}/live-analytics`, {
      method: "PUT", body: JSON.stringify({ app_id: Number(submitted.form.appID), credential_id: submitted.form.credentialID, enabled: submitted.form.enabled }),
    }),
    onSuccess: (saved, submitted) => {
      client.setQueryData(["live-analytics-config", submitted.profileID], saved);
      draft.saved(submitted.form);
      void client.invalidateQueries({ queryKey: ["live-analytics-config", submitted.profileID] });
      void client.invalidateQueries({ queryKey: ["live-analytics-sessions", profileID] });
    },
  });

  if (profiles.isPending || credentials.isPending) return <PageStatus loading />;
  if (profiles.isError || credentials.isError) return <PageStatus retry={() => { void profiles.refetch(); void credentials.refetch(); }} />;
  return <div className="feature-page space-y-6">
    <UnsavedChangesGuard dirty={draft.dirty || secretDirty} />
    <header><p className="text-xs font-semibold uppercase tracking-widest text-accent">OpenLive</p><h1 className="mt-2 text-3xl font-semibold">{en ? "Live operations analytics" : "直播运营分析"}</h1><p className="mt-2 text-sm text-muted">{en ? "Capture authorized live events first; analysis will remain traceable to raw evidence." : "先完整采集获授权直播事件，后续分析均可追溯到原始证据。"}</p></header>
    <section className="console-card space-y-4 p-5">
      <h2 className="text-lg font-semibold">{en ? "Collector configuration" : "采集配置"}</h2>
      <label className="grid gap-2 text-sm">{en ? "Recording profile" : "录制配置"}<select className="console-input" value={profileID} disabled={save.isPending || createCredential.isPending} onChange={(event) => {
        if ((draft.dirty || secretDirty) && !window.confirm(en ? "Discard unsaved changes?" : "放弃未保存的修改？")) return;
        draft.discard(); setLabel(""); setAccessKeyID(""); setAccessKeySecret(""); setIdentityCode("");
        setProfileID(Number(event.target.value));
      }}>{(profiles.data?.items ?? []).map((profile) => <option key={profile.id} value={profile.id}>{profile.name} · {profile.room_id}</option>)}</select></label>
      <div className="grid gap-4 md:grid-cols-2">
        <label className="grid gap-2 text-sm">App ID<input className="console-input" inputMode="numeric" value={appID} onChange={(event) => setAppID(event.target.value)} /></label>
        <label className="grid gap-2 text-sm">{en ? "OpenLive credential" : "OpenLive 凭证"}<select className="console-input" value={credentialID} onChange={(event) => setCredentialID(Number(event.target.value))}><option value={0}>{en ? "Select credential" : "请选择凭证"}</option>{openLiveCredentials.map((item) => <option key={item.id} value={item.id}>{item.account_label}</option>)}</select></label>
      </div>
      <label className="flex items-center gap-3 text-sm"><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />{en ? "Keep the server collector running" : "持续运行服务器采集器"}</label>
      {config.data?.last_error ? <p className="rounded-md bg-red-50 p-3 text-sm text-red-700">{config.data.last_error}</p> : null}
      <Button disabled={save.isPending || config.isPending || config.isError || !profileID || !Number.isSafeInteger(Number(appID)) || (enabled && (Number(appID) <= 0 || !credentialID))} onClick={() => save.mutate({profileID, form: draft.form})}>{save.isPending ? (en ? "Saving…" : "正在保存…") : (en ? "Save collector" : "保存采集配置")}</Button>
      {save.isSuccess && !draft.dirty ? <p role="status">{en ? "Saved." : "已保存。"}</p> : null}
      {save.isError ? <p role="alert" className="text-sm text-red-700">{en ? "Could not save collector configuration." : "采集配置保存失败。"}</p> : null}
    </section>
    <section className="console-card space-y-4 p-5">
      <h2 className="text-lg font-semibold">{en ? "Add encrypted credential" : "新增加密凭证"}</h2>
      <p className="text-sm text-muted">{en ? "Secrets stay in memory until submission and are never returned by the API." : "密钥和身份码仅在提交前保留于内存，API 保存后不会回显。"}</p>
      <fieldset disabled={createCredential.isPending} className="grid gap-4 md:grid-cols-2"><label className="grid gap-2 text-sm">{en ? "Label" : "名称"}<input className="console-input" value={label} onChange={(e) => setLabel(e.target.value)} /></label><label className="grid gap-2 text-sm">Access Key ID<input className="console-input" value={accessKeyID} onChange={(e) => setAccessKeyID(e.target.value)} /></label><label className="grid gap-2 text-sm">Access Key Secret<input className="console-input" type="password" value={accessKeySecret} onChange={(e) => setAccessKeySecret(e.target.value)} /></label><label className="grid gap-2 text-sm">{en ? "Streamer identity code" : "主播身份码"}<input className="console-input" type="password" value={identityCode} onChange={(e) => setIdentityCode(e.target.value)} /></label></fieldset>
      <Button disabled={createCredential.isPending || !label || !accessKeyID || !accessKeySecret || !identityCode} onClick={() => createCredential.mutate()}>{en ? "Save credential" : "保存凭证"}</Button>
      {createCredential.isError ? <p role="alert" className="text-sm text-red-700">{en ? "Could not save credential." : "凭证保存失败。"}</p> : null}
    </section>
    <section className="console-card p-5"><div className="flex items-center justify-between"><h2 className="text-lg font-semibold">{en ? "Capture sessions" : "采集场次"}</h2><Button onClick={() => void sessions.refetch()}>{en ? "Refresh" : "刷新"}</Button></div>
      <div className="mt-4 overflow-auto"><table className="w-full min-w-[1000px] text-left text-sm"><thead className="text-xs uppercase text-muted"><tr><th className="p-2">{en ? "Status" : "状态"}</th><th className="p-2">{en ? "Started" : "开始"}</th><th className="p-2">{en ? "Last event" : "最后事件"}</th><th className="p-2">{en ? "Events" : "事件数"}</th><th className="p-2">{en ? "Types" : "事件类型"}</th><th className="p-2">{en ? "Raw evidence" : "原始证据"}</th><th className="p-2">{en ? "Gaps" : "缺口"}</th></tr></thead><tbody>{(sessions.data?.items ?? []).map((item) => <tr key={item.id} className="border-t border-border"><td className="p-2 font-medium"><Link className="text-accent underline" to={`/admin/live-analytics/sessions/${item.id}`}>{item.status} #{item.id}</Link></td><td className="p-2">{item.started_at}</td><td className="p-2">{item.last_event_at || "—"}</td><td className="p-2">{item.event_count}</td><td className="p-2 text-xs">{Object.entries(item.event_counts ?? {}).map(([cmd, count]) => `${cmd}: ${count}`).join(" · ") || "—"}</td><td className="p-2">{item.raw_status} · {formatBytes(item.raw_size_bytes)}</td><td className="p-2">{item.gap_count}</td></tr>)}</tbody></table></div>
      {!sessions.data?.items.length ? <p className="mt-4 text-sm text-muted">{en ? "No capture session yet." : "暂无采集场次。"}</p> : null}
    </section>
  </div>;
}
