// Interactive TUI email client demo — matches real Matcha layout
const { useState, useEffect, useRef, useCallback, useMemo } = React;

// ---------- Data: folders, accounts, messages ----------
const FOLDERS_LIST = [
  "INBOX",
  "Archive",
  "Deleted Messages",
  "Drafts",
  "Junk",
  "Notes",
  "Sent Messages",
  "Work",
  "Receipts",
];

const ACCOUNTS_DEFAULT = [
  { id: "all", label: "ALL", isAll: true },
  { id: "home", label: "sam@proton.me" },
  { id: "work", label: "s.park@kestrel.works" },
  { id: "side", label: "hello@driftnotes.xyz" },
  { id: "news", label: "inbox@fastmail.sam" },
];

const MSGS_DEFAULT = [
  {
    acct: "work",
    from: "GitHub",
    subject:
      "[kestrel/hedge] PR #184 ready for review — refactor auth middleware",
    date: "2 min ago",
  },
  {
    acct: "home",
    from: "Linear",
    subject: 'You were assigned KST-219 · "Fix timezone drift in daily digest"',
    date: "14 min ago",
  },
  {
    acct: "home",
    from: "Margo Tran",
    subject: "re: pickup pottery on saturday?",
    date: "1 hour ago",
  },
  {
    acct: "side",
    from: "Stripe",
    subject: "Your payout of $642.11 is on its way",
    date: "3 hours ago",
  },
  {
    acct: "news",
    from: "Hacker News Digest",
    subject: 'Top 10: "Ask HN: what are you running on your homelab in 2026?"',
    date: "5 hours ago",
  },
  {
    acct: "home",
    from: "DMV",
    subject: "Appointment confirmed — Tue May 12, 10:45 AM",
    date: "8 hours ago",
  },
  {
    acct: "work",
    from: "Figma",
    subject: 'Ines shared a file with you: "Hedge · onboarding v3"',
    date: "Yesterday",
  },
  {
    acct: "home",
    from: "Alaska Airlines",
    subject: "Check in for flight AS 1312 — SEA → SFO",
    date: "Yesterday",
  },
  {
    acct: "news",
    from: "The Browser",
    subject: "Five long reads for a slow sunday",
    date: "Yesterday",
  },
  {
    acct: "work",
    from: "Vercel",
    subject: "Deploy failed — hedge-app (main) · missing env DATABASE_URL",
    date: "21/04/2026 17:12",
  },
  {
    acct: "side",
    from: "Plausible Analytics",
    subject: "driftnotes.xyz · 1,204 visitors this week (+18%)",
    date: "21/04/2026 09:02",
  },
  {
    acct: "home",
    from: "Spotify",
    subject: "Your Discover Weekly is ready",
    date: "20/04/2026 08:00",
  },
  {
    acct: "news",
    from: "Metafilter",
    subject: 'MeFi Digest — "I finally fixed my kitchen sink (a story)"',
    date: "19/04/2026 22:44",
  },
  {
    acct: "work",
    from: "Sentry",
    subject:
      "[hedge-prod] New issue: TimeoutError in worker.py:128 (×42 events)",
    date: "18/04/2026 13:27",
  },
  {
    acct: "home",
    from: "Oliver Kim",
    subject: "we are going to regret this tattoo idea (attached)",
    date: "17/04/2026 23:10",
  },
  {
    acct: "home",
    from: "Patagonia",
    subject: "Your Nano Puff has shipped — arriving Thu Apr 25",
    date: "16/04/2026 11:03",
  },
  {
    acct: "side",
    from: "Cloudflare",
    subject: "Reminder: driftnotes.xyz renews in 9 days",
    date: "15/04/2026 04:18",
  },
  {
    acct: "work",
    from: "Notion",
    subject: 'Weekly digest: 4 pages edited in "Kestrel / Engineering"',
    date: "14/04/2026 17:55",
  },
  {
    acct: "news",
    from: "arXiv daily",
    subject: "cs.DC — 6 new submissions (2026-04-13)",
    date: "13/04/2026 06:00",
  },
  {
    acct: "home",
    from: "mom",
    subject: "did the package get there ok??",
    date: "12/04/2026 19:44",
  },
];

