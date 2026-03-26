require "rails_helper"

RSpec.describe "Users", type: :request do
  let(:user) { create(:user) }

  describe "GET /users" do
    it "returns only kept users" do
      active    = create(:user)
      discarded = create(:user)
      discarded.discard

      get "/users"

      expect(response).to have_http_status(:ok)
      ids = response.parsed_body.map { |u| u["id"] }
      expect(ids).to include(active.id)
      expect(ids).not_to include(discarded.id)
    end
  end

  describe "PUT /users/:id" do
    context "when authenticated" do
      it "updates the user's own profile and returns 200" do
        put "/users/#{user.id}",
            params: { user: { username: "new_name" } }.to_json,
            headers: auth_headers(user)
        expect(response).to have_http_status(:ok)
        expect(response.parsed_body["username"]).to eq("new_name")
      end
    end

    context "when unauthenticated" do
      it "returns 401" do
        put "/users/#{user.id}",
            params: { user: { username: "hacked" } }.to_json,
            headers: { "Content-Type" => "application/json" }
        expect(response).to have_http_status(:unauthorized)
      end
    end

    context "updating another user's profile" do
      it "returns 403" do
        other_user = create(:user)
        put "/users/#{user.id}",
            params: { user: { username: "hacked" } }.to_json,
            headers: auth_headers(other_user)
        expect(response).to have_http_status(:forbidden)
      end
    end
  end

  describe "DELETE /users/:id" do
    context "when authenticated" do
      it "soft-deletes the user and returns 204" do
        delete "/users/#{user.id}", headers: auth_headers(user)
        expect(response).to have_http_status(:no_content)
        expect(user.reload.discarded_at).not_to be_nil
      end
    end

    context "deleting another user's account" do
      it "returns 403" do
        other_user = create(:user)
        delete "/users/#{user.id}", headers: auth_headers(other_user)
        expect(response).to have_http_status(:forbidden)
      end
    end
  end
end
