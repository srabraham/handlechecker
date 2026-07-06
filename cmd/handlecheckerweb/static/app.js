// @ts-check
"use strict";

// This file is plain JavaScript that ships to the browser verbatim (embedded by
// the Go server) — there is no build step. The `// @ts-check` directive above
// plus jsconfig.json let TypeScript type-check it in the editor and in CI
// (`tsc --noEmit`) via JSDoc annotations, so we get TypeScript's safety while
// still serving hand-authored JS. Keep the JSDoc typedefs below in sync with the
// Go DTOs in main.go (issueDTO / the /api/check response).

/**
 * One reviewed decision, retained so it can be undone.
 * @typedef {Object} HistoryEntry
 * @property {"approve"|"reject"} action
 * @property {string} candidate
 */

/**
 * The whole app's persisted state (mirrored to localStorage).
 * @typedef {Object} State
 * @property {string[]} reserved
 * @property {string[]} existing   Baseline already-approved handles (never grows during review).
 * @property {string[]} proposed   The review queue.
 * @property {number} queueIndex
 * @property {string[]} approved   Candidates approved this session.
 * @property {string[]} rejected   Candidates rejected this session.
 * @property {HistoryEntry[]} history
 */

/**
 * A single finding from /api/check — mirrors issueDTO in main.go.
 * @typedef {Object} Issue
 * @property {string} severity      "HIGH", "MEDIUM", …
 * @property {number} severityRank
 * @property {string} kind
 * @property {string} detail
 * @property {string} b             Conflicting baseline term ("" for self checks).
 * @property {string} source        "reserved" | "existing" | "self".
 */

/**
 * The /api/check response — mirrors the response struct in main.go.
 * @typedef {Object} CheckResult
 * @property {string} candidate
 * @property {Issue[]} issues
 * @property {string} worst       Severity string of the worst issue, "" if none.
 * @property {number} worstRank   -1 if no issues.
 */

const STORAGE_KEY = "handlechecker.state.v1";

// One-shot flag: set the first time we auto-load the default Reserved handles
// into the setup textarea, so we never do it again. Its presence — not the
// textarea's contents — is what gates the auto-load, so once the user has been
// offered the defaults they can clear the field and it stays cleared. A full
// Reset clears the flag, so reset returns the app to a first-visit state.
const RESERVED_DEFAULTS_KEY = "handlechecker.reservedDefaultsLoaded.v1";

// The server-provided default Reserved handles, fetched once at startup. Empty
// string when none are configured (or the fetch failed).
let reservedDefaultsText = "";

// Application state. `existing` is the fixed baseline of already-approved
// handles; session approvals accumulate in `approved` (not `existing`) but are
// still checked against, so later candidates are vetted against earlier ones.
/** @type {State} */
let state = {
  reserved: [],
  existing: [],   // baseline already-approved handles (fixed during review)
  proposed: [],   // the review queue
  queueIndex: 0,
  approved: [],   // candidates approved this session
  rejected: [],   // candidates rejected this session
  history: [],    // decision log, for Undo: {action, candidate}
};

// --- DOM helpers -------------------------------------------------------------

// `$` is intentionally typed loosely (any): elements are accessed for many
// different concrete types (.value on textareas, .disabled on buttons, etc.) and
// per-call casts would drown the file in noise. The real type safety lives in
// the State/Issue/CheckResult typedefs and the function signatures below.
/** @type {(id: string) => any} */
const $ = (id) => document.getElementById(id);
const sections = { setup: $("setup"), review: $("review"), summary: $("summary") };

/** @param {string} name */
function show(name) {
  for (const [k, el] of Object.entries(sections)) {
    el.classList.toggle("hidden", k !== name);
  }
  // The propose card and the persistence notice are peers of setup and share
  // its phase — they only appear on the initial page.
  $("propose").classList.toggle("hidden", name !== "setup");
  $("notice").classList.toggle("hidden", name !== "setup");
}

// parseList splits raw textarea input into trimmed, de-duplicated terms, one
// per line. Handles may contain spaces and commas, so newlines are the only
// separator. Duplicates are removed case-insensitively, keeping the first
// spelling seen.
/**
 * @param {string} raw
 * @returns {string[]}
 */
function parseList(raw) {
  const seen = new Set();
  const out = [];
  for (const tok of raw.split("\n")) {
    const t = tok.trim();
    if (!t) continue;
    const key = t.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(t);
  }
  return out;
}

/**
 * @param {string[]} list
 * @param {string} term
 * @returns {boolean}
 */
function hasCI(list, term) {
  const key = term.toLowerCase();
  return list.some((x) => x.toLowerCase() === key);
}

