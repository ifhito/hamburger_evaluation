class RequireShopNames < ActiveRecord::Migration[8.0]
  class MigrationShop < ActiveRecord::Base
    self.table_name = "shops"
  end

  def up
    MigrationShop.where(name: nil).update_all(name: "Unnamed shop")
    change_column_null :shops, :name, false
  end

  def down
    change_column_null :shops, :name, true
  end
end
