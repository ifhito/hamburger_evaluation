require "rails_helper"

RSpec.describe BurgerStat, type: :model do
  describe "associations" do
    it "belongs to a burger" do
      burger = create(:burger)
      stat = described_class.create!(burger: burger)

      expect(stat.burger).to eq(burger)
    end
  end
end
