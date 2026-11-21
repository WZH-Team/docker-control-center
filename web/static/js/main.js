// DCC 前端入口：会话引导、路由与主布局。
import { api, setCsrfToken } from "./api.js";
import { t, setLang, getLang, errorMessage } from "./i18n.js";
import { esc, toast, openModal, confirmDialog } from "./ui.js";
import { openJobModal } from "./jobs.js";
import { renderLogin } from "./views/login.js";
import { renderContainers } from "./views/containers.js";
import { renderImages } from "./views/images.js";
import { renderVolumes } from "./views/volumes.js";
import { renderCompose } from "./views/compose.js";
import { renderLogs } from "./views/logs.js";
import { renderSettings } from "./views/settings.js";

const routes = {
  "/containers": { render: renderContainers, titleKey: "nav.containers" },
  "/images": { render: renderImages, titleKey: "nav.images" },
  "/volumes": { render: renderVolumes, titleKey: "nav.volumes" },
  "/compose": { render: renderCompose, titleKey: "nav.compose" },
  "/logs": { render: renderLogs, titleKey: "nav.logs" },
  "/settings": { render: renderSettings, titleKey: "nav.settings" }
};

const appCtx = {
  session: null,
  status: null,
  navigate,
  refreshStatus: loadStatus,
  openJobModal
};

// ---------- 启动 ----------

async function boot() {
  setLang(getLang());
  try {
    const data = await api.get("/api/session");
    if (data && data.authenticated) {
      setCsrfToken(data.csrfToken);
      appCtx.session = data;
      renderAppShell();
      navigate(normalizePath(location.pathname), true);
    } else {
      renderLoginPage();
    }
  } catch (_e) {
    renderLoginPage();
  }
}

function normalizePath(path) {
  if (!path || path === "/" || !routes[path]) return "/containers";
  return path;
}

// ---------- 登录页 ----------

function renderLoginPage() {
  const root = document.getElementById("app");
  root.innerHTML = `
    <div class="flex min-h-screen items-center justify-center bg-gradient-to-br from-slate-900 via-brand-900 to-cyan-900 p-4">
      <div class="w-full max-w-md">
        <div class="mb-8 text-center">
          <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-white/10 ring-1 ring-white/20">
            <img src="/favicon.svg" alt="DCC" class="h-11 w-11" />
          </div>
          <h1 class="text-2xl font-bold tracking-tight text-white">${esc(t("app.name"))}</h1>
          <p class="mt-1.5 text-sm text-brand-100/80">${esc(t("app.subtitle"))}</p>
        </div>
        <div class="card p-8 shadow-pop" id="login-card"></div>
        <div class="mt-6 flex items-center justify-center gap-2 text-xs text-brand-100/60">
          <span>DCC · Docker Control Center</span>
          <span>·</span>
          <button type="button" id="login-lang" class="underline-offset-2 hover:text-white hover:underline">${getLang() === "zh" ? "English" : "中文"}</button>
        </div>
      </div>
    </div>`;

  document.getElementById("login-lang").addEventListener("click", () => {
    setLang(getLang() === "zh" ? "en" : "zh");
    renderLoginPage();
  });

  renderLogin(document.getElementById("login-card"), (data) => {
    setCsrfToken(data.csrfToken);
    appCtx.session = data;
    toast(t("login.welcome", { name: data.username }), "success");
    renderAppShell();
    navigate("/containers", true);
  });
}

// ---------- 主布局 ----------

