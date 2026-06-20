"use strict";

const STORAGE_KEY = "handlechecker.state.v1";

// Application state. `existing` grows as handles are approved, so later
// candidates are checked against earlier approvals.
let state = {
  reserved: [],
  existing: [],   // baseline existing handles + approvals so far
  proposed: [],   // the review queue
  queueIndex: 0,
  approved: [],   // candidates approved this session (subset of existing)
  rejected: [],
};

// --- DOM helpers -------------------------------------------------------------

const $ = (id) => document.getElementById(id);
const sections = { setup: $("setup"), review: $("review"), summary: $("summary") };

function show(name) {
  for (const [k, el] of Object.entries(sections)) {
    el.classList.toggle("hidden", k !== name);
  }
}

// parseList splits raw textarea input into trimmed, de-duplicated terms, one
// per line. Handles may contain spaces and commas, so newlines are the only
// separator. Duplicates are removed case-insensitively, keeping the first
// spelling seen.
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

function hasCI(list, term) {
  const key = term.toLowerCase();
  return list.some((x) => x.toLowerCase() === key);
}

// --- persistence -------------------------------------------------------------

function save() {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

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

function currentCandidate() {
  return state.proposed[state.queueIndex];
}

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
    $("banner").textContent = "Error checking handle: " + err.message;
  } finally {
    setReviewButtons(true);
  }
}

function setReviewButtons(enabled) {
  for (const id of ["approve", "reject", "skip"]) $(id).disabled = !enabled;
}

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

function sourceBadge(source) {
  if (source === "reserved") return ` <span class="source-badge reserved">reserved</span>`;
  if (source === "existing") return ` <span class="source-badge existing">existing</span>`;
  return "";
}

function esc(s) {
  return String(s).replace(/[&<>"']/g, (c) =>
    ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function approve() {
  const c = currentCandidate();
  if (!hasCI(state.existing, c)) state.existing.push(c);
  if (!hasCI(state.approved, c)) state.approved.push(c);
  advance();
}

function reject() {
  const c = currentCandidate();
  if (!hasCI(state.rejected, c)) state.rejected.push(c);
  advance();
}

function skip() {
  advance();
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

// resumeReview returns to the review pane from the summary. If the queue was
// fully reviewed, step back to the last handle so the pane has something to
// show (otherwise showCurrent would bounce straight back to the summary).
function resumeReview() {
  if (state.proposed.length === 0) return;
  if (state.queueIndex >= state.proposed.length) {
    state.queueIndex = state.proposed.length - 1;
    save();
  }
  show("review");
  showCurrent();
}

function showSummary() {
  $("summaryLine").textContent =
    `${state.approved.length} approved, ${state.rejected.length} rejected, ` +
    `${state.proposed.length} reviewed.`;
  $("approvedList").value = state.approved.join("\n");
  $("fullList").value = state.existing.join("\n");
  show("summary");
}

function download(filename, text) {
  const blob = new Blob([text], { type: "text/plain" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

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
  state = { reserved: [], existing: [], proposed: [], queueIndex: 0, approved: [], rejected: [] };
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
  $("skip").addEventListener("click", skip);
  $("finish").addEventListener("click", showSummary);
  $("resumeReview").addEventListener("click", resumeReview);
  $("reset").addEventListener("click", reset);

  $("copyApproved").addEventListener("click", (e) => copyText(state.approved.join("\n"), e.target));
  $("copyFull").addEventListener("click", (e) => copyText(state.existing.join("\n"), e.target));
  $("downloadApproved").addEventListener("click", () => download("approved-handles.txt", state.approved.join("\n")));
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