const MSGS_DEV = [
  {
    acct: "work",
    from: "GitHub",
    subject: "[kestrel/hedge] PR #184 approved by ines-w",
    date: "09:42",
  },
  {
    acct: "work",
    from: "Vercel",
    subject: "Deploy succeeded — hedge-app (main) · commit 9f2c31a",
    date: "09:18",
  },
  {
    acct: "work",
    from: "Linear",
    subject: "KST-219 moved to In Review · assigned to you",
    date: "08:55",
  },
  {
    acct: "work",
    from: "Sentry",
    subject: "[hedge-prod] Regression: 3 new issues in last 24h",
    date: "Yesterday",
  },
  {
    acct: "work",
    from: "Cloudflare",
    subject: "Worker deployed: hedge-edge@v4.1.0",
    date: "Yesterday",
  },
  {
    acct: "side",
    from: "npm",
    subject: "Your package 'haze-schedule' received 1,043 downloads",
    date: "Mon",
  },
  {
    acct: "news",
    from: "Hacker News",
    subject: "Show HN: A tiny CRDT you can read in one afternoon",
    date: "Mon",
  },
  {
    acct: "home",
    from: "Stripe",
    subject: "New payout: $428.00 USD to Mercury ••4411",
    date: "Mon",
  },
  {
    acct: "work",
    from: "GitHub",
    subject: "[kestrel/hedge] Issue #412: wrap long headers in digest",
    date: "Sun",
  },
  {
    acct: "work",
    from: "AWS Billing",
    subject: "April forecast: $387.22 (-12% vs Mar)",
    date: "Sun",
  },
  {
    acct: "side",
    from: "Fly.io",
    subject: "Machine restarted in ord: driftnotes-web (oom)",
    date: "Sat",
  },
  {
    acct: "news",
    from: "arXiv daily",
    subject: "cs.PL — 4 new submissions (2026-04-20)",
    date: "Sat",
  },
];

const MSGS_PERSONAL = [
  { acct: "home", from: "mom", subject: "did you eat?", date: "12:14" },
  { acct: "home", from: "Margo", subject: "pottery saturday??", date: "09:33" },
  {
    acct: "home",
    from: "REI",
    subject: "Your order has shipped",
    date: "10:01",
  },
  {
    acct: "home",
    from: "Strava",
    subject: "Your week: 38km · 3 runs",
    date: "Yesterday",
  },
  {
    acct: "home",
    from: "Ava (landlord)",
    subject: "building — water shutoff Sat 9-11am",
    date: "Yesterday",
  },
  {
    acct: "home",
    from: "Goodreads",
    subject: "Nadia finished 'The MANIAC'",
    date: "Mon",
  },
  {
    acct: "home",
    from: "Calendly",
    subject: "New booking: Coffee w/ Oliver — Fri 3pm",
    date: "Mon",
  },
  {
    acct: "home",
    from: "your past self",
    subject: "rotate the sourdough starter today",
    date: "Sun",
  },
  {
    acct: "home",
    from: "Spotify",
    subject: "Your Spring Mix is ready",
    date: "Sun",
  },
  {
    acct: "news",
    from: "The New Yorker",
    subject: "The daily — a long weekend read",
    date: "Sat",
  },
];

const DATASETS = {
  default: {
    accounts: ACCOUNTS_DEFAULT,
    messages: MSGS_DEFAULT,
    label: "inbox",
  },
  dev: { accounts: ACCOUNTS_DEFAULT, messages: MSGS_DEV, label: "work" },
  personal: {
    accounts: ACCOUNTS_DEFAULT,
    messages: MSGS_PERSONAL,
    label: "personal",
  },
};

const EMAIL_BODIES = {
  "re: pickup pottery on saturday?": `yes! saturday works — 11am at the studio?
they said the bowls finally came out of the kiln.

also margo owes you coffee. i'm bringing cash.

— m`,
  "Hello world": `Hello world!

this is a simple greeting from nowhere in particular.
just confirming the new inbox is working as expected.

cheers,
m`,
};

function bodyFor(msg) {
  if (EMAIL_BODIES[msg.subject]) return EMAIL_BODIES[msg.subject];
  return `[no plain-text part]

This message has no rendered body in the demo.
Press esc to return to the inbox.`;
}

