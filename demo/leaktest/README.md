# acme-payments

Internal payments service. Handles Stripe charges, writes receipts to S3,
and records transactions in Postgres.

## Setup

Copy `.env.example` to `.env` and fill in credentials. Application config is
assembled in `src/config.py`; database wiring is in `src/db.js`.

## Layout

- `.env`            — runtime configuration
- `src/config.py`   — service configuration
- `src/db.js`       — database connection pools
- `src/utils.py`    — shared helpers
- `credentials.json` — Google API service-account credentials
