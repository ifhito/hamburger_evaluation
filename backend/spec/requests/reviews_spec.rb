require "rails_helper"

RSpec.describe "Reviews", type: :request do
  let(:user)   { create(:user) }
  let(:shop)   { create(:shop) }
  let(:burger) { create(:burger).tap { |b| ShopsAndBurger.create!(shop: shop, burger: b) } }

  describe "GET /reviews" do
    it "returns all kept reviews" do
      create_list(:review, 3, burger: burger)
      discarded = create(:review, burger: burger)
      discarded.discard

      get "/reviews"

      expect(response).to have_http_status(:ok)
      ids = response.parsed_body.map { |r| r["id"] }
      expect(ids).not_to include(discarded.id)
      expect(ids.length).to eq(3)
    end

    context "with rating filter" do
      it "returns only reviews with the specified rating" do
        create(:review, burger: burger, rating: 3)
        r5 = create(:review, burger: burger, rating: 5)

        get "/reviews?rating=5"

        expect(response).to have_http_status(:ok)
        ids = response.parsed_body.map { |r| r["id"] }
        expect(ids).to contain_exactly(r5.id)
      end
    end

    context "with keyword filter" do
      it "returns only reviews whose comment matches the keyword" do
        matched = create(:review, burger: burger, comment: "外はcrispy中はジューシー")
        create(:review, burger: burger, comment: "普通のバーガー")

        get "/reviews?keyword=crispy"

        expect(response).to have_http_status(:ok)
        ids = response.parsed_body.map { |r| r["id"] }
        expect(ids).to contain_exactly(matched.id)
      end
    end

    context "JSON shape (Serializer)" do
      it "includes user.username in each review" do
        create(:review, user: user, burger: burger)

        get "/reviews"

        body = response.parsed_body
        expect(body.first["user"]).to include("username" => user.username)
      end

      it "does not include discarded_at in the response" do
        create(:review, user: user, burger: burger)

        get "/reviews"

        body = response.parsed_body
        expect(body.first.keys).not_to include("discarded_at")
      end

      it "does not include updated_at in the response" do
        create(:review, user: user, burger: burger)

        get "/reviews"

        body = response.parsed_body
        expect(body.first.keys).not_to include("updated_at")
      end

    end
  end

  describe "GET /reviews/:id" do
    context "with a valid id" do
      it "returns the review" do
        review = create(:review, user: user, burger: burger)
        get "/reviews/#{review.id}"
        expect(response).to have_http_status(:ok)
        expect(response.parsed_body["id"]).to eq(review.id)
      end

    end

    context "with a non-existent id" do
      it "returns 404" do
        get "/reviews/0"
        expect(response).to have_http_status(:not_found)
      end
    end
  end

  describe "POST /reviews" do
    let(:valid_params) { { review: { rating: 4, comment: "Great burger!", shop_id: shop.id, burger_name: "Big Mac" } } }

    context "when authenticated" do
      it "creates a review and returns 201" do
        post "/reviews", params: valid_params.to_json, headers: auth_headers(user)
        expect(response).to have_http_status(:created)
        expect(response.parsed_body["rating"]).to eq(4)
      end
    end

    context "when unauthenticated" do
      it "returns 401" do
        post "/reviews", params: valid_params.to_json,
                         headers: { "Content-Type" => "application/json" }
        expect(response).to have_http_status(:unauthorized)
      end
    end
  end

  describe "PUT /reviews/:id" do
    let(:review) { create(:review, user: user, burger: burger) }

    context "updating own review" do
      it "returns 200 with updated data" do
        put "/reviews/#{review.id}",
            params: { review: { comment: "Updated comment", rating: 5 } }.to_json,
            headers: auth_headers(user)
        expect(response).to have_http_status(:ok)
        expect(response.parsed_body["comment"]).to eq("Updated comment")
      end
    end

    context "updating another user's review" do
      it "returns 404" do
        other_user = create(:user)
        put "/reviews/#{review.id}",
            params: { review: { comment: "Hacked" } }.to_json,
            headers: auth_headers(other_user)
        expect(response).to have_http_status(:not_found)
      end
    end
  end

  describe "DELETE /reviews/:id" do
    context "when authenticated" do
      it "soft-deletes the review and returns 204" do
        review = create(:review, user: user, burger: burger)
        delete "/reviews/#{review.id}", headers: auth_headers(user)
        expect(response).to have_http_status(:no_content)
        expect(review.reload.discarded_at).not_to be_nil
      end
    end

    context "when unauthenticated" do
      it "returns 401" do
        review = create(:review, user: user, burger: burger)
        delete "/reviews/#{review.id}",
               headers: { "Content-Type" => "application/json" }
        expect(response).to have_http_status(:unauthorized)
      end
    end
  end
end