// ---------- Watermark (original — desert + running figure, NOT copyrighted) ----------
function Watermark() {
  return (
    <svg
      className="tui-watermark"
      viewBox="0 0 1000 520"
      aria-hidden="true"
      preserveAspectRatio="xMidYMax meet"
    >
      {/* dust/star dots */}
      {Array.from({ length: 60 }).map((_, i) => {
        const x = (i * 173) % 1000;
        const y = (i * 97 + 50) % 380;
        const r = 0.8 + ((i * 13) % 5) * 0.3;
        return (
          <circle
            key={i}
            cx={x}
            cy={y}
            r={r}
            fill="currentColor"
            opacity={0.25 + ((i * 7) % 5) * 0.06}
          />
        );
      })}
      {/* ground */}
      <path
        d="M 0 470 L 260 470 Q 310 460 360 470 L 540 470 Q 580 482 620 470 L 1000 470"
        stroke="currentColor"
        strokeWidth="1.2"
        fill="none"
        opacity="0.35"
      />
      {/* original: saguaro cactus */}
      <g transform="translate(660 300)" opacity="0.35" fill="currentColor">
        <rect x="54" y="10" width="24" height="160" rx="4" />
        <rect x="30" y="60" width="24" height="70" rx="4" />
        <rect x="30" y="60" width="16" height="14" rx="3" />
        <rect x="78" y="40" width="24" height="50" rx="4" />
        <rect x="78" y="40" width="24" height="14" rx="3" />
        <rect x="60" y="170" width="12" height="4" />
      </g>
      {/* original: little running figure (stick person) — not a branded character */}
      <g transform="translate(190 360)" fill="currentColor" opacity="0.32">
        <circle cx="22" cy="14" r="11" />
        <rect x="14" y="26" width="22" height="42" rx="4" />
        <rect
          x="4"
          y="34"
          width="14"
          height="5"
          rx="2"
          transform="rotate(-18 11 36)"
        />
        <rect
          x="30"
          y="30"
          width="14"
          height="5"
          rx="2"
          transform="rotate(20 37 32)"
        />
        <rect
          x="10"
          y="68"
          width="8"
          height="24"
          rx="3"
          transform="rotate(-12 14 80)"
        />
        <rect
          x="26"
          y="68"
          width="8"
          height="24"
          rx="3"
          transform="rotate(14 30 80)"
        />
      </g>
    </svg>
  );
}

// ---------- Selection indicator char ----------
function cursor(i, cur, visual, vStart) {
  if (visual && vStart != null) {
    const lo = Math.min(cur, vStart),
      hi = Math.max(cur, vStart);
    const inRange = i >= lo && i <= hi;
    if (inRange && i === cur) return ">*";
    if (inRange) return " *";
    if (i === cur) return "> ";
    return "  ";
  }
  return i === cur ? "> " : "  ";
}

