import { requestJson } from "../../shared/api/client";
import type { PublicInfo } from "./model";

export function getPublicInfo(signal?: AbortSignal) {
  return requestJson<PublicInfo>("/api/public-info", signal);
}
