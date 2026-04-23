const { useState, useEffect } = React;

function FloatpaneMark({ size = 20 }) {
  return (
    <img
      src="../assets/floatpane.png"
      alt="Floatpane logo"
      className="wm-logo"
      height={size}
      width={size}
    />
  );
}

function MatchaWordmark() {
  const [version, setVersion] = useState("v0.8.2");
  useEffect(() => {
    // floatpane/matcha repo — fetch latest release tag
    fetch("https://api.github.com/repos/floatpane/matcha/releases/latest")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (d && d.tag_name)
          setVersion(
            d.tag_name.startsWith("v") ? d.tag_name : "v" + d.tag_name,
          );
      })
      .catch(() => {});
  }, []);
  return (
    <div className="wordmark">
      <img
        src="../assets/logo-transparent.png"
        alt="Matcha logo"
        className="wm-logo"
        height={24}
        width={24}
      />
      <span className="wm-name">matcha</span>
      <span className="wm-dim">— {version}</span>
    </div>
  );
}

function TopNav() {
  const [stars, setStars] = useState(null);
  useEffect(() => {
    fetch("https://api.github.com/repos/floatpane/matcha")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (d && typeof d.stargazers_count === "number") {
          const n = d.stargazers_count;
          setStars(n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n));
        }
      })
      .catch(() => {});
  }, []);
  return (
    <header className="nav">
      <a href="#top" className="nav-brand">
        <MatchaWordmark />
      </a>
      <nav className="nav-links">
        <a href="#features">Features</a>
        <a href="#keys">Keybinds</a>
        <a href="#install">Install</a>
        <a href="https://docs.matcha.floatpane.com">Docs ↗</a>
        <a href="https://github.com/floatpane/matcha" className="nav-github">
          <span>GitHub</span>
          {stars && <span className="nav-star">★ {stars}</span>}
        </a>
      </nav>
      <div className="nav-right">
        <a href="#install" className="btn btn-ghost">
          install
        </a>
      </div>
    </header>
  );
}

function Hero({ datasetKey, setDatasetKey }) {
  const [pressed, setPressed] = useState(false);
  const TUI = window.MatchaTUI;
  return (
    <section className="hero" id="top">
      <div className="hero-copy">
        <div className="hero-eyebrow">
          <span className="dot-live" />
          <span>by floatpane · local-first · secure · no telemetry</span>
        </div>
        <h1 className="hero-h1">
          A powerful, feature-rich
          <br />
          email client <em>for your terminal.</em>
        </h1>
        <p className="hero-sub">
          Matcha is a modern TUI email client for people who live in the shell.
          Vim keybindings, PGP, IMAP multi-account, markdown composing,
          visual-mode batch ops, and a CLI that speaks your language.
        </p>
        <div className="hero-cta">
          <a href="#install" className="btn btn-primary">
            <span>Install</span>
            <span className="btn-k">↵</span>
          </a>
          <a href="https://docs.matcha.floatpane.com" className="btn btn-ghost">
            <span>Read the docs</span>
            <span className="btn-k">→</span>
          </a>
        </div>
        <div className="hero-meta">
          <div>
            <span className="dim">license</span> MIT
          </div>
          <div>
            <span className="dim">runtime</span> single static binary
          </div>
          <div>
            <span className="dim">platforms</span> macOS · Linux · Windows
          </div>
        </div>
      </div>

      <div className="hero-demo">
        <div className="hero-demo-chrome">
          <div className="hero-demo-label">
            <span className="dim">demo ·</span>
            <button
              className={"demo-swap " + (datasetKey === "default" ? "on" : "")}
              onClick={() => setDatasetKey("default")}
            >
              drew's inbox
            </button>
            <button
              className={"demo-swap " + (datasetKey === "dev" ? "on" : "")}
              onClick={() => setDatasetKey("dev")}
            >
              floatpane dev
            </button>
            <button
              className={"demo-swap " + (datasetKey === "personal" ? "on" : "")}
              onClick={() => setDatasetKey("personal")}
            >
              personal
            </button>
          </div>
          <div className="hero-demo-hint">
            {pressed ? (
              <span>✓ keyboard live</span>
            ) : (
              <span className="dim">
                click · <kbd>j</kbd>
                <kbd>k</kbd> · <kbd>tab</kbd> · <kbd>↵</kbd>
              </span>
            )}
          </div>
        </div>
        {TUI && (
          <TUI datasetKey={datasetKey} onKeyPressed={() => setPressed(true)} />
        )}
      </div>
    </section>
  );
}

