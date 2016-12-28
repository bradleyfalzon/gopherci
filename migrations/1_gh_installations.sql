-- +migrate Up
CREATE TABLE gh_installations (
    id INT UNSIGNED NOT NULL AUTO_INCREMENT,
    installation_id INT UNSIGNED NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enabled_at TIMESTAMP NULL DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE installation_id (installation_id)
);

-- +migrate Down
DROP TABLE gh_installations;
