// Cloudflare Worker reading from a sqlitedeploy primary over Hrana HTTP.
//
// Why this file is here: it's the proof point for sqlitedeploy v2's "edge
// support" pitch. With v1 (Litestream), Workers couldn't talk to your DB at
// all because there was no daemon they could embed. With v2 (sqld), Workers
// connect to the primary's Hrana endpoint over HTTP using @libsql/client and
// authenticate with the replica JWT minted by `sqlitedeploy up`.
//
// Deploy:
//   wrangler secret put SQLITEDEPLOY_REPLICA_JWT < .sqlitedeploy/auth/replica.jwt
//   wrangler deploy
//
// See README.md in this directory for the full setup.

import { createClient, type Client } from "@libsql/client";

interface Env {
	/** Hrana HTTPS URL of the sqlitedeploy primary, e.g. https://random.trycloudflare.com. */
	PRIMARY_URL: string;
	/** Replica JWT minted by `sqlitedeploy up`. Stored as a Wrangler secret. */
	SQLITEDEPLOY_REPLICA_JWT: string;
}

let cached: Client | null = null;

function client(env: Env): Client {
	// Workers re-use the same isolate across requests for hot paths; a single
	// libsql Client instance is safe to reuse and saves the per-request
	// connection setup.
	if (cached) return cached;
	cached = createClient({
		url: env.PRIMARY_URL,
		authToken: env.SQLITEDEPLOY_REPLICA_JWT,
	});
	return cached;
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		const url = new URL(request.url);

		if (url.pathname === "/health") {
			return new Response("ok\n", { status: 200 });
		}

		// Read-only example: count rows in a "users" table if it exists, else
		// return the SQLite version. Adjust the query for your schema.
		try {
			const db = client(env);
			const result = await db.execute(
				"SELECT name FROM sqlite_master WHERE type='table'",
			);
			const tables = result.rows.map((r) => r.name);
			const version = await db.execute("SELECT sqlite_version() AS v");
			return Response.json({
				ok: true,
				sqlite_version: version.rows[0]?.v,
				tables,
				query_pathname: url.pathname,
			});
		} catch (err) {
			// Common causes:
			//   - PRIMARY_URL unreachable (firewall / not running / wrong host)
			//   - JWT signed by a different keypair than the primary expects
			//   - JWT expired (up mints 10y tokens by default; rare)
			return Response.json(
				{
					ok: false,
					error: err instanceof Error ? err.message : String(err),
					hint: "Check PRIMARY_URL is reachable and SQLITEDEPLOY_REPLICA_JWT was copied from the right primary.",
				},
				{ status: 500 },
			);
		}
	},
} satisfies ExportedHandler<Env>;
