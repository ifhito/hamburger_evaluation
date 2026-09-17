require "rails_helper"

RSpec.describe Shops::ShopStatus do
  describe ".initial" do
    it "新規店舗の初期状態は pending" do
      expect(described_class.initial.pending?).to be true
    end
  end

  describe "#initialize" do
    it "有効な値 pending/active/rejected を受け付ける" do
      %w[pending active rejected].each do |v|
        expect { described_class.new(v) }.not_to raise_error
      end
    end

    it "シンボルも文字列化して受け付ける" do
      expect(described_class.new(:active).active?).to be true
    end

    it "不正な値で ArgumentError" do
      expect { described_class.new("unknown") }.to raise_error(ArgumentError)
    end

    it "初期化後は frozen" do
      expect(described_class.new("active")).to be_frozen
    end
  end

  describe "述語" do
    it "pending?/active?/rejected? がそれぞれ対応する" do
      expect(described_class.new("pending").pending?).to be true
      expect(described_class.new("active").active?).to be true
      expect(described_class.new("rejected").rejected?).to be true
    end
  end

  describe "遷移" do
    it "#approve は active を返す" do
      expect(described_class.new("pending").approve).to eq(described_class.new("active"))
    end

    it "#reject は rejected を返す" do
      expect(described_class.new("pending").reject).to eq(described_class.new("rejected"))
    end
  end

  describe "#== / #to_s" do
    it "同じ値なら等しい" do
      expect(described_class.new("active")).to eq(described_class.new("active"))
    end

    it "異なる値とは等しくない" do
      expect(described_class.new("active")).not_to eq(described_class.new("pending"))
    end

    it "#to_s は値文字列を返す" do
      expect(described_class.new("active").to_s).to eq("active")
    end
  end

  describe "定数" do
    it "PUBLICLY_LISTED は active のみ" do
      expect(described_class::PUBLICLY_LISTED).to eq([ "active" ])
    end
  end
end
