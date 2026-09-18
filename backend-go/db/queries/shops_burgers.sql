-- name: CreateShopBurger :exec
INSERT INTO shops_burgers (shop_id, burger_id)
VALUES ($1, $2);

-- name: ListShopBurgersByShop :many
SELECT * FROM shops_burgers
WHERE shop_id = $1
ORDER BY burger_id;

-- name: DeleteShopBurger :exec
DELETE FROM shops_burgers
WHERE shop_id = $1 AND burger_id = $2;
