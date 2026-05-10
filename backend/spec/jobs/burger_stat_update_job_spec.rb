require "rails_helper"

RSpec.describe BurgerStatUpdateJob, type: :job do
  describe "#perform" do
    it "recalculates the burger stat and then the parent shop stat" do
      burger = create(:burger)
      repository = instance_double(BurgerStats::BurgerStatRepository)
      burger_service = instance_double(BurgerStats::RecalculateBurgerStatService)
      shop_service = instance_double(ShopStats::RecalculateShopStatService)

      expect(BurgerStats::BurgerStatRepository).to receive(:new).and_return(repository)
      expect(repository).to receive(:find_burger!).with(burger.id).and_return(burger)
      expect(BurgerStats::RecalculateBurgerStatService).to receive(:new).with(burger).and_return(burger_service)
      expect(burger_service).to receive(:invoke)
      expect(ShopStats::RecalculateShopStatService).to receive(:new).with(burger.shop).and_return(shop_service)
      expect(shop_service).to receive(:invoke)

      described_class.new.perform(burger.id)
    end
  end

  describe "queue" do
    it "is queued on the default queue" do
      expect(described_class.new.queue_name).to eq("default")
    end
  end
end
