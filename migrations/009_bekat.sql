-- «Registon» xaritasi «Bekat» bilan almashtirildi (tarixiy va muqaddas joyda otishma bo'lmasin).
-- Statistika, turnirlar, yozuvlar va o'yinlar tarixidagi nomi ham yangilanadi.

-- ikkalasi bir o'yinchida bo'lsa (bo'lmasligi kerak) — qo'shib qo'yamiz
INSERT INTO player_stats (user_id, key, value)
SELECT user_id, replace(key, ':registon', ':bekat'), value FROM player_stats WHERE key IN ('mp:registon', 'mw:registon')
ON CONFLICT (user_id, key) DO UPDATE SET value = player_stats.value + EXCLUDED.value;
DELETE FROM player_stats WHERE key IN ('mp:registon', 'mw:registon');

UPDATE tournaments SET map = 'bekat' WHERE map = 'registon';
UPDATE demos SET map = 'bekat' WHERE map = 'registon';
UPDATE matches SET room = 'bekat' || substr(room, 9) WHERE room LIKE 'registon-%';
