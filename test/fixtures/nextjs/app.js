// Minimal Next.js / TypeScript usage.
//
// Workflow:
//   npm i               # installs sqlitedeploy + better-sqlite3
//   npx sqlitedeploy auth login
//   npx sqlitedeploy init    # creates ./data/app.db
//   npx sqlitedeploy run &   # background replication daemon
//   node app.js              # your application reads/writes the file
//
// In a real Next.js app you'd open the connection inside a route handler
// (or a server-side singleton) — the path is just `./data/app.db`.

const Database = require('better-sqlite3');

const db = new Database('./data/app.db');
db.pragma('journal_mode = WAL'); // sqlitedeploy already set this; harmless to confirm

db.exec('CREATE TABLE IF NOT EXISTS visits (ts TEXT NOT NULL)');
db.prepare('INSERT INTO visits (ts) VALUES (?)').run(new Date().toISOString());

const rows = db.prepare('SELECT COUNT(*) AS n FROM visits').get();
console.log(`recorded ${rows.n} visit(s)`);
