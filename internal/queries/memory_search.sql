SELECT
  m.id,
  m.content,
  m.metadata,
  m.source,
  m.source_id,
  m.created_at,
  m.deleted_at,
  v.distance
FROM vec_memory v
JOIN memory m ON m.id = v.id
WHERE v.embedding MATCH ?1 AND k = ?2
ORDER BY v.distance ASC
