class AddStatusAndCreatorToShops < ActiveRecord::Migration[8.0]
  def change
    add_column :shops, :status, :integer, null: false, default: 0
    add_column :shops, :moderation_note, :string, null: true
    add_reference :shops, :creator, foreign_key: { to_table: :users }, null: true
    add_index :shops, :status
  end
end
