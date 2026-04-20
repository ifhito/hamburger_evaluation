class AddScoreToBurgerStats < ActiveRecord::Migration[8.0]
  def change
    add_column :burger_stats, :weighted_score, :float, default: 0.0
    add_column :burger_stats, :confidence, :float, default: 0.0
  end
end
