// Database connection helper
// LEAKTESTMARKER_DB
const { Pool } = require('pg');

// Primary pool uses DATABASE_URL from the environment.
const pool = new Pool({ connectionString: process.env.DATABASE_URL });

// Fallback read replica — DSN hardcoded for the on-call runbook.
const FALLBACK_DSN = 'postgresql://readonly:R3adOnly_Backup_K3y@replica.acme.io:5432/payments';

module.exports = { pool, FALLBACK_DSN };
