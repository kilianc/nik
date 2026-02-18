-- ?1: query  ?2: threshold  ?3: limit
WITH exact AS (
  SELECT *, 1.0 AS score FROM contact
  WHERE id = ?1
     OR EXISTS (SELECT 1 FROM json_each(emails) WHERE value = ?1)
     OR EXISTS (SELECT 1 FROM json_each(phone_numbers) WHERE value = ?1)
     OR EXISTS (SELECT 1 FROM json_each(whatsapp_ids) WHERE value = ?1)
     OR EXISTS (SELECT 1 FROM json_each(telegram_ids) WHERE value = ?1)
     OR EXISTS (SELECT 1 FROM json_each(slack_ids) WHERE value = ?1)
),
scored AS (
  SELECT *,
    MAX(
      jaro_winkler_similarity(lower(name), lower(?1)),
      (SELECT coalesce(max(jaro_winkler_similarity(lower(j.value), lower(?1))), 0)
       FROM json_each(nicknames) j),
      CASE WHEN one_liner IS NOT NULL
        THEN jaro_winkler_similarity(lower(one_liner), lower(?1))
        ELSE 0 END,
      CASE WHEN notes IS NOT NULL
        THEN jaro_winkler_similarity(lower(notes), lower(?1))
        ELSE 0 END
    ) AS score
  FROM contact
  WHERE NOT EXISTS (SELECT 1 FROM exact)
),
fuzzy AS (
  SELECT * FROM scored WHERE score >= ?2
)
SELECT
  id,
  name,
  nicknames,
  emails,
  whatsapp_ids,
  telegram_ids,
  slack_ids,
  phone_numbers,
  timezone,
  location,
  one_liner,
  notes,
  last_message_at,
  last_seen_at,
  created_at,
  updated_at,
  score
FROM (
  SELECT * FROM exact
  UNION ALL
  SELECT * FROM fuzzy
)
ORDER BY score DESC
LIMIT ?3;
