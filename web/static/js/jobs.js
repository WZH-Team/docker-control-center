// 任务输出面板：打开模态框轮询任务状态与输出，支持取消。
import { api } from "./api.js";
import { t, errorMessage } from "./i18n.js";
import { esc, openModal, toast, stateBadge } from "./ui.js";

// openJobModal: 打开任务输出窗口并轮询直至任务结束。
// onDone(status) 在任务结束后回调（可选）。
export function openJobModal(jobId, { title, onDone } = {}) {
  const body = document.createElement("div");
  body.innerHTML = `
    <div class="mb-3 flex items-center justify-between gap-3">
      <div data-status class="text-sm text-slate-600">${esc(t("jobs.running"))}</div>
      <div class="flex items-center gap-2">
        <button type="button" data-cancel-job class="btn btn-secondary btn-xs">${esc(t("jobs.cancel"))}</button>
        <button type="button" data-close-job class="btn btn-ghost btn-xs">${esc(t("common.close"))}</button>
      </div>
    </div>
    <pre class="code-block h-80 whitespace-pre-wrap break-all" data-output>${esc(t("common.loading"))}</pre>`;

  const { close } = openModal({
    title: title || t("jobs.title"),
    body,
    size: "max-w-3xl",
    onClose: () => clearInterval(timer)
  });

  const statusEl = body.querySelector("[data-status]");
  const outputEl = body.querySelector("[data-output]");
  body.querySelector("[data-close-job]").addEventListener("click", close);
  body.querySelector("[data-cancel-job]").addEventListener("click", async () => {
    try {
      await api.post(`/api/jobs/${encodeURIComponent(jobId)}/cancel`);
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  });

  let timer = null;
  let finished = false;

  const statusLabel = (info) => {
    const map = {
      running: t("jobs.running"),
      queued: t("jobs.queued"),
      success: t("jobs.success"),
      failed: t("jobs.failed"),
      canceled: t("jobs.canceled")
    };
    return map[info.status] || info.status;
  };

  const poll = async () => {
    try {
      const info = await api.get(`/api/jobs/${encodeURIComponent(jobId)}`);
      statusEl.innerHTML = `${stateBadge(
        info.status === "success" ? "running" : info.status === "failed" || info.status === "canceled" ? "exited" : "restarting"
      )} <span class="ml-2">${esc(statusLabel(info))}</span>`;
      outputEl.textContent = info.output || "";
      outputEl.scrollTop = outputEl.scrollHeight;

      if (info.status !== "running" && info.status !== "queued") {
        finished = true;
        clearInterval(timer);
        if (info.status === "failed") {
          outputEl.textContent += `\n\n[${t("jobs.failed")}] ${info.error || ""}`;
          toast(errorMessage({ message: info.error }), "error");
        } else if (info.status === "success") {
          toast(t("common.success"), "success");
        }
        body.querySelector("[data-cancel-job]").disabled = true;
        if (onDone) onDone(info.status, info);
      }
    } catch (err) {
      // 任务可能已过期：停止轮询并提示。
      clearInterval(timer);
      if (!finished) {
        statusEl.textContent = errorMessage(err);
      }
    }
  };

  poll();
  timer = setInterval(poll, 1000);
  return { close };
}
