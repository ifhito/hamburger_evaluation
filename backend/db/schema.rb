# This file is auto-generated from the current state of the database. Instead
# of editing this file, please use the migrations feature of Active Record to
# incrementally modify your database, and then regenerate this schema definition.
#
# This file is the source Rails uses to define your schema when running `bin/rails
# db:schema:load`. When creating a new database, `bin/rails db:schema:load` tends to
# be faster and is potentially less error prone than running all of your
# migrations from scratch. Old migrations may fail to apply correctly if those
# migrations use external dependencies or application code.
#
# It's strongly recommended that you check this file into your version control system.

ActiveRecord::Schema[8.0].define(version: 2026_03_25_105924) do
  # These are extensions that must be enabled in order to support this database
  enable_extension "pg_catalog.plpgsql"

  create_table "burger_stats", force: :cascade do |t|
    t.bigint "burger_id", null: false
    t.float "average_rating", default: 0.0, null: false
    t.integer "review_count", default: 0, null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.float "weighted_score", default: 0.0
    t.float "confidence", default: 0.0
    t.index ["burger_id"], name: "index_burger_stats_on_burger_id", unique: true
  end

  create_table "burgers", force: :cascade do |t|
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.string "name", null: false
  end

  create_table "reviews", force: :cascade do |t|
    t.integer "rating"
    t.text "comment"
    t.bigint "user_id", null: false
    t.bigint "burger_id", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.datetime "discarded_at"
    t.index ["burger_id"], name: "index_reviews_on_burger_id"
    t.index ["discarded_at"], name: "index_reviews_on_discarded_at"
    t.index ["user_id"], name: "index_reviews_on_user_id"
  end

  create_table "shops", force: :cascade do |t|
    t.string "name"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
  end

  create_table "shops_and_burgers", force: :cascade do |t|
    t.bigint "shop_id", null: false
    t.bigint "burger_id", null: false
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.index ["burger_id"], name: "index_shops_and_burgers_on_burger_id"
    t.index ["shop_id"], name: "index_shops_and_burgers_on_shop_id"
  end

  create_table "users", force: :cascade do |t|
    t.string "email"
    t.string "password_digest"
    t.string "username"
    t.datetime "created_at", null: false
    t.datetime "updated_at", null: false
    t.datetime "discarded_at"
    t.index ["discarded_at"], name: "index_users_on_discarded_at"
    t.index ["email"], name: "index_users_on_email", unique: true
  end

  add_foreign_key "burger_stats", "burgers"
  add_foreign_key "reviews", "burgers"
  add_foreign_key "reviews", "users"
  add_foreign_key "shops_and_burgers", "burgers"
  add_foreign_key "shops_and_burgers", "shops"
end
