import { type SongSettingsForm } from "../../shared/console/types";

export const emptySongSettingsForm: SongSettingsForm = {
  enabled: false,
  credential_id: "",
  region: "",
  container_id: "",
  destination_cos_storage_profile_id: "",
  songs_prefix: "songs",
  boundary_padding_ms: 0,
  algorithm_version: "v1",
};

export function formatSongOffset(milliseconds: number): string {
  const totalSeconds = Math.max(0, Math.floor(milliseconds / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
    : `${minutes}:${String(seconds).padStart(2, "0")}`;
}
