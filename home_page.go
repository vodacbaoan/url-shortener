package main

import "net/http"

const homePageHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>URL Shortener</title>
</head>
<body>
  <main>
    <h1>URL Shortener</h1>
    <p>Sign up or log in to create links, see your links, and view private stats.</p>

    <h2>Account</h2>
    <div id="auth-status" aria-live="polite"><p>Checking session...</p></div>

    <form id="signup-form">
      <h3>Sign up</h3>
      <label for="signup-email">Email</label><br>
      <input id="signup-email" name="email" type="email" placeholder="you@example.com" required><br><br>
      <label for="signup-password">Password</label><br>
      <input id="signup-password" name="password" type="password" placeholder="At least 8 characters" required><br><br>
      <button id="signup-submit" type="submit">Create account</button>
    </form>

    <br>

    <form id="login-form">
      <h3>Log in</h3>
      <label for="login-email">Email</label><br>
      <input id="login-email" name="email" type="email" placeholder="you@example.com" required><br><br>
      <label for="login-password">Password</label><br>
      <input id="login-password" name="password" type="password" placeholder="Password" required><br><br>
      <button id="login-submit" type="submit">Log in</button>
    </form>

    <br>

    <button id="logout-button" type="button" hidden>Log out</button>

    <hr>

    <h2>My links</h2>
    <div id="links-result" aria-live="polite"><p>Log in to see your links.</p></div>

    <hr>

    <h2>Shorten a URL</h2>
    <form id="shorten-form">
      <label for="long-url">Long URL</label><br>
      <input id="long-url" name="url" type="url" placeholder="https://example.com/article" required disabled><br><br>
      <button id="shorten-submit" type="submit" disabled>Create short link</button>
    </form>
    <div id="shorten-result" aria-live="polite"></div>

    <hr>

    <h2>Lookup stats</h2>
    <form id="stats-form">
      <label for="short-code">Short code</label><br>
      <input id="short-code" name="short_code" type="text" placeholder="Ab12Cd" required disabled><br><br>
      <button id="stats-submit" type="submit" disabled>Load stats</button>
    </form>
    <div id="stats-result" aria-live="polite"></div>

    <hr>

    <p>API endpoints: <code>POST /auth/signup</code>, <code>POST /auth/login</code>, <code>POST /shorten</code>, <code>GET /links</code>, <code>GET /stats/{code}</code>, <code>GET /{code}</code>, <code>GET /healthz</code></p>
  </main>

  <script>
    const authStatus = document.getElementById("auth-status");
    const signupForm = document.getElementById("signup-form");
    const loginForm = document.getElementById("login-form");
    const logoutButton = document.getElementById("logout-button");
    const linksResult = document.getElementById("links-result");
    const shortenForm = document.getElementById("shorten-form");
    const shortenResult = document.getElementById("shorten-result");
    const statsForm = document.getElementById("stats-form");
    const statsResult = document.getElementById("stats-result");

    let currentUser = null;

    function showResult(element, html, isError) {
      element.innerHTML = isError ? "<p>" + html + "</p>" : html;
    }

    function setProtectedState(enabled) {
      document.getElementById("long-url").disabled = !enabled;
      document.getElementById("shorten-submit").disabled = !enabled;
      document.getElementById("short-code").disabled = !enabled;
      document.getElementById("stats-submit").disabled = !enabled;
    }

    function setLoggedOut(message) {
      currentUser = null;
      signupForm.hidden = false;
      loginForm.hidden = false;
      logoutButton.hidden = true;
      setProtectedState(false);
      authStatus.innerHTML = message ? "<p>" + message + "</p>" : "<p>Not logged in.</p>";
      linksResult.innerHTML = "<p>Log in to see your links.</p>";
    }

    function setLoggedIn(user) {
      currentUser = user;
      signupForm.hidden = true;
      loginForm.hidden = true;
      logoutButton.hidden = false;
      setProtectedState(true);
      authStatus.innerHTML = "<p>Logged in as <strong>" + user.email + "</strong></p>";
    }

    async function apiFetch(url, options, allowRefresh) {
      const response = await fetch(url, options);
      if (response.status !== 401 || allowRefresh === false || url === "/auth/refresh") {
        return response;
      }

      const refreshResponse = await fetch("/auth/refresh", { method: "POST" });
      if (!refreshResponse.ok) {
        return response;
      }

      return fetch(url, options);
    }

    async function loadLinks() {
      if (!currentUser) {
        linksResult.innerHTML = "<p>Log in to see your links.</p>";
        return;
      }

      const response = await apiFetch("/links");
      if (response.status === 401) {
        setLoggedOut("Session expired. Log in again.");
        return;
      }
      if (!response.ok) {
        const message = await response.text();
        showResult(linksResult, message || "Unable to load links.", true);
        return;
      }

      const links = await response.json();
      if (!Array.isArray(links) || links.length === 0) {
        linksResult.innerHTML = "<p>No links yet.</p>";
        return;
      }

      const items = links.map(function (link) {
        const shortURL = window.location.origin + "/" + link.short_code;
        return "<li>" +
          "<strong><a href=\"" + shortURL + "\" target=\"_blank\" rel=\"noreferrer\">" + shortURL + "</a></strong><br>" +
          "Target: <a href=\"" + link.target_url + "\" target=\"_blank\" rel=\"noreferrer\">" + link.target_url + "</a><br>" +
          "Clicks: <code>" + link.click_count + "</code><br>" +
          "Created: <code>" + new Date(link.created_at).toLocaleString() + "</code>" +
          "</li>";
      }).join("");

      linksResult.innerHTML = "<ul>" + items + "</ul>";
    }

    async function loadSession() {
      const response = await apiFetch("/me");
      if (response.status === 401) {
        setLoggedOut();
        return;
      }
      if (!response.ok) {
        setLoggedOut("Unable to check session.");
        return;
      }

      const user = await response.json();
      setLoggedIn(user);
      await loadLinks();
    }

    async function submitAuthForm(event, endpoint) {
      event.preventDefault();
      const form = event.currentTarget;
      const formData = new FormData(form);
      const email = String(formData.get("email") || "").trim();
      const password = String(formData.get("password") || "");

      const response = await fetch(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: email, password: password })
      });

      if (!response.ok) {
        const message = await response.text();
        showResult(authStatus, message || "Authentication failed.", true);
        return;
      }

      const user = await response.json();
      form.reset();
      shortenResult.innerHTML = "";
      statsResult.innerHTML = "";
      setLoggedIn(user);
      await loadLinks();
    }

    signupForm.addEventListener("submit", async function (event) {
      await submitAuthForm(event, "/auth/signup");
    });

    loginForm.addEventListener("submit", async function (event) {
      await submitAuthForm(event, "/auth/login");
    });

    logoutButton.addEventListener("click", async function () {
      await fetch("/auth/logout", { method: "POST" });
      shortenResult.innerHTML = "";
      statsResult.innerHTML = "";
      setLoggedOut("Logged out.");
    });

    shortenForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      if (!currentUser) {
        showResult(shortenResult, "Log in to create short links.", true);
        return;
      }

      const formData = new FormData(shortenForm);
      const rawURL = String(formData.get("url") || "").trim();

      try {
        const response = await apiFetch("/shorten", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ url: rawURL })
        });

        if (response.status === 401) {
          setLoggedOut("Session expired. Log in again.");
          showResult(shortenResult, "Log in to create short links.", true);
          return;
        }
        if (!response.ok) {
          const message = await response.text();
          showResult(shortenResult, message || "Unable to create short link.", true);
          return;
        }

        const payload = await response.json();
        const shortURL = window.location.origin + "/" + payload.short_code;
        showResult(
          shortenResult,
          "<p><strong>Short link ready</strong></p>" +
            "<p><a href=\"" + shortURL + "\" target=\"_blank\" rel=\"noreferrer\">" + shortURL + "</a></p>" +
            "<p>Code: <code>" + payload.short_code + "</code></p>",
          false
        );
        shortenForm.reset();
        await loadLinks();
      } catch (error) {
        showResult(shortenResult, "Network error while creating short link.", true);
      }
    });

    statsForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      if (!currentUser) {
        showResult(statsResult, "Log in to view stats.", true);
        return;
      }

      const formData = new FormData(statsForm);
      const shortCode = String(formData.get("short_code") || "").trim();

      try {
        const response = await apiFetch("/stats/" + encodeURIComponent(shortCode));

        if (response.status === 401) {
          setLoggedOut("Session expired. Log in again.");
          showResult(statsResult, "Log in to view stats.", true);
          return;
        }
        if (!response.ok) {
          const message = await response.text();
          showResult(statsResult, message || "Unable to load stats.", true);
          return;
        }

        const payload = await response.json();
        showResult(
          statsResult,
          "<p><strong>Stats for " + payload.short_code + "</strong></p>" +
            "<p>Target: <a href=\"" + payload.target_url + "\" target=\"_blank\" rel=\"noreferrer\">" + payload.target_url + "</a></p>" +
            "<p>Clicks: <code>" + payload.click_count + "</code></p>" +
            "<p>Created: <code>" + new Date(payload.created_at).toLocaleString() + "</code></p>",
          false
        );
      } catch (error) {
        showResult(statsResult, "Network error while loading stats.", true);
      }
    });

    loadSession();
  </script>
</body>
</html>
`

func serveHomePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(homePageHTML))
}