// removeCI removes the first case-insensitive match of term from list in place.
/**
 * @param {string[]} list
 * @param {string} term
 */
function removeCI(list, term) {
  const key = term.toLowerCase();
  const i = list.findIndex((x) => x.toLowerCase() === key);
  if (i >= 0) list.splice(i, 1);
}

// confirmDialog shows the in-app modal and resolves true if the user confirms,
// false if they cancel or dismiss it (Cancel button, Escape, or backdrop click).
// It replaces window.confirm(), which browsers suppress when the page/tab isn't
// focused — leaving the confirm-guarded action to silently no-op.
/**
 * @param {{title: string, message: string, confirmLabel?: string, danger?: boolean}} opts
 * @returns {Promise<boolean>}
 */
function confirmDialog({ title, message, confirmLabel = "Confirm", danger = false }) {
  /** @type {HTMLDialogElement} */
  const dlg = $("confirmModal");
  const ok = $("confirmOk");
  const cancel = $("confirmCancel");
  $("confirmTitle").textContent = title;
  $("confirmMessage").textContent = message;
  ok.textContent = confirmLabel;
  ok.className = danger ? "danger" : "primary";

  return new Promise((resolve) => {
    const onOk = () => dlg.close("ok");
    const onCancel = () => dlg.close("cancel");
    // A click whose target is the dialog itself landed on the backdrop, not the
    // content — treat it as a dismiss.
    const onBackdrop = (/** @type {MouseEvent} */ e) => { if (e.target === dlg) dlg.close("cancel"); };
    const onClose = () => {
      ok.removeEventListener("click", onOk);
      cancel.removeEventListener("click", onCancel);
      dlg.removeEventListener("click", onBackdrop);
      dlg.removeEventListener("close", onClose);
      resolve(dlg.returnValue === "ok"); // "" on Escape ⇒ false
    };
    ok.addEventListener("click", onOk);
    cancel.addEventListener("click", onCancel);
    dlg.addEventListener("click", onBackdrop);
    dlg.addEventListener("close", onClose);
    dlg.returnValue = ""; // reset so a stale value can't read as confirmed
    dlg.showModal();
    ok.focus();
  });
}

// --- persistence -------------------------------------------------------------

function save() {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

/** @returns {boolean} True if a resumable in-progress session was loaded. */
function load() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return false;
    const s = JSON.parse(raw);
    if (!s || !Array.isArray(s.proposed)) return false;
    state = Object.assign(state, s);
    return state.proposed.length > 0;
  } catch {
    return false;
  }
}

// --- setup phase -------------------------------------------------------------

function startReview() {
  state.reserved = parseList($("reserved").value);
  state.existing = parseList($("existing").value);
  state.proposed = parseList($("proposed").value);
  state.queueIndex = 0;
  state.approved = [];
  state.rejected = [];
  state.history = [];

  if (state.proposed.length === 0) {
    $("setupSummary").textContent = "Enter at least one proposed handle to review.";
    return;
  }
  save();
  show("review");
  showCurrent();
}

function backToSetup() {
  // Repopulate the textareas from current state so edits don't lose data.
  $("reserved").value = state.reserved.join("\n");
  $("existing").value = state.existing.join("\n");
  $("proposed").value = state.proposed.join("\n");
  show("setup");
}

// setupReservedDefaults wires up the "Load defaults" button beside the Reserved
// handles textarea, and on the user's first visit auto-fills the textarea with
// those defaults. The defaults come from the server (GET /api/defaults/reserved),
// which only returns them when an operator has supplied a defaults file;
// otherwise it 404s and we leave the button hidden. The fetched text is cached so
// the click fills the textarea instantly.
/** @returns {Promise<void>} */
async function setupReservedDefaults() {
  try {
    const resp = await fetch("/api/defaults/reserved");
    if (!resp.ok) return; // 404 = no defaults configured; keep the button hidden
    reservedDefaultsText = await resp.text();
  } catch {
    return; // network/server error: silently leave the button hidden
  }
  const btn = $("loadReservedDefaults");
  btn.classList.remove("hidden");
  btn.addEventListener("click", async () => {
    const ta = $("reserved");
    if (ta.value.trim() && !(await confirmDialog({
      title: "Replace reserved handles?",
      message: "This replaces the current Reserved handles with the defaults.",
      confirmLabel: "Replace",
    }))) return;
    ta.value = reservedDefaultsText;
  });

  maybePrefillReservedDefaults();
}

