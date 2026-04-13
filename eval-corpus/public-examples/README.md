# Public-example corpus

Files in this directory are **known-safe strings** that should never be
flagged as secrets. They are the calibration corpus for eval task 8
(false-positive rate).

Contents:

- `gitsha.txt` — git commit SHAs and release tags. High-hex-entropy but public.
- `uuids.txt` — UUIDs from migrations and request IDs. Structured but not secrets.
- `package-lock-excerpt.json` — `package-lock.json` integrity hashes. Long, high-entropy, and entirely public.
- `version-constants.txt` — version numbers, hostnames, and explicitly low-entropy values (`password`, `changeme`, `localhost`). These verify the denylist and entropy gates.
- `aws-docs-placeholder.txt` — the AWS-documented `AKIAIOSFODNN7EXAMPLE` credentials. These match the pattern by design; task 8 expects the redactor to *either* skip them via a known-placeholder allowlist *or* flag them and be penalized. The Phase 0 policy is documented in the task's rubric.

Task 8 runs the detector over every file in this directory and counts any
non-empty finding list as a false positive.

**Do not add real secrets here under any circumstances.** This directory is
part of the public eval corpus and will be published in Phase 1.
