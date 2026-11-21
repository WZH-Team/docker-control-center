// UI 基础库：DOM 辅助、模态框、确认框、Toast、格式化。
import { t } from "./i18n.js";

// esc: HTML 转义，防止注入。
export function esc(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

// fmtBytes: 人类可读的大小。
export function fmtBytes(bytes) {
  const n = Number(bytes || 0);
  if (n < 1024) return `${n} B`;
  const units = ["KB", "MB", "GB", "TB", "PB"];
  let value = n / 1024;
  let i = 0;
  while (value >= 1024 && i < units.length - 1) {
    value /= 1024;
    i++;
  }
  return `${value.toFixed(value >= 100 ? 0 : 1)} ${units[i]}`;
}

// fmtTime: ISO 时间 → 本地可读时间。
export function fmtTime(iso) {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return String(iso);
  const pad = (x) => String(x).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

// fmtDuration: 秒数 → 可读时长。
export function fmtDuration(sec) {
  const s = Math.max(0, Math.floor(sec));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m ${s % 60}s`;
}

export function shortId(id) {
  const s = String(id || "").replace(/^sha256:/, "");
  return s.length > 12 ? s.slice(0, 12) : s;
}

// toast: 右上角轻提示。
export function toast(message, type = "info", timeout = 3600) {
  const root = document.getElementById("toast-root");
  const colors = {
    success: "border-emerald-200 bg-emerald-50 text-emerald-800",
    error: "border-rose-200 bg-rose-50 text-rose-800",
    info: "border-brand-200 bg-brand-50 text-brand-800"
  };
  const icons = { success: "✓", error: "✕", info: "ℹ" };
  const el = document.createElement("div");
  el.className = `pointer-events-auto flex items-start gap-2.5 rounded-xl border px-4 py-3 text-sm shadow-pop ${colors[type] || colors.info}`;
  el.innerHTML = `<span class="mt-0.5 font-semibold">${icons[type] || icons.info}</span><span class="flex-1">${esc(message)}</span>`;
  root.appendChild(el);
  setTimeout(() => {
    el.style.transition = "opacity .35s, transform .35s";
    el.style.opacity = "0";
    el.style.transform = "translateY(-6px)";
    setTimeout(() => el.remove(), 380);
  }, timeout);
}

// openModal: 通用模态框，返回 { close, panel }。
export function openModal({ title, body, footer, size = "max-w-lg", onClose }) {
  const root = document.getElementById("modal-root");
  const overlay = document.createElement("div");
  overlay.className = "modal-overlay";
  overlay.innerHTML = `
    <div class="modal-panel ${size} max-h-[88vh] overflow-hidden">
      <div class="flex items-center justify-between border-b border-slate-100 px-6 py-4">
        <h3 class="text-lg font-semibold text-slate-800">${esc(title)}</h3>
        <button type="button" data-close class="rounded-lg p-1.5 text-slate-400 transition-colors hover:bg-slate-100 hover:text-slate-600">
          <svg class="h-5 w-5" viewBox="0 0 20 20" fill="currentColor"><path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z"/></svg>
        </button>
      </div>
      <div class="max-h-[64vh] overflow-auto px-6 py-5" data-body></div>
      <div class="flex items-center justify-end gap-2 border-t border-slate-100 px-6 py-4" data-footer></div>
    </div>`;

  const bodyEl = overlay.querySelector("[data-body]");
  if (typeof body === "string") {
    bodyEl.innerHTML = body;
  } else if (body instanceof Node) {
    bodyEl.appendChild(body);
  }
  const footerEl = overlay.querySelector("[data-footer]");
  if (footer instanceof Node) {
    footerEl.appendChild(footer);
  } else if (typeof footer === "string") {
    footerEl.innerHTML = footer;
  } else {
    footerEl.remove();
  }

  const close = () => {
    overlay.remove();
    if (onClose) onClose();
  };
  overlay.querySelector("[data-close]").addEventListener("click", close);
  overlay.addEventListener("click", (e) => {
    if (e.target === overlay) close();
  });

  root.appendChild(overlay);
  return { close, panel: overlay.querySelector(".modal-panel"), overlay };
}

// confirmDialog: 确认对话框（支持危险样式与复选框），返回 Promise<{ok, checked}>。
export function confirmDialog({ title, message, detail, confirmText, danger = false, checkboxLabel }) {
  return new Promise((resolve) => {
    const footer = document.createElement("div");
    footer.className = "flex w-full items-center justify-end gap-2";
    footer.innerHTML = `
      <button type="button" data-cancel class="btn btn-secondary">${esc(t("common.cancel"))}</button>
      <button type="button" data-ok class="btn ${danger ? "btn-danger" : "btn-primary"}">${esc(confirmText || t("common.confirm"))}</button>`;

    const body = document.createElement("div");
    body.innerHTML = `
      <p class="text-sm leading-relaxed text-slate-600">${esc(message)}</p>
      ${detail ? `<div class="mt-3 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 font-mono text-xs text-slate-600">${esc(detail)}</div>` : ""}
      ${checkboxLabel ? `
        <label class="mt-4 flex cursor-pointer items-start gap-2.5 text-sm text-slate-700">
          <input type="checkbox" data-check class="mt-0.5 h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-500" />
          <span>${esc(checkboxLabel)}</span>
        </label>` : ""}`;

    const { close } = openModal({ title, body, footer, size: "max-w-md" });
    const done = (ok) => {
      const checked = checkboxLabel ? Boolean(body.querySelector("[data-check]").checked) : false;
      close();
      resolve({ ok, checked });
    };
    footer.querySelector("[data-cancel]").addEventListener("click", () => done(false));
    footer.querySelector("[data-ok]").addEventListener("click", () => done(true));
  });
}

// promptDialog: 单输入对话框，返回 Promise<string|null>。
export function promptDialog({ title, message, placeholder, inputType = "text", confirmText, validate }) {
  return new Promise((resolve) => {
    const body = document.createElement("div");
    body.innerHTML = `
      ${message ? `<p class="mb-3 text-sm text-slate-600">${esc(message)}</p>` : ""}
      <input type="${esc(inputType)}" data-input class="input" placeholder="${esc(placeholder || "")}" autocomplete="off" />
      <p class="mt-2 hidden text-xs text-rose-600" data-error></p>`;

    const footer = document.createElement("div");
    footer.className = "flex w-full items-center justify-end gap-2";
    footer.innerHTML = `
      <button type="button" data-cancel class="btn btn-secondary">${esc(t("common.cancel"))}</button>
      <button type="button" data-ok class="btn btn-primary">${esc(confirmText || t("common.confirm"))}</button>`;

    const { close } = openModal({ title, body, footer, size: "max-w-md" });
    const input = body.querySelector("[data-input]");
    const errEl = body.querySelector("[data-error]");
    setTimeout(() => input.focus(), 50);

    const done = (value) => {
      close();
      resolve(value);
    };
    footer.querySelector("[data-cancel]").addEventListener("click", () => done(null));
    const submit = () => {
      const value = input.value.trim();
      if (validate) {
        const err = validate(value);
        if (err) {
          errEl.textContent = err;
          errEl.classList.remove("hidden");
          return;
        }
      }
      done(value);
    };
    footer.querySelector("[data-ok]").addEventListener("click", submit);
    input.addEventListener("keydown", (e) => {
      if (e.key === "Enter") submit();
    });
  });
}

// stateBadge: 容器/任务状态徽章。
export function stateBadge(state) {
  const map = {
    running: ["bg-emerald-100 text-emerald-700", "bg-emerald-500", t("common.running")],
    exited: ["bg-slate-100 text-slate-600", "bg-slate-400", t("common.stopped")],
    created: ["bg-cyan-100 text-cyan-700", "bg-cyan-500", "created"],
    paused: ["bg-amber-100 text-amber-700", "bg-amber-500", "paused"],
    restarting: ["bg-amber-100 text-amber-700", "bg-amber-500", "restarting"],
    removing: ["bg-rose-100 text-rose-700", "bg-rose-500", "removing"],
    dead: ["bg-rose-100 text-rose-700", "bg-rose-500", "dead"]
  };
  const [cls, dot, label] = map[state] || ["bg-slate-100 text-slate-600", "bg-slate-400", state || t("common.unknown")];
  return `<span class="badge ${cls}"><span class="badge-dot ${dot}"></span>${esc(label)}</span>`;
}

// spinner: 加载指示器。
export function spinner(label) {
  return `
    <div class="flex items-center justify-center gap-3 py-16 text-slate-500">
      <svg class="h-5 w-5 animate-spin text-brand-600" viewBox="0 0 24 24" fill="none">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"></path>
      </svg>
      <span class="text-sm">${esc(label || t("common.loading"))}</span>
    </div>`;
}

// emptyState: 空状态占位。
export function emptyState(text) {
  return `
    <div class="flex flex-col items-center justify-center gap-2 py-16 text-slate-400">
      <svg class="h-10 w-10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5m8.25 3v6.75m0 0l-3-3m3 3l3-3M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z"/>
      </svg>
      <p class="text-sm">${esc(text || t("common.empty"))}</p>
    </div>`;
}

// copyText: 复制到剪贴板并提示。
// 优先使用异步剪贴板 API（仅安全上下文可用）；
// 在 HTTP 等非安全上下文回退到 execCommand，保证局域网访问也能复制。
export async function copyText(text) {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      toast(t("common.copied"), "success", 1800);
      return;
    } catch (_e) {
      // 权限被拒或文档失焦时回退到传统方式
    }
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.position = "fixed";
  ta.style.top = "-9999px";
  document.body.appendChild(ta);
  ta.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch (_e) {
    ok = false;
  }
  ta.remove();
  if (ok) {
    toast(t("common.copied"), "success", 1800);
  } else {
    toast(t("common.error"), "error");
  }
}

// kv: 详情键值对渲染。
export function kv(label, value) {
  return `
    <div>
      <dt class="text-xs font-medium uppercase tracking-wider text-slate-400">${esc(label)}</dt>
      <dd class="mt-1 break-all text-sm text-slate-700">${value ?? "-"}</dd>
    </div>`;
}
