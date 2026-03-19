# This file should ensure the existence of records required to run the application in every environment (production,
# development, test). The code here should be idempotent so that it can be executed at any point in every environment.
# The data can then be loaded with the bin/rails db:seed command (or created alongside the database with db:setup).

puts "Seeding..."

# ----------------------------------------
# Users
# ----------------------------------------
users = [
  { username: "alice",   email: "alice@example.com",   password: "password123" },
  { username: "bob",     email: "bob@example.com",     password: "password123" },
  { username: "charlie", email: "charlie@example.com", password: "password123" }
]

created_users = users.map do |attrs|
  User.find_or_create_by!(email: attrs[:email]) do |u|
    u.username = attrs[:username]
    u.password = attrs[:password]
  end
end

puts "  #{created_users.size} users created"

# ----------------------------------------
# Shops
# ----------------------------------------
shop_names = [
  "Shake Shack 渋谷",
  "J.S. BURGERS CAFE 新宿",
  "フレッシュネスバーガー 原宿"
]

created_shops = shop_names.map do |name|
  Shop.find_or_create_by!(name: name)
end

puts "  #{created_shops.size} shops created"

# ----------------------------------------
# Burgers & ShopsAndBurgers（各店に2つのバーガーを紐付け）
# ----------------------------------------
burger_count = 0
created_shops.each do |shop|
  2.times do
    burger = Burger.create!
    ShopsAndBurger.find_or_create_by!(shop: shop, burger: burger)
    burger_count += 1
  end
end

puts "  #{burger_count} burgers created"

# ----------------------------------------
# Reviews
# ----------------------------------------
all_burgers = Burger.all.to_a
comments = [
  "肉汁たっぷりで最高でした！",
  "バンズがふわふわで美味しかった。",
  "チーズの量がちょうどよかった。",
  "また絶対行きたいです。",
  "ボリューム満点でコスパ良し。",
  "ちょっとしょっぱかったけど美味しい。"
]

review_count = 0
created_users.each_with_index do |user, user_idx|
  all_burgers.sample(3).each_with_index do |burger, i|
    Review.create!(
      user: user,
      burger: burger,
      rating: [ 3, 4, 4, 5, 5 ].sample,
      comment: comments[(user_idx * 3 + i) % comments.size]
    )
    review_count += 1
  end
end

puts "  #{review_count} reviews created"
puts "Done!"
