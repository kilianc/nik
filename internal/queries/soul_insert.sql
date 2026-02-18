INSERT INTO soul (id, version, content, dream_date)
VALUES (?1, (SELECT COALESCE(MAX(version), 0) + 1 FROM soul), ?2, ?3)
