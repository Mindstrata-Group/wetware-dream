import { apiFetch } from "./api";
import type { paths } from "./generated/openapi";

type JsonContent<T> = T extends { content: { "application/json": infer Body } } ? Body : never;
type ResponseBody<Path extends keyof paths, Method extends keyof paths[Path], Status extends number = 200> =
  paths[Path][Method] extends { responses: Record<Status, infer Response> }
    ? JsonContent<Response>
    : never;
type RequestBody<Path extends keyof paths, Method extends keyof paths[Path]> =
  paths[Path][Method] extends { requestBody: infer Body }
    ? JsonContent<Body>
    : never;

export type AuthMeResponse = ResponseBody<"/api/auth/me", "get">;
export type ProfileResponse = ResponseBody<"/api/profile", "get">;
export type AccessStatusResponse = ResponseBody<"/api/access/status", "get">;
export type ChatStartResponse = ResponseBody<"/api/chat/start", "post">;
export type ChatSendRequest = RequestBody<"/api/chat/send", "post">;
export type ChatSendResponse = ResponseBody<"/api/chat/send", "post">;
export type YooKassaPaymentCreateRequest = RequestBody<"/api/payments/yookassa/create", "post">;
export type YooKassaPaymentCreateResponse = ResponseBody<"/api/payments/yookassa/create", "post">;
export type AdminPaymentsResponse = ResponseBody<"/api/admin/payments", "get">;

export function getAuthMe() {
  return apiFetch<AuthMeResponse>("/api/auth/me");
}

export function getProfile() {
  return apiFetch<ProfileResponse>("/api/profile");
}

export function getAccessStatus() {
  return apiFetch<AccessStatusResponse>("/api/access/status");
}

export function startChat() {
  return apiFetch<ChatStartResponse>("/api/chat/start", { method: "POST" });
}

export function sendChatMessage(body: ChatSendRequest) {
  return apiFetch<ChatSendResponse>("/api/chat/send", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function createYooKassaPayment(body: YooKassaPaymentCreateRequest) {
  return apiFetch<YooKassaPaymentCreateResponse>("/api/payments/yookassa/create", {
    method: "POST",
    body: JSON.stringify(body),
  });
}

export function getAdminPayments() {
  return apiFetch<AdminPaymentsResponse>("/api/admin/payments");
}
