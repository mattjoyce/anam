ALTER TABLE facet RENAME COLUMN binding_person_id TO binding_subject_id;
INSERT INTO ledger_meta (key, value) SELECT 'subject_id', value FROM ledger_meta WHERE key = 'person_id';
