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
    context "永続化依存を注入した場合" do
      it "repositoryに店舗取得・ロック・バーガー解決・レビュー作成を委譲すること" do
        fake_shop = instance_double(Shop)
        fake_burger = instance_double(Burger)
        fake_review = instance_double(Review)
        repository = instance_double(Reviews::CreatingReviewRepository)

        allow(repository).to receive(:transaction).and_yield
        allow(repository).to receive(:find_shop!).with(shop.id).and_return(fake_shop)
        allow(repository).to receive(:with_shop_lock).with(fake_shop).and_yield
        allow(repository).to receive(:find_burger_for_shop).with(shop: fake_shop, burger_name: "Classic Burger").and_return(nil)
        allow(repository).to receive(:create_burger_for_shop).with(shop: fake_shop, burger_name: "Classic Burger").and_return(fake_burger)
        allow(repository).to receive(:create_review!).with(user: user, burger: fake_burger, rating: 4, comment: "Juicy and crispy").and_return(fake_review)

        result = described_class.new(user: user, params: params, repository: repository).invoke

        expect(result).to eq(fake_review)
      end
    end

    context "バーガーが存在しない場合" do
      it "レビューを作成すること" do
        expect { service.invoke }.to change(Review, :count).by(1)
      end

      it "バーガーを店舗内に作成すること" do
        expect { service.invoke }.to change(Burger, :count).by(1)
        expect(Burger.last.shop).to eq(shop)
      end

      it "作成したレビューを返すこと" do
        review = service.invoke
        expect(review.rating).to eq(4)
        expect(review.comment).to eq("Juicy and crispy")
      end
    end

    context "同名バーガーが既に存在する場合" do
      before do
        Burger.create!(shop: shop, name: "Classic Burger")
      end

      it "既存バーガーを再利用してレビューを作成すること" do
        expect { service.invoke }.to change(Review, :count).by(1)
        expect(Burger.count).to eq(1)
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

      it "バーガー作成をロールバックすること" do
        expect { service.invoke rescue nil }.not_to change(Burger, :count)
      end
    end
  end
end
