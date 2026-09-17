require "rails_helper"

RSpec.describe "Shops", type: :request do
  let(:shop)   { create(:shop) }
  let(:burger) { create(:burger).tap { |b| ShopsAndBurger.create!(shop: shop, burger: b) } }

  describe "GET /shops" do
    it "returns all shops with id and name" do
      shop1 = create(:shop, name: "Alpha Burger")
      shop2 = create(:shop, name: "Zeta Grill")

      get "/shops"

      expect(response).to have_http_status(:ok)
      names = response.parsed_body.map { |s| s["name"] }
      expect(names).to include("Alpha Burger", "Zeta Grill")
    end

    context "with keyword filter" do
      it "returns only shops whose name matches the keyword" do
        create(:shop, name: "McDonald's")
        create(:shop, name: "Burger King")

        get "/shops?keyword=burger"

        expect(response).to have_http_status(:ok)
        names = response.parsed_body.map { |s| s["name"] }
        expect(names).to include("Burger King")
        expect(names).not_to include("McDonald's")
      end

      it "is case-insensitive" do
        create(:shop, name: "Shake Shack")
        create(:shop, name: "Five Guys")

        get "/shops?keyword=SHAKE"

        expect(response).to have_http_status(:ok)
        names = response.parsed_body.map { |s| s["name"] }
        expect(names).to include("Shake Shack")
        expect(names).not_to include("Five Guys")
      end
    end
  end

  describe "GET /shops/:id" do
    context "with a valid id" do
      it "returns the shop with id and name" do
        create(:review, burger: burger)

        get "/shops/#{shop.id}"

        expect(response).to have_http_status(:ok)
        body = response.parsed_body
        expect(body["id"]).to eq(shop.id)
        expect(body["name"]).to eq(shop.name)
      end

      it "returns reviews for that shop" do
        review = create(:review, burger: burger)

        get "/shops/#{shop.id}"

        body = response.parsed_body
        ids = body["reviews"].map { |r| r["id"] }
        expect(ids).to include(review.id)
      end

      it "does not include reviews from other shops" do
        other_shop   = create(:shop)
        other_burger = create(:burger).tap { |b| ShopsAndBurger.create!(shop: other_shop, burger: b) }
        create(:review, burger: burger)
        other_review = create(:review, burger: other_burger)

        get "/shops/#{shop.id}"

        body = response.parsed_body
        ids = body["reviews"].map { |r| r["id"] }
        expect(ids).not_to include(other_review.id)
      end

      it "includes burger name in each review" do
        create(:review, burger: burger)

        get "/shops/#{shop.id}"

        body = response.parsed_body
        expect(body["reviews"].first["burger"]).to have_key("name")
      end
    end

    context "with a non-existent id" do
      it "returns 404" do
        get "/shops/0"
        expect(response).to have_http_status(:not_found)
      end
    end

    context "with a pending shop" do
      let(:owner) { create(:user) }
      let(:pending_shop) { create(:shop, :pending, creator: owner) }

      it "returns 404 for anonymous viewers" do
        get "/shops/#{pending_shop.id}"
        expect(response).to have_http_status(:not_found)
      end

      it "returns 404 for a different user" do
        get "/shops/#{pending_shop.id}", headers: auth_headers(create(:user))
        expect(response).to have_http_status(:not_found)
      end

      it "returns the shop for its creator" do
        get "/shops/#{pending_shop.id}", headers: auth_headers(owner)
        expect(response).to have_http_status(:ok)
        expect(response.parsed_body["status"]).to eq("pending")
      end

      it "returns the shop for an admin" do
        get "/shops/#{pending_shop.id}", headers: auth_headers(create(:user, :admin))
        expect(response).to have_http_status(:ok)
      end
    end
  end

  describe "GET /shops visibility" do
    it "excludes pending shops for anonymous viewers" do
      create(:shop, name: "Active Shop")
      create(:shop, :pending, name: "Pending Shop")

      get "/shops"

      names = response.parsed_body.map { |s| s["name"] }
      expect(names).to include("Active Shop")
      expect(names).not_to include("Pending Shop")
    end

    it "includes the requester's own pending shop" do
      owner = create(:user)
      create(:shop, :pending, name: "Mine", creator: owner)

      get "/shops", headers: auth_headers(owner)

      names = response.parsed_body.map { |s| s["name"] }
      expect(names).to include("Mine")
    end

    it "shows all shops to admins" do
      create(:shop, :pending, name: "PendingForAdmin")

      get "/shops", headers: auth_headers(create(:user, :admin))

      names = response.parsed_body.map { |s| s["name"] }
      expect(names).to include("PendingForAdmin")
    end
  end

  describe "POST /shops" do
    let(:user) { create(:user) }

    it "requires authentication" do
      post "/shops", params: { shop: { name: "New" } }.to_json,
                     headers: { "Content-Type" => "application/json" }
      expect(response).to have_http_status(:unauthorized)
    end

    it "creates a pending shop owned by the creator" do
      post "/shops", params: { shop: { name: "My New Shop" } }.to_json,
                     headers: auth_headers(user)

      expect(response).to have_http_status(:created)
      body = response.parsed_body
      expect(body["name"]).to eq("My New Shop")
      expect(body["status"]).to eq("pending")
      created = Shop.find(body["id"])
      expect(created.creator).to eq(user)
    end

    it "returns 422 for a blank name" do
      post "/shops", params: { shop: { name: "" } }.to_json,
                     headers: auth_headers(user)
      expect(response).to have_http_status(422)
    end
  end
end
