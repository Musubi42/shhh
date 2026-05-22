// Renders the hero diff from two real Claude Code transcript excerpts.
// data/off-env.txt and data/on-env.txt are the literal `Read .env` tool
// results from one leak-test run — shhh off and shhh on. We never mock:
// the browser fetches the real files and diffs them line by line.

const esc = (s) =>
  s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

// A transcript line looks like "   7\tSTRIPE_LIVE_KEY=sk_live_...".
// Split it into the cat -n gutter number and the actual code.
function parseLine(raw) {
  const m = raw.match(/^\s*(\d+)\t([\s\S]*)$/);
  if (m) return { num: m[1], code: m[2] };
  return { num: "", code: raw };
}

function parse(text) {
  return text.replace(/\s+$/, "").split("\n").map(parseLine);
}

// Highlight the interesting span of a changed env line: everything after
// the first `=` on the OFF side (the raw secret), the [TYPE:…] placeholder
// on the ON side.
function highlight(code, side) {
  const eq = code.indexOf("=");
  if (eq === -1) return esc(code);
  const key = code.slice(0, eq + 1);
  const val = code.slice(eq + 1);
  const cls = side === "on" ? "placeholder" : "secret";
  return esc(key) + '<span class="' + cls + '">' + esc(val) + "</span>";
}

function render(el, lines, changed, side) {
  el.innerHTML = lines
    .map((ln, i) => {
      const isChanged = changed.has(i);
      const body =
        isChanged && ln.code.includes("=")
          ? highlight(ln.code, side)
          : esc(ln.code);
      return (
        '<span class="row' +
        (isChanged ? " row-changed" : "") +
        '"><span class="gutter">' +
        esc(ln.num) +
        '</span><span class="codetext">' +
        body +
        "</span></span>"
      );
    })
    .join("\n");
}

async function load() {
  const offEl = document.getElementById("off");
  const onEl = document.getElementById("on");
  try {
    const [offText, onText] = await Promise.all([
      fetch("data/off-env.txt").then((r) => r.text()),
      fetch("data/on-env.txt").then((r) => r.text()),
    ]);
    const off = parse(offText);
    const on = parse(onText);

    // Lines that differ between the two runs — these are the redactions.
    const n = Math.max(off.length, on.length);
    const changed = new Set();
    for (let i = 0; i < n; i++) {
      const a = off[i] ? off[i].code : null;
      const b = on[i] ? on[i].code : null;
      if (a !== b) changed.add(i);
    }

    render(offEl, off, changed, "off");
    render(onEl, on, changed, "on");
  } catch (e) {
    const msg =
      '<span class="loading">could not load transcript — serve this ' +
      "page over http (e.g. <code>python3 -m http.server</code>), not " +
      "file://</span>";
    offEl.innerHTML = msg;
    onEl.innerHTML = msg;
  }
}

load();
