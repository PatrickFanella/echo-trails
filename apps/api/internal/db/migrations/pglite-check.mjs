import { readFile, readdir } from 'node:fs/promises'
import { basename, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { PGlite } from '@electric-sql/pglite'
import { citext } from '@electric-sql/pglite/contrib/citext'
import { pg_trgm } from '@electric-sql/pglite/contrib/pg_trgm'
import { unaccent } from '@electric-sql/pglite/contrib/unaccent'
import { postgis } from '@electric-sql/pglite-postgis'

const currentDir = dirname(fileURLToPath(import.meta.url))
const apiRoot = join(currentDir, '../../..')
const migrationsDir = join(apiRoot, 'db/migrations')

const db = await PGlite.create({
  extensions: { citext, pg_trgm, postgis, unaccent },
})

const migrationFiles = await readdir(migrationsDir)
const upMigrations = migrationFiles.filter((name) => name.endsWith('.up.sql')).sort()
const downMigrations = migrationFiles.filter((name) => name.endsWith('.down.sql')).sort().reverse()

if (upMigrations.length === 0 || upMigrations.length !== downMigrations.length) {
  throw new Error('migration check requires matching up and down migrations')
}

for (const migration of upMigrations) {
  const sql = await readFile(join(migrationsDir, migration), 'utf8')
  console.log(`executing ${basename(migration)}`)
  await db.exec(sql)
}

const coreTables = [
  'users',
  'user_profiles',
  'sessions',
  'devices',
  'auth_identities',
  'consents',
  'places',
  'echoes',
  'replies',
  'media_assets',
  'moderation_actions',
  'analytics_events',
  'audit_logs',
  'jobs',
  'idempotency_keys',
  'media_uploads',
  'media_transcripts',
  'media_moderation_reviews',
  'creation_authorizations',
  'place_imports',
  'reply_revisions',
  'reply_mentions',
  'user_acknowledgements',
  'notification_preferences',
  'notification_mutes',
  'echo_engagement_events',
  'user_trust_scores',
  'echo_rank_snapshots',
  'place_alert_deliveries',
  'privacy_jobs',
  'email_auth_requests',
  'hidden_targets',
  'notification_deliveries',
  'user_privacy_settings',
  'admin_sessions',
  'session_refresh_replays',
]

for (const tableName of coreTables) {
  const result = await db.query('SELECT to_regclass($1) AS regclass', [`public.${tableName}`])
  if (result.rows[0].regclass === null) {
    throw new Error(`up migrations did not create expected table: ${tableName}`)
  }
}

const demoPlaces = await db.query(
  "SELECT count(*)::integer AS count FROM places WHERE id >= '019f486a-0001-7000-8000-000000000001' AND id <= '019f486a-0009-7000-8000-000000000009'",
)
if (demoPlaces.rows[0].count !== 9) {
  throw new Error(`expected 9 curated Downers Grove places, found ${demoPlaces.rows[0].count}`)
}

const plaintextCoordinateColumn = await db.query(
  "SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = 'echoes' AND column_name = 'private_creation_location'",
)
if (plaintextCoordinateColumn.rows.length !== 0) {
  throw new Error('release schema must not retain plaintext exact coordinates on echoes')
}

for (const migration of downMigrations) {
  const sql = await readFile(join(migrationsDir, migration), 'utf8')
  console.log(`executing ${basename(migration)}`)
  await db.exec(sql)
}

for (const tableName of coreTables) {
  const result = await db.query('SELECT to_regclass($1) AS regclass', [`public.${tableName}`])
  if (result.rows[0].regclass !== null) {
    throw new Error(`down migrations left table behind: ${tableName}`)
  }
}

const extensions = await db.query("SELECT extname FROM pg_extension WHERE extname IN ('postgis', 'citext', 'pg_trgm', 'unaccent')")
const extensionNames = new Set(extensions.rows.map((row) => row.extname))
for (const extensionName of ['postgis', 'citext', 'pg_trgm', 'unaccent']) {
  if (!extensionNames.has(extensionName)) {
    throw new Error(`down migration should retain ${extensionName} extension`)
  }
}

await db.close()
