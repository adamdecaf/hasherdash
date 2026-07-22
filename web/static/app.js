/* hasherdash frontend — vanilla JS + canvas charts */

(() => {
  "use strict";

  // Fleet table columns. `defaultOn` is the initial set; `locked` columns stay visible.
  const COLUMNS = [
    { id: "ip", label: "IP", sort: "ip", defaultOn: true, locked: true },
    { id: "hostname", label: "Host", sort: "hostname", defaultOn: true },
    { id: "make", label: "Make", sort: "make", defaultOn: true },
    { id: "model", label: "Model", sort: "model", defaultOn: true },
    { id: "firmware", label: "FW", sort: "firmware", defaultOn: true },
    { id: "firmware_version", label: "FW ver", sort: "firmware_version", defaultOn: false },
    { id: "algo", label: "Algo", sort: "algo", defaultOn: false },
    { id: "mac", label: "MAC", sort: "mac", defaultOn: false },
    { id: "serial", label: "Serial", sort: "serial", defaultOn: false },
    { id: "hashrate_th", label: "TH/s", sort: "hashrate_th", num: true, defaultOn: true },
    { id: "expected_th", label: "Exp TH", sort: "expected_th", num: true, defaultOn: false },
    { id: "asic_temp", label: "ASIC °C", sort: "asic_temp_max", num: true, title: "ASIC chip temp min–max", defaultOn: true },
    { id: "vr_temp", label: "VR °C", sort: "vr_temp_max", num: true, title: "VR / board temp min–max", defaultOn: true },
    { id: "avg_temp", label: "Avg °C", sort: "avg_temp_c", num: true, defaultOn: false },
    { id: "wattage", label: "W", sort: "wattage", num: true, defaultOn: true },
    { id: "efficiency", label: "J/TH", sort: "efficiency", num: true, defaultOn: true },
    { id: "total_chips", label: "Chips", sort: "total_chips", num: true, defaultOn: true },
    { id: "expected_chips", label: "Exp chips", sort: "expected_chips", num: true, defaultOn: false },
    { id: "boards", label: "Boards", sort: "boards", num: true, defaultOn: false },
    { id: "fans", label: "Fans", sort: "fans", num: true, defaultOn: false },
    { id: "uptime", label: "Uptime", sort: "uptime_sec", defaultOn: false },
    { id: "pool_user", label: "Pool user", sort: "pool_user", defaultOn: false },
    { id: "pool_host", label: "Pool host", sort: "pool_host", defaultOn: false },
    { id: "is_mining", label: "Mining", sort: "is_mining", defaultOn: true },
    { id: "status", label: "Status", sort: "error", defaultOn: true },
  ];
  const COLUMN_BY_ID = Object.fromEntries(COLUMNS.map((c) => [c.id, c]));
  const COLUMNS_LS_KEY = "tableColumns";
  const FILTERS_WIDTH_LS_KEY = "filtersWidth";
  const FILTERS_WIDTH_DEFAULT = 280;
  const FILTERS_WIDTH_MIN = 180;
  const FILTERS_WIDTH_MAX = 640;
  const FILTERS_RESIZER_PX = 10;
  const FILTERS_SECTIONS_LS_KEY = "filterSectionsCollapsed";
  // Right column needs room for the chart/table; keep at least this much.
  const FILTERS_RIGHT_MIN = 320;

  function defaultColumnIds() {
    return COLUMNS.filter((c) => c.defaultOn || c.locked).map((c) => c.id);
  }

  function maxFiltersWidthForViewport() {
    // Prefer live grid width once the DOM is ready; fall back to viewport.
    const grid = document.getElementById("dash-grid");
    let avail = 0;
    if (grid) {
      avail = grid.clientWidth || grid.getBoundingClientRect().width || 0;
    }
    if (!avail) {
      avail = Math.max(0, (window.innerWidth || 1200) - 32);
    }
    // Leave resizer + a usable right column so we never overflow horizontally.
    return Math.max(
      FILTERS_WIDTH_MIN,
      Math.min(FILTERS_WIDTH_MAX, Math.floor(avail - FILTERS_RESIZER_PX - FILTERS_RIGHT_MIN)),
    );
  }

  function clampFiltersWidth(px) {
    const n = Math.round(Number(px));
    if (!Number.isFinite(n)) return FILTERS_WIDTH_DEFAULT;
    const max = maxFiltersWidthForViewport();
    return Math.min(max, Math.max(FILTERS_WIDTH_MIN, n));
  }

  function loadFiltersWidth() {
    const raw = parseInt(localStorage.getItem(FILTERS_WIDTH_LS_KEY), 10);
    // Soft clamp only (min/max constants) at load time; viewport clamp happens on apply.
    if (!Number.isFinite(raw)) return FILTERS_WIDTH_DEFAULT;
    return Math.min(FILTERS_WIDTH_MAX, Math.max(FILTERS_WIDTH_MIN, Math.round(raw)));
  }

  function applyFiltersWidth(px, { save = true } = {}) {
    const w = clampFiltersWidth(px);
    document.documentElement.style.setProperty("--filters-width", w + "px");
    const handle = document.getElementById("filters-resizer");
    if (handle) {
      handle.setAttribute("aria-valuenow", String(w));
      handle.setAttribute("aria-valuemin", String(FILTERS_WIDTH_MIN));
      handle.setAttribute("aria-valuemax", String(maxFiltersWidthForViewport()));
    }
    if (save) localStorage.setItem(FILTERS_WIDTH_LS_KEY, String(w));
    state.filtersWidth = w;
    return w;
  }

  function loadFilterSectionsCollapsed() {
    try {
      const raw = localStorage.getItem(FILTERS_SECTIONS_LS_KEY);
      if (!raw) return {};
      const parsed = JSON.parse(raw);
      return parsed && typeof parsed === "object" ? parsed : {};
    } catch {
      return {};
    }
  }

  function saveFilterSectionsCollapsed(map) {
    localStorage.setItem(FILTERS_SECTIONS_LS_KEY, JSON.stringify(map));
  }

  function setFilterSectionCollapsed(section, collapsed, { save = true } = {}) {
    if (!section) return;
    const id = section.dataset.filterSection;
    section.classList.toggle("collapsed", collapsed);
    const btn = section.querySelector(".filter-section-toggle");
    if (btn) btn.setAttribute("aria-expanded", collapsed ? "false" : "true");
    if (save && id) {
      const map = loadFilterSectionsCollapsed();
      if (collapsed) map[id] = true;
      else delete map[id];
      saveFilterSectionsCollapsed(map);
    }
  }

  function bindFilterSections() {
    const sections = document.querySelectorAll(".filter-section[data-filter-section]");
    const saved = loadFilterSectionsCollapsed();
    for (const section of sections) {
      const id = section.dataset.filterSection;
      if (saved[id]) setFilterSectionCollapsed(section, true, { save: false });
      const btn = section.querySelector(".filter-section-toggle");
      if (!btn) continue;
      btn.addEventListener("click", () => {
        const next = !section.classList.contains("collapsed");
        setFilterSectionCollapsed(section, next, { save: true });
      });
    }
  }

  /** Mark sections that currently have active filter values (visual hint when collapsed). */
  function updateFilterSectionActiveHints() {
    const sections = document.querySelectorAll(".filter-section[data-filter-section]");
    for (const section of sections) {
      const inputs = section.querySelectorAll("input, select");
      let active = false;
      for (const el of inputs) {
        if (el.tagName === "SELECT" && el.multiple) {
          if (Array.from(el.selectedOptions).length) { active = true; break; }
        } else if (String(el.value || "").trim() !== "") {
          active = true;
          break;
        }
      }
      section.classList.toggle("has-active", active);
    }
  }

  function loadVisibleColumns() {
    const locked = COLUMNS.filter((c) => c.locked).map((c) => c.id);
    let ids = defaultColumnIds();
    try {
      const raw = localStorage.getItem(COLUMNS_LS_KEY);
      if (raw) {
        const parsed = JSON.parse(raw);
        if (Array.isArray(parsed)) {
          const valid = parsed.filter((id) => COLUMN_BY_ID[id]);
          if (valid.length) ids = valid;
        }
      }
    } catch {
      /* ignore corrupt storage */
    }
    // Keep locked columns first and always present; preserve user order for the rest.
    const seen = new Set();
    const ordered = [];
    for (const id of locked) {
      if (!seen.has(id)) {
        ordered.push(id);
        seen.add(id);
      }
    }
    for (const id of ids) {
      if (!seen.has(id) && COLUMN_BY_ID[id]) {
        ordered.push(id);
        seen.add(id);
      }
    }
    if (!ordered.length) return defaultColumnIds();
    return ordered;
  }

  function saveVisibleColumns(ids) {
    localStorage.setItem(COLUMNS_LS_KEY, JSON.stringify(ids));
  }

  const SORT_LS_KEY = "tableSort";
  const VALID_SORT_KEYS = new Set(COLUMNS.map((c) => c.sort));

  function loadSort() {
    try {
      const raw = localStorage.getItem(SORT_LS_KEY);
      if (!raw) return { sortKey: "ip", sortDir: 1 };
      const parsed = JSON.parse(raw);
      const key = parsed && typeof parsed.key === "string" ? parsed.key : "ip";
      const dir = parsed && Number(parsed.dir) === -1 ? -1 : 1;
      return {
        sortKey: VALID_SORT_KEYS.has(key) ? key : "ip",
        sortDir: dir,
      };
    } catch {
      return { sortKey: "ip", sortDir: 1 };
    }
  }

  function saveSort() {
    localStorage.setItem(
      SORT_LS_KEY,
      JSON.stringify({ key: state.sortKey, dir: state.sortDir }),
    );
  }

  const CHART_RANGE_LS_KEY = "chartRange";
  const CHART_CUSTOM_LS_KEY = "chartCustomRange";
  const VALID_CHART_RANGES = new Set(["4h", "12h", "1d", "3d", "7d", "custom"]);

  function loadChartRange() {
    const raw = localStorage.getItem(CHART_RANGE_LS_KEY) || "1d";
    return VALID_CHART_RANGES.has(raw) ? raw : "1d";
  }

  function loadChartCustom() {
    try {
      const raw = localStorage.getItem(CHART_CUSTOM_LS_KEY);
      if (!raw) return { from: "", to: "" };
      const parsed = JSON.parse(raw);
      return {
        from: parsed && typeof parsed.from === "string" ? parsed.from : "",
        to: parsed && typeof parsed.to === "string" ? parsed.to : "",
      };
    } catch {
      return { from: "", to: "" };
    }
  }

  function saveChartCustom() {
    localStorage.setItem(
      CHART_CUSTOM_LS_KEY,
      JSON.stringify({ from: state.chartFrom, to: state.chartTo }),
    );
  }

  /** Convert datetime-local value to UTC RFC3339 for the API. */
  function localInputToRFC3339(v) {
    if (!v) return null;
    const d = new Date(v);
    if (Number.isNaN(d.getTime())) return null;
    return d.toISOString();
  }

  /** Format a Date as datetime-local value in local timezone. */
  function toLocalInputValue(d) {
    if (!(d instanceof Date) || Number.isNaN(d.getTime())) return "";
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  }

  function windowMs(range) {
    switch (range) {
      case "4h": return 4 * 3600 * 1000;
      case "12h": return 12 * 3600 * 1000;
      case "1d": return 24 * 3600 * 1000;
      case "3d": return 3 * 24 * 3600 * 1000;
      case "7d": return 7 * 24 * 3600 * 1000;
      default: return 24 * 3600 * 1000;
    }
  }

  /** Narrow viewports: hide custom From/To and only expose preset ranges. */
  const CHART_MOBILE_MQ = window.matchMedia("(max-width: 720px)");
  function isChartMobile() {
    return CHART_MOBILE_MQ.matches;
  }

  /**
   * Range used for history/chart queries.
   * On mobile, custom from/to is hidden — treat custom as 1d without wiping storage.
   */
  function effectiveChartRange() {
    if (isChartMobile() && state.chartRange === "custom") return "1d";
    return state.chartRange;
  }

  /** Resolve the active chart time window as epoch ms { since, until }. */
  function chartWindowBounds() {
    const now = Date.now();
    const range = effectiveChartRange();
    if (range === "custom") {
      let since = state.chartFrom ? new Date(state.chartFrom).getTime() : NaN;
      let until = state.chartTo ? new Date(state.chartTo).getTime() : NaN;
      if (!Number.isFinite(since) && !Number.isFinite(until)) {
        until = now;
        since = now - windowMs("1d");
      } else if (!Number.isFinite(since)) {
        since = until - windowMs("1d");
      } else if (!Number.isFinite(until)) {
        until = now;
      }
      return { since, until };
    }
    const span = windowMs(range);
    return { since: now - span, until: now };
  }

  /** Build since/until query params for /api/history from the current range UI. */
  function historyTimeParams() {
    const params = new URLSearchParams();
    const range = effectiveChartRange();
    if (range === "custom") {
      const since = localInputToRFC3339(state.chartFrom);
      const until = localInputToRFC3339(state.chartTo);
      if (since) params.set("since", since);
      if (until) params.set("until", until);
      // If only "to" is set, still bound the window; if neither, fall back to 1d.
      if (!since && !until) {
        params.set("window", "1d");
      }
    } else {
      params.set("window", range);
    }
    return params;
  }

  const savedSort = loadSort();
  const savedCustom = loadChartCustom();
  const state = {
    miners: [],
    meta: null,
    selectedId: null,
    detail: null,
    history: [],
    hiddenSeries: new Set(),
    sortKey: savedSort.sortKey,
    sortDir: savedSort.sortDir,
    visibleColumns: loadVisibleColumns(),
    filtersWidth: loadFiltersWidth(),
    chartRange: loadChartRange(),
    chartFrom: savedCustom.from,
    chartTo: savedCustom.to,
    refreshSec: Number(localStorage.getItem("refreshSec") || 30),
    timer: null,
  };

  const $ = (id) => document.getElementById(id);
  const els = {
    status: $("status-pill"),
    pollMeta: $("poll-meta"),
    pollInterval: $("poll-interval"),
    btnRefresh: $("btn-refresh"),
    btnRescan: $("btn-rescan"),
    btnTheme: $("btn-theme"),
    btnClear: $("btn-clear-filters"),
    btnToggleFilters: $("btn-toggle-filters"),
    filtersPanel: $("filters-panel"),
    filtersBody: $("filters-body"),
    filtersCollapsedSummary: $("filters-collapsed-summary"),
    filterActiveBadge: $("filter-active-badge"),
    filterCountCollapsed: $("filter-count-collapsed"),
    filterSummaryBits: $("filter-summary-bits"),
    search: $("f-search"),
    make: $("f-make"),
    model: $("f-model"),
    firmware: $("f-firmware"),
    algo: $("f-algo"),
    mining: $("f-mining"),
    errors: $("f-errors"),
    hrMin: $("f-hr-min"),
    hrMax: $("f-hr-max"),
    tempMin: $("f-temp-min"),
    tempMax: $("f-temp-max"),
    vrMin: $("f-vr-min"),
    vrMax: $("f-vr-max"),
    wMin: $("f-w-min"),
    wMax: $("f-w-max"),
    chipsMin: $("f-chips-min"),
    chipsMax: $("f-chips-max"),
    effMin: $("f-eff-min"),
    effMax: $("f-eff-max"),
    filterCount: $("filter-count"),
    detail: $("detail-body"),
    chartMetric: $("chart-metric"),
    chartRange: $("chart-range"),
    chartCustomRange: $("chart-custom-range"),
    chartFrom: $("chart-from"),
    chartTo: $("chart-to"),

    chart: $("chart"),
    legend: $("chart-legend"),
    tbody: $("miners-tbody"),
    theadRow: $("miners-thead-row"),
    tableCount: $("table-count"),
    totals: $("fleet-totals"),
    table: $("miners-table"),
    btnColumns: $("btn-columns"),
    columnsPanel: $("columns-panel"),
    columnsList: $("columns-list"),
    btnColumnsAll: $("btn-columns-all"),
    btnColumnsDefault: $("btn-columns-default"),
    dashGrid: $("dash-grid"),
    colLeft: $("col-left"),
    filtersResizer: $("filters-resizer"),
  };

  const COLORS = [
    "#3b82f6", "#ef4444", "#22c55e", "#f59e0b", "#a855f7",
    "#06b6d4", "#f97316", "#84cc16", "#ec4899", "#14b8a6",
    "#6366f1", "#eab308", "#0ea5e9", "#d946ef", "#65a30d",
  ];

  async function api(path, opts = {}) {
    const r = await fetch(path, {
      headers: { Accept: "application/json", ...(opts.headers || {}) },
      ...opts,
    });
    if (!r.ok) {
      let msg = `${path}: ${r.status}`;
      try {
        const t = await r.text();
        if (t) msg += ` ${t.trim()}`;
      } catch { /* ignore */ }
      throw new Error(msg);
    }
    if (r.status === 204) return null;
    const ct = r.headers.get("content-type") || "";
    if (ct.includes("application/json")) return r.json();
    return null;
  }

  async function sleep(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /** Wait until the backend poll cycle finishes (or timeout). */
  async function waitUntilIdle(timeoutMs = 120000) {
    const start = Date.now();
    // Give the server a moment to flip polling=true.
    await sleep(300);
    while (Date.now() - start < timeoutMs) {
      try {
        const meta = await api("/api/meta");
        if (meta && !meta.polling) return meta;
      } catch {
        /* keep waiting */
      }
      await sleep(1000);
    }
    return null;
  }

  function multiValues(sel) {
    return Array.from(sel.selectedOptions).map((o) => o.value);
  }

  function numOrNull(input) {
    const v = input.value.trim();
    if (v === "") return null;
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }

  function fillSelect(sel, values, keep) {
    const prev = new Set(multiValues(sel));
    sel.innerHTML = "";
    for (const v of values) {
      const o = document.createElement("option");
      o.value = v;
      o.textContent = v;
      if (prev.has(v) || (keep && keep.has(v))) o.selected = true;
      sel.appendChild(o);
    }
  }

  function applyFilters(list) {
    const q = els.search.value.trim().toLowerCase();
    const makes = multiValues(els.make);
    const models = multiValues(els.model);
    const fws = multiValues(els.firmware);
    const algos = multiValues(els.algo);
    const mining = els.mining.value;
    const status = els.errors.value;
    const hrMin = numOrNull(els.hrMin);
    const hrMax = numOrNull(els.hrMax);
    const tMin = numOrNull(els.tempMin);
    const tMax = numOrNull(els.tempMax);
    const vrMin = numOrNull(els.vrMin);
    const vrMax = numOrNull(els.vrMax);
    const wMin = numOrNull(els.wMin);
    const wMax = numOrNull(els.wMax);
    const cMin = numOrNull(els.chipsMin);
    const cMax = numOrNull(els.chipsMax);
    const eMin = numOrNull(els.effMin);
    const eMax = numOrNull(els.effMax);

    return list.filter((m) => {
      if (q) {
        const hay = [m.ip, m.hostname, m.make, m.model, m.firmware, m.serial, m.mac, m.algo]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();
        if (!hay.includes(q)) return false;
      }
      if (makes.length && !makes.includes(m.make)) return false;
      if (models.length && !models.includes(m.model)) return false;
      if (fws.length && !fws.includes(m.firmware)) return false;
      if (algos.length && !algos.includes(m.algo)) return false;
      if (mining === "1" && !m.is_mining) return false;
      if (mining === "0" && m.is_mining) return false;
      if (status === "ok" && m.error) return false;
      if (status === "err" && !m.error) return false;
      if (status === "msg" && !(m.messages && m.messages.length)) return false;
      if (hrMin != null && m.hashrate_th < hrMin) return false;
      if (hrMax != null && m.hashrate_th > hrMax) return false;
      // ASIC / VR filters use the max reading for that sensor family.
      const asicMax = m.has_asic_temp ? m.asic_temp_max : (m.avg_temp_c || 0);
      const vrMaxVal = m.has_vr_temp ? m.vr_temp_max : 0;
      if (tMin != null && asicMax < tMin) return false;
      if (tMax != null && asicMax > tMax) return false;
      if (vrMin != null && (!m.has_vr_temp || vrMaxVal < vrMin)) return false;
      if (vrMax != null && (!m.has_vr_temp || vrMaxVal > vrMax)) return false;
      if (wMin != null && (m.wattage || 0) < wMin) return false;
      if (wMax != null && (m.wattage || 0) > wMax) return false;
      if (cMin != null && (m.total_chips || 0) < cMin) return false;
      if (cMax != null && (m.total_chips || 0) > cMax) return false;
      if (eMin != null && (m.efficiency || 0) < eMin) return false;
      if (eMax != null && (m.efficiency || 0) > eMax) return false;
      return true;
    });
  }

  function sortValue(m, key) {
    switch (key) {
      case "asic_temp_max":
        return m.has_asic_temp ? m.asic_temp_max : (m.avg_temp_c || 0);
      case "vr_temp_max":
        return m.has_vr_temp ? m.vr_temp_max : 0;
      case "pool_user":
        return (m.pool_users && m.pool_users[0]) || "";
      case "pool_host":
        return (m.pool_hosts && m.pool_hosts[0]) || "";
      case "error":
        return m.error || (m.messages && m.messages.length ? "msg" : "ok");
      default:
        return m[key];
    }
  }

  function sortList(list) {
    const key = state.sortKey;
    const dir = state.sortDir;
    return list.slice().sort((a, b) => {
      let av = sortValue(a, key);
      let bv = sortValue(b, key);
      if (typeof av === "boolean") av = av ? 1 : 0;
      if (typeof bv === "boolean") bv = bv ? 1 : 0;
      if (av == null) av = "";
      if (bv == null) bv = "";
      if (typeof av === "string") {
        return av.localeCompare(bv, undefined, { numeric: true }) * dir;
      }
      return (av - bv) * dir;
    });
  }

  function fmt(n, d = 1) {
    if (n == null || n === "" || Number.isNaN(n)) return "—";
    return Number(n).toFixed(d);
  }

  /** Format min–max range; collapses to a single value when equal. */
  function fmtRange(min, max, d = 1) {
    if (min == null || max == null || Number.isNaN(min) || Number.isNaN(max)) return "—";
    if (Number(min) === Number(max)) return fmt(min, d);
    return `${fmt(min, d)}–${fmt(max, d)}`;
  }

  function asicRange(m) {
    if (m.has_asic_temp) return fmtRange(m.asic_temp_min, m.asic_temp_max);
    if (m.avg_temp_c) return fmt(m.avg_temp_c, 1);
    return "—";
  }

  function vrRange(m) {
    if (m.has_vr_temp) return fmtRange(m.vr_temp_min, m.vr_temp_max);
    return "—";
  }

  function isWarm(m) {
    if (m.has_asic_temp && m.asic_temp_max >= 70) return true;
    if (m.has_vr_temp && m.vr_temp_max >= 70) return true;
    if (!m.has_asic_temp && (m.avg_temp_c || 0) >= 70) return true;
    return false;
  }

  function fmtUptime(sec) {
    if (!sec) return "—";
    const d = Math.floor(sec / 86400);
    const h = Math.floor((sec % 86400) / 3600);
    const m = Math.floor((sec % 3600) / 60);
    if (d > 0) return `${d}d ${h}h`;
    if (h > 0) return `${h}h ${m}m`;
    return `${m}m`;
  }

  function renderMeta() {
    const m = state.meta;
    if (!m) return;
    els.status.textContent = m.polling ? "polling…" : "live";
    els.status.className = "badge " + (m.last_poll_err ? "err" : "ok");
    const last = m.last_poll_at ? new Date(m.last_poll_at).toLocaleTimeString() : "—";
    els.pollMeta.textContent = `${m.miner_count} miners · poll ${m.poll_interval_sec}s · last ${last}` +
      (m.last_poll_err ? ` · ${m.last_poll_err}` : "");
  }

  /** Populate brand/model (and related) filters from /api/history series metadata. */
  function fillFiltersFromHistory(seriesList) {
    const makes = new Set();
    const models = new Set();
    const firmwares = new Set();
    const algos = new Set();
    for (const s of seriesList || []) {
      if (s.make) makes.add(s.make);
      if (s.model) models.add(s.model);
      if (s.firmware) firmwares.add(s.firmware);
      if (s.algo) algos.add(s.algo);
    }
    // Prefer history facets; fall back to live miner list if history is empty.
    if (!makes.size && !models.size) {
      for (const m of state.miners) {
        if (m.make) makes.add(m.make);
        if (m.model) models.add(m.model);
        if (m.firmware) firmwares.add(m.firmware);
        if (m.algo) algos.add(m.algo);
      }
    }
    fillSelect(els.make, [...makes].sort());
    fillSelect(els.model, [...models].sort());
    fillSelect(els.firmware, [...firmwares].sort());
    fillSelect(els.algo, [...algos].sort());
  }

  function activeFilterSummary() {
    const bits = [];
    const q = els.search.value.trim();
    if (q) bits.push(`“${q}”`);
    const makes = multiValues(els.make);
    if (makes.length) bits.push(makes.length === 1 ? makes[0] : `${makes.length} brands`);
    const models = multiValues(els.model);
    if (models.length) bits.push(models.length === 1 ? models[0] : `${models.length} models`);
    const fws = multiValues(els.firmware);
    if (fws.length) bits.push(fws.length === 1 ? fws[0] : `${fws.length} FW`);
    if (els.mining.value === "1") bits.push("mining");
    if (els.mining.value === "0") bits.push("not mining");
    if (els.errors.value) bits.push(els.errors.value);
    const ranges = [
      [els.hrMin, els.hrMax, "TH"],
      [els.tempMin, els.tempMax, "ASIC °C"],
      [els.vrMin, els.vrMax, "VR °C"],
      [els.wMin, els.wMax, "W"],
      [els.chipsMin, els.chipsMax, "chips"],
      [els.effMin, els.effMax, "J/TH"],
    ];
    for (const [a, b, label] of ranges) {
      const lo = a.value.trim();
      const hi = b.value.trim();
      if (lo || hi) bits.push(`${lo || "…"}–${hi || "…"} ${label}`);
    }
    return bits;
  }

  function setFiltersCollapsed(collapsed) {
    if (!els.filtersPanel) return;
    els.filtersPanel.classList.toggle("collapsed", collapsed);
    if (els.btnToggleFilters) {
      els.btnToggleFilters.setAttribute("aria-expanded", collapsed ? "false" : "true");
      els.btnToggleFilters.title = collapsed ? "Expand filters" : "Collapse filters";
    }
    if (els.filtersCollapsedSummary) {
      els.filtersCollapsedSummary.hidden = !collapsed;
    }
    localStorage.setItem("filtersCollapsed", collapsed ? "1" : "0");
    updateFilterChrome();
  }

  function updateFilterChrome() {
    const n = applyFilters(state.miners).length;
    if (els.filterCount) els.filterCount.textContent = String(n);
    if (els.filterCountCollapsed) els.filterCountCollapsed.textContent = String(n);
    const bits = activeFilterSummary();
    if (els.filterSummaryBits) {
      els.filterSummaryBits.textContent = bits.length ? " · " + bits.join(" · ") : "";
    }
    if (els.filterActiveBadge) {
      els.filterActiveBadge.hidden = !bits.length;
    }
    updateFilterSectionActiveHints();
  }

  function cellHTML(col, m) {
    switch (col.id) {
      case "ip":
        return esc(m.ip);
      case "hostname":
        return esc(m.hostname || "");
      case "make":
        return esc(m.make || "");
      case "model":
        return esc(m.model || "");
      case "firmware":
        return esc(m.firmware || "");
      case "firmware_version":
        return esc(m.firmware_version || "");
      case "algo":
        return esc(m.algo || "");
      case "mac":
        return esc(m.mac || "");
      case "serial":
        return esc(m.serial || "");
      case "hashrate_th":
        return fmt(m.hashrate_th, 2);
      case "expected_th":
        return fmt(m.expected_th, 1);
      case "asic_temp":
        return asicRange(m);
      case "vr_temp":
        return vrRange(m);
      case "avg_temp":
        return fmt(m.avg_temp_c, 1);
      case "wattage":
        return fmt(m.wattage, 0);
      case "efficiency":
        return fmt(m.efficiency, 1);
      case "total_chips":
        return m.total_chips || "—";
      case "expected_chips":
        return m.expected_chips || "—";
      case "boards":
        return m.boards || "—";
      case "fans":
        return m.fans || "—";
      case "uptime":
        return fmtUptime(m.uptime_sec);
      case "pool_user":
        return esc((m.pool_users && m.pool_users[0]) || "");
      case "pool_host":
        return esc((m.pool_hosts && m.pool_hosts[0]) || "");
      case "is_mining":
        return m.is_mining ? "yes" : "no";
      case "status":
        return esc(m.error || (m.messages && m.messages.length ? "msg" : "ok"));
      default:
        return "—";
    }
  }

  function cellClass(col) {
    const parts = [];
    if (col.num) parts.push("num");
    if (col.id === "asic_temp") parts.push("temp", "asic-temp");
    if (col.id === "vr_temp") parts.push("temp", "vr-temp");
    if (col.id === "avg_temp") parts.push("temp");
    if (col.id === "status") parts.push("status");
    return parts.join(" ");
  }

  function visibleColumnDefs() {
    return state.visibleColumns.map((id) => COLUMN_BY_ID[id]).filter(Boolean);
  }

  function renderTableHeader() {
    if (!els.theadRow) return;
    const cols = visibleColumnDefs();
    const frag = document.createDocumentFragment();
    for (const col of cols) {
      const th = document.createElement("th");
      th.dataset.sort = col.sort;
      th.dataset.col = col.id;
      th.textContent = col.label;
      if (col.num) th.classList.add("num");
      if (col.title) th.title = col.title;
      if (col.sort === state.sortKey) {
        th.classList.add(state.sortDir > 0 ? "sort-asc" : "sort-desc");
      }
      frag.appendChild(th);
    }
    els.theadRow.replaceChildren(frag);
  }

  function setVisibleColumns(ids) {
    const locked = COLUMNS.filter((c) => c.locked).map((c) => c.id);
    const seen = new Set();
    const next = [];
    for (const id of locked) {
      if (!seen.has(id)) {
        next.push(id);
        seen.add(id);
      }
    }
    for (const id of ids) {
      if (!seen.has(id) && COLUMN_BY_ID[id]) {
        next.push(id);
        seen.add(id);
      }
    }
    // Never allow an empty table (locked columns guarantee at least IP).
    state.visibleColumns = next.length ? next : defaultColumnIds();
    saveVisibleColumns(state.visibleColumns);
    // If current sort column was hidden, fall back to IP.
    const stillSortable = visibleColumnDefs().some((c) => c.sort === state.sortKey);
    if (!stillSortable) {
      state.sortKey = "ip";
      state.sortDir = 1;
      saveSort();
    }
    renderColumnsPanel();
    renderTable();
  }

  function renderColumnsPanel() {
    if (!els.columnsList) return;
    const on = new Set(state.visibleColumns);
    const frag = document.createDocumentFragment();
    for (const col of COLUMNS) {
      const label = document.createElement("label");
      if (col.locked) label.classList.add("locked");
      const input = document.createElement("input");
      input.type = "checkbox";
      input.value = col.id;
      input.checked = on.has(col.id) || !!col.locked;
      input.disabled = !!col.locked;
      input.addEventListener("change", () => {
        const selected = COLUMNS
          .filter((c) => {
            if (c.locked) return true;
            const el = els.columnsList.querySelector(`input[value="${c.id}"]`);
            return el && el.checked;
          })
          .map((c) => c.id);
        setVisibleColumns(selected);
      });
      label.appendChild(input);
      label.appendChild(document.createTextNode(col.label));
      frag.appendChild(label);
    }
    els.columnsList.replaceChildren(frag);
  }

  function setColumnsPanelOpen(open) {
    if (!els.columnsPanel || !els.btnColumns) return;
    els.columnsPanel.hidden = !open;
    els.btnColumns.setAttribute("aria-expanded", open ? "true" : "false");
  }

  function renderTable() {
    const filtered = sortList(applyFilters(state.miners));
    els.filterCount.textContent = String(filtered.length);
    if (els.filterCountCollapsed) els.filterCountCollapsed.textContent = String(filtered.length);
    updateFilterChrome();
    els.tableCount.textContent = `(${filtered.length}/${state.miners.length})`;

    let th = 0, w = 0, chips = 0, mining = 0;
    for (const m of filtered) {
      th += m.hashrate_th || 0;
      w += m.wattage || 0;
      chips += m.total_chips || 0;
      if (m.is_mining) mining++;
    }
    els.totals.innerHTML =
      `<span>${fmt(th, 1)} TH/s</span>` +
      `<span>${fmt(w, 0)} W</span>` +
      `<span>${mining} mining</span>` +
      `<span>${chips} chips</span>`;

    renderTableHeader();
    const cols = visibleColumnDefs();

    const frag = document.createDocumentFragment();
    for (const m of filtered) {
      const tr = document.createElement("tr");
      if (m.id === state.selectedId) tr.classList.add("selected");
      if (m.error) tr.classList.add("has-error");
      if (isWarm(m)) tr.classList.add("warm");
      tr.dataset.id = m.id;
      for (const col of cols) {
        const td = document.createElement("td");
        const cls = cellClass(col);
        if (cls) td.className = cls;
        if (col.id === "asic_temp") td.title = "ASIC chip min–max";
        if (col.id === "vr_temp") td.title = "VR / board min–max";
        td.innerHTML = cellHTML(col, m);
        tr.appendChild(td);
      }
      frag.appendChild(tr);
    }
    els.tbody.replaceChildren(frag);
  }

  function esc(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function renderDetail() {
    const d = state.detail;
    if (!d) {
      els.detail.innerHTML = `<p class="text-light">Select a miner from the list.</p>`;
      return;
    }
    const rows = [
      ["IP", d.ip],
      ["Hostname", d.hostname],
      ["Make", d.make],
      ["Model", d.model],
      ["Firmware", `${d.firmware || ""}${d.firmware_version ? " " + d.firmware_version : ""}`],
      ["Algo", d.algo],
      ["MAC", d.mac],
      ["Serial", d.serial],
      ["Mining", d.is_mining ? "yes" : "no"],
      ["Hashrate", `${fmt(d.hashrate_th, 2)} TH/s (exp ${fmt(d.expected_th, 1)})`],
      ["ASIC temp", d.has_asic_temp
        ? `${fmtRange(d.asic_temp_min, d.asic_temp_max)} °C`
        : (d.avg_temp_c ? `${fmt(d.avg_temp_c, 1)} °C` : "—")],
      ["VR temp", d.has_vr_temp
        ? `${fmtRange(d.vr_temp_min, d.vr_temp_max)} °C`
        : "—"],
      ["Avg temp", d.avg_temp_c ? `${fmt(d.avg_temp_c, 1)} °C` : "—"],
      ["Fluid temp", d.fluid_temp_c ? `${fmt(d.fluid_temp_c, 1)} °C` : "—"],
      ["Power", `${fmt(d.wattage, 0)} W`],
      ["Efficiency", `${fmt(d.efficiency, 2)} J/TH`],
      ["Chips", `${d.total_chips || "—"} / ${d.expected_chips || "—"}`],
      ["Boards", d.boards || "—"],
      ["Fans", d.fans || "—"],
      ["Uptime", fmtUptime(d.uptime_sec)],
      ["Updated", d.updated_at ? new Date(d.updated_at).toLocaleString() : "—"],
    ];
    let html = "<dl>";
    for (const [k, v] of rows) {
      if (v == null || v === "" || v === "—") continue;
      html += `<dt>${esc(k)}</dt><dd>${esc(v)}</dd>`;
    }
    html += "</dl>";

    if (d.error) {
      html += `<p class="status-err"><strong>Error:</strong> ${esc(d.error)}</p>`;
    }
    if (d.messages && d.messages.length) {
      html += `<h4>Messages</h4><ul class="msg-list">${d.messages.map((m) => `<li>${esc(m)}</li>`).join("")}</ul>`;
    }
    if (d.hashboards && d.hashboards.length) {
      html += `<h4>Hashboards</h4><table><thead><tr>
        <th>#</th><th>TH/s</th><th>ASIC in</th><th>ASIC out</th><th>VR</th><th>Chips</th><th>MHz</th><th>V</th>
      </tr></thead><tbody>`;
      for (const b of d.hashboards) {
        const asicIn = b.has_asic_in ? fmt(b.asic_temp_in, 1) : "—";
        const asicOut = b.has_asic_out ? fmt(b.asic_temp_out, 1) : "—";
        const vr = b.has_vr_temp ? fmt(b.vr_temp_c, 1) : (b.board_temp_c ? fmt(b.board_temp_c, 1) : "—");
        html += `<tr>
          <td>${b.position}</td>
          <td>${fmt(b.hashrate_th, 2)}</td>
          <td class="num">${asicIn}</td>
          <td class="num">${asicOut}</td>
          <td class="num">${vr}</td>
          <td>${b.working_chips || "—"}/${b.expected_chips || "—"}</td>
          <td>${fmt(b.frequency, 0)}</td>
          <td>${fmt(b.voltage, 2)}</td>
        </tr>`;
      }
      html += "</tbody></table>";
    }
    if (d.fan_rpms && d.fan_rpms.length) {
      html += `<h4>Fans</h4><p>${d.fan_rpms.map((r) => fmt(r, 0) + " rpm").join(" · ")}</p>`;
    }
    if (d.pools && d.pools.length) {
      html += `<h4>Pools</h4><table><thead><tr>
        <th>URL</th><th>User</th><th>Acc</th><th>Rej</th>
      </tr></thead><tbody>`;
      for (const p of d.pools) {
        html += `<tr>
          <td>${esc(p.url || "")}</td>
          <td>${esc(p.user || "")}</td>
          <td>${p.accepted ?? "—"}</td>
          <td>${p.rejected ?? "—"}</td>
        </tr>`;
      }
      html += "</tbody></table>";
    }
    els.detail.innerHTML = html;
  }

  function seriesColor(i) {
    return COLORS[i % COLORS.length];
  }

  /** Color for a history series — type-agg shares one hue across avg/min/max. */
  function colorForSeries(s, fallbackIdx) {
    if (s && Number.isFinite(s.typeColor)) {
      return COLORS[s.typeColor % COLORS.length];
    }
    if (s && s.id) {
      const idx = state.history.findIndex((x) => x.id === s.id);
      if (idx >= 0) return COLORS[idx % COLORS.length];
    }
    return COLORS[fallbackIdx % COLORS.length];
  }

  /**
   * Max gap between samples before we treat the hole as missing data
   * (e.g. power outage). Derived from poll interval with jitter headroom.
   * Type-aggregated series use coarser buckets, so scale the threshold up.
   */
  function chartGapThresholdMs() {
    if (els.chartMetric && els.chartMetric.value === "hashrate_by_type") {
      return typeAggBucketMs() * 2.5;
    }
    const pollSec = (state.meta && state.meta.poll_interval_sec) || 30;
    return Math.max(pollSec * 2.5, 90) * 1000;
  }

  /**
   * Split a series into drawable segments. Large time holes become separate
   * segments with zero stubs at the edges so the line drops to 0 and does not
   * bridge the empty space.
   */
  function chartSegments(points, gapMs, tMax) {
    const pts = [];
    for (const p of points || []) {
      const t = new Date(p.t).getTime();
      const v = Number(p.v);
      if (!Number.isFinite(t) || !Number.isFinite(v)) continue;
      pts.push({ t, v });
    }
    pts.sort((a, b) => a.t - b.t);

    const segments = [];
    let seg = [];
    const pushSeg = () => {
      if (seg.length) segments.push(seg);
      seg = [];
    };

    for (let i = 0; i < pts.length; i++) {
      const cur = pts[i];
      if (seg.length) {
        const prev = seg[seg.length - 1];
        if (cur.t - prev.t > gapMs) {
          // Drop to zero after the last real sample, then break the path.
          if (prev.v !== 0) {
            seg.push({ t: prev.t, v: 0 });
          }
          pushSeg();
          // Rise from zero into the next sample (separate segment).
          if (cur.v !== 0) {
            seg.push({ t: cur.t, v: 0 });
          }
        }
      }
      seg.push(cur);
    }

    // Ongoing outage: last sample is stale relative to the chart window end.
    if (seg.length && Number.isFinite(tMax)) {
      const last = seg[seg.length - 1];
      if (tMax - last.t > gapMs) {
        if (last.v !== 0) {
          seg.push({ t: last.t, v: 0 });
        }
      }
    }
    pushSeg();
    return segments;
  }

  function drawChart() {
    const canvas = els.chart;
    const wrap = canvas.parentElement;
    const dpr = window.devicePixelRatio || 1;
    // Size to the right-column chart wrap only (not the full viewport).
    const cssW = Math.max(1, Math.floor((wrap && wrap.clientWidth) || canvas.clientWidth || 400));
    const cssH = 260;
    canvas.style.width = cssW + "px";
    canvas.style.height = cssH + "px";
    canvas.width = Math.floor(cssW * dpr);
    canvas.height = Math.floor(cssH * dpr);
    const ctx = canvas.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

    const w = cssW;
    const h = cssH;
    const pad = { l: 48, r: 12, t: 12, b: 28 };

    ctx.clearRect(0, 0, w, h);
    const series = state.history.filter((s) => !state.hiddenSeries.has(s.id) && s.points && s.points.length);

    // axes background
    ctx.fillStyle = getComputedStyle(canvas).color || "#888";
    ctx.globalAlpha = 0.08;
    ctx.fillRect(0, 0, w, h);
    ctx.globalAlpha = 1;

    if (!series.length) {
      ctx.fillStyle = getComputedStyle(document.body).color || "#888";
      ctx.globalAlpha = 0.5;
      ctx.font = "13px system-ui,sans-serif";
      ctx.fillText("No history yet — waiting for poll samples…", pad.l, h / 2);
      ctx.globalAlpha = 1;
      renderLegend();
      return;
    }

    // Prefer the selected UI window for the x-axis so empty ranges still show full width.
    const win = chartWindowBounds();
    let tMin = win.since;
    let tMax = win.until;
    const gapMs = chartGapThresholdMs();
    const prepared = series.map((s) => ({
      series: s,
      segments: chartSegments(s.points, gapMs, tMax),
    }));

    let vMin = Infinity, vMax = -Infinity;
    let dataTMin = Infinity, dataTMax = -Infinity;
    let hasGaps = false;
    for (const prep of prepared) {
      if (prep.segments.length > 1) hasGaps = true;
      for (const seg of prep.segments) {
        // A trailing zero stub (ongoing outage) also counts as a gap.
        if (seg.length >= 2 && seg[seg.length - 1].v === 0 && seg[seg.length - 2].v !== 0) {
          hasGaps = true;
        }
        for (const p of seg) {
          if (p.t < dataTMin) dataTMin = p.t;
          if (p.t > dataTMax) dataTMax = p.t;
          if (p.v < vMin) vMin = p.v;
          if (p.v > vMax) vMax = p.v;
        }
      }
    }
    // If data extends outside the nominal window (clock skew), expand slightly.
    if (Number.isFinite(dataTMin) && dataTMin < tMin) tMin = dataTMin;
    if (Number.isFinite(dataTMax) && dataTMax > tMax) tMax = dataTMax;
    if (tMin === tMax) tMax = tMin + 1;
    // Missing-data zeros should sit on the baseline.
    if (hasGaps && vMin > 0) vMin = 0;
    if (vMin === vMax) {
      vMin = vMin - 1;
      vMax = vMax + 1;
    }
    // pad y
    const yPad = (vMax - vMin) * 0.08 || 1;
    vMin -= yPad;
    vMax += yPad;

    const plotW = w - pad.l - pad.r;
    const plotH = h - pad.t - pad.b;
    const xOf = (t) => pad.l + ((t - tMin) / (tMax - tMin)) * plotW;
    const yOf = (v) => pad.t + (1 - (v - vMin) / (vMax - vMin)) * plotH;

    // grid
    ctx.strokeStyle = getComputedStyle(document.body).color || "#888";
    ctx.globalAlpha = 0.12;
    ctx.lineWidth = 1;
    for (let i = 0; i <= 4; i++) {
      const y = pad.t + (plotH * i) / 4;
      ctx.beginPath();
      ctx.moveTo(pad.l, y);
      ctx.lineTo(w - pad.r, y);
      ctx.stroke();
    }
    ctx.globalAlpha = 1;

    // y labels
    ctx.fillStyle = getComputedStyle(document.body).color || "#888";
    ctx.globalAlpha = 0.7;
    ctx.font = "11px system-ui,sans-serif";
    ctx.textAlign = "right";
    ctx.textBaseline = "middle";
    for (let i = 0; i <= 4; i++) {
      const v = vMax - ((vMax - vMin) * i) / 4;
      const y = pad.t + (plotH * i) / 4;
      ctx.fillText(fmt(v, vMax - vMin > 20 ? 0 : 1), pad.l - 6, y);
    }
    // x labels — include date when the window spans more than ~1 day
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    const xTicks = 5;
    const spanMs = tMax - tMin;
    const multiDay = spanMs > 26 * 3600 * 1000;
    for (let i = 0; i <= xTicks; i++) {
      const t = tMin + ((tMax - tMin) * i) / xTicks;
      const x = xOf(t);
      const d = new Date(t);
      let label;
      if (multiDay) {
        label = d.toLocaleString([], {
          month: "short",
          day: "numeric",
          hour: "2-digit",
          minute: "2-digit",
        });
      } else {
        label = d.toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
          second: spanMs < 2 * 3600 * 1000 ? "2-digit" : undefined,
        });
      }
      ctx.fillText(label, x, h - pad.b + 6);
    }
    ctx.globalAlpha = 1;

    // lines — one path per contiguous segment (gaps are not bridged)
    prepared.forEach((prep, idx) => {
      const s = prep.series;
      const color = colorForSeries(s, idx);
      ctx.strokeStyle = color;
      const band = s.stat === "min" || s.stat === "max";
      ctx.lineWidth = band ? 1.25 : 1.8;
      ctx.globalAlpha = band ? 0.55 : 1;
      ctx.setLineDash(band ? [5, 4] : []);
      for (const seg of prep.segments) {
        if (!seg.length) continue;
        ctx.beginPath();
        seg.forEach((p, i) => {
          const x = xOf(p.t);
          const y = yOf(p.v);
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        });
        ctx.stroke();
      }
      ctx.setLineDash([]);
      ctx.globalAlpha = 1;
    });

    renderLegend();
  }

  function renderLegend() {
    const frag = document.createDocumentFragment();
    state.history.forEach((s, i) => {
      const el = document.createElement("span");
      el.className = "item" + (state.hiddenSeries.has(s.id) ? " off" : "");
      el.dataset.id = s.id;
      const name = (s.stat || s.metric === "hashrate_by_type")
        ? (s.label || s.id)
        : (s.label && !looksLikeIP(s.label) && !looksLikeMAC(s.label) ? s.label : hostnameLabel(s, null));
      el.innerHTML = `<span class="swatch" style="background:${colorForSeries(s, i)}"></span>${esc(name)}`;
      el.addEventListener("click", () => {
        if (state.hiddenSeries.has(s.id)) state.hiddenSeries.delete(s.id);
        else state.hiddenSeries.add(s.id);
        drawChart();
      });
      frag.appendChild(el);
    });
    els.legend.replaceChildren(frag);
  }

  async function loadDetail(id) {
    if (!id) {
      state.detail = null;
      renderDetail();
      return;
    }
    try {
      state.detail = await api(`/api/miners/${encodeURIComponent(id)}`);
    } catch (e) {
      state.detail = {
        id,
        ip: id,
        error: String(e.message || e),
      };
    }
    renderDetail();
  }

  function syncChartRangeUI() {
    const mobile = isChartMobile();
    if (els.chartRange) {
      const customOpt = els.chartRange.querySelector('option[value="custom"]');
      if (customOpt) customOpt.hidden = mobile;
      // Select must show a visible option; keep stored custom for desktop.
      const display = mobile && state.chartRange === "custom" ? "1d" : state.chartRange;
      els.chartRange.value = display;
    }
    if (els.chartCustomRange) {
      els.chartCustomRange.hidden = mobile || state.chartRange !== "custom";
    }
    if (els.chartFrom && state.chartFrom) els.chartFrom.value = state.chartFrom;
    if (els.chartTo && state.chartTo) els.chartTo.value = state.chartTo;
  }

  function ensureCustomDefaults() {
    // When switching to custom with empty bounds, seed last 24h.
    if (state.chartRange !== "custom") return;
    if (state.chartFrom && state.chartTo) return;
    const to = new Date();
    const from = new Date(to.getTime() - windowMs("1d"));
    if (!state.chartFrom) state.chartFrom = toLocalInputValue(from);
    if (!state.chartTo) state.chartTo = toLocalInputValue(to);
    if (els.chartFrom) els.chartFrom.value = state.chartFrom;
    if (els.chartTo) els.chartTo.value = state.chartTo;
    saveChartCustom();
  }

  /**
   * Filters that can be evaluated on a history series alone (no live snapshot).
   * Used so miners that left the fleet still appear on the chart while they
   * have samples inside the selected window.
   */
  function seriesPassesFilters(s) {
    const q = els.search.value.trim().toLowerCase();
    const makes = multiValues(els.make);
    const models = multiValues(els.model);
    const fws = multiValues(els.firmware);
    const algos = multiValues(els.algo);
    // Live-only filters (mining / errors / numeric ranges) do not exclude
    // departed miners — they have no current snapshot.
    if (q) {
      const hay = [s.id, s.label, s.make, s.model, s.firmware, s.algo]
        .filter(Boolean)
        .join(" ")
        .toLowerCase();
      if (!hay.includes(q)) return false;
    }
    if (makes.length && !makes.includes(s.make)) return false;
    if (models.length && !models.includes(s.model)) return false;
    if (fws.length && !fws.includes(s.firmware)) return false;
    if (algos.length && !algos.includes(s.algo)) return false;
    return true;
  }

  /** Time bucket size for hashrate-by-type aggregation (ms). */
  function typeAggBucketMs() {
    const { since, until } = chartWindowBounds();
    const span = Math.max(1, until - since);
    const pollSec = (state.meta && state.meta.poll_interval_sec) || 30;
    const pollMs = Math.max(15, pollSec) * 1000;
    if (span <= 4 * 3600 * 1000) return Math.max(pollMs, 30 * 1000);
    if (span <= 24 * 3600 * 1000) return Math.max(pollMs, 60 * 1000);
    if (span <= 3 * 24 * 3600 * 1000) return 5 * 60 * 1000;
    return 15 * 60 * 1000;
  }

  /**
   * Group key for hashrate-by-type. Requires a real make and/or model —
   * never invents an "Unknown" bucket (untyped series are skipped).
   */
  function typeGroupKey(s) {
    const make = (s.make || "").trim();
    const model = (s.model || "").trim();
    if (make && model) return { key: `${make}\0${model}`, make, model, label: `${make} ${model}` };
    if (model) return { key: `\0${model}`, make: "", model, label: model };
    if (make) return { key: make, make, model: "", label: make };
    return null;
  }

  function looksLikeIP(s) {
    return /^\d{1,3}(\.\d{1,3}){3}$/.test(String(s || "").trim());
  }

  function looksLikeMAC(s) {
    return /^([0-9a-f]{2}[:-]){5}[0-9a-f]{2}$/i.test(String(s || "").trim()) ||
      /^[0-9a-f]{12}$/i.test(String(s || "").trim());
  }

  /** Chart legend label: hostname only (never IP / MAC / raw id). */
  function hostnameLabel(s, live) {
    const host = String((live && live.hostname) || s.hostname || s.label || "").trim();
    if (host && !looksLikeIP(host) && !looksLikeMAC(host)) return host;
    const model = String((live && live.model) || s.model || "").trim();
    if (model) return model;
    const make = String((live && live.make) || s.make || "").trim();
    if (make) return make;
    return "miner";
  }

  /** Find live miner matching a history series id (stable id, MAC, or IP alias). */
  function liveForSeries(s, byId, byMAC, byIP) {
    const id = String(s.id || "").trim();
    return byId.get(id) || byMAC.get(id.toLowerCase()) || byIP.get(id) || null;
  }

  /** Attach make/model/hostname from the live fleet (by id, MAC, or IP). */
  function enrichSeriesFromLive(seriesList, miners) {
    const byId = new Map();
    const byMAC = new Map();
    const byIP = new Map();
    for (const m of miners || []) {
      if (m.id) byId.set(m.id, m);
      const mac = (m.mac || "").trim().toLowerCase();
      if (mac) byMAC.set(mac, m);
      const ip = (m.ip || "").trim();
      if (ip) byIP.set(ip, m);
    }
    return (seriesList || []).map((s) => {
      // Type-aggregated series already have human labels (Make Model · avg).
      if (s.stat || s.metric === "hashrate_by_type") return s;
      const live = liveForSeries(s, byId, byMAC, byIP);
      return {
        ...s,
        make: (live && live.make) || s.make || "",
        model: (live && live.model) || s.model || "",
        firmware: (live && live.firmware) || s.firmware || "",
        algo: (live && live.algo) || s.algo || "",
        hostname: (live && live.hostname) || s.hostname || "",
        label: hostnameLabel(s, live),
      };
    });
  }

  /**
   * Collapse per-miner hashrate series into avg / min / max per make+model.
   * Samples are bucketed in time so miners polled a few seconds apart line up.
   * Series without make/model are omitted (no Unknown group).
   */
  function aggregateHashrateByType(seriesList) {
    const bucketMs = typeAggBucketMs();
    const groups = new Map(); // key -> { make, model, label, buckets: Map<t, number[]> }

    for (const s of seriesList || []) {
      const g = typeGroupKey(s);
      if (!g) continue;
      let entry = groups.get(g.key);
      if (!entry) {
        entry = { make: g.make, model: g.model, label: g.label, buckets: new Map() };
        groups.set(g.key, entry);
      }
      for (const p of s.points || []) {
        const t = new Date(p.t).getTime();
        const v = Number(p.v);
        if (!Number.isFinite(t) || !Number.isFinite(v)) continue;
        const b = Math.floor(t / bucketMs) * bucketMs;
        let arr = entry.buckets.get(b);
        if (!arr) {
          arr = [];
          entry.buckets.set(b, arr);
        }
        arr.push(v);
      }
    }

    const keys = [...groups.keys()].sort((a, b) => {
      const la = groups.get(a).label.toLowerCase();
      const lb = groups.get(b).label.toLowerCase();
      return la < lb ? -1 : la > lb ? 1 : 0;
    });

    const out = [];
    keys.forEach((key, typeIdx) => {
      const g = groups.get(key);
      const times = [...g.buckets.keys()].sort((a, b) => a - b);
      if (!times.length) return;

      const avgPts = [];
      const minPts = [];
      const maxPts = [];
      for (const t of times) {
        const vals = g.buckets.get(t);
        if (!vals || !vals.length) continue;
        let min = vals[0];
        let max = vals[0];
        let sum = 0;
        for (const v of vals) {
          if (v < min) min = v;
          if (v > max) max = v;
          sum += v;
        }
        const iso = new Date(t).toISOString();
        avgPts.push({ t: iso, v: sum / vals.length });
        minPts.push({ t: iso, v: min });
        maxPts.push({ t: iso, v: max });
      }

      const base = {
        make: g.make,
        model: g.model,
        metric: "hashrate_by_type",
        typeColor: typeIdx,
      };
      const slug = g.label.toLowerCase().replace(/\s+/g, "-");
      out.push({
        ...base,
        id: `type:${slug}:avg`,
        label: `${g.label} · avg`,
        stat: "avg",
        points: avgPts,
      });
      out.push({
        ...base,
        id: `type:${slug}:min`,
        label: `${g.label} · min`,
        stat: "min",
        points: minPts,
      });
      out.push({
        ...base,
        id: `type:${slug}:max`,
        label: `${g.label} · max`,
        stat: "max",
        points: maxPts,
      });
    });
    return out;
  }

  async function loadHistory() {
    const metric = els.chartMetric.value;
    const timeQ = historyTimeParams();
    const byType = metric === "hashrate_by_type";
    // Grouped view is derived from per-miner hashrate samples.
    const apiMetric = byType ? "hashrate" : metric;

    // Full window history (no ids) — includes miners no longer in the live fleet.
    let allSeries = [];
    try {
      const q = new URLSearchParams(timeQ);
      q.set("metric", apiMetric);
      allSeries = await api(`/api/history?${q.toString()}`);
    } catch {
      allSeries = [];
    }
    fillFiltersFromHistory(allSeries);

    // Resolve hostname/make/model from live fleet (id, MAC, or IP).
    allSeries = enrichSeriesFromLive(allSeries, state.miners);

    // Chart the filtered fleet (table filters). Legend toggles visibility;
    // all series start selected (visible).
    const liveFilteredIds = new Set(applyFilters(state.miners).map((m) => m.id));
    const byId = new Map(state.miners.map((m) => [m.id, m]));
    const byMAC = new Map(
      state.miners.filter((m) => m.mac).map((m) => [String(m.mac).toLowerCase(), m]),
    );
    const byIP = new Map(state.miners.filter((m) => m.ip).map((m) => [m.ip, m]));

    let chartSeries = allSeries.filter((s) => {
      const live = liveForSeries(s, byId, byMAC, byIP);
      if (live) return liveFilteredIds.has(live.id);
      // In-window history for miners not currently listed: keep if filters match.
      return seriesPassesFilters(s);
    });
    // Cap individual series; type aggregation wants the full filtered set.
    if (!byType && chartSeries.length > 40) chartSeries = chartSeries.slice(0, 40);

    if (byType) {
      chartSeries = aggregateHashrateByType(chartSeries);
    }

    // Drop stale legend hide flags for series that are gone; keep user toggles
    // for series still present. New series are visible by default.
    const present = new Set(chartSeries.map((s) => s.id));
    for (const id of [...state.hiddenSeries]) {
      if (!present.has(id)) state.hiddenSeries.delete(id);
    }

    state.history = chartSeries;
    drawChart();
  }

  async function refresh() {
    try {
      const [meta, miners] = await Promise.all([
        api("/api/meta"),
        api("/api/miners"),
      ]);
      state.meta = meta;
      state.miners = miners;
      renderMeta();
      renderTable();
      if (state.selectedId) {
        await loadDetail(state.selectedId);
      }
      await loadHistory();
      // Table filter counts depend on filter selects now filled from history.
      renderTable();
    } catch (e) {
      els.status.textContent = "error";
      els.status.className = "badge err";
      els.pollMeta.textContent = String(e.message || e);
    }
  }

  function schedule() {
    if (state.timer) clearInterval(state.timer);
    state.timer = setInterval(refresh, state.refreshSec * 1000);
  }

  function clearFilters() {
    els.search.value = "";
    [els.make, els.model, els.firmware, els.algo].forEach((s) => {
      Array.from(s.options).forEach((o) => (o.selected = false));
    });
    els.mining.value = "";
    els.errors.value = "";
    [els.hrMin, els.hrMax, els.tempMin, els.tempMax, els.vrMin, els.vrMax,
      els.wMin, els.wMax, els.chipsMin, els.chipsMax, els.effMin, els.effMax]
      .forEach((i) => { if (i) i.value = ""; });
    renderTable();
    updateFilterChrome();
    loadHistory();
  }

  function bind() {
    els.pollInterval.value = String(state.refreshSec);
    els.pollInterval.addEventListener("change", () => {
      state.refreshSec = Number(els.pollInterval.value) || 30;
      localStorage.setItem("refreshSec", String(state.refreshSec));
      schedule();
    });
    els.btnRefresh.addEventListener("click", () => refresh());
    if (els.btnRescan) {
      els.btnRescan.addEventListener("click", async () => {
        if (els.btnRescan.disabled) return;
        els.btnRescan.disabled = true;
        const prevLabel = els.btnRescan.textContent;
        els.btnRescan.textContent = "Scanning…";
        els.status.textContent = "rescanning…";
        els.status.className = "badge";
        try {
          await api("/api/rescan", { method: "POST" });
          await waitUntilIdle();
          await refresh();
        } catch (e) {
          els.status.textContent = "error";
          els.status.className = "badge err";
          els.pollMeta.textContent = String(e.message || e);
        } finally {
          els.btnRescan.disabled = false;
          els.btnRescan.textContent = prevLabel;
        }
      });
    }
    els.btnTheme.addEventListener("click", () => {
      const cur = document.documentElement.getAttribute("data-theme") ||
        (matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
      const next = cur === "dark" ? "light" : "dark";
      document.documentElement.style.colorScheme = next;
      document.documentElement.setAttribute("data-theme", next);
      localStorage.setItem("theme", next);
      drawChart();
    });
    els.btnClear.addEventListener("click", clearFilters);

    if (els.btnToggleFilters) {
      els.btnToggleFilters.addEventListener("click", () => {
        const collapsed = !els.filtersPanel.classList.contains("collapsed");
        setFiltersCollapsed(collapsed);
      });
      // Restore last collapse state (default expanded).
      setFiltersCollapsed(localStorage.getItem("filtersCollapsed") === "1");
    }

    bindFilterSections();

    const filterInputs = [
      els.search, els.make, els.model, els.firmware, els.algo, els.mining, els.errors,
      els.hrMin, els.hrMax, els.tempMin, els.tempMax, els.vrMin, els.vrMax,
      els.wMin, els.wMax, els.chipsMin, els.chipsMax, els.effMin, els.effMax,
    ].filter(Boolean);
    for (const el of filterInputs) {
      el.addEventListener("input", () => {
        renderTable();
        loadHistory();
      });
      el.addEventListener("change", () => {
        renderTable();
        loadHistory();
      });
    }

    els.chartMetric.addEventListener("change", () => loadHistory());

    if (els.chartRange) {
      syncChartRangeUI();
      els.chartRange.addEventListener("change", () => {
        let next = els.chartRange.value;
        if (isChartMobile() && next === "custom") next = "1d";
        state.chartRange = next;
        if (!VALID_CHART_RANGES.has(state.chartRange)) state.chartRange = "1d";
        localStorage.setItem(CHART_RANGE_LS_KEY, state.chartRange);
        if (state.chartRange === "custom") ensureCustomDefaults();
        syncChartRangeUI();
        loadHistory();
      });
      // Re-apply mobile simplification when crossing the breakpoint.
      const onChartMobileChange = () => {
        syncChartRangeUI();
        loadHistory();
      };
      if (typeof CHART_MOBILE_MQ.addEventListener === "function") {
        CHART_MOBILE_MQ.addEventListener("change", onChartMobileChange);
      } else if (typeof CHART_MOBILE_MQ.addListener === "function") {
        CHART_MOBILE_MQ.addListener(onChartMobileChange);
      }
    }
    if (els.chartFrom) {
      els.chartFrom.addEventListener("change", () => {
        state.chartFrom = els.chartFrom.value;
        saveChartCustom();
        if (state.chartRange === "custom" && !isChartMobile()) loadHistory();
      });
    }
    if (els.chartTo) {
      els.chartTo.addEventListener("change", () => {
        state.chartTo = els.chartTo.value;
        saveChartCustom();
        if (state.chartRange === "custom" && !isChartMobile()) loadHistory();
      });
    }

    els.table.querySelector("thead").addEventListener("click", (ev) => {
      const th = ev.target.closest("th[data-sort]");
      if (!th) return;
      const key = th.dataset.sort;
      if (state.sortKey === key) state.sortDir *= -1;
      else {
        state.sortKey = key;
        const texty = new Set([
          "ip", "hostname", "make", "model", "firmware", "firmware_version",
          "algo", "mac", "serial", "error", "pool_user", "pool_host",
        ]);
        state.sortDir = texty.has(key) ? 1 : -1;
      }
      saveSort();
      renderTable();
    });

    els.tbody.addEventListener("click", (ev) => {
      const tr = ev.target.closest("tr[data-id]");
      if (!tr) return;
      state.selectedId = tr.dataset.id;
      renderTable();
      loadDetail(state.selectedId);
    });

    if (els.btnColumns && els.columnsPanel) {
      els.btnColumns.addEventListener("click", (ev) => {
        ev.stopPropagation();
        setColumnsPanelOpen(els.columnsPanel.hidden);
      });
      els.columnsPanel.addEventListener("click", (ev) => ev.stopPropagation());
      document.addEventListener("click", () => setColumnsPanelOpen(false));
      document.addEventListener("keydown", (ev) => {
        if (ev.key === "Escape") setColumnsPanelOpen(false);
      });
      if (els.btnColumnsAll) {
        els.btnColumnsAll.addEventListener("click", () => {
          setVisibleColumns(COLUMNS.map((c) => c.id));
        });
      }
      if (els.btnColumnsDefault) {
        els.btnColumnsDefault.addEventListener("click", () => {
          setVisibleColumns(defaultColumnIds());
        });
      }
      renderColumnsPanel();
    }

    bindFiltersResizer();

    window.addEventListener("resize", () => {
      // Keep stored width within viewport-friendly bounds on window resize.
      if (state.filtersWidth) applyFiltersWidth(state.filtersWidth, { save: false });
      drawChart();
    });
  }

  function bindFiltersResizer() {
    const handle = els.filtersResizer;
    if (!handle || !els.dashGrid) return;

    applyFiltersWidth(state.filtersWidth, { save: false });

    let dragging = false;
    let dragPointerId = null;

    const onMove = (clientX) => {
      if (!dragging) return;
      // Measure from the left column's left edge so padding/gaps don't skew width.
      const left = (els.colLeft || els.dashGrid).getBoundingClientRect().left;
      applyFiltersWidth(clientX - left, { save: false });
      // Avoid thrashing the chart on every pixel; resize observer / end will redraw.
    };

    const endDrag = (ev) => {
      if (!dragging) return;
      if (ev && dragPointerId != null && ev.pointerId != null && ev.pointerId !== dragPointerId) {
        return;
      }
      dragging = false;
      dragPointerId = null;
      els.dashGrid.classList.remove("resizing");
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      applyFiltersWidth(state.filtersWidth, { save: true });
      drawChart();
    };

    handle.addEventListener("pointerdown", (ev) => {
      // Only primary button / touch.
      if (ev.button != null && ev.button !== 0) return;
      dragging = true;
      dragPointerId = ev.pointerId;
      els.dashGrid.classList.add("resizing");
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
      try {
        handle.setPointerCapture(ev.pointerId);
      } catch {
        /* ignore */
      }
      ev.preventDefault();
      onMove(ev.clientX);
    });

    // Listen on the handle (with capture) and document so drag keeps working
    // even if the pointer briefly leaves the thin hit target.
    const moveHandler = (ev) => {
      if (!dragging) return;
      if (dragPointerId != null && ev.pointerId != null && ev.pointerId !== dragPointerId) return;
      onMove(ev.clientX);
    };
    handle.addEventListener("pointermove", moveHandler);
    document.addEventListener("pointermove", moveHandler);
    handle.addEventListener("pointerup", endDrag);
    handle.addEventListener("pointercancel", endDrag);
    document.addEventListener("pointerup", endDrag);
    document.addEventListener("pointercancel", endDrag);

    // Double-click restores default width.
    handle.addEventListener("dblclick", () => {
      applyFiltersWidth(FILTERS_WIDTH_DEFAULT, { save: true });
      drawChart();
    });

    // Keyboard: arrows nudge width; Home/End jump to min/max.
    handle.addEventListener("keydown", (ev) => {
      const step = ev.shiftKey ? 40 : 16;
      let next = state.filtersWidth;
      if (ev.key === "ArrowLeft") next -= step;
      else if (ev.key === "ArrowRight") next += step;
      else if (ev.key === "Home") next = FILTERS_WIDTH_MIN;
      else if (ev.key === "End") next = maxFiltersWidthForViewport();
      else return;
      ev.preventDefault();
      applyFiltersWidth(next, { save: true });
      drawChart();
    });
  }

  bind();
  renderTableHeader();
  refresh();
  schedule();
})();
