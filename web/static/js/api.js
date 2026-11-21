// API 封装：统一响应解析、CSRF 携带与错误对象。
import { t } from "./i18n.js";

let csrfToken = null;

export function setCsrfToken(token) {
  csrfToken = token || null;
}

export class ApiError extends Error {
  constructor(code, message, status) {
    super(message || code);
    this.code = code;
    this.status = status;
  }
}

async function request(method, path, body) {
  const opts = {
    method,
    credentials: "same-origin",
    headers: { Accept: "application/json" }
  };
  if (body !== undefined) {
    opts.headers["Content-Type"] = "application/json";
    opts.body = JSON.stringify(body);
  }
  if (method !== "GET" && csrfToken) {
    opts.headers["X-CSRF-Token"] = csrfToken;
  }

  let res;
  try {
    res = await fetch(path, opts);
  } catch (_e) {
    throw new ApiError("err.network", t("err.network"), 0);
  }

  let payload = null;
  try {
    payload = await res.json();
  } catch (_e) {
    // 非 JSON 响应（如反代错误页）
  }

  if (!payload || payload.ok !== true) {
    const code = (payload && payload.error && payload.error.code) || "err.internal";
    const message = (payload && payload.error && payload.error.message) || `HTTP ${res.status}`;
    throw new ApiError(code, message, res.status);
  }
  return payload.data;
}

export const api = {
  get: (path) => request("GET", path),
  post: (path, body) => request("POST", path, body === undefined ? {} : body),
  del: (path, body) => request("DELETE", path, body)
};