function renderAppShell() {
  const root = document.getElementById("app");
  root.innerHTML = `
    <div class="flex h-dvh overflow-hidden">
      <aside id="sidebar" class="fixed inset-y-0 left-0 z-40 flex h-full w-60 -translate-x-full flex-col bg-gradient-to-b from-brand-900 via-brand-800 to-slate-900 transition-transform duration-200 lg:static lg:translate-x-0">
        <div class="flex h-16 shrink-0 items-center gap-3 px-5">
          <img src="/favicon.svg" alt="DCC" class="h-9 w-9 rounded-lg ring-1 ring-white/20" />
          <div>
            <div class="text-sm font-bold leading-tight text-white">${esc(t("app.short"))}</div>
            <div class="text-[11px] leading-tight text-brand-100/70">Docker Control Center</div>
          </div>
        </div>
        <nav class="flex-1 space-y-1 overflow-y-auto px-3 py-2" id="nav"></nav>
        <div class="shrink-0 border-t border-white/10 px-5 py-4 text-[11px]" id="sidebar-foot"></div>
      </aside>
      <div id="sidebar-overlay" class="fixed inset-0 z-30 hidden bg-slate-900/40 lg:hidden"></div>

      <div class="flex h-full min-w-0 flex-1 flex-col">
        <header class="flex h-16 shrink-0 items-center gap-2 border-b border-slate-200 bg-white px-3 sm:gap-3 sm:px-6">
          <button type="button" id="menu-btn" class="rounded-lg p-2 text-slate-500 hover:bg-slate-100 lg:hidden">
            <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" d="M4 6h16M4 12h16M4 18h16"/></svg>
          </button>
          <h1 class="min-w-0 flex-1 truncate text-lg font-semibold text-slate-800" id="page-title"></h1>
          <div class="flex shrink-0 items-center gap-2">
            <select id="lang-select" class="input w-auto max-w-[4.5rem] py-1.5 text-xs">
              <option value="zh">中文</option>
              <option value="en">English</option>
            </select>
            <div class="hidden items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-1.5 sm:flex">
              <span class="flex h-6 w-6 items-center justify-center rounded-full bg-brand-600 text-xs font-semibold text-white">${esc((appCtx.session?.username || "U").slice(0, 1).toUpperCase())}</span>
              <span class="text-sm text-slate-600">${esc(appCtx.session?.username || "")}</span>
            </div>
            <button type="button" id="logout-btn" class="btn btn-secondary btn-xs">${esc(t("login.logout"))}</button>
          </div>
        </header>

        <main class="flex-1 overflow-y-auto p-4 sm:p-6" id="view"></main>
      </div>
    </div>`;

  renderNav();
  updateSidebarFoot();

  const langSelect = document.getElementById("lang-select");
  langSelect.value = getLang();
  langSelect.addEventListener("change", () => {
    setLang(langSelect.value);
    renderAppShell();
    navigate(location.pathname, true);
  });

  document.getElementById("logout-btn").addEventListener("click", async () => {
    const { ok } = await confirmDialog({
      title: t("login.logout"),
      message: t("login.logout") + "?",
      confirmText: t("login.logout")
    });
    if (!ok) return;
    try {
      await api.post("/api/logout");
    } catch (_e) { /* 会话可能已失效 */ }
    location.href = "/";
  });

  // 移动端侧边栏。
  const sidebar = document.getElementById("sidebar");
  const overlay = document.getElementById("sidebar-overlay");
  const toggle = (show) => {
    sidebar.classList.toggle("-translate-x-full", !show);
    overlay.classList.toggle("hidden", !show);
  };
  document.getElementById("menu-btn").addEventListener("click", () => toggle(true));
  overlay.addEventListener("click", () => toggle(false));

  loadStatus();
}

function renderNav() {
  const nav = document.getElementById("nav");
  const icons = {
    // 容器：等距立方体（Heroicons cube，与整套图标同源）
    "/containers": '<path stroke-linecap="round" stroke-linejoin="round" d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9"/>',
    "/images": '<path stroke-linecap="round" stroke-linejoin="round" d="M2.25 15.75l5.159-5.159a2.25 2.25 0 013.182 0l5.159 5.159m-1.5-1.5l1.409-1.409a2.25 2.25 0 013.182 0l2.909 2.909m-18 3.75h16.5a1.5 1.5 0 001.5-1.5V6a1.5 1.5 0 00-1.5-1.5H3.75A1.5 1.5 0 002.25 6v12a1.5 1.5 0 001.5 1.5zm10.5-11.25h.008v.008h-.008V8.25zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"/>',
    "/volumes": '<path stroke-linecap="round" stroke-linejoin="round" d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/><path stroke-linecap="round" stroke-linejoin="round" d="M15.91 11.672a.375.375 0 010 .656l-5.603 3.113a.375.375 0 01-.557-.328V8.887c0-.286.307-.466.557-.327l5.603 3.112z"/>',
    "/compose": '<path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z"/>',
    "/logs": '<path stroke-linecap="round" stroke-linejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 00-3.375-3.375h-1.5A1.125 1.125 0 0113.5 7.125v-1.5a3.375 3.375 0 00-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 00-9-9z"/>',
    "/settings": '<path stroke-linecap="round" stroke-linejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z"/><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/>'
  };

  nav.innerHTML = Object.entries(routes)
    .map(([path, cfg]) => `
      <a href="${path}" data-link data-path="${path}" class="nav-link">
        <svg class="h-5 w-5 shrink-0" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6">${icons[path]}</svg>
        <span>${esc(t(cfg.titleKey))}</span>
      </a>`)
    .join("");

  nav.querySelectorAll("[data-link]").forEach((a) => {
    a.addEventListener("click", (e) => {
      e.preventDefault();
      navigate(a.getAttribute("data-path"));
      document.getElementById("sidebar").classList.add("-translate-x-full");
      document.getElementById("sidebar-overlay").classList.add("hidden");
    });
  });
  highlightNav();
}

function highlightNav() {
  const path = normalizePath(location.pathname);
  document.querySelectorAll("#nav [data-link]").forEach((a) => {
    a.classList.toggle("nav-link-active", a.getAttribute("data-path") === path);
  });
}

