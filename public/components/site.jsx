const { useState, useEffect } = React;

// --- Global hooks ---

function useScrollReveal() {
  useEffect(() => {
    const els = document.querySelectorAll(".reveal");
    if (!els.length) return;
    if (!("IntersectionObserver" in window)) {
      els.forEach((el) => el.classList.add("visible"));
      return;
    }
    const obs = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) {
            e.target.classList.add("visible");
            obs.unobserve(e.target);
          }
        });
      },
      { threshold: 0.08, rootMargin: "0px 0px -40px 0px" },
    );
    els.forEach((el) => obs.observe(el));
    return () => obs.disconnect();
  }, []);
}

function useMouseGlow() {
  useEffect(() => {
    const move = (e) => {
      document.documentElement.style.setProperty("--mx", e.clientX + "px");
      document.documentElement.style.setProperty("--my", e.clientY + "px");
    };
    window.addEventListener("mousemove", move, { passive: true });
    return () => window.removeEventListener("mousemove", move);
  }, []);
}

// --- Components ---

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
        <a href="#install">Install</a>
        <a href="https://docs.matcha.email">Docs ↗</a>
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

function QuickStart() {
  const CMD1 = "brew install floatpane/matcha/matcha";
  const CMD2 = "matcha";
  const [l1, setL1] = useState("");
  const [l2, setL2] = useState("");
  const [phase, setPhase] = useState("pre"); // pre → t1 → pause → t2 → done

  useEffect(() => {
    let t;
    if (phase === "pre") {
      t = setTimeout(() => setPhase("t1"), 1400);
    } else if (phase === "t1") {
      if (l1.length < CMD1.length) {
        t = setTimeout(() => setL1(CMD1.slice(0, l1.length + 1)), 36);
      } else {
        t = setTimeout(() => setPhase("pause"), 420);
      }
    } else if (phase === "pause") {
      t = setTimeout(() => setPhase("t2"), 320);
    } else if (phase === "t2") {
      if (l2.length < CMD2.length) {
        t = setTimeout(() => setL2(CMD2.slice(0, l2.length + 1)), 90);
      } else {
        setPhase("done");
      }
    }
    return () => clearTimeout(t);
  }, [phase, l1, l2]);

  const caret1 = phase === "pre" || phase === "t1" || phase === "pause";
  const showL2 = phase === "t2" || phase === "done";

  return (
    <div className="quickstart">
      <div className="qs-bar">
        <span className="qs-dot qs-r" />
        <span className="qs-dot qs-y" />
        <span className="qs-dot qs-g" />
        <span className="qs-bar-label">terminal</span>
      </div>
      <pre className="qs-code">
        <span className="qs-prompt">$ </span>
        {l1}
        {caret1 && <span className="qs-caret" />}
        {showL2 && (
          <>
            {"\n"}
            <span className="qs-prompt">$ </span>
            {l2}
            <span className="qs-caret" />
          </>
        )}
      </pre>
    </div>
  );
}

function Hero() {
  return (
    <section className="hero" id="top">
      <div className="hero-copy">
        <div className="hero-eyebrow">
          <span className="dot-live" />
          <span>by floatpane · local-first · secure · no telemetry</span>
        </div>
        <h1 className="hero-h1">
          Email for people who
          <br />
          live in the <em>terminal.</em>
        </h1>
        <p className="hero-sub">
          Matcha is a keyboard-native email client built for the shell.
          Multi-account IMAP, PGP encryption, markdown composing, and a CLI
          that pipes. One static binary. No cloud. No trackers.
        </p>
        <div className="hero-cta">
          <a href="#install" className="btn btn-primary btn-lg">
            Install now
          </a>
          <a href="https://docs.matcha.email" className="btn btn-ghost btn-lg">
            Read the docs <span className="btn-k">→</span>
          </a>
        </div>
        <QuickStart />
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
    </section>
  );
}

