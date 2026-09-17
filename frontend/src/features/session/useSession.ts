import { useQuery } from "@tanstack/react-query";
import { requestJson, type Session } from "../../shared/api/client";
export function useSession() {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => requestJson<Session>("/api/v1/me"),
    retry: false,
  });
}
