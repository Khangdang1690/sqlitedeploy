# Minimal FastAPI / Python usage.
#
# Workflow:
#   pip install sqlitedeploy fastapi uvicorn
#   sqlitedeploy auth login
#   sqlitedeploy init                # creates ./data/app.db
#   sqlitedeploy run &               # background replication daemon
#   uvicorn main:app --port 8000     # your FastAPI app
#
# In production you'd run sqlitedeploy under a process manager (systemd,
# supervisor, foreman) so it restarts alongside the app.

import sqlite3
from contextlib import contextmanager
from datetime import datetime, timezone

from fastapi import FastAPI

DB_PATH = "data/app.db"
app = FastAPI()


@contextmanager
def conn():
    c = sqlite3.connect(DB_PATH)
    try:
        yield c
        c.commit()
    finally:
        c.close()


@app.on_event("startup")
def _ensure_schema():
    with conn() as c:
        c.execute("CREATE TABLE IF NOT EXISTS visits (ts TEXT NOT NULL)")


@app.post("/visit")
def record_visit():
    with conn() as c:
        c.execute("INSERT INTO visits (ts) VALUES (?)", (datetime.now(timezone.utc).isoformat(),))
    return {"ok": True}


@app.get("/visits")
def count_visits():
    with conn() as c:
        (n,) = c.execute("SELECT COUNT(*) FROM visits").fetchone()
    return {"count": n}
