require "rails_helper"

RSpec.describe "Admin::Shops", type: :request do
  let(:admin) { create(:user, :admin) }
  let(:user)  { create(:user) }

  describe "GET /admin/shops" do
    it "requires authentication" do
      get "/admin/shops"
      expect(response).to have_http_status(:unauthorized)
    end

    it "forbids non-admins" do
      get "/admin/shops", headers: auth_headers(user)
      expect(response).to have_http_status(:forbidden)
    end

    it "lists all shops for admins" do
      create(:shop, :pending, name: "PendingAdmin")
      get "/admin/shops", headers: auth_headers(admin)
      expect(response).to have_http_status(:ok)
      names = response.parsed_body.map { |s| s["name"] }
      expect(names).to include("PendingAdmin")
    end

    it "filters by status" do
      create(:shop, :pending, name: "P1")
      create(:shop, name: "A1")
      get "/admin/shops?status=pending", headers: auth_headers(admin)
      names = response.parsed_body.map { |s| s["name"] }
      expect(names).to include("P1")
      expect(names).not_to include("A1")
    end
  end

  describe "POST /admin/shops/:id/approve" do
    it "activates a pending shop" do
      shop = create(:shop, :pending)
      post "/admin/shops/#{shop.id}/approve", headers: auth_headers(admin)
      expect(response).to have_http_status(:ok)
      expect(response.parsed_body["status"]).to eq("active")
      expect(shop.reload).to be_active
    end

    it "forbids non-admins" do
      shop = create(:shop, :pending)
      post "/admin/shops/#{shop.id}/approve", headers: auth_headers(user)
      expect(response).to have_http_status(:forbidden)
    end

    it "returns 404 for a missing shop" do
      post "/admin/shops/0/approve", headers: auth_headers(admin)
      expect(response).to have_http_status(:not_found)
    end
  end

  describe "POST /admin/shops/:id/reject" do
    it "rejects a shop with a moderation note" do
      shop = create(:shop, :pending)
      post "/admin/shops/#{shop.id}/reject",
           params: { moderation_note: "不適切な内容" }.to_json,
           headers: auth_headers(admin)
      expect(response).to have_http_status(:ok)
      body = response.parsed_body
      expect(body["status"]).to eq("rejected")
      expect(body["moderation_note"]).to eq("不適切な内容")
      expect(shop.reload).to be_rejected
    end
  end

  describe "PUT /admin/shops/:id" do
    it "updates the shop name" do
      shop = create(:shop, name: "Old")
      put "/admin/shops/#{shop.id}",
          params: { shop: { name: "New" } }.to_json,
          headers: auth_headers(admin)
      expect(response).to have_http_status(:ok)
      expect(shop.reload.name).to eq("New")
    end

    it "forbids non-admins" do
      shop = create(:shop)
      put "/admin/shops/#{shop.id}",
          params: { shop: { name: "Nope" } }.to_json,
          headers: auth_headers(user)
      expect(response).to have_http_status(:forbidden)
    end
  end
end
