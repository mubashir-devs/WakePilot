CREATE TABLE devices (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT    NOT NULL,
    mac         TEXT    NOT NULL UNIQUE,
    ip          TEXT    NOT NULL DEFAULT '',
    broadcast   TEXT    NOT NULL DEFAULT '',
    port        INTEGER NOT NULL DEFAULT 9 CHECK (port BETWEEN 1 AND 65535),
    interface   TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    created_at  TEXT    NOT NULL,
    updated_at  TEXT    NOT NULL
);

CREATE TABLE wake_history (
    id                      INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id               INTEGER REFERENCES devices(id) ON DELETE SET NULL,
    device_name             TEXT    NOT NULL,
    mac                     TEXT    NOT NULL,
    broadcast               TEXT    NOT NULL DEFAULT '',
    port                    INTEGER NOT NULL DEFAULT 0,
    interface               TEXT    NOT NULL DEFAULT '',
    status                  TEXT    NOT NULL CHECK (status IN ('sent', 'failed')),
    error                   TEXT    NOT NULL DEFAULT '',
    reachability            TEXT    NOT NULL DEFAULT 'not_checked'
                            CHECK (reachability IN ('pending', 'reachable', 'unknown', 'not_checked')),
    reachability_checked_at TEXT,
    requested_at            TEXT    NOT NULL
);

CREATE INDEX idx_wake_history_requested ON wake_history (requested_at DESC, id DESC);
CREATE INDEX idx_wake_history_device ON wake_history (device_id);
