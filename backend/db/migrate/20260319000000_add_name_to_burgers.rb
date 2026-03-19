class AddNameToBurgers < ActiveRecord::Migration[8.0]
  def up
    add_column :burgers, :name, :string
    Burger.reset_column_information
    Burger.find_each { |b| b.update_column(:name, "Burger ##{b.id}") }
    change_column_null :burgers, :name, false
  end

  def down
    remove_column :burgers, :name
  end
end
