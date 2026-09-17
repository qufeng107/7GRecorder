// Synthetic development data only. No external calls and no stored secret values.
export function createConsoleFixtures() {
  const stamp = "2026-09-17T10:00:00Z";
  const profiles = [
    {
      id: 1,
      name: "晚风电台",
      owner_user_id: 1,
      owner_username: "demo",
      platform: "bilibili",
      room_id: "10001",
      streamer_name: "晚风",
      timezone: "Asia/Shanghai",
      enabled: false,
      public_enabled: false,
      public_slug: "",
      archived_at: "",
      recording_settings: {
        auto_record: false,
        quality: "original",
        record_danmaku: true,
        segment_duration_sec: 1800,
        finalize_grace_period_sec: 300,
      },
      runtime: {
        stream_status: "OFFLINE",
        recorder_status: "IDLE",
        sync_status: "SYNCED",
      },
    },
  ];
  const accounts = [
    {
      id: 1,
      username: "demo",
      role: "SUPER_ADMIN",
      enabled: true,
      profile_count: 1,
      created_at: stamp,
      updated_at: stamp,
    },
    {
      id: 2,
      username: "viewer",
      role: "MANAGER",
      enabled: true,
      profile_count: 0,
      created_at: stamp,
      updated_at: stamp,
    },
  ];
  const credentials = [
    {
      id: 1,
      owner_user_id: 1,
      scope: "USER",
      platform: "bilibili",
      purpose: "PUBLISHER",
      account_label: "演示投稿账号",
      status: "UNVERIFIED",
      created_at: stamp,
      updated_at: stamp,
    },
  ];
  const source = {
    id: 20,
    recording_profile_id: 1,
    profile_name: "晚风电台",
    room_id: "10001",
    streamer_name: "晚风",
    title: "秋日晚风 · 聊天与音乐",
    started_at: stamp,
    completed_at: "2026-09-17T11:00:00Z",
    duration_ms: 3600000,
    status: "READY_TO_UPLOAD",
    total_bytes: 102400000,
    local_protected: false,
    recording_count: 1,
    file_count: 1,
    max_gap_seconds: 0,
    merge_gap_threshold_seconds: 600,
    review_status: "REQUIRED",
    edit_decision_json: "",
    local_cleanup_status: "AVAILABLE",
    bilibili_status: "PENDING",
    cos_status: "AVAILABLE",
    segments: [
      {
        id: 1,
        upload_source_id: 20,
        recording_id: 10,
        recording_file_id: 100,
        sort_order: 0,
        source_started_at: stamp,
        source_completed_at: "2026-09-17T11:00:00Z",
        timeline_start_ms: 0,
        timeline_end_ms: 3600000,
        relative_path: "recordings/demo/source.flv",
        size_bytes: 102400000,
        duration_ms: 3600000,
      },
    ],
    outputs: [
      {
        id: 200,
        upload_source_id: 20,
        part_index: 1,
        status: "READY_TO_UPLOAD",
        relative_path: "upload-sources/demo/p01.mp4",
        size_bytes: 102400000,
        duration_ms: 3600000,
        timeline_start_ms: 0,
        timeline_end_ms: 3600000,
        cos_status: "AVAILABLE",
        bilibili_status: "PENDING",
      },
    ],
    danmaku_files: [],
  };
  let bilibili = {
    recording_profile_id: 1,
    platform: "bilibili",
    credential_id: 1,
    enabled: false,
    settings: {},
  };
  let cos = {
    recording_profile_id: 1,
    credential_id: 0,
    enabled: false,
    region: "",
    bucket: "",
    prefix: "demo/",
    max_managed_bytes: 1073741824,
  };
  let storageSettings = {
    max_recording_bytes: 10737418240,
    min_system_free_bytes: 1073741824,
    cleanup_target_ratio: 0.85,
    absolute_emergency_free_bytes: 536870912,
  };
  let tls = {
    enabled: false,
    primary_domain: "test.invalid",
    additional_domains: [],
    status: "DISABLED",
    updated_at: stamp,
  };
  let editing = false;
  const list = (items: unknown[], empty: boolean) => ({
    items: empty ? [] : items,
    total: empty ? 0 : items.length,
  });
  return (
    path: string,
    method: string,
    payload: Record<string, unknown>,
    scenario: string,
    role: string,
  ): { status: number; data: unknown } | undefined => {
    const empty = scenario === "empty";
    const ok = (data: unknown) => ({ status: 200, data });
    if (
      (path.startsWith("/api/v1/system/") ||
        path.startsWith("/api/v1/storage/") ||
        path.startsWith("/api/v1/accounts")) &&
      role !== "SUPER_ADMIN"
    )
      return { status: 403, data: { error: { code: "FORBIDDEN" } } };
    if (path === "/api/v1/recording-profiles") {
      if (method === "POST") {
        if (!payload.name || !payload.room_id || !payload.streamer_name)
          return {
            status: 400,
            data: { error: { code: "VALIDATION_FAILED" } },
          };
        if (role !== "SUPER_ADMIN") return { status: 403, data: {} };
        const next = { ...profiles[0], ...payload, id: profiles.length + 1 };
        profiles.push(next);
        return ok(next);
      }
      return ok(list(profiles, empty));
    }
    if (
      path.match(/^\/api\/v1\/recording-profiles\/\d+(\/recording-settings)?$/)
    ) {
      const profile = profiles.find((p) => p.id === Number(path.split("/")[4]));
      if (!profile) return { status: 404, data: {} };
      if (method !== "GET" && role !== "SUPER_ADMIN")
        return { status: 403, data: {} };
      if (path.endsWith("recording-settings")) {
        Object.assign(profile.recording_settings, payload);
        return ok(profile.recording_settings);
      }
      Object.assign(profile, payload);
      return ok(profile);
    }
    if (path.endsWith("/publishing/bilibili")) {
      if (method !== "GET") bilibili = { ...bilibili, ...payload };
      return ok(bilibili);
    }
    if (path.endsWith("/storage/cos")) {
      if (method !== "GET") cos = { ...cos, ...payload };
      return ok(cos);
    }
    if (path === "/api/v1/credentials") {
      if (method === "POST") {
        const item = {
          ...credentials[0],
          id: credentials.length + 1,
          platform: String(payload.platform),
          account_label: String(payload.account_label),
        };
        credentials.push(item);
        return ok(item);
      }
      return ok(list(credentials, empty));
    }
    if (path === "/api/v1/accounts") {
      if (method === "POST") {
        const item = {
          ...accounts[1],
          id: accounts.length + 1,
          username: String(payload.username),
        };
        accounts.push(item);
        return ok(item);
      }
      return ok(list(accounts, empty));
    }
    if (/^\/api\/v1\/accounts\/\d+$/.test(path)) {
      const item = accounts.find((a) => a.id === Number(path.split("/")[4]));
      if (!item) return { status: 404, data: {} };
      const { username, enabled } = payload;
      if (typeof username === "string") item.username = username;
      if (typeof enabled === "boolean") item.enabled = enabled;
      return ok(item);
    }
    if (path.endsWith("/policy")) return ok({ ...payload, updated_at: stamp });
    if (path === "/api/v1/upload-sources") return ok(list([source], empty));
    if (path === "/api/v1/recordings") return ok(list([], empty));
    if (path === "/api/v1/upload-sources/20/actions/require-review") {
      source.review_status = "REQUIRED";
      return ok(source);
    }
    if (path === "/api/v1/upload-sources/20/actions/apply-edit") {
      editing = true;
      source.edit_decision_json = JSON.stringify(payload);
      return ok(source);
    }
    if (path === "/api/v1/upload-sources/20/actions/approve-review") {
      if (editing)
        return { status: 409, data: { error: { code: "EDIT_PENDING" } } };
      source.review_status = "APPROVED";
      return ok(source);
    }
    if (path.endsWith("/actions/download-url"))
      return ok({
        url: "/__mock/download",
        expires_at: stamp,
        object_key: "demo/p01.mp4",
      });
    if (path === "/api/v1/storage/local/settings") {
      storageSettings = { ...storageSettings, ...payload };
      return ok(storageSettings);
    }
    if (path === "/api/v1/storage/local")
      return ok({
        data_root: "/synthetic",
        disk_total_bytes: 100000000000,
        disk_free_bytes: 80000000000,
        disk_available_bytes: 80000000000,
        indexed_video_bytes: 102400000,
        indexed_video_files: 1,
        protected_recordings: 0,
        completed_recordings: 1,
        settings_configured: true,
        health: "OK",
        need_reclaim_bytes: empty ? 0 : 102400000,
        target_video_bytes: 0,
        settings: storageSettings,
      });
    if (path === "/api/v1/storage/local/cleanup-candidates")
      return ok({
        items: empty
          ? []
          : [
              {
                recording_id: 10,
                title: "合成清理候选",
                profile_name: "晚风电台",
                room_id: "10001",
                completed_at: stamp,
                file_count: 1,
                reclaimable_bytes: 102400000,
              },
            ],
        total: empty ? 0 : 1,
        preview_reclaimable_bytes: empty ? 0 : 102400000,
      });
    if (path === "/api/v1/storage/local/actions/cleanup")
      return ok({
        deleted_recordings: 0,
        deleted_files: 0,
        reclaimed_bytes: 0,
        skipped_recordings: 0,
      });
    if (path === "/api/v1/system/site-tls") {
      if (method === "PUT") tls = { ...tls, ...payload };
      return ok(tls);
    }
    if (path === "/api/v1/song-settings")
      return ok({
        enabled: false,
        region: "",
        container_id: "",
        songs_prefix: "songs",
        boundary_padding_ms: 0,
        algorithm_version: "v1",
        updated_at: stamp,
        ...payload,
      });
    if (
      [
        "/api/v1/song-analysis/sources",
        "/api/v1/song-analysis/runs",
        "/api/v1/songs",
      ].includes(path)
    )
      return ok(list([], empty));
    return undefined;
  };
}
