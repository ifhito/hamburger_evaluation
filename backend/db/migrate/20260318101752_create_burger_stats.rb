class CreateBurgerStats < ActiveRecord::Migration[8.0]
  def change
    create_table :burger_stats do |t|
      t.references :burger, null: false, foreign_key: true, index: { unique: true }
      t.float   :average_rating, null: false, default: 0.0
      t.integer :review_count,   null: false, default: 0
      t.timestamps
    end
  end
end
