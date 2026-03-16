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
    <p>Create a short link or check stats for an existing short code.</p>

    <h2>Shorten a URL</h2>
    <form id="shorten-form">
      <label for="long-url">Long URL</label><br>
      <input id="long-url" name="url" type="url" placeholder="https://example.com/article" required><br><br>
      <button type="submit">Create short link</button>
    </form>
    <div id="shorten-result" aria-live="polite"></div>

    <hr>

    <h2>Lookup stats</h2>
    <form id="stats-form">
      <label for="short-code">Short code</label><br>
      <input id="short-code" name="short_code" type="text" placeholder="Ab12Cd" required><br><br>
      <button type="submit">Load stats</button>
    </form>
    <div id="stats-result" aria-live="polite"></div>

    <hr>

    <p>API endpoints: <code>POST /shorten</code>, <code>GET /stats/{code}</code>, <code>GET /{code}</code>, <code>GET /healthz</code></p>
  </main>

  <script>
    const shortenForm = document.getElementById("shorten-form");
    const shortenResult = document.getElementById("shorten-result");
    const statsForm = document.getElementById("stats-form");
    const statsResult = document.getElementById("stats-result");

    function showResult(element, html, isError) {
      element.innerHTML = isError ? "<p>" + html + "</p>" : html;
    }

    shortenForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      const formData = new FormData(shortenForm);
      const rawURL = String(formData.get("url") || "").trim();

      try {
        const response = await fetch("/shorten", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ url: rawURL })
        });

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
      } catch (error) {
        showResult(shortenResult, "Network error while creating short link.", true);
      }
    });

    statsForm.addEventListener("submit", async function (event) {
      event.preventDefault();
      const formData = new FormData(statsForm);
      const shortCode = String(formData.get("short_code") || "").trim();

      try {
        const response = await fetch("/stats/" + encodeURIComponent(shortCode));

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
  </script>
</body>
</html>
`

func serveHomePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(homePageHTML))
}
