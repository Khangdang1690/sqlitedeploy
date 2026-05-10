"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.db = db;
exports.initSchema = initSchema;
const client_1 = require("@libsql/client");
let _client = null;
function db() {
    if (_client)
        return _client;
    const url = process.env.LIBSQL_URL;
    if (!url)
        throw new Error("LIBSQL_URL is not set");
    _client = (0, client_1.createClient)({ url, authToken: process.env.LIBSQL_AUTH_TOKEN });
    return _client;
}
async function initSchema() {
    await db().execute(`
    CREATE TABLE IF NOT EXISTS posts (
      id         INTEGER PRIMARY KEY AUTOINCREMENT,
      title      TEXT NOT NULL,
      body       TEXT NOT NULL,
      created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
    )
  `);
}
