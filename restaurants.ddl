DROP TABLE IF EXISTS restaurants;
CREATE TABLE restaurants (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    address TEXT,
    city VARCHAR(100),
    cuisine_type VARCHAR(100),
    rating DECIMAL(2,1),
    price_range VARCHAR(50), -- e.g., $, $$, $$$
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
