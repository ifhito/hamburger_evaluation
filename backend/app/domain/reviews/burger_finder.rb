module Reviews
  class BurgerFinder
    def find_for_shop(shop, burger_name)
      shop.burgers.find_by(name: burger_name)
    end
  end
end