const FEATURES = [
  {
    k: "01",
    title: "Email, the way you move",
    body: "Read, reply, delete, archive — all from the home row. j/k to move, h/l between accounts, tab between folders. No mouse, no modals.",
    mono: "j  k  h  l  r  d  a  ↵",
  },
  {
    k: "02",
    title: "Visual mode, for real",
    body: "Press v to enter Vim-style multi-select. Expand with j/k, then d, a, or m to run batch ops as a single IMAP command.",
    mono: "v  j j j  d\n→ deleted 4 messages",
  },
  {
    k: "03",
    title: "Compose in markdown",
    body: "Write in the syntax you already know. Headings, lists, fenced code, and tables render cleanly on the other side.",
    mono: "# subject\n- bullet\n`inline`",
  },
  {
    k: "04",
    title: "Multi-account, tabbed",
    body: "IMAP, Gmail, Fastmail, Proton Bridge — all tabbed in one window. h and l switch between them so you never reply from the wrong address.",
    mono: "← me@andrinoff\n→ drew@floatpane",
  },
  {
    k: "05",
    title: "Fuzzy filter",
    body: "Press / to fuzzy-filter across senders, subjects, and bodies in the active view. Results stream in as you type.",
    mono: "/lena  →  3 hits",
  },
  {
    k: "06",
    title: "Local-first drafts",
    body: "Every keystroke hits disk before it hits the wire. Close the laptop, open it anywhere, pick up mid-sentence. Esc saves.",
    mono: "~/.cache/matcha/drafts",
  },
  {
    k: "07",
    title: "CLI that composes",
    body: "Pipe errors into apologies. Send from scripts, CI, or cron. `matcha send` does one thing well.",
    mono: "$ matcha send --to …",
  },
  {
    k: "08",
    title: "AI, on your terms",
    body: "Rewrite drafts with the model of your choice. Let agents send on your behalf — with strict scopes and an audit log.",
    mono: "alt + r: make it more formal",
  },
  {
    k: "09",
    title: "Smart image rendering",
    body: "Images render inline via iterm2 or kitty-graphics where supported. Toggle with i. Off by default, always.",
    mono: "i  →  ◧ images on",
  },
];

function Features() {
  return (
    <section className="features" id="features">
      <div className="section-head">
        <div className="section-head-l">
          <div className="section-eyebrow">§ features</div>
          <h2 className="section-h2">
            Everything you'd expect.
            <br />
            <span className="dim">And nothing you wouldn't.</span>
          </h2>
        </div>
        <p className="section-head-r">
          Matcha is opinionated. It won't follow you around the web, won't
          upsell you on AI credits, and won't sync your signatures to a SaaS. It
          reads mail. It writes mail. It stays out of the way.
        </p>
      </div>
      <div className="feature-grid">
        {FEATURES.map((f) => (
          <article key={f.k} className="feature">
            <div className="feature-head">
              <span className="feature-k">{f.k}</span>
              <span className="feature-dash">——</span>
            </div>
            <h3 className="feature-title">{f.title}</h3>
            <p className="feature-body">{f.body}</p>
            <pre className="feature-mono">{f.mono}</pre>
          </article>
        ))}
      </div>
    </section>
  );
}

