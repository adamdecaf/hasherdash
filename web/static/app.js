/* minerdash frontend — vanilla JS + canvas charts */

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

  function defaultColumnIds() {
    return COLUMNS.filter((c) => c.defaultOn || c.locked).map((c) => c.id);
  }

  function clampFiltersWidth(px) {
    const n = Math.round(Number(px));
    if (!Number.isFinite(n)) return FILTERS_WIDTH_DEFAULT;
    return Math.min(FILTERS_WIDTH_MAX, Math.max(FILTERS_WIDTH_MIN, n));
  }

  function loadFiltersWidth() {
    const raw = parseInt(localStorage.getItem(FILTERS_WIDTH_LS_KEY), 10);
    if (Number.isFinite(raw)) return clampFiltersWidth(raw);
    return FILTERS_WIDTH_DEFAULT;
  }

  function applyFiltersWidth(px, { save = true } = {}) {
    const w = clampFiltersWidth(px);
    document.documentElement.style.setProperty("--filters-width", w + "px");
    if (els.filtersResizer) {
      els.filtersResizer.setAttribute("aria-valuenow", String(w));
      els.filtersResizer.setAttribute("aria-valuemin", String(FILTERS_WIDTH_MIN));
      els.filtersResizer.setAttribute("aria-valuemax", String(FILTERS_WIDTH_MAX));
    }
    if (save) localStorage.setItem(FILTERS_WIDTH_LS_KEY, String(w));
    state.filtersWidth = w;
    return w;
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

  const state = {
    miners: [],
    meta: null,
    selectedId: null,
    detail: null,
    history: [],
    hiddenSeries: new Set(),
    sortKey: "ip",
    sortDir: 1,
    visibleColumns: loadVisibleColumns(),
    filtersWidth: loadFiltersWidth(),
    refreshSec: Number(localStorage.getItem("refreshSec") || 30),
    timer: null,
  };

  const $ = (id) => document.getElementById(id);
  const els = {
    status: $("status-pill"),
    pollMeta: $("poll-meta"),
    pollInterval: $("poll-interval"),
    btnRefresh: $("btn-refresh"),
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
    chartScope: $("chart-scope"),
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

  async function api(path) {
    const r = await fetch(path, { headers: { Accept: "application/json" } });
    if (!r.ok) throw new Error(`${path}: ${r.status}`);
    return r.json();
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

    let tMin = Infinity, tMax = -Infinity, vMin = Infinity, vMax = -Infinity;
    for (const s of series) {
      for (const p of s.points) {
        const t = new Date(p.t).getTime();
        if (t < tMin) tMin = t;
        if (t > tMax) tMax = t;
        if (p.v < vMin) vMin = p.v;
        if (p.v > vMax) vMax = p.v;
      }
    }
    if (tMin === tMax) tMax = tMin + 1;
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
    // x labels
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    const xTicks = 5;
    for (let i = 0; i <= xTicks; i++) {
      const t = tMin + ((tMax - tMin) * i) / xTicks;
      const x = xOf(t);
      ctx.fillText(new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" }), x, h - pad.b + 6);
    }
    ctx.globalAlpha = 1;

    // lines
    series.forEach((s, idx) => {
      const color = seriesColor(state.history.indexOf(s) >= 0 ? state.history.findIndex((x) => x.id === s.id) : idx);
      ctx.strokeStyle = color;
      ctx.lineWidth = 1.6;
      ctx.beginPath();
      s.points.forEach((p, i) => {
        const x = xOf(new Date(p.t).getTime());
        const y = yOf(p.v);
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.stroke();
    });

    renderLegend();
  }

  function renderLegend() {
    const frag = document.createDocumentFragment();
    state.history.forEach((s, i) => {
      const el = document.createElement("span");
      el.className = "item" + (state.hiddenSeries.has(s.id) ? " off" : "");
      el.dataset.id = s.id;
      el.innerHTML = `<span class="swatch" style="background:${seriesColor(i)}"></span>${esc(s.label || s.id)}`;
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

  async function loadHistory() {
    const metric = els.chartMetric.value;
    const scope = els.chartScope.value;

    // Full fleet history (no ids) supplies brand/model facets for filters.
    let allSeries = [];
    try {
      allSeries = await api(`/api/history?metric=${encodeURIComponent(metric)}`);
    } catch {
      allSeries = [];
    }
    fillFiltersFromHistory(allSeries);

    let chartSeries = allSeries;
    if (scope === "selected" && state.selectedId) {
      chartSeries = allSeries.filter((s) => s.id === state.selectedId);
    } else if (scope === "filtered") {
      let ids = applyFilters(state.miners).map((m) => m.id);
      if (ids.length > 40) ids = ids.slice(0, 40);
      const want = new Set(ids);
      chartSeries = allSeries.filter((s) => want.has(s.id));
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

    const filterInputs = [
      els.search, els.make, els.model, els.firmware, els.algo, els.mining, els.errors,
      els.hrMin, els.hrMax, els.tempMin, els.tempMax, els.vrMin, els.vrMax,
      els.wMin, els.wMax, els.chipsMin, els.chipsMax, els.effMin, els.effMax,
    ].filter(Boolean);
    for (const el of filterInputs) {
      el.addEventListener("input", () => {
        renderTable();
        if (els.chartScope.value === "filtered") loadHistory();
      });
      el.addEventListener("change", () => {
        renderTable();
        if (els.chartScope.value === "filtered") loadHistory();
      });
    }

    els.chartMetric.addEventListener("change", () => loadHistory());
    els.chartScope.addEventListener("change", () => loadHistory());

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
      renderTable();
    });

    els.tbody.addEventListener("click", (ev) => {
      const tr = ev.target.closest("tr[data-id]");
      if (!tr) return;
      state.selectedId = tr.dataset.id;
      renderTable();
      loadDetail(state.selectedId);
      if (els.chartScope.value === "selected") loadHistory();
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

    const onMove = (clientX) => {
      if (!dragging) return;
      const gridLeft = els.dashGrid.getBoundingClientRect().left;
      // Width is distance from grid left edge to the drag handle.
      applyFiltersWidth(clientX - gridLeft, { save: false });
      drawChart();
    };

    const endDrag = () => {
      if (!dragging) return;
      dragging = false;
      els.dashGrid.classList.remove("resizing");
      document.body.style.cursor = "";
      applyFiltersWidth(state.filtersWidth, { save: true });
      drawChart();
    };

    handle.addEventListener("pointerdown", (ev) => {
      // Only primary button / touch.
      if (ev.button != null && ev.button !== 0) return;
      dragging = true;
      els.dashGrid.classList.add("resizing");
      document.body.style.cursor = "col-resize";
      handle.setPointerCapture?.(ev.pointerId);
      ev.preventDefault();
    });

    handle.addEventListener("pointermove", (ev) => {
      if (!dragging) return;
      onMove(ev.clientX);
    });

    handle.addEventListener("pointerup", endDrag);
    handle.addEventListener("pointercancel", endDrag);
    handle.addEventListener("lostpointercapture", endDrag);

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
      else if (ev.key === "End") next = FILTERS_WIDTH_MAX;
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
