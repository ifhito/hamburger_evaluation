class CreateShopStats < ActiveRecord::Migration[8.0]
  def change
    create_table :shop_stats do |t|
      t.references :shop, null: false, foreign_key: true, index: { unique: true }
      t.float :average_rating, null: false, default: 0.0
      t.integer :burger_count, null: false, default: 0
      t.integer :review_count, null: false, default: 0
      t.float :weighted_score, null: false, default: 0.0
      t.float :confidence, null: false, default: 0.0
      t.timestamps
    end
  end
end
