DELETE FROM snapshot_facet;
DELETE FROM snapshot_part;
DELETE FROM snapshot;
ALTER TABLE snapshot_part ADD COLUMN digest TEXT NOT NULL DEFAULT '';
