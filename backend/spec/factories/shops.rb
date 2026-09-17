FactoryBot.define do
  factory :shop do
    name { Faker::Restaurant.name }
    status { :active }

    trait :pending do
      status { :pending }
    end

    trait :rejected do
      status { :rejected }
    end
  end
end
