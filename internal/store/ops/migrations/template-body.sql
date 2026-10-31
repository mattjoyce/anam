INSERT INTO template_body (hash, body) SELECT hash, body FROM inline_projection WHERE true ON CONFLICT (hash) DO NOTHING;
DROP TABLE inline_projection;
