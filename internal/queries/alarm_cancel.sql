UPDATE alarm SET cancelled_at = NOW_ISO8601_MS() WHERE id = ?1