const FEATURES = [
  {
    k: "01",
    title: "Keyboard-native",
    body: "Read, reply, delete, archive — all from the keyboard. Navigate messages, switch accounts, and jump between folders without touching the mouse.",
    mono: "j  k  r  d  a  ↵  esc",
  },
  {
    k: "02",
    title: "Visual mode batch ops",
    body: "Enter visual mode to select a range of messages, then delete, archive, or move them all as a single IMAP command.",
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
    body: "IMAP, Gmail, Fastmail, Proton Bridge — all in one window. Switch between them instantly so you never reply from the wrong address.",
    mono: "← me@andrinoff\n→ drew@floatpane",
  },
  {
    k: "05",
    title: "Fuzzy filter",
    body: "Filter across senders, subjects, and bodies in the active view. Results stream in as you type.",
    mono: "/lena  →  3 hits",
  },
  {
    k: "06",
    title: "Local-first drafts",
    body: "Every keystroke hits disk before it hits the wire. Close the laptop, open it anywhere, pick up mid-sentence.",
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
    title: "Inline image rendering",
    body: "Images render inline via iTerm2 or kitty graphics where supported. Toggle with a key. Off by default, always.",
    mono: "→ ◧ images on",
  },
  {
    k: "09",
    title: "Full-disk encryption",
    body: "Encrypt all local data with a password that is never stored — not on disk, not in the keyring. Matcha shows a lock screen on startup. Forget the password and there is no reset.",
    mono: "matcha is locked\n> ••••••••\nenter: unlock",
  },
];

function Features() {
  return (
    <section className="features" id="features">
      <div className="section-head reveal">
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
          upsell you on credits, and won't sync your signatures to a SaaS. It
          reads mail. It writes mail. It stays out of the way.
        </p>
      </div>
      <div className="feature-grid">
        {FEATURES.map((f, i) => (
          <article
            key={f.k}
            className="feature reveal"
            style={{ transitionDelay: `${i * 55}ms` }}
          >
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

const INSTALL_TABS = {
  brew: {
    plat: "macOS · Linux",
    cmd: "$ brew install floatpane/matcha/matcha\n$ matcha",
  },
  winget: {
    plat: "Windows 10 / 11",
    cmd: "$ winget install --id=floatpane.matcha\n$ matcha",
  },
  scoop: {
    plat: "Windows",
    cmd: "$ scoop install matcha\n$ matcha",
  },
  snap: { plat: "Ubuntu · Linux", cmd: "$ sudo snap install matcha\n$ matcha" },
  flatpak: {
    plat: "Linux",
    cmd: "$ flatpak install https://matcha.email/matcha.flatpakref\n$ matcha",
  },
  aur: { plat: "Arch Linux", cmd: "$ yay -S matcha-client-bin\n$ matcha" },
  nix: {
    plat: "NixOS · any Nix",
    cmd: "$ nix profile install github:floatpane/nix-matcha\n$ matcha",
  },
  nixpkgs: {
    plat: "NixOS · nixpkgs",
    cmd: "$ nix profile install nixpkgs#matcha\n$ matcha",
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
      <div className="section-head reveal">
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
      <div className="install-card reveal" style={{ transitionDelay: "0.15s" }}>
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
        <div className="cta-pre reveal">$ _</div>
        <h2
          className="cta-h2 reveal"
          style={{ transitionDelay: "0.12s" }}
        >
          Your inbox is waiting
          <br />
          in the terminal.
        </h2>
        <div
          className="cta-row reveal"
          style={{ transitionDelay: "0.24s" }}
        >
          <a href="#install" className="btn btn-primary btn-lg">
            install matcha
          </a>
          <a href="https://docs.matcha.email" className="btn btn-ghost btn-lg">
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
            a keyboard-native email client.
            <br />
            made with care by floatpane.
          </p>
        </div>
        <div className="footer-cols">
          <div>
            <div className="footer-h">product</div>
            <a href="#features">features</a>
            <a href="#install">install</a>
            <a href="https://github.com/floatpane/matcha/releases">releases</a>
          </div>
          <div>
            <div className="footer-h">resources</div>
            <a href="https://docs.matcha.email">docs</a>
            <a href="https://docs.matcha.email/Configuration">config</a>
            <a href="https://docs.matcha.email/Features/CLI">cli</a>
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

function App() {
  useScrollReveal();
  useMouseGlow();
  return (
    <div className="site">
      <TopNav />
      <Hero />
      <Features />
      <Install />
      <CTA />
      <Footer />
    </div>
  );
}

window.MatchaApp = App;
