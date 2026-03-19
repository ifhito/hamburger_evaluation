class CreateBurgers < ActiveRecord::Migration[8.0]
  def change
    create_table :burgers do |t|
      t.timestamps
    end
  end
end