function Keybinds() {
  const rows = [
    {
      g: "motion",
      items: [
        ["j / k", "next / prev message"],
        ["↑ / ↓", "next / prev message"],
        ["h / l", "prev / next account"],
        ["← / →", "prev / next account"],
        ["tab", "next folder"],
        ["shift-tab", "prev folder"],
      ],
    },
    {
      g: "inbox",
      items: [
        ["↵", "open email"],
        ["r", "refresh"],
        ["d", "delete"],
        ["a", "archive"],
        ["/", "filter"],
        ["v", "visual mode"],
        ["esc", "back / main menu"],
      ],
    },
    {
      g: "visual mode",
      items: [
        ["v", "enter visual mode"],
        ["j / k", "expand selection"],
        ["d", "delete all selected"],
        ["a", "archive all selected"],
        ["m", "move to folder"],
        ["v / esc", "exit visual mode"],
      ],
    },
    {
      g: "email view",
      items: [
        ["j / k", "scroll body"],
        ["r", "reply"],
        ["d", "delete"],
        ["a", "archive"],
        ["tab", "focus attachments"],
        ["i", "toggle images"],
        ["esc", "back to inbox"],
      ],
    },
    {
      g: "attachments",
      items: [
        ["j / k", "navigate"],
        ["↵", "download & open"],
        ["tab / esc", "back to body"],
      ],
    },
    {
      g: "composer",
      items: [
        ["tab / shift-tab", "navigate fields"],
        ["↵ on From", "select account"],
        ["↵ on Attachment", "open file picker"],
        ["↵ on Send", "send email"],
        ["↑ / ↓", "contact suggestions"],
        ["esc", "save draft & exit"],
      ],
    },
  ];
  return (
    <section className="keybinds" id="keys">
      <div className="section-head">
        <div className="section-head-l">
          <div className="section-eyebrow">§ keybinds</div>
          <h2 className="section-h2">
            Vim-native.
            <br />
            <span className="dim">Home row to inbox zero.</span>
          </h2>
        </div>
        <p className="section-head-r">
          Every binding is documented at{" "}
          <a
            href="https://docs.matcha.floatpane.com"
            className="underline-link"
          >
            docs.matcha.floatpane.com
          </a>
          . Muscle-memory for vimmers, learnable for everyone else.
        </p>
      </div>
      <div className="keybinds-grid">
        {rows.map((row) => (
          <div key={row.g} className="keybinds-col">
            <div className="keybinds-col-head">── {row.g} ──</div>
            {row.items.map(([k, label]) => (
              <div key={k} className="keybind-row">
                <span className="keybind-k">
                  <kbd>{k}</kbd>
                </span>
                <span className="keybind-dots">{"·".repeat(26)}</span>
                <span className="keybind-label">{label}</span>
              </div>
            ))}
          </div>
        ))}
      </div>
    </section>
  );
}

const INSTALL_TABS = {
  brew: {
    plat: "macOS · Linux",
    cmd: "$ brew install floatpane/matcha/matcha\n$ matcha",
  },
  winget: {
    plat: "Windows 10 / 11",
    cmd: "$ winget install --id=floatpane.matcha\n$ matcha",
  },
  snap: { plat: "Ubuntu · Linux", cmd: "$ sudo snap install matcha\n$ matcha" },
  flatpak: {
    plat: "Linux",
    cmd: "$ flatpak install https://matcha.floatpane.com/matcha.flatpakref\n$ matcha",
  },
  aur: { plat: "Arch Linux", cmd: "$ yay -S matcha-client-bin\n$ matcha" },
  nix: {
    plat: "NixOS · any Nix",
    cmd: "$ nix profile install github:floatpane/matcha\n$ matcha",
  },
};

function Install() {
  const [tab, setTab] = useState("brew");
  const [meta, setMeta] = useState({
    version: "0.8.2",
    date: "apr 23, 2026",
    size: null,
  });
  useEffect(() => {
    fetch("https://api.github.com/repos/floatpane/matcha/releases/latest")
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => {
        if (!d) return;
        const version = (d.tag_name || "").replace(/^v/, "") || "0.8.2";
        const date = d.published_at
          ? new Date(d.published_at)
              .toLocaleDateString("en-US", {
                month: "short",
                day: "numeric",
                year: "numeric",
              })
              .toLowerCase()
          : "apr 23, 2026";
        const asset = (d.assets || []).find((a) => a.size) || null;
        const size = asset
          ? (asset.size / (1024 * 1024)).toFixed(1) + " MB"
          : null;
        setMeta({ version, date, size });
      })
      .catch(() => {});
  }, []);
  const t = INSTALL_TABS[tab];
  return (
    <section className="install" id="install">
      <div className="section-head">
        <div className="section-head-l">
          <div className="section-eyebrow">§ install</div>
          <h2 className="section-h2">
            One binary.
            <br />
            <span className="dim">Pick your package manager.</span>
          </h2>
        </div>
        <p className="section-head-r">
          No runtime. Ships natively for macOS, Linux, Windows. Source and
          issues at{" "}
          <a
            href="https://github.com/floatpane/matcha"
            className="underline-link"
          >
            github.com/floatpane/matcha
          </a>
          .
        </p>
      </div>
      <div className="install-card">
        <div className="install-tabs">
          {Object.keys(INSTALL_TABS).map((k) => (
            <button
              key={k}
              onClick={() => setTab(k)}
              className={"install-tab " + (tab === k ? "active" : "")}
            >
              {k}
            </button>
          ))}
          <div className="install-tabs-spacer" />
          <span className="install-plat">{t.plat}</span>
        </div>
        <pre className="install-code">{t.cmd}</pre>
        <div className="install-foot">
          <div>
            <span className="dim">latest</span> {meta.version} · {meta.date}
          </div>
          <div>
            <span className="dim">source</span> github.com/floatpane/matcha
          </div>
        </div>
      </div>
    </section>
  );
}

