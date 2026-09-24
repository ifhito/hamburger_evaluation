-- 過去のCREATE TABLEを適用済みのDBにも閉業日時を追加する。
-- 列を含むCREATE TABLEから作ったDBでは、既存の閉業日時をそのまま保持する。
ALTER TABLE shops ADD COLUMN IF NOT EXISTS closed_at timestamptz;
