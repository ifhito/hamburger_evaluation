require "rails_helper"

RSpec.describe ShopStat, type: :model do
  describe "associations" do
    it { is_expected.to belong_to(:shop) }
  end
end
