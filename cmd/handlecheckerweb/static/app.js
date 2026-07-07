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
 * @property {"approve"|"reject"|"skip"} action
 * @property {number} slot        Index into state.slots.
 * @property {string} candidate   The handle decided ("" for skip).
 */

/**
 * One person's ranked candidates and review outcome. `candidates[tried]` is
 * the attempt currently under review; `tried` counts rejections, so the slot
 * is exhausted when tried reaches candidates.length without an approval.
 * @typedef {Object} Slot
 * @property {string[]} candidates  Ranked handles, first pick first; ad-hoc backups append.
 * @property {number} tried         How many of candidates have been rejected.
 * @property {string} approved      The winning handle ("" while none).
 * @property {boolean} skipped      True when the user gave up on this person.
 */

/**
 * The whole app's persisted state (mirrored to localStorage).
 * @typedef {Object} State
 * @property {string[]} reserved
 * @property {string[]} existing   Baseline already-approved handles (never grows during review).
 * @property {Slot[]} slots        The review queue: one slot per person.
 * @property {number} slotIndex
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
  slots: [],      // the review queue: one slot per person
  slotIndex: 0,
  approved: [],   // candidates approved this session
  rejected: [],   // candidates rejected this session
  history: [],    // decision log, for Undo: {action, slot, candidate}
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

// parseSlots splits the proposed-handles textarea into review slots: one line
// per person, holding that person's ranked candidates (first pick first)
// separated by commas or tabs — so a pasted CSV row or spreadsheet row works
// as-is, and a plain one-per-line list yields one-candidate slots. Candidates
// are de-duplicated case-insensitively within a line only; the same handle on
// two people's lines is legitimate contention and the check itself will flag
// the loser once the winner is approved.
/**
 * @param {string} raw
 * @returns {Slot[]}
 */
function parseSlots(raw) {
  const out = [];
  for (const line of raw.split("\n")) {
    const seen = new Set();
    /** @type {string[]} */
    const candidates = [];
    for (const tok of line.split(/[,\t]/)) {
      const t = tok.trim();
      if (!t) continue;
      const key = t.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      candidates.push(t);
    }
    if (candidates.length > 0) {
      out.push({ candidates, tried: 0, approved: "", skipped: false });
    }
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
    let s = JSON.parse(raw);
    if (s && Array.isArray(s.proposed)) s = migrateV1(s);
    if (!s || !Array.isArray(s.slots)) return false;
    state = Object.assign(state, s);
    return state.slots.length > 0;
  } catch {
    return false;
  }
}

// migrateV1 converts a persisted state from before the per-person slot model
// (a flat `proposed` queue plus `queueIndex`) into the current shape: each
// proposed handle becomes a one-candidate slot, with handles the user already
// decided marked from the approved/rejected lists (a v1 rejection resolved its
// person, hence skipped). History entries gain their slot index — v1 queues
// were de-duplicated, so a candidate names its slot uniquely.
/**
 * @param {any} s
 * @returns {State|null}
 */
function migrateV1(s) {
  /** @type {string[]} */
  const proposed = s.proposed;
  const queueIndex = typeof s.queueIndex === "number" ? s.queueIndex : 0;
  const approved = Array.isArray(s.approved) ? s.approved : [];
  const rejected = Array.isArray(s.rejected) ? s.rejected : [];
  const slots = proposed.map((c, i) => {
    const wasRejected = i < queueIndex && hasCI(rejected, c);
    return {
      candidates: [c],
      tried: wasRejected ? 1 : 0,
      approved: i < queueIndex && hasCI(approved, c) ? c : "",
      skipped: wasRejected,
    };
  });
  /** @type {{action: "approve"|"reject", candidate: string}[]} */
  const v1History = Array.isArray(s.history) ? s.history : [];
  const history = v1History
    .map((h) => ({
      action: h.action,
      candidate: h.candidate,
      slot: proposed.findIndex((c) => c.toLowerCase() === h.candidate.toLowerCase()),
    }))
    .filter((h) => h.slot >= 0);
  return {
    reserved: Array.isArray(s.reserved) ? s.reserved : [],
    existing: Array.isArray(s.existing) ? s.existing : [],
    slots,
    slotIndex: queueIndex,
    approved,
    rejected,
    history,
  };
}

