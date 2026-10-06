import { request } from "./client";
import type { JSONContent } from "@tiptap/react";

export interface BroadcastAttachment {
  id: string;
  filename: string;
  media_type: "photo" | "document";
  content_type: string;
  size: number;
  position: number;
}
export interface BroadcastRecipient {
  user_id: string;
  username: string;
  status: string;
  reason: string;
  parts: number;
}
export interface Broadcast {
  id: string;
  author_id: string;
  document: JSONContent;
  body: string;
  audience_mode: "all" | "selected";
  status: "draft" | "sending" | "completed" | "cancelled";
  version: number;
  created_at: string;
  updated_at: string;
  sent_at: string | null;
  completed_at: string | null;
  attachments: BroadcastAttachment[];
  recipients: BroadcastRecipient[];
  counts: Record<string, number>;
}
export interface BroadcastAudience {
  recipients: BroadcastRecipient[];
  eligible: number;
  total: number;
}
export interface BroadcastPreview {
  broadcast: Broadcast;
  body: string;
  eligible: number;
}
export interface BroadcastInput {
  document: JSONContent;
  audience_mode: "all" | "selected";
  user_ids: string[];
  attachment_ids: string[];
  version: number;
}
const root = "/api/broadcasts";
const json = (method: string, body: unknown) => ({
  method,
  body: JSON.stringify(body),
});
export const broadcastAPI = {
  list: (offset = 0) => request<Broadcast[]>(`${root}?offset=${offset}`),
  create: () => request<{ id: string }>(root, { method: "POST" }),
  get: (id: string) => request<Broadcast>(`${root}/${id}`),
  save: (id: string, input: BroadcastInput) =>
    request<Broadcast>(`${root}/${id}`, json("PUT", input)),
  audience: (mode: string, usernames: string) =>
    request<BroadcastAudience>(
      `${root}/audience`,
      json("POST", { mode, usernames }),
    ),
  preview: (id: string) =>
    request<BroadcastPreview>(`${root}/${id}/preview`, { method: "POST" }),
  send: (id: string, version: number, key: string) =>
    request<Broadcast>(`${root}/${id}/send`, json("POST", { version, key })),
  stop: (id: string) =>
    request<Broadcast>(`${root}/${id}/stop`, { method: "POST" }),
  delete: (id: string) => request<void>(`${root}/${id}`, { method: "DELETE" }),
  upload: (
    id: string,
    version: number,
    file: File,
    kind: "photo" | "document",
  ) => {
    const body = new FormData();
    body.append("file", file);
    return request<Broadcast>(
      `${root}/${id}/attachments?version=${version}&type=${kind}`,
      { method: "POST", body },
    );
  },
  removeAttachment: (id: string, attachment: string, version: number) =>
    request<Broadcast>(
      `${root}/${id}/attachments/${attachment}?version=${version}`,
      { method: "DELETE" },
    ),
};
export const attachmentURL = (id: string, attachment: string) =>
  `${root}/${id}/attachments/${attachment}`;
