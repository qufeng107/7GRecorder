import { PageStatus } from "../../shared/ui/PageStatus";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import {
  type RecordingSortKey,
  type MeResponse,
  type UploadSourceListResponse,
  type RecordingListResponse,
  type JobListResponse,
  type RecordingItem,
  type ReconcileResult,
  type UploadSourceDiscoverResult,
  type UploadSourceRegroupResult,
  type UploadSourceRepairResult,
  type UploadSourceItem,
  type UploadSourceEditCut,
  type COSDownloadURLResponse,
} from "../../shared/console/types";
import { requestJson } from "../../shared/api/client";
import {
  hasManagerPermission,
  UPLOAD_SOURCE_MERGE_GAP_SECONDS,
  chinaDateFromTimestamp,
} from "../../shared/console/format";
import { uiCopy } from "../../shared/console/copy";
import {
  uploadSourceToRecordingItem,
  recordingToActiveRecordingItem,
  filterRecordings,
  latestRecordingChinaDate,
  currentChinaDate,
  parseEditCutDraft,
} from "./model";
import { RecordingsPanel } from "./views";
import { useLanguage } from "../../app/preferences";

export default function RecordingsPage() {
  const language = useLanguage();
  const queryClient = useQueryClient();
  const [recordingSearch, setRecordingSearch] = useState("");
  const [recordingSort, setRecordingSort] =
    useState<RecordingSortKey>("started_desc");
  const [editDrafts, setEditDrafts] = useState<Record<number, string>>({});
  const meQuery = useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<MeResponse>("/api/v1/me"),
    retry: false,
  });
  const user = meQuery.data?.user;
  const ownPolicy = meQuery.data?.policy;
  const canManageSystemSettings = user?.role === "SUPER_ADMIN";
  const canManageLocalFiles = hasManagerPermission(
    user,
    ownPolicy,
    "can_manage_local_files",
  );
  const canScanLocalFiles = Boolean(canManageSystemSettings);
  const ui = uiCopy[language];
  const recordingsQuery = useQuery({
    queryKey: ["upload-sources"],
    queryFn: () =>
      requestJson<UploadSourceListResponse>(
        `/api/v1/upload-sources?merge_gap_seconds=${UPLOAD_SOURCE_MERGE_GAP_SECONDS}`,
      ),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 15000,
  });
  const rawRecordingsQuery = useQuery({
    queryKey: ["recordings"],
    queryFn: () => requestJson<RecordingListResponse>("/api/v1/recordings"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 5000,
  });
  const jobsQuery = useQuery({
    queryKey: ["jobs"],
    queryFn: () => requestJson<JobListResponse>("/api/v1/jobs?limit=100"),
    enabled: Boolean(meQuery.data?.user),
    retry: false,
    refetchInterval: 5000,
  });
  const recordings = useMemo(() => {
    const uploadSourceItems = (recordingsQuery.data?.items ?? []).map(
      uploadSourceToRecordingItem,
    );
    const representedRecordingIDs = new Set<number>();
    for (const item of uploadSourceItems) {
      for (const segment of item.source_segments ?? []) {
        representedRecordingIDs.add(segment.recording_id);
      }
    }
    const activeItems = (rawRecordingsQuery.data?.items ?? [])
      .filter(
        (recording) =>
          recording.local_storage_status !== "DELETED" &&
          !representedRecordingIDs.has(recording.id),
      )
      .map(recordingToActiveRecordingItem);
    return [...activeItems, ...uploadSourceItems].sort((left, right) => {
      return (
        new Date(right.started_at).getTime() -
        new Date(left.started_at).getTime()
      );
    });
  }, [rawRecordingsQuery.data?.items, recordingsQuery.data?.items]);
  const recordingTotal =
    (recordingsQuery.data?.total ?? 0) +
    recordings.filter((item) => item.is_active_recording).length;
  const jobs = jobsQuery.data?.items ?? [];
  const visibleRecordings = filterRecordings(
    recordings,
    recordingSearch,
    recordingSort,
  );
  const reconcileMutation = useMutation({
    mutationFn: async () => {
      const reconcile = await requestJson<ReconcileResult>(
        "/api/v1/recording-files/reconcile",
        {
          method: "POST",
          body: "{}",
        },
      );
      const discover = await requestJson<UploadSourceDiscoverResult>(
        `/api/v1/upload-sources/actions/discover?merge_gap_seconds=${UPLOAD_SOURCE_MERGE_GAP_SECONDS}`,
        {
          method: "POST",
          body: "{}",
        },
      );
      return { reconcile, discover };
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    },
  });
  const regroupTodayMutation = useMutation({
    mutationFn: async () => {
      const chinaDate =
        latestRecordingChinaDate(recordings) ?? currentChinaDate();
      const profileIds = Array.from(
        new Set(
          recordings
            .filter(
              (recording) =>
                chinaDateFromTimestamp(recording.started_at) === chinaDate,
            )
            .map((recording) => recording.recording_profile_id),
        ),
      );
      const items: UploadSourceRegroupResult[] = [];
      for (const profileId of profileIds) {
        const result = await requestJson<UploadSourceRegroupResult>(
          "/api/v1/upload-sources/actions/regroup",
          {
            method: "POST",
            body: JSON.stringify({
              recording_profile_id: profileId,
              china_date: chinaDate,
              merge_gap_seconds: UPLOAD_SOURCE_MERGE_GAP_SECONDS,
            }),
          },
        );
        items.push(result);
      }
      return { china_date: chinaDate, items };
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
    },
  });
  const repairUploadSourcesMutation = useMutation({
    mutationFn: () =>
      requestJson<UploadSourceRepairResult>(
        "/api/v1/upload-sources/actions/repair",
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
    },
  });
  const protectRecordingMutation = useMutation({
    mutationFn: (request: { id: number; protected: boolean }) =>
      requestJson<RecordingItem>(
        `/api/v1/recordings/${request.id}/actions/${request.protected ? "protect-local" : "unprotect-local"}`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["local-storage"] });
      void queryClient.invalidateQueries({ queryKey: ["cleanup-candidates"] });
    },
  });
  const requireRecordingReviewMutation = useMutation({
    mutationFn: (recordingId: number) =>
      requestJson<RecordingItem>(
        `/api/v1/recordings/${recordingId}/actions/require-upload-review`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["recordings"] });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
  const requireUploadSourceReviewMutation = useMutation({
    mutationFn: (uploadSourceId: number) =>
      requestJson<UploadSourceItem>(
        `/api/v1/upload-sources/${uploadSourceId}/actions/require-review`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
  const approveUploadSourceReviewMutation = useMutation({
    mutationFn: (uploadSourceId: number) =>
      requestJson<UploadSourceItem>(
        `/api/v1/upload-sources/${uploadSourceId}/actions/approve-review`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
  const applyUploadSourceEditMutation = useMutation({
    mutationFn: (request: {
      uploadSourceId: number;
      cuts: UploadSourceEditCut[];
    }) =>
      requestJson<UploadSourceItem>(
        `/api/v1/upload-sources/${request.uploadSourceId}/actions/apply-edit`,
        {
          method: "POST",
          body: JSON.stringify({ cuts: request.cuts }),
        },
      ),
    onSuccess: (_result, request) => {
      setEditDrafts((current) => {
        const next = { ...current };
        delete next[request.uploadSourceId];
        return next;
      });
      void queryClient.invalidateQueries({ queryKey: ["upload-sources"] });
      void queryClient.invalidateQueries({ queryKey: ["jobs"] });
    },
  });
  const cosDownloadUrlMutation = useMutation({
    mutationFn: (request: { uploadSourceId: number; outputId: number }) =>
      requestJson<COSDownloadURLResponse>(
        `/api/v1/upload-sources/${request.uploadSourceId}/outputs/${request.outputId}/actions/download-url`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: (result) => {
      window.location.assign(result.url);
    },
  });
  const cosFileDownloadUrlMutation = useMutation({
    mutationFn: (request: { fileId: number }) =>
      requestJson<COSDownloadURLResponse>(
        `/api/v1/recording-files/${request.fileId}/actions/cos-download-url`,
        {
          method: "POST",
          body: "{}",
        },
      ),
    onSuccess: (result) => {
      window.location.assign(result.url);
    },
  });
  const downloadLocalUploadSourceOutput = (
    uploadSourceId: number,
    outputId: number,
  ) => {
    window.location.assign(
      `/api/v1/upload-sources/${uploadSourceId}/outputs/${outputId}/download`,
    );
  };
  if (!user) return null;
  if (
    recordingsQuery.isError ||
    rawRecordingsQuery.isError ||
    jobsQuery.isError
  )
    return (
      <PageStatus
        retry={() => {
          void recordingsQuery.refetch();
          void rawRecordingsQuery.refetch();
          void jobsQuery.refetch();
        }}
      />
    );
  if (
    recordingsQuery.isLoading ||
    rawRecordingsQuery.isLoading ||
    jobsQuery.isLoading
  )
    return <PageStatus loading />;
  return (
    <div className="feature-page space-y-6">
      {[
        protectRecordingMutation,
        requireRecordingReviewMutation,
        requireUploadSourceReviewMutation,
        approveUploadSourceReviewMutation,
        cosDownloadUrlMutation,
        cosFileDownloadUrlMutation,
      ].some((mutation) => mutation.isError) && (
        <p role="alert" className="console-card p-4 text-red-600">
          {language === "zh"
            ? "未能确认操作结果，请刷新确认状态后重试；剪辑处理中暂不能完成审核。"
            : "Action failed. Refresh the status before retrying; pending edits cannot be approved."}
        </p>
      )}
      <RecordingsPanel
        isLoading={recordingsQuery.isLoading || rawRecordingsQuery.isLoading}
        jobs={jobs}
        labels={ui}
        canManageLocalFiles={canManageLocalFiles}
        canScanLocalFiles={canScanLocalFiles}
        cosDownloadPending={cosDownloadUrlMutation.isPending}
        cosDownloadPendingOutputId={
          cosDownloadUrlMutation.variables?.outputId ?? null
        }
        cosFileDownloadPending={cosFileDownloadUrlMutation.isPending}
        cosFileDownloadPendingFileId={
          cosFileDownloadUrlMutation.variables?.fileId ?? null
        }
        protectPending={protectRecordingMutation.isPending}
        reviewPending={
          requireRecordingReviewMutation.isPending ||
          requireUploadSourceReviewMutation.isPending ||
          approveUploadSourceReviewMutation.isPending ||
          applyUploadSourceEditMutation.isPending
        }
        regroupError={regroupTodayMutation.isError}
        regroupPending={regroupTodayMutation.isPending}
        regroupResult={regroupTodayMutation.data}
        repairError={repairUploadSourcesMutation.isError}
        repairPending={repairUploadSourcesMutation.isPending}
        repairResult={repairUploadSourcesMutation.data}
        reconcileError={reconcileMutation.isError}
        reconcilePending={reconcileMutation.isPending}
        reconcileResult={reconcileMutation.data}
        recordings={visibleRecordings}
        search={recordingSearch}
        sort={recordingSort}
        total={recordingTotal}
        visibleTotal={visibleRecordings.length}
        onReconcile={() => reconcileMutation.mutate()}
        onRegroupToday={() => {
          if (window.confirm(ui.regroupTodayConfirm)) {
            regroupTodayMutation.mutate();
          }
        }}
        onRepairUploadSources={() => {
          if (window.confirm(ui.repairUploadSourcesConfirm)) {
            repairUploadSourcesMutation.mutate();
          }
        }}
        onDownloadOutput={(uploadSourceId, outputId) =>
          cosDownloadUrlMutation.mutate({ uploadSourceId, outputId })
        }
        onDownloadLocalOutput={downloadLocalUploadSourceOutput}
        onDownloadFile={(fileId) =>
          cosFileDownloadUrlMutation.mutate({ fileId })
        }
        onApproveReview={(recording) => {
          if (recording.upload_source_id) {
            approveUploadSourceReviewMutation.mutate(
              recording.upload_source_id,
            );
          }
        }}
        editDrafts={editDrafts}
        editError={applyUploadSourceEditMutation.isError}
        editPending={applyUploadSourceEditMutation.isPending}
        onApplyEdit={(uploadSourceId, draft) => {
          const cuts = parseEditCutDraft(draft);
          if (cuts.length === 0) {
            return;
          }
          applyUploadSourceEditMutation.mutate({ uploadSourceId, cuts });
        }}
        onEditDraftChange={(uploadSourceId, value) =>
          setEditDrafts((current) => ({ ...current, [uploadSourceId]: value }))
        }
        onRequireReview={(recording) => {
          if (recording.upload_source_id) {
            requireUploadSourceReviewMutation.mutate(
              recording.upload_source_id,
            );
            return;
          }
          requireRecordingReviewMutation.mutate(recording.id);
        }}
        onSearchChange={setRecordingSearch}
        onSortChange={setRecordingSort}
        onToggleProtect={(recording) =>
          protectRecordingMutation.mutate({
            id: recording.id,
            protected: !recording.local_protected,
          })
        }
      />
    </div>
  );
}