// ---------- TUI component ----------
function TUI({ datasetKey = "default", onKeyPressed }) {
  const dataset = DATASETS[datasetKey] || DATASETS.default;
  const [folderIdx, setFolderIdx] = useState(0);
  const [acctIdx, setAcctIdx] = useState(0);
  const [cur, setCur] = useState(0);
  const [mode, setMode] = useState("list"); // "list" | "email" | "filter" | "visual"
  const [filter, setFilter] = useState("");
  const [visualStart, setVisualStart] = useState(null);
  const [flash, setFlash] = useState("");
  const [deleted, setDeleted] = useState(new Set()); // indices into dataset.messages
  const containerRef = useRef(null);
  const [focused, setFocused] = useState(false);

  const messages = dataset.messages;
  const accounts = dataset.accounts;
  const activeAcct = accounts[acctIdx];

  // Reset on dataset change
  useEffect(() => {
    setFolderIdx(0);
    setAcctIdx(0);
    setCur(0);
    setMode("list");
    setFilter("");
    setVisualStart(null);
    setDeleted(new Set());
    setFlash(`loaded: ${dataset.label}`);
    const t = setTimeout(() => setFlash(""), 1400);
    return () => clearTimeout(t);
  }, [datasetKey]);

  // Visible messages
  const visible = useMemo(() => {
    let list = messages
      .map((m, origIdx) => ({ ...m, origIdx }))
      .filter((m) => !deleted.has(m.origIdx));
    if (folderIdx === 0) {
      if (!activeAcct.isAll)
        list = list.filter((m) => m.acct === activeAcct.id);
    } else {
      // Other folders are empty in the demo
      list = [];
    }
    if (filter.trim()) {
      const q = filter.toLowerCase();
      list = list.filter(
        (m) =>
          m.subject.toLowerCase().includes(q) ||
          m.from.toLowerCase().includes(q),
      );
    }
    return list;
  }, [messages, deleted, folderIdx, activeAcct, filter]);

  const selected = visible[cur] || null;

  const focusMe = useCallback(() => {
    if (containerRef.current) containerRef.current.focus();
  }, []);

  const flashFor = (msg, ms = 1200) => {
    setFlash(msg);
    setTimeout(() => setFlash((f) => (f === msg ? "" : f)), ms);
  };

  const doDelete = (indices) => {
    setDeleted((prev) => {
      const next = new Set(prev);
      indices.forEach((i) => next.add(i));
      return next;
    });
    flashFor(
      `✓ deleted ${indices.length} message${indices.length === 1 ? "" : "s"}`,
    );
  };

  // Key handling
  const handleKey = (e) => {
    if (onKeyPressed) onKeyPressed();
    const k = e.key;

    if (mode === "filter") {
      if (k === "Enter" || k === "Escape") {
        e.preventDefault();
        setMode("list");
        flashFor(filter ? `filter: "${filter}"` : "filter cleared");
        return;
      }
      if (k === "Backspace") {
        e.preventDefault();
        setFilter((f) => f.slice(0, -1));
        return;
      }
      if (k.length === 1) {
        e.preventDefault();
        setFilter((f) => f + k);
        return;
      }
      return;
    }

    if (mode === "email") {
      if (k === "Escape") {
        e.preventDefault();
        setMode("list");
        return;
      }
      if (k === "r") {
        e.preventDefault();
        flashFor("⇢ reply (composer)");
        return;
      }
      if (k === "f") {
        e.preventDefault();
        flashFor("⇢ forward");
        return;
      }
      if (k === "d") {
        e.preventDefault();
        if (selected) {
          doDelete([selected.origIdx]);
          setMode("list");
          setCur((c) => Math.max(0, Math.min(c, visible.length - 2)));
        }
        return;
      }
      if (k === "a") {
        e.preventDefault();
        flashFor("▤ archived");
        setMode("list");
        return;
      }
      if (k === "i") {
        e.preventDefault();
        flashFor("◧ images toggled");
        return;
      }
      return;
    }

    if (mode === "visual") {
      if (k === "v" || k === "Escape") {
        e.preventDefault();
        setMode("list");
        setVisualStart(null);
        return;
      }
      if (k === "j" || k === "ArrowDown") {
        e.preventDefault();
        setCur((c) => Math.min(c + 1, visible.length - 1));
        return;
      }
      if (k === "k" || k === "ArrowUp") {
        e.preventDefault();
        setCur((c) => Math.max(c - 1, 0));
        return;
      }
      if (k === "d") {
        e.preventDefault();
        const lo = Math.min(cur, visualStart),
          hi = Math.max(cur, visualStart);
        const indices = visible.slice(lo, hi + 1).map((m) => m.origIdx);
        doDelete(indices);
        setMode("list");
        setVisualStart(null);
        setCur((c) =>
          Math.max(0, Math.min(lo, visible.length - indices.length - 1)),
        );
        return;
      }
      if (k === "a") {
        e.preventDefault();
        flashFor("▤ archived batch");
        setMode("list");
        setVisualStart(null);
        return;
      }
      if (k === "m") {
        e.preventDefault();
        flashFor("⇢ move to folder…");
        setMode("list");
        setVisualStart(null);
        return;
      }
      return;
    }

    // list mode
    if (k === "Escape") {
      e.preventDefault();
      flashFor("← main menu");
      return;
    }
    if (k === "/") {
      e.preventDefault();
      setMode("filter");
      setFilter("");
      return;
    }
    if (k === "j" || k === "ArrowDown") {
      e.preventDefault();
      setCur((c) => Math.min(c + 1, visible.length - 1));
      return;
    }
    if (k === "k" || k === "ArrowUp") {
      e.preventDefault();
      setCur((c) => Math.max(c - 1, 0));
      return;
    }
    if (k === "h" || k === "ArrowLeft") {
      e.preventDefault();
      setAcctIdx((i) => (i - 1 + accounts.length) % accounts.length);
      setCur(0);
      return;
    }
    if (k === "l" || k === "ArrowRight") {
      e.preventDefault();
      setAcctIdx((i) => (i + 1) % accounts.length);
      setCur(0);
      return;
    }
    if (k === "Tab" && !e.shiftKey) {
      e.preventDefault();
      setFolderIdx((i) => (i + 1) % FOLDERS_LIST.length);
      setCur(0);
      return;
    }
    if (k === "Tab" && e.shiftKey) {
      e.preventDefault();
      setFolderIdx((i) => (i - 1 + FOLDERS_LIST.length) % FOLDERS_LIST.length);
      setCur(0);
      return;
    }
    if (k === "Enter") {
      e.preventDefault();
      if (selected) setMode("email");
      return;
    }
    if (k === "v") {
      e.preventDefault();
      if (visible.length) {
        setMode("visual");
        setVisualStart(cur);
      }
      return;
    }
    if (k === "d") {
      e.preventDefault();
      if (selected) {
        doDelete([selected.origIdx]);
        setCur((c) => Math.max(0, Math.min(c, visible.length - 2)));
      }
      return;
    }
    if (k === "a") {
      e.preventDefault();
      flashFor("▤ archived");
      return;
    }
    if (k === "r") {
      e.preventDefault();
      flashFor("↻ inbox refreshed");
      return;
    }
    if (k === "q") {
      e.preventDefault();
      flashFor("quit");
      return;
    }
  };

  const titleSuffix =
    mode === "visual"
      ? ` - VISUAL (${Math.abs(cur - (visualStart ?? cur)) + 1} selected)`
      : "";
  const folderLabel = FOLDERS_LIST[folderIdx];
  const isInbox = folderIdx === 0;
  const acctLabel = activeAcct.isAll ? "All Accounts" : activeAcct.label;

  return (
    <div
      ref={containerRef}
      className={"tui-root " + (focused ? "is-focused" : "")}
      tabIndex={0}
      onKeyDown={handleKey}
      onFocus={() => setFocused(true)}
      onBlur={() => setFocused(false)}
      onClick={focusMe}
    >
      {/* Title bar */}
      <div className="tui-titlebar">
        <div className="tui-titlebar-left">
          <span className="tui-traffic">
            <span className="tl tl-r" />
            <span className="tl tl-y" />
            <span className="tl tl-g" />
          </span>
          <span className="tui-title-text">matcha — kai@floatpane</span>
        </div>
        <div className="tui-titlebar-right">
          <span className="sep">·</span>
          <span>80×24</span>
        </div>
      </div>

      {/* Body: sidebar + main pane */}
      <div className="tui-shell">
        {/* Sidebar */}
        <div className="tui-sidebar">
          <div className="tui-sidebar-head">Drew Smirnoff</div>
          {FOLDERS_LIST.map((f, i) => (
            <div
              key={f}
              className={"tui-folder " + (i === folderIdx ? "active" : "")}
              onClick={() => {
                setFolderIdx(i);
                setCur(0);
                focusMe();
              }}
            >
              {f}
            </div>
          ))}
        </div>

        {/* Main */}
        <div className="tui-main">
          {mode !== "email" ? (
            <>
              {/* Account tabs */}
              <div className="tui-acct-row">
                {accounts.map((a, i) => (
                  <div
                    key={a.id}
                    className={"tui-acct " + (i === acctIdx ? "active" : "")}
                    onClick={() => {
                      setAcctIdx(i);
                      setCur(0);
                      focusMe();
                    }}
                  >
                    {a.label}
                  </div>
                ))}
              </div>

              {/* Header line */}
              <div className="tui-heading">
                {isInbox ? (
                  <>
                    INBOX — {acctLabel}
                    {titleSuffix}
                  </>
                ) : (
                  <>
                    {folderLabel}
                    {titleSuffix}
                  </>
                )}
              </div>
              <div className="tui-subhead">
                {isInbox
                  ? `${visible.length} ${visible.length === 1 ? "email" : "emails"}`
                  : "folder empty"}
                {filter && <span className="tui-filter-chip"> /{filter}</span>}
              </div>

              {/* Message rows */}
              <div className="tui-list">
                {visible.length === 0 && (
                  <div className="tui-row-empty">— no messages —</div>
                )}
                {visible.map((m, i) => {
                  const acct = accounts.find((a) => a.id === m.acct);
                  return (
                    <div
                      key={m.origIdx}
                      className={
                        "tui-row " +
                        (i === cur ? "cur " : "") +
                        (mode === "visual" ? "visual " : "")
                      }
                      onClick={() => {
                        setCur(i);
                        focusMe();
                      }}
                      onDoubleClick={() => {
                        setCur(i);
                        setMode("email");
                        focusMe();
                      }}
                    >
                      <span className="tui-row-cursor">
                        {cursor(i, cur, mode === "visual", visualStart)}
                      </span>
                      <span className="tui-row-num">
                        {String(i + 1).padStart(2, " ")}.
                      </span>
                      <span className="tui-row-acct">
                        [{acct ? acct.label : m.acct}]
                      </span>
                      <span className="tui-row-sep">◆</span>
                      <span className="tui-row-from">{m.from}</span>
                      <span className="tui-row-sep">•</span>
                      <span className="tui-row-subj">{m.subject}</span>
                      <span className="tui-row-date">{m.date}</span>
                    </div>
                  );
                })}
                <div className="tui-row-dots">• • • •</div>
              </div>

              <Watermark />
            </>
          ) : (
            <EmailView
              msg={selected}
              acct={accounts.find((a) => a.id === selected?.acct)}
            />
          )}
        </div>
      </div>

      {/* Filter input */}
      {mode === "filter" && (
        <div className="tui-cmdline">
          <span className="tui-cmdline-prompt">/</span>
          <span className="tui-cmdline-buf">{filter}</span>
          <span className="tui-caret">█</span>
        </div>
      )}

      {/* Flash */}
      {flash && <div className="tui-flash">{flash}</div>}

      {/* Status line */}
      <div className="tui-status">
        {mode === "email" ? (
          <>
            <span className="seg">↑↓ r: reply</span>
            <span className="seg">↵ f: forward</span>
            <span className="seg">● d: delete</span>
            <span className="seg">□ a: archive</span>
            <span className="seg">⇥ tab: focus attachments</span>
            <span className="seg">⎋ esc: back to inbox</span>
            <span className="seg">⚫ i: toggle images</span>
          </>
        ) : mode === "visual" ? (
          <>
            <span className="seg hi">-- VISUAL --</span>
            <span className="seg">j/k expand</span>
            <span className="seg">d delete</span>
            <span className="seg">a archive</span>
            <span className="seg">m move</span>
            <span className="seg">v/esc exit</span>
          </>
        ) : mode === "filter" ? (
          <>
            <span className="seg hi">-- FILTER --</span>
            <span className="seg">enter apply</span>
            <span className="seg">esc cancel</span>
          </>
        ) : (
          <>
            <span className="seg">↑/k up</span>
            <span className="seg">↓/j down</span>
            <span className="seg">/ filter</span>
            <span className="seg">v visual mode</span>
            <span className="seg">● d delete</span>
            <span className="seg">□ a archive</span>
            <span className="seg">◇ r refresh</span>
            <span className="seg">←/h prev tab</span>
            <span className="seg">→/l next tab</span>
            <span className="seg">tab next folder</span>
            <span className="seg">shift+tab prev folder</span>
            <span className="seg">m move</span>
            <span className="seg">q quit</span>
          </>
        )}
      </div>

      {/* Focus hint */}
      {!focused && (
        <div className="tui-focus-hint">
          click · try <kbd>j</kbd>
          <kbd>k</kbd> <kbd>tab</kbd> <kbd>/</kbd> <kbd>↵</kbd>
        </div>
      )}
    </div>
  );
}

function EmailView({ msg, acct }) {
  if (!msg) return null;
  const body = bodyFor(msg);
  const from = acct ? `<${acct.label}>` : "";
  return (
    <div className="tui-email">
      <div className="tui-email-head">
        To: {acct ? acct.label : "you"} | From: {msg.from} {from} | Subject:{" "}
        {msg.subject}
      </div>
      <div className="tui-email-body">
        {body.split("\n").map((line, i) => {
          // highlight 'this guy' style blue token if present
          const parts = line.split(/(this guy)/);
          return (
            <div key={i} className="tui-email-line">
              {parts.map((p, j) =>
                p === "this guy" ? (
                  <span key={j} className="tui-link">
                    {p}
                  </span>
                ) : (
                  <span key={j}>{p || "\u00a0"}</span>
                ),
              )}
            </div>
          );
        })}
      </div>
      <Watermark />
    </div>
  );
}

window.MatchaTUI = TUI;
