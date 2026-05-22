# web/ — the shhh demo site

A single static page that shows, with a **real Claude Code session
transcript**, what a coding agent sends to the model provider when it
reads a `.env` — with and without shhh.

No framework, no build step. `index.html` + `style.css` + `app.js`.

## The data is real

`data/off-env.txt` and `data/on-env.txt` are the literal `Read .env`
tool results pulled from one `demo/run.sh` leak-test run — shhh off and
shhh on. `app.js` fetches them and diffs them in the browser. To refresh
them, run the leak test and re-extract the `.env` tool result from the
two transcripts.

## Preview locally

`fetch()` needs http, not `file://`:

```sh
cd web && python3 -m http.server 8000
# open http://localhost:8000
```

## Deploy

Pushed to `main`, `.github/workflows/pages.yml` publishes this directory
to GitHub Pages automatically.