function CTA() {
  return (
    <section className="cta">
      <div className="cta-inner">
        <div className="cta-pre">$ _</div>
        <h2 className="cta-h2">
          Your inbox is waiting
          <br />
          in the terminal.
        </h2>
        <div className="cta-row">
          <a href="#install" className="btn btn-primary btn-lg">
            make your emails secure
          </a>
          <a
            href="https://docs.matcha.floatpane.com"
            className="btn btn-ghost btn-lg"
          >
            read the docs →
          </a>
        </div>
      </div>
    </section>
  );
}

function Footer() {
  return (
    <footer className="footer">
      <div className="footer-top">
        <div className="footer-brand">
          <MatchaWordmark />
          <p className="footer-tag">
            a modern TUI email client.
            <br />
            made with care by floatpane.
          </p>
        </div>
        <div className="footer-cols">
          <div>
            <div className="footer-h">product</div>
            <a href="#features">features</a>
            <a href="#keys">keybinds</a>
            <a href="#install">install</a>
            <a href="https://github.com/floatpane/matcha/releases">releases</a>
          </div>
          <div>
            <div className="footer-h">resources</div>
            <a href="https://docs.matcha.floatpane.com">docs</a>
            <a href="https://docs.matcha.floatpane.com/Configuration">config</a>
            <a href="https://docs.matcha.floatpane.com/Features/CLI">cli</a>
            <a href="https://github.com/floatpane/matcha/blob/master/SECURITY.md">
              security
            </a>
          </div>
          <div>
            <div className="footer-h">community</div>
            <a href="https://github.com/floatpane/matcha">github</a>
            <a href="https://discord.gg/RxNrJgfatk">discord</a>
            <a href="https://fosstodon.org/@floatpane">mastodon</a>
          </div>
          <div>
            <div className="footer-h">floatpane</div>
            <a href="https://floatpane.com">website</a>
            <a href="mailto:us@floatpane.com">contact</a>
            <a href="mailto:support@floatpane.com">support</a>
          </div>
        </div>
      </div>
      <div className="footer-bot">
        <div className="footer-copy">
          <FloatpaneMark size={16} />
          <span>
            © {new Date().getFullYear()} floatpane · MIT licensed · no trackers
            on this page
          </span>
        </div>
      </div>
    </footer>
  );
}

// ---------- Tweaks ----------
function Tweaks({ datasetKey, setDatasetKey, visible }) {
  if (!visible) return null;
  return (
    <div className="tweaks">
      <div className="tweaks-head">Tweaks</div>
      <div className="tweaks-sub">Demo content</div>
      {Object.entries({
        default: "drew's inbox",
        dev: "floatpane dev",
        personal: "personal",
      }).map(([k, label]) => (
        <button
          key={k}
          onClick={() => setDatasetKey(k)}
          className={"tweaks-opt " + (datasetKey === k ? "on" : "")}
        >
          <span className="tweaks-dot">{datasetKey === k ? "●" : "○"}</span>
          <span>{label}</span>
        </button>
      ))}
    </div>
  );
}

function App() {
  const [datasetKey, setDatasetKey] = useState(() => {
    try {
      return localStorage.getItem("matcha-dataset") || "default";
    } catch {
      return "default";
    }
  });
  const [tweaksVisible, setTweaksVisible] = useState(false);

  useEffect(() => {
    try {
      localStorage.setItem("matcha-dataset", datasetKey);
    } catch {}
  }, [datasetKey]);

  useEffect(() => {
    const onMsg = (e) => {
      const d = e.data || {};
      if (d.type === "__activate_edit_mode") setTweaksVisible(true);
      if (d.type === "__deactivate_edit_mode") setTweaksVisible(false);
    };
    window.addEventListener("message", onMsg);
    window.parent.postMessage({ type: "__edit_mode_available" }, "*");
    return () => window.removeEventListener("message", onMsg);
  }, []);

  useEffect(() => {
    window.parent.postMessage(
      { type: "__edit_mode_set_keys", edits: { datasetKey } },
      "*",
    );
  }, [datasetKey]);

  return (
    <div className="site">
      <TopNav />
      <Hero datasetKey={datasetKey} setDatasetKey={setDatasetKey} />
      <Features />
      <Keybinds />
      <Install />
      <CTA />
      <Footer />
      <Tweaks
        datasetKey={datasetKey}
        setDatasetKey={setDatasetKey}
        visible={tweaksVisible}
      />
    </div>
  );
}

window.MatchaApp = App;
