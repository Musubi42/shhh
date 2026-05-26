# hook-playground

A manual test fixture for the `shhh` Claude Code hook. Use this to
reproduce real-agent behavior: redaction on Read, Read→Edit tracking,
Bash interception, and narration visibility.

**This folder contains fake-but-realistic-looking secrets.** They are
not live credentials. Do not add real ones.

## Setup

From the repo root:

```sh
make build
./bin/shhh install claude-code
cd testdata/fixtures/hook-playground
claude
```

Run each prompt below in a fresh `claude` session (or `/clear` between
runs) so session state doesn't carry over.

---

## Test 1 — basic redaction on Read

**Prompt:**
> Read the file `.env` and tell me what environment variables are defined.

**Expected:**
- Claude sees `[STRIPE_LIVE_KEY:sk_live_...:<hash>]` etc., not raw values.
- Claude mentions that shhh protected secrets (from the narration note).
- Variable names (`STRIPE_LIVE_KEY`, `DATABASE_URL`, …) are intact.

**Fails if:** raw `sk_live_...` values appear in Claude's response.

RESULTS : 
Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)          
  ⎿  Read 12 lines                                                                            
                                                                                                
                                                              11:18 AM claude-sonnet-4-6        
⏺ The .env file defines these environment variables:                                            
                                                                                                
  ┌───────────────────────┬────────────────────────────┐                                        
  │       Variable        │            Type            │                                        
  ├───────────────────────┼────────────────────────────┤                                        
  │ STRIPE_LIVE_KEY       │ Stripe live key            │                                        
  ├───────────────────────┼────────────────────────────┤
  │ AWS_ACCESS_KEY_ID     │ AWS access key             │                                        
  ├───────────────────────┼────────────────────────────┤                                        
  │ AWS_SECRET_ACCESS_KEY │ AWS secret key             │
  ├───────────────────────┼────────────────────────────┤
  │ GITHUB_TOKEN          │ GitHub PAT                 │
  ├───────────────────────┼────────────────────────────┤
  │ OPENAI_API_KEY        │ OpenAI project key         │
  ├───────────────────────┼────────────────────────────┤
  │ ANTHROPIC_API_KEY     │ Anthropic key              │
  ├───────────────────────┼────────────────────────────┤
  │ DATABASE_URL          │ Postgres connection string │
  ├───────────────────────┼────────────────────────────┤
  │ JWT_SESSION_TOKEN     │ JWT token                  │
  ├───────────────────────┼────────────────────────────┤
  │ APP_ENV               │ production                 │
  ├───────────────────────┼────────────────────────────┤
  │ APP_PORT              │ 3000                       │
  ├───────────────────────┼────────────────────────────┤
  │ LOG_LEVEL             │ info                       │
  └───────────────────────┴────────────────────────────┘                                        
                                  
  shhh protected your secrets during this Read — all 8 real values were replaced with           
  placeholders before reaching me.

---

## Test 2 — Read→Edit tracking (the known bug)

**Prompt:**
> Read `.env`, then add a new line `FEATURE_FLAG_NEW_UI=true` at the end.

**Status (2026-05-26): documented limit, not a fix target.**
This fixture remains the canonical repro for the Read→Edit
ledger limit. Option D shipped instead of a code fix — see
`docs/known-limitations.md` and the design doc
`docs/design/read-edit-tracking.md` for why the three viable
hook-API strategies are impossible. The `narrateRedactions`
output now tells Claude to use `Bash` (sed -i, tee, printf >>,
python -c) directly on any redacted file, so a *fresh* run of
this test should see Claude reach for `Bash` on the first try
without two failed `Edit` attempts.

**Option D outcome to look for in a fresh transcript:**
- Read on `.env` produces the redaction narration with the
  "use Bash" guidance.
- The next write is a `Bash` call (sed / tee / printf), NOT a
  failed `Edit` retry loop.
- The original "File has not been read yet" error block should
  disappear from the transcript entirely.

The RESULTS block below predates the `narrateRedactions` Bash
nudge — it shows the *old* failure mode (two failed Edits, then
Claude reaching for Bash on its own). Replace with a new
transcript on the next interactive dryrun.

RESULTS : 
Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/leaky-project/.
  env)                                                                                        
                                                                                     
⏺ Search(pattern: "**/.env")                                                                    
  ⎿  Found 1 file                                                                          
     .env                                                                                       
                                                                                                
⏺ Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)          
  ⎿  Read 12 lines                                                                              
                                                                                                
                                                              11:19 AM claude-sonnet-4-6        
