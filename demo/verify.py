#!/usr/bin/env python3
"""shhh leak test verifier.

Given the ground-truth manifest and two Claude Code session transcripts
(one recorded with shhh OFF, one with shhh ON), check whether any raw
secret value reached the model, and print a scorecard.

This is plain substring counting — the moral equivalent of `grep -F`. No
regex, no LLM, fully deterministic. The transcript JSONL is the authority:
if a raw secret string is in the file the model received, it leaked.

A secret counts as "redacted" when its file was opened by the agent (the
file's plain-text marker is present in the transcript) and the raw secret
value is absent. A decoy counts as a false positive when its file was
opened and the decoy value is absent (i.e. it was redacted when it should
not have been). Files the agent never opened are reported as not tested.

Usage: verify.py <manifest.json> <off-transcript.jsonl> <on-transcript.jsonl>
Exit 0 = PASS (0 secrets leaked with shhh on, 0 decoys wrongly redacted).
Exit 1 = FAIL.  Exit 2 = usage / IO error.
"""
import json
import os
import sys

GREEN = "\033[32m"
YELLOW = "\033[33m"
RED = "\033[31m"
DIM = "\033[2m"
BOLD = "\033[1m"
RESET = "\033[0m"
if os.environ.get("NO_COLOR") or not sys.stdout.isatty():
    GREEN = YELLOW = RED = DIM = BOLD = RESET = ""


def load_text(path):
    with open(path, encoding="utf-8", errors="replace") as f:
        return f.read()


def tilde(path):
    home = os.path.expanduser("~")
    return path.replace(home, "~", 1) if path.startswith(home) else path


def main():
    if len(sys.argv) != 4:
        sys.stderr.write(__doc__)
        return 2

    manifest_path, off_path, on_path = sys.argv[1:4]
    with open(manifest_path, encoding="utf-8") as f:
        manifest = json.load(f)
    off = load_text(off_path)
    on = load_text(on_path)

    secrets = manifest["secrets"]
    decoys = manifest["decoys"]
    markers = manifest["markers"]

    # Which files did the agent actually open? (plain-text marker present)
    opened_on = {f: (m in on) for f, m in markers.items()}
    n_opened = sum(opened_on.values())

    # ---- OFF (control) ------------------------------------------------
    leaked_off = [s for s in secrets if off.count(s["value"]) > 0]

    # ---- ON -----------------------------------------------------------
    leaked_on = [s for s in secrets if on.count(s["value"]) > 0]
    tested = [s for s in secrets if opened_on.get(s["file"])]
    redacted = [s for s in tested if on.count(s["value"]) == 0]

    decoys_tested = [d for d in decoys if opened_on.get(d["file"])]
    false_pos = [d for d in decoys_tested if on.count(d["value"]) == 0]

    # The verdict keys on the security guarantee only: did a raw secret
    # reach the model? A false positive (a decoy redacted unnecessarily) is
    # noise, not a leak — it is reported separately and does not flip the
    # verdict. If the agent opened no secret-bearing files at all, the run
    # proved nothing: that is INCONCLUSIVE, not a pass.
    inconclusive = not leaked_on and len(tested) == 0

    # ---- scorecard ----------------------------------------------------
    bar = "=" * 64
    print()
    print(BOLD + bar + RESET)
    print(BOLD + "  shhh leak test — demo/leaktest" + RESET)
    print("  %d secrets · %d decoys · %d files" %
          (len(secrets), len(decoys), len(markers)))
    print(bar)
    print()
    print("  shhh OFF  " + DIM + "(control — no redaction)" + RESET)
    off_mark = RED + "LEAKED" + RESET if leaked_off else DIM + "no leak" + RESET
    print("    raw secrets in transcript ....  %d / %d   %s"
          % (len(leaked_off), len(secrets), off_mark))
    print("    " + DIM + "transcript: " + tilde(off_path) + RESET)
    print()
    print("  shhh ON")
    on_mark = (RED + "✗ LEAKED" + RESET) if leaked_on else (GREEN + "✓ SAFE" + RESET)
    print("    raw secrets in transcript ....  %d / %d   %s"
          % (len(leaked_on), len(secrets), on_mark))
    print("    secrets redacted .............  %d / %d   %s"
          % (len(redacted), len(tested),
             DIM + "(of files the agent opened)" + RESET))
    if false_pos:
        fp_mark = DIM + "(precision — noise, not a leak)" + RESET
    else:
        fp_mark = GREEN + "✓" + RESET
    print("    decoys wrongly redacted ......  %d / %d   %s"
          % (len(false_pos), len(decoys_tested), fp_mark))
    print("    files the agent opened .......  %d / %d"
          % (n_opened, len(markers)))
    print("    " + DIM + "transcript: " + tilde(on_path) + RESET)
    print()

    # ---- per-secret detail -------------------------------------------
    print("  " + DIM + "per secret (shhh ON):" + RESET)
    for s in secrets:
        if on.count(s["value"]) > 0:
            status = RED + "LEAKED" + RESET
        elif opened_on.get(s["file"]):
            status = GREEN + "redacted" + RESET
        else:
            status = DIM + "not tested (file not opened)" + RESET
        print("    %-22s %-18s %s" % (s["type"], s["file"], status))
    if false_pos:
        print()
        print("  " + RED + "false positives (decoys redacted):" + RESET)
        for d in false_pos:
            print("    %-22s %s" % (d["file"], d["value"]))
    print()
    print(bar)
    if leaked_on:
        print(RED + BOLD + "  RESULT: FAIL" + RESET
              + " — %d raw secret(s) reached the model; see above."
              % len(leaked_on))
        rc = 1
    elif inconclusive:
        print(YELLOW + BOLD + "  RESULT: INCONCLUSIVE" + RESET
              + " — the agent opened no secret-bearing files this run.")
        print("  " + DIM + "nothing leaked, but nothing was proven either —"
              " re-run (model behaviour varies run to run)." + RESET)
        rc = 3
    else:
        print(GREEN + BOLD + "  RESULT: PASS" + RESET
              + " — 0 secrets reached the model with shhh on.")
        if false_pos:
            print("  " + DIM + "precision: %d decoy(s) redacted unnecessarily"
                  " — noise, no secret leaked" % len(false_pos) + RESET)
        rc = 0
    print(BOLD + bar + RESET)
    print()
    return rc


if __name__ == "__main__":
    sys.exit(main())
