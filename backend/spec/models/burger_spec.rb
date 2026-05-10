require "rails_helper"

RSpec.describe Burger, type: :model do
  describe "associations" do
    it { is_expected.to belong_to(:shop) }
    it { is_expected.to have_many(:reviews).dependent(:destroy) }
    it { is_expected.to have_one(:burger_stat) }
  end

  describe "validations" do
    subject(:burger) { build(:burger) }

    it { is_expected.to validate_presence_of(:name) }
    it { is_expected.to validate_uniqueness_of(:name).scoped_to(:shop_id) }
  end
end
