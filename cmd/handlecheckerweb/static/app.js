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
 * @property {boolean} wasNewExisting  Whether approving added the handle to `existing`.
 */

/**
 * The whole app's persisted state (mirrored to localStorage).
 * @typedef {Object} State
 * @property {string[]} reserved
 * @property {string[]} existing   Baseline existing handles + approvals so far.
 * @property {string[]} proposed   The review queue.
 * @property {number} queueIndex
 * @property {string[]} approved   Candidates approved this session (subset of `existing`).
 * @property {string[]} rejected
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

// Application state. `existing` grows as handles are approved, so later
// candidates are checked against earlier approvals.
/** @type {State} */
let state = {
  reserved: [],
  existing: [],   // baseline existing handles + approvals so far
  proposed: [],   // the review queue
  queueIndex: 0,
  approved: [],   // candidates approved this session (subset of existing)
  rejected: [],
  history: [],    // decision log, for Undo: {action, candidate, wasNewExisting}
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
  // The propose card is a peer of setup and shares its phase.
  $("propose").classList.toggle("hidden", name !== "setup");
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
        existing: state.existing,
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
  const wasNewExisting = !hasCI(state.existing, c);
  if (wasNewExisting) state.existing.push(c);
  if (!hasCI(state.approved, c)) state.approved.push(c);
  state.history.push({ action: "approve", candidate: c, wasNewExisting });
  advance();
}

function reject() {
  const c = currentCandidate();
  if (!hasCI(state.rejected, c)) state.rejected.push(c);
  state.history.push({ action: "reject", candidate: c, wasNewExisting: false });
  advance();
}

// undo reverses the most recent decision: it removes the candidate from the
// approved/rejected lists (and from `existing` if approving had added it there),
// steps the queue back to that handle, and re-checks it. Reachable from both the
// review pane and the summary, so the last decision can always be corrected.
function undo() {
  const last = state.history.pop();
  if (!last) return;
  removeCI(state.approved, last.candidate);
  removeCI(state.rejected, last.candidate);
  if (last.action === "approve" && last.wasNewExisting) {
    removeCI(state.existing, last.candidate);
  }
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
  $("fullList").value = state.existing.join("\n");
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

function reset() {
  if (!confirm("Clear all lists and start over?")) return;
  localStorage.removeItem(STORAGE_KEY);
  state = { reserved: [], existing: [], proposed: [], queueIndex: 0, approved: [], rejected: [], history: [] };
  $("reserved").value = "";
  $("existing").value = "";
  $("proposed").value = "";
  $("setupSummary").textContent = "";
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
  $("copyFull").addEventListener("click", (/** @type {Event} */ e) => copyText(state.existing.join("\n"), /** @type {HTMLElement} */ (e.target)));
  $("downloadApproved").addEventListener("click", () => download("approved-handles.txt", state.approved.join("\n")));
  $("downloadRejected").addEventListener("click", () => download("rejected-handles.txt", state.rejected.join("\n")));
  $("downloadFull").addEventListener("click", () => download("all-handles.txt", state.existing.join("\n")));

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
