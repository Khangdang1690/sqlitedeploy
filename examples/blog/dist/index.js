"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const node_server_1 = require("@hono/node-server");
const hono_1 = require("hono");
const db_1 = require("./db");
const app = new hono_1.Hono();
// ── HTML helpers ─────────────────────────────────────────────────────────────
function esc(s) {
    return s.replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c] ?? c));
}
function layout(title, body) {
    return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>${esc(title)} — sqlitedeploy blog</title>
  <style>
    *, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: system-ui, sans-serif; max-width: 720px; margin: 2rem auto; padding: 0 1rem; line-height: 1.6; color: #111; }
    h1 { font-size: 2rem; margin-bottom: 1rem; }
    h2 { font-size: 1.25rem; margin-bottom: 0.25rem; }
    a { color: #0070f3; text-decoration: none; }
    a:hover { text-decoration: underline; }
    nav { margin-bottom: 2rem; font-size: 0.9rem; color: #555; }
    nav a { color: #0070f3; }
    .post-list { list-style: none; }
    .post-list li { padding: 1rem 0; border-bottom: 1px solid #eee; }
    .post-meta { font-size: 0.8rem; color: #888; margin-top: 0.2rem; }
    .post-body { margin: 1.5rem 0; white-space: pre-wrap; line-height: 1.7; }
    form { display: flex; flex-direction: column; gap: 1rem; max-width: 600px; }
    label { display: flex; flex-direction: column; gap: 0.25rem; font-weight: 600; font-size: 0.9rem; }
    input, textarea { padding: 0.5rem 0.75rem; border: 1px solid #ccc; border-radius: 6px; font: inherit; }
    input:focus, textarea:focus { outline: 2px solid #0070f3; border-color: transparent; }
    textarea { min-height: 200px; resize: vertical; }
    button { align-self: flex-start; padding: 0.5rem 1.25rem; border: none; border-radius: 6px; font: inherit; cursor: pointer; }
    .btn-primary { background: #0070f3; color: #fff; }
    .btn-primary:hover { background: #005cc5; }
    .btn-danger { background: #d00; color: #fff; font-size: 0.85rem; }
    .btn-danger:hover { background: #a00; }
    .empty { color: #888; margin-top: 1rem; }
  </style>
</head>
<body>
  <nav><a href="/">All posts</a> · <a href="/posts/new">New post</a></nav>
  ${body}
</body>
</html>`;
}
// ── Routes ───────────────────────────────────────────────────────────────────
app.get("/health", (c) => c.json({ status: "ok" }));
app.get("/", async (c) => {
    const result = await (0, db_1.db)().execute("SELECT id, title, created_at FROM posts ORDER BY created_at DESC");
    const rows = result.rows;
    const list = rows.length
        ? `<ul class="post-list">${rows
            .map((p) => `<li>
               <h2><a href="/posts/${p.id}">${esc(p.title)}</a></h2>
               <p class="post-meta">${esc(p.created_at)}</p>
             </li>`)
            .join("")}</ul>`
        : `<p class="empty">No posts yet. <a href="/posts/new">Write the first one.</a></p>`;
    return c.html(layout("All posts", `<h1>Blog</h1>${list}`));
});
app.get("/posts/new", (c) => c.html(layout("New post", `<h1>New Post</h1>
       <form method="POST" action="/posts">
         <label>Title<input name="title" required maxlength="255" autofocus /></label>
         <label>Body<textarea name="body" required></textarea></label>
         <button class="btn-primary" type="submit">Publish</button>
       </form>`)));
app.post("/posts", async (c) => {
    const body = await c.req.parseBody();
    const title = String(body.title ?? "").trim();
    const text = String(body.body ?? "").trim();
    if (!title || !text)
        return c.redirect("/posts/new", 303);
    await (0, db_1.db)().execute({
        sql: "INSERT INTO posts (title, body) VALUES (?, ?)",
        args: [title, text],
    });
    return c.redirect("/", 303);
});
app.get("/posts/:id", async (c) => {
    const id = Number(c.req.param("id"));
    if (!Number.isInteger(id) || id < 1)
        return c.notFound();
    const result = await (0, db_1.db)().execute({
        sql: "SELECT id, title, body, created_at FROM posts WHERE id = ?",
        args: [id],
    });
    const row = result.rows[0];
    if (!row)
        return c.notFound();
    return c.html(layout(row.title, `<h1>${esc(row.title)}</h1>
       <p class="post-meta">Published ${esc(row.created_at)}</p>
       <div class="post-body">${esc(row.body)}</div>
       <form method="POST" action="/posts/${row.id}/delete" onsubmit="return confirm('Delete this post?')">
         <button class="btn-danger" type="submit">Delete post</button>
       </form>`));
});
app.post("/posts/:id/delete", async (c) => {
    const id = Number(c.req.param("id"));
    if (Number.isInteger(id) && id >= 1) {
        await (0, db_1.db)().execute({ sql: "DELETE FROM posts WHERE id = ?", args: [id] });
    }
    return c.redirect("/", 303);
});
// ── Bootstrap ────────────────────────────────────────────────────────────────
const PORT = Number(process.env.PORT ?? 3000);
(0, db_1.initSchema)()
    .then(() => {
    console.log("[blog] schema ready");
    (0, node_server_1.serve)({ fetch: app.fetch, port: PORT }, (info) => {
        console.log(`[blog] listening on http://0.0.0.0:${info.port}`);
    });
})
    .catch((err) => {
    console.error("[blog] fatal: schema init failed:", err);
    process.exit(1);
});