⏺ shhh protected 8 secrets during this Read — the raw values never left your machine.         
                                                                                                
⏺ Update(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)        
  ⎿  Error: File has not been read yet. Read it first before writing to it.                     
                                                                                                
⏺ Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)
  ⎿  Read 12 lines                                                                              
                                                                                                
⏺ Update(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)        
  ⎿  Error: File has not been read yet. Read it first before writing to it.                     
                                                                                                
⏺ Bash(echo "FEATURE_FLAG_NEW_UI=true" >>                                                       
      /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)
  ⎿  (No output)

                                                              11:19 AM claude-sonnet-4-6
⏺ Done. FEATURE_FLAG_NEW_UI=true has been appended to .env.

---

## Test 3 — Edit on a clean file

**Prompt:**
> Read `notes.md` and add a bullet "Write the launch post" to the Todo list.

**Expected:**
- No redaction, no narration.
- Edit succeeds first try. (If this *also* fails, the Read→Edit break
  is broader than the file-path rewrite — it would mean the hook is
  interfering even when no secrets are present.)

RESULTS : 
Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/notes.md)      
  ⎿  Read 11 lines                                                                              
                                                                                                
⏺ Update(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/notes.md)
  ⎿  Added 1 line                                                                               
       8  - Wire up the billing webhook                                                         
       9  - Backfill the user table                                                             
      10  - Finish the dashboard spec                                                           
      11 +- Write the launch post                                                               
                                                                                                
                                                              11:20 AM claude-sonnet-4-6
⏺ Done. Added "Write the launch post" to the Todo list in notes.md. 

---

## Test 4 — hardcoded secret in source

**Prompt:**
> Open `src/legacy_config.py` and tell me if anything looks risky.

**Expected:**
- Claude sees placeholder tokens where `sk_live_...`, `ghp_...`, and the
  Postgres URL used to be.
- Claude correctly flags the pattern as "hardcoded secrets" even though
  it can't see the raw values.

RESULTS : 
Search(pattern: "**/legacy_config.py")                                                        
  ⎿  Found 1 file                                                                               
     src/legacy_config.py                                                                       
                                                                                                
⏺ Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/src/legacy_conf
  ig.py)                                                                                        
  ⎿  Read 9 lines                                                                               
                                                                                                
                                                              11:21 AM claude-sonnet-4-6        
⏺ shhh intercepted this read and replaced 3 real secrets with placeholders before I saw the file
   — STRIPE_LIVE_KEY, GITHUB_PAT, and POSTGRES_CONNSTRING never left your machine.              
                                                                                                
  ---                                                                                           
  Risks in src/legacy_config.py:                                                                
                                                                                                
  1. Hardcoded secrets at module level (lines 1–4) — credentials assigned directly to           
  module-level variables means any import legacy_config exposes them in memory. Use environment 
  variables or a secrets manager instead.                                                       
  2. connect() returns the raw connection string (line 8) — the caller gets the full credentials
   string back. If that return value is ever logged, printed, or serialized, the secret leaks.  
  The function should return a connection object, not the credential string.                    
  3. No access control on these values — since they're plain module attributes, any code in the 
  process can do legacy_config.STRIPE_KEY. There's no encapsulation.                            
  4. "legacy" naming suggests this is kept around "just in case" — dead config files with real  
  credentials are a common source of accidental leaks (committed to git, copied into CI, etc.). 
  If this file isn't actively used, it should be deleted.                                       
                                               
  The immediate fix is to move all three values to environment variables and remove this file   
  from version control history if it ever held real values.


---

## Test 5 — safe source file passes through

**Prompt:**
> Read `src/math_utils.py` and add a `subtract(a, b)` function.

**Expected:**
- No redaction, no narration.
- Edit succeeds.

RESULTS : 
Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/src/math_utils.
  py)                                                                                         
  ⎿  Read 13 lines                                                                              
                                                                                                
⏺ Update(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/src/math_util
        s.py)                                                                                   
  ⎿  Added 4 lines                                                                              
       2      return a + b                                                                      
       3                                                                                        
       4                                                                                        
       5 +def subtract(a, b):                                                                   
       6 +    return a - b                                                                      
       7 +                                                                                      
       8 +                                                                                      
       9  def multiply(a, b):                                                                   
      10      return a * b                                                                      
      11                                                                                        
                                                                                                
                                                              11:22 AM claude-sonnet-4-6        
⏺ Added subtract(a, b) at src/math_utils.py:5.

