require "rails_helper"

RSpec.describe "Auth", type: :request do
  describe "POST /signup" do
    let(:valid_params) do
      {
        username: "testuser",
        email: "test@example.com",
        password: "password123",
        password_confirmation: "password123"
      }
    end

    context "with valid parameters" do
      it "creates a user and returns 201 with a token" do
        post "/signup", params: valid_params.to_json,
                        headers: { "Content-Type" => "application/json" }

        expect(response).to have_http_status(:created)
        body = response.parsed_body
        expect(body["token"]).to be_present
        expect(body["email"]).to eq("test@example.com")
      end
    end

    context "with a duplicate email" do
      it "returns 422" do
        create(:user, email: "test@example.com")
        post "/signup", params: valid_params.to_json,
                        headers: { "Content-Type" => "application/json" }

        expect(response).to have_http_status(:unprocessable_entity)
      end
    end

    context "with missing parameters" do
      it "returns 422 when email is absent" do
        post "/signup", params: valid_params.except(:email).to_json,
                        headers: { "Content-Type" => "application/json" }

        expect(response).to have_http_status(:unprocessable_entity)
      end
    end
  end

  describe "POST /login" do
    let!(:user) { create(:user, email: "login@example.com", password: "secret123", password_confirmation: "secret123") }

    context "with correct credentials" do
      it "returns 200 with a token" do
        post "/login", params: { email: "login@example.com", password: "secret123" }.to_json,
                       headers: { "Content-Type" => "application/json" }

        expect(response).to have_http_status(:ok)
        expect(response.parsed_body["token"]).to be_present
      end
    end

    context "with an incorrect password" do
      it "returns 401" do
        post "/login", params: { email: "login@example.com", password: "wrong" }.to_json,
                       headers: { "Content-Type" => "application/json" }

        expect(response).to have_http_status(:unauthorized)
      end
    end

    context "with a discarded user" do
      it "returns 401" do
        user.discard
        post "/login", params: { email: "login@example.com", password: "secret123" }.to_json,
                       headers: { "Content-Type" => "application/json" }

        expect(response).to have_http_status(:unauthorized)
      end
    end
  end

  describe "POST /logout" do
    it "returns 200" do
      post "/logout", headers: { "Content-Type" => "application/json" }
      expect(response).to have_http_status(:ok)
    end
  end
end
