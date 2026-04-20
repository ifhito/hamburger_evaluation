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

    context "when authenticated as a different user" do
      it "returns 403 and does not modify the target user" do
        other_user = create(:user)
        original_username = other_user.username
        put "/users/#{other_user.id}",
            params: { user: { username: "hacked" } }.to_json,
            headers: auth_headers(user)
        expect(response).to have_http_status(:forbidden)
        expect(other_user.reload.username).to eq(original_username)
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

    context "when authenticated as a different user" do
      it "returns 403 and does not delete the target user" do
        other_user = create(:user)
        delete "/users/#{other_user.id}", headers: auth_headers(user)
        expect(response).to have_http_status(:forbidden)
        expect(other_user.reload.discarded_at).to be_nil
      end
    end
  end
end
