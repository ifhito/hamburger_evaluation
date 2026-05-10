class StoreBurgersWithinShops < ActiveRecord::Migration[8.0]
  class MigrationBurger < ActiveRecord::Base
    self.table_name = "burgers"
  end

  class MigrationShopBurger < ActiveRecord::Base
    self.table_name = "shops_and_burgers"
  end

  def up
    add_reference :burgers, :shop, foreign_key: true

    MigrationBurger.reset_column_information
    MigrationShopBurger.reset_column_information

    MigrationBurger.find_each do |burger|
      link = MigrationShopBurger.where(burger_id: burger.id).order(:id).first
      raise "Burger ##{burger.id} has no shop" if link.nil?

      burger.update_columns(shop_id: link.shop_id)
    end

    change_column_null :burgers, :shop_id, false
    add_index :burgers, %i[shop_id name], unique: true
    drop_table :shops_and_burgers
  end

  def down
    create_table :shops_and_burgers do |t|
      t.references :shop, null: false, foreign_key: true
      t.references :burger, null: false, foreign_key: true
      t.timestamps
    end

    add_index :shops_and_burgers, %i[shop_id burger_id], unique: true

    MigrationBurger.reset_column_information
    MigrationBurger.find_each do |burger|
      MigrationShopBurger.create!(shop_id: burger.shop_id, burger_id: burger.id)
    end

    remove_index :burgers, column: %i[shop_id name]
    remove_reference :burgers, :shop, foreign_key: true
  end
end
