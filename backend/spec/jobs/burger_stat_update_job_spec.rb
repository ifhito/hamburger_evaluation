require "rails_helper"

RSpec.describe BurgerStatUpdateJob, type: :job do
  describe "#perform" do
    it "recalculates the burger stat through the application service" do
      burger = create(:burger)
      service = instance_double(BurgerStats::RecalculateBurgerStatService)

      expect(BurgerStats::RecalculateBurgerStatService).to receive(:new).with(burger).and_return(service)
      expect(service).to receive(:invoke)

      described_class.new.perform(burger.id)
    end
  end

  describe "queue" do
    it "is queued on the default queue" do
      expect(described_class.new.queue_name).to eq("default")
    end
  end
end