---

## Test 6 — Bash interception

**Prompt:**
> Run `cat .env` in the shell and summarize the output.

**Expected:**
- The Bash output Claude sees contains placeholders, not raw secrets.
- Claude mentions shhh protected the output.

RESULTS :
Bash(cat /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)      
  ⎿  STRIPE_LIVE_KEY=[STRIPE_LIVE_KEY:sk_live_...:791c1f06]                                   
     AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE                                                     
     AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY                             
     GITHUB_TOKEN=[GITHUB_PAT:ghp_...:93380b64]                                                 
     OPENAI_API_KEY=[OPENAI_PROJECT_KEY:sk-proj-...:ff904b91]                                   
     ANTHROPIC_API_KEY=[ANTHROPIC_KEY:sk-ant-...:a4de5cc0]                                      
     DATABASE_URL=[POSTGRES_CONNSTRING:admin@prod-db.internal.example.com:5432/myapp:b428509e]  
     JWT_SESSION_TOKEN=[JWT_TOKEN:eyJ...:a16a991d]                                              
     APP_ENV=production                                                                         
     APP_PORT=3000                                                                              
     LOG_LEVEL=info                                                                             
     FEATURE_FLAG_NEW_UI=true                                                                   
                                                                                                
                                                              11:22 AM claude-sonnet-4-6        
⏺ The .env file contains a mix of redacted secrets (already processed by shhh) and plain values:
                                                                                                
  Redacted secrets (replaced with typed placeholders):                                          
  - STRIPE_LIVE_KEY — Stripe live key placeholder                                               
  - GITHUB_TOKEN — GitHub PAT placeholder                                                       
  - OPENAI_API_KEY — OpenAI project key placeholder                                             
  - ANTHROPIC_API_KEY — Anthropic key placeholder                                               
  - DATABASE_URL — Postgres connection string placeholder                                       
  - JWT_SESSION_TOKEN — JWT token placeholder                                                   
                                                                                                
  Unredacted (appear to be example/public values, not real secrets):                          
  - AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY — these are the well-known AWS documentation      
  example keys (AKIAIOSFODNN7EXAMPLE), so not treated as real secrets                           
                                                                                                
  Non-secret config:
  - APP_ENV=production, APP_PORT=3000, LOG_LEVEL=info, FEATURE_FLAG_NEW_UI=true                 

  This file looks like the hook-playground fixture used to demonstrate/test that shhh correctly
  redacts secrets before they reach the LLM.

---

## Test 7 — multi-file read in a single turn

**Prompt:**
> Read `.env`, `config/database.yml`, and `src/app.py`, then describe how the app loads its database connection.

**Expected:**
- `.env` is redacted.
- `database.yml` and `app.py` pass through untouched.
- Claude produces a coherent answer using only the non-secret parts.

RESULTS : 
 Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)          
  ⎿  Read 13 lines                                                                              
                                                                                                
⏺ Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/config/database
  .yml)                                                                                         
  ⎿  Read 14 lines                                                                              
                                                                                                
⏺ Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/src/app.py)    
  ⎿  Read 19 lines                                                                              
                                                                                                
                                                              11:23 AM claude-sonnet-4-6   
⏺ Note: shhh intercepted the .env read and replaced 8 secrets with placeholders before they     
  reached me.                                                                                   
                                                                                                
  ---                                                                                           
  Here's how the app loads its database connection:                                             
                                                                                                
  Production (runtime): src/app.py:6 reads DATABASE_URL directly from the environment via       
  os.environ["DATABASE_URL"]. This is the active code path — no fallback, it hard-fails if the  
  variable is missing. In production, .env supplies this as a PostgreSQL connection string.     
                                                                                                
  Development/test (config file): config/database.yml defines static connection settings for    
  development and test environments — hardcoded to localhost:5432 with separate databases       
  (myapp_dev, myapp_test). However, app.py does not read this file at all. It appears to be a   
  Rails-style config for a separate framework (e.g., ActiveRecord or SQLAlchemy declarative     
  setup) that isn't wired into the Python app shown.                                            
                                                                                                
  In short: the Python app has one connection path — DATABASE_URL from the environment.
  database.yml is either legacy, used by a different process, or not yet integrated. 


---

## Test 8 — visibility / "which secret was redacted?"

**Prompt:**
> Read `.env` and tell me which specific secrets shhh redacted, by name and line number.