// updateSidebarFoot 侧边栏底部：Docker 连接状态 + 版本号。
function updateSidebarFoot() {
  const el = document.getElementById("sidebar-foot");
  if (!el) return;
  const ok = Boolean(appCtx.status?.docker?.reachable);
  const version = appCtx.status?.app?.version || "1.0.0";
  el.innerHTML = `
    <div class="flex items-center gap-2">
      <span class="h-2 w-2 rounded-full ${ok ? "bg-emerald-400" : "bg-amber-400"}"></span>
      <span class="text-brand-100/70">${esc(ok ? t("docker.connected") : t("docker.disconnected"))}</span>
    </div>
    <div class="mt-1.5 text-brand-100/40">DCC v${esc(version)}</div>`;
}

// currentCleanup 当前视图的清理函数（如自动刷新定时器）。
let currentCleanup = null;

// ---------- 路由 ----------

async function navigate(path, replace = false) {
  const target = normalizePath(path);
  if (replace) {
    history.replaceState({}, "", target);
  } else if (location.pathname !== target) {
    history.pushState({}, "", target);
  }
  highlightNav();

  const cfg = routes[target];
  const titleEl = document.getElementById("page-title");
  if (titleEl) titleEl.textContent = t(cfg.titleKey);

  const view = document.getElementById("view");
  if (!view) return;
  if (typeof currentCleanup === "function") {
    try { currentCleanup(); } catch (_e) { /* 视图清理失败不影响切换 */ }
    currentCleanup = null;
  }
  try {
    const cleanup = await cfg.render(view, appCtx);
    if (typeof cleanup === "function") currentCleanup = cleanup;
  } catch (err) {
    if (err && err.code === "err.unauthorized") {
      location.href = "/";
      return;
    }
    view.innerHTML = `<div class="card p-8 text-center text-sm text-rose-600">${esc(errorMessage(err))}</div>`;
  }
}

window.addEventListener("popstate", () => navigate(location.pathname));

// 全局拦截站内链接。
document.addEventListener("click", (e) => {
  const a = e.target.closest("a[data-link]");
  if (!a) return;
  e.preventDefault();
  navigate(a.getAttribute("href"));
});

// ---------- Docker 状态 ----------

async function loadStatus() {
  try {
    appCtx.status = await api.get("/api/status");
  } catch (_e) {
    appCtx.status = null;
  }
  updateSidebarFoot();
  renderDockerBanner();
  return appCtx.status;
}

function renderDockerBanner() {
  const view = document.getElementById("view");
  if (!view) return;
  const old = document.getElementById("docker-banner");
  if (old) old.remove();

  const status = appCtx.status?.docker;
  if (!status || status.reachable) return;

  const banner = document.createElement("div");
  banner.id = "docker-banner";
  banner.className = "mb-4 rounded-xl border border-amber-200 bg-amber-50 p-4";
  banner.innerHTML = `
    <div class="flex flex-wrap items-center gap-4">
      <div class="flex h-10 w-10 items-center justify-center rounded-full bg-amber-100 text-amber-600">
        <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z"/></svg>
      </div>
      <div class="flex-1">
        <div class="font-semibold text-amber-800">${esc(t("docker.notInstalled"))}</div>
        <div class="mt-0.5 text-sm text-amber-700">${esc(t("docker.notInstalledMsg"))}</div>
        ${status.error ? `<div class="mt-1 font-mono text-xs text-amber-600/80">${esc(status.error)}</div>` : ""}
        ${status.binaryFound ? `<div class="mt-1 text-xs text-amber-700">${esc(t("docker.startHint"))}</div>` : ""}
      </div>
      <div class="flex items-center gap-2">
        <button type="button" id="docker-install-btn" class="btn btn-primary">
          ${esc(t("docker.install"))}
        </button>
      </div>
    </div>`;
  view.prepend(banner);

  document.getElementById("docker-install-btn").addEventListener("click", async () => {
    const isLinux = (appCtx.status?.app?.platform || "").startsWith("linux");
    if (!isLinux) {
      toast(t("docker.installLinuxOnly"), "error");
      return;
    }
    const { ok } = await confirmDialog({
      title: t("docker.install"),
      message: t("docker.notInstalledMsg"),
      detail: "curl -fsSL https://get.docker.com | bash",
      confirmText: t("docker.install")
    });
    if (!ok) return;
    try {
      const data = await api.post("/api/docker/install");
      openJobModal(data.jobId, {
        title: t("docker.installing"),
        onDone: (status) => {
          if (status === "success") {
            toast(t("docker.installDone"), "success");
            loadStatus();
          } else {
            toast(t("docker.installFailed"), "error");
          }
        }
      });
    } catch (err) {
      toast(errorMessage(err), "error");
    }
  });
}

boot();
