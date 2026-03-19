class CreateShopsAndBurgers < ActiveRecord::Migration[8.0]
  def change
    create_table :shops_and_burgers do |t|
      t.references :shop, null: false, foreign_key: true
      t.references :burger, null: false, foreign_key: true

      t.timestamps
    end
  end
end