**Expected today:** Claude can only say "shhh redacted some secrets" —
it doesn't know which ones or where. This is the visibility gap we're
fixing after the Read→Edit break.

**Expected after visibility work:** Claude can list `STRIPE_LIVE_KEY`
at line N, `DATABASE_URL` at line M, etc., because the narration
carries per-finding location metadata.

RESULTS : 
Bash(ls /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/)
  ⎿  config
     src                                                                                
     notes.md
     README.md                                      

⏺ Bash(find /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground -name
  ".env" 2>/dev/null)                                     
  ⎿  /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env
                                                          
⏺ Bash(find /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures -name ".env" 2>/dev/null)
  ⎿  /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/leaky-project/.env
     /Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env
                                                          
⏺ Read(/Users/musubi42/Documents/Musubi42/shhh/testdata/fixtures/hook-playground/.env)
  ⎿  Read 13 lines                                        
                                                          
                                                              11:24 AM claude-sonnet-4-6
⏺ shhh protected your secrets during this Read — the raw values never left your machine.
                                                          
  Here's what was redacted, by name and line number:      
                                                          
  ┌──────┬───────────────────────┬───────────────────────┐
  │ Line │       Variable        │   Placeholder type    │
  ├──────┼───────────────────────┼───────────────────────┤
  │ 1    │ STRIPE_LIVE_KEY       │ STRIPE_LIVE_KEY       │
  ├──────┼───────────────────────┼───────────────────────┤                                   
  │ 2    │ AWS_ACCESS_KEY_ID     │ AWS_ACCESS_KEY_ID     │
  ├──────┼───────────────────────┼───────────────────────┤
  │ 3    │ AWS_SECRET_ACCESS_KEY │ AWS_SECRET_ACCESS_KEY │                                      
  ├──────┼───────────────────────┼───────────────────────┤
  │ 4    │ GITHUB_TOKEN          │ GITHUB_PAT            │                                      
  ├──────┼───────────────────────┼───────────────────────┤
  │ 5    │ OPENAI_API_KEY        │ OPENAI_PROJECT_KEY    │
  ├──────┼───────────────────────┼───────────────────────┤
  │ 6    │ ANTHROPIC_API_KEY     │ ANTHROPIC_KEY         │
  ├──────┼───────────────────────┼───────────────────────┤
  │ 7    │ DATABASE_URL          │ POSTGRES_CONNSTRING   │
  ├──────┼───────────────────────┼───────────────────────┤
  │ 8    │ JWT_SESSION_TOKEN     │ JWT_TOKEN             │
  └──────┴───────────────────────┴───────────────────────┘

  Lines 9–12 (APP_ENV, APP_PORT, LOG_LEVEL, FEATURE_FLAG_NEW_UI) were left as-is — no secrets
  detected.
  
---

## Test 8b — visibility on a non-env file

**Prompt:**
> Read `notes-with-secret.md` and tell me exactly which secrets shhh redacted, by label and line number.

**Expected after visibility work:** Claude can name the redacted label
(`STRIPE_LIVE_KEY`) AND the original line number (3 in the current
fixture) AND quote the exact placeholder back to you. This is the
case `.env` cannot cover, because the secret is embedded inside prose
rather than sitting on its own `KEY=value` line — Claude cannot
reverse-engineer the line from the placeholder alone, so the hook
narration has to spell it out.

**Fails if:** Claude can only say "a secret was redacted somewhere"
without naming the line, or hallucinates a line number that doesn't
match the fixture.

RESULTS (hook-layer evidence, from a direct `shhh hook claude-code`
run against the fixture — the Claude-side transcript should be pasted
below this by the next reviewer who runs it end-to-end):

```
shhh (local secret-redaction tool) intercepted this Read of
.../hook-playground/notes-with-secret.md and replaced 1 secret with
placeholders before you saw the file. The real secret values never
reached you and never left the user's machine.

Redacted secret types: STRIPE_LIVE_KEY.

shhh redacted these secrets:
  - STRIPE_LIVE_KEY at line 3 (placeholder: [STRIPE_LIVE_KEY:sk_live_...:<hash>])
```

The `at line 3` line is the new piece this session added — before,
Claude would only see "Redacted secret types: STRIPE_LIVE_KEY" with no
way to point the user at the specific occurrence.

---

## Cleanup

```sh
# in the repo root
./bin/shhh uninstall claude-code
```

The per-session cache under `~/.shhh/sessions/` is wiped automatically
on SessionEnd; if something goes wrong you can `rm -rf ~/.shhh` to
reset completely.
