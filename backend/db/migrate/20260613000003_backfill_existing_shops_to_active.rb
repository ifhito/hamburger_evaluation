class BackfillExistingShopsToActive < ActiveRecord::Migration[8.0]
  # 既存(seed)店舗は公開済み扱いにする。
  # status の enum 整数: pending=0, active=1, rejected=2 -> active は 1。
  def up
    execute("UPDATE shops SET status = 1")
  end

  def down
    execute("UPDATE shops SET status = 0")
  end
end
