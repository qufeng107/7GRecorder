import {
  type CredentialForm,
  type UploadSettingsForm,
} from "../../shared/console/types";

export const emptyCredentialForm: CredentialForm = {
  platform: "bilibili",
  account_label: "",
  external_uid: "",
  secret: "{}",
};

export const defaultBilibiliTitleTemplate =
  "{{profile_name}} {{date_compact}} 第{{live_ordinal}}场直播";

export const defaultBilibiliDescriptionTemplate =
  "主播：{{streamer_name}}\n直播间：{{room_id}}\n录制时间：{{started_at_china}} - {{completed_at_china}}\n分片：{{part_count}} 个\n\n由 7GRecorder 自动归档。";

export const emptyUploadSettingsForm: UploadSettingsForm = {
  profile_id: "",
  bilibili_enabled: false,
  bilibili_credential_id: "",
  bilibili_title_template: defaultBilibiliTitleTemplate,
  bilibili_description_template: defaultBilibiliDescriptionTemplate,
  bilibili_tags: "录播,七宫筱野",
  bilibili_copyright: 2,
  bilibili_source: "https://live.bilibili.com/{{room_id}}",
  bilibili_upload_limit: 1,
  cos_enabled: false,
  cos_credential_id: "",
  cos_region: "",
  cos_bucket: "",
  cos_prefix: "",
  cos_max_managed_gb: 100,
};

export function parseConfigJSON(value: string): unknown {
  const trimmed = value.trim();
  if (!trimmed) {
    return {};
  }
  return JSON.parse(trimmed) as unknown;
}

export function isPlainObject(
  value: unknown,
): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

export function bilibiliSettingsFromConfig(value: unknown) {
  const settings = isPlainObject(value) ? value : {};
  return {
    title_template:
      typeof settings.title_template === "string"
        ? settings.title_template
        : defaultBilibiliTitleTemplate,
    description_template:
      typeof settings.description_template === "string"
        ? settings.description_template
        : defaultBilibiliDescriptionTemplate,
    tags: Array.isArray(settings.tags)
      ? settings.tags
          .filter((tag): tag is string => typeof tag === "string")
          .join(",")
      : typeof settings.tags === "string"
        ? settings.tags
        : "录播,七宫筱野",
    copyright: typeof settings.copyright === "number" ? settings.copyright : 2,
    source:
      typeof settings.source === "string"
        ? settings.source
        : "https://live.bilibili.com/{{room_id}}",
    upload_limit:
      typeof settings.upload_limit === "number" ? settings.upload_limit : 1,
  };
}

export function bilibiliSettingsPayload(form: UploadSettingsForm) {
  return {
    title_template: form.bilibili_title_template,
    description_template: form.bilibili_description_template,
    tags: form.bilibili_tags
      .split(/[,\n，]/)
      .map((tag) => tag.trim())
      .filter(Boolean),
    copyright: form.bilibili_copyright,
    source: form.bilibili_source.trim(),
    upload_limit: Math.min(
      8,
      Math.max(1, Math.round(form.bilibili_upload_limit || 1)),
    ),
  };
}

export function cosSettingsFromConfig(
  config?: import("../../shared/api/contracts.generated").COSStorageConfig,
) {
  return {
    cos_enabled: config?.enabled ?? false,
    cos_credential_id: config?.credential_id
      ? String(config.credential_id)
      : "",
    cos_region: config?.region ?? "",
    cos_bucket: config?.bucket ?? "",
    cos_prefix: config?.prefix ?? "",
    cos_max_managed_gb: config
      ? Math.max(0, Math.round(config.max_managed_bytes / 1024 ** 3))
      : emptyUploadSettingsForm.cos_max_managed_gb,
  };
}
