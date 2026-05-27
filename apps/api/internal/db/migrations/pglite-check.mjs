import { readFile } from 'node:fs/promises'
import { basename, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { PGlite } from '@electric-sql/pglite'
import { citext } from '@electric-sql/pglite/contrib/citext'
import { postgis } from '@electric-sql/pglite-postgis'

const currentDir = dirname(fileURLToPath(import.meta.url))
const apiRoot = join(currentDir, '../../..')
const migrationsDir = join(apiRoot, 'db/migrations')

const db = await PGlite.create({
  extensions: { citext, postgis },
})

for (const migration of ['0001_core_schema.up.sql', '0001_core_schema.down.sql']) {
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
]

for (const tableName of coreTables) {
  const result = await db.query('SELECT to_regclass($1) AS regclass', [`public.${tableName}`])
  if (result.rows[0].regclass !== null) {
    throw new Error(`down migration left core table behind: ${tableName}`)
  }
}

const extensions = await db.query("SELECT extname FROM pg_extension WHERE extname IN ('postgis', 'citext')")
const extensionNames = new Set(extensions.rows.map((row) => row.extname))
for (const extensionName of ['postgis', 'citext']) {
  if (!extensionNames.has(extensionName)) {
    throw new Error(`down migration should retain ${extensionName} extension`)
  }
}

await db.close()
