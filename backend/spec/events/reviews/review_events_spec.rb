require "rails_helper"

RSpec.describe Reviews::ReviewEvents do
  describe ".review_changed_for_burger" do
    it "enqueues a burger stat projection update" do
      expect {
        described_class.review_changed_for_burger(123)
      }.to have_enqueued_job(BurgerStatUpdateJob).with(123)
    end
  end
end
