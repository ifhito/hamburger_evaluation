require "rails_helper"

RSpec.describe BurgerStatUpdateJob, type: :job do
  describe "#perform" do
    it "calls BurgerStat.recalculate_for with the burger" do
      burger = create(:burger)
      expect(BurgerStat).to receive(:recalculate_for).with(burger)
      described_class.new.perform(burger.id)
    end
  end

  describe "queue" do
    it "is queued on the default queue" do
      expect(described_class.new.queue_name).to eq("default")
    end
  end
end
