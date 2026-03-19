class AddDiscardedAtToReviews < ActiveRecord::Migration[8.0]
  def change
    add_column :reviews, :discarded_at, :datetime
    add_index :reviews, :discarded_at
  end
end