// --- setup phase -------------------------------------------------------------

function startReview() {
  state.reserved = parseList($("reserved").value);
  state.existing = parseList($("existing").value);
  state.slots = parseSlots($("proposed").value);
  state.slotIndex = 0;
  state.approved = [];
  state.rejected = [];
  state.history = [];

  if (state.slots.length === 0) {
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
  $("proposed").value = state.slots.map((s) => s.candidates.join(", ")).join("\n");
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

/** @returns {Slot} */
function currentSlot() {
  return state.slots[state.slotIndex];
}

/** @returns {string} */
function currentCandidate() {
  const slot = currentSlot();
  return slot.candidates[slot.tried];
}

// setReviewMode switches the review card between checking a candidate
// (Approve/Reject visible) and the out-of-candidates backup prompt.
/** @param {"check"|"exhausted"} mode */
function setReviewMode(mode) {
  $("approve").classList.toggle("hidden", mode !== "check");
  $("reject").classList.toggle("hidden", mode !== "check");
  $("tryAnother").classList.toggle("hidden", mode !== "exhausted");
}

// renderAttempts shows the current person's already-rejected attempts as
// struck-through chips, so a backup is reviewed with its predecessors visible.
/** @param {Slot} slot */
function renderAttempts(slot) {
  const box = $("attempts");
  box.innerHTML = "";
  box.classList.toggle("hidden", slot.tried === 0);
  for (const c of slot.candidates.slice(0, slot.tried)) {
    const chip = document.createElement("span");
    chip.className = "attempt-chip";
    chip.textContent = c;
    box.appendChild(chip);
  }
}

// showExhausted renders the backup prompt for a person whose every candidate
// was rejected: type another handle to check, or skip them.
/** @param {Slot} slot */
function showExhausted(slot) {
  $("progress").textContent =
    `Person ${state.slotIndex + 1} of ${state.slots.length} · out of candidates`;
  $("candidate").textContent = "";
  $("banner").className = "banner med";
  $("banner").textContent = slot.candidates.length === 1
    ? "Their only candidate was rejected — try a backup, or skip this person."
    : `All ${slot.candidates.length} of their candidates were rejected — try a backup, or skip this person.`;
  $("issues").innerHTML = "";
  $("backupError").textContent = "";
  setReviewMode("exhausted");
  $("undo").disabled = state.history.length === 0;
  $("backupInput").focus();
}

/** @returns {Promise<void>} */
async function showCurrent() {
  updateTallies();
  if (state.slotIndex >= state.slots.length) {
    showSummary();
    return;
  }
  const slot = currentSlot();
  renderAttempts(slot);
  if (slot.tried >= slot.candidates.length) {
    showExhausted(slot);
    return;
  }
  const candidate = currentCandidate();
  const attempt = slot.candidates.length > 1
    ? ` · candidate ${slot.tried + 1} of ${slot.candidates.length}`
    : "";
  $("progress").textContent =
    `Person ${state.slotIndex + 1} of ${state.slots.length}${attempt}`;
  $("candidate").textContent = candidate;
  setReviewMode("check");
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
  const slot = currentSlot();
  const c = currentCandidate();
  slot.approved = c;
  if (!hasCI(state.approved, c)) state.approved.push(c);
  state.history.push({ action: "approve", slot: state.slotIndex, candidate: c });
  state.slotIndex++;
  save();
  showCurrent();
}

// reject falls through within the same person: the next call to showCurrent
// presents their next ranked candidate, or the backup prompt when none remain.
function reject() {
  const slot = currentSlot();
  const c = currentCandidate();
  slot.tried++;
  if (!hasCI(state.rejected, c)) state.rejected.push(c);
  state.history.push({ action: "reject", slot: state.slotIndex, candidate: c });
  save();
  showCurrent();
}

// skipPerson resolves the current person with no handle after every candidate
// was rejected. Recorded in history so it can be undone like any decision.
function skipPerson() {
  currentSlot().skipped = true;
  state.history.push({ action: "skip", slot: state.slotIndex, candidate: "" });
  state.slotIndex++;
  save();
  showCurrent();
}

// tryBackup appends an ad-hoc backup handle to the current (exhausted) person's
// candidate list and re-enters the normal check flow on it. Deliberately not a
// history entry: rejecting the backup is the undoable decision, and undoing
// past that rejection simply re-presents it.
function tryBackup() {
  const slot = currentSlot();
  const h = $("backupInput").value.trim();
  if (!h) return;
  if (hasCI(slot.candidates, h)) {
    $("backupError").textContent = `“${h}” was already tried for this person.`;
    return;
  }
  $("backupInput").value = "";
  $("backupError").textContent = "";
  slot.candidates.push(h);
  save();
  showCurrent();
}

// undo reverses the most recent decision: it reverts the slot it happened in,
// steps the review back there, and re-checks. Reachable from both the review
// pane and the summary, so the last decision can always be corrected.
function undo() {
  const last = state.history.pop();
  if (!last) return;
  const slot = state.slots[last.slot];
  if (last.action === "approve") {
    removeCI(state.approved, last.candidate);
    slot.approved = "";
  } else if (last.action === "reject") {
    removeCI(state.rejected, last.candidate);
    slot.tried--;
    slot.skipped = false; // migrated pre-slot states mark rejected handles skipped too
  } else { // skip
    slot.skipped = false;
  }
  state.slotIndex = last.slot;
  save();
  show("review");
  showCurrent();
}

function updateTallies() {
  $("approvedCount").textContent = state.approved.length;
  $("rejectedCount").textContent = state.rejected.length;
}

// --- summary phase -----------------------------------------------------------

// slotOutcome renders one person's review as a single line: rejected attempts
// struck out (✗) in order, then the winning handle (✓), "(no handle)" when the
// person was skipped or left undecided at the backup prompt, or the untried
// candidates when the review never reached them.
/**
 * @param {Slot} slot
 * @returns {string}
 */
function slotOutcome(slot) {
  const parts = slot.candidates.slice(0, slot.tried).map((c) => c + " ✗");
  if (slot.approved) {
    parts.push(slot.approved + " ✓");
  } else if (slot.skipped || slot.tried >= slot.candidates.length) {
    parts.push("(no handle)");
  } else {
    parts.push(slot.candidates.slice(slot.tried).join(", ") + " (not reviewed)");
  }
  return parts.join(" → ");
}

/** @returns {string} */
function outcomesText() {
  return state.slots.map(slotOutcome).join("\n");
}

function showSummary() {
  const resolved = state.slots.filter((s) => s.approved).length;
  $("summaryLine").textContent =
    `${resolved} of ${state.slots.length} people got an approved handle; ` +
    `${state.rejected.length} rejected along the way.`;
  $("outcomeList").value = outcomesText();
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
  state = { reserved: [], existing: [], slots: [], slotIndex: 0, approved: [], rejected: [], history: [] };
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
  $("checkBackup").addEventListener("click", tryBackup);
  $("backupInput").addEventListener("keydown", (/** @type {KeyboardEvent} */ e) => {
    if (e.key === "Enter") tryBackup();
  });
  $("skipPerson").addEventListener("click", skipPerson);
  $("undo").addEventListener("click", undo);
  $("undoSummary").addEventListener("click", undo);
  $("finish").addEventListener("click", showSummary);
  $("backToStart").addEventListener("click", backToSetup);
  $("reset").addEventListener("click", reset);
  $("resetSetup").addEventListener("click", reset);

  $("copyOutcomes").addEventListener("click", (/** @type {Event} */ e) => copyText(outcomesText(), /** @type {HTMLElement} */ (e.target)));
  $("downloadOutcomes").addEventListener("click", () => download("handle-outcomes.txt", outcomesText()));
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
    if (state.slotIndex >= state.slots.length) {
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
