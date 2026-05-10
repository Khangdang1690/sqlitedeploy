import { createClient, type Client } from "@libsql/client";

let _client: Client | null = null;

export function db(): Client {
  if (_client) return _client;
  const url = process.env.LIBSQL_URL;
  if (!url) throw new Error("LIBSQL_URL is not set");
  _client = createClient({ url, authToken: process.env.LIBSQL_AUTH_TOKEN });
  return _client;
}

export async function initSchema(): Promise<void> {
  await db().execute(`
    CREATE TABLE IF NOT EXISTS posts (
      id         INTEGER PRIMARY KEY AUTOINCREMENT,
      title      TEXT NOT NULL,
      body       TEXT NOT NULL,
      created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
    )
  `);
}
