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

type StatsResponse = {
  short_code: string;
  target_url: string;
  click_count: number;
  created_at: string;
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
  const [statsCode, setStatsCode] = useState("");
  const [shortLink, setShortLink] = useState("");
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [status, setStatus] = useState("Checking session...");
  const [busy, setBusy] = useState(false);

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
      setStatus("Not logged in.");
      return;
    }
    if (!response.ok) {
      setStatus("Unable to check session.");
      return;
    }

    const currentUser = (await response.json()) as User;
    setUser(currentUser);
    setStatus(`Logged in as ${currentUser.email}`);
    await loadLinks();
  }

  useEffect(() => {
    void loadSession();
  }, []);

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
      setStats(null);
      setStatus(`Logged in as ${currentUser.email}`);
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
      setStats(null);
      setStatus("Logged out.");
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
      setStatus("Short link created.");
      await loadLinks();
    } catch {
      setStatus("Network error while creating short link.");
    } finally {
      setBusy(false);
    }
  }

  async function submitStats(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!user) {
      setStatus("Log in to view stats.");
      return;
    }

    setBusy(true);
    setStats(null);

    try {
      const response = await apiFetch(`/stats/${encodeURIComponent(statsCode.trim())}`);
      if (response.status === 401) {
        setUser(null);
        setStatus("Session expired. Log in again.");
        return;
      }
      if (!response.ok) {
        setStatus(await readError(response, "Unable to load stats."));
        return;
      }

      setStats((await response.json()) as StatsResponse);
      setStatus("Stats loaded.");
    } catch {
      setStatus("Network error while loading stats.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <main className="shell">
      <section className="workspace">
        <header className="masthead">
          <div>
            <p className="eyebrow">Go API + Next.js</p>
            <h1>URL Shortener</h1>
          </div>
          <div className="status" aria-live="polite">
            {status}
          </div>
        </header>

        <div className="grid">
          <section className="panel account-panel">
            <div className="panel-heading">
              <h2>Account</h2>
              {user ? (
                <button className="secondary" type="button" onClick={logout} disabled={busy}>
                  Log out
                </button>
              ) : null}
            </div>

            {user ? (
              <div className="signed-in">
                <span>{user.email}</span>
                <small>Member since {new Date(user.created_at).toLocaleDateString()}</small>
              </div>
            ) : (
              <>
                <div className="segments" role="tablist" aria-label="Auth mode">
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

                <form className="stack" onSubmit={submitAuth}>
                  <label>
                    Email
                    <input
                      type="email"
                      value={email}
                      onChange={(event) => setEmail(event.target.value)}
                      placeholder="you@example.com"
                      required
                    />
                  </label>
                  <label>
                    Password
                    <input
                      type="password"
                      value={password}
                      onChange={(event) => setPassword(event.target.value)}
                      placeholder="At least 8 characters"
                      required
                    />
                  </label>
                  <button type="submit" disabled={busy}>
                    {authMode === "signup" ? "Create account" : "Log in"}
                  </button>
                </form>
              </>
            )}
          </section>

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
                        {link.click_count} clicks · {new Date(link.created_at).toLocaleString()}
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
            <form className="inline-form" onSubmit={submitStats}>
              <label>
                Short code
                <input
                  type="text"
                  value={statsCode}
                  onChange={(event) => setStatsCode(event.target.value)}
                  placeholder="Ab12Cd"
                  disabled={!user || busy}
                  required
                />
              </label>
              <button type="submit" disabled={!user || busy}>
                Load
              </button>
            </form>

            {stats ? (
              <dl className="stats">
                <div>
                  <dt>Target</dt>
                  <dd>
                    <a href={stats.target_url} target="_blank" rel="noreferrer">
                      {stats.target_url}
                    </a>
                  </dd>
                </div>
                <div>
                  <dt>Clicks</dt>
                  <dd>{stats.click_count}</dd>
                </div>
                <div>
                  <dt>Created</dt>
                  <dd>{new Date(stats.created_at).toLocaleString()}</dd>
                </div>
              </dl>
            ) : null}
          </section>
        </div>
      </section>
    </main>
  );
}
