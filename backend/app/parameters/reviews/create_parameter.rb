module Reviews
  class CreateParameter < Dry::Struct
    module Types
      include Dry.Types()
    end

    attribute :rating,      Types::Coercible::Integer
    attribute :comment,     Types::String
    attribute :shop_id,     Types::Coercible::Integer
    attribute :burger_name, Types::String
  end
end
