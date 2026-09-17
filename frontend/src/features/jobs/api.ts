import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { requestJson, type ListResponse } from "../../shared/api/client";
import type { Job, RetryRequest } from "../../shared/api/contracts.generated";
export function useJobs() {
  return useQuery({
    queryKey: ["jobs"],
    queryFn: () => requestJson<ListResponse<Job>>("/api/v1/jobs?limit=100"),
    refetchInterval: 5000,
    retry: false,
  });
}
export function useJobAction() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({
      id,
      action,
      confirm = false,
    }: {
      id: number;
      action: "retry" | "cancel";
      confirm?: boolean;
    }) => {
      const body: RetryRequest = { confirm_ambiguous_bilibili: confirm };
      return requestJson<Job>(`/api/v1/jobs/${id}/actions/${action}`, {
        method: "POST",
        body: JSON.stringify(action === "retry" ? body : {}),
      });
    },
    onSuccess: () => client.invalidateQueries({ queryKey: ["jobs"] }),
  });
}
