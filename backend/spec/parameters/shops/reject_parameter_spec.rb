require "rails_helper"

RSpec.describe Shops::RejectParameter do
  it "allows a nil moderation note" do
    expect(described_class.new(moderation_note: nil).moderation_note).to be_nil
  end

  it "exposes the moderation note when provided" do
    expect(described_class.new(moderation_note: "x").moderation_note).to eq("x")
  end
end