// maybePrefillReservedDefaults fills the Reserved textarea with the server
// defaults on the user's first visit, so they start from the standard baseline
// instead of a blank field. It runs once ever (guarded by RESERVED_DEFAULTS_KEY,
// which it sets): if the user then clears the field, it stays cleared. A full
// Reset clears the flag, so reset returns to this first-visit prefill. The
// empty-field check avoids clobbering handles already entered/restored, and the
// no-op when defaults are unconfigured leaves the flag unset so a later-added
// defaults file still prefills. Safe to call before the fetch resolves.
function maybePrefillReservedDefaults() {
  if (!reservedDefaultsText) return;
  if (localStorage.getItem(RESERVED_DEFAULTS_KEY)) return;
  if ($("reserved").value.trim()) return;
  localStorage.setItem(RESERVED_DEFAULTS_KEY, "1");
  $("reserved").value = reservedDefaultsText;
}

// --- review phase ------------------------------------------------------------

/** @returns {string} */
function currentCandidate() {
  return state.proposed[state.queueIndex];
}

/** @returns {Promise<void>} */
async function showCurrent() {
  updateTallies();
  if (state.queueIndex >= state.proposed.length) {
    showSummary();
    return;
  }
  const candidate = currentCandidate();
  $("progress").textContent = `Reviewing ${state.queueIndex + 1} of ${state.proposed.length}`;
  $("candidate").textContent = candidate;
  $("banner").className = "banner";
  $("banner").textContent = "Checking…";
  $("issues").innerHTML = "";
  setReviewButtons(false);

  try {
    const resp = await fetch("/api/check", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        candidate,
        reserved: state.reserved,
        // Check against the fixed baseline plus everything approved so far this
        // session, so each handle is vetted against earlier approvals too. Both
        // map to the "approved" source badge, which is accurate for either.
        existing: state.existing.concat(state.approved),
      }),
    });
    if (!resp.ok) throw new Error(await resp.text());
    const data = await resp.json();
    renderResult(data);
  } catch (err) {
    $("banner").className = "banner high";
    $("banner").textContent =
      "Error checking handle: " + (err instanceof Error ? err.message : String(err));
  } finally {
    setReviewButtons(true);
  }
}

/** @param {boolean} enabled */
function setReviewButtons(enabled) {
  for (const id of ["approve", "reject"]) $(id).disabled = !enabled;
  // Undo is available whenever there's a decision to undo and we aren't mid-check.
  $("undo").disabled = !enabled || state.history.length === 0;
}

/** @param {CheckResult} data */
function renderResult(data) {
  // Recommendation banner keyed off the worst severity.
  const banner = $("banner");
  const rank = data.worstRank;
  if (rank < 0) {
    banner.className = "banner ok";
    banner.textContent = "✓ No conflicts found — safe to approve.";
  } else if (rank >= 3) { // HIGH or CRITICAL
    banner.className = "banner high";
    banner.textContent = `⚠ Has ${data.worst} conflicts — review carefully before approving.`;
  } else if (rank === 2) { // MEDIUM
    banner.className = "banner med";
    banner.textContent = "⚠ Has MEDIUM conflicts — check whether they're acceptable.";
  } else { // LOW / INFO
    banner.className = "banner low";
    banner.textContent = "Minor conflicts only — likely fine.";
  }

  const box = $("issues");
  box.innerHTML = "";
  for (const is of data.issues) {
    const row = document.createElement("div");
    row.className = "issue sev-" + is.severity;

    const tag = document.createElement("span");
    tag.className = "sev-tag";
    tag.textContent = is.severity;
    row.appendChild(tag);

    const detail = document.createElement("div");
    detail.className = "detail";
    if (is.b) {
      detail.innerHTML =
        `<b>${esc(data.candidate)}</b> <span class="vs">/</span> <b>${esc(is.b)}</b>` +
        sourceBadge(is.source) +
        ` — ${esc(is.detail)}`;
    } else {
      detail.innerHTML = `<b>${esc(data.candidate)}</b> — ${esc(is.detail)}`;
    }
    row.appendChild(detail);
    box.appendChild(row);
  }
}

/**
 * @param {string} source
 * @returns {string}
 */
function sourceBadge(source) {
  if (source === "reserved") return ` <span class="source-badge reserved">reserved</span>`;
  if (source === "existing") return ` <span class="source-badge existing">approved</span>`;
  return "";
}

/**
 * @param {string} s
 * @returns {string}
 */
