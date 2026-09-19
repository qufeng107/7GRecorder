import {
  type ProfileForm,
  type RecordingProfile,
  type ProfileSortKey,
} from "../../shared/console/types";
import { includesSearch } from "../../shared/console/format";

export const emptyProfileForm: ProfileForm = {
  owner_user_id: "",
  name: "",
  room_id: "",
  streamer_name: "",
  streamer_uid: "",
  timezone: "Asia/Shanghai",
  enabled: true,
  public_enabled: false,
  public_slug: "",
  auto_record: true,
  quality: "original",
  record_danmaku: false,
  segment_duration_sec: 1800,
  finalize_grace_period_sec: 300,
};

export function profilePayload(form: ProfileForm) {
  return {
    owner_user_id: form.owner_user_id ? Number(form.owner_user_id) : undefined,
    name: form.name,
    room_id: form.room_id,
    streamer_name: form.streamer_name,
    streamer_uid: form.streamer_uid,
    timezone: form.timezone,
    enabled: form.enabled,
    public_enabled: form.public_enabled,
    public_slug: form.public_slug,
    recording_settings: {
      auto_record: form.auto_record,
      quality: form.quality,
      record_danmaku: false,
      segment_duration_sec: form.segment_duration_sec,
      finalize_grace_period_sec: form.finalize_grace_period_sec,
    },
  };
}

export function filterProfiles(
  items: RecordingProfile[],
  search: string,
  sort: ProfileSortKey,
): RecordingProfile[] {
  const filtered = items.filter((profile) => {
    if (!search.trim()) {
      return true;
    }
    return (
      includesSearch(profile.name, search) ||
      includesSearch(profile.room_id, search) ||
      includesSearch(profile.streamer_name, search) ||
      includesSearch(profile.owner_username, search)
    );
  });
  return [...filtered].sort((left, right) => {
    if (sort === "room_asc") {
      return left.room_id.localeCompare(right.room_id, "zh-CN", {
        numeric: true,
      });
    }
    return left.name.localeCompare(right.name, "zh-CN");
  });
}
