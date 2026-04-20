require "rails_helper"

RSpec.describe Reviews::CreateReviewService do
  let(:user)   { create(:user) }
  let(:shop)   { create(:shop) }
  let(:params) do
    Reviews::CreateParameter.new(
      rating:      4,
      comment:     "Juicy and crispy",
      shop_id:     shop.id,
      burger_name: "Classic Burger"
    )
  end

  subject(:service) { described_class.new(user: user, params: params) }

  describe "#invoke" do
    context "バーガーが存在しない場合" do
      it "レビューを作成すること" do
        expect { service.invoke }.to change(Review, :count).by(1)
      end

      it "バーガーと店舗の紐付けを作成すること" do
        expect { service.invoke }.to change(Burger, :count).by(1)
          .and change(ShopsAndBurger, :count).by(1)
      end

      it "作成したレビューを返すこと" do
        review = service.invoke
        expect(review.rating).to eq(4)
        expect(review.comment).to eq("Juicy and crispy")
      end
    end

    context "同名バーガーが既に存在する場合" do
      before do
        burger = Burger.create!(name: "Classic Burger")
        ShopsAndBurger.create!(shop: shop, burger: burger)
      end

      it "既存バーガーを再利用してレビューを作成すること" do
        expect { service.invoke }.to change(Review, :count).by(1)
        expect(Burger.count).to eq(1)
        expect(ShopsAndBurger.count).to eq(1)
      end
    end

    context "存在しないshop_idの場合" do
      let(:params) do
        Reviews::CreateParameter.new(
          rating: 4, comment: "test", shop_id: 0, burger_name: "Test"
        )
      end

      it "ActiveRecord::RecordNotFoundを発生させること" do
        expect { service.invoke }.to raise_error(ActiveRecord::RecordNotFound)
      end
    end

    context "ratingが無効な場合（バーガー作成後にロールバック）" do
      let(:params) do
        Reviews::CreateParameter.new(
          rating: 6, comment: "test", shop_id: shop.id, burger_name: "Rollback Burger"
        )
      end

      it "バーガーと紐付けをロールバックすること" do
        expect { service.invoke rescue nil }.not_to change(Burger, :count)
        expect(ShopsAndBurger.where(shop: shop).count).to eq(0)
      end
    end
  end
end