function esc(s) {
  /** @type {Record<string, string>} */
  const map = { "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" };
  return String(s).replace(/[&<>"']/g, (c) => map[c]);
}

function approve() {
  const c = currentCandidate();
  if (!hasCI(state.approved, c)) state.approved.push(c);
  state.history.push({ action: "approve", candidate: c });
  advance();
}

function reject() {
  const c = currentCandidate();
  if (!hasCI(state.rejected, c)) state.rejected.push(c);
  state.history.push({ action: "reject", candidate: c });
  advance();
}

// undo reverses the most recent decision: it removes the candidate from the
// approved/rejected lists, steps the queue back to that handle, and re-checks
// it. Reachable from both the review pane and the summary, so the last decision
// can always be corrected.
function undo() {
  const last = state.history.pop();
  if (!last) return;
  removeCI(state.approved, last.candidate);
  removeCI(state.rejected, last.candidate);
  if (state.queueIndex > 0) state.queueIndex--;
  save();
  show("review");
  showCurrent();
}

function advance() {
  state.queueIndex++;
  save();
  showCurrent();
}

function updateTallies() {
  $("approvedCount").textContent = state.approved.length;
  $("rejectedCount").textContent = state.rejected.length;
}

// --- summary phase -----------------------------------------------------------

function showSummary() {
  $("summaryLine").textContent =
    `${state.approved.length} approved, ${state.rejected.length} rejected, ` +
    `${state.proposed.length} reviewed.`;
  $("approvedList").value = state.approved.join("\n");
  $("rejectedList").value = state.rejected.join("\n");
  $("fullList").value = state.existing.concat(state.approved).join("\n");
  $("undoSummary").disabled = state.history.length === 0;
  show("summary");
}

/**
 * @param {string} filename
 * @param {string} text
 */
function download(filename, text) {
  const blob = new Blob([text], { type: "text/plain" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/**
 * @param {string} text
 * @param {HTMLElement} btn
 * @returns {Promise<void>}
 */
async function copyText(text, btn) {
  try {
    await navigator.clipboard.writeText(text);
    const old = btn.textContent;
    btn.textContent = "Copied!";
    setTimeout(() => (btn.textContent = old), 1200);
  } catch {
    /* clipboard may be unavailable on insecure origins; ignore */
  }
}

async function reset() {
  if (!(await confirmDialog({
    title: "Reset everything?",
    message: "This clears all lists and starts over. This can't be undone.",
    confirmLabel: "Reset everything",
    danger: true,
  }))) return;
  localStorage.removeItem(STORAGE_KEY);
  // Also clear the one-shot flag so reset returns to a first-visit state,
  // re-prefilling the Reserved defaults below.
  localStorage.removeItem(RESERVED_DEFAULTS_KEY);
  state = { reserved: [], existing: [], proposed: [], queueIndex: 0, approved: [], rejected: [], history: [] };
  $("reserved").value = "";
  $("existing").value = "";
  $("proposed").value = "";
  $("setupSummary").textContent = "";
  maybePrefillReservedDefaults();
  show("setup");
}

// --- wiring ------------------------------------------------------------------

function init() {
  $("start").addEventListener("click", startReview);
  $("backToSetup").addEventListener("click", backToSetup);
  $("approve").addEventListener("click", approve);
  $("reject").addEventListener("click", reject);
  $("undo").addEventListener("click", undo);
  $("undoSummary").addEventListener("click", undo);
  $("finish").addEventListener("click", showSummary);
  $("backToStart").addEventListener("click", backToSetup);
  $("reset").addEventListener("click", reset);
  $("resetSetup").addEventListener("click", reset);

  $("copyApproved").addEventListener("click", (/** @type {Event} */ e) => copyText(state.approved.join("\n"), /** @type {HTMLElement} */ (e.target)));
  $("copyRejected").addEventListener("click", (/** @type {Event} */ e) => copyText(state.rejected.join("\n"), /** @type {HTMLElement} */ (e.target)));
  $("copyFull").addEventListener("click", (/** @type {Event} */ e) => copyText(state.existing.concat(state.approved).join("\n"), /** @type {HTMLElement} */ (e.target)));
  $("downloadApproved").addEventListener("click", () => download("approved-handles.txt", state.approved.join("\n")));
  $("downloadRejected").addEventListener("click", () => download("rejected-handles.txt", state.rejected.join("\n")));
  $("downloadFull").addEventListener("click", () => download("all-handles.txt", state.existing.concat(state.approved).join("\n")));

  // Offer server-provided default reserved handles, if any are configured.
  setupReservedDefaults();

  // Resume an in-progress session if one was saved.
  if (load()) {
    if (state.queueIndex >= state.proposed.length) {
      showSummary();
    } else {
      show("review");
      showCurrent();
    }
  } else {
    show("setup");
  }
}

document.addEventListener("DOMContentLoaded", init);
