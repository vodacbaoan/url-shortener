"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";

type User = {
  id: number;
  email: string;
  created_at: string;
};

type OwnedLink = {
  short_code: string;
  target_url: string;
  click_count: number;
  created_at: string;
};

type ShortenResponse = {
  short_code: string;
};

const fallbackAPIBase = "http://localhost:8081";

function trimTrailingSlash(value: string) {
  return value.replace(/\/+$/, "");
}

async function readError(response: Response, fallback: string) {
  const message = (await response.text()).trim();
  return message || fallback;
}

export default function Home() {
  const apiBase = useMemo(
    () => trimTrailingSlash(process.env.NEXT_PUBLIC_API_URL || fallbackAPIBase),
    [],
  );

  const [user, setUser] = useState<User | null>(null);
  const [links, setLinks] = useState<OwnedLink[]>([]);
  const [authMode, setAuthMode] = useState<"login" | "signup">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [longURL, setLongURL] = useState("");
  const [statsSearch, setStatsSearch] = useState("");
  const [shortLink, setShortLink] = useState("");
  const [status, setStatus] = useState("");
  const [busy, setBusy] = useState(false);
  const statsFilter = statsSearch.trim().toLowerCase();
  const filteredStatsLinks = useMemo(() => {
    if (!statsFilter) {
      return links;
    }

    return links.filter((link) => link.short_code.toLowerCase().includes(statsFilter));
  }, [links, statsFilter]);

  function apiURL(path: string) {
    return `${apiBase}${path}`;
  }

  async function apiFetch(path: string, init: RequestInit = {}, allowRefresh = true) {
    const response = await fetch(apiURL(path), {
      ...init,
      credentials: "include",
    });

    if (response.status !== 401 || !allowRefresh || path === "/auth/refresh") {
      return response;
    }

    const refreshResponse = await fetch(apiURL("/auth/refresh"), {
      method: "POST",
      credentials: "include",
    });

    if (!refreshResponse.ok) {
      return response;
    }

    return fetch(apiURL(path), {
      ...init,
      credentials: "include",
    });
  }

  async function loadLinks() {
    const response = await apiFetch("/links");
    if (response.status === 401) {
      setUser(null);
      setLinks([]);
      return;
    }
    if (!response.ok) {
      setStatus(await readError(response, "Unable to load links."));
      return;
    }

    setLinks((await response.json()) as OwnedLink[]);
  }

  async function loadSession() {
    const response = await apiFetch("/me");
    if (response.status === 401) {
      setUser(null);
      setLinks([]);
      setStatus("");
      return;
    }
    if (!response.ok) {
      setStatus("Unable to check session.");
      return;
    }

    const currentUser = (await response.json()) as User;
    setUser(currentUser);
    setStatus("");
    await loadLinks();
  }

  useEffect(() => {
    void loadSession();
  }, []);

  useEffect(() => {
    if (!user) {
      return;
    }

    function refreshLinks() {
      if (document.visibilityState === "visible") {
        void loadLinks();
      }
    }

    window.addEventListener("focus", refreshLinks);
    document.addEventListener("visibilitychange", refreshLinks);

    return () => {
      window.removeEventListener("focus", refreshLinks);
      document.removeEventListener("visibilitychange", refreshLinks);
    };
  }, [user]);

  async function submitAuth(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setStatus(authMode === "signup" ? "Creating account..." : "Logging in...");

    try {
      const response = await fetch(apiURL(`/auth/${authMode}`), {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: email.trim(), password }),
      });

      if (!response.ok) {
        setStatus(await readError(response, "Authentication failed."));
        return;
      }

      const currentUser = (await response.json()) as User;
      setUser(currentUser);
      setEmail("");
      setPassword("");
      setShortLink("");
      setStatsSearch("");
      setStatus("");
      await loadLinks();
    } catch {
      setStatus("Network error while authenticating.");
    } finally {
      setBusy(false);
    }
  }

  async function logout() {
    setBusy(true);

    try {
      await fetch(apiURL("/auth/logout"), {
        method: "POST",
        credentials: "include",
      });
    } finally {
      setUser(null);
      setLinks([]);
      setShortLink("");
      setStatsSearch("");
      setStatus("");
      setBusy(false);
    }
  }

  async function submitShorten(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!user) {
      setStatus("Log in to create short links.");
      return;
    }

    setBusy(true);
    setShortLink("");
    setStatus("");

    try {
      const response = await apiFetch("/shorten", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ url: longURL.trim() }),
      });

      if (response.status === 401) {
        setUser(null);
        setStatus("Session expired. Log in again.");
        return;
      }
      if (!response.ok) {
        setStatus(await readError(response, "Unable to create short link."));
        return;
      }

      const payload = (await response.json()) as ShortenResponse;
      const nextShortLink = `${apiBase}/${payload.short_code}`;
      setShortLink(nextShortLink);
      setLongURL("");
      setStatsSearch("");
      setStatus("Short link created.");
      await loadLinks();
    } catch {
      setStatus("Network error while creating short link.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="shell">
      <section className="workspace">
        <header className="page-header">  
          <div>
            <h1>URL Shortener</h1>
          </div>
          {user ? (
            <div className="user-menu">
              <span>
                Logged in as <strong>{user.email}</strong>
              </span>
              <button className="secondary" type="button" onClick={logout} disabled={busy}>
                Log out
              </button>
            </div>
          ) : (
            <form className="header-auth" onSubmit={submitAuth}>
              <div className="auth-tabs" role="tablist" aria-label="Auth mode">
                <button
                  className={authMode === "login" ? "active" : ""}
                  type="button"
                  onClick={() => setAuthMode("login")}
                >
                  Log in
                </button>
                <button
                  className={authMode === "signup" ? "active" : ""}
                  type="button"
                  onClick={() => setAuthMode("signup")}
                >
                  Sign up
                </button>
              </div>
              <input
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="you@example.com"
                aria-label="Email"
                required
              />
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Password"
                aria-label="Password"
                required
              />
              <button type="submit" disabled={busy}>
                {authMode === "signup" ? "Create account" : "Log in"}
              </button>
            </form>
          )}
        </header>

        {status ? (
          <div className="status" aria-live="polite">
            {status}
          </div>
        ) : null}

        <div className="grid">
          <section className="panel shorten-panel">
            <div className="panel-heading">
              <h2>Shorten</h2>
            </div>
            <form className="stack" onSubmit={submitShorten}>
              <label>
                Long URL
                <input
                  type="url"
                  value={longURL}
                  onChange={(event) => setLongURL(event.target.value)}
                  placeholder="https://example.com/article"
                  disabled={!user || busy}
                  required
                />
              </label>
              <button type="submit" disabled={!user || busy}>
                Create link
              </button>
            </form>

            {shortLink ? (
              <div className="result">
                <span>Short link</span>
                <a href={shortLink} target="_blank" rel="noreferrer">
                  {shortLink}
                </a>
              </div>
            ) : null}
          </section>

          <section className="panel links-panel">
            <div className="panel-heading">
              <h2>My links</h2>
              <span className="count">{links.length}</span>
            </div>
            {links.length === 0 ? (
              <p className="muted">{user ? "No links yet." : "Log in to see your links."}</p>
            ) : (
              <ul className="links-list">
                {links.map((link) => {
                  const shortURL = `${apiBase}/${link.short_code}`;

                  return (
                    <li key={link.short_code}>
                      <a href={shortURL} target="_blank" rel="noreferrer">
                        {shortURL}
                      </a>
                      <span>{link.target_url}</span>
                      <small>
                        {link.click_count} clicks - {new Date(link.created_at).toLocaleString()}
                      </small>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>

          <section className="panel stats-panel">
            <div className="panel-heading">
              <h2>Stats</h2>
            </div>
            <label>
              Short code
              <input
                type="text"
                value={statsSearch}
                onChange={(event) => setStatsSearch(event.target.value)}
                placeholder="Filter by short code"
                disabled={!user || links.length === 0}
              />
            </label>

            {!user ? <p className="muted">Log in to view stats.</p> : null}
            {user && links.length === 0 ? <p className="muted">No link stats yet.</p> : null}
            {user && links.length > 0 && filteredStatsLinks.length === 0 ? (
              <p className="muted">No matching short codes.</p>
            ) : null}
            {user && filteredStatsLinks.length > 0 ? (
              <dl className="stats">
                {filteredStatsLinks.map((link) => (
                  <div key={link.short_code}>
                    <dt>{link.short_code}</dt>
                    <dd>
                      <a href={`${apiBase}/${link.short_code}`} target="_blank" rel="noreferrer">
                        {`${apiBase}/${link.short_code}`}
                      </a>
                    </dd>
                    <dd>{link.click_count} clicks</dd>
                    <dd>{new Date(link.created_at).toLocaleString()}</dd>
                  </div>
                ))}
              </dl>
            ) : null}
          </section>
        </div>
      </section>
    </main>
  );
}
