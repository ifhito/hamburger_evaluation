-- 取り消すと閉業日時は失われるが、店舗行は残る。
ALTER TABLE shops DROP COLUMN IF EXISTS closed_at;
