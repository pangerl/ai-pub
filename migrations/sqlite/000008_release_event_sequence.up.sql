ALTER TABLE release_events ADD COLUMN event_seq INTEGER;

UPDATE release_events SET event_seq = rowid WHERE event_seq IS NULL;

CREATE UNIQUE INDEX idx_release_events_event_seq ON release_events(event_seq);

CREATE TRIGGER release_events_assign_event_seq
AFTER INSERT ON release_events
FOR EACH ROW WHEN NEW.event_seq IS NULL
BEGIN
  UPDATE release_events SET event_seq = NEW.rowid WHERE rowid = NEW.rowid;
END;
